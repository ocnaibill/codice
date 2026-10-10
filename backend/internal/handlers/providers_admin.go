package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/metaproviders"
	"github.com/redis/go-redis/v9"
)

// ProvidersHandler lets the owner decide which external metadata providers may be asked, and lets the staff see
// the choice (#68, DEC-045): asking one sends the title of a work to a third party.
type ProvidersHandler struct {
	DB          *sql.DB
	RedisClient *redis.Client
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

// Test asks the worker to test one provider (owner, DEC-145): a fixed, public question, with nothing of the library in it, whether the provider is on or
// not. The answer is in the list of providers when the worker has it. Asking again while one waits is the same request.
func (h *ProvidersHandler) Test(w http.ResponseWriter, r *http.Request) {
	id, queued, err := metaproviders.RequestTest(r.Context(), h.DB, chi.URLParam(r, "id"), currentUserID(r))
	if errors.Is(err, metaproviders.ErrUnknown) {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error queueing the test of a metadata provider:", err)
		http.Error(w, "Error queueing the test", http.StatusInternalServerError)
		return
	}
	if !queued {
		writeJSON(w, http.StatusOK, map[string]any{"queued": true, "alreadyQueued": true})
		return
	}
	// Wake the workers; a failure only means they will find it on their next poll.
	jobs.Notify(r.Context(), h.RedisClient, id)
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "jobId": id})
}
