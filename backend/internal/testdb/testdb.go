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

// Work describes a work to seed. Everything but Title is optional: with a Path it
// also gets a file at that location, with an Author a contributor. ID and
// AuthorID fix the primary keys when a test needs to name them.
type Work struct {
	ID        int
	Title     string
	Path      string
	Format    string
	Author    string
	AuthorID  int
	Language  string
	Publisher string
	Date      string
	ISBN      string
}

// AddWork inserts a work with its primary edition, file, managed location and
// author, the way the application records them. It returns their ids (the file
// id is 0 when there is no Path).
func AddWork(t testing.TB, db *sql.DB, w Work) (workID, editionID int, fileID int64) {
	t.Helper()
	nullable := func(s string) any { return sql.NullString{String: s, Valid: s != ""} }
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	if w.ID > 0 {
		must(db.QueryRow(`INSERT INTO works (id, original_title) VALUES ($1, $2) RETURNING id`, w.ID, w.Title).Scan(&workID))
	} else {
		must(db.QueryRow(`INSERT INTO works (original_title) VALUES ($1) RETURNING id`, w.Title).Scan(&workID))
	}
	must(db.QueryRow(`
		INSERT INTO editions (work_id, title, language, publisher, publication_date, isbn, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE) RETURNING id`,
		workID, w.Title, nullable(w.Language), nullable(w.Publisher), nullable(w.Date), nullable(w.ISBN)).Scan(&editionID))

	if w.Path != "" {
		must(db.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, $2) RETURNING id`, editionID, nullable(w.Format)).Scan(&fileID))
		_, err := db.Exec(`INSERT INTO storage_locations (file_id, mode, path) VALUES ($1, 'managed', $2)`, fileID, w.Path)
		must(err)
	}

	personID := w.AuthorID
	switch {
	case personID == 0 && w.Author != "":
		err := db.QueryRow(`SELECT id FROM person WHERE name = $1`, w.Author).Scan(&personID)
		if err == sql.ErrNoRows {
			err = db.QueryRow(`INSERT INTO person (name) VALUES ($1) RETURNING id`, w.Author).Scan(&personID)
		}
		must(err)
	}
	if personID != 0 {
		_, err := db.Exec(`INSERT INTO work_contributors (work_id, person_id, role, position) VALUES ($1, $2, 'author', 0)`, workID, personID)
		must(err)
	}
	return workID, editionID, fileID
}
