package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/people"
)

func (s *catalogStack) pendingNames(query string) people.NamesPage {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/people/names"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /admin/people/names%s: %d %s", query, rec.Code, rec.Body.String())
	}
	var out people.NamesPage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatal(err)
	}
	return out
}

func (s *catalogStack) putName(id int, body string) int {
	return s.do(admin, "PUT", fmt.Sprintf("/admin/people/%d/name", id), body).Code
}

func pendingNamesOf(p people.NamesPage) []string {
	names := []string{}
	for _, n := range p.Data {
		names = append(names, n.Name)
	}
	return names
}

func TestPendingNames_AreThePeopleWithWorksWhoseSurnameNobodyHasTold(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.addWork("Messias de Duna", "Frank Herbert", "b.epub", "epub")
	s.addWork("O amor nos tempos do cólera", "Gabriel García Márquez", "c.epub", "epub")
	s.addWork("A República", "Plato", "d.epub", "epub")
	s.addWork("Sobrenome sabido", "Ursula Le Guin", "e.epub", "epub")
	s.addWork("Sem sobrenome", "Médicos Sem Fronteiras", "f.epub", "epub")
	gone := s.addWork("Retirada", "Autor Retirado", "g.epub", "epub")
	s.retire(gone)
	s.exec(`INSERT INTO person (name) VALUES ('Pessoa Sem Obra')`)
	s.exec(`UPDATE person SET family_name = 'Le Guin', given_name = 'Ursula' WHERE name = 'Ursula Le Guin'`)
	s.exec(`UPDATE person SET name_undivided = TRUE WHERE name = 'Médicos Sem Fronteiras'`)

	got := s.pendingNames("")
	if want := []string{"Frank Herbert", "Gabriel García Márquez"}; strings.Join(pendingNamesOf(got), "|") != strings.Join(want, "|") {
		t.Fatalf("names = %v, want %v (most works first; one word, divided, undivided, retired and without works are left out)", pendingNamesOf(got), want)
	}
	if got.Total != 2 || got.Page != 1 || got.Limit != 20 || got.TotalPages != 1 {
		t.Errorf("page = %+v", got)
	}
	herbert := got.Data[0]
	if herbert.Works != 2 || strings.Join(herbert.Titles, "|") != "Duna|Messias de Duna" {
		t.Errorf("Herbert = %+v", herbert)
	}
	if herbert.Suggestion == nil || herbert.Suggestion.Family != "Herbert" || herbert.Suggestion.Given != "Frank" {
		t.Errorf("suggestion = %+v", herbert.Suggestion)
	}
	if m := got.Data[1]; m.Suggestion == nil || m.Suggestion.Family != "Márquez" || m.Suggestion.Given != "Gabriel García" {
		t.Errorf("the last word is the proposal, to be corrected: %+v", m.Suggestion)
	}
}

func TestPendingNames_ALineThatNamesSeveralPeopleHasNoProposalButIsStillListed(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Watchmen", "Alan Moore & Dave Gibbons", "a.cbz", "cbz")
	got := s.pendingNames("")
	if len(got.Data) != 1 || got.Data[0].Suggestion != nil {
		t.Fatalf("%+v", got.Data)
	}
}

func TestPendingNames_AtMostThreeTitlesAreShown(t *testing.T) {
	s := newCatalogStack(t)
	for i := 1; i <= 5; i++ {
		s.addWork(fmt.Sprintf("Livro %d", i), "Frank Herbert", fmt.Sprintf("%d.epub", i), "epub")
	}
	got := s.pendingNames("")
	if len(got.Data) != 1 || got.Data[0].Works != 5 || strings.Join(got.Data[0].Titles, "|") != "Livro 1|Livro 2|Livro 3" {
		t.Errorf("%+v", got.Data)
	}
}

