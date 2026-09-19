package middleware

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthMiddleware_RejectsRequestsWithoutCredentialsInAnyEnv(t *testing.T) {
	for _, env := range []string{"", "development", "test", "staging", "production"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)

			handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("handler must not run without credentials")
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without credentials, got %d", rec.Code)
			}
		})
	}
}

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestAuthMiddleware_RejectsAnyBasicHeader(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run for a Basic header on the common middleware")
	}))

	req := httptest.NewRequest("GET", "/works", nil)
	req.Header.Set("Authorization", basicHeader("anyone", "anything"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for Basic on common middleware, got %d", rec.Code)
	}
}

func fakeVerifier(user, pass, id, role string) BasicVerifier {
	return func(ctx context.Context, u, p string) (string, string, error) {
		if u == user && p == pass {
			return id, role, nil
		}
		return "", "", ErrInvalidCredentials
	}
}

func TestAuthMiddlewareWithBasic_AcceptsValidCredentials(t *testing.T) {
	handler := AuthMiddlewareWithBasic(fakeVerifier("ana", "s3cret", "user-1", "reader"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Context().Value(UserIDKey); got != "user-1" {
				t.Errorf("expected user-1 in context, got %v", got)
			}
			if got := r.Context().Value(UserRoleKey); got != "reader" {
				t.Errorf("expected reader in context, got %v", got)
			}
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest("GET", "/files/book.epub", nil)
	req.Header.Set("Authorization", basicHeader("ana", "s3cret"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with valid Basic credentials, got %d", rec.Code)
	}
}

func TestAuthMiddlewareWithBasic_RejectsBadCredentials(t *testing.T) {
	cases := map[string]string{
		"wrong password":     basicHeader("ana", "wrong"),
		"unknown user":       basicHeader("bob", "s3cret"),
		"empty password":     basicHeader("ana", ""),
		"no colon":           "Basic " + base64.StdEncoding.EncodeToString([]byte("ana")),
		"invalid base64":     "Basic !!!not-base64!!!",
		"empty basic":        "Basic ",
		"unsupported scheme": "Digest abc",
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			handler := AuthMiddlewareWithBasic(fakeVerifier("ana", "s3cret", "user-1", "reader"))(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("handler must not run with bad credentials")
				}))

			req := httptest.NewRequest("GET", "/files/book.epub", nil)
			req.Header.Set("Authorization", header)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("expected a WWW-Authenticate challenge so Basic clients can prompt again")
			}
		})
	}
}

func TestAuthMiddlewareWithBasic_RejectsMissingCredentials(t *testing.T) {
	handler := AuthMiddlewareWithBasic(fakeVerifier("ana", "s3cret", "user-1", "reader"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler must not run without credentials")
		}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/files/book.epub", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected a WWW-Authenticate challenge")
	}
}

func TestAuthMiddlewareWithBasic_NilVerifierDeniesBasic(t *testing.T) {
	handler := AuthMiddlewareWithBasic(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run when no verifier is configured")
	}))

	req := httptest.NewRequest("GET", "/files/book.epub", nil)
	req.Header.Set("Authorization", basicHeader("ana", "s3cret"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareWithBasic_VerifierFailureIsNotAuthentication(t *testing.T) {
	verifier := func(ctx context.Context, u, p string) (string, string, error) {
		return "", "", errors.New("database unavailable")
	}
	handler := AuthMiddlewareWithBasic(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run when verification fails")
	}))

	req := httptest.NewRequest("GET", "/files/book.epub", nil)
	req.Header.Set("Authorization", basicHeader("ana", "s3cret"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when the verifier itself fails, got %d", rec.Code)
	}
}

func TestAuthMiddlewareWithBasic_BearerStillWorks(t *testing.T) {
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-9",
		"role": "reader",
		"exp":  time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte("test_secret_key_for_testing_12345678"))

	handler := AuthMiddlewareWithBasic(fakeVerifier("ana", "s3cret", "user-1", "reader"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Context().Value(UserIDKey); got != "user-9" {
				t.Errorf("expected user-9, got %v", got)
			}
		}))

	req := httptest.NewRequest("GET", "/files/book.epub", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with Bearer token, got %d", rec.Code)
	}
}

func TestGetJWTSecret_MissingSecretIsFatalInAnyEnv(t *testing.T) {
	for _, env := range []string{"", "development", "production"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			t.Setenv("JWT_SECRET", "")

			defer func() {
				if recover() == nil {
					t.Error("expected GetJWTSecret to panic when JWT_SECRET is unset, no built-in default allowed")
				}
			}()
			GetJWTSecret()
		})
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