package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthMiddleware_DevBypass(t *testing.T) {
	os.Setenv("APP_ENV", "development")
	defer os.Unsetenv("APP_ENV")

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(UserIDKey)
		if userID != DefaultDevUserID {
			t.Errorf("expected dev user ID, got %v", userID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 in dev mode without token, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ProductionRejectsNoToken(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	defer os.Unsetenv("APP_ENV")

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without token in production")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 in production without token, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidJWT(t *testing.T) {
	os.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	defer os.Unsetenv("JWT_SECRET")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-123",
		"role": "admin",
		"exp":  time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte("test_secret_key_for_testing_12345678"))

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(UserIDKey)
		if userID != "user-123" {
			t.Errorf("expected user-123, got %v", userID)
		}
		role := r.Context().Value(UserRoleKey)
		if role != "admin" {
			t.Errorf("expected admin, got %v", role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d", rec.Code)
	}
}

func TestAuthMiddleware_InvalidAlgorithmRejected(t *testing.T) {
	os.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	defer os.Unsetenv("JWT_SECRET")

	// Create token with 'none' algorithm (SEC-10)
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":  "user-123",
		"role": "admin",
		"exp":  time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid algorithm")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with 'none' algorithm, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	os.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	defer os.Unsetenv("JWT_SECRET")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-123",
		"role": "admin",
		"exp":  time.Now().Add(-1 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte("test_secret_key_for_testing_12345678"))

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with expired token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with expired token, got %d", rec.Code)
	}
}

func TestAuthMiddleware_QueryParamToken(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	os.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	defer os.Unsetenv("APP_ENV")
	defer os.Unsetenv("JWT_SECRET")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-123",
		"role": "reader",
		"exp":  time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte("test_secret_key_for_testing_12345678"))

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(UserIDKey)
		if userID != "user-123" {
			t.Errorf("expected user-123 from query param, got %v", userID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/files/test.jpg?token="+tokenString, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with query param token, got %d", rec.Code)
	}
}