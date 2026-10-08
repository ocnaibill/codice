package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func (s *catalogStack) personPage(a actor, id int) (PersonPage, int) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/people/%d", id), "")
	var out PersonPage
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func roleList(p PersonPage) string {
	var out []string
	for _, r := range p.Roles {
		out = append(out, fmt.Sprintf("%s:%d", r.Role, r.Works))
	}
	return strings.Join(out, ",")
}

func TestPeoplePage_SaysWhoTheyAreAndInWhichRolesTheLibraryHasWorksOfTheirs(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	messias := s.addWork("Messias", "Frank Herbert", "b.epub", "epub")
	gone := s.addWork("Na lixeira", "Frank Herbert", "c.epub", "epub")
	other := s.addWork("Outro", "Plato", "d.epub", "epub")
	s.credit(admin, duna, "Frank Herbert", "editor") // the same person, another role, on one of the works
	s.credit(admin, other, "Frank Herbert", "narrator")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)
	s.exec(`INSERT INTO person_alias (person_id, alias) VALUES ($1, 'Herbert, Frank'), ($1, 'F. Herbert'), ($1, 'Frank Herbert')`, s.personID("Frank Herbert"))
	_ = messias

	p, code := s.personPage(ana, s.personID("Frank Herbert"))
	if code != 200 || p.Name != "Frank Herbert" || p.DisplayName != "Frank Herbert" {
		t.Fatalf("page: %d %+v", code, p)
	}
	// The works in the trash do not count; a work counts once for a role, however many times; roles come in the order of the roles.
	if got := roleList(p); got != "author:2,narrator:1,editor:1" {
		t.Errorf("roles = %s", got)
	}
	// The aliases are the other writings, and not the name it goes by.
	if strings.Join(p.Aliases, "|") != "F. Herbert|Herbert, Frank" {
		t.Errorf("aliases = %v", p.Aliases)
	}
	// Somebody with no work in the library still has a page, with nothing in it.
	s.exec(`INSERT INTO person (name) VALUES ('Sem Obras')`)
	if p, code := s.personPage(ana, s.personID("Sem Obras")); code != 200 || len(p.Roles) != 0 || len(p.Collections) != 0 || len(p.Aliases) != 0 {
		t.Errorf("a person with no work: %d %+v", code, p)
	}
}

func TestPeoplePage_IsShownTheWayTheAccountChoseToSeeNames(t *testing.T) {
	s := newCatalogStack(t)
	s.authorsBook()
	frank, plato := s.personID("Frank Herbert"), s.personID("Plato")
	s.setChoice(ana, "family_first")
	if p, _ := s.personPage(ana, frank); p.DisplayName != "Herbert, Frank" || p.Name != "Frank Herbert" {
		t.Errorf("surname first: %+v", p)
	}
	// A name whose parts are not known is left as it is; and another account sees it its own way.
	if p, _ := s.personPage(ana, plato); p.DisplayName != "Plato" {
		t.Errorf("a name not divided: %+v", p)
	}
	if p, _ := s.personPage(bob, frank); p.DisplayName != "Frank Herbert" {
		t.Errorf("bob chose nothing: %+v", p)
	}
}

func TestPeoplePage_ListsTheOfficialCollectionsThatHaveWorksOfTheirs(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	b := s.addWork("Messias", "Frank Herbert", "b.epub", "epub")
	c := s.addWork("Outro da série", "Alguém", "c.epub", "epub")
	solo := s.addWork("Solto", "Frank Herbert", "d.epub", "epub")
	s.exec(`UPDATE works SET series = 'Crônicas de Duna', series_index = 1 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = 'Crônicas de Duna', series_index = 2 WHERE id = $1`, b)
	s.exec(`UPDATE works SET series = 'Crônicas de Duna', series_index = 3 WHERE id = $1`, c) // not his work, in the same collection
	s.exec(`UPDATE works SET series = 'Antologias' WHERE id = $1`, solo)
	// A list of a person is not a collection of the library.
	list := s.makeList(ana, "Para ler")
	s.putInList(ana, list, a, "")

	p, _ := s.personPage(ana, s.personID("Frank Herbert"))
	if len(p.Collections) != 2 || p.Collections[0].Name != "Antologias" || p.Collections[0].Works != 1 ||
		p.Collections[1].Name != "Crônicas de Duna" || p.Collections[1].Works != 2 {
		t.Errorf("collections = %+v (his works in each, by name; not the lists)", p.Collections)
	}
	// A retired collection and a work in the trash are left out.
	s.exec(`UPDATE collections SET retired_at = now() WHERE name = 'Antologias'`)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, b)
	p, _ = s.personPage(ana, s.personID("Frank Herbert"))
	if len(p.Collections) != 1 || p.Collections[0].Name != "Crônicas de Duna" || p.Collections[0].Works != 1 {
		t.Errorf("collections after retiring = %+v", p.Collections)
	}
}

