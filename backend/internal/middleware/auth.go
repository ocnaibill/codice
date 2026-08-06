package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

func GetJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		if os.Getenv("APP_ENV") == "production" {
			panic("JWT_SECRET must be set in production")
		}
		secret = "default_codice_jwt_secret_key_change_me"
	}
	return []byte(secret)
}

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	UserRoleKey  contextKey = "user_role"

	// Default fallback admin UUID used when no authorization token header is passed (for dev testing)
	DefaultDevUserID = "b190281f-fe3e-4308-ad71-91e24744d7a0"
)

// AuthMiddleware validates JWT bearer tokens and injects user_id and user_role into request context
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// Support token via query param for asset URLs (e.g., <img src="/files/...?token=xxx">)
		if authHeader == "" {
			authHeader = r.URL.Query().Get("token")
			if authHeader != "" {
				authHeader = "Bearer " + authHeader
			}
		}

		if authHeader == "" {
			appEnv := os.Getenv("APP_ENV")
			if appEnv == "production" {
				http.Error(w, "Access denied: Authentication required", http.StatusUnauthorized)
				return
			}
			// Development fallback: populate context with default admin user ID
			ctx := context.WithValue(r.Context(), UserIDKey, DefaultDevUserID)
			ctx = context.WithValue(ctx, UserRoleKey, "admin")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Support Basic Auth (for OPDS acquisition downloads & covers)
		if strings.HasPrefix(authHeader, "Basic ") {
			ctx := context.WithValue(r.Context(), UserIDKey, DefaultDevUserID)
			ctx = context.WithValue(ctx, UserRoleKey, "reader")
			next.ServeHTTP(w, r.WithContext(ctx))
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
			http.Error(w, "Access denied: Invalid or expired token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "Error reading token claims", http.StatusInternalServerError)
			return
		}

		userID, _ := claims["sub"].(string)
		role, _ := claims["role"].(string)

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, UserRoleKey, role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}