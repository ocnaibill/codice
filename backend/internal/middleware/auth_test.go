package middleware

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test_secret_key_for_testing_12345678"

// liveSessions is a fake session table: sid -> (user, role). Removing a key
// models revocation or blocking.
type liveSessions map[string][2]string

func (l liveSessions) check(ctx context.Context, sid string) (string, string, error) {
	if v, ok := l[sid]; ok {
		return v[0], v[1], nil
	}
	return "", "", ErrInvalidSession
}

func newAuth(t *testing.T, sessions liveSessions) Authenticator {
	t.Helper()
	t.Setenv("JWT_SECRET", testSecret)
	return Authenticator{
		Sessions: sessions.check,
		Basic: func(ctx context.Context, u, p string) (string, string, error) {
			if u == "ana" && p == "cdc_valid" {
				return "user-ana", "reader", nil
			}
			return "", "", ErrInvalidCredentials
		},
	}
}

func bearer(t *testing.T, sid, uid string) string {
	t.Helper()
	tok, err := IssueSessionToken(sid, uid, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + tok
}

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// run executes mw around a recording handler.
func run(mw func(http.Handler) http.Handler, method, target, authHeader string) (code int, seen *http.Request, rec *httptest.ResponseRecorder) {
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = r }))
	req := httptest.NewRequest(method, target, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, seen, rec
}

func TestMiddleware_RejectsRequestsWithoutCredentialsInAnyEnv(t *testing.T) {
	a := newAuth(t, liveSessions{})
	for _, env := range []string{"", "development", "test", "staging", "production"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			if code, seen, _ := run(a.Middleware, "GET", "/works", ""); code != 401 || seen != nil {
				t.Errorf("code=%d ran=%v, want 401 and no handler", code, seen != nil)
			}
		})
	}
}

func TestMiddleware_ValidSessionInjectsIdentityFromTheDatabase(t *testing.T) {
	a := newAuth(t, liveSessions{"sid-1": {"user-1", "admin"}})
	// The token itself carries no role; even a forged "role" claim is ignored.
	code, seen, _ := run(a.Middleware, "GET", "/works", bearer(t, "sid-1", "user-1"))
	if code != 200 || seen == nil {
		t.Fatalf("code=%d", code)
	}
	if got := seen.Context().Value(UserIDKey); got != "user-1" {
		t.Errorf("user id = %v", got)
	}
	if got := seen.Context().Value(UserRoleKey); got != "admin" {
		t.Errorf("role = %v, want the database role", got)
	}
	if got := seen.Context().Value(SessionIDKey); got != "sid-1" {
		t.Errorf("session id = %v", got)
	}
}

func TestMiddleware_RevokedOrBlockedSessionIsRefusedImmediately(t *testing.T) {
	sessions := liveSessions{"sid-1": {"user-1", "reader"}}
	a := newAuth(t, sessions)
	header := bearer(t, "sid-1", "user-1")

	if code, _, _ := run(a.Middleware, "GET", "/works", header); code != 200 {
		t.Fatalf("live session: %d", code)
	}
	delete(sessions, "sid-1") // logout, revocation, or account blocked
	if code, seen, _ := run(a.Middleware, "GET", "/works", header); code != 401 || seen != nil {
		t.Errorf("after revocation: code=%d ran=%v, want 401", code, seen != nil)
	}
}

func TestMiddleware_TokenNotBackedByASessionIsRefused(t *testing.T) {
	a := newAuth(t, liveSessions{})
	// Perfectly signed, unexpired, but its session does not exist.
	if code, _, _ := run(a.Middleware, "GET", "/works", bearer(t, "ghost", "user-1")); code != 401 {
		t.Errorf("got %d, want 401", code)
	}
}

func TestMiddleware_WithoutSessionCheckerFailsClosed(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	a := Authenticator{}
	if code, _, _ := run(a.Middleware, "GET", "/works", bearer(t, "sid-1", "u")); code != 401 {
		t.Errorf("got %d, want 401 when no session checker is configured", code)
	}
}

