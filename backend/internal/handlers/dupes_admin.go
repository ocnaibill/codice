package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/dupes"
	"github.com/ocnaibill/codice/backend/internal/jobs"
)

// DuplicatesHandler lets owner and admin review works that may be the same book
// (DEC-029). The system only proposes; nothing is merged without a decision.
type DuplicatesHandler struct {
	DB *sql.DB
}

const (
	duplicatesPage    = 50  // pairs in a page when the caller does not say
	duplicatesMaxPage = 200 // the most a caller may ask for at once
)

// List returns a page of the pairs waiting for a decision, with how many wait in all. `limit` is the size of the page and
// `after` the id of the last pair already seen; the answer says `more` when there is a next page.
func (h *DuplicatesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, after := duplicatesPage, int64(0)
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, duplicatesMaxPage)
	}
	if v, err := strconv.ParseInt(q.Get("after"), 10, 64); err == nil && v > 0 {
		after = v
	}
	list, err := dupes.ListPending(r.Context(), h.DB, after, limit)
	if err != nil {
		log.Println("Error listing duplicates:", err)
		http.Error(w, "Error listing possible duplicates", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func candidateID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// Dismiss says the pair is not a duplicate; it will not be proposed again.
func (h *DuplicatesHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := candidateID(r)
	if !ok {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	if err := dupes.Dismiss(r.Context(), h.DB, id, currentUserID(r)); errors.Is(err, dupes.ErrNotFound) {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Error dismissing the candidate", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Link makes one work an edition of the other. It cannot be undone, so it needs
// the work to keep and an explicit confirmation.
func (h *DuplicatesHandler) Link(w http.ResponseWriter, r *http.Request) {
	id, ok := candidateID(r)
	if !ok {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	var req struct {
		Keep    int  `json:"keep"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Keep <= 0 || !req.Confirm {
		http.Error(w, "keep (the work that stays) and confirm=true are required", http.StatusBadRequest)
		return
	}
	switch err := dupes.Link(r.Context(), h.DB, id, req.Keep, currentUserID(r)); {
	case errors.Is(err, dupes.ErrNotFound):
		http.Error(w, "Candidate not found", http.StatusNotFound)
	case errors.Is(err, dupes.ErrBadKeep):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, dupes.ErrRetired):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		log.Println("Error linking works:", err)
		http.Error(w, "Error linking the works", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// Scan queues a comparison of every active work with every other.
func (h *DuplicatesHandler) Scan(w http.ResponseWriter, r *http.Request) {
	var id int64
	err := h.DB.QueryRowContext(r.Context(), `SELECT id FROM jobs WHERE type = 'dedupe' AND work_id IS NULL AND state IN ('pending', 'running') LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = h.DB.QueryRowContext(r.Context(), `INSERT INTO jobs (type, payload, priority, created_by) VALUES ('dedupe', '{}', $1, NULLIF($2, '')::uuid) RETURNING id`,
			jobs.PriorityManual, currentUserID(r)).Scan(&id)
	}
	if err != nil {
		http.Error(w, "Error queueing the comparison", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]int64{"job_id": id})
}
