package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/ocnaibill/codice/backend/internal/logins"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

const testSecret = "test_secret_key_for_testing_12345678"

const someUUID = "b190281f-fe3e-4308-ad71-91e24744d7a0"

// The fake session table encodes the role in the session id ("role:admin"),
// standing in for the database lookup that reads the role of a live session.
func fakeSessions(ctx context.Context, sid string) (string, string, error) {
	if role, ok := strings.CutPrefix(sid, "role:"); ok {
		return someUUID, role, nil
	}
	return "", "", middleware.ErrInvalidSession
}

// testRouter builds the real route table with no database or Redis: every
// request in these tests must be stopped (or fail validation) before a
// handler touches them.
func testRouter(t *testing.T) http.Handler {
	t.Helper()
	return testRouterWith(t, nil)
}

// testRouterWith is testRouter with a change to the dependencies.
func testRouterWith(t *testing.T, change func(*routerDeps)) http.Handler {
	t.Helper()
	t.Setenv("JWT_SECRET", testSecret)
	auth := middleware.Authenticator{Sessions: fakeSessions}
	notFound := func(ctx context.Context, rel string) (handlers.FileAccess, error) {
		return handlers.FileAccess{}, handlers.ErrFileNotFound
	}
	deps := routerDeps{
		Auth:        auth,
		WS:          &handlers.WsHandler{Auth: auth},
		StoragePath: t.TempDir(),
		// Every path is unknown: enough to tell "past authentication" (404)
		// from "refused" (401), without a database.
		FileLookup:  notFound,
		CoverLookup: func(ctx context.Context, name string) (handlers.FileAccess, error) { return handlers.FileAccess{}, nil },
	}
	if change != nil {
		change(&deps)
	}
	return newRouter(deps)
}

