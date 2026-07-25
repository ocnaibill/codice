package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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

func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "default_codice_jwt_secret_key_change_me"
	}
	return []byte(secret)
}

// Register creates a new user account with hashed password
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
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
	tokenString, err := token.SignedString(getJWTSecret())
	if err != nil {
		http.Error(w, "Error generating authentication token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{Token: tokenString})
}
