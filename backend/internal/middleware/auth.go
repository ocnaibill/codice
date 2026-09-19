package middleware

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// GetJWTSecret returns the signing secret. There is no built-in default in any
// environment: a public fallback key would let anyone forge tokens. It panics
// when JWT_SECRET is unset, so call it once at startup to fail fast.
func GetJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET must be set")
	}
	return []byte(secret)
}

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	UserRoleKey contextKey = "user_role"
)

// ErrInvalidCredentials is returned by a BasicVerifier when the username or
// password does not match. Any other error means verification itself failed.
var ErrInvalidCredentials = errors.New("invalid credentials")

// BasicVerifier checks a username/password pair and returns the account's id
// and role. It must return ErrInvalidCredentials for a wrong username or
// password.
type BasicVerifier func(ctx context.Context, username, password string) (id, role string, err error)

// AuthMiddleware validates JWT bearer tokens and injects user_id and user_role
// into the request context. Requests without a valid token are always rejected,
// in every APP_ENV. HTTP Basic is not accepted here; see AuthMiddlewareWithBasic.
func AuthMiddleware(next http.Handler) http.Handler {
	return AuthMiddlewareWithBasic(nil)(next)
}

// AuthMiddlewareWithBasic behaves like AuthMiddleware and additionally accepts
// HTTP Basic credentials confirmed by verify, for clients that cannot send a
// bearer token (OPDS readers downloading files and covers). With a nil verify,
// Basic is refused.
func AuthMiddlewareWithBasic(verify BasicVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			// Support token via query param for asset URLs (e.g., <img src="/files/...?token=xxx">)
			if authHeader == "" {
				if q := r.URL.Query().Get("token"); q != "" {
					authHeader = "Bearer " + q
				}
			}

			if authHeader == "" {
				deny(w, verify != nil, "Access denied: Authentication required")
				return
			}

			if strings.HasPrefix(authHeader, "Basic ") {
				if verify == nil {
					deny(w, false, "Access denied: Basic authentication is not accepted here")
					return
				}
				id, role, err := verifyBasic(r.Context(), authHeader, verify)
				if errors.Is(err, ErrInvalidCredentials) {
					deny(w, true, "Access denied: Invalid credentials")
					return
				}
				if err != nil {
					http.Error(w, "Authentication unavailable", http.StatusInternalServerError)
					return
				}
				next.ServeHTTP(w, withIdentity(r, id, role))
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				deny(w, verify != nil, "Access denied: Unsupported authorization method")
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				// SEC-10: Validate algorithm
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return GetJWTSecret(), nil
			})

			if err != nil || !token.Valid {
				deny(w, verify != nil, "Access denied: Invalid or expired token")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "Error reading token claims", http.StatusInternalServerError)
				return
			}

			userID, _ := claims["sub"].(string)
			role, _ := claims["role"].(string)

			next.ServeHTTP(w, withIdentity(r, userID, role))
		})
	}
}

func verifyBasic(ctx context.Context, authHeader string, verify BasicVerifier) (id, role string, err error) {
	payload, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
	if decodeErr != nil {
		return "", "", ErrInvalidCredentials
	}
	user, pass, found := strings.Cut(string(payload), ":")
	if !found || user == "" || pass == "" {
		return "", "", ErrInvalidCredentials
	}
	return verify(ctx, user, pass)
}

func withIdentity(r *http.Request, id, role string) *http.Request {
	ctx := context.WithValue(r.Context(), UserIDKey, id)
	ctx = context.WithValue(ctx, UserRoleKey, role)
	return r.WithContext(ctx)
}

// deny writes a 401. When challenge is true it also asks Basic clients to
// prompt for credentials again.
func deny(w http.ResponseWriter, challenge bool, msg string) {
	if challenge {
		w.Header().Set("WWW-Authenticate", `Basic realm="Codice"`)
	}
	http.Error(w, msg, http.StatusUnauthorized)
}
