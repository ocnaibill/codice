package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"golang.org/x/crypto/bcrypt"
)

// authStack wires the real store, handlers and middleware over a live database,
// the way the router does, with a few probe routes.
type authStack struct {
	db       *sql.DB
	store    *sessions.Store
	router   http.Handler
	authH    *AuthHandler
	lastRole string
}

func newAuthStack(t *testing.T) *authStack {
	t.Helper()
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	db := migratedDB(t)
	st := &sessions.Store{DB: db}
	a := middleware.Authenticator{Sessions: st.CheckSession, Basic: st.VerifyAppToken}
	s := &authStack{db: db, store: st, authH: &AuthHandler{DB: db, Sessions: st}}
	tokens := &AppTokensHandler{Sessions: st}

	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastRole, _ = r.Context().Value(middleware.UserRoleKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	r.Post("/auth/login", s.authH.Login)
	r.With(a.Middleware).Post("/auth/logout", s.authH.Logout)
	r.With(a.Middleware).Post("/auth/resource-token", s.authH.ResourceToken)
	r.With(a.Middleware).Post("/auth/app-tokens", tokens.Create)
	r.With(a.Middleware).Get("/auth/app-tokens", tokens.List)
	r.With(a.Middleware).Delete("/auth/app-tokens/{id}", tokens.Revoke)
	r.With(a.Middleware).Get("/probe", probe)
	r.With(a.AssetsWithBasic).Get("/files/probe", probe)
	s.router = r
	return s
}

func (s *authStack) addUserWithPassword(t *testing.T, name, role, password string) string {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	var id string
	if err := s.db.QueryRow(
		`INSERT INTO users (username, email, password_hash, role) VALUES ($1,$2,$3,$4) RETURNING id`,
		name, name+"@x", string(hash), role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (s *authStack) req(method, target, bearer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func (s *authStack) login(t *testing.T, user, pass string) string {
	t.Helper()
	rec := s.req("POST", "/auth/login", "", `{"username":"`+user+`","password":"`+pass+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body.String())
	}
	var body AuthResponse
	json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Token
}

func TestSessions_LoginLogoutRevokesTheTokenImmediately(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")

	token := s.login(t, "ana", "s3cret")
	if rec := s.req("GET", "/probe", token, ""); rec.Code != 200 {
		t.Fatalf("live session: %d", rec.Code)
	}
	if rec := s.req("POST", "/auth/logout", token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", token, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("token after logout: %d, want 401", rec.Code)
	}
}

func TestSessions_BlockedAccountLosesEverythingAtOnce(t *testing.T) {
	s := newAuthStack(t)
	id := s.addUserWithPassword(t, "ana", "reader", "s3cret")

	token := s.login(t, "ana", "s3cret")
	resource := mintToken(t, s, token, "assets")
	_, appToken := createAppToken(t, s, token, "kobo")
	if rec := s.req("GET", "/probe", token, ""); rec.Code != 200 {
		t.Fatalf("before block: %d", rec.Code)
	}

	if _, err := s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	if rec := s.req("GET", "/probe", token, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("session of a blocked account: %d, want 401", rec.Code)
	}
	if rec := s.req("GET", "/files/probe?rt="+resource, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("resource token of a blocked account: %d, want 401", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("app token of a blocked account: %d, want 401", rec.Code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"s3cret"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("login of a blocked account: %d, want 401", rec.Code)
	}

	// Unblocking is reversible for the account (sessions were not destroyed).
	s.db.Exec(`UPDATE users SET blocked_at = NULL WHERE id = $1`, id)
	if rec := s.req("GET", "/probe", token, ""); rec.Code != 200 {
		t.Errorf("after unblock: %d, want 200", rec.Code)
	}
}

func TestSessions_RoleComesFromTheDatabaseOnEveryRequest(t *testing.T) {
	s := newAuthStack(t)
	id := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	token := s.login(t, "ana", "s3cret")

	s.req("GET", "/probe", token, "")
	if s.lastRole != "reader" {
		t.Fatalf("role = %q", s.lastRole)
	}
	s.db.Exec(`UPDATE users SET role = 'admin' WHERE id = $1`, id)
	s.req("GET", "/probe", token, "")
	if s.lastRole != "admin" {
		t.Errorf("after promotion role = %q, want admin without a new login", s.lastRole)
	}
	s.db.Exec(`UPDATE users SET role = 'reader' WHERE id = $1`, id)
	s.req("GET", "/probe", token, "")
	if s.lastRole != "reader" {
		t.Errorf("after demotion role = %q, want reader immediately", s.lastRole)
	}
}

func TestSessions_ExpiredSessionIsRefused(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	token := s.login(t, "ana", "s3cret")

	s.db.Exec(`UPDATE sessions SET expires_at = now() - interval '1 minute'`)
	if rec := s.req("GET", "/probe", token, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("expired session: %d, want 401", rec.Code)
	}
}

func TestLogin_FailuresAreIndistinguishable(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.db.Exec(`INSERT INTO users (username, email, role) VALUES ('sso_only','s@x','reader')`)

	var bodies []string
	for _, creds := range []string{
		`{"username":"ana","password":"wrong"}`,
		`{"username":"nobody","password":"whatever"}`,
		`{"username":"sso_only","password":"whatever"}`,
	} {
		rec := s.req("POST", "/auth/login", "", creds)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", creds, rec.Code)
		}
		bodies = append(bodies, rec.Body.String())
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Errorf("login failures differ, which leaks which usernames exist: %q", bodies)
	}
}

func mintToken(t *testing.T, s *authStack, session, scope string) string {
	t.Helper()
	rec := s.req("POST", "/auth/resource-token", session, `{"scope":"`+scope+`"}`)
	if rec.Code != 200 {
		t.Fatalf("resource token: %d", rec.Code)
	}
	var b resourceTokenResponse
	json.Unmarshal(rec.Body.Bytes(), &b)
	return b.Token
}

func createAppToken(t *testing.T, s *authStack, session, name string) (id, token string) {
	t.Helper()
	rec := s.req("POST", "/auth/app-tokens", session, `{"name":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app token: %d %s", rec.Code, rec.Body.String())
	}
	var b createAppTokenResponse
	json.Unmarshal(rec.Body.Bytes(), &b)
	if !strings.HasPrefix(b.Token, sessions.AppTokenPrefix) {
		t.Fatalf("token %q lacks the %s prefix", b.Token, sessions.AppTokenPrefix)
	}
	return b.ID, b.Token
}

func basicReq(s *authStack, user, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/files/probe", nil)
	req.SetBasicAuth(user, secret)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestAppTokens_Lifecycle(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.addUserWithPassword(t, "bob", "reader", "pw-bob")
	anaSession := s.login(t, "ana", "s3cret")
	bobSession := s.login(t, "bob", "pw-bob")

	id, secret := createAppToken(t, s, anaSession, "Kobo")

	// The app token works over Basic; the account password does not (DEC-071).
	if rec := basicReq(s, "ana", secret); rec.Code != 200 {
		t.Errorf("valid app token: %d", rec.Code)
	}
	if rec := basicReq(s, "ana", "s3cret"); rec.Code != http.StatusUnauthorized {
		t.Errorf("account password accepted over Basic: %d", rec.Code)
	}
	// It belongs to ana: another username cannot borrow it.
	if rec := basicReq(s, "bob", secret); rec.Code != http.StatusUnauthorized {
		t.Errorf("app token accepted for another user: %d", rec.Code)
	}
	// It is not a session and opens nothing else.
	if rec := s.req("GET", "/probe", secret, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("app token accepted as bearer: %d", rec.Code)
	}

	// Only a hash is stored, and listing never reveals the secret.
	var stored string
	s.db.QueryRow(`SELECT token_hash FROM app_tokens WHERE id = $1`, id).Scan(&stored)
	if stored == "" || strings.Contains(stored, secret) || len(stored) != 64 {
		t.Errorf("stored value %q is not a SHA-256 of the token", stored)
	}
	list := s.req("GET", "/auth/app-tokens", anaSession, "")
	if strings.Contains(list.Body.String(), secret) || !strings.Contains(list.Body.String(), "Kobo") {
		t.Errorf("list body: %s", list.Body.String())
	}
	if rec := s.req("GET", "/auth/app-tokens", bobSession, ""); strings.Contains(rec.Body.String(), "Kobo") {
		t.Error("bob sees ana's tokens")
	}

	// Bob cannot revoke ana's token; ana can, and it dies at once.
	if rec := s.req("DELETE", "/auth/app-tokens/"+id, bobSession, ""); rec.Code != http.StatusNotFound {
		t.Errorf("revoking someone else's token: %d, want 404", rec.Code)
	}
	if rec := basicReq(s, "ana", secret); rec.Code != 200 {
		t.Errorf("token affected by another user's revoke attempt: %d", rec.Code)
	}
	if rec := s.req("DELETE", "/auth/app-tokens/"+id, anaSession, ""); rec.Code != http.StatusNoContent {
		t.Errorf("revoke: %d", rec.Code)
	}
	if rec := basicReq(s, "ana", secret); rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked app token still works: %d", rec.Code)
	}
}

func TestAppTokens_RejectsBadInput(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	session := s.login(t, "ana", "s3cret")
	for _, body := range []string{`{}`, `{"name":"   "}`, `{"name":"` + strings.Repeat("x", 101) + `"}`, `nope`} {
		if rec := s.req("POST", "/auth/app-tokens", session, body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %.20q: got %d, want 400", body, rec.Code)
		}
	}
	if rec := s.req("DELETE", "/auth/app-tokens/not-a-uuid", session, ""); rec.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", rec.Code)
	}
}

func TestRevokeAllForUser_EndsSessionsAndAppTokens(t *testing.T) {
	s := newAuthStack(t)
	id := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	other := s.addUserWithPassword(t, "bob", "reader", "pw-bob")
	_ = other
	anaSession := s.login(t, "ana", "s3cret")
	bobSession := s.login(t, "bob", "pw-bob")
	_, secret := createAppToken(t, s, anaSession, "Kobo")

	if err := s.store.RevokeAllForUser(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if rec := s.req("GET", "/probe", anaSession, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("session after RevokeAllForUser: %d", rec.Code)
	}
	if rec := basicReq(s, "ana", secret); rec.Code != http.StatusUnauthorized {
		t.Errorf("app token after RevokeAllForUser: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", bobSession, ""); rec.Code != 200 {
		t.Errorf("another user's session was affected: %d", rec.Code)
	}
}
