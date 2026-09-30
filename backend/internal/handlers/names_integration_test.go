package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/people"
)

func (s *catalogStack) prefs(a actor) people.Preference {
	s.t.Helper()
	var p people.Preference
	json.Unmarshal(s.do(a, "GET", "/auth/preferences", "").Body.Bytes(), &p)
	return p
}

func (s *catalogStack) setChoice(a actor, order string) int {
	return s.do(a, "PUT", "/auth/preferences", fmt.Sprintf(`{"nameOrder":%q}`, order)).Code
}

// A person whose surname is known (written the catalogue's way with a role), one whose name is not divided,
// and a work with two authors, one of each.
func (s *catalogStack) authorsBook() (herbert, plato, both int) {
	s.t.Helper()
	herbert = s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	plato = s.addWork("A República", "Plato", "b.epub", "epub")
	both = s.addWork("Dois", "Frank Herbert", "c.epub", "epub")
	s.exec(`UPDATE person SET family_name = 'Herbert', given_name = 'Frank' WHERE name = 'Frank Herbert'`)
	s.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'author', 1 FROM person WHERE name = 'Plato'`, both)
	return
}

func authorOf(s *catalogStack, a actor, id int) string {
	w, code := s.detail(a, id)
	if code != 200 {
		s.t.Fatalf("detail %d: %d", id, code)
	}
	return w.Author
}

func TestNames_TheLibraryDefaultIsGivenNamesFirstAndTheOwnerCanChangeIt(t *testing.T) {
	s := newCatalogStack(t)
	herbert, plato, both := s.authorsBook()
	if p := s.prefs(ana); p.Effective != "given_first" || p.Library != "given_first" || p.Choice != "" {
		t.Fatalf("a new library: %+v", p)
	}
	if got := authorOf(s, ana, herbert); got != "Frank Herbert" {
		t.Errorf("default = %q", got)
	}

	if code := s.do(admin, "PUT", "/admin/name-order", `{"nameOrder":"family_first"}`).Code; code != 200 {
		t.Fatalf("set the library default: %d", code)
	}
	if p := s.prefs(ana); p.Effective != "family_first" || p.Library != "family_first" || p.Choice != "" {
		t.Errorf("after the owner chose: %+v", p)
	}
	// Surname first where it is known; a name that is not divided is shown as it is.
	if got := authorOf(s, ana, herbert); got != "Herbert, Frank" {
		t.Errorf("family first = %q", got)
	}
	if got := authorOf(s, ana, plato); got != "Plato" {
		t.Errorf("an undivided name = %q", got)
	}
	if got := authorOf(s, ana, both); got != "Herbert, Frank; Plato" {
		t.Errorf("two authors, surname first, = %q (the separator must not be a comma: \"Herbert, Frank\" has one)", got)
	}
	if code := s.do(admin, "PUT", "/admin/name-order", `{"nameOrder":"whatever"}`).Code; code != 400 {
		t.Errorf("an invalid default: %d", code)
	}
	if code := s.do(admin, "PUT", "/admin/name-order", `{}`).Code; code != 400 {
		t.Errorf("no default: %d", code)
	}
}

func TestNames_EachAccountCanChooseAndGoBackToTheLibrarysDefault(t *testing.T) {
	s := newCatalogStack(t)
	herbert, _, _ := s.authorsBook()
	if code := s.setChoice(ana, "family_first"); code != 200 {
		t.Fatalf("ana chooses: %d", code)
	}
	if got := authorOf(s, ana, herbert); got != "Herbert, Frank" {
		t.Errorf("ana sees %q", got)
	}
	if got := authorOf(s, bob, herbert); got != "Frank Herbert" {
		t.Errorf("a choice is personal: bob sees %q", got)
	}
	// The library's default follows for whoever has not chosen, and a choice wins over it.
	s.do(admin, "PUT", "/admin/name-order", `{"nameOrder":"family_first"}`)
	if got := authorOf(s, bob, herbert); got != "Herbert, Frank" {
		t.Errorf("bob follows the library: %q", got)
	}
	if code := s.setChoice(ana, "given_first"); code != 200 {
		t.Fatal(code)
	}
	if got := authorOf(s, ana, herbert); got != "Frank Herbert" {
		t.Errorf("a choice wins over the library: %q", got)
	}
	if p := s.prefs(ana); p.Choice != "given_first" || p.Library != "family_first" || p.Effective != "given_first" {
		t.Errorf("ana: %+v", p)
	}
	// Empty goes back to the library's.
	s.setChoice(ana, "")
	if p := s.prefs(ana); p.Choice != "" || p.Effective != "family_first" {
		t.Errorf("after going back: %+v", p)
	}
	if code := s.setChoice(ana, "sideways"); code != 400 {
		t.Errorf("an invalid choice: %d", code)
	}
	if p := s.prefs(ana); p.Choice != "" {
		t.Errorf("an invalid choice was saved: %+v", p)
	}
}

func TestNames_EverySurfaceThatShowsAnAuthorFollowsThePreference(t *testing.T) {
	s := newCatalogStack(t)
	herbert, _, _ := s.authorsBook()
	s.setChoice(ana, "family_first")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", herbert), "")
	s.exec(`INSERT INTO files (edition_id, format) SELECT id, 'epub' FROM editions WHERE work_id = $1 LIMIT 0`, herbert)

	if l := s.list(ana, ""); !containsAuthor(l.Data, "Herbert, Frank") {
		t.Errorf("the catalogue: %+v", l.Data)
	}
	var fav struct{ Data []FavoriteSeriesItem }
	json.Unmarshal(s.do(ana, "GET", "/favorites", "").Body.Bytes(), &fav)
	if len(fav.Data) != 1 || fav.Data[0].Author != "Herbert, Frank" {
		t.Errorf("favorites: %+v", fav.Data)
	}
	s.index(s.primaryFile(herbert), segment{text: "uma passagem sobre areia e especiaria no deserto", locator: `{"type":"epub","href":"c1.xhtml"}`})
	if _, r := s.search(ana, q("especiaria")); len(r.Data) != 1 || r.Data[0].WorkAuthor != "Herbert, Frank" {
		t.Errorf("content search: %+v", r.Data)
	}
	if body := s.do(ana, "GET", "/opds/recent", "").Body.String(); !strings.Contains(body, "<name>Herbert, Frank</name>") {
		t.Errorf("OPDS: %s", body)
	}
	// Bob has not chosen: he still sees the default everywhere.
	if body := s.do(bob, "GET", "/opds/recent", "").Body.String(); !strings.Contains(body, "<name>Frank Herbert</name>") {
		t.Errorf("OPDS for bob: %s", body)
	}
}

func containsAuthor(works []Work, author string) bool {
	for _, w := range works {
		if w.Author == author {
			return true
		}
	}
	return false
}

func TestNames_TheCatalogueIsSortedByTheAuthorAsItIsShown(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("T1", "Frank Herbert", "1.epub", "epub")
	s.addWork("T2", "Ursula Le Guin", "2.epub", "epub")
	s.addWork("T3", "Isaac Asimov", "3.epub", "epub")
	s.addWork("T4", "Arthur Clarke", "4.epub", "epub")
	s.exec(`UPDATE person SET family_name = split_part(name, ' ', 2), given_name = split_part(name, ' ', 1) WHERE name <> 'Ursula Le Guin'`)
	s.exec(`UPDATE person SET family_name = 'Le Guin', given_name = 'Ursula' WHERE name = 'Ursula Le Guin'`)
	noAuthor := s.addWork("T5", "", "5.epub", "epub")
	s.exec(`DELETE FROM work_contributors WHERE work_id = $1`, noAuthor)

	titles := func(a actor) string {
		var out []string
		for _, w := range s.list(a, "?sort=author").Data {
			out = append(out, w.Title)
		}
		return strings.Join(out, ",")
	}
	// Given names first: Arthur, Frank, Isaac, Ursula. Nobody with no author comes last.
	if got := titles(ana); got != "T4,T1,T3,T2,T5" {
		t.Errorf("by given name = %s", got)
	}
	s.setChoice(ana, "family_first")
	// Surname first: Asimov, Clarke, Herbert, Le Guin.
	if got := titles(ana); got != "T3,T4,T1,T2,T5" {
		t.Errorf("by surname = %s", got)
	}
	if got := titles(bob); got != "T4,T1,T3,T2,T5" {
		t.Errorf("a choice is personal: bob = %s", got)
	}
	// The default order is untouched, and by title works too.
	if got := strings.Join(idsToTitles(s.list(ana, "").Data), ","); got != "T5,T4,T3,T2,T1" {
		t.Errorf("default = %s (newest first)", got)
	}
	if got := strings.Join(idsToTitles(s.list(ana, "?sort=title").Data), ","); got != "T1,T2,T3,T4,T5" {
		t.Errorf("by title = %s", got)
	}
	if got := strings.Join(idsToTitles(s.list(ana, "?sort=whatever").Data), ","); got != "T5,T4,T3,T2,T1" {
		t.Errorf("an unknown sort = %s", got)
	}
}

func idsToTitles(works []Work) []string {
	var out []string
	for _, w := range works {
		out = append(out, w.Title)
	}
	return out
}

func TestNames_SortingIgnoresAccentsAndCaseAndNeverRepeatsAWorkBetweenPages(t *testing.T) {
	s := newCatalogStack(t)
	for i, title := range []string{"Árvore", "abelha", "Zebra", "ânsia", "Bola"} {
		s.addWork(title, "Mesmo Autor", fmt.Sprintf("%d.epub", i), "epub")
	}
	var all []string
	for page := 1; page <= 3; page++ {
		all = append(all, idsToTitles(s.list(ana, fmt.Sprintf("?sort=author&limit=2&page=%d", page)).Data)...)
	}
	if got := strings.Join(all, ","); got != "abelha,ânsia,Árvore,Bola,Zebra" {
		t.Errorf("pages = %s", got)
	}
	if got := strings.Join(idsToTitles(s.list(ana, "?sort=title").Data), ","); got != "abelha,ânsia,Árvore,Bola,Zebra" {
		t.Errorf("by title = %s", got)
	}
}

func TestNames_SearchFindsTheAuthorWhicheverWayItIsShown(t *testing.T) {
	s := newCatalogStack(t)
	herbert, _, _ := s.authorsBook()
	s.setChoice(ana, "family_first")
	for _, term := range []string{"Frank Herbert", "Herbert, Frank", "herbert"} {
		if l := s.list(ana, "?search="+url.QueryEscape(term)); len(l.Data) == 0 || l.Data[0].ID != herbert && !containsID(l.Data, herbert) {
			t.Errorf("searching %q found %+v", term, idsToTitles(l.Data))
		}
	}
}

func containsID(works []Work, id int) bool {
	for _, w := range works {
		if w.ID == id {
			return true
		}
	}
	return false
}

func TestNames_ACataloguesWayWithARoleLearnsTheSurnameAndAnythingElseDoesNot(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("dune.pdf", "", "d.pdf", "pdf")
	c := s.addCandidate(id, "author", "Herbert, Frank, author", "openlibrary")
	if code := s.decide(id, c, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if got := s.scalar(`SELECT family_name || '|' || given_name FROM person WHERE name = 'Frank Herbert'`); got != "Herbert|Frank" {
		t.Errorf("parts = %q", got)
	}
	other := s.addWork("outro.pdf", "", "o.pdf", "pdf")
	c2 := s.addCandidate(other, "author", "Brian Herbert", "openlibrary")
	s.decide(other, c2, "accept")
	if got := s.scalar(`SELECT COALESCE(family_name, '-') FROM person WHERE name = 'Brian Herbert'`); got != "-" {
		t.Errorf("a name the writing does not divide got a surname: %q", got)
	}
	// Accepting the same author again, written another way, does not replace what is known.
	third := s.addWork("terceiro.pdf", "", "t.pdf", "pdf")
	c3 := s.addCandidate(third, "author", "Frank Herbert", "google_books")
	s.decide(third, c3, "accept")
	if got := s.scalar(`SELECT family_name || '|' || given_name FROM person WHERE name = 'Frank Herbert'`); got != "Herbert|Frank" {
		t.Errorf("what was known was lost: %q", got)
	}
}

func TestNames_AHumanSayingTheyAreTheSameLearnsWhereTheSurnameEnds(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("A", "Herbert, Frank", "a.epub", "epub")
	s.addWork("B", "Frank Herbert", "b.epub", "epub")
	people.Detect(s.t.Context(), s.db)
	pair := s.personPairs()[0]
	keep := s.personID("Frank Herbert")
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/people/merges/%d/merge", pair.ID), fmt.Sprintf(`{"keep":%d,"confirm":true}`, keep)); rec.Code != http.StatusNoContent {
		t.Fatalf("merge: %d", rec.Code)
	}
	if got := s.scalar(`SELECT family_name || '|' || given_name FROM person WHERE id = $1`, keep); got != "Herbert|Frank" {
		t.Errorf("parts after the merge = %q", got)
	}
	s.setChoice(ana, "family_first")
	w := s.list(ana, "")
	if !containsAuthor(w.Data, "Herbert, Frank") {
		t.Errorf("after the merge the surname is known: %+v", idsToTitles(w.Data))
	}
}

func TestNames_AnAdminCanCorrectWhichWordsAreTheSurnameButNeverRenameAnyone(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("A", "Gabriel García Márquez", "a.epub", "epub")
	pid := s.personID("Gabriel García Márquez")
	path := fmt.Sprintf("/admin/people/%d/name", pid)

	if code := s.do(admin, "PUT", path, `{"family":"García Márquez","given":"Gabriel"}`).Code; code != 204 {
		t.Fatalf("correct: %d", code)
	}
	s.setChoice(ana, "family_first")
	if got := authorOf(s, ana, id); got != "García Márquez, Gabriel" {
		t.Errorf("shown = %q", got)
	}
	if code := s.do(admin, "PUT", path, `{"family":"Márquez","given":"Gabriel García"}`).Code; code != 204 {
		t.Errorf("another split of the same words: %d", code)
	}
	if code := s.do(admin, "PUT", path, `{"family":"Outro","given":"Nome"}`).Code; code != 400 {
		t.Errorf("words the name does not have: %d", code)
	}
	if code := s.do(admin, "PUT", path, `{"family":"","given":"Gabriel García Márquez"}`).Code; code != 400 {
		t.Errorf("given names with no surname: %d", code)
	}
	if got := s.scalar(`SELECT name FROM person WHERE id = $1`, pid); got != "Gabriel García Márquez" {
		t.Errorf("the person was renamed: %q", got)
	}
	// A mononym: the surname alone.
	mono := s.personID("Gabriel García Márquez")
	if code := s.do(admin, "PUT", fmt.Sprintf("/admin/people/%d/name", mono), `{"family":"Gabriel García Márquez"}`).Code; code != 204 {
		t.Errorf("a surname only: %d", code)
	}
	// Clearing takes the division away, and the name is shown as it is.
	if code := s.do(admin, "PUT", path, `{"family":"","given":""}`).Code; code != 204 {
		t.Errorf("clear: %d", code)
	}
	if got := authorOf(s, ana, id); got != "Gabriel García Márquez" {
		t.Errorf("after clearing = %q", got)
	}
	if code := s.do(admin, "PUT", "/admin/people/99999/name", `{"family":"X"}`).Code; code != 404 {
		t.Errorf("unknown: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.parts'`); got == "0" {
		t.Errorf("not audited")
	}
}

