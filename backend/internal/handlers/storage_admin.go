package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// StorageHandler is the administration of where files live (RF-044). Reorganizing
// is always explicit: first a preview, then a confirmation of that exact preview.
type StorageHandler struct {
	Mover *storage.Mover
	DB    interface {
		audit.Execer
	}
}

// PreviewReorganize lists what a reorganization would move, and to where, and a
// hash of that plan. Nothing is moved.
func (h *StorageHandler) PreviewReorganize(w http.ResponseWriter, r *http.Request) {
	plan, err := h.Mover.Plan(r.Context())
	if err != nil {
		log.Println("Error planning reorganization:", err)
		http.Error(w, "Error planning the reorganization", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

type reorganizeRequest struct {
	Hash string `json:"hash"`
}

// Reorganize executes the plan the admin previewed. It plans again and refuses
// (409) if the result is not the plan that was shown.
func (h *StorageHandler) Reorganize(w http.ResponseWriter, r *http.Request) {
	var req reorganizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hash == "" {
		http.Error(w, "The hash of the previewed plan is required", http.StatusBadRequest)
		return
	}
	res, err := h.Mover.Apply(r.Context(), req.Hash)
	if errors.Is(err, storage.ErrPlanChanged) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error reorganizing storage:", err)
		http.Error(w, "Error reorganizing the storage", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), h.DB, currentUserID(r), "storage.reorganize", "storage", "managed",
		map[string]any{"moved": res.Moved, "failed": len(res.Failures)}); err != nil {
		log.Println("Could not audit the reorganization:", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