// tokenFor returns a session token whose session resolves to the given role.
func tokenFor(t *testing.T, role string) string {
	t.Helper()
	s, err := middleware.IssueSessionToken("role:"+role, someUUID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type route struct{ method, path string }

// Routes the specification reserves to owner and admin (DEC-003, RF-007).
var staffRoutes = []route{
	{"GET", "/admin/logins"},
	{"GET", "/admin/performance"},
	{"GET", "/users/" + someUUID + "/sessions"},
	{"DELETE", "/users/" + someUUID + "/sessions"},
	{"DELETE", "/users/" + someUUID + "/sessions/" + someUUID},
	{"GET", "/admin/backup"},
	{"GET", "/password-resets"},
	{"POST", "/password-resets/" + someUUID + "/approve"},
	{"POST", "/password-resets/" + someUUID + "/reject"},
	{"DELETE", "/users/" + someUUID},
	{"GET", "/invitations"},
	{"POST", "/invitations"},
	{"DELETE", "/invitations/" + someUUID},
	{"GET", "/users"},
	{"POST", "/users/" + someUUID + "/block"},
	{"POST", "/users/" + someUUID + "/unblock"},
	{"POST", "/upload"},
	{"POST", "/works/bulk-import"},
	{"PUT", "/works/1"},
	{"POST", "/works/1/titles"},
	{"DELETE", "/works/1/titles/1"},
	{"PATCH", "/works/1/editions/1"},
	{"POST", "/works/1/contributors"},
	{"PUT", "/works/1/contributors/order"},
	{"DELETE", "/works/1/contributors/1/author"},
	{"POST", "/collections"},
	{"PATCH", "/collections/1"},
	{"DELETE", "/collections/1"},
	{"POST", "/collections/1/restore"},
	{"PUT", "/collections/1/works/1"},
	{"DELETE", "/collections/1/works/1"},
	{"PUT", "/collections/1/order"},
	{"PUT", "/collections/1/classification"},
	{"DELETE", "/works/1"},
	{"POST", "/works/1/restore"},
	{"GET", "/admin/jobs"},
	{"GET", "/admin/duplicates"},
	{"POST", "/admin/duplicates/scan"},
	{"POST", "/admin/duplicates/1/dismiss"},
	{"POST", "/admin/duplicates/1/link"},
	{"POST", "/admin/works/1/join"},
	{"POST", "/admin/works/1/not-same-as"},
	{"POST", "/admin/editions/1/split"},
	{"GET", "/admin/storage/referenced"},
	{"GET", "/admin/storage/transfers?ids=1"},
	{"GET", "/admin/suggestions"},
	{"GET", "/admin/metadata-providers"},
	{"GET", "/admin/people/merges"},
	{"GET", "/admin/people/names"},
	{"GET", "/admin/ocr"},
	{"POST", "/admin/works/1/ocr/retry"},
	{"POST", "/admin/files/1/ocr/language"},
	{"POST", "/admin/people/merges/1/merge"},
	{"POST", "/admin/people/merges/1/dismiss"},
	{"PUT", "/admin/people/1/name"},
	{"GET", "/admin/retired-works"},
	{"GET", "/admin/trash"},
	{"POST", "/admin/trash/empty"},
	{"GET", "/admin/trash/policy"},
	{"GET", "/admin/storage/orphans"},
	{"GET", "/admin/storage/reorganize"},
	{"GET", "/admin/storage/roots"},
	{"POST", "/admin/storage/roots/1/purge-retired"},
	{"POST", "/admin/library/scan"},
	{"POST", "/admin/library/move-to-managed"},
	{"GET", "/admin/storage/cleanups"},
	{"POST", "/admin/storage/cleanups/retry"},
	{"POST", "/admin/storage/reorganize"},
	{"POST", "/admin/jobs/rerun-failed"},
	{"POST", "/admin/jobs/1/rerun"},
	{"POST", "/admin/jobs/1/cancel"},
	{"GET", "/works/1/candidates"},
	{"POST", "/works/1/candidates/1/accept"},
	{"POST", "/works/1/candidates/1/reject"},
}

// Routes reserved to the owner alone (DEC-056).
var ownerRoutes = []route{
	{"PUT", "/admin/logins/settings"},
	{"PUT", "/admin/performance"},
	{"PUT", "/admin/ldap/policy"},
	{"POST", "/ownership/transfer"},
	{"PUT", "/admin/trash/policy"},
	{"POST", "/admin/trash/policy/apply"},
	{"POST", "/admin/storage/roots"},
	{"PUT", "/admin/name-order"},
	{"PUT", "/admin/metadata-providers/openlibrary"},
	{"PUT", "/users/" + someUUID + "/role"},
}

func TestStaffRoutes_ReaderIsForbidden(t *testing.T) {
	h := testRouter(t)
	reader := tokenFor(t, "reader")
	for _, rt := range staffRoutes {
		if rec := do(h, rt.method, rt.path, reader, ""); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as reader: got %d, want 403", rt.method, rt.path, rec.Code)
		}
	}
}

func TestStaffRoutes_AnonymousIsUnauthorized(t *testing.T) {
	h := testRouter(t)
	for _, rt := range append(append([]route{}, staffRoutes...), ownerRoutes...) {
		if rec := do(h, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: got %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

func TestStaffRoutes_AdminAndOwnerReachTheHandler(t *testing.T) {
	h := testRouter(t)
	// POST /upload with no multipart body is rejected by the handler itself
	// (400) before it touches storage, proving the middleware let it through.
	for _, role := range []string{"admin", "owner"} {
		rec := do(h, "POST", "/upload", tokenFor(t, role), "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST /upload as %s: got %d, want 400 from the handler", role, rec.Code)
		}
	}
}

func TestOwnerRoutes_OnlyOwnerPasses(t *testing.T) {
	h := testRouter(t)
	for _, rt := range ownerRoutes {
		for _, role := range []string{"admin", "reader"} {
			if rec := do(h, rt.method, rt.path, tokenFor(t, role), `{"role":"admin"}`); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", rt.method, rt.path, role, rec.Code)
			}
		}
		// Owner passes the middleware; an invalid role is rejected by the
		// handler with 400 before any database access.
		if rec := do(h, rt.method, rt.path, tokenFor(t, "owner"), `{"role":"owner"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s as owner with role=owner: got %d, want 400", rt.method, rt.path, rec.Code)
		}
	}
}

func TestAuthRoutes_RequireASession(t *testing.T) {
	h := testRouter(t)
	for _, rt := range []route{
		{"GET", "/auth/me"},
		{"GET", "/auth/preferences"},
		{"PUT", "/auth/preferences"},
		{"GET", "/ownership/transfer"},
		{"POST", "/ownership/transfer/accept"},
		{"POST", "/ownership/transfer/decline"},
		{"POST", "/auth/notices/1/ack"},
		{"POST", "/auth/password"},
		{"POST", "/auth/logout"},
		{"POST", "/auth/resource-token"},
		{"POST", "/auth/app-tokens"},
		{"GET", "/auth/app-tokens"},
		{"DELETE", "/auth/app-tokens/" + someUUID},
		{"GET", "/works"},
		{"GET", "/works/1/series"},
		{"GET", "/notes"},
		{"GET", "/notes/facets"},
		{"GET", "/graph/types"},
		{"GET", "/concepts"},
		{"POST", "/concepts"},
		{"GET", "/concepts/resolve?name=x"},
		{"GET", "/concepts/1"},
		{"PATCH", "/concepts/1"},
		{"DELETE", "/concepts/1"},
		{"GET", "/relations"},
		{"POST", "/relations"},
		{"PATCH", "/relations/1"},
		{"DELETE", "/relations/1"},
	} {
		if rec := do(h, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: got %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

func mintResourceToken(t *testing.T, h http.Handler, session, scope string) string {
	t.Helper()
	rec := do(h, "POST", "/auth/resource-token", session, `{"scope":"`+scope+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resource-token(%s): %d %s", scope, rec.Code, rec.Body.String())
	}
	var body struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Token == "" {
		t.Fatal("empty resource token")
	}
	return body.Token
}

func TestResourceTokens_OnlyOpenAssetRoutes(t *testing.T) {
	h := testRouter(t)
	session := tokenFor(t, "reader")
	rt := mintResourceToken(t, h, session, "assets")

	// Accepted where a browser loads assets without headers; the static
	// handler then answers 404 for the missing file, which is past auth.
	for _, path := range []string{"/covers/none.jpg", "/files/none.epub"} {
		if rec := do(h, "GET", path+"?rt="+rt, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s?rt=: got %d, want 404 (authenticated)", path, rec.Code)
		}
	}

	// Refused on every other route and for every unsafe method.
	for _, r := range []route{{"GET", "/works"}, {"GET", "/notes"}, {"GET", "/auth/app-tokens"}, {"PUT", "/works/1"}, {"POST", "/upload"}} {
		if rec := do(h, r.method, r.path+"?rt="+rt, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s?rt=: got %d, want 401", r.method, r.path, rec.Code)
		}
	}
	if rec := do(h, "DELETE", "/covers/none.jpg?rt="+rt, "", ""); rec.Code == http.StatusNotFound || rec.Code == http.StatusOK {
		t.Errorf("DELETE with rt reached the file server: %d", rec.Code)
	}

	// The session token in the query string is gone for good.
	if rec := do(h, "GET", "/covers/none.jpg?token="+session, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("?token= accepted: %d", rec.Code)
	}
	// A ws ticket is not an asset token.
	ws := mintResourceToken(t, h, session, "ws")
	if rec := do(h, "GET", "/covers/none.jpg?rt="+ws, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("ws ticket accepted for assets: %d", rec.Code)
	}
	// Unknown scopes are not minted.
	if rec := do(h, "POST", "/auth/resource-token", session, `{"scope":"everything"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown scope: got %d, want 400", rec.Code)
	}
}

func TestWebSocket_RejectsAnonymousAndQueryToken(t *testing.T) {
	h := testRouter(t)
	if rec := do(h, "GET", "/ws", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous /ws: %d", rec.Code)
	}
	if rec := do(h, "GET", "/ws?token="+tokenFor(t, "owner"), "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("/ws?token=: %d", rec.Code)
	}
}

func TestOldStyleTokenWithRoleClaimIsRefused(t *testing.T) {
	// Tokens minted before sessions (sub + role, no sid) must not open any door.
	h := testRouter(t)
	rec := do(h, "PUT", "/works/1", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ4Iiwicm9sZSI6ImFkbWluIn0.x", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

func TestDictionaryLookup_NeedsASignedInReader(t *testing.T) {
	h := testRouter(t)
	if rec := do(h, "GET", "/dictionary?lang=pt&word=casa", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: got %d, want 401", rec.Code)
	}
}

func TestOwnerOnlyRoutes_ThatNeedTheDatabaseAreStillGuarded(t *testing.T) {
	// Removing an authorised directory reaches the database once past the guard,
	// so only the guard itself is checked here.
	h := testRouter(t)
	for _, rt := range []route{{"DELETE", "/admin/storage/roots/1"}, {"GET", "/admin/trash/policy/preview"}, {"DELETE", "/ownership/transfer"}, {"GET", "/admin/ldap"}, {"POST", "/admin/ldap/check"}, {"GET", "/admin/embeddings"}, {"PUT", "/admin/embeddings"}, {"POST", "/admin/dictionaries/wikt-pt/install"}, {"POST", "/admin/dictionaries/wikt-pt/cancel"}, {"DELETE", "/admin/dictionaries/wikt-pt"}, {"GET", "/admin/ocr/settings"}, {"PUT", "/admin/ocr/settings"}} {
		for _, role := range []string{"admin", "reader"} {
			if rec := do(h, rt.method, rt.path, tokenFor(t, role), ""); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", rt.method, rt.path, role, rec.Code)
			}
		}
		if rec := do(h, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: got %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

// Recovery of the owner exists only as a command on the server (DEC-057, RF-038):
// no route may offer it, remotely or to an admin.
func TestOwnerRecoveryIsNotReachableOverHTTP(t *testing.T) {
	routes, ok := testRouter(t).(chi.Routes)
	if !ok {
		t.Fatal("the router cannot be walked")
	}
	chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.Contains(strings.ToLower(route), "recover") {
			t.Errorf("%s %s exposes recovery over HTTP", method, route)
		}
		return nil
	})
}

func TestHealthz_IsPublicAndAnswersEvenWithoutADatabase(t *testing.T) {
	rec := do(testRouter(t), "GET", "/healthz", "", "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"database":"down"`) {
		t.Errorf("no database: %d %s (public, and honest about being down)", rec.Code, rec.Body.String())
	}
}

func limited(h http.Handler, remote, xff string) int {
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestAuthRateLimit_ForgedHeadersCannotDodgeItWithoutATrustedProxy(t *testing.T) {
	h := authRateLimiter(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	for i := 0; i < 10; i++ {
		if code := limited(h, "203.0.113.9:1000", "1.1.1."+string(rune('0'+i))); code != 200 {
			t.Fatalf("request %d: %d", i, code)
		}
	}
	if code := limited(h, "203.0.113.9:1000", "9.9.9.9"); code != http.StatusTooManyRequests {
		t.Errorf("an 11th request with a new forged address: %d, want 429", code)
	}
}

func TestAuthRateLimit_BehindATrustedProxyClientsDoNotShareABudget(t *testing.T) {
	trusted, err := middleware.ParseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	h := authRateLimiter(trusted, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	proxy := "172.18.0.5:4000"
	for i := 0; i < 10; i++ {
		limited(h, proxy, "198.51.100.1")
	}
	if code := limited(h, proxy, "198.51.100.1"); code != http.StatusTooManyRequests {
		t.Errorf("the eleventh request from one client: %d, want 429", code)
	}
	if code := limited(h, proxy, "198.51.100.2"); code != 200 {
		t.Errorf("another client behind the same proxy: %d, want 200 (they must not share one budget)", code)
	}
	// A forged left-hand entry does not give an attacker a fresh budget.
	if code := limited(h, proxy, "6.6.6.6, 198.51.100.1"); code != http.StatusTooManyRequests {
		t.Errorf("forged left entry: %d, want 429", code)
	}
}

// A client that is not signed in must not be able to make the server read an unbounded body:
// login decodes JSON before it looks at anything else.
func TestRouter_RefusesAnOversizedBodyBeforeTheHandlerReadsIt(t *testing.T) {
	r := testRouter(t)
	huge := `{"username":"ana","password":"` + strings.Repeat("x", middleware.MaxJSONBody+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(huge))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a body over %d bytes", rec.Code, middleware.MaxJSONBody)
	}
}

// The "Sobre" screen is for whoever is signed in, whatever the role.
func TestRouter_AboutNeedsASessionButNoRole(t *testing.T) {
	r := testRouter(t)
	if rec := do(r, "GET", "/about", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d, want 401", rec.Code)
	}
	for _, role := range []string{"reader", "admin", "owner"} {
		rec := do(r, "GET", "/about", tokenFor(t, role), "")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d, want 200", role, rec.Code)
		}
	}
}

// The owner's backup buttons (DEC-123): the owner alone, with a session, and never without the password.
var panelOwnerRoutes = []route{
	{"POST", "/admin/backup/run"},
	{"POST", "/admin/backup/verify"},
}

func TestBackupPanelRoutes_AreTheOwnersAlone(t *testing.T) {
	h := testRouter(t)
	for _, rt := range panelOwnerRoutes {
		if rec := do(h, rt.method, rt.path, "", `{"password":"x"}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: %d, want 401", rt.method, rt.path, rec.Code)
		}
		for _, role := range []string{"reader", "admin"} {
			if rec := do(h, rt.method, rt.path, tokenFor(t, role), `{"password":"x"}`); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403", rt.method, rt.path, role, rec.Code)
			}
		}
		// The owner gets past the middleware; the test router has no panel set up, so the handler says so.
		if rec := do(h, rt.method, rt.path, tokenFor(t, "owner"), `{"password":"x"}`); rec.Code != http.StatusConflict {
			t.Errorf("%s %s as owner: %d, want 409 (panel not set up)", rt.method, rt.path, rec.Code)
		}
	}
}

// "Sessões e dispositivos" is for whoever is signed in, whatever the role, and for no one who is not.
func TestSessionsRoutes_NeedASessionButNoRole(t *testing.T) {
	h := testRouter(t)
	for _, rt := range []route{{"GET", "/auth/sessions"}, {"GET", "/auth/logins"}, {"GET", "/auth/export"}, {"POST", "/auth/export"}, {"GET", "/auth/export/" + someUUID + "/download"}, {"DELETE", "/auth/export/" + someUUID}, {"DELETE", "/auth/sessions/" + someUUID}, {"POST", "/auth/sessions/revoke-others"}} {
		if rec := do(h, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

// The app asks "has the server been set up?" at every load, and a sign-in has ten tries a minute: reloading the page must not
// use up the tries, and the page must not be refused after a few reloads (the app showed the login screen over a good session).
func TestSetupStatusRateLimit_HasABudgetOfItsOwn(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := statusRateLimiter(nil)(ok)
	codes := map[int]int{}
	for i := 0; i < StatusRequestsPerMinute+5; i++ {
		req := httptest.NewRequest("GET", "/auth/setup-status", nil)
		req.RemoteAddr = "203.0.113.7:4000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes[rec.Code]++
	}
	if codes[200] != StatusRequestsPerMinute || codes[http.StatusTooManyRequests] != 5 {
		t.Errorf("answers = %v, want %d allowed and 5 stopped", codes, StatusRequestsPerMinute)
	}
	if StatusRequestsPerMinute <= 10 {
		t.Errorf("the limit (%d) must be well above the ten tries of a sign-in", StatusRequestsPerMinute)
	}
}

func TestSetupStatusRateLimit_IsPerClientBehindATrustedProxy(t *testing.T) {
	trusted, _ := middleware.ParseTrustedProxies("172.16.0.0/12")
	h := statusRateLimiter(trusted)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	call := func(client string) int {
		req := httptest.NewRequest("GET", "/auth/setup-status", nil)
		req.RemoteAddr = "172.18.0.10:5000"
		req.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < StatusRequestsPerMinute; i++ {
		call("203.0.113.7")
	}
	if code := call("203.0.113.7"); code != http.StatusTooManyRequests {
		t.Errorf("the client over its budget: %d, want 429", code)
	}
	if code := call("203.0.113.8"); code != http.StatusOK {
		t.Errorf("another client behind the same proxy: %d, want 200", code)
	}
}

func TestRouter_SetupStatusDoesNotSpendTheTriesOfASignIn(t *testing.T) {
	h := testRouter(t)
	from := "203.0.113.50:4000"
	do := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = from
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// Thirty loads of the page (the test router has no database, so the answer itself is an error, but never a refusal by the limit).
	for i := 0; i < 30; i++ {
		if code := do("GET", "/auth/setup-status", ""); code == http.StatusTooManyRequests {
			t.Fatalf("load %d was refused by the limit", i+1)
		}
	}
	// And the sign-in still has its ten tries (a body that is not JSON answers 400 before any database), then the eleventh is refused.
	for i := 0; i < 10; i++ {
		if code := do("POST", "/auth/login", "x"); code != http.StatusBadRequest {
			t.Fatalf("sign-in try %d: %d, want 400 (not refused)", i+1, code)
		}
	}
	if code := do("POST", "/auth/login", "x"); code != http.StatusTooManyRequests {
		t.Errorf("the eleventh sign-in try: %d, want 429", code)
	}
}

// Someone who is stopped by the limit on sign-ins goes into the record of sign-ins (DEC-121): it is exactly what the
// owner wants to see. The answer is the same as before, and a run of refusals is one row with a count.
func TestAuthRateLimiter_RecordsWhoWasStopped(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	rec := &logins.Recorder{DB: db}
	h := authRateLimiter(nil, rec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	codes := map[int]int{}
	var last *httptest.ResponseRecorder
	for i := 0; i < 13; i++ {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = "203.0.113.7:4000"
		req.Header.Set("User-Agent", "curl/8")
		last = httptest.NewRecorder()
		h.ServeHTTP(last, req)
		codes[last.Code]++
	}
	if codes[200] != 10 || codes[http.StatusTooManyRequests] != 3 {
		t.Fatalf("answers = %v, want 10 allowed and 3 stopped", codes)
	}
	if body := strings.TrimSpace(last.Body.String()); body != "Too Many Requests" {
		t.Errorf("the answer changed: %q", body)
	}
	var result, method, ip string
	var count, rows int
	db.QueryRow(`SELECT count(*) FROM login_events`).Scan(&rows)
	db.QueryRow(`SELECT result, method, ip, count FROM login_events`).Scan(&result, &method, &ip, &count)
	if rows != 1 || result != "rate_limited" || method != "local" || ip != "203.0.113.7" || count != 3 {
		t.Errorf("record: %d rows, %s/%s %s x%d; want one row rate_limited/local 203.0.113.7 x3", rows, result, method, ip, count)
	}
	// A nil recorder (a router with none) still limits.
	h2 := authRateLimiter(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = "198.51.100.1:4000"
		out := httptest.NewRecorder()
		h2.ServeHTTP(out, req)
		if i == 10 && out.Code != http.StatusTooManyRequests {
			t.Errorf("without a recorder the 11th: %d", out.Code)
		}
	}
}

// The gate of the heavy reads stands in front of the four reads the screens repeat while a library is being imported, and in
// front of nothing else: the sign-in, the health check, the sheet of a work and the saving of a reading position must never
// wait in the line of the list.
func TestCatalogGate_StandsInFrontOfTheHeavyReadsAndNothingElse(t *testing.T) {
	const gated = http.StatusTeapot
	h := testRouterWith(t, func(d *routerDeps) {
		d.CatalogGate = func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(gated) })
		}
	})
	token := tokenFor(t, "reader")
	for _, r := range []route{{"GET", "/works"}, {"GET", "/search?q=a"}, {"GET", "/favorites"}, {"GET", "/stats"}} {
		if rec := do(h, r.method, r.path, "", ""); rec.Code != gated {
			t.Errorf("%s %s (anonymous) = %d, want it stopped by the gate (before the authentication)", r.method, r.path, rec.Code)
		}
	}
	for _, r := range []route{
		{"GET", "/healthz"}, {"GET", "/auth/me"}, {"POST", "/auth/login"}, {"GET", "/works/1"},
		{"PUT", "/progress/files/1"}, {"POST", "/works/1/reading-heartbeat"}, {"GET", "/dictionary/languages"}, {"GET", "/dictionary/others"},
	} {
		if rec := do(h, r.method, r.path, token, "{}"); rec.Code == gated {
			t.Errorf("%s %s went through the gate of the catalog", r.method, r.path)
		}
	}
}

func TestCatalogGate_AbsentMeansNoLimit(t *testing.T) {
	h := testRouter(t)
	if rec := do(h, "GET", "/stats", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /stats without a gate and without a session = %d, want 401", rec.Code)
	}
}

// The lists of a person are for anyone signed in, the staff or not (#207): a reader reaches the handlers, and nobody who is
// not signed in does.
var personalCollectionRoutes = []route{
	{"POST", "/my/collections"},
	{"PATCH", "/my/collections/1"},
	{"DELETE", "/my/collections/1"},
	{"POST", "/my/collections/1/restore"},
	{"PUT", "/my/collections/1/works/1"},
	{"DELETE", "/my/collections/1/entries/1"},
	{"PUT", "/my/collections/1/order"},
	{"POST", "/collections/1/favorite"},
	{"DELETE", "/collections/1/favorite"},
}

func TestPersonalCollectionRoutes_AnyoneSignedInReachesTheHandlerAndNobodyElse(t *testing.T) {
	h := testRouter(t)
	for _, rt := range personalCollectionRoutes {
		if rec := do(h, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: got %d, want 401", rt.method, rt.path, rec.Code)
		}
		for _, role := range []string{"reader", "admin", "owner"} {
			// Past the middleware the handler answers for itself (the body is empty, or the list is not there), never 401 or 403.
			if rec := do(h, rt.method, rt.path, tokenFor(t, role), ""); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want the handler to answer", rt.method, rt.path, role, rec.Code)
			}
		}
	}
}
