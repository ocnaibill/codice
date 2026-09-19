package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// UsersHandler manages accounts: listing, roles and blocking. Invitations,
// removal and password resets arrive in later slices of the accounts phase.
type UsersHandler struct {
	DB *sql.DB
	// Disconnect, when set, closes the live connections of an account that was
	// just blocked.
	Disconnect func(userID string)
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

// UpdateRole promotes a reader to admin or demotes an admin to reader
// (DEC-056). The actor's role is read from the database, not the token, so a
// stale token cannot exercise a role the account no longer has. Owner is never
// assignable here: it only changes hands through an explicit transfer.
func (h *UsersHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if !uuidPattern.MatchString(targetID) {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !authz.IsAssignableRole(req.Role) {
		http.Error(w, "role must be 'admin' or 'reader'", http.StatusBadRequest)
		return
	}

	actorID, _ := r.Context().Value(middleware.UserIDKey).(string)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var actorRole, targetRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1 FOR UPDATE`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1 FOR UPDATE`, targetID).Scan(&targetRole); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}

	action := authz.ActionDemote
	if req.Role == authz.RoleAdmin {
		action = authz.ActionPromote
	}
	if !authz.CanManageAccount(actorRole, targetRole, action) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if targetRole != req.Role {
		if _, err := tx.Exec(`UPDATE users SET role = $1 WHERE id = $2`, req.Role, targetID); err != nil {
			http.Error(w, "Error updating role", http.StatusInternalServerError)
			return
		}
		if err := audit.Record(r.Context(), tx, actorID, "user.role_change", "user", targetID,
			map[string]any{"from": targetRole, "to": req.Role}); err != nil {
			http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing role change", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": targetID, "role": req.Role})
}

