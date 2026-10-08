package database_test

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

// addSeries inserts a work the way the writers of the catalogue do, with the series as plain text.
func addSeries(t *testing.T, db *sql.DB, title string, series any, index float64) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`INSERT INTO works (original_title, series, series_index) VALUES ($1, $2, $3) RETURNING id`,
		title, series, index).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// officialOf is the id of the official collection a work belongs to, 0 when there is none.
func officialOf(t *testing.T, db *sql.DB, work int) int {
	t.Helper()
	var id int
	err := db.QueryRow(`SELECT collection_id FROM collection_works WHERE work_id = $1 AND official`, work).Scan(&id)
	if err == sql.ErrNoRows {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func positionOf(t *testing.T, db *sql.DB, work int) sql.NullFloat64 {
	t.Helper()
	var p sql.NullFloat64
	if err := db.QueryRow(`SELECT position FROM collection_works WHERE work_id = $1 AND official`, work).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCollections_ASeriesOnAWorkMakesTheCollectionAndTheNextOneJoinsIt(t *testing.T) {
	db := migrated(t)
	a := addSeries(t, db, "Pedra Filosofal", "Harry Potter", 1)
	c := officialOf(t, db, a)
	if c == 0 {
		t.Fatal("a work with a series is in no collection")
	}
	var kind, name, origin string
	var edited, retired sql.NullTime
	db.QueryRow(`SELECT kind, name, origin, edited_at, retired_at FROM collections WHERE id = $1`, c).Scan(&kind, &name, &origin, &edited, &retired)
	if kind != "official" || name != "Harry Potter" || origin != "metadata" || edited.Valid || retired.Valid {
		t.Errorf("collection = %s %q %s edited=%v retired=%v", kind, name, origin, edited, retired)
	}
	if p := positionOf(t, db, a); !p.Valid || p.Float64 != 1 {
		t.Errorf("position = %v, want 1", p)
	}

	// The same series written another way is the same collection: case, accents and runs of spaces are not told apart.
	b := addSeries(t, db, "Câmara Secreta", "  harry   POTTER ", 2)
	messy := addSeries(t, db, "Outra série", "  Duas   Palavras ", 1)
	if n := count(t, db, `SELECT count(*) FROM collections WHERE id = $1 AND name = 'Duas Palavras'`, officialOf(t, db, messy)); n != 1 {
		t.Error("the name of a collection keeps the stray spaces of the text it was born from")
	}
	if officialOf(t, db, b) != c {
		t.Error("another writing of the series made another collection")
	}
	e := addSeries(t, db, "Outro", "Hístória", 1)
	f := addSeries(t, db, "Outro 2", "historia", 2)
	if officialOf(t, db, e) == 0 || officialOf(t, db, e) != officialOf(t, db, f) {
		t.Error("accents told two series apart")
	}
	if n := count(t, db, `SELECT count(*) FROM collections`); n != 3 {
		t.Errorf("collections = %d, want 3", n)
	}
}

func TestCollections_AWorkWithNoSeriesIsInNoCollection(t *testing.T) {
	db := migrated(t)
	for _, s := range []any{nil, "", "   "} {
		w := addSeries(t, db, "Solto", s, 0)
		if officialOf(t, db, w) != 0 {
			t.Errorf("series %#v put a work in a collection", s)
		}
	}
	if n := count(t, db, `SELECT count(*) FROM collections`); n != 0 {
		t.Errorf("collections = %d, want none", n)
	}
}

func TestCollections_ChangingTheSeriesMovesTheWorkAndClearingTakesItOut(t *testing.T) {
	db := migrated(t)
	w := addSeries(t, db, "Obra", "Duna", 1)
	other := addSeries(t, db, "Outra", "Duna", 2)
	first := officialOf(t, db, w)

	db.Exec(`UPDATE works SET series = 'Fundação' WHERE id = $1`, w)
	moved := officialOf(t, db, w)
	if moved == 0 || moved == first {
		t.Fatalf("the work stayed in the old collection (%d, was %d)", moved, first)
	}
	if officialOf(t, db, other) != first {
		t.Error("the other work left the old collection")
	}
	if n := count(t, db, `SELECT count(*) FROM collection_works WHERE work_id = $1`, w); n != 1 {
		t.Errorf("the work is in %d collections", n)
	}

	// A number written later reaches the membership; 0 is "no number".
	db.Exec(`UPDATE works SET series_index = 3.5 WHERE id = $1`, w)
	if p := positionOf(t, db, w); !p.Valid || p.Float64 != 3.5 {
		t.Errorf("position = %v, want 3.5", p)
	}
	db.Exec(`UPDATE works SET series_index = 0 WHERE id = $1`, w)
	if p := positionOf(t, db, w); p.Valid {
		t.Errorf("position = %v, want none for index 0", p)
	}

	db.Exec(`UPDATE works SET series = NULL WHERE id = $1`, w)
	if officialOf(t, db, w) != 0 {
		t.Error("clearing the series left the work in a collection")
	}
	// What is not about the series does not touch the membership.
	db.Exec(`UPDATE works SET series = 'Fundação' WHERE id = $1`, w)
	before := moved
	db.Exec(`UPDATE works SET original_title = 'Novo título' WHERE id = $1`, w)
	if officialOf(t, db, w) != before {
		t.Error("a title change moved the work")
	}
}

func TestCollections_ARenamedCollectionStillTakesTheTextItWasBornFrom(t *testing.T) {
	db := migrated(t)
	a := addSeries(t, db, "Vol 1", "Marvel Civil War", 1)
	c := officialOf(t, db, a)
	// What a rename does (#205): the new name is one more key, and the old one stays.
	db.Exec(`UPDATE collections SET name = 'Guerra Civil', edited_at = now() WHERE id = $1`, c)
	db.Exec(`INSERT INTO collection_keys (key, collection_id) VALUES ('guerra civil', $1)`, c)

	b := addSeries(t, db, "Vol 2", "Marvel Civil War", 2)
	d := addSeries(t, db, "Vol 3", "Guerra Civil", 3)
	if officialOf(t, db, b) != c || officialOf(t, db, d) != c {
		t.Error("a work with the old or the new text did not join the renamed collection")
	}
	if n := count(t, db, `SELECT count(*) FROM collections`); n != 1 {
		t.Errorf("collections = %d, want 1", n)
	}
}

func TestCollections_ARetiredCollectionTakesNoWork(t *testing.T) {
	db := migrated(t)
	a := addSeries(t, db, "Vol 1", "Saga", 1)
	c := officialOf(t, db, a)
	db.Exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, c)

	b := addSeries(t, db, "Vol 2", "Saga", 2)
	if officialOf(t, db, b) != 0 {
		t.Error("a work joined a retired collection")
	}
	if n := count(t, db, `SELECT count(*) FROM collections`); n != 1 {
		t.Errorf("a retired collection was made again: %d collections", n)
	}
	// Touching the series of a work that is in it takes it out: what the text leads to is retired.
	db.Exec(`UPDATE works SET series_index = 9 WHERE id = $1`, a)
	if officialOf(t, db, a) != 0 {
		t.Error("the work stayed in the retired collection")
	}
	// Restoring it and running the sync again brings the works back.
	db.Exec(`UPDATE collections SET retired_at = NULL WHERE id = $1`, c)
	db.Exec(`SELECT collection_sync_work(id, series, series_index) FROM works`)
	if officialOf(t, db, a) != c || officialOf(t, db, b) != c {
		t.Error("the works did not come back with the collection")
	}
}

func TestCollections_OneOfficialCollectionPerWorkAndPersonalOnesAreLeftAlone(t *testing.T) {
	db := migrated(t)
	w := addSeries(t, db, "Obra", "Série A", 1)
	var owner string
	if err := db.QueryRow(`INSERT INTO users (username, email, password_hash) VALUES ('ana', 'ana@example.test', 'x') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var personal int
	if err := db.QueryRow(`INSERT INTO collections (kind, owner_id, name) VALUES ('personal', $1, 'Para ler') RETURNING id`, owner).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, personal, w)

	// A second official one is refused by the database.
	var other int
	db.QueryRow(`INSERT INTO collections (kind, name) VALUES ('official', 'Série B') RETURNING id`).Scan(&other)
	if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, TRUE)`, other, w); err == nil {
		t.Error("a work got into two official collections")
	}
	// The personal one does not count, and is not touched by the series.
	db.Exec(`UPDATE works SET series = NULL WHERE id = $1`, w)
	if n := count(t, db, `SELECT count(*) FROM collection_works WHERE collection_id = $1 AND work_id = $2`, personal, w); n != 1 {
		t.Error("the series took the work out of a personal collection")
	}
	if _, err := db.Exec(`INSERT INTO collections (kind, name) VALUES ('shared', 'x')`); err == nil {
		t.Error("a collection of an unknown kind was accepted")
	}
	// The kind and the owner go together.
	if _, err := db.Exec(`INSERT INTO collections (kind, name) VALUES ('personal', 'sem dono')`); err == nil {
		t.Error("a personal collection with no owner was accepted")
	}
	if _, err := db.Exec(`INSERT INTO collections (kind, owner_id, name) VALUES ('official', $1, 'com dono')`, owner); err == nil {
		t.Error("an official collection with an owner was accepted")
	}
	// A person who leaves takes the personal collections along; an official one has no owner to lose.
	db.Exec(`DELETE FROM users WHERE id = $1`, owner)
	if n := count(t, db, `SELECT count(*) FROM collections WHERE id = $1`, personal); n != 0 {
		t.Error("a personal collection outlived its owner")
	}
}

func TestCollections_ADeletedWorkLeavesTheCollection(t *testing.T) {
	db := migrated(t)
	w := addSeries(t, db, "Obra", "Série", 1)
	c := officialOf(t, db, w)
	db.Exec(`DELETE FROM works WHERE id = $1`, w)
	if n := count(t, db, `SELECT count(*) FROM collection_works WHERE collection_id = $1`, c); n != 0 {
		t.Error("the membership of a deleted work stayed")
	}
}

func TestCollections_TheSeriesOfTheCatalogueBecomeCollectionsWhenMigrating(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 51); err != nil {
		t.Fatal(err)
	}
	ins := func(title string, series any, index float64) {
		if _, err := db.Exec(`INSERT INTO works (original_title, series, series_index) VALUES ($1, $2, $3)`, title, series, index); err != nil {
			t.Fatal(err)
		}
	}
	ins("A1", "Duna", 1)
	ins("A2", "duna", 2)
	ins("B1", "Fundação", 0)
	ins("Solto", nil, 0)
	ins("Vazio", "", 0)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT count(*) FROM collections WHERE kind = 'official' AND origin = 'metadata'`); n != 2 {
		t.Errorf("collections = %d, want 2", n)
	}
	if n := count(t, db, `SELECT count(*) FROM collection_works`); n != 3 {
		t.Errorf("memberships = %d, want 3", n)
	}
	if n := count(t, db, `SELECT count(*) FROM collection_works cw JOIN collections c ON c.id = cw.collection_id WHERE c.name = 'Duna'`); n != 2 {
		t.Errorf("Duna has %d works, want 2", n)
	}
	// Going back leaves the works as they were: the collections are a copy of what the works say.
	if err := database.RollbackTo(db, 51); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT count(*) FROM works WHERE series IS NOT NULL AND series <> ''`); n != 3 {
		t.Errorf("works with a series after rolling back = %d, want 3", n)
	}
	if tableExists(t, db, "collections") {
		t.Error("the collections stayed after rolling back")
	}
}

