package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/logins"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

// LoginsAdminHandler is the record of sign-ins (DEC-121, #137): the staff reads it all, the owner also sets how long
// it is kept and sees the names typed for accounts that do not exist, and every person reads what happened at the
// door of their own account.
type LoginsAdminHandler struct {
	DB     *sql.DB
	Logins *logins.Recorder
}

var knownResults = map[string]bool{
	logins.Success: true, logins.BadPassword: true, logins.UnknownUser: true, logins.Blocked: true,
	logins.DirectoryUnavailable: true, logins.RateLimited: true, logins.LinkOffered: true, logins.BadAppToken: true,
}

func (h *LoginsAdminHandler) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func parseWhen(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	return t, err == nil
}

// List answers a page of the record, with the filters of the query string (result, username, from, to, before,
// limit), how long it is kept, and whether it looks like the server cannot tell its clients apart.
func (h *LoginsAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := logins.Query{Result: q.Get("result"), Username: q.Get("username")}
	if query.Result != "" && !knownResults[query.Result] {
		http.Error(w, "Unknown result", http.StatusBadRequest)
		return
	}
	var ok1, ok2 bool
	query.From, ok1 = parseWhen(q.Get("from"))
	query.To, ok2 = parseWhen(q.Get("to"))
	if !ok1 || !ok2 {
		http.Error(w, "The period is not valid: use RFC 3339 dates", http.StatusBadRequest)
		return
	}
	if raw := q.Get("before"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			http.Error(w, "The page is not valid", http.StatusBadRequest)
			return
		}
		query.Before = n
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			http.Error(w, "The page is not valid", http.StatusBadRequest)
			return
		}
		query.Limit = n
	}
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	query.ShowTyped = role == "owner"

	entries, more, err := h.Logins.List(r.Context(), query)
	if err != nil {
		http.Error(w, "Error reading the record of sign-ins", http.StatusInternalServerError)
		return
	}
	days, err := logins.Retention(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the record of sign-ins", http.StatusInternalServerError)
		return
	}
	address, warn, err := logins.SameAddress(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the record of sign-ins", http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, map[string]any{
		"entries": entries, "more": more, "retentionDays": days,
		"sameAddress": map[string]any{"warn": warn, "address": address},
	})
}

type retentionRequest struct {
	RetentionDays *int `json:"retentionDays"`
}

// SetRetention sets how many days the record is kept, for the owner.
func (h *LoginsAdminHandler) SetRetention(w http.ResponseWriter, r *http.Request) {
	var req retentionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.RetentionDays == nil {
		http.Error(w, "retentionDays is required", http.StatusBadRequest)
		return
	}
	if d := *req.RetentionDays; d < logins.MinRetentionDays || d > logins.MaxRetentionDays {
		http.Error(w, "retentionDays must be between 7 and 3650", http.StatusBadRequest)
		return
	}
	userID := currentUserID(r)
	if err := logins.SetRetention(r.Context(), h.DB, *req.RetentionDays); err != nil {
		http.Error(w, "Error saving the setting", http.StatusInternalServerError)
		return
	}
	audit.Record(r.Context(), h.DB, userID, "logins.retention", "instance", "login_events", map[string]any{"days": *req.RetentionDays})
	h.writeJSON(w, map[string]int{"retentionDays": *req.RetentionDays})
}

// Own answers what happened at the door of the caller's own account, the latest first: the sign-ins and the attempts
// that failed. It is how a person notices that someone has been trying.
func (h *LoginsAdminHandler) Own(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Logins.Own(r.Context(), currentUserID(r), 20)
	if err != nil {
		http.Error(w, "Error reading the record of sign-ins", http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, entries)
}

// BasicFailureRecorder is what the authenticator calls when an app (OPDS) gives a name and a secret that do not
// match: the name of an account is recorded as that account's, any other as an unknown name.
func BasicFailureRecorder(db *sql.DB, rec *logins.Recorder) func(*http.Request, string) {
	return func(r *http.Request, username string) {
		e := logins.Event{Method: logins.App, IP: middleware.RequestClientIP(r), UserAgent: r.UserAgent()}
		var id string
		if username != "" {
			db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE username = $1`, username).Scan(&id)
		}
		if id != "" {
			e.Result, e.UserID = logins.BadAppToken, id
		} else {
			e.Result, e.Typed = logins.UnknownUser, username
		}
		rec.Record(r.Context(), e)
	}
}