func TestPendingNames_AreNarrowedByWhatTheNameHasWithoutCaseOrAccents(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.addWork("Cólera", "Gabriel García Márquez", "b.epub", "epub")
	s.addWork("Desconto", "Ana 100% Pura", "c.epub", "epub")
	s.addWork("Sublinhado", "Ana_Maria Lima", "d.epub", "epub")
	for q, want := range map[string]string{
		"garcia":             "Gabriel García Márquez",
		"MÁRQUEZ":            "Gabriel García Márquez",
		" herb ":             "Frank Herbert",
		"100%":               "Ana 100% Pura",
		"a_m":                "Ana_Maria Lima",
		"zzz":                "",
		url.QueryEscape("é"): "",
	} {
		got := pendingNamesOf(s.pendingNames("?q=" + url.QueryEscape(q)))
		if strings.Join(got, "|") != want {
			t.Errorf("q=%q: %v, want %q", q, got, want)
		}
	}
	// A "%" and a "_" in what was typed are letters, not wildcards.
	if got := s.pendingNames("?q=" + url.QueryEscape("%")); got.Total != 1 {
		t.Errorf("%% matched %d names", got.Total)
	}
	if got := s.pendingNames("?q=" + url.QueryEscape("_")); got.Total != 1 {
		t.Errorf("_ matched %d names", got.Total)
	}
}

func TestPendingNames_ComeInPagesWithTheOnesWithMostWorksFirstAndATieBrokenByName(t *testing.T) {
	s := newCatalogStack(t)
	for i, name := range []string{"Carlos Cunha", "Ana Abreu", "Bia Barros", "Dora Dias", "Eva Esteves"} {
		for j := 0; j <= i%2; j++ { // Ana and Dora have 2 works, the others 1... by position
			s.addWork(fmt.Sprintf("%s %d", name, j), name, fmt.Sprintf("%d-%d.epub", i, j), "epub")
		}
	}
	all := pendingNamesOf(s.pendingNames(""))
	// Carlos (i=0) 1 work, Ana (1) 2, Bia (2) 1, Dora (3) 2, Eva (4) 1.
	if want := "Ana Abreu|Dora Dias|Bia Barros|Carlos Cunha|Eva Esteves"; strings.Join(all, "|") != want {
		t.Fatalf("order = %v, want %s", all, want)
	}
	first := s.pendingNames("?limit=2")
	second := s.pendingNames("?limit=2&page=2")
	last := s.pendingNames("?limit=2&page=3")
	if strings.Join(pendingNamesOf(first), "|") != "Ana Abreu|Dora Dias" || first.TotalPages != 3 || first.Total != 5 {
		t.Errorf("first = %v %+v", pendingNamesOf(first), first)
	}
	if strings.Join(pendingNamesOf(second), "|") != "Bia Barros|Carlos Cunha" || second.Page != 2 {
		t.Errorf("second = %v", pendingNamesOf(second))
	}
	if strings.Join(pendingNamesOf(last), "|") != "Eva Esteves" {
		t.Errorf("last = %v", pendingNamesOf(last))
	}
	// What is not a page or a size is the first page of 20; a size over 100 is not accepted either.
	for _, q := range []string{"?page=0", "?page=-1", "?page=x", "?limit=0", "?limit=101", "?limit=x"} {
		if got := s.pendingNames(q); got.Page != 1 || got.Limit != 20 || len(got.Data) != 5 {
			t.Errorf("%s: page %d limit %d with %d names", q, got.Page, got.Limit, len(got.Data))
		}
	}
	if got := s.pendingNames("?limit=100"); got.Limit != 100 {
		t.Errorf("limit 100 = %d", got.Limit)
	}
	if got := s.pendingNames("?page=9"); len(got.Data) != 0 || got.Total != 5 {
		t.Errorf("past the end: %+v", got)
	}
}

func TestPendingNames_ConfirmingTheDivisionTakesThePersonOffTheListAndChangesHowTheNameIsShown(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.addWork("A República", "Plato", "b.epub", "epub")
	id := s.personID("Frank Herbert")

	s.setChoice(ana, "family_first")
	if got := authorOf(s, ana, duna); got != "Frank Herbert" {
		t.Fatalf("before: %q (the surname is not known, so the name is shown as it is)", got)
	}
	if code := s.putName(id, `{"family":"Herbert","given":"Frank"}`); code != 204 {
		t.Fatalf("confirming: %d", code)
	}
	if got := authorOf(s, ana, duna); got != "Herbert, Frank" {
		t.Errorf("after, family first: %q", got)
	}
	s.setChoice(ana, "given_first")
	if got := authorOf(s, ana, duna); got != "Frank Herbert" {
		t.Errorf("after, given first: %q", got)
	}
	if n := s.pendingNames("").Total; n != 0 {
		t.Errorf("%d names still pending", n)
	}
	if got := s.scalar(`SELECT details->>'family' || '|' || (details->>'given') FROM audit_log WHERE action = 'person.parts'`); got != "Herbert|Frank" {
		t.Errorf("audit = %q", got)
	}
}

