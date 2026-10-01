package handlers

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Search by the stem of a word, in the language of the edition (#41).

func (s *catalogStack) setLanguage(work int, language string) {
	s.t.Helper()
	if language == "" {
		s.exec(`UPDATE editions SET language = NULL WHERE id = (SELECT edition_id FROM work_primary WHERE work_id = $1)`, work)
		return
	}
	s.exec(`UPDATE editions SET language = $2 WHERE id = (SELECT edition_id FROM work_primary WHERE work_id = $1)`, work, language)
}

func (s *catalogStack) works(query string) string {
	s.t.Helper()
	_, r := s.search(ana, q(query))
	ids := map[int]bool{}
	for _, h := range r.Data {
		ids[h.WorkID] = true
	}
	out := []string{}
	for _, h := range r.Data {
		if ids[h.WorkID] {
			out = append(out, fmt.Sprint(h.WorkID))
			delete(ids, h.WorkID)
		}
	}
	return strings.Join(out, ",")
}

const portugueseText = "A corrida de rua é uma corrida longa; ele gosta de correr todos os dias. Informações importantes sobre a ação."

func TestSearch_FindsOtherFormsOfAWordInTheLanguageOfTheEdition(t *testing.T) {
	s := newCatalogStack(t)
	pt := s.addWork("Corridas", "Ana", "c.epub", "epub")
	en := s.addWork("Running", "Bob", "r.epub", "epub")
	s.setLanguage(pt, "pt")
	s.setLanguage(en, "en")
	s.index(s.primaryFile(pt), segment{text: portugueseText})
	s.index(s.primaryFile(en), segment{text: "He was running to the station, and they run there every day."})

	for query, want := range map[string]string{
		"correr":        fmt.Sprint(pt), // the verb finds the noun
		"corridas":      fmt.Sprint(pt),
		"corrida":       fmt.Sprint(pt),
		"informação":    fmt.Sprint(pt), // with its accent, the plural is found too ("informações")
		"ação":          fmt.Sprint(pt),
		"run":           fmt.Sprint(en),
		"runs":          fmt.Sprint(en),
		"running":       fmt.Sprint(en),
		"correr -longa": "",
	} {
		if got := s.works(query); got != want {
			t.Errorf("%q found in works %q, want %q", query, got, want)
		}
	}
}

func TestSearch_QuotesMakeTheWholeSearchExact(t *testing.T) {
	s := newCatalogStack(t)
	pt := s.addWork("Corridas", "Ana", "c.epub", "epub")
	s.setLanguage(pt, "pt")
	s.index(s.primaryFile(pt), segment{text: portugueseText}, segment{text: "Quem sabe correr sabe voar."})

	count := func(query string) int { _, r := s.search(ana, q(query)); return len(r.Data) }
	for query, want := range map[string]int{
		`correr`:            2, // by stem: the noun in the first passage, the verb in the second
		`"correr"`:          2, // the word as written: it is in both ("gosta de correr", "sabe correr")
		`"corridas"`:        0, // not written anywhere: the stem would have found it
		`corridas`:          2, // the stem of the noun is the stem of the verb
		`"uma corrida"`:     1,
		`"uma corridas"`:    0, // a phrase is exact too
		`"corrida" longa`:   1, // one quote is enough to make it all exact
		`"sabe correr" voo`: 0,
	} {
		if got := count(query); got != want {
			t.Errorf("%s: %d hits, want %d", query, got, want)
		}
	}

	var out struct{ Mode string }
	rec := s.do(ana, "GET", "/search?"+q("correr"), "")
	if !strings.Contains(rec.Body.String(), `"mode":"stem"`) {
		t.Errorf("a search by stem says so: %s", rec.Body.String())
	}
	rec = s.do(ana, "GET", "/search?"+q(`"correr"`), "")
	if !strings.Contains(rec.Body.String(), `"mode":"exact"`) {
		t.Errorf("a search between quotes says it is exact: %s", rec.Body.String())
	}
	_ = out
}

func TestSearch_ByStemAddsHitsAndNeverTakesOneAway(t *testing.T) {
	s := newCatalogStack(t)
	pt := s.addWork("Corridas", "Ana", "c.epub", "epub")
	s.setLanguage(pt, "pt")
	s.index(s.primaryFile(pt),
		segment{text: "Nada acontece sem AÇÃO. A ação e a reação são iguais."},
		segment{text: "O rei morreu e o povo chorou na praça."},
		segment{text: "O povo riu na praça, e o rei sorriu."},
	)
	count := func(query string) int { _, r := s.search(ana, q(query)); return len(r.Data) }
	for query, want := range map[string]int{
		"acao":         1, // typed without the accent, as it has always worked
		"AÇÃO":         1,
		"povo":         2,
		"rei povo":     2,
		`rei OR praça`: 2,
		"povo -rei":    0,
		"o":            2, // no word is dropped as too common, in any language (the first passage has no "o")
		"de a o":       0, // a stopword is a word like any other here
		"inexistente":  0,
		"!!! ???":      0,
	} {
		if got := count(query); got != want {
			t.Errorf("%q: %d hits, want %d", query, got, want)
		}
	}
	// "o" is in all of them: a query made only of what the language calls stopwords still finds.
	if got := count("o povo"); got != 2 {
		t.Errorf("o povo: %d", got)
	}
}

func TestSearch_AFileOfAnotherLanguageOrOfNoneIsSearchedAsItAlwaysWas(t *testing.T) {
	s := newCatalogStack(t)
	for _, language := range []string{"", "de", "ja", "xx"} {
		work := s.addWork("Livro "+language, "Ana", "l"+language+".epub", "epub")
		s.setLanguage(work, language)
		s.index(s.primaryFile(work), segment{text: "Ele gosta de correr e de ver a corrida. Informações importantes."})
	}
	count := func(query string) int { _, r := s.search(ana, q(query)); return len(r.Data) }
	if got := count("correr"); got != 4 {
		t.Errorf("the word as it is written is found in all of them: %d", got)
	}
	if got := count("corridas"); got != 0 {
		t.Errorf("no stem where the language is not known: %d hits", got)
	}
	if got := count("informacoes"); got != 4 {
		t.Errorf("without accents, as it has always worked: %d hits", got)
	}
}

func TestSearch_TheLanguageIsReadTheWayAFileDeclaresIt(t *testing.T) {
	s := newCatalogStack(t)
	for _, language := range []string{"pt", "pt-BR", "PT_pt", "por", "Pt-br"} {
		work := s.addWork("Livro "+language, "Ana", "l"+language+".epub", "epub")
		s.setLanguage(work, language)
		s.index(s.primaryFile(work), segment{text: "A corrida começou."})
		if got := s.works("correr"); !strings.Contains(","+got+",", fmt.Sprintf(",%d,", work)) {
			t.Errorf("language %q: stem not applied (found in %q)", language, got)
		}
	}
	for _, c := range []struct{ language, config string }{
		{"pt", "codice_portuguese"}, {"en-US", "codice_english"}, {"eng", "codice_english"}, {"es", "codice_spanish"},
		{"fre", "codice_french"}, {"fr-CA", "codice_french"}, {"it", "codice_italian"}, {"cat", "codice_catalan"},
		{"ro", "codice_romanian"}, {"rum", "codice_romanian"}, {"de", "codice_simple"}, {"", "codice_simple"}, {"  ", "codice_simple"},
	} {
		if got := s.scalar(fmt.Sprintf(`SELECT codice_search_config('%s')::text`, c.language)); got != c.config {
			t.Errorf("codice_search_config(%q) = %s, want %s", c.language, got, c.config)
		}
	}
	if got := s.scalar(`SELECT codice_search_config(NULL)::text`); got != "codice_simple" {
		t.Errorf("no language: %s", got)
	}
}

func TestSearch_FollowsTheLanguageOfTheEditionWhenItChanges(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Livro", "Ana", "l.epub", "epub")
	file := s.primaryFile(work)
	s.index(file, segment{text: "A corrida começou cedo."})

	count := func() int { _, r := s.search(ana, q("correr")); return len(r.Data) }
	if got := count(); got != 0 {
		t.Fatalf("language unknown: no stem, %d hits", got)
	}
	// An administrator confirms the language: the text already extracted is found by stem, without extracting it again.
	s.setLanguage(work, "pt")
	if got := count(); got != 1 {
		t.Errorf("language set after the text was indexed: %d hits, want 1", got)
	}
	// A segment written after that, by a worker that knows nothing of this, is indexed the same way.
	s.index(file, segment{text: "A corrida começou cedo."}, segment{text: "Ele quer correr."})
	if got := count(); got != 2 {
		t.Errorf("a new generation: %d hits, want 2", got)
	}
	s.setLanguage(work, "de")
	if got := count(); got != 1 {
		t.Errorf("language changed to one without a stemmer: %d hits, want only the exact word", got)
	}
	s.setLanguage(work, "")
	if got := count(); got != 1 {
		t.Errorf("language cleared: %d hits", got)
	}
}

func TestSearch_AFileThatMovesToAnotherEditionTakesItsLanguage(t *testing.T) {
	s := newCatalogStack(t)
	pt := s.addWork("Português", "Ana", "p.epub", "epub")
	other := s.addWork("Sem idioma", "Ana", "o.epub", "epub")
	s.setLanguage(pt, "pt")
	moved := s.primaryFile(other)
	s.index(moved, segment{text: "A corrida começou cedo."})
	if got := s.works("correr"); got != "" {
		t.Fatalf("before: %q", got)
	}
	s.exec(`UPDATE files SET edition_id = (SELECT edition_id FROM work_primary WHERE work_id = $1) WHERE id = $2`, pt, moved)
	// The file now belongs to the edition (and so the work) that has a language.
	if got := s.works("correr"); got != fmt.Sprint(pt) {
		t.Errorf("after joining the editions, the text is found by stem, in the work it moved to: %q", got)
	}
}

func TestSearch_ByStemStillOnlyShowsWhatCanBeOpened(t *testing.T) {
	s := newCatalogStack(t)
	live := s.addWork("Aberto", "Ana", "a.epub", "epub")
	retired := s.addWork("Aposentado", "Ana", "r.epub", "epub")
	missing := s.addWork("Sumiu", "Ana", "m.epub", "epub")
	for _, w := range []int{live, retired, missing} {
		s.setLanguage(w, "pt")
		s.index(s.primaryFile(w), segment{text: "A corrida começou."})
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retired)
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, s.primaryFile(missing))
	if got := s.works("correr"); got != fmt.Sprint(live) {
		t.Errorf("found in %q, want only the work that can be opened (%d)", got, live)
	}
}

