package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func (s *catalogStack) addTitle(a actor, work int, body string) (AlternativeTitle, *httptest.ResponseRecorder) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/works/%d/titles", work), body)
	var out AlternativeTitle
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec
}

func (s *catalogStack) altTitles(work int) []AlternativeTitle {
	s.t.Helper()
	w, code := s.detail(admin, work)
	if code != 200 || w.Metadata == nil {
		s.t.Fatalf("detail of %d: %d", work, code)
	}
	return w.Metadata.AlternativeTitles
}

func titleList(ts []AlternativeTitle) string {
	var out []string
	for _, t := range ts {
		out = append(out, fmt.Sprintf("%s|%s|%s", t.Title, t.Language, t.Source))
	}
	return strings.Join(out, ";")
}

func TestWorkTitles_AddingKeepsAnotherNameAndShowsItWithItsLanguageAndOrigin(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	got, rec := s.addTitle(admin, duna, `{"title":"  Dune  ","language":"en"}`)
	if rec.Code != 201 || got.ID == 0 || got.Title != "Dune" || got.Language != "en" || got.Source != "manual" {
		t.Fatalf("add: %d %+v", rec.Code, got)
	}
	// The language is optional.
	if _, rec := s.addTitle(admin, duna, `{"title":"Arrakis"}`); rec.Code != 201 {
		t.Errorf("without a language: %d", rec.Code)
	}
	for _, lang := range []string{"pt-BR", "zh_Hant", "ja"} {
		if _, rec := s.addTitle(admin, duna, fmt.Sprintf(`{"title":"Nome %s","language":%q}`, lang, lang)); rec.Code != 201 {
			t.Errorf("language %s: %d", lang, rec.Code)
		}
	}
	if got := titleList(s.altTitles(duna)); !strings.HasPrefix(got, "Dune|en|manual;Arrakis||manual;") {
		t.Errorf("alternative titles = %s", got)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.title_add'`); n != "5" {
		t.Errorf("audit entries = %s, want 5", n)
	}
}

func TestWorkTitles_ATitleIsKeptWithItsSpacesFixedAndNoLanguageIsNone(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	got, rec := s.addTitle(admin, duna, "{\"title\":\"  O   planeta \\t  do deserto \"}")
	if rec.Code != 201 || got.Title != "O planeta do deserto" {
		t.Fatalf("add: %d %+v", rec.Code, got)
	}
	if n := s.scalar(`SELECT title || '|' || (language IS NULL)::text FROM work_titles WHERE id = $1`, got.ID); n != "O planeta do deserto|true" {
		t.Errorf("stored = %q, want the spaces fixed and no language kept as none", n)
	}
	// An edition with no title says nothing.
	ed := s.addEdition(duna, "es")
	s.exec(`UPDATE editions SET title = '' WHERE id = $1`, ed)
	for _, a := range s.altTitles(duna) {
		if a.Title == "" {
			t.Errorf("an edition with no title is listed: %+v", a)
		}
	}
}

func TestWorkTitles_TwoTitlesAddedAtTheSameTimeWaitForEachOther(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Somebody else has the work in hand (they are in the middle of adding a title). The foreign key of the title would not wait for
	// this, which is the kind of lock that does not stop a title going in; adding one must wait all the same, to count them right.
	if _, err := tx.Exec(`SELECT 1 FROM works WHERE id = $1 FOR NO KEY UPDATE`, duna); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		_, rec := s.addTitle(admin, duna, `{"title":"Dune"}`)
		done <- rec.Code
	}()
	select {
	case code := <-done:
		t.Fatalf("the second title did not wait for the first (answered %d)", code)
	case <-time.After(400 * time.Millisecond):
	}
	tx.Commit()
	if code := <-done; code != 201 {
		t.Errorf("after the wait: %d, want 201", code)
	}
}

