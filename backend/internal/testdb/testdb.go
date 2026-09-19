// Package testdb opens the PostgreSQL used by integration tests.
package testdb

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// lockKey serialises tests across packages: `go test ./...` runs packages in
// parallel, and each test drops the schema, so two of them at once would wipe
// each other's tables.
const lockKey = 7301199

// Open returns a clean database for the test. It needs TEST_DATABASE_URL
// pointing at a database whose name ends in "_test": its public schema is
// dropped and recreated first. Without the variable the test is skipped. The
// database is held exclusively (an advisory lock) until the test ends.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(u.Path, "/"), "_test") {
		t.Fatalf("refusing to run: TEST_DATABASE_URL database name must end in _test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(20)

	lock, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		lock.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey)
		lock.Close()
	})

	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	return db
}
