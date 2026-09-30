package database_test

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/people"
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

// The names that were stored before #36 are fixed by the migration with the same rule as
// people.NormalizeName, and what they were is kept as an alias.
func TestMigrate_StoredAuthorNamesAreFixedLikePeopleNormalizeName(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 25); err != nil {
		t.Fatal(err)
	}
	inputs := []string{
		"Herbert, Frank, author", "Herbert, Frank, Author", "  Herbert ,  Frank ,  author  ", "Herbert, Frank, author.",
		"García Márquez, Gabriel, autor", "Saint-Exupéry, Antoine de, auteur", "Schoenherr, John, illustrator",
		"Macedo, Henrique de, tradutor", "Brown, Dan, éditeur", "Frank Herbert, author", "Frank Herbert (author)", "Herbert, Frank (author)",
		"Frank Herbert", "Herbert, Frank", "Plato, Aristotle", "Frank Herbert, Brian Herbert",
		"Herbert, Frank, Schoenherr, John, author", "King, Martin Luther, Jr., author", "author",
	}
	ids := map[string]int{}
	for _, in := range inputs {
		var id int
		if err := db.QueryRow(`INSERT INTO person (name) VALUES ($1) RETURNING id`, in).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[in] = id
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, in := range inputs {
		want, _ := people.NormalizeName(in)
		if want == "Frank Herbert" && in != "Frank Herbert" {
			continue // the spellings of a person that already exists go into it: checked below
		}
		var got string
		if err := db.QueryRow(`SELECT name FROM person WHERE id = $1`, ids[in]).Scan(&got); err != nil {
			t.Errorf("%q: the person is gone: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q became %q, want %q", in, got, want)
		}
		var aliases int
		db.QueryRow(`SELECT count(*) FROM person_alias WHERE person_id = $1 AND alias = $2`, ids[in], in).Scan(&aliases)
		if (want != in) != (aliases == 1) {
			t.Errorf("%q: alias count %d, name changed = %v", in, aliases, want != in)
		}
	}
	// Eight inputs are Frank Herbert: one person, with every other spelling as an alias.
	var people_, aliases int
	db.QueryRow(`SELECT count(*) FROM person WHERE name = 'Frank Herbert'`).Scan(&people_)
	if people_ != 1 {
		t.Fatalf("persons called Frank Herbert = %d, want 1", people_)
	}
	db.QueryRow(`SELECT count(*) FROM person_alias a JOIN person p ON p.id = a.person_id WHERE p.name = 'Frank Herbert'`).Scan(&aliases)
	if aliases != 7 { // the spellings that are not "Frank Herbert" itself
		t.Errorf("aliases of Frank Herbert = %d, want 7", aliases)
	}
}

func TestMigrate_AWorkThatHadBothSpellingsKeepsOneAuthor(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 25); err != nil {
		t.Fatal(err)
	}
	a, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "Dune", Path: "a.epub", Format: "epub", Author: "Herbert, Frank, author"})
	b, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "Duna", Path: "b.epub", Format: "epub", Author: "Frank Herbert"})
	var kept int
	db.QueryRow(`SELECT id FROM person WHERE name = 'Frank Herbert'`).Scan(&kept)
	if _, err := db.Exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'author', 1 FROM person WHERE name = 'Herbert, Frank, author'`, b); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM work_contributors WHERE work_id = $1 AND person_id = $2`, b, kept).Scan(&n)
	if n != 1 {
		t.Errorf("the work that had both spellings has %d author rows, want 1", n)
	}
	db.QueryRow(`SELECT count(*) FROM work_contributors WHERE work_id = $1 AND person_id = $2`, a, kept).Scan(&n)
	if n != 1 {
		t.Errorf("the other work's author = %d, want 1", n)
	}
	db.QueryRow(`SELECT count(*) FROM person WHERE name LIKE '%author%'`).Scan(&n)
	if n != 0 {
		t.Errorf("persons left with a role in their name: %d", n)
	}
}
