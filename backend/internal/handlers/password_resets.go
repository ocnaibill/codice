package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"golang.org/x/crypto/bcrypt"
)

const (
	resetRequestTTL = 7 * 24 * time.Hour // how long a request waits for a decision
	resetLinkTTL    = time.Hour          // how long an approved link works
)

// PasswordResetsHandler is the password reset without e-mail (DEC-063, RF-049):
// the person asks on the login page, the owner or an admin approves and hands the
// link over by their own means, and the person sets a new password with it.
type PasswordResetsHandler struct {
	DB *sql.DB
	// Disconnect closes the live connections of an account whose access was ended.
	Disconnect func(userID string)
}

// Request records that someone forgot their password. It answers the same way
// whether or not the account exists, and never records a request for an account
// that cannot be reset here (the owner, a blocked account, one without a local
// password), so the endpoint reveals nothing about which accounts exist.
func (h *PasswordResetsHandler) Request(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	username := strings.TrimSpace(req.Username)

	if username != "" && len(username) <= 50 {
		if err := h.record(r, username); err != nil {
			log.Println("Error recording a password reset request:", err)
			http.Error(w, "Error recording the request", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "If the account exists, the request was sent to the people who administer this library."})
}

func (h *PasswordResetsHandler) record(r *http.Request, username string) error {
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID, role string
	var blocked bool
	var hasPassword bool
	err = tx.QueryRow(`SELECT id, COALESCE(role, 'reader'), blocked_at IS NOT NULL, COALESCE(password_hash, '') <> ''
		FROM users WHERE username = $1`, username).Scan(&userID, &role, &blocked, &hasPassword)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// The owner recovers access with the local command (DEC-054, DEC-057), not here.
	if role == authz.RoleOwner || blocked || !hasPassword {
		return nil
	}

	// A request that waited too long lapses, so a new one can replace it.
	if _, err := tx.Exec(`UPDATE password_resets SET decision = 'expired', decided_at = now()
		WHERE user_id = $1 AND decision IS NULL AND created_at <= now() - $2::interval`, userID, resetRequestTTL.String()); err != nil {
		return err
	}
	var id string
	err = tx.QueryRow(`INSERT INTO password_resets (user_id) VALUES ($1) ON CONFLICT DO NOTHING RETURNING id`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // already waiting: repeated requests consolidate
	}
	if err != nil {
		return err
	}
	if err := audit.Record(r.Context(), tx, "", "password_reset.request", "user", userID, map[string]any{"username": username, "role": role}); err != nil {
		return err
	}
	return tx.Commit()
}

type resetView struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	State       string     `json:"state"` // requested, approved, rejected, expired or used
	RequestedAt time.Time  `json:"requestedAt"`
	DecidedBy   string     `json:"decidedBy,omitempty"`
	LinkExpires *time.Time `json:"linkExpiresAt,omitempty"`
	// CanDecide says whether the caller may approve or reject this request.
	CanDecide bool `json:"canDecide"`
}