func TestSearch_ByStemKeepsToTheWorkAndFileAsked(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "Ana", "a.epub", "epub")
	b := s.addWork("B", "Ana", "b.epub", "epub")
	for _, w := range []int{a, b} {
		s.setLanguage(w, "pt")
		s.index(s.primaryFile(w), segment{text: "A corrida começou."})
	}
	for _, c := range []struct {
		filter string
		want   int
	}{
		{fmt.Sprintf("workId=%d", a), 1}, {fmt.Sprintf("fileId=%d", s.primaryFile(b)), 1},
		{fmt.Sprintf("workId=%d&fileId=%d", a, s.primaryFile(b)), 0}, {"", 2},
	} {
		_, r := s.search(ana, q("correr")+"&"+c.filter)
		if len(r.Data) != c.want {
			t.Errorf("%q: %d hits, want %d", c.filter, len(r.Data), c.want)
		}
	}
}

func TestSearch_AHitThatMatchesBothWaysIsOneHitAndMarksTheWordsFound(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Corridas", "Ana", "c.epub", "epub")
	s.setLanguage(work, "pt")
	s.index(s.primaryFile(work), segment{text: "Ele gosta de correr. A corrida de ontem foi longa. Os corredores descansaram."})

	_, r := s.search(ana, q("correr"))
	if len(r.Data) != 1 {
		t.Fatalf("one passage, one hit: %d", len(r.Data))
	}
	hit := r.Data[0]
	runes := []rune(hit.Snippet)
	var found []string
	for _, m := range hit.Matches {
		found = append(found, string(runes[m[0]:m[1]]))
	}
	// The words marked are the ones in the text, not the ones asked for.
	if strings.Join(found, ",") != "correr,corrida" {
		t.Errorf("words marked: %q in %q", found, hit.Snippet)
	}
	// The exact word, written as asked, is also found by the exact search, once.
	_, r = s.search(ana, q("corrida"))
	if len(r.Data) != 1 {
		t.Errorf("a passage that matches by stem and as written: %d hits", len(r.Data))
	}
}

func TestSearch_TheMigrationGivesTheTextAlreadyThereTheLanguageOfItsEdition(t *testing.T) {
	s := newCatalogStack(t)
	pt := s.addWork("Português", "Ana", "p.epub", "epub")
	none := s.addWork("Sem idioma", "Ana", "n.epub", "epub")
	s.setLanguage(pt, "pt")
	s.index(s.primaryFile(pt), segment{text: "A corrida começou."})
	s.index(s.primaryFile(none), segment{text: "A corrida começou."})
	// As it was before the migration: everything indexed as plain words.
	s.exec(`UPDATE document_segments SET search_config = 'codice_simple'`)
	if got := s.works("correr"); got != "" {
		t.Fatalf("before: %q", got)
	}
	// The statement of the migration itself, read from its file.
	file, err := os.ReadFile("../database/migrations/00034_search_stemming.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(file)
	start := strings.Index(text, "UPDATE document_segments s SET search_config")
	if start < 0 {
		t.Fatal("the backfill is not in the migration")
	}
	s.exec(text[start : start+strings.Index(text[start:], ";")])
	if got := s.works("correr"); got != fmt.Sprint(pt) {
		t.Errorf("after: found in %q, want only the work with a language (%d)", got, pt)
	}
}
