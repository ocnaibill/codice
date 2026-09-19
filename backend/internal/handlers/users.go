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
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// UsersHandler manages account roles. Only role changes live here for now;
// invitations, blocking and removal arrive with the accounts phase.
type UsersHandler struct {
	DB *sql.DB
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
