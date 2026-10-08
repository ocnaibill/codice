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
	s.exec(`UPDATE editions SET title = 'Dune', title_manual = TRUE WHERE id = $1`, en)
	s.exec(`UPDATE editions SET title = 'Dune', title_manual = TRUE WHERE id = $1`, es) // the same title twice: once
	s.exec(`UPDATE editions SET title = 'DUNA', title_manual = TRUE WHERE id = $1`, same)
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
	if got[1].ID != 0 || got[1].EditionID != int64(en) {
		t.Errorf("an edition title has no id of its own, and says its edition: %+v (edition %d)", got[1], en)
	}
}

func (s *catalogStack) editEdition(a actor, work int, edition int64, body string) (int, map[string]any) {
	s.t.Helper()
	rec := s.do(a, "PATCH", fmt.Sprintf("/works/%d/editions/%d", work, edition), body)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (s *catalogStack) findEdition(work int, id int64) Edition {
	s.t.Helper()
	w, _ := s.detail(admin, work)
	for _, e := range w.Editions {
		if int64(e.ID) == id {
			return e
		}
	}
	s.t.Fatalf("edition %d not in the work %d", id, work)
	return Edition{}
}

func TestWorkTitles_TheTitleAnEditionGotFromTheFileIsNotANameToShowButIsFound(t *testing.T) {
	s := newCatalogStack(t)
	nuvem := s.addWork("A Nuvem 2", "Neal Shusterman", "nuvem.epub", "epub")
	pdf := s.addEdition(nuvem, "pt")
	s.exec(`UPDATE editions SET title = 'A Nuvem 2 - Neal Shusterman.pdf' WHERE id = $1`, pdf)
	s.exec(`UPDATE editions SET title = 'A Nuvem 2 - Neal Shusterman.epub' WHERE work_id = $1 AND is_primary`, nuvem)
	if got := s.altTitles(nuvem); len(got) != 0 {
		t.Errorf("the titles the files brought are not shown as other names: %v", titleList(got))
	}
	if e := s.findEdition(nuvem, int64(pdf)); e.TitleSet || e.Title != "A Nuvem 2 - Neal Shusterman.pdf" {
		t.Errorf("the edition says what it has and that nobody wrote it: %+v", e)
	}
	// The search still finds the work by it.
	if got := ids(s.list(ana, "?search=Shusterman.pdf").Data); !sameIDs(got, nuvem) {
		t.Errorf("search by the title of the file = %v", got)
	}
}

func TestWorkTitles_OwnerAndAdminWriteTheTitleOfAnEditionAndItBecomesAnotherName(t *testing.T) {
	s := newCatalogStack(t)
	nuvem := s.addWork("A Nuvem 2", "Neal Shusterman", "nuvem.epub", "epub")
	en := s.addEdition(nuvem, "en")
	s.exec(`UPDATE editions SET title = 'file.en.epub' WHERE id = $1`, en)

	code, out := s.editEdition(admin, nuvem, int64(en), `{"title":"  The   Cloud 2 "}`)
	if code != 200 || out["title"] != "The Cloud 2" || out["titleSet"] != true || int64(out["id"].(float64)) != int64(en) {
		t.Fatalf("edit: %d %v", code, out)
	}
	e := s.findEdition(nuvem, int64(en))
	if e.Title != "The Cloud 2" || !e.TitleSet || e.Language != "en" {
		t.Errorf("the edition = %+v (its language is not touched)", e)
	}
	got := s.altTitles(nuvem)
	if len(got) != 1 || got[0].Title != "The Cloud 2" || got[0].Language != "en" || got[0].Source != "edition" || got[0].EditionID != int64(en) {
		t.Errorf("the other names = %+v", got)
	}
	if n := s.scalar(`SELECT details->>'previous' FROM audit_log WHERE action = 'work.edition_title'`); n != "file.en.epub" {
		t.Errorf("the audit entry keeps what it was: %q", n)
	}
	// The search finds the work by the new title.
	if got := ids(s.list(ana, "?search=Cloud").Data); !sameIDs(got, nuvem) {
		t.Errorf("search by the new title = %v", got)
	}
	// Writing it again changes it, and the primary edition can be written too.
	if code, _ := s.editEdition(admin, nuvem, int64(en), `{"title":"The Cloud, book 2"}`); code != 200 || s.findEdition(nuvem, int64(en)).Title != "The Cloud, book 2" {
		t.Errorf("writing again: %d", code)
	}
	primary := s.primaryEdition(nuvem)
	if code, _ := s.editEdition(admin, nuvem, primary, `{"title":"A Nuvem II"}`); code != 200 {
		t.Errorf("the primary edition: %d", code)
	}
	// One that says the same as the main title is a name the work already has: it is set, and not listed again.
	if code, _ := s.editEdition(admin, nuvem, int64(en), `{"title":"a nuvem 2"}`); code != 200 || !s.findEdition(nuvem, int64(en)).TitleSet {
		t.Errorf("the main title: %d", code)
	}
	for _, a := range s.altTitles(nuvem) {
		if a.EditionID == int64(en) {
			t.Errorf("a title equal to the main one is listed: %+v", a)
		}
	}
}

func (s *catalogStack) primaryEdition(work int) int64 {
	s.t.Helper()
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM editions WHERE work_id = $1 AND is_primary`, work).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func TestWorkTitles_TheTitleOfAnEditionIsRefusedWhenItIsEmptyTooLongOrNotOfTheWork(t *testing.T) {
	s := newCatalogStack(t)
	nuvem := s.addWork("A Nuvem 2", "Neal Shusterman", "nuvem.epub", "epub")
	other := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	en := s.addEdition(nuvem, "en")
	for _, body := range []string{`{"title":""}`, `{"title":"   "}`, `{"title":"!!!"}`, `{}`, `{"title":"` + strings.Repeat("x", 256) + `"}`, `not json`} {
		if code, _ := s.editEdition(admin, nuvem, int64(en), body); code != 400 {
			t.Errorf("%.30s: %d, want 400", body, code)
		}
	}
	if code, _ := s.editEdition(admin, nuvem, int64(en), `{"title":"`+strings.Repeat("x", 255)+`"}`); code != 200 {
		t.Errorf("255 characters: %d", code)
	}
	if code, _ := s.editEdition(admin, nuvem, int64(en), `{"title":"`+strings.Repeat("é", 255)+`"}`); code != 200 {
		t.Errorf("255 characters that are two bytes each: %d", code)
	}
	// An edition of another work, one that does not exist and addresses that are not numbers are not found.
	for _, target := range []string{
		fmt.Sprintf("/works/%d/editions/%d", other, en), fmt.Sprintf("/works/%d/editions/99999", nuvem), fmt.Sprintf("/works/99999/editions/%d", en),
		"/works/abc/editions/1", fmt.Sprintf("/works/%d/editions/abc", nuvem), fmt.Sprintf("/works/%d/editions/0", nuvem),
	} {
		if rec := s.do(admin, "PATCH", target, `{"title":"Novo"}`); rec.Code != 404 {
			t.Errorf("PATCH %s: %d, want 404", target, rec.Code)
		}
	}
	if s.findEdition(nuvem, int64(en)).Title == "Novo" {
		t.Error("an edition of another work was written")
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.edition_title'`); n != "2" {
		t.Errorf("only the two writes that were made are in the audit: %s", n)
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

func TestWorkTitles_TheCardCarriesTheTitleWrittenForTheEditionBeingRead(t *testing.T) {
	s := newCatalogStack(t)
	nuvem := s.addWork("A Nuvem 2", "Neal Shusterman", "nuvem.epub", "epub")
	pt := s.primaryFile(nuvem)
	en := s.addEdition(nuvem, "en")
	enFile := s.addFile(en, "epub", "cloud.epub", "managed")
	s.exec(`UPDATE editions SET title = 'The Cloud 2', title_manual = TRUE WHERE id = $1`, en)
	card := func(a actor) Work {
		l := s.list(a, "")
		if len(l.Data) != 1 {
			t.Fatalf("the list = %v", ids(l.Data))
		}
		return l.Data[0]
	}
	if w := card(ana); w.Continue != nil || w.Title != "A Nuvem 2" {
		t.Errorf("nobody read: %+v %q", w.Continue, w.Title)
	}
	// Reading the English edition: the card says its title; the main title stays what it is.
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 30)`, idAna, enFile)
	if w := card(ana); w.Continue == nil || w.Continue.Title != "The Cloud 2" || w.Title != "A Nuvem 2" {
		t.Errorf("reading the English one: %+v %q", w.Continue, w.Title)
	}
	// It is the reader's: another person reading the Portuguese edition, whose title nobody wrote, has none.
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 30)`, idBob, pt)
	if w := card(bob); w.Continue == nil || w.Continue.Title != "" || w.Title != "A Nuvem 2" {
		t.Errorf("bob in the Portuguese edition: %+v %q", w.Continue, w.Title)
	}
	if w := card(ana); w.Continue.Title != "The Cloud 2" {
		t.Errorf("ana keeps hers: %+v", w.Continue)
	}
	// A title the file brought is not a name to show: with it only found, the card says none.
	s.exec(`UPDATE editions SET title_manual = FALSE WHERE id = $1`, en)
	if w := card(ana); w.Continue.Title != "" {
		t.Errorf("a title nobody wrote: %q", w.Continue.Title)
	}
	// The detail says the same.
	s.exec(`UPDATE editions SET title_manual = TRUE WHERE id = $1`, en)
	if w, _ := s.detail(ana, nuvem); w.Continue == nil || w.Continue.Title != "The Cloud 2" {
		t.Errorf("detail: %+v", w.Continue)
	}
}
