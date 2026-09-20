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

func TestUpgrader_AllowsTheSameOriginAndStillChecksOthersAgainstTheList(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://codice.example.com")
	check := func(host, origin string) bool {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return upgrader.CheckOrigin(r)
	}
	cases := []struct {
		name         string
		host, origin string
		want         bool
	}{
		{"the listed origin", "codice.internal", "https://codice.example.com", true},
		{"same origin, an address on the local network", "192.168.1.20:8080", "http://192.168.1.20:8080", true},
		{"same origin, localhost", "localhost:8080", "http://localhost:8080", true},
		{"same host, different case", "Codice.Home:8080", "http://codice.home:8080", true},
		{"another site", "codice.internal", "https://evil.example", false},
		{"same name, other port is another origin", "localhost:8080", "http://localhost:9999", false},
		{"no Origin at all (a command-line client)", "localhost:8080", "", true},
	}
	for _, c := range cases {
		if got := check(c.host, c.origin); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
