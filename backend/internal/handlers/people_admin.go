package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// PeopleHandler lets owner and admin review people who may be the same person ("Herbert, Frank" and "Frank
// Herbert", #36). The system only proposes; nothing is merged without a decision.
type PeopleHandler struct {
	DB *sql.DB
}

// List returns the pairs waiting for a decision.
func (h *PeopleHandler) List(w http.ResponseWriter, r *http.Request) {
	pairs, err := people.ListPending(r.Context(), h.DB)
	if err != nil {
		log.Println("Error listing people that may be the same:", err)
		http.Error(w, "Error listing people", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": pairs})
}

func mergeID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// Dismiss records that the two are not the same person.
func (h *PeopleHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := mergeID(r)
	if !ok {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	switch err := people.Dismiss(r.Context(), h.DB, id, currentUserID(r)); {
	case errors.Is(err, people.ErrNotFound):
		http.Error(w, "Candidate not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error dismissing a person pair:", err)
		http.Error(w, "Error recording the decision", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// Merge makes the two people one. It cannot be undone, so it needs the person to keep and an explicit
// confirmation.
func (h *PeopleHandler) Merge(w http.ResponseWriter, r *http.Request) {
	id, ok := mergeID(r)
	if !ok {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	var req struct {
		Keep    int  `json:"keep"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Keep <= 0 || !req.Confirm {
		http.Error(w, "keep (the person who stays) and confirm=true are required", http.StatusBadRequest)
		return
	}
	switch err := people.Merge(r.Context(), h.DB, id, req.Keep, currentUserID(r)); {
	case errors.Is(err, people.ErrNotFound):
		http.Error(w, "Candidate not found", http.StatusNotFound)
	case errors.Is(err, people.ErrBadKeep):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		log.Println("Error merging people:", err)
		http.Error(w, "Error merging the people", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
