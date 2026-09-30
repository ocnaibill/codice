package database_test

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var ok bool
	if err := db.QueryRow(`SELECT to_regclass('public.' || $1) IS NOT NULL`, name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestMigrate_FreshDatabaseReachesTheLatestVersion(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"works", "editions", "users", "sessions", "app_tokens", "notes", "goose_db_version"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %s missing after migrating a fresh database", table)
		}
	}
	v, err := database.Version(db)
	if err != nil || v < 1 {
		t.Errorf("version = %d, err = %v", v, err)
	}
	// Running again is a no-op.
	if err := database.Migrate(db); err != nil {
		t.Errorf("second run: %v", err)
	}
}

func TestMigrate_AdoptsADatabaseCreatedBeforeGoose(t *testing.T) {
	db := testdb.Open(t)
	// The former startup routine created these tables with no version record.
	if err := database.MigrateTo(db, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO person (name) VALUES ('Frank Herbert');
		INSERT INTO works (original_title, file_path, author_id) VALUES ('Duna', 'duna.epub', 1);
		DROP TABLE goose_db_version;`); err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(db); err != nil {
		t.Fatalf("adopting an existing database failed: %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM works WHERE original_title = 'Duna'`).Scan(&n)
	if n != 1 {
		t.Errorf("existing data was lost while adopting the database: %d rows", n)
	}
}

func TestMigrate_ConcurrentInstancesDoNotRace(t *testing.T) {
	db := testdb.Open(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = database.Migrate(db)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("instance %d: %v", i, err)
		}
	}
}

// Joining files by hand (migration 00025) can be rolled back and applied again, and what it added is
// taken away without touching the pairs the system itself proposed.
func TestMigrate_JoiningVersionsCanBeRolledBackAndReapplied(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	a, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "A", Path: "a.epub", Format: "epub"})
	b, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "B", Path: "b.epub", Format: "epub"})
	c, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "C", Path: "c.epub", Format: "epub"})
	for _, q := range []string{
		`INSERT INTO duplicate_candidates (work_a, work_b, reason, state) VALUES (` + itoa(a) + `, ` + itoa(b) + `, 'manual', 'dismissed')`,
		`INSERT INTO duplicate_candidates (work_a, work_b, reason, state) VALUES (` + itoa(a) + `, ` + itoa(c) + `, 'isbn', 'pending')`,
		`UPDATE editions SET former_work_id = ` + itoa(b) + ` WHERE work_id = ` + itoa(a),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := database.RollbackTo(db, 24); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM duplicate_candidates`).Scan(&n)
	if n != 1 {
		t.Errorf("after rolling back, pairs = %d, want only the one the system proposed", n)
	}
	if _, err := db.Exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES (` + itoa(b) + `, ` + itoa(c) + `, 'manual')`); err == nil {
		t.Error("a manual pair was accepted by the schema before the migration")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason, state) VALUES (` + itoa(b) + `, ` + itoa(c) + `, 'manual', 'linked')`); err != nil {
		t.Errorf("after reapplying: %v", err)
	}
}

func itoa(n int) string { return fmt.Sprint(n) }
