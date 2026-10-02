package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// OCRSettingsHandler controls reading the pages of scanned PDFs by OCR (#24). It is a processing of its own, apart from
// the switch of the AI areas (DEC-044): the deployment installs the engine in a separate worker, and the owner decides
// whether the library spends its time on it. Nothing is read until the owner says so, and then every file that needs
// it is read once, in the background, and kept for everyone.
type OCRSettingsHandler struct{ DB *sql.DB }

// defaultOCRLanguage is what a PDF whose edition declares no language is read as, which is most scans: Portuguese, with
// English for what is quoted in it. A file with a language is read in that language alone.
const defaultOCRLanguage = "por+eng"

// languageSet is one or more language codes of the engine joined by "+" (por, por+eng).
var languageSet = regexp.MustCompile(`^[a-z][a-z0-9_]{1,15}(\+[a-z][a-z0-9_]{1,15}){0,3}$`)

type ocrWorker struct {
	Available bool
	Engine    string
	Version   string
	Languages []string
	State     string
	Error     string
}

func (h *OCRSettingsHandler) worker(r *http.Request) (ocrWorker, error) {
	return readOCRWorker(r.Context(), h.DB)
}

// readOCRWorker is what the OCR service says about itself (the languages it has), and whether it is running.
func readOCRWorker(ctx context.Context, db *sql.DB) (ocrWorker, error) {
	var w ocrWorker
	var languages sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT updated_at > now() - interval '2 minutes' FROM settings WHERE key = 'ocr.worker'), false),
		       COALESCE((SELECT value->>'engine' FROM settings WHERE key = 'ocr.worker'), ''),
		       COALESCE((SELECT value->>'version' FROM settings WHERE key = 'ocr.worker'), ''),
		       (SELECT value->'languages' FROM settings WHERE key = 'ocr.worker'),
		       COALESCE((SELECT value->>'state' FROM settings WHERE key = 'ocr.worker'), ''),
		       COALESCE((SELECT value->>'error' FROM settings WHERE key = 'ocr.worker'), '')`).
		Scan(&w.Available, &w.Engine, &w.Version, &languages, &w.State, &w.Error)
	if err != nil {
		return w, err
	}
	w.Languages = []string{}
	if languages.Valid {
		json.Unmarshal([]byte(languages.String), &w.Languages)
	}
	return w, nil
}

// missingLanguage is the first language of the set that the engine does not have, or "". With the engine not running,
// or not saying what it has, nothing can be told to be missing.
func (w ocrWorker) missingLanguage(set string) string {
	if !w.Available || len(w.Languages) == 0 {
		return ""
	}
	have := map[string]bool{}
	for _, l := range w.Languages {
		have[l] = true
	}
	for _, l := range strings.Split(set, "+") {
		if !have[l] {
			return l
		}
	}
	return ""
}

func (h *OCRSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	var language string
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE((SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'ocr'), false),
		       COALESCE(NULLIF((SELECT value->>'language' FROM settings WHERE key = 'ocr'), ''), $1)`, defaultOCRLanguage).
		Scan(&enabled, &language); err != nil {
		http.Error(w, "Error reading the OCR settings", http.StatusInternalServerError)
		return
	}
	worker, err := h.worker(r)
	if err != nil {
		http.Error(w, "Error reading the OCR settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled, "language": language, "available": worker.Available, "engine": worker.Engine,
		"engineVersion": worker.Version, "languages": worker.Languages, "state": worker.State, "error": worker.Error,
	})
}

func (h *OCRSettingsHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled  bool   `json:"enabled"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "invalid settings", http.StatusBadRequest)
		return
	}
	req.Language = strings.TrimSpace(req.Language)
	if req.Language == "" {
		req.Language = defaultOCRLanguage
	}
	if !languageSet.MatchString(req.Language) {
		http.Error(w, "Idioma do OCR inválido: use os códigos do motor, como por ou por+eng.", http.StatusBadRequest)
		return
	}
	worker, err := h.worker(r)
	if err != nil {
		http.Error(w, "Error reading the OCR settings", http.StatusInternalServerError)
		return
	}
	// With the engine running, only the languages it has can be chosen: a language it lacks would fail every page.
	if missing := worker.missingLanguage(req.Language); missing != "" {
		http.Error(w, "O serviço de OCR não tem o idioma "+missing+".", http.StatusBadRequest)
		return
	}
	if req.Enabled && !worker.Available {
		http.Error(w, "O serviço de OCR não está em execução.", http.StatusConflict)
		return
	}
	value, _ := json.Marshal(map[string]any{"enabled": req.Enabled, "language": req.Language})
	if _, err := h.DB.ExecContext(r.Context(), `INSERT INTO settings (key, value, updated_by)
		VALUES ('ocr', $1::jsonb, $2) ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, value, currentUserID(r)); err != nil {
		http.Error(w, "Error saving the OCR settings", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "ocr.policy", "settings", "ocr", map[string]any{"enabled": req.Enabled, "language": req.Language})
	h.Get(w, r)
}
