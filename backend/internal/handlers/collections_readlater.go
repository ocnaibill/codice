package handlers

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
)

const (
	// readLaterKey names the list of "Ler depois" among a person's lists (DEC-152), and readLaterName is what it is called.
	readLaterKey  = "read_later"
	readLaterName = "Ler depois"
)

// AddReadLater answers PUT /works/{id}/read-later: the work goes to the end of the list "Ler depois" of the caller, which is made the
// first time. A work that is in already stays where it is, and doing it twice is not an error.
func (h *PersonalCollectionsHandler) AddReadLater(w http.ResponseWriter, r *http.Request) {
	work64, ok := collectionIDParam(r, "id")
	if !ok || work64 > math.MaxInt32 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	// The same lock as making a list: two requests at once must not make the list twice.
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('personal-collections:' || $1::text, 0))`, userID); err != nil {
		h.fail(w, "read later", err)
		return
	}
	var workRetired bool
	if err := tx.QueryRowContext(r.Context(), `SELECT retired_at IS NOT NULL FROM works WHERE id = $1`, work64).Scan(&workRetired); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		h.fail(w, "read later", err)
		return
	}
	if workRetired {
		http.Error(w, "A obra está na lixeira.", http.StatusConflict)
		return
	}
	var id int64
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO collections (kind, owner_id, name, origin, system_key)
		VALUES ('personal', $1::uuid, $2, 'manual', $3)
		ON CONFLICT (owner_id, system_key) WHERE system_key IS NOT NULL DO UPDATE SET system_key = EXCLUDED.system_key
		RETURNING id`, userID, readLaterName, readLaterKey).Scan(&id); err != nil {
		h.fail(w, "read later", err)
		return
	}
	var entries int
	var member bool
	if err := tx.QueryRowContext(r.Context(), `
		SELECT count(*), COALESCE(bool_or(work_id = $2), FALSE) FROM collection_works WHERE collection_id = $1`, id, work64).Scan(&entries, &member); err != nil {
		h.fail(w, "read later", err)
		return
	}
	if !member {
		if entries >= maxPersonalEntries {
			conflict(w, "A lista já tem "+strconv.Itoa(maxPersonalEntries)+" obras.", id)
			return
		}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO collection_works (collection_id, work_id, official, position)
			SELECT $1, $2, FALSE, COALESCE(max(position), 0) + 1 FROM collection_works WHERE collection_id = $1`, id, work64); err != nil {
			h.fail(w, "read later", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "read later", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveReadLater answers DELETE /works/{id}/read-later: the work leaves the list "Ler depois" of the caller. One that was not there, or a
// person who has no such list yet, is not an error.
func (h *PersonalCollectionsHandler) RemoveReadLater(w http.ResponseWriter, r *http.Request) {
	work64, ok := collectionIDParam(r, "id")
	if !ok || work64 > math.MaxInt32 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `
		DELETE FROM collection_works cw USING collections c
		WHERE cw.collection_id = c.id AND c.owner_id = $1::uuid AND c.system_key = $2 AND cw.work_id = $3`,
		currentUserID(r), readLaterKey, work64); err != nil {
		h.fail(w, "read later", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// isSystemList says whether a personal collection is one the Códice keeps (and a person does not rename or put away).
func isSystemList(tx *sql.Tx, r *http.Request, id int64) (bool, error) {
	var system bool
	err := tx.QueryRowContext(r.Context(), `SELECT system_key IS NOT NULL FROM collections WHERE id = $1`, id).Scan(&system)
	return system, err
}
