package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/metaproviders"
)

// ProvidersHandler lets the owner decide which external metadata providers may be asked, and lets the staff see
// the choice (#68, DEC-045): asking one sends the title of a work to a third party.
type ProvidersHandler struct {
	DB *sql.DB
}

// List returns every provider with whether it is on, and what asking it sends.
func (h *ProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := metaproviders.List(r.Context(), h.DB)
	if err != nil {
		log.Println("Error listing the metadata providers:", err)
		http.Error(w, "Error listing providers", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

// Set turns one provider on or off (owner).
func (h *ProvidersHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Enabled == nil {
		http.Error(w, "enabled must be true or false", http.StatusBadRequest)
		return
	}
	err := metaproviders.Set(r.Context(), h.DB, chi.URLParam(r, "id"), *req.Enabled, currentUserID(r))
	if errors.Is(err, metaproviders.ErrUnknown) {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, metaproviders.ErrNoKey) {
		http.Error(w, "This provider needs an API key and the worker has none: set it in the environment of the worker", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error setting a metadata provider:", err)
		http.Error(w, "Error saving the choice", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
