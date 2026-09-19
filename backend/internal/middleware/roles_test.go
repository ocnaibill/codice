package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveWithIdentity(mw func(http.Handler) http.Handler, id, role string) (code int, ran bool) {
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true }))
	req := httptest.NewRequest("GET", "/x", nil)
	if id != "" || role != "" {
		ctx := context.WithValue(req.Context(), UserIDKey, id)
		ctx = context.WithValue(ctx, UserRoleKey, role)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, ran
}

func TestRequireStaff(t *testing.T) {
	cases := []struct {
		id, role string
		wantCode int
		wantRan  bool
	}{
		{"u1", "owner", 200, true},
		{"u1", "admin", 200, true},
		{"u1", "reader", 403, false},
		{"u1", "", 403, false},
		{"u1", "root", 403, false},
		{"", "", 401, false},
		{"", "admin", 401, false},
	}
	for _, c := range cases {
		code, ran := serveWithIdentity(RequireStaff, c.id, c.role)
		if code != c.wantCode || ran != c.wantRan {
			t.Errorf("RequireStaff(id=%q, role=%q): code=%d ran=%v, want code=%d ran=%v", c.id, c.role, code, ran, c.wantCode, c.wantRan)
		}
	}
}

func TestRequireOwner(t *testing.T) {
	cases := []struct {
		id, role string
		wantCode int
		wantRan  bool
	}{
		{"u1", "owner", 200, true},
		{"u1", "admin", 403, false},
		{"u1", "reader", 403, false},
		{"", "owner", 401, false},
		{"", "", 401, false},
	}
	for _, c := range cases {
		code, ran := serveWithIdentity(RequireOwner, c.id, c.role)
		if code != c.wantCode || ran != c.wantRan {
			t.Errorf("RequireOwner(id=%q, role=%q): code=%d ran=%v, want code=%d ran=%v", c.id, c.role, code, ran, c.wantCode, c.wantRan)
		}
	}
}
