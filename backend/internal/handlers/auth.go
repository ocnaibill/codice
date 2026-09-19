package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
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

// Login authenticates user credentials, records a session and returns its token.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	var id, passwordHash string
	var blockedAt sql.NullTime
	err := h.DB.QueryRow(
		"SELECT id, COALESCE(password_hash, ''), blocked_at FROM users WHERE username = $1", req.Username,
	).Scan(&id, &passwordHash, &blockedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}

	if errors.Is(err, sql.ErrNoRows) || passwordHash == "" {
		burnPasswordCheck(req.Password)
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil || blockedAt.Valid {
		http.Error(w, invalidLoginMessage, http.StatusUnauthorized)
		return
	}

	h.upgradePasswordHash(id, passwordHash, req.Password)

	tokenString, err := h.newSessionToken(r, id)
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{Token: tokenString})
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": userID, "username": username, "role": role})
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
