package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// EmbeddingsAdminHandler controls use of an optional local worker. Deployment installs the
// capability; the owner decides whether the library may spend resources on it.
type EmbeddingsAdminHandler struct{ DB *sql.DB }

func (h *EmbeddingsAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	var enabled, available bool
	var provider, model, state, workerError string
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE((SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'equivalence.embeddings'), false),
		       COALESCE((SELECT updated_at > now() - interval '2 minutes' FROM settings WHERE key = 'embeddings.worker'), false),
		       COALESCE((SELECT value->>'provider' FROM settings WHERE key = 'embeddings.worker'), ''),
		       COALESCE((SELECT value->>'model' FROM settings WHERE key = 'embeddings.worker'), ''),
		       COALESCE((SELECT value->>'state' FROM settings WHERE key = 'embeddings.worker'), ''),
		       COALESCE((SELECT value->>'error' FROM settings WHERE key = 'embeddings.worker'), '')`).Scan(&enabled, &available, &provider, &model, &state, &workerError); err != nil {
		http.Error(w, "Error reading semantic matching settings", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"enabled": enabled, "available": available, "provider": provider, "model": model, "state": state, "error": workerError})
}

func (h *EmbeddingsAdminHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "invalid settings", http.StatusBadRequest)
		return
	}
	if req.Enabled {
		var available bool
		if err := h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(
			(SELECT updated_at > now() - interval '2 minutes' FROM settings WHERE key = 'embeddings.worker'), false)`).Scan(&available); err != nil {
			http.Error(w, "Error reading semantic matching settings", http.StatusInternalServerError)
			return
		}
		if !available {
			http.Error(w, "O perfil local de embeddings não está em execução.", http.StatusConflict)
			return
		}
	}
	value, _ := json.Marshal(map[string]bool{"enabled": req.Enabled})
	if _, err := h.DB.ExecContext(r.Context(), `INSERT INTO settings (key, value, updated_by)
		VALUES ('equivalence.embeddings', $1::jsonb, $2) ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, value, currentUserID(r)); err != nil {
		http.Error(w, "Error saving semantic matching settings", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "embeddings.policy", "settings", "equivalence.embeddings", map[string]any{"enabled": req.Enabled})
	h.Get(w, r)
}
