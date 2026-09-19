package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/identity"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB       *sql.DB
	Sessions *sessions.Store
	// Directory is the LDAP directory, or nil when LDAP is not configured. It never
	// takes part for the owner, whose sign-in is always local (DEC-073).
	Directory ldapauth.Directory
}

type AuthRequest struct {
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// burnPasswordCheck spends the same bcrypt time as a real comparison, so a
// login for an unknown user is not measurably faster than one for a real user.
func burnPasswordCheck(password string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("codice-dummy-password"), bcryptCost)
	})
	bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// bcryptCost is the work factor for new password hashes. 12 costs roughly a
// quarter of a second on current hardware, which is negligible for a person
// logging in and expensive for someone guessing offline; login attempts are
// also rate limited. Older, cheaper hashes are upgraded on the next login.
const bcryptCost = 12

// invalidLoginMessage is the single answer for an unknown user, a wrong
// password, an account without a local password, and a blocked account, so the
// response does not reveal which usernames exist.
const invalidLoginMessage = "Invalid username or password"

// newSessionToken records a session and returns its bearer token.
func (h *AuthHandler) newSessionToken(r *http.Request, userID string) (string, error) {
	sid, expires, err := h.Sessions.CreateSession(r.Context(), userID, r.UserAgent())
	if err != nil {
		return "", err
	}
	return middleware.IssueSessionToken(sid, userID, expires)
}

// Register creates a new user account with hashed password
// Registration can be disabled via ALLOW_REGISTRATION=false or APP_ENV=production without explicit ALLOW_REGISTRATION=true
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	allowRegistration := os.Getenv("ALLOW_REGISTRATION")
	if allowRegistration == "" {
		allowRegistration = "false"
	}
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "production" && allowRegistration != "true" {
		http.Error(w, "Registration is disabled in production", http.StatusForbidden)
		return
	}
	if allowRegistration == "false" {
		http.Error(w, "Registration is disabled", http.StatusForbidden)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	email := req.Email
	if email == "" {
		email = req.Username + "@codice.local"
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		http.Error(w, "Error processing password hash", http.StatusInternalServerError)
		return
	}

	_, err = h.DB.Exec(
		"INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, 'reader')",
		req.Username, email, string(hashedPassword),
	)

	if err != nil {
		http.Error(w, "Error creating user. Username or email already exists.", http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Login authenticates a person from the single login form. The backend picks the
// method from the ACCOUNT, deterministically (DEC-072): a local account checks its
// local password; an account linked to the directory checks the directory; an
// unknown name reaches the directory only if the owner allowed first-login
// creation. There is no cascade: a wrong local password is not sent to the
// directory except when the directory has an entry of that very name (DEC-075),
// and the owner never leaves local sign-in. Every credential failure has the same
// public answer; an unreachable directory is the one distinct, operational answer.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	var id, passwordHash, role string
	var blockedAt sql.NullTime
	err := h.DB.QueryRow(
		"SELECT id, COALESCE(password_hash, ''), blocked_at, COALESCE(role, 'reader') FROM users WHERE username = $1", req.Username,
	).Scan(&id, &passwordHash, &blockedAt, &role)
	if errors.Is(err, sql.ErrNoRows) {
		h.loginUnknown(w, r, req)
		return
	}
	if err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}

	// An account tied to the directory checks its password there, never locally.
	if role != authz.RoleOwner {
		if subject, linked, err := identity.SubjectOf(r.Context(), h.DB, id); err != nil {
			http.Error(w, "Error querying database", http.StatusInternalServerError)
			return
		} else if linked {
			h.loginLinked(w, r, id, subject, blockedAt.Valid, req.Password)
			return
		}
	}

	if passwordHash != "" && bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) == nil && !blockedAt.Valid {
		h.upgradePasswordHash(id, passwordHash, req.Password)
		h.finishLogin(w, r, id)
		return
	}
	if passwordHash == "" {
		burnPasswordCheck(req.Password)
	}

	// The local password did not match. If the directory has someone with this very
	// name, the typed password may be theirs, and proving both is how the two are
	// joined (DEC-075). The directory is asked about the name first, without any password.
	if h.Directory != nil && role != authz.RoleOwner && !blockedAt.Valid && passwordHash != "" {
		if h.offerLink(w, r, id, req) {
			return
		}
	}
	http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
}

