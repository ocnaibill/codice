package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
)

// PersonalCollectionsHandler is how a person keeps their own collections, the lists of works (#207, DEC-130, RN-006). They are
// the person's alone: every query is made by the owner, and what belongs to another person is not found, never forbidden.
// Nothing here touches the metadata of a work, or the official collections: a list only points at works.
type PersonalCollectionsHandler struct{ DB *sql.DB }

const (
	// The most lists a person keeps, and the most works in one of them: a bound for what one account can pile up.
	maxPersonalCollections = 200
	maxPersonalEntries     = 5000
)

func (h *PersonalCollectionsHandler) begin(w http.ResponseWriter, r *http.Request) (*sql.Tx, bool) {
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return nil, false
	}
	return tx, true
}

func (h *PersonalCollectionsHandler) fail(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, errCollectionNotFound) {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	log.Println("Error in the personal collections ("+what+"):", err)
	http.Error(w, "Error managing the collection", http.StatusInternalServerError)
}

// lockMine reads and locks a personal collection of the caller; one that is not theirs is not found.
func (h *PersonalCollectionsHandler) lockMine(tx *sql.Tx, r *http.Request, id int64) (retired bool, err error) {
	var gone sql.NullTime
	err = tx.QueryRowContext(r.Context(), `
		SELECT retired_at FROM collections WHERE id = $1 AND kind = 'personal' AND owner_id = $2::uuid FOR NO KEY UPDATE`,
		id, currentUserID(r)).Scan(&gone)
	if errors.Is(err, sql.ErrNoRows) {
		return false, errCollectionNotFound
	}
	return gone.Valid, err
}

// Create answers POST /my/collections {"name": "..."}: an empty list of the caller.
func (h *PersonalCollectionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	userID := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	// One person making lists twice at once must not slip past the bound.
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('personal-collections:' || $1::text, 0))`, userID); err != nil {
		h.fail(w, "create", err)
		return
	}
	var have int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM collections WHERE kind = 'personal' AND owner_id = $1::uuid AND retired_at IS NULL AND system_key IS NULL`, userID).Scan(&have); err != nil {
		h.fail(w, "create", err)
		return
	}
	if have >= maxPersonalCollections {
		conflict(w, "Você já tem "+strconv.Itoa(maxPersonalCollections)+" listas: aposente alguma antes de fazer outra.", 0)
		return
	}
	var id int64
	if err := tx.QueryRowContext(r.Context(), `INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1::uuid, $2, 'manual') RETURNING id`, userID, name).Scan(&id); err != nil {
		h.fail(w, "create", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "create", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id, "name": name})
}

