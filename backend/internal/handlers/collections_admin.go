package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// CollectionsAdminHandler is how owner and admin manage the official collections (#205, DEC-130).
//
// The membership of an official collection is the series metadata of the work (the trigger of migration 00052 follows it),
// so every change here is a change of `series` and `series_index` of the works, made like any other manual edit of the
// metadata: the field is confirmed, locked against the automatic analysis, and its source is "manual". That is what makes
// the manual decision win, and nothing is written to a file.
type CollectionsAdminHandler struct{ DB *sql.DB }

const maxCollectionName = 512

var errCollectionNotFound = errors.New("collection not found")

// collectionName cleans what a person typed: the stray spaces go, as they do in the name a series gives.
func collectionName(s string) (string, bool) {
	name := strings.Join(strings.Fields(s), " ")
	return name, name != "" && utf8.RuneCountInString(name) <= maxCollectionName
}

// describeCollection sets the description of a collection (empty clears it). The collection counts as made the person's own, so
// it is shown even with no works in it.
func describeCollection(ctx context.Context, tx *sql.Tx, id int64, text string) error {
	_, err := tx.ExecContext(ctx, `UPDATE collections SET description = NULLIF($2, ''), edited_at = now() WHERE id = $1`, id, text)
	return err
}

// setReadingDirection sets how a series is read; empty takes it away.
func setReadingDirection(ctx context.Context, tx *sql.Tx, id int64, direction string) error {
	_, err := tx.ExecContext(ctx, `UPDATE collections SET reading_direction = NULLIF($2, '') WHERE id = $1`, id, direction)
	return err
}

// lockOfficial reads and locks an official collection of the library for the transaction.
func lockOfficial(ctx context.Context, tx *sql.Tx, id int64) (name string, retired bool, err error) {
	var gone sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT name, retired_at FROM collections WHERE id = $1 AND kind = 'official' FOR NO KEY UPDATE`, id).Scan(&name, &gone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, errCollectionNotFound
	}
	return name, gone.Valid, err
}

// setSeries writes the series of a work as a person does: the fields that change are confirmed and locked, and the series
// is locked even if nothing changed, since the person has just said where the work belongs.
func setSeries(ctx context.Context, tx *sql.Tx, workID int, actor, series string, index float64) error {
	cur, retired, err := readWorkFields(tx, workID)
	if err != nil {
		return err
	}
	next := cur
	next.Series, next.SeriesIndex = series, index
	yes := true
	_, err = applyWorkFields(ctx, tx, workID, actor, sourceManual, cur, retired, next, map[string]*bool{"series": &yes})
	return err
}

// keyOwner is the collection that already has the key of a name, 0 if none does.
func keyOwner(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT collection_id FROM collection_keys WHERE key = series_key($1)`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func collectionIDParam(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func (h *CollectionsAdminHandler) begin(w http.ResponseWriter, r *http.Request) (*sql.Tx, bool) {
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return nil, false
	}
	return tx, true
}

func (h *CollectionsAdminHandler) fail(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, errCollectionNotFound) {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	log.Println("Error in the management of collections ("+what+"):", err)
	http.Error(w, "Error managing the collection", http.StatusInternalServerError)
}

func conflict(w http.ResponseWriter, message string, id int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]any{"error": message, "collectionId": id})
}

const maxCollectionDescription = 2000

// collectionEdit is what PATCH of a collection carries (DEC-163): a name, a description, or both. A description sent empty clears it.
type collectionEdit struct {
	name        *string
	description *string
	// direction is how the series is read ("ltr", "rtl", "webtoon"), or "" to take it away (DEC-166); only an official collection has one.
	direction *string
}

