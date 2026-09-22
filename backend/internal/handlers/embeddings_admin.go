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

const defaultEmbeddingModel = "sentence-transformers/LaBSE"

var embeddingModels = []map[string]any{
	{"id": "intfloat/multilingual-e5-small", "name": "Leve (e5-small)", "downloadMB": 471, "scope": "Português, inglês e outros idiomas de alfabeto latino"},
	{"id": defaultEmbeddingModel, "name": "Multilíngue amplo (LaBSE)", "downloadMB": 1880, "scope": "Inclui alfabetos distantes, como árabe"},
}

func supportedEmbeddingModel(model string) bool {
	for _, candidate := range embeddingModels {
		if candidate["id"] == model {
			return true
		}
	}
	return false
}

func (h *EmbeddingsAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	var enabled, available bool
	var provider, model, activeModel, state, workerError string
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE((SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'equivalence.embeddings'), false),
		       COALESCE((SELECT value->>'model' FROM settings WHERE key = 'equivalence.embeddings'), $1),
		       COALESCE((SELECT updated_at > now() - interval '2 minutes' FROM settings WHERE key = 'embeddings.worker'), false),
		       COALESCE((SELECT value->>'provider' FROM settings WHERE key = 'embeddings.worker'), ''),
		       COALESCE((SELECT value->>'model' FROM settings WHERE key = 'embeddings.worker'), ''),
		       COALESCE((SELECT value->>'state' FROM settings WHERE key = 'embeddings.worker'), ''),
		       COALESCE((SELECT value->>'error' FROM settings WHERE key = 'embeddings.worker'), '')`, defaultEmbeddingModel).Scan(&enabled, &model, &available, &provider, &activeModel, &state, &workerError); err != nil {
		http.Error(w, "Error reading semantic matching settings", http.StatusInternalServerError)
		return
	}
	if activeModel != model && enabled {
		state = "preparing"
	}
	json.NewEncoder(w).Encode(map[string]any{"enabled": enabled, "available": available, "provider": provider,
		"model": model, "activeModel": activeModel, "models": embeddingModels, "state": state, "error": workerError})
}

func (h *EmbeddingsAdminHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool   `json:"enabled"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "invalid settings", http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = defaultEmbeddingModel
	}
	if !supportedEmbeddingModel(req.Model) {
		http.Error(w, "modelo de embeddings não suportado", http.StatusBadRequest)
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
	value, _ := json.Marshal(map[string]any{"enabled": req.Enabled, "model": req.Model})
	if _, err := h.DB.ExecContext(r.Context(), `INSERT INTO settings (key, value, updated_by)
		VALUES ('equivalence.embeddings', $1::jsonb, $2) ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, value, currentUserID(r)); err != nil {
		http.Error(w, "Error saving semantic matching settings", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "embeddings.policy", "settings", "equivalence.embeddings", map[string]any{"enabled": req.Enabled, "model": req.Model})
	h.Get(w, r)
}
