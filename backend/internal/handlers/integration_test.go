package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

// Integration tests need a real PostgreSQL: see testdb.Open.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func migratedDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return db
}

func TestMigrations_LegacyAdminBecomesOwner_AndIsIdempotent(t *testing.T) {
	db := testDB(t)

	// Legacy schema as shipped before roles: no CHECK, no owner index.
	if _, err := db.Exec(`
		CREATE TABLE users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username VARCHAR(50) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255),
			sso_id VARCHAR(255) UNIQUE,
			role VARCHAR(20) DEFAULT 'reader',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO users (username, email, role, created_at) VALUES
			('first_admin',  'a@x', 'admin',  '2026-01-01'),
			('second_admin', 'b@x', 'admin',  '2026-02-01'),
			('reader',       'c@x', 'reader', '2026-03-01');`); err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if err := database.Migrate(db); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
		roles := map[string]string{}
		rows, err := db.Query(`SELECT username, role FROM users`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var u, r string
			rows.Scan(&u, &r)
			roles[u] = r
		}
		rows.Close()
		if roles["first_admin"] != "owner" || roles["second_admin"] != "admin" || roles["reader"] != "reader" {
			t.Fatalf("run %d: unexpected roles %v", run, roles)
		}
	}

	// The database itself must refuse a second owner and unknown roles.
	if _, err := db.Exec(`INSERT INTO users (username, email, role) VALUES ('x','x@x','owner')`); err == nil {
		t.Error("a second owner was accepted; the unique index is missing")
	}
	if _, err := db.Exec(`INSERT INTO users (username, email, role) VALUES ('y','y@x','root')`); err == nil {
		t.Error("an unknown role was accepted; the CHECK constraint is missing")
	}
}

