package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/performance"
)

// PerformanceHandler is the "Desempenho" part of the System tab: what the owner may tune without editing the .env or
// restarting anything (see internal/performance). The staff sees the values; only the owner changes them.
type PerformanceHandler struct {
	DB       *sql.DB
	Defaults performance.Settings
	Machine  performance.Machine
	// Live is what this process follows: the owner's change reaches it at once. The OCR service reads the same row by itself.
	Live *performance.Live
}

// performanceView is what the screen shows.
type performanceView struct {
	Values     map[string]int               `json:"values"`
	Defaults   map[string]int               `json:"defaults"`
	Overridden []string                     `json:"overridden"`
	Limits     map[string]performance.Range `json:"limits"`
	Machine    performance.Machine          `json:"machine"`
	Warnings   []string                     `json:"warnings"`
	OCR        performanceOCR               `json:"ocr"`
}

// performanceOCR is what the OCR service says it uses (it reads the setting at the start of each job, so it may be one
// job behind what was just chosen).
type performanceOCR struct {
	Reported bool `json:"reported"`
	Pages    int  `json:"pages"`
	Threads  int  `json:"threads"`
}

func (h *PerformanceHandler) view(r *http.Request) (performanceView, error) {
	overrides, err := performance.Overrides(r.Context(), h.DB, h.Machine)
	if err != nil {
		return performanceView{}, err
	}
	values := performance.Effective(h.Defaults, overrides)
	overridden := []string{}
	for _, field := range performance.Fields {
		if _, ok := overrides[field]; ok {
			overridden = append(overridden, field)
		}
	}
	v := performanceView{
		Values: values, Defaults: performance.Effective(h.Defaults, nil), Overridden: overridden,
		Limits: performance.Limits(h.Machine), Machine: h.Machine, Warnings: performance.Warnings(values, h.Machine),
	}
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	var reported sql.NullBool
	var pages, threads sql.NullInt64
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT (SELECT updated_at > now() - interval '2 minutes' FROM settings WHERE key = 'ocr.worker'),
		       (SELECT (value->>'pages')::int FROM settings WHERE key = 'ocr.worker' AND value->>'pages' ~ '^[0-9]+$'),
		       (SELECT (value->>'threads')::int FROM settings WHERE key = 'ocr.worker' AND value->>'threads' ~ '^[0-9]+$')`).
		Scan(&reported, &pages, &threads); err == nil {
		v.OCR = performanceOCR{Reported: reported.Valid && reported.Bool && pages.Valid && threads.Valid, Pages: int(pages.Int64), Threads: int(threads.Int64)}
	}
	return v, nil
}

// Get is what the owner and the staff see.
func (h *PerformanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	v, err := h.view(r)
	if err != nil {
		http.Error(w, "Error reading the performance settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// Set changes some of the values (a null goes back to the default of the installation) and applies them at once, in this
// process; the OCR service takes them at the start of its next job. Only the owner.
func (h *PerformanceHandler) Set(w http.ResponseWriter, r *http.Request) {
	var changes map[string]*int
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&changes); err != nil || len(changes) == 0 {
		http.Error(w, "invalid settings", http.StatusBadRequest)
		return
	}
	if err := performance.Validate(changes, h.Machine); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := performance.Save(r.Context(), h.DB, currentUserID(r), changes, h.Machine); err != nil {
		http.Error(w, "Error saving the performance settings", http.StatusInternalServerError)
		return
	}
	overrides, err := performance.Overrides(r.Context(), h.DB, h.Machine)
	if err != nil {
		http.Error(w, "Error reading the performance settings", http.StatusInternalServerError)
		return
	}
	if h.Live != nil {
		h.Live.Apply(overrides)
	}
	h.Get(w, r)
}