func TestPeoplePage_AWorkCountsOncePerCollectionWhateverTheRolesOfThePersonOnIt(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.exec(`UPDATE works SET series = 'Crônicas de Duna', series_index = 1 WHERE id = $1`, a)
	s.credit(admin, a, "Frank Herbert", "editor")
	s.credit(admin, a, "Frank Herbert", "illustrator")
	p, _ := s.personPage(ana, s.personID("Frank Herbert"))
	if len(p.Collections) != 1 || p.Collections[0].Works != 1 {
		t.Errorf("collections = %+v: one work with three roles of the person counts once", p.Collections)
	}
	if got := roleList(p); got != "author:1,editor:1,illustrator:1" {
		t.Errorf("roles = %s", got)
	}
}

func TestPeoplePage_APersonThatIsNotThereIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	for _, id := range []string{"0", "-1", "abc", "99999", "1.5"} {
		if rec := s.do(ana, "GET", "/people/"+id, ""); rec.Code != 404 {
			t.Errorf("GET /people/%s: %d, want 404", id, rec.Code)
		}
	}
}

func TestWorksOfAPerson_TheListCanBeFilteredByPersonAndRole(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	messias := s.addWork("Messias", "Frank Herbert", "b.epub", "epub")
	rep := s.addWork("A República", "Plato", "c.epub", "epub")
	gone := s.addWork("Na lixeira", "Frank Herbert", "d.epub", "epub")
	s.credit(admin, rep, "Frank Herbert", "translator")
	s.credit(admin, duna, "Brian Herbert", "author")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)
	frank, brian, plato := s.personID("Frank Herbert"), s.personID("Brian Herbert"), s.personID("Plato")

	for q, want := range map[string][]int{
		fmt.Sprintf("?person=%d", frank):                 {duna, messias, rep}, // any role; the one in the trash out
		fmt.Sprintf("?person=%d&role=author", frank):     {duna, messias},
		fmt.Sprintf("?person=%d&role=translator", frank): {rep},
		fmt.Sprintf("?person=%d&role=narrator", frank):   {},
		fmt.Sprintf("?person=%d&role=author", brian):     {duna}, // a coauthor
		fmt.Sprintf("?person=%d", plato):                 {rep},
		"?role=translator":                               {rep}, // a role alone: whoever has it
		"?person=abc":                                    {},
		"?person=0":                                      {},
		"?person=99999":                                  {},
		fmt.Sprintf("?person=%d&role=composer", frank):   {},
		"?role=composer":                                 {},
	} {
		if got := ids(s.list(ana, q).Data); !sameIDs(got, want...) {
			t.Errorf("GET /works%s = %v, want %v", q, got, want)
		}
	}
	// It goes with the rest of the filters, and the total and the pages agree.
	l := s.list(ana, fmt.Sprintf("?person=%d&limit=1&sort=title", frank))
	if l.Total != 3 || len(l.Data) != 1 {
		t.Errorf("total %d, listed %d: want 3 and 1", l.Total, len(l.Data))
	}
	if got := ids(s.list(ana, fmt.Sprintf("?person=%d&search=mess", frank)).Data); !sameIDs(got, messias) {
		t.Errorf("with a search = %v, want %v", got, messias)
	}
	// What is retired is for the staff, whose list shows it with the person too.
	if got := ids(s.list(admin, fmt.Sprintf("?person=%d&retired=true", frank)).Data); !sameIDs(got, gone) {
		t.Errorf("retired works of the person = %v, want %v", got, gone)
	}
}

func TestWorkContributors_TheCreditsCarryTheNameTheAccountIsShown(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.exec(`UPDATE person SET family_name = 'Herbert', given_name = 'Frank' WHERE name = 'Frank Herbert'`)
	s.setChoice(ana, "family_first")
	w, _ := s.detail(ana, duna)
	if c := w.Metadata.Contributors; len(c) != 1 || c[0].Name != "Frank Herbert" || c[0].DisplayName != "Herbert, Frank" || c[0].PersonID == 0 {
		t.Errorf("credits for ana = %+v", c)
	}
	w, _ = s.detail(bob, duna)
	if c := w.Metadata.Contributors; c[0].DisplayName != "Frank Herbert" {
		t.Errorf("credits for bob = %+v", c)
	}
}
