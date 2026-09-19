package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

const (
	invitationTTL     = 7 * 24 * time.Hour
	minPasswordLength = 8
)

// InvitationsHandler issues, lists, revokes and redeems invitations. The link is
// handed over outside the system (DEC-059): the secret is shown once, when the
// invitation is created, and never stored or logged.
type InvitationsHandler struct {
	DB       *sql.DB
	Sessions interface {
		CreateSession(ctx context.Context, userID, userAgent string) (string, time.Time, error)
	}
}

func hashInvitation(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newInvitationToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

type createInvitationRequest struct {
	Role  string `json:"role"`
	Email string `json:"email"`
}

// Create issues an invitation. An admin may invite readers only; inviting an
// admin is the owner's alone, which closes "invite as admin, skip the promotion
// rule" (DEC-056). No invitation grants owner.
func (h *InvitationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = authz.RoleReader
	}
	if !authz.IsAssignableRole(req.Role) {
		http.Error(w, "role must be 'admin' or 'reader'", http.StatusBadRequest)
		return
	}
	email := normalizeEmail(req.Email)
	if email != "" && (!strings.Contains(email, "@") || len(email) > 255) {
		http.Error(w, "The email address is not valid", http.StatusBadRequest)
		return
	}

	actorID := currentUserID(r)
	var actorRole string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id = $1 AND blocked_at IS NULL`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !authz.CanInvite(actorRole, req.Role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	token, err := newInvitationToken()
	if err != nil {
		http.Error(w, "Error creating the invitation", http.StatusInternalServerError)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error creating the invitation", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var id string
	var expires time.Time
	err = tx.QueryRow(`
		INSERT INTO invitations (token_hash, role, email, created_by, expires_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, now() + $5::interval) RETURNING id, expires_at`,
		hashInvitation(token), req.Role, email, actorID, invitationTTL.String()).Scan(&id, &expires)
	if err != nil {
		log.Println("Error creating invitation:", err)
		http.Error(w, "Error creating the invitation", http.StatusInternalServerError)
		return
	}
	// The token is never part of the audit entry.
	if err := audit.Record(r.Context(), tx, actorID, "invitation.create", "invitation", id,
		map[string]any{"role": req.Role, "emailRestricted": email != "", "expiresAt": expires}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error creating the invitation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id, "token": token, "role": req.Role, "email": email, "expiresAt": expires})
}

type invitationView struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Email     string     `json:"email,omitempty"`
	State     string     `json:"state"` // pending, used, expired or revoked
	CreatedBy string     `json:"createdBy,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	UsedBy    string     `json:"usedBy,omitempty"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	// CanRevoke says whether the caller may revoke it (only pending ones, and an
	// admin may not touch an admin invitation).
	CanRevoke bool `json:"canRevoke"`
}