func TestPendingNames_ACompoundSurnameIsConfirmedWithTheWordsThePersonChose(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Cólera", "Gabriel García Márquez", "a.epub", "epub")
	id := s.personID("Gabriel García Márquez")
	if code := s.putName(id, `{"family":"García Márquez","given":"Gabriel"}`); code != 204 {
		t.Fatalf("%d", code)
	}
	if got := s.scalar(`SELECT family_name || '|' || given_name FROM person WHERE id = $1`, id); got != "García Márquez|Gabriel" {
		t.Errorf("stored %q", got)
	}
	// Words that are not the ones of the name are not accepted: this says which is which, it does not rename.
	s.addWork("Outro", "Ursula Le Guin", "b.epub", "epub")
	if code := s.putName(s.personID("Ursula Le Guin"), `{"family":"Guin","given":"Maria"}`); code != 400 {
		t.Errorf("other words: %d, want 400", code)
	}
	if got := s.pendingNames("").Total; got != 1 {
		t.Errorf("a refused division left %d pending, want 1", got)
	}
}

func TestPendingNames_ANameWithNoSurnameIsMarkedAndNotAskedAboutAgainUntilTheMarkIsTakenBack(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Relatório", "Médicos Sem Fronteiras", "a.epub", "epub")
	id := s.personID("Médicos Sem Fronteiras")
	s.setChoice(ana, "family_first")

	if code := s.putName(id, `{"undivided":true}`); code != 204 {
		t.Fatalf("marking: %d", code)
	}
	if n := s.pendingNames("").Total; n != 0 {
		t.Errorf("still pending: %d", n)
	}
	if got := authorOf(s, ana, work); got != "Médicos Sem Fronteiras" {
		t.Errorf("shown as it is, whatever the order: %q", got)
	}
	if got := s.scalar(`SELECT details->>'undivided' FROM audit_log WHERE action = 'person.undivided'`); got != "true" {
		t.Errorf("audit = %q", got)
	}
	// Taking the division away asks about the person again.
	if code := s.putName(id, `{"family":"","given":""}`); code != 204 {
		t.Fatalf("taking back: %d", code)
	}
	if n := s.pendingNames("").Total; n != 1 {
		t.Errorf("after taking it back: %d pending, want 1", n)
	}
}

func TestPendingNames_MarkingANameWithNoSurnameTakesAwayADivisionItHad(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	id := s.personID("Frank Herbert")
	s.putName(id, `{"family":"Herbert","given":"Frank"}`)
	s.setChoice(ana, "family_first")
	if code := s.putName(id, `{"undivided":true}`); code != 204 {
		t.Fatalf("%d", code)
	}
	if got := s.scalar(`SELECT coalesce(family_name, '-') || '|' || coalesce(given_name, '-') FROM person WHERE id = $1`, id); got != "-|-" {
		t.Errorf("parts = %q", got)
	}
	if got := authorOf(s, ana, work); got != "Frank Herbert" {
		t.Errorf("shown as it is: %q", got)
	}
	// And dividing it again clears the mark.
	if code := s.putName(id, `{"family":"Herbert","given":"Frank"}`); code != 204 {
		t.Fatalf("%d", code)
	}
	if got := s.scalar(`SELECT name_undivided::text FROM person WHERE id = $1`, id); got != "false" {
		t.Errorf("mark = %q after dividing", got)
	}
}

func TestPendingNames_AMarkAndPartsTogetherAreRefusedAndAnUnknownPersonIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	id := s.personID("Frank Herbert")
	if code := s.putName(id, `{"undivided":true,"family":"Herbert","given":"Frank"}`); code != 400 {
		t.Errorf("both: %d, want 400", code)
	}
	if code := s.putName(id, `{"undivided":true,"family":"Herbert"}`); code != 400 {
		t.Errorf("mark and one part: %d, want 400", code)
	}
	if n := s.pendingNames("").Total; n != 1 {
		t.Errorf("a refused request changed something: %d pending", n)
	}
	if code := s.putName(999999, `{"undivided":true}`); code != 404 {
		t.Errorf("unknown person: %d, want 404", code)
	}
}