// finishLogin opens a session and answers with its token.
func (h *AuthHandler) finishLogin(w http.ResponseWriter, r *http.Request, userID string) {
	tokenString, err := h.newSessionToken(r, userID)
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{Token: tokenString})
}

const directoryUnavailableMessage = "The sign-in directory is not answering. Try again shortly."

func unavailable(w http.ResponseWriter) {
	http.Error(w, directoryUnavailableMessage, http.StatusServiceUnavailable)
}

// loginLinked signs in an account that is tied to the directory. The entry is
// found by its stable identifier, so a rename in the directory changes nothing.
func (h *AuthHandler) loginLinked(w http.ResponseWriter, r *http.Request, userID, subject string, blocked bool, password string) {
	if blocked {
		// Blocking is immediate and local: the directory is not even asked.
		burnPasswordCheck(password)
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	}
	if h.Directory == nil {
		unavailable(w) // linked, but the directory is not configured: operational, not a wrong password
		return
	}
	entry, err := h.Directory.LookupBySubject(r.Context(), subject)
	switch {
	case errors.Is(err, ldapauth.ErrNotFound):
		burnPasswordCheck(password)
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized) // removed or disabled in the directory
		return
	case err != nil:
		unavailable(w)
		return
	}
	switch err := h.Directory.Authenticate(r.Context(), entry, password); {
	case errors.Is(err, ldapauth.ErrInvalidCredentials):
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	case err != nil:
		unavailable(w)
		return
	}
	identity.Touch(r.Context(), h.DB, subject)
	h.finishLogin(w, r, userID)
}

// offerLink handles a local account whose typed password did not match. It reports
// whether it answered. It answers with a link ticket only when the directory has an
// entry with this name, the typed password is that entry's, and the entry is not
// already tied to another account. No session is opened yet.
func (h *AuthHandler) offerLink(w http.ResponseWriter, r *http.Request, userID string, req AuthRequest) bool {
	entry, err := h.Directory.Lookup(r.Context(), req.Username)
	if err != nil {
		return false
	}
	if l, _ := identity.FindLinked(r.Context(), h.DB, entry.Subject); l != nil {
		return false
	}
	if h.Directory.Authenticate(r.Context(), entry, req.Password) != nil {
		return false
	}
	h.sendTicket(w, r, userID, req.Username, entry.Subject)
	return true
}

func (h *AuthHandler) sendTicket(w http.ResponseWriter, r *http.Request, userID, username, subject string) {
	ticket, err := identity.NewTicket(r.Context(), h.DB, userID, subject)
	if err != nil {
		http.Error(w, "Error starting the link", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"linkRequired": true, "ticket": ticket, "username": username})
}