func signRaw(t *testing.T, method jwt.SigningMethod, claims jwt.MapClaims, key interface{}) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMiddleware_RejectsMalformedTokens(t *testing.T) {
	a := newAuth(t, liveSessions{"sid-1": {"user-1", "owner"}})
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()
	secret := []byte(testSecret)

	cases := map[string]string{
		"alg none": signRaw(t, jwt.SigningMethodNone,
			jwt.MapClaims{"typ": "session", "sid": "sid-1", "exp": future}, jwt.UnsafeAllowNoneSignatureType),
		"expired": signRaw(t, jwt.SigningMethodHS256,
			jwt.MapClaims{"typ": "session", "sid": "sid-1", "exp": past}, secret),
		"no exp": signRaw(t, jwt.SigningMethodHS256,
			jwt.MapClaims{"typ": "session", "sid": "sid-1"}, secret),
		"wrong secret": signRaw(t, jwt.SigningMethodHS256,
			jwt.MapClaims{"typ": "session", "sid": "sid-1", "exp": future}, []byte("another-secret-entirely-000000000")),
		"no typ (old style JWT)": signRaw(t, jwt.SigningMethodHS256,
			jwt.MapClaims{"sub": "user-1", "role": "admin", "exp": future}, secret),
		"no sid": signRaw(t, jwt.SigningMethodHS256,
			jwt.MapClaims{"typ": "session", "exp": future}, secret),
		"garbage": "not.a.jwt",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if code, seen, _ := run(a.Middleware, "GET", "/works", "Bearer "+tok); code != 401 || seen != nil {
				t.Errorf("got %d ran=%v, want 401", code, seen != nil)
			}
		})
	}
}

func TestMiddleware_QueryStringSessionTokenIsNotAccepted(t *testing.T) {
	a := newAuth(t, liveSessions{"sid-1": {"user-1", "reader"}})
	tok := bearer(t, "sid-1", "user-1")[len("Bearer "):]
	for _, mw := range []func(http.Handler) http.Handler{a.Middleware, a.Assets, a.AssetsWithBasic, a.WithBasic} {
		if code, seen, _ := run(mw, "GET", "/files/x.epub?token="+tok, ""); code != 401 || seen != nil {
			t.Errorf("?token= accepted: code=%d", code)
		}
		if code, seen, _ := run(mw, "GET", "/files/x.epub?rt="+tok, ""); code != 401 || seen != nil {
			t.Errorf("session token accepted as ?rt=: code=%d", code)
		}
	}
}

func TestMiddleware_BasicIsRefusedOnTheCommonMiddleware(t *testing.T) {
	a := newAuth(t, liveSessions{})
	if code, seen, _ := run(a.Middleware, "GET", "/works", basicHeader("ana", "cdc_valid")); code != 401 || seen != nil {
		t.Errorf("Basic accepted on the common middleware: %d", code)
	}
	if code, _, _ := run(a.Assets, "GET", "/files/x", basicHeader("ana", "cdc_valid")); code != 401 {
		t.Errorf("Basic accepted on Assets: %d", code)
	}
}

func TestWithBasic(t *testing.T) {
	a := newAuth(t, liveSessions{})
	for _, mw := range []func(http.Handler) http.Handler{a.WithBasic, a.AssetsWithBasic} {
		code, seen, _ := run(mw, "GET", "/opds/v1.2/catalog", basicHeader("ana", "cdc_valid"))
		if code != 200 || seen == nil || seen.Context().Value(UserIDKey) != "user-ana" {
			t.Errorf("valid app token: code=%d", code)
		}
		bad := map[string]string{
			"wrong secret":       basicHeader("ana", "cdc_other"),
			"account password":   basicHeader("ana", "hunter2"),
			"empty password":     basicHeader("ana", ""),
			"wrong user":         basicHeader("bob", "cdc_valid"),
			"no colon":           "Basic " + base64.StdEncoding.EncodeToString([]byte("ana")),
			"invalid base64":     "Basic !!!",
			"unsupported scheme": "Digest abc",
		}
		for name, h := range bad {
			code, seen, rec := run(mw, "GET", "/opds/v1.2/catalog", h)
			if code != 401 || seen != nil {
				t.Errorf("%s: code=%d ran=%v", name, code, seen != nil)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Errorf("%s: missing Basic challenge", name)
			}
		}
		if code, _, rec := run(mw, "GET", "/opds/v1.2/catalog", ""); code != 401 || rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("no credentials: code=%d, want 401 with a challenge", code)
		}
	}
}

func TestWithBasic_VerifierFailureIsNotAuthentication(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	a := Authenticator{Basic: func(ctx context.Context, u, p string) (string, string, error) {
		return "", "", errors.New("database unavailable")
	}}
	if code, seen, _ := run(a.WithBasic, "GET", "/x", basicHeader("ana", "cdc_x")); code != 500 || seen != nil {
		t.Errorf("got %d, want 500", code)
	}
}