func TestCollections_WorksOfANewSeriesWrittenAtTheSameTimeMakeOneCollection(t *testing.T) {
	db := migrated(t)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if _, err := db.Exec(`INSERT INTO works (original_title, series, series_index) VALUES ($1, 'Nova Série', $2)`, fmt.Sprintf("Vol %d", i), i+1); err != nil {
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := count(t, db, `SELECT count(*) FROM collections`); n != 1 {
		t.Errorf("collections = %d, want 1", n)
	}
	if n := count(t, db, `SELECT count(*) FROM collection_works`); n != 40 {
		t.Errorf("memberships = %d, want 40", n)
	}
}

// The second writer arrives while the first has made the collection and not yet committed: it must wait, and then join
// that collection, instead of making its own and failing on the key.
func TestCollections_AWriterWaitsForTheOneThatIsMakingTheCollection(t *testing.T) {
	db := migrated(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO works (original_title, series) VALUES ('Primeiro', 'Em Obras')`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := db.Exec(`INSERT INTO works (original_title, series) VALUES ('Segundo', 'em obras')`)
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the second writer failed: %v", err)
	}
	if n := count(t, db, `SELECT count(*) FROM collections`); n != 1 {
		t.Errorf("collections = %d, want 1", n)
	}
	if n := count(t, db, `SELECT count(*) FROM collection_works`); n != 2 {
		t.Errorf("memberships = %d, want 2", n)
	}
}

func TestPersonalCollections_AWorkDeletedForGoodLeavesItsLabelInTheListsOfThePeopleAndGoesFromTheOfficialOne(t *testing.T) {
	db := migrated(t)
	var ana string
	if err := db.QueryRow(`INSERT INTO users (username, email, password_hash) VALUES ('ana', 'ana@example.test', 'x') RETURNING id`).Scan(&ana); err != nil {
		t.Fatal(err)
	}
	var author int
	db.QueryRow(`INSERT INTO person (name) VALUES ('Frank Herbert') RETURNING id`).Scan(&author)
	w := addSeries(t, db, "Duna", "Duna", 1)
	other := addSeries(t, db, "Messias de Duna", "Duna", 2)
	if _, err := db.Exec(`INSERT INTO work_contributors (work_id, person_id, role, position) VALUES ($1, $2, 'author', 0)`, w, author); err != nil {
		t.Fatal(err)
	}
	var list1, list2 int
	db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Um', 'manual') RETURNING id`, ana).Scan(&list1)
	db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Dois', 'manual') RETURNING id`, ana).Scan(&list2)
	for _, l := range []int{list1, list2} {
		if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 1)`, l, w); err != nil {
			t.Fatal(err)
		}
	}
	official := officialOf(t, db, w)

	if _, err := db.Exec(`DELETE FROM works WHERE id = $1`, w); err != nil {
		t.Fatal(err)
	}
	// The places in the lists stay, pointing at nothing, with what the person saw.
	rows, err := db.Query(`SELECT collection_id, label, author_label FROM collection_works WHERE work_id IS NULL ORDER BY collection_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var c int
		var label, authors sql.NullString
		rows.Scan(&c, &label, &authors)
		if label.String != "Duna" || authors.String != "Frank Herbert" {
			t.Errorf("place in list %d = %q by %q", c, label.String, authors.String)
		}
		n++
	}
	if n != 2 {
		t.Errorf("places left = %d, want 2", n)
	}
	// The official membership went with the work, and the other work stayed in the collection.
	if c := count(t, db, `SELECT count(*) FROM collection_works WHERE collection_id = $1`, official); c != 1 {
		t.Errorf("official collection has %d places, want only the other work", c)
	}
	if officialOf(t, db, other) != official {
		t.Error("the other work left the official collection")
	}
	// A work with no author leaves with no author.
	bare := addSeries(t, db, "Sem autor", nil, 0)
	db.Exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 2)`, list1, bare)
	db.Exec(`DELETE FROM works WHERE id = $1`, bare)
	if c := count(t, db, `SELECT count(*) FROM collection_works WHERE label = 'Sem autor' AND author_label IS NULL`); c != 1 {
		t.Errorf("a work with no author: %d places with a label and no author, want 1", c)
	}
}

func TestPersonalCollections_TheRulesOfThePlaces(t *testing.T) {
	db := migrated(t)
	var ana string
	db.QueryRow(`INSERT INTO users (username, email, password_hash) VALUES ('ana', 'ana@example.test', 'x') RETURNING id`).Scan(&ana)
	w := addSeries(t, db, "Obra", "Saga", 1)
	var list, list2, official int
	db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Um', 'manual') RETURNING id`, ana).Scan(&list)
	db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Dois', 'manual') RETURNING id`, ana).Scan(&list2)
	official = officialOf(t, db, w)

	// A work is in a list once, and in as many lists as the person wants.
	if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, list, w); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, list, w); err == nil {
		t.Error("a work got into the same list twice")
	}
	if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, list2, w); err != nil {
		t.Errorf("a work in two lists: %v", err)
	}
	// Places of works that are gone are many, and each is its own.
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official, label) VALUES ($1, NULL, FALSE, 'x')`, list); err != nil {
			t.Errorf("a place with no work: %v", err)
		}
	}
	// An official membership cannot outlive its work.
	if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, NULL, TRUE)`, official); err == nil {
		t.Error("an official place with no work was accepted")
	}
	// The sync of the series leaves the places of the lists alone, and its own go on working.
	db.Exec(`UPDATE works SET series = 'Outra' WHERE id = $1`, w)
	if c := count(t, db, `SELECT count(*) FROM collection_works WHERE work_id = $1 AND NOT official`, w); c != 2 {
		t.Errorf("the series touched the lists: %d places, want 2", c)
	}
	if officialOf(t, db, w) == official || officialOf(t, db, w) == 0 {
		t.Error("the series did not move the work to the new official collection")
	}
}