type accountView struct {
	ID        string     `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	BlockedAt *time.Time `json:"blockedAt"`
	CreatedAt *time.Time `json:"createdAt"`
	// CanBlock says whether the caller may block or unblock this account, so the
	// interface never has to repeat the policy.
	// External names the directory the account signs in through, if any.
	External  string `json:"external,omitempty"`
	CanBlock  bool   `json:"canBlock"`
	CanRemove bool   `json:"canRemove"`
	IsSelf    bool   `json:"isSelf"`
}

// List shows the accounts to the owner and admins. The caller's role comes from
// the database, and nothing secret (hash, external ids) is returned.
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	actorID, _ := r.Context().Value(middleware.UserIDKey).(string)
	var actorRole string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id = $1`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, username, email, COALESCE(role, 'reader'), blocked_at, created_at,
		       COALESCE((SELECT e.provider FROM external_identities e WHERE e.user_id = users.id ORDER BY e.id LIMIT 1), '')
		FROM users ORDER BY (role = 'owner') DESC, (role = 'admin') DESC, username`)
	if err != nil {
		http.Error(w, "Error listing accounts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []accountView{}
	for rows.Next() {
		var a accountView
		if err := rows.Scan(&a.ID, &a.Username, &a.Email, &a.Role, &a.BlockedAt, &a.CreatedAt, &a.External); err != nil {
			http.Error(w, "Error reading accounts", http.StatusInternalServerError)
			return
		}
		a.IsSelf = a.ID == actorID
		a.CanBlock = !a.IsSelf && authz.CanManageAccount(actorRole, a.Role, authz.ActionBlock)
		a.CanRemove = !a.IsSelf && authz.CanManageAccount(actorRole, a.Role, authz.ActionRemove)
		out = append(out, a)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// Block stops an account from signing in and ends its sessions, app tokens and
// live connections at once (DEC-060). It is reversible and keeps notes and
// progress; deleting an account is a separate action.
func (h *UsersHandler) Block(w http.ResponseWriter, r *http.Request) { h.setBlocked(w, r, true) }

// Unblock lets a blocked account sign in again. Its old sessions stay ended.
func (h *UsersHandler) Unblock(w http.ResponseWriter, r *http.Request) { h.setBlocked(w, r, false) }

func (h *UsersHandler) setBlocked(w http.ResponseWriter, r *http.Request, block bool) {
	targetID := chi.URLParam(r, "id")
	if !uuidPattern.MatchString(targetID) {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	actorID, _ := r.Context().Value(middleware.UserIDKey).(string)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var actorRole, targetRole string
	var blockedAt sql.NullTime
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1 FOR UPDATE`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if actorID != targetID {
		err = tx.QueryRow(`SELECT role, blocked_at FROM users WHERE id = $1 FOR UPDATE`, targetID).Scan(&targetRole, &blockedAt)
	} else {
		err = tx.QueryRow(`SELECT role, blocked_at FROM users WHERE id = $1`, targetID).Scan(&targetRole, &blockedAt)
	}
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	if actorID == targetID || !authz.CanManageAccount(actorRole, targetRole, authz.ActionBlock) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	action := "user.unblock"
	if block {
		action = "user.block"
	}
	// Already in the wanted state: nothing to record. A block still re-ends any
	// access, which costs nothing and closes a half-finished earlier attempt.
	changed := blockedAt.Valid != block
	if changed {
		query := `UPDATE users SET blocked_at = NULL WHERE id = $1`
		if block {
			query = `UPDATE users SET blocked_at = now() WHERE id = $1`
		}
		if _, err := tx.Exec(query, targetID); err != nil {
			http.Error(w, "Error updating the account", http.StatusInternalServerError)
			return
		}
	}
	if block {
		if err := sessions.RevokeAllForUserTx(r.Context(), tx, targetID); err != nil {
			http.Error(w, "Error ending the account's access", http.StatusInternalServerError)
			return
		}
		// Links the account handed out but nobody used yet die with its access.
		if err := revokeIssuedInvitations(r.Context(), tx, targetID, actorID); err != nil {
			http.Error(w, "Error ending the account's invitations", http.StatusInternalServerError)
			return
		}
	}
	if changed {
		if err := audit.Record(r.Context(), tx, actorID, action, "user", targetID, map[string]any{"role": targetRole}); err != nil {
			http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing the change", http.StatusInternalServerError)
		return
	}
	if block && h.Disconnect != nil {
		h.Disconnect(targetID)
	}
	w.WriteHeader(http.StatusNoContent)
}

type deleteAccountRequest struct {
	// ConfirmUsername must repeat the account's username: deleting is permanent,
	// so a click is not enough.
	ConfirmUsername string `json:"confirmUsername"`
}

// Delete removes an account and its personal data for good (DEC-060): notes,
// favourites, reading progress, sessions, app tokens and linked identities. It is
// separate from blocking, which is reversible and keeps them. Works, files and
// the audit log are not personal data and stay; the audit entries keep the
// username they were written with. Nobody deletes the owner or themself, and an
// admin deletes readers only.
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if !uuidPattern.MatchString(targetID) {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	var req deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	actorID, _ := r.Context().Value(middleware.UserIDKey).(string)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var actorRole, targetRole, username string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1 FOR UPDATE`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if actorID == targetID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	err = tx.QueryRow(`SELECT role, username FROM users WHERE id = $1 FOR UPDATE`, targetID).Scan(&targetRole, &username)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	if !authz.CanManageAccount(actorRole, targetRole, authz.ActionRemove) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if req.ConfirmUsername != username {
		http.Error(w, "confirmUsername must repeat the account's username", http.StatusBadRequest)
		return
	}

	var notes, favorites, progress int
	if err := tx.QueryRow(`
		SELECT (SELECT count(*) FROM notes WHERE user_id = $1),
		       (SELECT count(*) FROM favorites WHERE user_id = $1),
		       (SELECT count(*) FROM reading_progress WHERE user_id = $1)`, targetID).Scan(&notes, &favorites, &progress); err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	if err := revokeIssuedInvitations(r.Context(), tx, targetID, actorID); err != nil {
		http.Error(w, "Error ending the account's invitations", http.StatusInternalServerError)
		return
	}
	// Recorded first, with counts only: the content of the notes is never read.
	if err := audit.Record(r.Context(), tx, actorID, "user.delete", "user", targetID, map[string]any{
		"username": username, "role": targetRole, "notes": notes, "favorites": favorites, "filesWithProgress": progress,
	}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = $1`, targetID); err != nil {
		http.Error(w, "Error deleting the account", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing the change", http.StatusInternalServerError)
		return
	}
	if h.Disconnect != nil {
		h.Disconnect(targetID)
	}
	w.WriteHeader(http.StatusNoContent)
}
