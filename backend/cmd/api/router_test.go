package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/ocnaibill/codice/backend/internal/middleware"
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
	t.Setenv("JWT_SECRET", testSecret)
	auth := middleware.Authenticator{Sessions: fakeSessions}
	notFound := func(ctx context.Context, rel string) (handlers.FileAccess, error) {
		return handlers.FileAccess{}, handlers.ErrFileNotFound
	}
	return newRouter(routerDeps{
		Auth:        auth,
		WS:          &handlers.WsHandler{Auth: auth},
		StoragePath: t.TempDir(),
		// Every path is unknown: enough to tell "past authentication" (404)
		// from "refused" (401), without a database.
		FileLookup:  notFound,
		CoverLookup: func(ctx context.Context, name string) (handlers.FileAccess, error) { return handlers.FileAccess{}, nil },
	})
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
	{"POST", "/upload"},
	{"POST", "/works/bulk-import"},
	{"PUT", "/works/1"},
	{"DELETE", "/works/1"},
	{"POST", "/works/1/restore"},
	{"GET", "/admin/jobs"},
	{"GET", "/admin/storage/reorganize"},
	{"POST", "/admin/storage/reorganize"},
	{"POST", "/admin/jobs/1/rerun"},
	{"POST", "/admin/jobs/1/cancel"},
	{"GET", "/works/1/candidates"},
	{"POST", "/works/1/candidates/1/accept"},
	{"POST", "/works/1/candidates/1/reject"},
}

// Routes reserved to the owner alone (DEC-056).
var ownerRoutes = []route{
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
		{"POST", "/auth/logout"},
		{"POST", "/auth/resource-token"},
		{"POST", "/auth/app-tokens"},
		{"GET", "/auth/app-tokens"},
		{"DELETE", "/auth/app-tokens/" + someUUID},
		{"GET", "/works"},
		{"GET", "/notes"},
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