func TestNamesDealtWith_AreListedToBeChangedOrTakenBack(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.addWork("Cólera", "Gabriel García Márquez", "b.epub", "epub")
	s.addWork("Relatório", "Médicos Sem Fronteiras", "c.epub", "epub")
	s.addWork("Sem divisão", "Ana Maria Lima", "d.epub", "epub")
	gone := s.addWork("Retirada", "Autor Retirado", "e.epub", "epub")
	s.putName(s.personID("Frank Herbert"), `{"family":"Herbert","given":"Frank"}`)
	s.putName(s.personID("Gabriel García Márquez"), `{"family":"García Márquez","given":"Gabriel"}`)
	s.putName(s.personID("Médicos Sem Fronteiras"), `{"undivided":true}`)
	s.putName(s.personID("Autor Retirado"), `{"family":"Retirado","given":"Autor"}`)
	s.retire(gone)

	done := s.pendingNames("?state=done")
	if got := strings.Join(pendingNamesOf(done), "|"); got != "Frank Herbert|Gabriel García Márquez|Médicos Sem Fronteiras" {
		t.Fatalf("dealt with = %s (not the one still to divide, nor the one of a retired work)", got)
	}
	by := map[string]people.NameRow{}
	for _, n := range done.Data {
		by[n.Name] = n
	}
	if n := by["Gabriel García Márquez"]; n.Family != "García Márquez" || n.Given != "Gabriel" || n.Undivided || n.Suggestion != nil {
		t.Errorf("García Márquez = %+v", n)
	}
	if n := by["Médicos Sem Fronteiras"]; !n.Undivided || n.Family != "" || n.Given != "" {
		t.Errorf("Médicos Sem Fronteiras = %+v", n)
	}
	// The other list is the opposite, and any state that is not "done" is the default.
	if got := strings.Join(pendingNamesOf(s.pendingNames("?state=pending")), "|"); got != "Ana Maria Lima" {
		t.Errorf("pending = %s", got)
	}
	if got := strings.Join(pendingNamesOf(s.pendingNames("?state=x")), "|"); got != "Ana Maria Lima" {
		t.Errorf("an unknown state = %s", got)
	}
	// A search narrows this list too, and a division changed there is what the list says next.
	if got := pendingNamesOf(s.pendingNames("?state=done&q=garcia")); len(got) != 1 || got[0] != "Gabriel García Márquez" {
		t.Errorf("search = %v", got)
	}
	s.putName(s.personID("Gabriel García Márquez"), `{"family":"Márquez","given":"Gabriel García"}`)
	if n := s.pendingNames("?state=done&q=garcia").Data[0]; n.Family != "Márquez" || n.Given != "Gabriel García" {
		t.Errorf("after changing = %+v", n)
	}
	// Taking one back returns it to the list to divide.
	s.putName(s.personID("Frank Herbert"), `{"family":"","given":""}`)
	if got := strings.Join(pendingNamesOf(s.pendingNames("?state=done")), "|"); got != "Gabriel García Márquez|Médicos Sem Fronteiras" {
		t.Errorf("after taking one back = %s", got)
	}
	if got := strings.Join(pendingNamesOf(s.pendingNames("")), "|"); got != "Ana Maria Lima|Frank Herbert" {
		t.Errorf("to divide after taking one back = %s", got)
	}
}

func TestPendingNames_APersonWhoIsAuthorAndIllustratorOfAWorkCountsThatWorkOnce(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna ilustrada", "Frank Herbert", "a.epub", "epub")
	s.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'illustrator', 2 FROM person WHERE name = 'Frank Herbert'`, work)
	got := s.pendingNames("")
	if len(got.Data) != 1 || got.Data[0].Works != 1 {
		t.Errorf("%+v", got.Data)
	}
	if got.Total != 1 {
		t.Errorf("total = %d", got.Total)
	}
}
