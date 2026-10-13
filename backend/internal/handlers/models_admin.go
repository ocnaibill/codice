package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// The local models of the library (DEC-176): what each one is for, whether it is the one chosen, whether the service that runs it is there,
// how much of it has been downloaded and how much it has been used. One list for the administration to show, with a control for each use
// (the semantic matching and the reading of comics) and "none" always among the choices.
//
// What the services say about themselves is kept in the settings table, as the embeddings worker has always done: a key per service with
// a value the service rewrites, and a service is there while it has done so in the last two minutes.

const (
	comicOCRSetting = "comic_ocr"        // {"model": "none" | "mobile" | "server"}: the owner's choice
	comicOCRWorker  = "comic_ocr.worker" // {"model", "state", "progress", "pages", "error"}: what the service says
	noModel         = "none"
)

// ModelsAdminHandler lists the local models and sets which one reads comics and manga.
type ModelsAdminHandler struct{ DB *sql.DB }

type comicOCRModel struct {
	ID, Name, Scope string
	SizeMB          int
}

var comicOCRModels = []comicOCRModel{
	{"mobile", "Leve (PaddleOCR mobile)", "Quadrinhos e mangás em português, inglês e outros idiomas de alfabeto latino; lê em metade do tempo", 13},
	{"server", "Servidor (PaddleOCR médio)", "Mais fiel, principalmente em mangá com letra em itálico sobre retícula; leva o dobro do tempo", 132},
}

// ModelEntry is one model as the administration shows it.
type ModelEntry struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // "semantic" or "comic_ocr"
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	SizeMB   int    `json:"sizeMB"`
	Selected bool   `json:"selected"`
	// State: "off" (not the one chosen), "service_missing" (chosen, and the service that runs it is not running), "preparing",
	// "downloading" (with Progress), "waiting", "ready" or "error".
	State    string `json:"state"`
	Progress *int   `json:"progress,omitempty"`
	InUse    bool   `json:"inUse"`
	Usage    struct {
		Count int    `json:"count"`
		Unit  string `json:"unit"`
	} `json:"usage"`
	Error string `json:"error,omitempty"`
}

// serviceBeat is what a service wrote about itself, and whether it did so recently.
type serviceBeat struct {
	Fresh bool
	Value map[string]any
}

func readServiceBeat(ctx context.Context, db *sql.DB, key string) (serviceBeat, error) {
	var raw []byte
	var fresh bool
	err := db.QueryRowContext(ctx, `SELECT value, updated_at > now() - interval '2 minutes' FROM settings WHERE key = $1`, key).Scan(&raw, &fresh)
	if errors.Is(err, sql.ErrNoRows) {
		return serviceBeat{Value: map[string]any{}}, nil
	}
	if err != nil {
		return serviceBeat{}, err
	}
	value := map[string]any{}
	_ = json.Unmarshal(raw, &value)
	return serviceBeat{Fresh: fresh, Value: value}, nil
}

func (b serviceBeat) text(key string) string {
	s, _ := b.Value[key].(string)
	return s
}

func (b serviceBeat) number(key string) (int, bool) {
	f, ok := b.Value[key].(float64)
	return int(f), ok
}

// stateOf says what a model is doing, from the choice and from what its service says. `have` is the model the service has loaded.
func stateOf(selected, available bool, have, model, said string, progress *int) string {
	switch {
	case !selected:
		return "off"
	case !available:
		return "service_missing"
	case have != model:
		return "preparing"
	}
	switch said {
	case "preparing":
		if progress != nil {
			return "downloading"
		}
		return "preparing"
	case "ready":
		return "ready"
	case "error":
		return "error"
	}
	return "waiting"
}

