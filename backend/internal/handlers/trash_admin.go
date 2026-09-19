package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// TrashHandler is the administration of the recoverable trash (RF-045).
// Everything that destroys bytes needs an explicit confirmation in the request.
type TrashHandler struct {
	Trash *storage.Trash
}

func (h *TrashHandler) record(r *http.Request, action, target string, details map[string]any) {
	if err := audit.Record(r.Context(), h.Trash.DB, currentUserID(r), action, "trash", target, details); err != nil {
		log.Println("Could not audit", action, err)
	}
}

func itemID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// List shows the trash, the space it occupies and the policy in force.
func (h *TrashHandler) List(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.Trash.List(r.Context())
	policy, perr := h.Trash.GetPolicy(r.Context())
	if err != nil || perr != nil {
		http.Error(w, "Error reading the trash", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": items, "totalBytes": total, "policy": policy})
}

// Restore puts an item back where it was (or under a new name if that is taken).
func (h *TrashHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	target, err := h.Trash.Restore(r.Context(), id)
	switch {
	case errors.Is(err, storage.ErrNoSuchItem):
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	case err != nil:
		log.Println("Error restoring from the trash:", err)
		http.Error(w, "Error restoring the item: "+err.Error(), http.StatusConflict)
		return
	}
	h.record(r, "trash.restore", strconv.FormatInt(id, 10), map[string]any{"path": target})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"path": target})
}

type confirmRequest struct {
	Confirm bool `json:"confirm"`
}

func confirmed(r *http.Request) bool {
	if r.URL.Query().Get("confirm") == "true" {
		return true
	}
	var c confirmRequest
	json.NewDecoder(r.Body).Decode(&c)
	return c.Confirm
}

// Delete destroys one item for good.
func (h *TrashHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if !confirmed(r) {
		http.Error(w, "Deleting for good needs confirm=true", http.StatusBadRequest)
		return
	}
	freed, err := h.Trash.Delete(r.Context(), id)
	if errors.Is(err, storage.ErrNoSuchItem) {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error deleting from the trash:", err)
		http.Error(w, "Error deleting the item", http.StatusInternalServerError)
		return
	}
	h.record(r, "trash.delete", strconv.FormatInt(id, 10), map[string]any{"freedBytes": freed})
	w.WriteHeader(http.StatusNoContent)
}

// Empty destroys everything in the trash for good.
func (h *TrashHandler) Empty(w http.ResponseWriter, r *http.Request) {
	if !confirmed(r) {
		http.Error(w, "Emptying the trash needs confirm=true", http.StatusBadRequest)
		return
	}
	deleted, freed, err := h.Trash.Empty(r.Context())
	if err != nil {
		log.Println("Error emptying the trash:", err)
		http.Error(w, "Error emptying the trash", http.StatusInternalServerError)
		return
	}
	h.record(r, "trash.empty", "all", map[string]any{"deleted": deleted, "freedBytes": freed})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"deleted": deleted, "freedBytes": freed})
}

// GetPolicy shows the automatic cleanup setting.
func (h *TrashHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := h.Trash.GetPolicy(r.Context())
	if err != nil {
		http.Error(w, "Error reading the policy", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// SetPolicy changes the automatic cleanup setting (owner only). It applies to
// files that enter the trash afterwards.
func (h *TrashHandler) SetPolicy(w http.ResponseWriter, r *http.Request) {
	var p storage.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if err := h.Trash.SetPolicy(r.Context(), p, currentUserID(r)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.record(r, "trash.policy", "policy", map[string]any{"enabled": p.Enabled, "days": p.Days})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// PreviewPolicy counts what applying the policy to items already in the trash
// would affect, including what is already past due (owner only).
func (h *TrashHandler) PreviewPolicy(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		p, _ := h.Trash.GetPolicy(r.Context())
		days = p.Days
	}
	prev, err := h.Trash.PreviewPolicy(r.Context(), days)
	if err != nil {
		http.Error(w, "Error previewing the policy", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"days": days, "items": prev.Items, "alreadyDue": prev.AlreadyDue})
}

// ApplyPolicy sets the expiry of every item already in the trash (owner only).
// Items past due are deleted by the next sweep, so this needs confirmation.
func (h *TrashHandler) ApplyPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Days    int  `json:"days"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Confirm || req.Days < 1 {
		http.Error(w, "days and confirm=true are required; preview first", http.StatusBadRequest)
		return
	}
	n, err := h.Trash.ApplyPolicy(r.Context(), req.Days)
	if err != nil {
		http.Error(w, "Error applying the policy", http.StatusInternalServerError)
		return
	}
	h.record(r, "trash.policy_apply", "policy", map[string]any{"days": req.Days, "items": n})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": n})
}

// Orphans lists files in the managed storage that the catalog does not know.
func (h *TrashHandler) Orphans(w http.ResponseWriter, r *http.Request) {
	list, err := h.Trash.FindOrphans(r.Context())
	if err != nil {
		http.Error(w, "Error looking for orphans", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": list})
}

// TrashOrphans moves the chosen orphans to the trash (recoverable).
func (h *TrashHandler) TrashOrphans(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Paths) == 0 || len(req.Paths) > 1000 {
		http.Error(w, "paths is required (1 to 1000)", http.StatusBadRequest)
		return
	}
	moved, err := h.Trash.TrashOrphans(r.Context(), req.Paths, currentUserID(r))
	if err != nil {
		log.Println("Error trashing orphans:", err)
		http.Error(w, "Error moving the orphans", http.StatusInternalServerError)
		return
	}
	h.record(r, "storage.orphans_trash", "orphans", map[string]any{"moved": moved})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"moved": moved})
}
