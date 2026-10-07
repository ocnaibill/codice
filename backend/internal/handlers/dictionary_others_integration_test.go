package handlers

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

type othersResult struct {
	Word    string `json:"word"`
	Lang    string `json:"lang"`
	Results []struct {
		Lang   string       `json:"lang"`
		Result lookupResult `json:"result"`
	} `json:"results"`
}

func others(t *testing.T, s *catalogStack, lang, word, prefer string) othersResult {
	t.Helper()
	q := "/dictionary/others?lang=" + url.QueryEscape(lang) + "&word=" + url.QueryEscape(word)
	if prefer != "" {
		q += "&prefer=" + prefer
	}
	rec := s.do(ana, "GET", q, "")
	if rec.Code != 200 {
		t.Fatalf("others %s %s: %d %s", lang, word, rec.Code, rec.Body.String())
	}
	var res othersResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func langsOf(res othersResult) string {
	var out []string
	for _, r := range res.Results {
		out = append(out, r.Lang)
	}
	return strings.Join(out, ",")
}

// French and Spanish entries besides the Portuguese ones of stockedDictionary.
func stockedOthers(t *testing.T, s *catalogStack) {
	t.Helper()
	stockedDictionary(t, s)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, entries) VALUES ('wikt-fr', 'ready', 'x', 3)`)
	entry := func(pkg, lang, word, norm, pos, data string) {
		s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ($1, $2, $3, $4, $5, $6::jsonb)`, pkg, lang, word, norm, pos, data)
	}
	entry("wikt-fr", "fr", "bonjour", "bonjour", "intj", `{"senses":[{"glosses":["salutation du matin"]}]}`)
	entry("wikt-fr", "fr", "livre", "livre", "noun", `{"senses":[{"glosses":["ouvrage imprimé"]}]}`)
	entry("wikt-fr", "es", "libro", "libro", "noun", `{"senses":[{"glosses":["obra impresa"]}]}`)
	entry("wikt-fr", "es", "livro", "livro", "noun", `{"senses":[{"glosses":["x"]}]}`)
}

func TestDictionaryOthers_FindsTheWordInTheLanguagesThatHaveIt(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	// the book is in Portuguese and the word is French: Portuguese has nothing, French has it
	if res := lookup(t, s, "pt", "bonjour"); len(res.Items) != 0 {
		t.Fatalf("pt has it: %+v", res.Items)
	}
	res := others(t, s, "pt", "bonjour", "pt")
	if res.Word != "bonjour" || res.Lang != "pt" || langsOf(res) != "fr" {
		t.Fatalf("others = %+v", res)
	}
	got := res.Results[0].Result
	if got.Lang != "fr" || got.Word != "bonjour" || !got.Installed || kinds(got) != "entry:bonjour:intj" {
		t.Fatalf("what was found: %+v", got)
	}
	if len(got.Sources) != 1 || got.Sources[0].Package != "wikt-fr" {
		t.Fatalf("where it comes from: %+v", got.Sources)
	}
}

func TestDictionaryOthers_LeavesOutTheLanguageThatWasAsked(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	// "livro" is Portuguese and Spanish (the fixtures say so): asked in Portuguese, only the others come
	if got := langsOf(others(t, s, "pt", "livro", "pt")); got != "es" {
		t.Fatalf("languages = %q, want es", got)
	}
	if got := langsOf(others(t, s, "es", "livro", "pt")); got != "pt" {
		t.Fatalf("languages = %q, want pt", got)
	}
}

func TestDictionaryOthers_ShowsEveryLanguageThatHasItInOrder(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	// "libro" is French "livre" no more: it is only Spanish; "livre" is French; a word in three: add it
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES
		('wikt-fr', 'fr', 'rio', 'rio', 'noun', '{"senses":[{"glosses":["fleuve"]}]}'),
		('wikt-fr', 'es', 'rio', 'rio', 'verb', '{"senses":[{"glosses":["forma de reír"]}]}'),
		('wikt-pt', 'pt', 'rio', 'rio', 'noun', '{"senses":[{"glosses":["curso de água"]}]}'),
		('wikt-pt', 'en', 'rio', 'rio', 'noun', '{"senses":[{"glosses":["the city"]}]}')`)
	if got := langsOf(others(t, s, "it", "rio", "pt")); got != "en,es,fr,pt" {
		t.Fatalf("languages = %q, want en,es,fr,pt (in order of their codes)", got)
	}
}

func TestDictionaryOthers_FindsAWordByAFormOrByATranslationToo(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	// "correndo" is a form of "correr" (pt); "走る" is listed as a translation of "correr" (a link of language ja)
	if got := langsOf(others(t, s, "en", "correndo", "pt")); got != "pt" {
		t.Fatalf("by a form: %q", got)
	}
	res := others(t, s, "pt", "走る", "pt")
	if langsOf(res) != "ja" || kinds(res.Results[0].Result) != "translation:correr:verb" {
		t.Fatalf("by a translation: %q %s", langsOf(res), kinds(res.Results[0].Result))
	}
}

func TestDictionaryOthers_LeavesOutALanguageThatOnlyHasTheWordAsAFormOfWhatIsNotThere(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	// Italian lists "zorro" as a form of "zorrare", an entry the dictionary does not have: nothing to show of it
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos, tags) VALUES ('wikt-fr', 'it', 'zorro', 'zorro', 'zorrare', 'verb', '{present}')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-fr', 'fr', 'zorro', 'zorro', 'noun', '{"senses":[{"glosses":["renard"]}]}')`)
	if got := langsOf(others(t, s, "pt", "zorro", "pt")); got != "fr" {
		t.Fatalf("languages = %q, want only fr", got)
	}
}

