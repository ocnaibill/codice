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

// Rerun puts a failed or cancelled job back in the queue.
func (h *JobsHandler) Rerun(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, "job.rerun", jobs.Rerun)
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
