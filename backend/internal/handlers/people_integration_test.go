package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/people"
)

type personSide struct {
	ID      int
	Name    string
	Aliases []string
	Works   int
	Titles  []string
}
type personPair struct {
	ID   int64
	A, B personSide
}

func (s *catalogStack) personPairs() []personPair {
	s.t.Helper()
	var out struct{ Data []personPair }
	json.Unmarshal(s.do(admin, "GET", "/admin/people/merges", "").Body.Bytes(), &out)
	return out.Data
}

func (s *catalogStack) personID(name string) int {
	s.t.Helper()
	return mustAtoi(s.scalar(`SELECT id FROM person WHERE name = $1`, name))
}

func TestPeople_TheSameWordsInAnotherOrderAreProposedAsTheSamePerson(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Dune", "Herbert, Frank", "a.epub", "epub")
	s.addWork("Duna", "Frank Herbert", "b.epub", "epub")
	s.addWork("Outro", "Brian Herbert", "c.epub", "epub")
	s.addWork("Solo", "Plato", "d.epub", "epub")

	n, err := people.Detect(context.Background(), s.db)
	if err != nil || n != 1 {
		t.Fatalf("pairs = %d, %v", n, err)
	}
	if again, _ := people.Detect(context.Background(), s.db); again != 0 {
		t.Errorf("a second scan found %d new pairs", again)
	}
	pairs := s.personPairs()
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v", pairs)
	}
	names := []string{pairs[0].A.Name, pairs[0].B.Name}
	if !(names[0] == "Herbert, Frank" && names[1] == "Frank Herbert") && !(names[1] == "Herbert, Frank" && names[0] == "Frank Herbert") {
		t.Errorf("names = %v", names)
	}
	for _, side := range []personSide{pairs[0].A, pairs[0].B} {
		if side.Works != 1 || len(side.Titles) != 1 {
			t.Errorf("a side should say what it wrote: %+v", side)
		}
	}
}

