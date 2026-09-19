package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

type AuthHandler struct {
	DB *sql.DB
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

// NewBasicVerifier returns a BasicVerifier that checks the account password
// against its bcrypt hash. An unknown user costs the same bcrypt comparison as
// a wrong password, so response time does not reveal which usernames exist.
func NewBasicVerifier(db *sql.DB) middleware.BasicVerifier {
	return func(ctx context.Context, username, password string) (string, string, error) {
		dummyHashOnce.Do(func() {
			dummyHash, _ = bcrypt.GenerateFromPassword([]byte("codice-dummy-password"), 10)
		})

		var id, role, hash string
		err := db.QueryRowContext(ctx,
			"SELECT id, role, COALESCE(password_hash, '') FROM users WHERE username = $1", username,
		).Scan(&id, &role, &hash)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && hash == "") {
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return "", "", middleware.ErrInvalidCredentials
		}
		if err != nil {
			return "", "", err
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			return "", "", middleware.ErrInvalidCredentials
		}
		return id, role, nil
	}
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

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
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

// Login authenticates user credentials and returns a signed JWT token
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	var id, role, passwordHash string
	err := h.DB.QueryRow("SELECT id, role, COALESCE(password_hash, '') FROM users WHERE username = $1", req.Username).Scan(&id, &role, &passwordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Error querying database", http.StatusInternalServerError)
		return
	}

	if passwordHash == "" {
		http.Error(w, "User has no password set (SSO login required)", http.StatusUnauthorized)
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		http.Error(w, "Incorrect password", http.StatusUnauthorized)
		return
	}

	expirationTime := time.Now().Add(7 * 24 * time.Hour)
	claims := jwt.MapClaims{
		"sub":  id,
		"role": role,
		"exp":  expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(middleware.GetJWTSecret())
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{Token: tokenString})
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
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
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

	expirationTime := time.Now().Add(7 * 24 * time.Hour)
	claims := jwt.MapClaims{
		"sub":  id,
		"role": "owner",
		"exp":  expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(middleware.GetJWTSecret())
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AuthResponse{Token: tokenString})
}