func resourceToken(t *testing.T, sid, scope string, ttl time.Duration) string {
	t.Helper()
	tok, _, err := IssueResourceToken(sid, "user-1", scope, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAssets_ResourceToken(t *testing.T) {
	sessions := liveSessions{"sid-1": {"user-1", "reader"}}
	a := newAuth(t, sessions)
	good := resourceToken(t, "sid-1", ScopeAssets, time.Minute)

	// Accepted on GET and HEAD of asset routes.
	for _, m := range []string{"GET", "HEAD"} {
		if code, seen, _ := run(a.Assets, m, "/covers/a.jpg?rt="+good, ""); code != 200 || seen == nil {
			t.Errorf("%s with valid rt: %d", m, code)
		}
	}
	// Never on unsafe methods.
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if code, seen, _ := run(a.Assets, m, "/covers/a.jpg?rt="+good, ""); code != 401 || seen != nil {
			t.Errorf("%s with rt: %d, want 401", m, code)
		}
	}
	// Not on the common middleware.
	if code, _, _ := run(a.Middleware, "GET", "/works?rt="+good, ""); code != 401 {
		t.Errorf("rt accepted on the common middleware: %d", code)
	}
	// A resource token is not a bearer session token.
	if code, _, _ := run(a.Middleware, "GET", "/works", "Bearer "+good); code != 401 {
		t.Errorf("resource token accepted as bearer: %d", code)
	}
	// Wrong scope, expired, and revoked-session tokens are refused.
	if code, _, _ := run(a.Assets, "GET", "/covers/a.jpg?rt="+resourceToken(t, "sid-1", ScopeWS, time.Minute), ""); code != 401 {
		t.Errorf("ws-scoped token accepted for assets: %d", code)
	}
	if code, _, _ := run(a.Assets, "GET", "/covers/a.jpg?rt="+resourceToken(t, "sid-1", ScopeAssets, -time.Minute), ""); code != 401 {
		t.Errorf("expired rt accepted: %d", code)
	}
	delete(sessions, "sid-1")
	if code, _, _ := run(a.Assets, "GET", "/covers/a.jpg?rt="+good, ""); code != 401 {
		t.Errorf("rt of a revoked session accepted: %d", code)
	}
}

func TestAuthenticateWS(t *testing.T) {
	sessions := liveSessions{"sid-1": {"user-1", "reader"}}
	a := newAuth(t, sessions)

	wsReq := func(target, header string) *http.Request {
		r := httptest.NewRequest("GET", target, nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		return r
	}

	if u, _, err := a.AuthenticateWS(wsReq("/ws?ticket="+resourceToken(t, "sid-1", ScopeWS, time.Minute), "")); err != nil || u != "user-1" {
		t.Errorf("valid ticket: %v %v", u, err)
	}
	if _, _, err := a.AuthenticateWS(wsReq("/ws?ticket="+resourceToken(t, "sid-1", ScopeAssets, time.Minute), "")); err == nil {
		t.Error("assets-scoped token accepted as a ws ticket")
	}
	if _, _, err := a.AuthenticateWS(wsReq("/ws", "")); err == nil {
		t.Error("connection without credentials accepted")
	}
	if _, _, err := a.AuthenticateWS(wsReq("/ws?token="+bearer(t, "sid-1", "user-1")[7:], "")); err == nil {
		t.Error("session token in the query string accepted")
	}
	if _, _, err := a.AuthenticateWS(wsReq("/ws", bearer(t, "sid-1", "user-1"))); err != nil {
		t.Errorf("bearer header for non-browser clients: %v", err)
	}
	delete(sessions, "sid-1")
	if _, _, err := a.AuthenticateWS(wsReq("/ws?ticket="+resourceToken(t, "sid-1", ScopeWS, time.Minute), "")); err == nil {
		t.Error("ticket of a revoked session accepted")
	}
}

func TestGetJWTSecret_MissingSecretIsFatalInAnyEnv(t *testing.T) {
	for _, env := range []string{"", "development", "production"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			t.Setenv("JWT_SECRET", "")
			defer func() {
				if recover() == nil {
					t.Error("expected GetJWTSecret to panic when JWT_SECRET is unset")
				}
			}()
			GetJWTSecret()
		})
	}
}