// List shows the latest invitations with their state. Secrets are not stored, so
// they cannot be shown again.
func (h *InvitationsHandler) List(w http.ResponseWriter, r *http.Request) {
	actorID := currentUserID(r)
	var actorRole string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id = $1`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT i.id, i.role, COALESCE(i.email, ''),
		       CASE WHEN i.used_at IS NOT NULL THEN 'used' WHEN i.revoked_at IS NOT NULL THEN 'revoked'
		            WHEN i.expires_at <= now() THEN 'expired' ELSE 'pending' END,
		       COALESCE(c.username, ''), i.created_at, i.expires_at, COALESCE(u.username, ''), i.used_at
		FROM invitations i
		LEFT JOIN users c ON c.id = i.created_by
		LEFT JOIN users u ON u.id = i.used_by
		ORDER BY i.created_at DESC LIMIT 100`)
	if err != nil {
		http.Error(w, "Error listing invitations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []invitationView{}
	for rows.Next() {
		var v invitationView
		if err := rows.Scan(&v.ID, &v.Role, &v.Email, &v.State, &v.CreatedBy, &v.CreatedAt, &v.ExpiresAt, &v.UsedBy, &v.UsedAt); err != nil {
			http.Error(w, "Error reading invitations", http.StatusInternalServerError)
			return
		}
		v.CanRevoke = v.State == "pending" && authz.CanInvite(actorRole, v.Role)
		out = append(out, v)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// Revoke cancels an invitation that has not been used yet.
func (h *InvitationsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !uuidPattern.MatchString(id) {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}
	actorID := currentUserID(r)
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error revoking the invitation", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var actorRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1`, actorID).Scan(&actorRole); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var role string
	var usedAt, revokedAt sql.NullTime
	var expires time.Time
	err = tx.QueryRow(`SELECT role, used_at, revoked_at, expires_at FROM invitations WHERE id = $1 FOR UPDATE`, id).
		Scan(&role, &usedAt, &revokedAt, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error revoking the invitation", http.StatusInternalServerError)
		return
	}
	if !authz.CanInvite(actorRole, role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if usedAt.Valid || revokedAt.Valid || !expires.After(time.Now()) {
		http.Error(w, "The invitation is no longer pending", http.StatusConflict)
		return
	}
	if _, err := tx.Exec(`UPDATE invitations SET revoked_at = now(), revoked_by = $2 WHERE id = $1`, id, actorID); err != nil {
		http.Error(w, "Error revoking the invitation", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, actorID, "invitation.revoke", "invitation", id, map[string]any{"role": role}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error revoking the invitation", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const invalidInvitationMessage = "This invitation is not valid"

// Check tells the sign-up page whether a link is usable. Unknown, used, expired
// and revoked links all get the same answer, so the endpoint reveals nothing
// about which of them a guess was.
func (h *InvitationsHandler) Check(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	var role string
	var email sql.NullString
	var expires time.Time
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT role, email, expires_at FROM invitations
		WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()`,
		hashInvitation(token)).Scan(&role, &email, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, invalidInvitationMessage, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error checking the invitation", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"role": role, "emailRestricted": email.Valid, "expiresAt": expires})
}

type redeemRequest struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// Redeem creates the account and consumes the invitation in one transaction. The
// invitation row is locked while it is checked, so two requests with the same
// link cannot both succeed; and if creating the account fails (name already
// taken, say) the invitation stays usable.
func (h *InvitationsHandler) Redeem(w http.ResponseWriter, r *http.Request) {
	var req redeemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Username) > 50 {
		http.Error(w, "A username of up to 50 characters is required", http.StatusBadRequest)
		return
	}
	if len(req.Password) < minPasswordLength {
		http.Error(w, "The password must have at least 8 characters", http.StatusBadRequest)
		return
	}

	// Hash first, for valid and invalid links alike: bcrypt is slow, and this keeps
	// the response time from telling a real link from a guess.
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		http.Error(w, "Error processing password hash", http.StatusInternalServerError)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error creating the account", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var invID, role string
	var invEmail sql.NullString
	err = tx.QueryRow(`
		SELECT id, role, email FROM invitations
		WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()
		FOR UPDATE`, hashInvitation(req.Token)).Scan(&invID, &role, &invEmail)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, invalidInvitationMessage, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error creating the account", http.StatusInternalServerError)
		return
	}

	email := normalizeEmail(req.Email)
	if invEmail.Valid {
		// Restricted to one address: it is compared, and a different one is refused.
		if email != "" && email != invEmail.String {
			http.Error(w, "This invitation is for another email address", http.StatusForbidden)
			return
		}
		email = invEmail.String
	}
	if email == "" {
		email = req.Username + "@codice.local"
	}

	var userID string
	err = tx.QueryRow(`INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
		req.Username, email, string(hashed), role).Scan(&userID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			http.Error(w, "That username or email is already taken", http.StatusConflict)
			return
		}
		http.Error(w, "Error creating the account", http.StatusInternalServerError)
		return
	}
	// The row lock above already serialises redeems; this condition keeps the
	// guarantee even if that ever changes: whoever loses consumes nothing, and the
	// account created just above is rolled back.
	res, err := tx.Exec(`UPDATE invitations SET used_at = now(), used_by = $2
		WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL`, invID, userID)
	if err != nil {
		http.Error(w, "Error creating the account", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		http.Error(w, invalidInvitationMessage, http.StatusNotFound)
		return
	}
	if err := audit.Record(r.Context(), tx, userID, "invitation.redeem", "invitation", invID, map[string]any{"role": role}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error creating the account", http.StatusInternalServerError)
		return
	}

	sid, expires, err := h.Sessions.CreateSession(r.Context(), userID, r.UserAgent())
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}
	token, err := middleware.IssueSessionToken(sid, userID, expires)
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AuthResponse{Token: token})
}

// revokeIssuedInvitations cancels the invitations an account handed out that
// nobody has used yet. It runs when the account is blocked or deleted: links the
// account gave out must not outlive its access.
func revokeIssuedInvitations(ctx context.Context, tx *sql.Tx, issuerID, actorID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE invitations SET revoked_at = now(), revoked_by = $2
		WHERE created_by = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, issuerID, actorID)
	return err
}
