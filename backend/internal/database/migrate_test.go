package database_test

import (
	"database/sql"
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