func TestWorkTitles_ATitleTheWorkGoesByAlreadyIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`UPDATE editions SET title = 'Duna: edição de bolso', language = 'pt' WHERE work_id = $1`, duna)
	ed := s.addEdition(duna, "en")
	s.exec(`UPDATE editions SET title = 'Dune (English edition)' WHERE id = $1`, ed)
	s.addTitle(admin, duna, `{"title":"Arrakis"}`)

	for name, body := range map[string]string{
		"the main title, written another way": `{"title":"  DUNA!  "}`,
		"the main title with accents folded":  `{"title":"dúna"}`,
		"the title of an edition":             `{"title":"Dune"}`,
		"the title of the primary edition":    `{"title":"duna"}`,
		"an alternative title":                `{"title":"arrakis"}`,
	} {
		if _, rec := s.addTitle(admin, duna, body); rec.Code != 409 {
			t.Errorf("%s: %d, want 409", name, rec.Code)
		}
	}
	if n := s.scalar(`SELECT COUNT(*) FROM work_titles WHERE work_id = $1`, duna); n != "1" {
		t.Errorf("titles kept = %s, want 1", n)
	}
	// What is not a title, or not a language, is not accepted.
	for name, body := range map[string]string{
		"empty":         `{"title":""}`,
		"spaces":        `{"title":"   "}`,
		"punctuation":   `{"title":"!!!"}`,
		"too long":      fmt.Sprintf(`{"title":%q}`, strings.Repeat("x", 513)),
		"not json":      `nope`,
		"bad language":  `{"title":"Outro","language":"portuguese"}`,
		"worse":         `{"title":"Outro","language":"p"}`,
		"with a space":  `{"title":"Outro","language":"pt BR"}`,
		"a long suffix": `{"title":"Outro","language":"pt-abcdefghij"}`,
	} {
		if _, rec := s.addTitle(admin, duna, body); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if _, rec := s.addTitle(admin, 99999, `{"title":"Outro"}`); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "POST", "/works/abc/titles", `{"title":"x"}`); rec.Code != 404 {
		t.Errorf("bad id: %d, want 404", rec.Code)
	}
	// The same name may be of another work.
	other := s.addWork("Dune", "Outro", "d.epub", "epub")
	if _, rec := s.addTitle(admin, other, `{"title":"Arrakis"}`); rec.Code != 201 {
		t.Errorf("a name another work has: %d, want 201", rec.Code)
	}
}

func TestWorkTitles_TheTitlesOfTheEditionsAreAlternativeTitlesToo(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	en := s.addEdition(duna, "en")
	es := s.addEdition(duna, "es")
	same := s.addEdition(duna, "pt")
	s.exec(`UPDATE editions SET title = 'Dune' WHERE id = $1`, en)
	s.exec(`UPDATE editions SET title = 'Dune' WHERE id = $1`, es) // the same title twice: once
	s.exec(`UPDATE editions SET title = 'DUNA' WHERE id = $1`, same)
	s.addTitle(admin, duna, `{"title":"Arrakis"}`)
	s.exec(`INSERT INTO work_titles (work_id, title, language, source, title_key) VALUES ($1, 'Dune', NULL, 'openlibrary', 'dune')`, duna)

	got := s.altTitles(duna)
	// Kept for the work first; the title of an edition appears only if the work does not go by it already.
	if titleList(got) != "Arrakis||manual;Dune||openlibrary" {
		t.Errorf("alternative titles = %s", titleList(got))
	}
	s.exec(`DELETE FROM work_titles WHERE source = 'openlibrary'`)
	got = s.altTitles(duna)
	if titleList(got) != "Arrakis||manual;Dune|en|edition" {
		t.Errorf("alternative titles = %s", titleList(got))
	}
	if got[1].ID != 0 {
		t.Errorf("an edition title has no id of its own: %+v", got[1])
	}
}