func TestPersonalCollections_TheMigrationKeepsWhatWasThereAndGoesBack(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 52); err != nil {
		t.Fatal(err)
	}
	var w int
	db.QueryRow(`INSERT INTO works (original_title, series, series_index) VALUES ('Duna', 'Duna', 1) RETURNING id`).Scan(&w)
	var ana string
	db.QueryRow(`INSERT INTO users (username, email, password_hash) VALUES ('ana', 'ana@example.test', 'x') RETURNING id`).Scan(&ana)
	var list int
	db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Minha', 'manual') RETURNING id`, ana).Scan(&list)
	db.Exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 4)`, list, w)

	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if c := count(t, db, `SELECT count(*) FROM collection_works WHERE work_id = $1 AND id IS NOT NULL`, w); c != 2 {
		t.Errorf("places after migrating = %d, want the official and the personal", c)
	}
	// Going back drops the places that no longer have a work, and keeps the rest.
	db.Exec(`INSERT INTO collection_works (collection_id, work_id, official, label) VALUES ($1, NULL, FALSE, 'sem obra')`, list)
	if err := database.RollbackTo(db, 52); err != nil {
		t.Fatal(err)
	}
	if c := count(t, db, `SELECT count(*) FROM collection_works`); c != 2 {
		t.Errorf("places after going back = %d, want 2", c)
	}
	if _, err := db.Exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, list, w); err == nil {
		t.Error("the key of the old shape (list, work) was not back")
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrating again: %v", err)
	}
}
