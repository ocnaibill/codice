package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
)

// SessionsHandler is "Sessões e dispositivos" (UI-15, UI-21): a person sees where their account is signed in
// and ends what is not theirs; whoever may cut an account's access (DEC-060) can end its sessions too.
// Ending is immediate (DEC-070): the very next request of that session is refused.
type SessionsHandler struct {
	DB       *sql.DB
	Sessions *sessions.Store
}

type sessionView struct {
	sessions.Session
	// Current marks the session the request came from.
	Current bool `json:"current"`
}

func (h *SessionsHandler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// List answers with the live sessions of the caller's own account, the one in use marked.
func (h *SessionsHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	current, _ := r.Context().Value(middleware.SessionIDKey).(string)
	list, err := h.Sessions.ListLive(r.Context(), userID)
	if err != nil {
		http.Error(w, "Error reading the sessions", http.StatusInternalServerError)
		return
	}
	out := make([]sessionView, 0, len(list))
	for _, s := range list {
		out = append(out, sessionView{Session: s, Current: s.ID == current})
	}
	h.writeJSON(w, http.StatusOK, out)
}

// Revoke ends one of the caller's own sessions (ending the one in use is signing out).
func (h *SessionsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	ok, err := h.Sessions.RevokeOwned(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Error ending the session", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	audit.Record(r.Context(), h.DB, userID, "session.revoke", "user", userID, map[string]any{"by": "owner of the account"})
	w.WriteHeader(http.StatusNoContent)
}

// RevokeOthers ends every session of the caller's account but the one in use.
func (h *SessionsHandler) RevokeOthers(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	current, _ := r.Context().Value(middleware.SessionIDKey).(string)
	n, err := h.Sessions.RevokeOthers(r.Context(), userID, current)
	if err != nil {
		http.Error(w, "Error ending the sessions", http.StatusInternalServerError)
		return
	}
	if n > 0 {
		audit.Record(r.Context(), h.DB, userID, "session.revoke_others", "user", userID, map[string]any{"count": n})
	}
	h.writeJSON(w, http.StatusOK, map[string]int{"revoked": n})
}

// target loads the account whose sessions the staff asks about, and refuses unless the caller may cut its
// access: the owner reaches admins and readers, an admin reaches readers, nobody reaches the owner or
// themselves (their own are at /auth/sessions).
func (h *SessionsHandler) target(w http.ResponseWriter, r *http.Request) (targetID string, ok bool) {
	targetID = chi.URLParam(r, "id")
	if !uuidPattern.MatchString(targetID) {
		http.Error(w, "User not found", http.StatusNotFound)
		return "", false
	}
	actorRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	var targetRole string
	err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id = $1`, targetID).Scan(&targetRole)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return "", false
	}
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return "", false
	}
	if targetID == currentUserID(r) || !authz.CanManageAccount(actorRole, targetRole, authz.ActionEndSessions) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return "", false
	}
	return targetID, true
}

// ListFor answers with the live sessions of another account, for the staff. The address is not in it: where a
// person signs in from is theirs, and the staff has the record of sign-ins for what it needs.
func (h *SessionsHandler) ListFor(w http.ResponseWriter, r *http.Request) {
	targetID, ok := h.target(w, r)
	if !ok {
		return
	}
	list, err := h.Sessions.ListLive(r.Context(), targetID)
	if err != nil {
		http.Error(w, "Error reading the sessions", http.StatusInternalServerError)
		return
	}
	for i := range list {
		list[i].IP = ""
	}
	h.writeJSON(w, http.StatusOK, list)
}

// RevokeFor ends one session of another account, for the staff.
func (h *SessionsHandler) RevokeFor(w http.ResponseWriter, r *http.Request) {
	targetID, ok := h.target(w, r)
	if !ok {
		return
	}
	done, err := h.Sessions.RevokeOwned(r.Context(), targetID, chi.URLParam(r, "sid"))
	if err != nil {
		http.Error(w, "Error ending the session", http.StatusInternalServerError)
		return
	}
	if !done {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	audit.Record(r.Context(), h.DB, currentUserID(r), "session.revoke", "user", targetID, map[string]any{"by": "staff"})
	w.WriteHeader(http.StatusNoContent)
}

// RevokeAllFor ends every session of another account, for the staff, without blocking it.
func (h *SessionsHandler) RevokeAllFor(w http.ResponseWriter, r *http.Request) {
	targetID, ok := h.target(w, r)
	if !ok {
		return
	}
	n, err := h.Sessions.RevokeAllSessions(r.Context(), targetID)
	if err != nil {
		http.Error(w, "Error ending the sessions", http.StatusInternalServerError)
		return
	}
	if n > 0 {
		audit.Record(r.Context(), h.DB, currentUserID(r), "session.revoke_all", "user", targetID, map[string]any{"count": n, "by": "staff"})
	}
	h.writeJSON(w, http.StatusOK, map[string]int{"revoked": n})
}
