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
	"github.com/ocnaibill/codice/backend/internal/versions"
)

// VersionsHandler lets owner and admin put the files of one book under one work by hand, and take
// them out again (#37). It is the decision of a person, so it does not wait for the system to propose it.
type VersionsHandler struct {
	DB *sql.DB
}

func pathID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	return id, err == nil && id > 0
}

func (h *VersionsHandler) fail(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, versions.ErrNotFound):
		http.Error(w, "Work or edition not found", http.StatusNotFound)
	case errors.Is(err, versions.ErrSameWork), errors.Is(err, versions.ErrOnlyEdition):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, versions.ErrRetired):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		log.Println("Error", what+":", err)
		http.Error(w, "Error "+what, http.StatusInternalServerError)
	}
}

// Join makes the work in the path (the one being looked at) part of another one: every edition of it
// goes under "into", and the work that is left is retired. It can be undone by separating the editions.
func (h *VersionsHandler) Join(w http.ResponseWriter, r *http.Request) {
	source, ok := pathID(r)
	var req struct {
		Into int `json:"into"`
	}
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil || req.Into <= 0 {
		http.Error(w, "into is the work that stays", http.StatusBadRequest)
		return
	}
	res, err := versions.Join(r.Context(), h.DB, req.Into, source, currentUserID(r))
	if err != nil {
		h.fail(w, "joining the works", err)
		return
	}
	// The work that stays may now look like others: propose what is new (a pair a person decided is left alone).
	if _, err := dupes.Detect(r.Context(), h.DB, req.Into); err != nil {
		log.Println("Error looking for duplicates of a joined work:", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"workId": req.Into, "editions": res.Editions})
}

// Split takes the edition in the path out of its work: back to the work it came from, or to a new one.
func (h *VersionsHandler) Split(w http.ResponseWriter, r *http.Request) {
	edition, ok := pathID(r)
	if !ok {
		http.Error(w, "Work or edition not found", http.StatusNotFound)
		return
	}
	res, err := versions.Split(r.Context(), h.DB, edition, currentUserID(r))
	if err != nil {
		h.fail(w, "separating the edition", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workId": res.WorkID, "restored": res.Restored})
}

// NotTheSame records that the work in the path and another are different books.
func (h *VersionsHandler) NotTheSame(w http.ResponseWriter, r *http.Request) {
	a, ok := pathID(r)
	var req struct {
		WorkID int `json:"workId"`
	}
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil || req.WorkID <= 0 {
		http.Error(w, "workId is the other work", http.StatusBadRequest)
		return
	}
	if err := versions.NotTheSame(r.Context(), h.DB, a, req.WorkID, currentUserID(r)); err != nil {
		h.fail(w, "recording the decision", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