// loginUnknown handles a name with no account. It reaches the directory only when
// the owner allowed creating accounts at first sign-in (DEC-051); otherwise it is
// an ordinary failed login.
func (h *AuthHandler) loginUnknown(w http.ResponseWriter, r *http.Request, req AuthRequest) {
	deny := func() {
		burnPasswordCheck(req.Password)
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
	}
	if h.Directory == nil {
		deny()
		return
	}
	if policy, err := identity.GetPolicy(r.Context(), h.DB); err != nil || !policy.AllowCreate {
		deny()
		return
	}
	entry, err := h.Directory.Lookup(r.Context(), req.Username)
	switch {
	case errors.Is(err, ldapauth.ErrNotFound):
		deny()
		return
	case err != nil:
		unavailable(w)
		return
	}
	switch err := h.Directory.Authenticate(r.Context(), entry, req.Password); {
	case errors.Is(err, ldapauth.ErrInvalidCredentials):
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	case err != nil:
		unavailable(w)
		return
	}

	// Already tied to an account (renamed in the directory): that account signs in.
	if l, _ := identity.FindLinked(r.Context(), h.DB, entry.Subject); l != nil {
		if l.Blocked || l.Role == authz.RoleOwner {
			http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
			return
		}
		identity.Touch(r.Context(), h.DB, entry.Subject)
		h.finishLogin(w, r, l.UserID)
		return
	}
	// The same e-mail as a local account is a reason to ask, not a proof: the person
	// must also give the local password before anything is joined (DEC-053).
	if uid, ok, _ := identity.LocalCandidateByEmail(r.Context(), h.DB, entry.Email); ok {
		h.sendTicket(w, r, uid, req.Username, entry.Subject)
		return
	}

	name := entry.Username
	if name == "" || len(name) > 50 {
		name = req.Username
	}
	id, err := identity.CreateAccount(r.Context(), h.DB, name, entry)
	if errors.Is(err, identity.ErrExists) || errors.Is(err, identity.ErrNameTaken) {
		// Either a concurrent first login for this same person just created the
		// account (the unique name or identity stopped this one), or the name belongs
		// to someone else. Only the first is a sign-in: the entry is tied to that account.
		if l, _ := identity.FindLinked(r.Context(), h.DB, entry.Subject); l != nil && !l.Blocked && l.Role != authz.RoleOwner {
			identity.Touch(r.Context(), h.DB, entry.Subject)
			h.finishLogin(w, r, l.UserID)
			return
		}
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Println("Error creating the directory account:", err)
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	}
	h.finishLogin(w, r, id)
}

type linkRequest struct {
	Ticket   string `json:"ticket"`
	Password string `json:"password"`
}

// Link completes joining a directory identity to a local account: the person has
// proven the directory password (that is what the ticket says) and now proves the
// local one. Any failure gets the same answer as a wrong password at login.
func (h *AuthHandler) Link(w http.ResponseWriter, r *http.Request) {
	var req linkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	userID, err := identity.LinkWithTicket(r.Context(), h.DB, req.Ticket, req.Password)
	if err != nil {
		if !errors.Is(err, identity.ErrTicketInvalid) && !errors.Is(err, identity.ErrBadPassword) &&
			!errors.Is(err, identity.ErrNotEligible) && !errors.Is(err, identity.ErrExists) {
			log.Println("Error linking the identity:", err)
			http.Error(w, "Error linking the account", http.StatusInternalServerError)
			return
		}
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	}
	h.finishLogin(w, r, userID)
}

