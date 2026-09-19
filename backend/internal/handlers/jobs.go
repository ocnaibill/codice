package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/redis/go-redis/v9"
)

// JobsHandler is the administration of asynchronous jobs (RF-020): see what is
// waiting, running, failed, and rerun or cancel it. Owner and admin only.
type JobsHandler struct {
	DB          *sql.DB
	RedisClient *redis.Client
	StoragePath string // root of the managed storage, to rebuild a file's absolute path
}

// List returns jobs newest first, optionally of one state (?state=failed), with
// the number of jobs in each state so the page can show a summary.
func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, counts, err := jobs.List(r.Context(), h.DB, r.URL.Query().Get("state"), limit)
	if err != nil {
		log.Println("Error listing jobs:", err)
		http.Error(w, "Error listing jobs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": list, "counts": counts})
}

// Rerun puts a failed or cancelled job back in the queue. An ingestion job keeps
// the absolute path the file had when the job was created, and the file may have
// been moved since, so the path is refreshed from where the file is now.
func (h *JobsHandler) Rerun(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, "job.rerun", func(ctx context.Context, db *sql.DB, id int64) error {
		if _, err := db.ExecContext(ctx, `
			UPDATE jobs j SET payload = jsonb_set(j.payload, '{file_path}', to_jsonb(
			       CASE WHEN l.mode = 'managed' THEN $2::text || '/' || l.path ELSE l.root || '/' || l.path END))
			FROM work_primary wp JOIN storage_locations l ON l.file_id = wp.file_id
			WHERE j.id = $1 AND j.type = 'ingest' AND wp.work_id = j.work_id AND j.state IN ('failed', 'cancelled')`,
			id, h.StoragePath); err != nil {
			return err
		}
		return jobs.Rerun(ctx, db, id)
	})
}

// Cancel stops a pending job and asks a running one to stop.
func (h *JobsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, "job.cancel", jobs.Cancel)
}

func (h *JobsHandler) act(w http.ResponseWriter, r *http.Request, action string,
	do func(ctx context.Context, db *sql.DB, id int64) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	if err := do(r.Context(), h.DB, id); err != nil {
		switch {
		case errors.Is(err, jobs.ErrNotFound):
			http.Error(w, "Job not found", http.StatusNotFound)
		case errors.Is(err, jobs.ErrState):
			http.Error(w, "The job is not in a state that allows this", http.StatusConflict)
		default:
			log.Println("Job action failed:", err)
			http.Error(w, "Error updating job", http.StatusInternalServerError)
		}
		return
	}
	if err := audit.Record(r.Context(), h.DB, currentUserID(r), action, "job", strconv.FormatInt(id, 10), nil); err != nil {
		log.Println("Could not audit", action, err)
	}
	if action == "job.rerun" {
		// Wake the workers; a failure only means they will find it on their next poll.
		jobs.Notify(r.Context(), h.RedisClient, id)
	}
	w.WriteHeader(http.StatusOK)
}