func TestNames_WhatIsKnownAboutWhichWordsAreTheSurnameIsNeverReplacedByAGuessOrALaterWriting(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("A", "Gabriel García Márquez", "a.epub", "epub")
	pid := s.personID("Gabriel García Márquez")
	s.do(admin, "PUT", fmt.Sprintf("/admin/people/%d/name", pid), `{"family":"García Márquez","given":"Gabriel"}`) // an admin said so

	// A later writing, with a role, that divides the same words differently, does not override what is known.
	other := s.addWork("B", "", "b.epub", "epub")
	c := s.addCandidate(other, "author", "Márquez, Gabriel García, author", "openlibrary")
	if code := s.decide(other, c, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if got := s.scalar(`SELECT family_name || '|' || given_name FROM person WHERE id = $1`, pid); got != "García Márquez|Gabriel" {
		t.Errorf("a later writing replaced what an admin said: %q", got)
	}

	// Nor does a merge that would suggest another division.
	s.exec(`INSERT INTO person (name) VALUES ('Márquez, Gabriel García')`)
	people.Detect(s.t.Context(), s.db)
	pair := s.personPairs()[0]
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/people/merges/%d/merge", pair.ID), fmt.Sprintf(`{"keep":%d,"confirm":true}`, pid)); rec.Code != http.StatusNoContent {
		t.Fatalf("merge: %d", rec.Code)
	}
	if got := s.scalar(`SELECT family_name || '|' || given_name FROM person WHERE id = $1`, pid); got != "García Márquez|Gabriel" {
		t.Errorf("a merge replaced what an admin said: %q", got)
	}
	_ = id
}
