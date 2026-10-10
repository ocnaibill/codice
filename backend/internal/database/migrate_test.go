package database_test

import (
	"database/sql"
	"fmt"
	"strings"
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

// The origin of a suggestion names every provider that took part ("OpenLibrary + Wikidata + Wikipedia"), which did not fit in the
// 32 characters of the column: the analysis of the file failed on the insert (DEC-141).
func TestMigrate_TheOriginOfASuggestionHasRoomForEveryProviderThatTookPart(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 61); err != nil {
		t.Fatal(err)
	}
	work, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "A", Path: "a.epub", Format: "epub"})
	label := "OpenLibrary + Wikidata + Wikipedia"
	insert := `INSERT INTO metadata_candidates (work_id, field, value, source) VALUES (` + itoa(work) + `, 'title', 'x', $1)`
	if _, err := db.Exec(insert, label); err == nil {
		t.Fatal("the label fitted before the migration: the test proves nothing")
	}
	if _, err := db.Exec(insert, "OpenLibrary + Wikidata"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{insert, `INSERT INTO work_field_sources (work_id, field, source) VALUES (` + itoa(work) + `, 'title', $1)`} {
		if _, err := db.Exec(q, label); err != nil {
			t.Errorf("%s: %v", q, err)
		}
		if _, err := db.Exec(q, strings.Repeat("x", 65)); err == nil {
			t.Errorf("a label of 65 characters was accepted: %s", q)
		}
	}
	// Going back cuts the labels that do not fit (the suggestion stays), and leaves alone what fitted.
	if err := database.RollbackTo(db, 61); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var sources string
	db.QueryRow(`SELECT string_agg(source, '|' ORDER BY source) FROM metadata_candidates`).Scan(&sources)
	if sources != "OpenLibrary + Wikidata|"+label[:32] {
		t.Errorf("after going back the candidates are %q: the long label is cut and the short one stays", sources)
	}
	db.QueryRow(`SELECT source FROM work_field_sources WHERE work_id = ` + itoa(work)).Scan(&sources)
	if sources != label[:32] {
		t.Errorf("the origin of a field was %q, want it cut to %q", sources, label[:32])
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}

// How each provider answered the last time is kept one row per provider (DEC-144), and only a state the page knows how to say is accepted.
func TestMigrate_TheHealthOfAProviderIsOneRowWithAKnownState(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO provider_health (provider, state, status) VALUES ($1, $2, 200)`
	for _, state := range []string{"ok", "key", "quota", "down", "error"} {
		if _, err := db.Exec(insert, "p-"+state, state); err != nil {
			t.Errorf("%s: %v", state, err)
		}
	}
	if _, err := db.Exec(insert, "p-bad", "fine"); err == nil {
		t.Error("a state the page does not know was accepted")
	}
	if _, err := db.Exec(insert, "p-ok", "ok"); err == nil {
		t.Error("a provider was given two rows")
	}
	var streak int
	var problem string
	if err := db.QueryRow(`SELECT empty_streak, problem FROM provider_health WHERE provider = 'p-ok'`).Scan(&streak, &problem); err != nil || streak != 0 || problem != "" {
		t.Errorf("defaults: %d %q %v", streak, problem, err)
	}
	if err := database.RollbackTo(db, 62); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if tableExists(t, db, "provider_health") {
		t.Error("the table stayed after going back")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if !tableExists(t, db, "provider_health") {
		t.Error("the table did not come back")
	}
}

// The profile of an author (DEC-146) is one row per person, goes with the person, and its biography is in a state the page knows.
func TestMigrate_TheProfileOfAPersonIsOneRowThatGoesWithThePerson(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var person int
	if err := db.QueryRow(`INSERT INTO person (name) VALUES ('Frank Herbert') RETURNING id`).Scan(&person); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO person_profile (person_id, wikidata_id) VALUES ($1, 'Q7934')`, person); err != nil {
		t.Fatal(err)
	}
	var state string
	var hidden, imageHidden bool
	if err := db.QueryRow(`SELECT bio_state, hidden, image_hidden FROM person_profile WHERE person_id = $1`, person).Scan(&state, &hidden, &imageHidden); err != nil ||
		state != "pending" || hidden || imageHidden {
		t.Errorf("defaults: %q %v %v %v", state, hidden, imageHidden, err)
	}
	if _, err := db.Exec(`INSERT INTO person_profile (person_id, wikidata_id) VALUES ($1, 'Q7934')`, person); err == nil {
		t.Error("a person was given two profiles")
	}
	var other int
	db.QueryRow(`INSERT INTO person (name) VALUES ('Outro') RETURNING id`).Scan(&other)
	if _, err := db.Exec(`INSERT INTO person_profile (person_id, wikidata_id, bio_state) VALUES ($1, 'Q1', 'maybe')`, other); err == nil {
		t.Error("a state the page does not know was accepted")
	}
	if _, err := db.Exec(`DELETE FROM person WHERE id = $1`, person); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM person_profile`).Scan(&n)
	if n != 0 {
		t.Errorf("the profile of a person who is gone stayed: %d", n)
	}
	if err := database.RollbackTo(db, 63); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if tableExists(t, db, "person_profile") {
		t.Error("the table stayed after going back")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("reapply: %v", err)
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

// A person keeps the surname and the given names only when it is certain which is which: the catalogue's
// way with a role, found in the aliases (#64).
func TestMigrate_NamePartsAreLearnedOnlyFromWhatIsCertain(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 28); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, aliases ...string) int {
		var id int
		if err := db.QueryRow(`INSERT INTO person (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for _, a := range aliases {
			if _, err := db.Exec(`INSERT INTO person_alias (person_id, alias) VALUES ($1, $2)`, id, a); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	herbert := mk("Frank Herbert", "Herbert, Frank, author")
	paren := mk("Gabriel García Márquez", "García Márquez, Gabriel (autor)")
	noRole := mk("Brian Herbert", "Herbert, Brian")           // a comma but no role: not certain
	plain := mk("Ursula K. Le Guin")                          // no alias at all
	wrong := mk("Isaac Asimov", "Clarke, Arthur, author")     // an alias that is not this person's name
	roleOnly := mk("Jules Verne", "Jules Verne, author")      // a role, but nothing says the surname
	list := mk("Plato", "Plato, Aristotle, Socrates, author") // more than one person
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]string{
		herbert: "Herbert|Frank", paren: "García Márquez|Gabriel",
		noRole: "|", plain: "|", wrong: "|", roleOnly: "|", list: "|",
	} {
		var family, given sql.NullString
		db.QueryRow(`SELECT family_name, given_name FROM person WHERE id = $1`, id).Scan(&family, &given)
		if got := family.String + "|" + given.String; got != want {
			t.Errorf("person %d: %q, want %q", id, got, want)
		}
	}
	if _, err := db.Exec(`UPDATE person SET family_name = NULL, given_name = 'X' WHERE id = $1`, plain); err == nil {
		t.Error("given names with no surname were accepted")
	}
	if _, err := db.Exec(`UPDATE users SET name_order = 'whatever'`); err == nil {
		// no users exist, so this cannot fail on the value: check the constraint directly
		var n int
		db.QueryRow(`SELECT count(*) FROM pg_constraint WHERE conrelid = 'users'::regclass AND pg_get_constraintdef(oid) LIKE '%family_first%'`).Scan(&n)
		if n != 1 {
			t.Error("name_order has no constraint on its values")
		}
	}
}

func TestMigrate_PersonAuthorityRollsBackAndReappliesAndDeletingAPersonTakesItsKeys(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO person (id, name) VALUES (1, 'Frank Herbert'), (2, 'Frank P. Herbert')`,
		`INSERT INTO person_authority (person_id, scheme, value) VALUES (1, 'openlibrary', 'OL1A'), (2, 'openlibrary', 'OL1A')`,
		`INSERT INTO person_merge_candidates (person_a, person_b, reason, evidence) VALUES (1, 2, 'authority', '{"scheme":"openlibrary"}')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO person_merge_candidates (person_a, person_b, reason) VALUES (1, 3, 'guess')`); err == nil {
		t.Error("the schema accepted a reason nobody defined")
	}
	if _, err := db.Exec(`DELETE FROM person WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	var keys, pairs int
	db.QueryRow(`SELECT count(*) FROM person_authority`).Scan(&keys)
	db.QueryRow(`SELECT count(*) FROM person_merge_candidates`).Scan(&pairs)
	if keys != 1 || pairs != 0 {
		t.Errorf("after deleting a person: keys = %d, pairs = %d, want 1 and 0", keys, pairs)
	}
	if err := database.RollbackTo(db, 29); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if tableExists(t, db, "person_authority") {
		t.Error("the table is still there after rolling back")
	}
	if _, err := db.Exec(`SELECT reason FROM person_merge_candidates`); err == nil {
		t.Error("the reason column is still there after rolling back")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}

func TestMigrate_LanguageSuggestionsOfProvidersAreRejectedAndTheDetectedOnesAreNot(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	w, _, _ := testdb.AddWork(t, db, testdb.Work{Title: "A", Path: "a.epub", Format: "epub"})
	if err := database.RollbackTo(db, 30); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO metadata_candidates (work_id, field, value, source, state) VALUES
			(` + itoa(w) + `, 'language', 'cat', 'OpenLibrary', 'pending'),
			(` + itoa(w) + `, 'language', 'pt', 'Google Books', 'pending'),
			(` + itoa(w) + `, 'language', 'en', 'detected', 'pending'),
			(` + itoa(w) + `, 'language', 'fr', 'OpenLibrary', 'accepted'),
			(` + itoa(w) + `, 'publisher', 'Aleph', 'OpenLibrary', 'pending')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rows, err := db.Query(`SELECT field || '/' || value || '/' || state FROM metadata_candidates ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		got = append(got, s)
	}
	want := []string{"language/cat/rejected", "language/pt/rejected", "language/en/pending", "language/fr/accepted", "publisher/Aleph/pending"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("states = %v, want %v", got, want)
	}
	if err := database.RollbackTo(db, 30); err != nil {
		t.Errorf("rolling the migration back: %v", err)
	}
}

// "The editions of a work" is asked for every card of the catalog and for every file imported; without an index on
// editions.work_id each of those reads the whole table (found with 10 000 works: 4.2 s for the last page of the library).
func TestMigrate_EditionsAreIndexedByWork(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	hasIndex := func() bool {
		var ok bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_index i
				JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
				WHERE i.indrelid = 'editions'::regclass AND a.attname = 'work_id' AND i.indpred IS NULL)`).Scan(&ok)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !hasIndex() {
		t.Fatal("no general index on editions(work_id)")
	}
	// It rolls back and applies again like the others.
	if err := database.RollbackTo(db, 49); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if hasIndex() {
		t.Error("the index stayed after rolling the migration back")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if !hasIndex() {
		t.Error("the index did not come back when the migration was applied again")
	}
}