// List shows the requests the caller may see: an admin sees readers' only, the
// owner sees everyone's (RF-049, DEC-056).
func (h *PasswordResetsHandler) List(w http.ResponseWriter, r *http.Request) {
	actorID := currentUserID(r)
	var actorRole string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id = $1`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT p.id, u.username, COALESCE(u.role, 'reader'), p.created_at, COALESCE(d.username, ''), p.token_expires_at,
		       CASE WHEN p.used_at IS NOT NULL THEN 'used'
		            WHEN p.decision = 'rejected' THEN 'rejected'
		            WHEN p.decision = 'expired' THEN 'expired'
		            WHEN p.decision = 'approved' THEN CASE WHEN p.token_expires_at <= now() THEN 'expired' ELSE 'approved' END
		            WHEN p.created_at <= now() - $1::interval THEN 'expired'
		            ELSE 'requested' END
		FROM password_resets p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN users d ON d.id = p.decided_by
		ORDER BY p.created_at DESC LIMIT 100`, resetRequestTTL.String())
	if err != nil {
		http.Error(w, "Error listing requests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []resetView{}
	for rows.Next() {
		var v resetView
		if err := rows.Scan(&v.ID, &v.Username, &v.Role, &v.RequestedAt, &v.DecidedBy, &v.LinkExpires, &v.State); err != nil {
			http.Error(w, "Error reading requests", http.StatusInternalServerError)
			return
		}
		if !authz.CanManageAccount(actorRole, v.Role, authz.ActionReset) {
			continue // not this caller's to see
		}
		v.CanDecide = v.State == "requested"
		out = append(out, v)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// lockRequest loads an open request and its account for a decision, checking the
// caller's reach. It writes the error response itself and reports whether to go on.
func (h *PasswordResetsHandler) lockRequest(w http.ResponseWriter, r *http.Request, tx *sql.Tx) (id, userID, username, role, actorID string, ok bool) {
	id = chi.URLParam(r, "id")
	if !uuidPattern.MatchString(id) {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}
	actorID = currentUserID(r)
	var actorRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var decision sql.NullString
	var created time.Time
	var blocked, hasPassword bool
	err := tx.QueryRow(`
		SELECT p.user_id, u.username, COALESCE(u.role, 'reader'), p.decision, p.created_at,
		       u.blocked_at IS NOT NULL, COALESCE(u.password_hash, '') <> ''
		FROM password_resets p JOIN users u ON u.id = p.user_id
		WHERE p.id = $1 FOR UPDATE OF p`, id).Scan(&userID, &username, &role, &decision, &created, &blocked, &hasPassword)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	if !authz.CanManageAccount(actorRole, role, authz.ActionReset) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if decision.Valid || time.Since(created) > resetRequestTTL {
		http.Error(w, "The request is no longer waiting for a decision", http.StatusConflict)
		return
	}
	if blocked || !hasPassword {
		http.Error(w, "This account cannot be reset from here", http.StatusConflict)
		return
	}
	ok = true
	return
}

// Approve turns a request into a single-use link, valid for one hour. The link is
// returned once and only its hash is kept. Whoever approves confirms the person's
// identity by their own means before handing the link over.
func (h *PasswordResetsHandler) Approve(w http.ResponseWriter, r *http.Request) {
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	id, userID, username, role, actorID, ok := h.lockRequest(w, r, tx)
	if !ok {
		return
	}
	token, err := newInvitationToken()
	if err != nil {
		http.Error(w, "Error creating the link", http.StatusInternalServerError)
		return
	}
	var expires time.Time
	if err := tx.QueryRow(`UPDATE password_resets
		SET decision = 'approved', decided_by = $2, decided_at = now(), token_hash = $3, token_expires_at = now() + $4::interval
		WHERE id = $1 RETURNING token_expires_at`, id, actorID, hashInvitation(token), resetLinkTTL.String()).Scan(&expires); err != nil {
		http.Error(w, "Error approving the request", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, actorID, "password_reset.approve", "user", userID,
		map[string]any{"username": username, "role": role, "linkExpiresAt": expires}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error approving the request", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id, "username": username, "token": token, "expiresAt": expires})
}

// Reject closes a request without changing anything.
func (h *PasswordResetsHandler) Reject(w http.ResponseWriter, r *http.Request) {
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	id, userID, username, _, actorID, ok := h.lockRequest(w, r, tx)
	if !ok {
		return
	}
	if _, err := tx.Exec(`UPDATE password_resets SET decision = 'rejected', decided_by = $2, decided_at = now() WHERE id = $1`, id, actorID); err != nil {
		http.Error(w, "Error rejecting the request", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, actorID, "password_reset.reject", "user", userID, map[string]any{"username": username}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error rejecting the request", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const invalidResetMessage = "This reset link is not valid"

// Check tells the reset page whether a link is usable. Unknown, used, expired and
// rejected links all get the same answer.
func (h *PasswordResetsHandler) Check(w http.ResponseWriter, r *http.Request) {
	var username string
	var expires time.Time
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT u.username, p.token_expires_at FROM password_resets p JOIN users u ON u.id = p.user_id
		WHERE p.token_hash = $1 AND p.decision = 'approved' AND p.used_at IS NULL AND p.token_expires_at > now()
		  AND u.blocked_at IS NULL`, hashInvitation(r.URL.Query().Get("token"))).Scan(&username, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, invalidResetMessage, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error checking the link", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"username": username, "expiresAt": expires})
}

// Redeem sets the new password with an approved link. It ends every session and
// app token of the account (DEC-070, DEC-071), so whoever had access before the
// reset loses it. The link is consumed in the same transaction.
func (h *PasswordResetsHandler) Redeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if len(req.Password) < minPasswordLength {
		http.Error(w, "The password must have at least 8 characters", http.StatusBadRequest)
		return
	}
	// Hash first, for real and made-up links alike, so timing does not tell them apart.
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		http.Error(w, "Error processing password hash", http.StatusInternalServerError)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var id, userID string
	err = tx.QueryRow(`
		SELECT p.id, p.user_id FROM password_resets p JOIN users u ON u.id = p.user_id
		WHERE p.token_hash = $1 AND p.decision = 'approved' AND p.used_at IS NULL AND p.token_expires_at > now()
		  AND u.blocked_at IS NULL
		FOR UPDATE OF p`, hashInvitation(req.Token)).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, invalidResetMessage, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	// Conditional, so that even without the lock only one use can win.
	res, err := tx.Exec(`UPDATE password_resets SET used_at = now() WHERE id = $1 AND used_at IS NULL`, id)
	if err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		http.Error(w, invalidResetMessage, http.StatusNotFound)
		return
	}
	if _, err := tx.Exec(`UPDATE users SET password_hash = $2 WHERE id = $1`, userID, string(hashed)); err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	// Any other approved link for this account dies with this use.
	if _, err := tx.Exec(`UPDATE password_resets SET token_expires_at = now()
		WHERE user_id = $1 AND id <> $2 AND decision = 'approved' AND used_at IS NULL`, userID, id); err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	if err := sessions.RevokeAllForUserTx(r.Context(), tx, userID); err != nil {
		http.Error(w, "Error ending the account's access", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, userID, "password_reset.use", "user", userID, nil); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	if h.Disconnect != nil {
		h.Disconnect(userID)
	}
	w.WriteHeader(http.StatusNoContent)
}