// decodeEdit reads the body of a PATCH of a collection, with the same name rules as decodeName. The description keeps its line
// breaks (it is a few lines of text) and loses the spaces around it.
func decodeEdit(w http.ResponseWriter, r *http.Request) (collectionEdit, bool) {
	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		ReadingDirection *string `json:"readingDirection"`
	}
	var e collectionEdit
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return e, false
	}
	if req.Name == nil && req.Description == nil && req.ReadingDirection == nil {
		http.Error(w, "Diga o nome, a descrição ou a direção de leitura.", http.StatusBadRequest)
		return e, false
	}
	if req.Name != nil {
		name, ok := collectionName(*req.Name)
		if !ok {
			http.Error(w, "O nome da coleção é obrigatório e tem até 512 caracteres.", http.StatusBadRequest)
			return e, false
		}
		e.name = &name
	}
	if req.Description != nil {
		text := strings.TrimSpace(strings.ReplaceAll(*req.Description, "\r\n", "\n"))
		if utf8.RuneCountInString(text) > maxCollectionDescription {
			http.Error(w, "A descrição tem até 2000 caracteres.", http.StatusBadRequest)
			return e, false
		}
		e.description = &text
	}
	if req.ReadingDirection != nil {
		switch *req.ReadingDirection {
		case "", "ltr", "rtl", "webtoon":
			e.direction = req.ReadingDirection
		default:
			http.Error(w, "A direção é esquerda para a direita, direita para a esquerda ou tira para rolar.", http.StatusBadRequest)
			return e, false
		}
	}
	return e, true
}

func decodeName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return "", false
	}
	name, ok := collectionName(req.Name)
	if !ok {
		http.Error(w, "O nome da coleção é obrigatório e tem até 512 caracteres.", http.StatusBadRequest)
		return "", false
	}
	return name, true
}

