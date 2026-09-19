package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/middleware"
)

func wsHandler(t *testing.T) *WsHandler {
	t.Helper()
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	return &WsHandler{Auth: middleware.Authenticator{
		Sessions: func(ctx context.Context, sid string) (string, string, error) {
			if sid == "live" {
				return "user-1", "reader", nil
			}
			return "", "", middleware.ErrInvalidSession
		},
	}}
}

func TestHandleWS_RejectsConnectionWithoutCredentialsInAnyEnv(t *testing.T) {
	for _, env := range []string{"", "development", "test", "production"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			h := wsHandler(t)
			t.Setenv("APP_ENV", env)
			rec := httptest.NewRecorder()
			h.HandleWS(rec, httptest.NewRequest("GET", "/ws", nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401", rec.Code)
			}
		})
	}
}

func TestHandleWS_RejectsGarbageSessionTokenAndRevokedTicket(t *testing.T) {
	h := wsHandler(t)

	rec := httptest.NewRecorder()
	h.HandleWS(rec, httptest.NewRequest("GET", "/ws?ticket=garbage", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("garbage ticket: got %d, want 401", rec.Code)
	}

	revoked, _, _ := middleware.IssueResourceToken("revoked", "user-1", middleware.ScopeWS, time.Minute)
	rec = httptest.NewRecorder()
	h.HandleWS(rec, httptest.NewRequest("GET", "/ws?ticket="+revoked, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked session ticket: got %d, want 401", rec.Code)
	}
}

func TestHandleWS_SessionTokenInQueryStringIsRefused(t *testing.T) {
	h := wsHandler(t)
	tok, _ := middleware.IssueSessionToken("live", "user-1", time.Now().Add(time.Hour))
	for _, param := range []string{"token", "ticket"} {
		rec := httptest.NewRecorder()
		h.HandleWS(rec, httptest.NewRequest("GET", "/ws?"+param+"="+tok, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("session token as ?%s=: got %d, want 401", param, rec.Code)
		}
	}
}

func TestHandleWS_ValidTicketPassesAuthentication(t *testing.T) {
	h := wsHandler(t)
	ticket, _, _ := middleware.IssueResourceToken("live", "user-1", middleware.ScopeWS, time.Minute)
	rec := httptest.NewRecorder()
	// A plain recorder cannot complete the upgrade, but the request must get
	// past authentication (the upgrader answers 400 for a non-WebSocket call).
	h.HandleWS(rec, httptest.NewRequest("GET", "/ws?ticket="+ticket, nil))
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusInternalServerError {
		t.Errorf("valid ticket rejected: %d", rec.Code)
	}
}
