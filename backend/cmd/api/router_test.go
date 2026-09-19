package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ocnaibill/codice/backend/internal/handlers"
)

const testSecret = "test_secret_key_for_testing_12345678"

const someUUID = "b190281f-fe3e-4308-ad71-91e24744d7a0"

// testRouter builds the real route table with no database or Redis: every
// request in these tests must be stopped (or fail validation) before a
// handler touches them.
func testRouter(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("JWT_SECRET", testSecret)
	return newRouter(routerDeps{
		WS:          &handlers.WsHandler{},
		StoragePath: t.TempDir(),
	})
}

func tokenFor(t *testing.T, role string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  someUUID,
		"role": role,
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(testSecret))
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

func TestReaderStillReachesReadingRoutes(t *testing.T) {
	// Sanity check that the guard is scoped to administration: a reader is
	// not blocked by role on the personal-data routes. Routes that would hit
	// the (absent) database are exercised only up to the auth layer, so we
	// assert on what the middleware chain alone decides.
	h := testRouter(t)
	rec := do(h, "GET", "/files/does-not-exist.epub", tokenFor(t, "reader"), "")
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Errorf("reader on /files: got %d, want the request to pass auth", rec.Code)
	}
}