func TestPeople_MergingKeepsOneWithEverythingTheOtherWroteAndWasCalled(t *testing.T) {
	s := newCatalogStack(t)
	w1 := s.addWork("Dune", "Herbert, Frank", "a.epub", "epub")
	w2 := s.addWork("Duna", "Frank Herbert", "b.epub", "epub")
	w3 := s.addWork("Messias", "Herbert, Frank", "c.epub", "epub")
	// A work that has both spellings among its authors.
	s.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'author', 1 FROM person WHERE name = 'Herbert, Frank'`, w2)
	other, keep := s.personID("Herbert, Frank"), s.personID("Frank Herbert")
	s.exec(`INSERT INTO person_alias (person_id, alias) VALUES ($1, 'Herbert, Frank, author')`, other)
	people.Detect(context.Background(), s.db)
	pair := s.personPairs()[0]

	path := fmt.Sprintf("/admin/people/merges/%d/merge", pair.ID)
	if code := s.do(admin, "POST", path, fmt.Sprintf(`{"keep":%d}`, keep)).Code; code != 400 {
		t.Errorf("without confirm: %d", code)
	}
	if code := s.do(admin, "POST", path, fmt.Sprintf(`{"keep":%d,"confirm":true}`, 99999)).Code; code != 400 {
		t.Errorf("a person outside the pair: %d", code)
	}
	if s.scalar(`SELECT count(*) FROM person`) != "2" {
		t.Fatal("a refused merge changed something")
	}
	if code := s.do(admin, "POST", "/admin/people/merges/99999/merge", fmt.Sprintf(`{"keep":%d,"confirm":true}`, keep)).Code; code != 404 {
		t.Errorf("unknown: %d", code)
	}

	if rec := s.do(admin, "POST", path, fmt.Sprintf(`{"keep":%d,"confirm":true}`, keep)); rec.Code != http.StatusNoContent {
		t.Fatalf("merge: %d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT count(*) FROM person`); got != "1" {
		t.Errorf("people = %s, want 1", got)
	}
	for _, w := range []int{w1, w2, w3} {
		if got := s.scalar(`SELECT count(*) FROM work_contributors WHERE work_id = $1 AND person_id = $2`, w, keep); got != "1" {
			t.Errorf("work %d has %s author rows for the one that stayed, want exactly 1", w, got)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM work_contributors`); got != "3" {
		t.Errorf("author rows = %s", got)
	}
	if got := s.scalar(`SELECT string_agg(alias, '|' ORDER BY alias) FROM person_alias WHERE person_id = $1`, keep); got != "Herbert, Frank|Herbert, Frank, author" {
		t.Errorf("aliases = %q: what the other was called, and what it had as aliases, are kept", got)
	}
	if got := s.scalar(`SELECT details->>'absorbed' || '>' || (details->>'kept') FROM audit_log WHERE action = 'person.merge'`); got != "Herbert, Frank>Frank Herbert" {
		t.Errorf("audit = %q", got)
	}
	if len(s.personPairs()) != 0 {
		t.Errorf("the resolved pair is still listed")
	}
	// The catalogue still finds the works by what the other was called.
	if w := s.list(ana, "?search="+"Herbert%2C+Frank"); len(w.Data) != 3 {
		t.Errorf("searching the old spelling found %d works, want 3", len(w.Data))
	}
}

func TestPeople_NotTheSamePersonIsRememberedAndNeverProposedAgain(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Dune", "Herbert, Frank", "a.epub", "epub")
	s.addWork("Duna", "Frank Herbert", "b.epub", "epub")
	people.Detect(context.Background(), s.db)
	pair := s.personPairs()[0]

	if code := s.do(admin, "POST", fmt.Sprintf("/admin/people/merges/%d/dismiss", pair.ID), "").Code; code != 204 {
		t.Fatalf("dismiss: %d", code)
	}
	if code := s.do(admin, "POST", fmt.Sprintf("/admin/people/merges/%d/dismiss", pair.ID), "").Code; code != 404 {
		t.Errorf("deciding twice: %d", code)
	}
	people.Detect(context.Background(), s.db)
	people.Detect(context.Background(), s.db)
	if len(s.personPairs()) != 0 {
		t.Errorf("a dismissed pair came back")
	}
	if got := s.scalar(`SELECT count(*) FROM person`); got != "2" {
		t.Errorf("people = %s: dismissing must not merge", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.not_same'`); got != "1" {
		t.Errorf("audit entries = %s", got)
	}
}

func TestPeople_MergingDropsTheOtherPairsOfThePersonWhoWentAndKeepsTheRest(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("A", "Herbert, Frank", "a.epub", "epub")
	s.addWork("B", "Frank Herbert", "b.epub", "epub")
	s.addWork("C", "Frank, Herbert", "c.epub", "epub") // a third spelling of the same words
	n, _ := people.Detect(context.Background(), s.db)
	if n != 3 {
		t.Fatalf("pairs among three spellings = %d, want 3", n)
	}
	pair := s.personPairs()[0]
	keep := pair.A.ID
	s.do(admin, "POST", fmt.Sprintf("/admin/people/merges/%d/merge", pair.ID), fmt.Sprintf(`{"keep":%d,"confirm":true}`, keep))
	// The pairs the merged person was in went with it; the one that stayed and the third are still to decide.
	people.Detect(context.Background(), s.db)
	left := s.personPairs()
	if len(left) != 1 {
		t.Fatalf("pairs left = %+v, want the one that stayed and the third", left)
	}
}

func TestPeople_AGroupOfManyIsNotProposed(t *testing.T) {
	s := newCatalogStack(t)
	for i, name := range []string{"Smith, John", "John Smith", "Smith John", "John, Smith", "Smith, John.", "John Smith.", "Smith. John"} {
		s.addWork(fmt.Sprintf("T%d", i), name, fmt.Sprintf("%d.epub", i), "epub")
	}
	if n, _ := people.Detect(context.Background(), s.db); n != 0 {
		t.Errorf("a name that many people have is not one person written in many ways: %d pairs", n)
	}
}
