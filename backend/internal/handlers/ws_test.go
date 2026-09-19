package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleWS_RejectsConnectionWithoutTokenInAnyEnv(t *testing.T) {
	for _, env := range []string{"", "development", "test", "production"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")

			h := &WsHandler{}
			req := httptest.NewRequest("GET", "/ws", nil)
			rec := httptest.NewRecorder()
			h.HandleWS(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without a token, got %d", rec.Code)
			}
		})
	}
}

func TestHandleWS_RejectsInvalidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")

	h := &WsHandler{}
	req := httptest.NewRequest("GET", "/ws?token=garbage", nil)
	rec := httptest.NewRecorder()
	h.HandleWS(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for an invalid token, got %d", rec.Code)
	}
}
