package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/middleware"
)

func opdsVerifier(user, pass string) middleware.BasicVerifier {
	return func(ctx context.Context, u, p string) (string, string, error) {
		if u == user && p == pass {
			return "user-1", "reader", nil
		}
		return "", "", middleware.ErrInvalidCredentials
	}
}

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestOpdsAuth_WrongPasswordIsRejected(t *testing.T) {
	h := &OPDSHandler{Verify: opdsVerifier("ana", "s3cret")}
	handler := h.OpdsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("catalog must not be served with a wrong password")
	}))

	req := httptest.NewRequest("GET", "/opds/v1.2/catalog", nil)
	req.Header.Set("Authorization", basic("ana", "not-the-password"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong password, got %d", rec.Code)
	}
}

func TestOpdsAuth_ExistingUserWithoutPasswordCheckIsRejected(t *testing.T) {
	// Regression: the old code only checked that the username existed.
	h := &OPDSHandler{Verify: opdsVerifier("ana", "s3cret")}
	handler := h.OpdsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("catalog must not be served on username alone")
	}))

	req := httptest.NewRequest("GET", "/opds/v1.2/catalog", nil)
	req.Header.Set("Authorization", basic("ana", ""))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestOpdsAuth_ValidCredentialsAreAccepted(t *testing.T) {
	h := &OPDSHandler{Verify: opdsVerifier("ana", "s3cret")}
	called := false
	handler := h.OpdsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := r.Context().Value(middleware.UserIDKey); got != "user-1" {
			t.Errorf("expected authenticated user in context, got %v", got)
		}
	}))

	req := httptest.NewRequest("GET", "/opds/v1.2/catalog", nil)
	req.Header.Set("Authorization", basic("ana", "s3cret"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called || rec.Code != http.StatusOK {
		t.Errorf("expected handler to run with 200, called=%v code=%d", called, rec.Code)
	}
}

func TestOpdsAuth_MissingHeaderChallenges(t *testing.T) {
	h := &OPDSHandler{Verify: opdsVerifier("ana", "s3cret")}
	handler := h.OpdsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run without credentials")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/opds/v1.2/catalog", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected a Basic challenge so OPDS clients prompt for credentials")
	}
}

func TestOpdsAuth_WithoutVerifierDeniesEverything(t *testing.T) {
	h := &OPDSHandler{}
	handler := h.OpdsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run when no verifier is configured")
	}))

	req := httptest.NewRequest("GET", "/opds/v1.2/catalog", nil)
	req.Header.Set("Authorization", basic("ana", "s3cret"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
