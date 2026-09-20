package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func health(h *HealthHandler) (int, map[string]any) {
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest("GET", "/healthz", nil))
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func deadRedis() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
}

func TestHealth_ReportsEachComponentAndOnlyTheDatabaseIsRequired(t *testing.T) {
	db := migratedDB(t)

	if code, body := health(&HealthHandler{DB: db}); code != http.StatusOK || body["status"] != "ok" ||
		body["components"].(map[string]any)["redis"] != "off" {
		t.Errorf("no Redis configured: %d %v", code, body)
	}

	// Redis unreachable: the service still works (jobs live in PostgreSQL), so it is
	// degraded, not down, and a container healthcheck must keep passing.
	code, body := health(&HealthHandler{DB: db, Redis: deadRedis()})
	c := body["components"].(map[string]any)
	if code != http.StatusOK || body["status"] != "degraded" || c["redis"] != "down" || c["database"] != "ok" {
		t.Errorf("Redis down: %d %v", code, body)
	}

	db.Close()
	code, body = health(&HealthHandler{DB: db, Redis: deadRedis()})
	if code != http.StatusServiceUnavailable || body["status"] != "down" || body["components"].(map[string]any)["database"] != "down" {
		t.Errorf("database down: %d %v", code, body)
	}
}

func TestHealth_LeaksNothingAboutTheDeployment(t *testing.T) {
	db := migratedDB(t)
	db.Close()
	rec := httptest.NewRecorder()
	(&HealthHandler{DB: db, Redis: deadRedis()}).Get(rec, httptest.NewRequest("GET", "/healthz", nil))
	for _, leak := range []string{"127.0.0.1", "connection", "refused", "sql:", "postgres", "closed"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), leak) {
			t.Errorf("the health answer mentions %q: %s", leak, rec.Body.String())
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("a health answer must not be cached")
	}
}