func TestWorkTitles_RemovingIsOfTheWorkAndOfTheKeptTitlesOnly(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	other := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	a, _ := s.addTitle(admin, duna, `{"title":"Dune"}`)
	b, _ := s.addTitle(admin, other, `{"title":"Neuromante"}`)

	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/titles/%d", duna, b.ID), ""); rec.Code != 404 {
		t.Errorf("the title of another work: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/titles/%d", duna, a.ID), ""); rec.Code != 204 {
		t.Fatalf("remove: %d", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/titles/%d", duna, a.ID), ""); rec.Code != 404 {
		t.Errorf("removing twice: %d, want 404", rec.Code)
	}
	for _, target := range []string{"/works/abc/titles/1", fmt.Sprintf("/works/%d/titles/abc", duna), fmt.Sprintf("/works/%d/titles/0", duna)} {
		if rec := s.do(admin, "DELETE", target, ""); rec.Code != 404 {
			t.Errorf("DELETE %s: %d, want 404", target, rec.Code)
		}
	}
	if len(s.altTitles(duna)) != 0 || len(s.altTitles(other)) != 1 {
		t.Errorf("after removing: %v / %v", s.altTitles(duna), s.altTitles(other))
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.title_remove'`); n != "1" {
		t.Errorf("audit entries = %s, want 1", n)
	}
	// The titles go with the work.
	s.exec(`DELETE FROM works WHERE id = $1`, other)
	if n := s.scalar(`SELECT COUNT(*) FROM work_titles`); n != "0" {
		t.Errorf("titles left after the work went: %s", n)
	}
}

func TestWorkTitles_AWorkKeepsAtMostFiftyOtherNames(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`INSERT INTO work_titles (work_id, title, title_key) SELECT $1, 'Nome ' || g, 'nome ' || g FROM generate_series(1, 50) g`, duna)
	if _, rec := s.addTitle(admin, duna, `{"title":"Mais um"}`); rec.Code != 409 {
		t.Errorf("the 51st title: %d, want 409", rec.Code)
	}
	other := s.addWork("Outra", "X", "o.epub", "epub")
	if _, rec := s.addTitle(admin, other, `{"title":"Mais um"}`); rec.Code != 201 {
		t.Errorf("the bound is of each work: %d, want 201", rec.Code)
	}
}

func TestWorkTitles_TheSearchFindsAWorkByAnyOfItsNames(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	nada := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	ed := s.addEdition(duna, "en")
	s.exec(`UPDATE editions SET title = 'Dune Messiah' WHERE id = $1`, ed)
	s.addTitle(admin, duna, `{"title":"Arrakis, o planeta do deserto"}`)

	for q, want := range map[string][]int{
		"?search=duna":         {duna},
		"?search=arrakis":      {duna}, // an alternative title
		"?search=ARRAKIS":      {duna}, // whatever the case
		"?search=messiah":      {duna}, // the title of an edition
		"?search=planeta%20do": {duna},
		"?search=neuro":        {nada},
		"?search=nenhum":       {},
	} {
		if got := ids(s.list(ana, q).Data); !sameIDs(got, want...) {
			t.Errorf("GET /works%s = %v, want %v", q, got, want)
		}
	}
	// The count and the page agree, and a work with several names that match is listed once.
	s.addTitle(admin, duna, `{"title":"Duna e Dune"}`)
	l := s.list(ana, "?search=dun")
	if l.Total != 1 || len(l.Data) != 1 {
		t.Errorf("one work, matched by three names: total %d, listed %d", l.Total, len(l.Data))
	}
	// A work in the trash is not found by any of them.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, duna)
	if got := ids(s.list(ana, "?search=arrakis").Data); len(got) != 0 {
		t.Errorf("a retired work was found: %v", got)
	}
}

func TestWorkTitles_TheOPDSSearchFindsAWorkByAnyOfItsNames(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	s.addTitle(admin, duna, `{"title":"Arrakis"}`)
	if body := s.do(ana, "GET", "/opds/search?q=arrakis", "").Body.String(); !strings.Contains(body, "Duna") || strings.Contains(body, "Neuromancer") {
		t.Errorf("OPDS search for an alternative title: %s", body)
	}
}
