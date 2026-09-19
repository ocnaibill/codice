package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/middleware"
)

// opdsHandler accepts exactly one app token, like the store would.
func opdsHandler() *OPDSHandler {
	return &OPDSHandler{Auth: middleware.Authenticator{
		Basic: func(ctx context.Context, u, p string) (string, string, error) {
			if u == "ana" && p == "cdc_secret" {
				return "user-1", "reader", nil
			}
			return "", "", middleware.ErrInvalidCredentials
		},
	}}
}

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func serveOPDS(h *OPDSHandler, header string) (*httptest.ResponseRecorder, bool) {
	ran := false
	handler := h.OpdsAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true }))
	req := httptest.NewRequest("GET", "/opds/v1.2/catalog", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, ran
}

func TestOpdsAuth_OnlyTheAppTokenIsAccepted(t *testing.T) {
	cases := map[string]string{
		"wrong token":            basic("ana", "cdc_other"),
		"account password":       basic("ana", "hunter2"),
		"existing user, no pass": basic("ana", ""),
		"unknown user":           basic("bob", "cdc_secret"),
	}
	for name, header := range cases {
		rec, ran := serveOPDS(opdsHandler(), header)
		if rec.Code != http.StatusUnauthorized || ran {
			t.Errorf("%s: code=%d ran=%v, want 401", name, rec.Code, ran)
		}
	}

	rec, ran := serveOPDS(opdsHandler(), basic("ana", "cdc_secret"))
	if rec.Code != http.StatusOK || !ran {
		t.Errorf("valid app token: code=%d ran=%v", rec.Code, ran)
	}
}

func TestOpdsAuth_MissingHeaderChallenges(t *testing.T) {
	rec, ran := serveOPDS(opdsHandler(), "")
	if rec.Code != http.StatusUnauthorized || ran {
		t.Errorf("code=%d ran=%v, want 401", rec.Code, ran)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected a Basic challenge so OPDS clients prompt for credentials")
	}
}

func TestOpdsAuth_WithoutVerifierDeniesEverything(t *testing.T) {
	rec, ran := serveOPDS(&OPDSHandler{}, basic("ana", "cdc_secret"))
	if rec.Code != http.StatusUnauthorized || ran {
		t.Errorf("code=%d ran=%v, want 401", rec.Code, ran)
	}
}