func TestDictionaryOthers_HasNothingForAWordNoLanguageHas(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	if res := others(t, s, "pt", "xyzzy", "pt"); len(res.Results) != 0 {
		t.Fatalf("%+v", res)
	}
	rec := s.do(ana, "GET", "/dictionary/others?lang=pt&word=xyzzy", "")
	if !strings.Contains(rec.Body.String(), `"results":[]`) {
		t.Fatalf("an empty answer is a list, not null: %s", rec.Body.String())
	}
}

func TestDictionaryOthers_WithNoDictionaryReadyThereIsNothing(t *testing.T) {
	s := newCatalogStack(t)
	if res := others(t, s, "pt", "bonjour", "pt"); len(res.Results) != 0 {
		t.Fatalf("nothing is installed: %+v", res)
	}
	// one that is only being installed is not one
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-fr', 'installing', 'x')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-fr', 'fr', 'bonjour', 'bonjour', '{"senses":[{"glosses":["salut"]}]}')`)
	if res := others(t, s, "pt", "bonjour", "pt"); len(res.Results) != 0 {
		t.Fatalf("while installing: %+v", res)
	}
}

func TestDictionaryOthers_ShowsAtMostSixLanguages(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	for _, lang := range []string{"de", "es", "fr", "it", "nl", "pl", "ru", "tr"} {
		s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', $1, 'taxi', 'taxi', 'noun', '{"senses":[{"glosses":["x"]}]}')`, lang)
	}
	if got := langsOf(others(t, s, "pt", "taxi", "pt")); got != "de,es,fr,it,nl,pl" {
		t.Fatalf("languages = %q, want the first six", got)
	}
}

func TestDictionaryOthers_AnswersLikeTheLookupWouldAndWithoutTheBridge(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	// the same item the lookup of that language gives, with the definitions first in the language asked for
	own := lookup(t, s, "fr", "livre")
	res := others(t, s, "pt", "livre", "pt")
	if langsOf(res) != "fr" || kinds(res.Results[0].Result) != kinds(own) || res.Results[0].Result.Prefer != "pt" {
		t.Fatalf("%+v vs %s", res, kinds(own))
	}
	// asked in its own language with the definitions wanted in Portuguese, the lookup of French tries the bridge ...
	if !strings.Contains(s.do(ana, "GET", "/dictionary?lang=fr&word=livre&prefer=pt", "").Body.String(), `"bridge"`) {
		t.Fatalf("the fixture no longer makes the lookup try the bridge, and this test no longer proves anything")
	}
	// ... and this answer has none: the bridge answers the question asked in the language of the book, not these
	if strings.Contains(s.do(ana, "GET", "/dictionary/others?lang=pt&word=livre&prefer=pt", "").Body.String(), `"bridge"`) {
		t.Fatalf("the bridge answers the question asked in the language of the book, not these")
	}
}

func TestDictionaryOthers_RefusesWhatItCannotLookUp(t *testing.T) {
	s := newCatalogStack(t)
	stockedOthers(t, s)
	long := strings.Repeat("a", 81)
	for name, q := range map[string]string{
		"no word":          "/dictionary/others?lang=pt",
		"no language":      "/dictionary/others?word=bonjour",
		"a bad language":   "/dictionary/others?lang=portugues-brasil&word=bonjour",
		"a bad preference": "/dictionary/others?lang=pt&word=bonjour&prefer=x",
		"a long text":      "/dictionary/others?lang=pt&word=" + long,
	} {
		if rec := s.do(ana, "GET", q, ""); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if rec := s.do(ana, "GET", "/dictionary/others?lang=pt&word="+url.QueryEscape("!!!"), ""); rec.Code != 400 {
		t.Errorf("a word that reads as nothing: %d, want 400", rec.Code)
	}
	// a region of a language is the language
	if got := langsOf(others(t, s, "pt-BR", "bonjour", "pt_BR")); got != "fr" {
		t.Errorf("pt-BR: %q", got)
	}
}
