package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
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

// hrefs are the links of a catalog. A client follows each one as written.
func catalogHrefs(t *testing.T, h *OPDSHandler, remote, host, proto string) []string {
	t.Helper()
	req := httptest.NewRequest("GET", "/opds/v1.2/catalog", nil)
	req.RemoteAddr = remote
	req.Host = host
	if proto != "" {
		req.Header.Set("X-Forwarded-Proto", proto)
	}
	rec := httptest.NewRecorder()
	h.RootCatalog(rec, req)
	var out []string
	for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(rec.Body.String(), -1) {
		out = append(out, m[1])
	}
	if len(out) < 3 {
		t.Fatalf("a catalog with fewer than 3 links: %v", out)
	}
	return out
}

func TestOPDSLinks_BehindAProxyThatEndedTheTLS(t *testing.T) {
	proxy := []netip.Prefix{netip.MustParsePrefix("172.30.77.10/32")}
	cases := []struct {
		name   string
		h      *OPDSHandler
		remote string
		proto  string
		want   string
	}{
		{"the trusted proxy says https", &OPDSHandler{TrustedProxies: proxy}, "172.30.77.10:4000", "https", "https://livros.example.com"},
		{"the trusted proxy says nothing", &OPDSHandler{TrustedProxies: proxy}, "172.30.77.10:4000", "", "http://livros.example.com"},
		{"nobody is trusted", &OPDSHandler{}, "172.30.77.10:4000", "https", "http://livros.example.com"},
		{"a stranger cannot choose the scheme", &OPDSHandler{TrustedProxies: proxy}, "203.0.113.9:4000", "https", "http://livros.example.com"},
		{"the configured address wins over the request", &OPDSHandler{PublicURL: "https://catalogo.example.org", TrustedProxies: proxy}, "172.30.77.10:4000", "http", "https://catalogo.example.org"},
		{"the configured address wins with no proxy at all", &OPDSHandler{PublicURL: "https://catalogo.example.org:8443"}, "203.0.113.9:4000", "", "https://catalogo.example.org:8443"},
	}
	for _, c := range cases {
		for _, href := range catalogHrefs(t, c.h, c.remote, "livros.example.com", c.proto) {
			if !strings.HasPrefix(href, c.want+"/") {
				t.Errorf("%s: link %q does not start with %s/", c.name, href, c.want)
			}
		}
	}
}
