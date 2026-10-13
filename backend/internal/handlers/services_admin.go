package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// ServicesHandler says, for the administration, whether each part of the system is working (DEC-176). It is the "Serviços" of the system
// tab: the database and Redis are asked; the others say it themselves, in the settings table, and are taken to be down when they have not
// said it in the last two minutes. A service the owner has not turned on is "off", which is not a defect.
type ServicesHandler struct {
	DB    *sql.DB
	Redis *redis.Client // nil when Redis is not used
}

// ServiceStatus is one part of the system.
type ServiceStatus struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"` // "ok", "down", "error" or "off"
	Detail string `json:"detail,omitempty"`
}

func beatStatus(id, label string, on bool, beat serviceBeat, offDetail, downDetail string) ServiceStatus {
	switch {
	case !on:
		return ServiceStatus{id, label, "off", offDetail}
	case !beat.Fresh:
		return ServiceStatus{id, label, "down", downDetail}
	case beat.text("state") == "error":
		detail := beat.text("error")
		if detail == "" {
			detail = "O serviço avisou que não consegue trabalhar."
		}
		return ServiceStatus{id, label, "error", detail}
	}
	return ServiceStatus{ID: id, Label: label, Status: "ok"}
}

// Get answers GET /admin/services (owner and admin).
func (h *ServicesHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out := []ServiceStatus{}

	db := ServiceStatus{ID: "database", Label: "Banco de dados", Status: "ok"}
	if h.DB == nil || h.DB.PingContext(ctx) != nil {
		db.Status, db.Detail = "down", "Nada funciona sem ele."
	}
	out = append(out, db)

	cache := ServiceStatus{ID: "redis", Label: "Redis", Status: "off", Detail: "Não usado."}
	if h.Redis != nil {
		cache.Status, cache.Detail = "ok", ""
		if h.Redis.Ping(ctx).Err() != nil {
			cache.Status, cache.Detail = "down", "Nada se perde: os trabalhos esperam no banco."
		}
	}
	out = append(out, cache)

	if db.Status == "down" {
		writeJSON(w, http.StatusOK, map[string]any{"services": out})
		return
	}

	beat := func(key string) serviceBeat {
		b, err := readServiceBeat(ctx, h.DB, key)
		if err != nil {
			return serviceBeat{Value: map[string]any{}}
		}
		return b
	}
	setting := func(key, field, otherwise string) string {
		var v string
		_ = h.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT value->>$2 FROM settings WHERE key = $1), $3)`, key, field, otherwise).Scan(&v)
		return v
	}

	out = append(out, beatStatus("jobs", "Serviço de trabalhos (worker)", true, beat("worker.jobs"), "",
		"Nenhum aviso nos últimos 2 minutos: os arquivos novos não são analisados."))
	out = append(out, beatStatus("ocr", "OCR de PDFs escaneados", setting("ocr", "enabled", "false") == "true", beat("ocr.worker"),
		"Desligado.", "O serviço de OCR não está em execução."))
	choice := setting(comicOCRSetting, "model", noModel)
	comic := beatStatus("comic_ocr", "OCR de quadrinhos e mangás", choice == "mobile" || choice == "server", beat(comicOCRWorker),
		"Nenhum modelo escolhido.", "O serviço de OCR de quadrinhos não está em execução.")
	out = append(out, comic)
	out = append(out, beatStatus("semantic", "Correspondência semântica", setting("equivalence.embeddings", "enabled", "false") == "true",
		beat("embeddings.worker"), "Desligada.", "O worker de embeddings não está em execução."))

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"services": out})
}
