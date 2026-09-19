package middleware

import (
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/authz"
)

// RequireStaff allows only owner and admin. It must run after AuthMiddleware:
// a request with no identity is answered 401, an authenticated reader 403.
func RequireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := r.Context().Value(UserIDKey).(string)
		if id == "" {
			http.Error(w, "Access denied: Authentication required", http.StatusUnauthorized)
			return
		}
		role, _ := r.Context().Value(UserRoleKey).(string)
		if !authz.IsStaff(role) {
			http.Error(w, "Forbidden: administrator role required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireOwner allows only the owner (instance-level actions such as changing
// admin roles). Same 401/403 split as RequireStaff.
func RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := r.Context().Value(UserIDKey).(string)
		if id == "" {
			http.Error(w, "Access denied: Authentication required", http.StatusUnauthorized)
			return
		}
		if role, _ := r.Context().Value(UserRoleKey).(string); role != authz.RoleOwner {
			http.Error(w, "Forbidden: owner role required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
