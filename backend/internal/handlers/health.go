package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// HealthHandler reports whether the API can do its job, by component (RF-021).
// It is public and answers only "ok", "down" or "off" per component: no versions,
// addresses or error text, so it is safe to expose to a load balancer or a
// container healthcheck.
//
// The database is required: without it the answer is 503. Redis is not: jobs live
// in PostgreSQL and Redis only wakes the worker, so an unreachable Redis makes the
// service "degraded" but still healthy.
type HealthHandler struct {
	DB    *sql.DB
	Redis *redis.Client // nil when Redis is not used
}

func (h *HealthHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	components := map[string]string{"database": "ok", "redis": "off"}
	status, code := "ok", http.StatusOK

	if h.DB == nil || h.DB.PingContext(ctx) != nil {
		components["database"] = "down"
		status, code = "down", http.StatusServiceUnavailable
	}
	if h.Redis != nil {
		if h.Redis.Ping(ctx).Err() != nil {
			components["redis"] = "down"
			if status == "ok" {
				status = "degraded"
			}
		} else {
			components["redis"] = "ok"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"status": status, "components": components})
}