// upgradePasswordHash re-hashes a correct password at the current cost when the
// stored hash is cheaper. It is best effort: a failure never blocks the login,
// and the compare-and-set keeps it from overwriting a password changed meanwhile.
func (h *AuthHandler) upgradePasswordHash(userID, oldHash, password string) {
	if cost, err := bcrypt.Cost([]byte(oldHash)); err != nil || cost >= bcryptCost {
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return
	}
	h.DB.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2 AND password_hash = $3`,
		string(newHash), userID, oldHash)
}

// Logout revokes the current session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sid, _ := r.Context().Value(middleware.SessionIDKey).(string)
	if err := h.Sessions.RevokeSession(r.Context(), sid); err != nil {
		http.Error(w, "Error ending session", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type resourceTokenRequest struct {
	Scope string `json:"scope"`
}

type resourceTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Lifetimes of resource tokens: long enough for a reading session's images and
// downloads while the page refreshes them, short enough to limit exposure of a
// URL that leaks into a log or history.
const (
	assetTokenTTL = 15 * time.Minute
	wsTicketTTL   = 60 * time.Second
)

// ResourceToken issues a short-lived token tied to the caller's session, for
// URLs the browser loads without an Authorization header (?rt=) and for the
// WebSocket handshake (?ticket=).
func (h *AuthHandler) ResourceToken(w http.ResponseWriter, r *http.Request) {
	var req resourceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	ttl := map[string]time.Duration{middleware.ScopeAssets: assetTokenTTL, middleware.ScopeWS: wsTicketTTL}[req.Scope]
	if ttl == 0 {
		http.Error(w, "scope must be 'assets' or 'ws'", http.StatusBadRequest)
		return
	}

	sid, _ := r.Context().Value(middleware.SessionIDKey).(string)
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	token, expires, err := middleware.IssueResourceToken(sid, userID, req.Scope, ttl)
	if err != nil {
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resourceTokenResponse{Token: token, ExpiresAt: expires})
}

// Me tells the client who is signed in and with which role, so it can show only
// what that role may use. The role is the one in the database right now.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	var username string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		http.Error(w, "Error reading the account", http.StatusInternalServerError)
		return
	}
	// Things the person must see now, such as a recovery done on the server.
	type notice struct {
		ID        int64           `json:"id"`
		Kind      string          `json:"kind"`
		Details   json.RawMessage `json:"details"`
		CreatedAt time.Time       `json:"createdAt"`
	}
	notices := []notice{}
	rows, err := h.DB.QueryContext(r.Context(), `SELECT id, kind, details, created_at FROM security_notices
		WHERE user_id = $1 AND acknowledged_at IS NULL ORDER BY id`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var n notice
			if rows.Scan(&n.ID, &n.Kind, &n.Details, &n.CreatedAt) == nil {
				notices = append(notices, n)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": userID, "username": username, "role": role, "notices": notices})
}

// SetupStatusResponse indicates whether the system needs first-run wizard initialization
type SetupStatusResponse struct {
	IsFirstRun bool `json:"isFirstRun"`
}

// GetSetupStatus checks if any users exist in the database
func (h *AuthHandler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	var count int
	err := h.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		http.Error(w, "Error querying database setup status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SetupStatusResponse{IsFirstRun: count == 0})
}

// setupLockKey serialises first-run setup across concurrent requests
// (pg_advisory_xact_lock). The value is arbitrary but must stay constant.
const setupLockKey = 7301122

// SetupMasterAdmin creates the initial owner during first-run setup. The
// emptiness check and the insert run in one transaction behind an advisory
// lock, so concurrent requests cannot create more than one account (RF-001);
// the unique owner index in the database is the second line of defence.
func (h *AuthHandler) SetupMasterAdmin(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required for master setup", http.StatusBadRequest)
		return
	}

	email := req.Email
	if email == "" {
		email = req.Username + "@codice.local"
	}

	// Hash before taking the lock: bcrypt is slow and must not hold it.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		http.Error(w, "Error processing password hash", http.StatusInternalServerError)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error checking database state", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, setupLockKey); err != nil {
		http.Error(w, "Error checking database state", http.StatusInternalServerError)
		return
	}

	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		http.Error(w, "Error checking database state", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "First-time setup has already been completed", http.StatusForbidden)
		return
	}

	var id string
	query := `INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, 'owner') RETURNING id`
	if err := tx.QueryRow(query, req.Username, email, string(hashedPassword)).Scan(&id); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			http.Error(w, "First-time setup has already been completed", http.StatusForbidden)
			return
		}
		http.Error(w, "Error creating master admin account", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error creating master admin account", http.StatusInternalServerError)
		return
	}

	tokenString, err := h.newSessionToken(r, id)
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AuthResponse{Token: tokenString})
}

type changePasswordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// ChangePassword lets a person change their own password. It needs the current
// one (a stolen session alone must not be enough), and it ends every OTHER
// session of the account, so a change made because someone else got in actually
// removes them. The session used stays, and app tokens are separate credentials
// that keep working.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	sid, _ := r.Context().Value(middleware.SessionIDKey).(string)
	if sid == "" {
		http.Error(w, "Forbidden: sign in to change the password", http.StatusForbidden)
		return
	}
	if len(req.New) < minPasswordLength {
		http.Error(w, "The new password must have at least 8 characters", http.StatusBadRequest)
		return
	}

	var hash string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(password_hash, '') FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}
	if hash == "" {
		http.Error(w, "This account has no local password", http.StatusBadRequest)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Current)) != nil {
		http.Error(w, "The current password is not correct", http.StatusForbidden)
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcryptCost)
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
	if _, err := tx.Exec(`UPDATE users SET password_hash = $2 WHERE id = $1`, userID, string(newHash)); err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	res, err := tx.Exec(`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL`, userID, sid)
	if err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	ended, _ := res.RowsAffected()
	if err := audit.Record(r.Context(), tx, userID, "user.password_change", "user", userID, map[string]any{"sessionsEnded": ended}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error changing the password", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