func (h *ModelsAdminHandler) chosenComicOCR(ctx context.Context) (string, error) {
	var model string
	err := h.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT value->>'model' FROM settings WHERE key = $1), $2)`, comicOCRSetting, noModel).Scan(&model)
	if model != "mobile" && model != "server" {
		model = noModel
	}
	return model, err
}

func (h *ModelsAdminHandler) state(ctx context.Context) (map[string]any, error) {
	var semanticOn bool
	var semanticModel string
	if err := h.DB.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'equivalence.embeddings'), false),
		       COALESCE((SELECT value->>'model' FROM settings WHERE key = 'equivalence.embeddings'), $1)`, defaultEmbeddingModel).Scan(&semanticOn, &semanticModel); err != nil {
		return nil, err
	}
	semantic, err := readServiceBeat(ctx, h.DB, "embeddings.worker")
	if err != nil {
		return nil, err
	}
	comicChoice, err := h.chosenComicOCR(ctx)
	if err != nil {
		return nil, err
	}
	comic, err := readServiceBeat(ctx, h.DB, comicOCRWorker)
	if err != nil {
		return nil, err
	}

	models := []ModelEntry{}
	for _, m := range embeddingModels {
		id := m["id"].(string)
		entry := ModelEntry{ID: id, Kind: "semantic", Name: m["name"].(string), Scope: m["scope"].(string), SizeMB: m["downloadMB"].(int),
			Selected: semanticOn && semanticModel == id}
		var progress *int
		if p, ok := semantic.number("progress"); ok && semantic.text("model") == id {
			progress = &p
		}
		entry.State = stateOf(entry.Selected, semantic.Fresh, semantic.text("model"), id, semantic.text("state"), progress)
		entry.Progress = progress
		entry.InUse = entry.State == "ready"
		entry.Error = ""
		if entry.Selected && semantic.text("model") == id {
			entry.Error = semantic.text("error")
		}
		if err := h.DB.QueryRowContext(ctx, `SELECT count(*) FROM text_embedding_status WHERE model = $1`, id).Scan(&entry.Usage.Count); err != nil {
			return nil, err
		}
		entry.Usage.Unit = "arquivos preparados"
		models = append(models, entry)
	}
	for _, m := range comicOCRModels {
		entry := ModelEntry{ID: m.ID, Kind: "comic_ocr", Name: m.Name, Scope: m.Scope, SizeMB: m.SizeMB, Selected: comicChoice == m.ID}
		var progress *int
		if p, ok := comic.number("progress"); ok && comic.text("model") == m.ID {
			progress = &p
		}
		entry.State = stateOf(entry.Selected, comic.Fresh, comic.text("model"), m.ID, comic.text("state"), progress)
		entry.Progress = progress
		entry.InUse = entry.State == "ready"
		if entry.Selected && comic.text("model") == m.ID {
			entry.Error = comic.text("error")
		}
		if comic.text("model") == m.ID {
			entry.Usage.Count, _ = comic.number("pages")
		}
		entry.Usage.Unit = "páginas lidas"
		models = append(models, entry)
	}

	semanticChoice := noModel
	if semanticOn {
		semanticChoice = semanticModel
	}
	// The pages read with a model that is not the one chosen: what "read again with this model" would read (DEC-176).
	otherPages := 0
	if comicChoice != noModel {
		if err := h.DB.QueryRowContext(ctx, `SELECT count(*) FROM comic_ocr_pages WHERE model <> $1 AND state IN ('done', 'blank')`, comicChoice).Scan(&otherPages); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"controls": map[string]string{"semantic": semanticChoice, "comicOcr": comicChoice},
		"services": map[string]bool{"semantic": semantic.Fresh, "comicOcr": comic.Fresh},
		"models":   models,
		"comicOcr": map[string]int{"otherModelPages": otherPages},
	}, nil
}

// Get answers GET /admin/models (owner).
func (h *ModelsAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	state, err := h.state(r.Context())
	if err != nil {
		http.Error(w, "Error reading the local models", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, state)
}

// SetComicOCR answers PUT /admin/models/comic-ocr {"model": "none" | "mobile" | "server"} (owner): which model reads comics and manga, or none.
// Choosing one needs its service to be running, as the semantic matching does: a choice that nothing carries out is not kept.
func (h *ModelsAdminHandler) SetComicOCR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "invalid settings", http.StatusBadRequest)
		return
	}
	if req.Model != noModel && req.Model != "mobile" && req.Model != "server" {
		http.Error(w, "modelo de OCR de quadrinhos não suportado", http.StatusBadRequest)
		return
	}
	if req.Model != noModel {
		beat, err := readServiceBeat(r.Context(), h.DB, comicOCRWorker)
		if err != nil {
			http.Error(w, "Error reading the local models", http.StatusInternalServerError)
			return
		}
		if !beat.Fresh {
			http.Error(w, "O serviço de OCR de quadrinhos não está em execução.", http.StatusConflict)
			return
		}
	}
	value, _ := json.Marshal(map[string]any{"model": req.Model})
	if _, err := h.DB.ExecContext(r.Context(), `INSERT INTO settings (key, value, updated_by)
		VALUES ($1, $2::jsonb, $3) ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, comicOCRSetting, value, currentUserID(r)); err != nil {
		http.Error(w, "Error saving the comic OCR model", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "models.comic_ocr", "settings", comicOCRSetting, map[string]any{"model": req.Model})
	h.Get(w, r)
}

// RereadComicOCR answers POST /admin/models/comic-ocr/reread (owner): the pages that were read with another model are read again with the
// one chosen. Their rows are dropped, which makes the comics unread again for the service, that queues them; the text of each file stays
// as it was until the new reading replaces it, so the search is never left without it. A model must be chosen and its service running.
func (h *ModelsAdminHandler) RereadComicOCR(w http.ResponseWriter, r *http.Request) {
	choice, err := h.chosenComicOCR(r.Context())
	if err != nil {
		http.Error(w, "Error reading the local models", http.StatusInternalServerError)
		return
	}
	if choice == noModel {
		http.Error(w, "Escolha um modelo antes de reler as páginas.", http.StatusConflict)
		return
	}
	beat, err := readServiceBeat(r.Context(), h.DB, comicOCRWorker)
	if err != nil {
		http.Error(w, "Error reading the local models", http.StatusInternalServerError)
		return
	}
	if !beat.Fresh {
		http.Error(w, "O serviço de OCR de quadrinhos não está em execução.", http.StatusConflict)
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM comic_ocr_pages WHERE model <> $1 AND state IN ('done', 'blank')`, choice)
	if err != nil {
		http.Error(w, "Error reading the pages again", http.StatusInternalServerError)
		return
	}
	pages, _ := res.RowsAffected()
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "models.comic_ocr.reread", "settings", comicOCRSetting, map[string]any{"model": choice, "pages": pages})
	h.Get(w, r)
}