// Create answers POST /collections {"name": "..."}: a collection made by hand, empty, that the works join when a person
// adds them. A name that leads to a collection already (retired ones too) is refused, with the one it leads to.
func (h *CollectionsAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	// The same lock the trigger takes for this key: a work that comes with the same series at this moment waits for us.
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('collection:' || series_key($1), 0))`, name); err != nil {
		h.fail(w, "create", err)
		return
	}
	if id, err := keyOwner(r.Context(), tx, name); err != nil {
		h.fail(w, "create", err)
		return
	} else if id != 0 {
		conflict(w, "Já existe uma coleção com esse nome.", id)
		return
	}
	var id int64
	if err := tx.QueryRowContext(r.Context(), `INSERT INTO collections (kind, name, origin) VALUES ('official', $1, 'manual') RETURNING id`, name).Scan(&id); err != nil {
		h.fail(w, "create", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO collection_keys (key, collection_id) VALUES (series_key($1), $2)`, name, id); err != nil {
		h.fail(w, "create", err)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "collection.create", "collection", strconv.FormatInt(id, 10), map[string]any{"name": name}); err != nil {
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

// Rename answers PATCH /collections/{id} {"name": "...", "description": "...", "readingDirection": "rtl"}, any of them: the name is the person's from now on. The text the collection was
// born from stays attached to it (a work that comes with it still joins), and the works in it get the new name as their
// series, so what a work says and what the collection says do not part.
func (h *CollectionsAdminHandler) Rename(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	edit, ok := decodeEdit(w, r)
	if !ok {
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	old, retired, err := lockOfficial(r.Context(), tx, id)
	if err != nil {
		h.fail(w, "rename", err)
		return
	}
	if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	if edit.name == nil {
		// No name: the description or the direction, and no series is touched.
		actor := currentUserID(r)
		details := map[string]any{"name": old}
		out := map[string]any{"id": id, "name": old}
		if edit.description != nil {
			if err := describeCollection(r.Context(), tx, id, *edit.description); err != nil {
				h.fail(w, "describe", err)
				return
			}
			details["length"] = utf8.RuneCountInString(*edit.description)
			out["description"] = *edit.description
		}
		if edit.direction != nil {
			if err := setReadingDirection(r.Context(), tx, id, *edit.direction); err != nil {
				h.fail(w, "describe", err)
				return
			}
			details["readingDirection"] = *edit.direction
			out["readingDirection"] = *edit.direction
		}
		if err := audit.Record(r.Context(), tx, actor, "collection.describe", "collection", strconv.FormatInt(id, 10), details); err != nil {
			h.fail(w, "describe", err)
			return
		}
		if err := tx.Commit(); err != nil {
			h.fail(w, "describe", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}
	name := *edit.name
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('collection:' || series_key($1), 0))`, name); err != nil {
		h.fail(w, "rename", err)
		return
	}
	if other, err := keyOwner(r.Context(), tx, name); err != nil {
		h.fail(w, "rename", err)
		return
	} else if other != 0 && other != id {
		conflict(w, "Já existe outra coleção com esse nome.", other)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO collection_keys (key, collection_id) VALUES (series_key($1), $2) ON CONFLICT DO NOTHING`, name, id); err != nil {
		h.fail(w, "rename", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE collections SET name = $2, edited_at = now() WHERE id = $1`, id, name); err != nil {
		h.fail(w, "rename", err)
		return
	}
	if edit.description != nil {
		if err := describeCollection(r.Context(), tx, id, *edit.description); err != nil {
			h.fail(w, "rename", err)
			return
		}
	}
	if edit.direction != nil {
		if err := setReadingDirection(r.Context(), tx, id, *edit.direction); err != nil {
			h.fail(w, "rename", err)
			return
		}
	}
	rows, err := tx.QueryContext(r.Context(), `
		SELECT cw.work_id, COALESCE(w.series_index, 0) FROM collection_works cw JOIN works w ON w.id = cw.work_id
		WHERE cw.collection_id = $1 ORDER BY cw.work_id`, id)
	if err != nil {
		h.fail(w, "rename", err)
		return
	}
	type member struct {
		id    int
		index float64
	}
	var members []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.id, &m.index); err != nil {
			rows.Close()
			h.fail(w, "rename", err)
			return
		}
		members = append(members, m)
	}
	rows.Close()
	actor := currentUserID(r)
	for _, m := range members {
		if err := setSeries(r.Context(), tx, m.id, actor, name, m.index); err != nil {
			h.fail(w, "rename", err)
			return
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "collection.rename", "collection", strconv.FormatInt(id, 10),
		map[string]any{"from": old, "to": name, "works": len(members)}); err != nil {
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

// AddWork answers PUT /collections/{id}/works/{workId} {"position": 3}: the work joins the collection (leaving the official
// one it was in), or, if it is in already, takes the new number. Without a position, a work that joins goes to the end.
func (h *CollectionsAdminHandler) AddWork(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	work64, ok2 := collectionIDParam(r, "workId")
	if !ok || !ok2 || work64 > math.MaxInt32 {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	workID := int(work64)
	var req struct {
		Position *float64 `json:"position"`
	}
	// No body at all means "at the end".
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.Position != nil && *req.Position < 0 {
		http.Error(w, "A posição não pode ser negativa.", http.StatusBadRequest)
		return
	}
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	name, retired, err := lockOfficial(r.Context(), tx, id)
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	cur, workRetired, err := readWorkFields(tx, workID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	if workRetired {
		http.Error(w, "A obra está na lixeira.", http.StatusConflict)
		return
	}
	var member bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM collection_works WHERE collection_id = $1 AND work_id = $2)`, id, workID).Scan(&member); err != nil {
		h.fail(w, "add", err)
		return
	}
	position := cur.SeriesIndex
	switch {
	case req.Position != nil:
		position = *req.Position
	case !member:
		// At the end of its group: after the highest number the collection has for that unit (volumes, chapters and the rest are
		// numbered each on its own, #187).
		if err := tx.QueryRowContext(r.Context(), `
			SELECT COALESCE(max(cw.position), 0) + 1 FROM collection_works cw JOIN works w ON w.id = cw.work_id
			WHERE cw.collection_id = $1 AND COALESCE(w.unit, '') = $2`, id, cur.Unit).Scan(&position); err != nil {
			h.fail(w, "add", err)
			return
		}
	}
	actor := currentUserID(r)
	if err := setSeries(r.Context(), tx, workID, actor, name, position); err != nil {
		h.fail(w, "add", err)
		return
	}
	if err := audit.Record(r.Context(), tx, actor, "collection.add_work", "collection", strconv.FormatInt(id, 10),
		map[string]any{"work": workID, "position": position, "was": cur.Series}); err != nil {
		h.fail(w, "add", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "add", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveWork answers DELETE /collections/{id}/works/{workId}: the work leaves the collection. Its series is cleared and
// locked, so that the analysis of the file does not put it back.
func (h *CollectionsAdminHandler) RemoveWork(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	work64, ok2 := collectionIDParam(r, "workId")
	if !ok || !ok2 || work64 > math.MaxInt32 {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	workID := int(work64)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	if _, retired, err := lockOfficial(r.Context(), tx, id); err != nil {
		h.fail(w, "remove", err)
		return
	} else if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	var cur string
	err := tx.QueryRowContext(r.Context(), `SELECT COALESCE(w.series, '') FROM collection_works cw JOIN works w ON w.id = cw.work_id WHERE cw.collection_id = $1 AND cw.work_id = $2`, id, workID).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "A obra não está nessa coleção.", http.StatusNotFound)
		return
	}
	if err != nil {
		h.fail(w, "remove", err)
		return
	}
	actor := currentUserID(r)
	if err := setSeries(r.Context(), tx, workID, actor, "", 0); err != nil {
		h.fail(w, "remove", err)
		return
	}
	if err := audit.Record(r.Context(), tx, actor, "collection.remove_work", "collection", strconv.FormatInt(id, 10),
		map[string]any{"work": workID, "series": cur}); err != nil {
		h.fail(w, "remove", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "remove", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Order answers PUT /collections/{id}/order {"workIds": [3, 1, 2]}: the works get the numbers 1, 2, 3... in that order. The
// list must be the works of the collection, all of them.
func (h *CollectionsAdminHandler) Order(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	var req struct {
		WorkIDs []int `json:"workIds"`
		// Unit, when it is there, says the list is of one group of the collection (#187): "volume", "chapter", "oneshot", "extra", or ""
		// for the works with no unit. The numbers are only of that group; the rest of the collection is left as it is.
		Unit *string `json:"unit"`
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
	name, retired, err := lockOfficial(r.Context(), tx, id)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	if req.Unit != nil && !validUnit(*req.Unit) {
		http.Error(w, "A unidade é volume, capítulo, único ou complementar.", http.StatusBadRequest)
		return
	}
	// Every work of the collection, or only those of the group that was asked for.
	group, all := "", req.Unit == nil
	if !all {
		group = *req.Unit
	}
	rows, err := tx.QueryContext(r.Context(), `
		SELECT cw.work_id FROM collection_works cw JOIN works w ON w.id = cw.work_id
		WHERE cw.collection_id = $1 AND w.retired_at IS NULL AND ($2 OR COALESCE(w.unit, '') = $3)`, id, all, group)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	have := map[int]bool{}
	for rows.Next() {
		var wid int
		if err := rows.Scan(&wid); err != nil {
			rows.Close()
			h.fail(w, "order", err)
			return
		}
		have[wid] = true
	}
	rows.Close()
	seen := map[int]bool{}
	for _, wid := range req.WorkIDs {
		if !have[wid] || seen[wid] {
			http.Error(w, "A lista deve ter cada obra da coleção uma vez, e só elas.", http.StatusBadRequest)
			return
		}
		seen[wid] = true
	}
	if len(seen) != len(have) {
		http.Error(w, "A lista deve ter cada obra da coleção uma vez, e só elas.", http.StatusBadRequest)
		return
	}
	actor := currentUserID(r)
	for i, wid := range req.WorkIDs {
		if err := setSeries(r.Context(), tx, wid, actor, name, float64(i+1)); err != nil {
			h.fail(w, "order", err)
			return
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "collection.order", "collection", strconv.FormatInt(id, 10), map[string]any{"works": req.WorkIDs}); err != nil {
		h.fail(w, "order", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "order", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Retire answers DELETE /collections/{id}: the collection is retired. Its works are not touched (they keep their series),
// they are just in no official collection while it is retired, and it can be restored. Retiring twice is not an error.
func (h *CollectionsAdminHandler) Retire(w http.ResponseWriter, r *http.Request) {
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
	if _, retired, err := lockOfficial(r.Context(), tx, id); err != nil {
		h.fail(w, "retire", err)
		return
	} else if retired {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	res, err := tx.ExecContext(r.Context(), `DELETE FROM collection_works WHERE collection_id = $1`, id)
	if err != nil {
		h.fail(w, "retire", err)
		return
	}
	works, _ := res.RowsAffected()
	if _, err := tx.ExecContext(r.Context(), `UPDATE collections SET retired_at = now() WHERE id = $1`, id); err != nil {
		h.fail(w, "retire", err)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "collection.retire", "collection", strconv.FormatInt(id, 10), map[string]any{"works": works}); err != nil {
		h.fail(w, "retire", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "retire", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Restore answers POST /collections/{id}/restore: the collection is back, with the works whose series leads to it.
func (h *CollectionsAdminHandler) Restore(w http.ResponseWriter, r *http.Request) {
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
	if _, retired, err := lockOfficial(r.Context(), tx, id); err != nil {
		h.fail(w, "restore", err)
		return
	} else if !retired {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE collections SET retired_at = NULL WHERE id = $1`, id); err != nil {
		h.fail(w, "restore", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		SELECT collection_sync_work(w.id, w.series, w.series_index) FROM works w
		WHERE series_key(w.series) IN (SELECT key FROM collection_keys WHERE collection_id = $1) ORDER BY w.id`, id); err != nil {
		h.fail(w, "restore", err)
		return
	}
	var works int
	tx.QueryRowContext(r.Context(), `SELECT count(*) FROM collection_works WHERE collection_id = $1`, id).Scan(&works)
	if err := audit.Record(r.Context(), tx, currentUserID(r), "collection.restore", "collection", strconv.FormatInt(id, 10), map[string]any{"works": works}); err != nil {
		h.fail(w, "restore", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "restore", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Classify answers PUT /collections/{id}/classification {"unit": "chapter", "comicKind": "manga", "onlyUnset": true}: the works of the
// collection get the unit and the kind of comic that are given (#187). A field that is not there is left alone; an empty one clears.
// With onlyUnset the works that already have a value for a field keep it. The works in the trash are left as they are. It says how
// many works each field changed on.
func (h *CollectionsAdminHandler) Classify(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	var req struct {
		Unit      *string `json:"unit"`
		ComicKind *string `json:"comicKind"`
		OnlyUnset bool    `json:"onlyUnset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.Unit == nil && req.ComicKind == nil {
		http.Error(w, "Diga a unidade ou o tipo.", http.StatusBadRequest)
		return
	}
	if req.Unit != nil && !validUnit(*req.Unit) {
		http.Error(w, "A unidade é volume, capítulo, único ou complementar.", http.StatusBadRequest)
		return
	}
	if req.ComicKind != nil && !validComicKind(*req.ComicKind) {
		http.Error(w, "O tipo é quadrinho ou mangá.", http.StatusBadRequest)
		return
	}
	actor := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	if _, retired, err := lockOfficial(r.Context(), tx, id); err != nil {
		h.fail(w, "classify", err)
		return
	} else if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	changed := map[string]int{}
	for _, f := range []struct {
		field, column string
		value         *string
	}{{"unit", "unit", req.Unit}, {"comic_kind", "comic_kind", req.ComicKind}} {
		if f.value == nil {
			continue
		}
		rows, err := tx.QueryContext(r.Context(), `
			UPDATE works SET `+f.column+` = NULLIF($2, ''), updated_at = CURRENT_TIMESTAMP
			WHERE id IN (SELECT cw.work_id FROM collection_works cw WHERE cw.collection_id = $1)
			  AND retired_at IS NULL AND `+f.column+` IS DISTINCT FROM NULLIF($2, '') AND (NOT $3 OR `+f.column+` IS NULL)
			RETURNING id`, id, *f.value, req.OnlyUnset)
		if err != nil {
			h.fail(w, "classify", err)
			return
		}
		var ids []int64
		for rows.Next() {
			var wid int64
			if err := rows.Scan(&wid); err != nil {
				rows.Close()
				h.fail(w, "classify", err)
				return
			}
			ids = append(ids, wid)
		}
		rows.Close()
		changed[f.field] = len(ids)
		if len(ids) > 0 {
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO work_field_sources (work_id, field, source, actor_id)
				SELECT unnest($1::int[]), $2, $3, NULLIF($4, '')::uuid
				ON CONFLICT (work_id, field) DO UPDATE SET source = EXCLUDED.source, actor_id = EXCLUDED.actor_id, updated_at = now()`,
				pq.Array(ids), f.field, sourceManual, actor); err != nil {
				h.fail(w, "classify", err)
				return
			}
		}
	}
	details := map[string]any{"changed": changed, "onlyUnset": req.OnlyUnset}
	if req.Unit != nil {
		details["unit"] = *req.Unit
	}
	if req.ComicKind != nil {
		details["comicKind"] = *req.ComicKind
	}
	if err := audit.Record(r.Context(), tx, actor, "collection.classify", "collection", strconv.FormatInt(id, 10), details); err != nil {
		h.fail(w, "classify", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "classify", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"changed": changed})
}

// Group answers PUT /collections/{id}/grouping (staff, DEC-169): says, for the chapters of a range, the bound volume that collected them and/or the
// story arc they are in, in one step. The range is by number in the series (`from` to `to`, both ends in); with none, every work of the unit; with a
// `unit`, only the works of that unit. An empty `storyArc` or `volumeNumber` takes the value away; one that is not sent is left alone. A work out
// of the library is not touched, and what changes is confirmed by hand, like the unit.
func (h *CollectionsAdminHandler) Group(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	var req struct {
		Unit         *string  `json:"unit"`
		From         *float64 `json:"from"`
		To           *float64 `json:"to"`
		StoryArc     *string  `json:"storyArc"`
		VolumeNumber *string  `json:"volumeNumber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.StoryArc == nil && req.VolumeNumber == nil {
		http.Error(w, "Diga o arco ou o volume.", http.StatusBadRequest)
		return
	}
	if req.Unit != nil && !validUnit(*req.Unit) {
		http.Error(w, "A unidade é volume, capítulo, único ou complementar.", http.StatusBadRequest)
		return
	}
	if req.From != nil && req.To != nil && *req.From > *req.To {
		http.Error(w, "O começo da faixa não pode passar do fim.", http.StatusBadRequest)
		return
	}
	var arc, volume string
	if req.StoryArc != nil {
		var ok bool
		if arc, ok = cleanStoryArc(*req.StoryArc); !ok {
			http.Error(w, "O arco tem até 255 caracteres.", http.StatusBadRequest)
			return
		}
	}
	if req.VolumeNumber != nil {
		var ok bool
		if volume, ok = cleanVolumeNumber(*req.VolumeNumber); !ok {
			http.Error(w, "O volume é um número maior que zero.", http.StatusBadRequest)
			return
		}
	}
	actor := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	name, retired, err := lockOfficial(r.Context(), tx, id)
	if err != nil {
		h.fail(w, "group", err)
		return
	}
	if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	// The works of the range; a number in the series is the position in the collection.
	rows, err := tx.QueryContext(r.Context(), `
		SELECT w.id FROM collection_works cw JOIN works w ON w.id = cw.work_id
		WHERE cw.collection_id = $1 AND w.retired_at IS NULL
		  AND ($2::text IS NULL OR COALESCE(w.unit, '') = $2)
		  AND ($3::real IS NULL OR cw.position >= $3) AND ($4::real IS NULL OR cw.position <= $4)
		  AND (($3::real IS NULL AND $4::real IS NULL) OR cw.position IS NOT NULL)
		ORDER BY w.id FOR UPDATE OF w`, id, req.Unit, req.From, req.To)
	if err != nil {
		h.fail(w, "group", err)
		return
	}
	var ids []int64
	for rows.Next() {
		var wid int64
		if err := rows.Scan(&wid); err != nil {
			rows.Close()
			h.fail(w, "group", err)
			return
		}
		ids = append(ids, wid)
	}
	rows.Close()
	changed := map[string]int{}
	if len(ids) > 0 {
		for _, f := range []struct {
			field string
			value *string
			text  string
		}{{"story_arc", req.StoryArc, arc}, {"volume_number", req.VolumeNumber, volume}} {
			if f.value == nil {
				continue
			}
			var n int
			err := tx.QueryRowContext(r.Context(), `
				WITH changed AS (
					UPDATE works SET `+f.field+` = NULLIF($2, '')`+map[string]string{"story_arc": "", "volume_number": "::real"}[f.field]+`, updated_at = CURRENT_TIMESTAMP
					WHERE id = ANY($1) AND `+f.field+`::text IS DISTINCT FROM NULLIF($2, '')
					RETURNING id)
				SELECT count(*) FROM changed`, pq.Array(ids), f.text).Scan(&n)
			if err != nil {
				h.fail(w, "group", err)
				return
			}
			changed[f.field] = n
			if n > 0 {
				if _, err := tx.ExecContext(r.Context(), `
					INSERT INTO work_field_sources (work_id, field, source, actor_id)
					SELECT unnest($1::int[]), $2, $3, NULLIF($4, '')::uuid
					ON CONFLICT (work_id, field) DO UPDATE SET source = EXCLUDED.source, actor_id = EXCLUDED.actor_id, updated_at = now()`,
					pq.Array(ids), f.field, sourceManual, actor); err != nil {
					h.fail(w, "group", err)
					return
				}
			}
		}
	}
	details := map[string]any{"name": name, "works": len(ids), "changed": changed}
	if req.StoryArc != nil {
		details["storyArc"] = arc
	}
	if req.VolumeNumber != nil {
		details["volumeNumber"] = volume
	}
	if req.From != nil {
		details["from"] = *req.From
	}
	if req.To != nil {
		details["to"] = *req.To
	}
	if err := audit.Record(r.Context(), tx, actor, "collection.group", "collection", strconv.FormatInt(id, 10), details); err != nil {
		h.fail(w, "group", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "group", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"works": len(ids), "changed": changed})
}