func TestSetup_ConcurrentRequestsCreateExactlyOneOwner(t *testing.T) {
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	db := migratedDB(t)
	h := &AuthHandler{DB: db, Sessions: &sessions.Store{DB: db}}

	const n = 12
	codes := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"username":"owner%d","password":"pw-%d"}`, i, i)
			req := httptest.NewRequest("POST", "/auth/setup", strings.NewReader(body))
			rec := httptest.NewRecorder()
			<-start
			h.SetupMasterAdmin(rec, req)
			codes[i] = rec.Code
		}(i)
	}
	close(start)
	wg.Wait()

	created, forbidden := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusForbidden:
			forbidden++
		}
	}
	if created != 1 || forbidden != n-1 {
		t.Errorf("want 1 created and %d forbidden, got codes %v", n-1, codes)
	}

	var owners, total int
	db.QueryRow(`SELECT COUNT(*) FILTER (WHERE role = 'owner'), COUNT(*) FROM users`).Scan(&owners, &total)
	if owners != 1 || total != 1 {
		t.Errorf("want exactly 1 user who is the owner, got owners=%d total=%d", owners, total)
	}
}

func TestSetup_CreatesOwnerRoleInToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	db := migratedDB(t)
	h := &AuthHandler{DB: db, Sessions: &sessions.Store{DB: db}}

	req := httptest.NewRequest("POST", "/auth/setup", strings.NewReader(`{"username":"boss","password":"pw"}`))
	rec := httptest.NewRecorder()
	h.SetupMasterAdmin(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: got %d: %s", rec.Code, rec.Body.String())
	}

	var role string
	db.QueryRow(`SELECT role FROM users WHERE username = 'boss'`).Scan(&role)
	if role != "owner" {
		t.Errorf("setup created role %q, want owner", role)
	}
	// Second setup is refused.
	rec = httptest.NewRecorder()
	h.SetupMasterAdmin(rec, httptest.NewRequest("POST", "/auth/setup", strings.NewReader(`{"username":"late","password":"pw"}`)))
	if rec.Code != http.StatusForbidden {
		t.Errorf("second setup: got %d, want 403", rec.Code)
	}
}

// roleRouter mounts UpdateRole and authenticates every request as actorID, the
// way AuthMiddleware would, but with a token role that the test controls.
func roleRouter(db *sql.DB, actorID, tokenRole string) http.Handler {
	h := &UsersHandler{DB: db}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, actorID)
			ctx = context.WithValue(ctx, middleware.UserRoleKey, tokenRole)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Put("/users/{id}/role", h.UpdateRole)
	return r
}

func addUser(t *testing.T, db *sql.DB, name, role string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(
		`INSERT INTO users (username, email, role) VALUES ($1, $2, $3) RETURNING id`,
		name, name+"@x", role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func roleOf(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var role string
	db.QueryRow(`SELECT role FROM users WHERE id = $1`, id).Scan(&role)
	return role
}

func putRole(h http.Handler, targetID, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PUT", "/users/"+targetID+"/role", strings.NewReader(fmt.Sprintf(`{"role":%q}`, role)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUpdateRole_Policy(t *testing.T) {
	db := migratedDB(t)
	owner := addUser(t, db, "owner", "owner")
	admin1 := addUser(t, db, "admin1", "admin")
	admin2 := addUser(t, db, "admin2", "admin")
	reader := addUser(t, db, "reader", "reader")

	// Owner promotes a reader, then demotes that admin.
	h := roleRouter(db, owner, "owner")
	if rec := putRole(h, reader, "admin"); rec.Code != 200 || roleOf(t, db, reader) != "admin" {
		t.Fatalf("owner promote: code=%d role=%s", rec.Code, roleOf(t, db, reader))
	}
	if rec := putRole(h, reader, "reader"); rec.Code != 200 || roleOf(t, db, reader) != "reader" {
		t.Fatalf("owner demote: code=%d role=%s", rec.Code, roleOf(t, db, reader))
	}

	// Owner cannot assign owner, and cannot change themself.
	if rec := putRole(h, reader, "owner"); rec.Code != http.StatusBadRequest {
		t.Errorf("assign owner: got %d, want 400", rec.Code)
	}
	if rec := putRole(h, owner, "reader"); rec.Code != http.StatusForbidden || roleOf(t, db, owner) != "owner" {
		t.Errorf("owner demoting self: code=%d role=%s", rec.Code, roleOf(t, db, owner))
	}

	// RN-022: an admin cannot promote a reader nor touch another admin or the owner.
	ha := roleRouter(db, admin1, "admin")
	for name, tc := range map[string]struct{ target, role string }{
		"promote reader":   {reader, "admin"},
		"demote admin":     {admin2, "reader"},
		"demote owner":     {owner, "reader"},
		"demote self":      {admin1, "reader"},
		"re-promote admin": {admin2, "admin"},
	} {
		if rec := putRole(ha, tc.target, tc.role); rec.Code != http.StatusForbidden {
			t.Errorf("admin %s: got %d, want 403", name, rec.Code)
		}
	}
	if roleOf(t, db, reader) != "reader" || roleOf(t, db, admin2) != "admin" || roleOf(t, db, owner) != "owner" {
		t.Error("an admin managed to change someone's role")
	}

	// A stale token claiming owner does not help an account that is now admin.
	hs := roleRouter(db, admin1, "owner")
	if rec := putRole(hs, reader, "admin"); rec.Code != http.StatusForbidden {
		t.Errorf("stale owner token: got %d, want 403 (role must come from the database)", rec.Code)
	}

	// Unknown and malformed targets.
	if rec := putRole(h, "b190281f-fe3e-4308-ad71-000000000000", "admin"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown user: got %d, want 404", rec.Code)
	}
	if rec := putRole(h, "not-a-uuid", "admin"); rec.Code != http.StatusNotFound {
		t.Errorf("malformed id: got %d, want 404", rec.Code)
	}

	var body map[string]string
	rec := putRole(h, reader, "admin")
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["role"] != "admin" {
		t.Errorf("response body: %v", body)
	}
}