// Rename answers PATCH /my/collections/{id} {"name": "..."}.
func (h *PersonalCollectionsHandler) Rename(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := h.lockMine(tx, r, id)
	if err != nil {
		h.fail(w, "rename", err)
		return
	}
	if retired {
		conflict(w, "A lista está aposentada: restaure antes de mudar.", id)
		return
	}
	if system, err := isSystemList(tx, r, id); err != nil {
		h.fail(w, "rename", err)
		return
	} else if system {
		conflict(w, "Essa lista é do Códice: ela não muda de nome.", id)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE collections SET name = $2, edited_at = now() WHERE id = $1`, id, name); err != nil {
		h.fail(w, "rename", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "rename", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id, "name": name})
}

// AddWork answers PUT /my/collections/{id}/works/{workId} {"position": 3}: the work goes into the list, at the end unless a
// place is given; one that is in already takes the new place, and with no place stays where it is. A list can hold a work
// once.
func (h *PersonalCollectionsHandler) AddWork(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	work64, ok2 := collectionIDParam(r, "workId")
	if !ok || !ok2 || work64 > math.MaxInt32 {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	var req struct {
		Position *float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.Position != nil && (*req.Position < 0 || math.IsNaN(*req.Position) || math.IsInf(*req.Position, 0)) {
		http.Error(w, "A posição não pode ser negativa.", http.StatusBadRequest)
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := h.lockMine(tx, r, id)
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	if retired {
		conflict(w, "A lista está aposentada: restaure antes de mudar.", id)
		return
	}
	var workRetired bool
	if err := tx.QueryRowContext(r.Context(), `SELECT retired_at IS NOT NULL FROM works WHERE id = $1`, work64).Scan(&workRetired); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		h.fail(w, "add", err)
		return
	}
	if workRetired {
		http.Error(w, "A obra está na lixeira.", http.StatusConflict)
		return
	}
	var entries int
	var member bool
	if err := tx.QueryRowContext(r.Context(), `
		SELECT count(*), COALESCE(bool_or(work_id = $2), FALSE) FROM collection_works WHERE collection_id = $1`, id, work64).Scan(&entries, &member); err != nil {
		h.fail(w, "add", err)
		return
	}
	switch {
	case member && req.Position == nil:
		// already there, nothing to say
	case member:
		if _, err := tx.ExecContext(r.Context(), `UPDATE collection_works SET position = $3 WHERE collection_id = $1 AND work_id = $2`, id, work64, *req.Position); err != nil {
			h.fail(w, "add", err)
			return
		}
	default:
		if entries >= maxPersonalEntries {
			conflict(w, "A lista já tem "+strconv.Itoa(maxPersonalEntries)+" obras.", id)
			return
		}
		position := 0.0
		if req.Position != nil {
			position = *req.Position
		} else if err := tx.QueryRowContext(r.Context(), `SELECT COALESCE(max(position), 0) + 1 FROM collection_works WHERE collection_id = $1`, id).Scan(&position); err != nil {
			h.fail(w, "add", err)
			return
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, $3)`, id, work64, position); err != nil {
			h.fail(w, "add", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "add", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveEntry answers DELETE /my/collections/{id}/entries/{entryId}: the place of a work in the list goes. It is the place that
// is named, and not the work, because the work may have left the library, and the list still shows what it was.
func (h *PersonalCollectionsHandler) RemoveEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	entry, ok2 := collectionIDParam(r, "entryId")
	if !ok || !ok2 {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := h.lockMine(tx, r, id)
	if err != nil {
		h.fail(w, "remove", err)
		return
	}
	if retired {
		conflict(w, "A lista está aposentada: restaure antes de mudar.", id)
		return
	}
	res, err := tx.ExecContext(r.Context(), `DELETE FROM collection_works WHERE id = $2 AND collection_id = $1`, id, entry)
	if err != nil {
		h.fail(w, "remove", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "A obra não está nessa lista.", http.StatusNotFound)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "remove", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Order answers PUT /my/collections/{id}/order {"entryIds": [3, 1, 2]}: the places get the numbers 1, 2, 3... in that order. The
// list must be all the places of the collection, each once.
func (h *PersonalCollectionsHandler) Order(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	var req struct {
		EntryIDs []int64 `json:"entryIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := h.lockMine(tx, r, id)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	if retired {
		conflict(w, "A lista está aposentada: restaure antes de mudar.", id)
		return
	}
	rows, err := tx.QueryContext(r.Context(), `SELECT id FROM collection_works WHERE collection_id = $1`, id)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	have := map[int64]bool{}
	for rows.Next() {
		var entry int64
		if err := rows.Scan(&entry); err != nil {
			rows.Close()
			h.fail(w, "order", err)
			return
		}
		have[entry] = true
	}
	rows.Close()
	seen := map[int64]bool{}
	for _, entry := range req.EntryIDs {
		if !have[entry] || seen[entry] {
			http.Error(w, "A lista deve ter cada obra da lista uma vez, e só elas.", http.StatusBadRequest)
			return
		}
		seen[entry] = true
	}
	if len(seen) != len(have) {
		http.Error(w, "A lista deve ter cada obra da lista uma vez, e só elas.", http.StatusBadRequest)
		return
	}
	for i, entry := range req.EntryIDs {
		if _, err := tx.ExecContext(r.Context(), `UPDATE collection_works SET position = $2 WHERE id = $1`, entry, float64(i+1)); err != nil {
			h.fail(w, "order", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "order", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Retire answers DELETE /my/collections/{id}: the list is put away, with its works in it, and can be restored. Doing it twice
// is not an error.
func (h *PersonalCollectionsHandler) Retire(w http.ResponseWriter, r *http.Request) {
	h.setRetired(w, r, true)
}

// Restore answers POST /my/collections/{id}/restore.
func (h *PersonalCollectionsHandler) Restore(w http.ResponseWriter, r *http.Request) {
	h.setRetired(w, r, false)
}

func (h *PersonalCollectionsHandler) setRetired(w http.ResponseWriter, r *http.Request, retire bool) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := h.lockMine(tx, r, id)
	if err != nil {
		h.fail(w, "retire", err)
		return
	}
	if system, err := isSystemList(tx, r, id); err != nil {
		h.fail(w, "retire", err)
		return
	} else if system {
		conflict(w, "Essa lista é do Códice: ela não sai. Tire as obras que não quer mais nela.", id)
		return
	}
	if retired == retire {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !retire {
		// Coming back counts against the bound like making a new one.
		if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('personal-collections:' || $1::text, 0))`, currentUserID(r)); err != nil {
			h.fail(w, "restore", err)
			return
		}
		var have int
		if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM collections WHERE kind = 'personal' AND owner_id = $1::uuid AND retired_at IS NULL AND system_key IS NULL`, currentUserID(r)).Scan(&have); err != nil {
			h.fail(w, "restore", err)
			return
		}
		if have >= maxPersonalCollections {
			conflict(w, "Você já tem "+strconv.Itoa(maxPersonalCollections)+" listas: aposente alguma antes de restaurar esta.", id)
			return
		}
	}
	set := `retired_at = now()`
	if !retire {
		set = `retired_at = NULL`
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE collections SET `+set+` WHERE id = $1`, id); err != nil {
		h.fail(w, "retire", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "retire", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
