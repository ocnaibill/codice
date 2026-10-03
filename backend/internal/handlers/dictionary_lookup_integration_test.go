package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

type lookupItem struct {
	Kind  string   `json:"kind"`
	Form  string   `json:"form"`
	Tags  []string `json:"tags"`
	Via   string   `json:"via"`
	Sense string   `json:"sense"`
	Entry struct {
		ID      int64           `json:"id"`
		Package string          `json:"package"`
		Lang    string          `json:"lang"`
		Word    string          `json:"word"`
		Pos     string          `json:"pos"`
		Data    json.RawMessage `json:"data"`
	} `json:"entry"`
}

type lookupResult struct {
	Word      string       `json:"word"`
	Lang      string       `json:"lang"`
	Installed bool         `json:"installed"`
	Items     []lookupItem `json:"items"`
	Sources   []struct {
		Package, Name, License, LicenseUrl, Source, SourceUrl string
	} `json:"sources"`
}

func lookup(t *testing.T, s *catalogStack, lang, word string) lookupResult {
	t.Helper()
	rec := s.do(ana, "GET", "/dictionary?lang="+url.QueryEscape(lang)+"&word="+url.QueryEscape(word), "")
	if rec.Code != 200 {
		t.Fatalf("lookup %s %s: %d %s", lang, word, rec.Code, rec.Body.String())
	}
	var res lookupResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func kinds(res lookupResult) string {
	var out []string
	for _, it := range res.Items {
		out = append(out, fmt.Sprintf("%s:%s:%s", it.Kind, it.Entry.Word, it.Entry.Pos))
	}
	return strings.Join(out, " ")
}

// A small Portuguese dictionary of what the tests look up, installed as the worker would have.
func stockedDictionary(t *testing.T, s *catalogStack) {
	t.Helper()
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, entries) VALUES ('wikt-pt', 'ready', 'x', 9)`)
	entry := func(lang, word, norm, pos, data string) {
		s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', $1, $2, $3, $4, $5::jsonb)`, lang, word, norm, pos, data)
	}
	entry("pt", "correr", "correr", "verb", `{"senses":[{"glosses":["mover-se com rapidez"]},{"glosses":["apressar-se"]}],"forms":[{"form":"corro","tags":["first-person"]}]}`)
	entry("pt", "correram", "correram", "verb", `{"senses":[{"glosses":["terceira pessoa do plural do pretérito perfeito do indicativo do verbo correr"],"form_of":[{"word":"correr"}]}]}`)
	entry("pt", "livro", "livro", "verb", `{"senses":[{"glosses":["primeira pessoa do singular do presente do indicativo do verbo livrar"],"form_of":[{"word":"livrar"}]}]}`)
	entry("pt", "livro", "livro", "noun", `{"senses":[{"glosses":["objeto feito de várias folhas de papel"]}]}`)
	entry("pt", "livrar", "livrar", "verb", `{"senses":[{"glosses":["tornar livre"]}]}`)
	entry("pt", "ação", "acao", "noun", `{"senses":[{"glosses":["ato"]}]}`)
	entry("en", "book", "book", "noun", `{"senses":[{"glosses":["livro"]}]}`)
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos, tags) VALUES
		('wikt-pt', 'pt', 'corro', 'corro', 'correr', 'verb', '{first-person,singular,present}'),
		('wikt-pt', 'pt', 'correndo', 'correndo', 'correr', 'verb', '{gerund}')`)
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word, pos, sense) VALUES
		('wikt-pt', 'ja', '走る', '走る', 'pt', 'correr', 'verb', 'mover-se com rapidez'),
		('wikt-pt', 'en', 'run', 'run', 'pt', 'correr', 'verb', '')`)
}

func TestDictionaryLookup_WithNoDictionaryInstalledSaysSo(t *testing.T) {
	s := newCatalogStack(t)
	res := lookup(t, s, "pt", "casa")
	if res.Installed || len(res.Items) != 0 || len(res.Sources) != 0 {
		t.Fatalf("result: %+v", res)
	}
	// One that is only being installed is not one.
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-pt', 'installing', 'x')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-pt', 'pt', 'casa', 'casa', '{"senses":[{"glosses":["lar"]}]}')`)
	if res := lookup(t, s, "pt", "casa"); res.Installed || len(res.Items) != 0 {
		t.Fatalf("while installing: %+v", res)
	}
}

func TestDictionaryLookup_FindsAnEntryByTheWordAsItIsWritten(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	res := lookup(t, s, "pt", "correr")
	if !res.Installed || res.Word != "correr" || res.Lang != "pt" {
		t.Fatalf("result: %+v", res)
	}
	if got := kinds(res); got != "entry:correr:verb" {
		t.Fatalf("items: %s", got)
	}
	if !strings.Contains(string(res.Items[0].Entry.Data), "mover-se com rapidez") || res.Items[0].Entry.Package != "wikt-pt" {
		t.Fatalf("entry: %+v", res.Items[0].Entry)
	}
}

func TestDictionaryLookup_ReadsTheWordTheWayTheDictionaryStoresIt(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	for _, word := range []string{"Ação", "ACAO", "  «ação»! ", "acao"} {
		if got := kinds(lookup(t, s, "pt", word)); got != "entry:ação:noun" {
			t.Errorf("%q: %s", word, got)
		}
	}
}

func TestDictionaryLookup_AnInflectedFormShowsWhatItIsAndWhereItComesFrom(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	res := lookup(t, s, "pt", "correram")
	if got := kinds(res); got != "entry:correram:verb lemma:correr:verb" {
		t.Fatalf("items: %s", got)
	}
	if res.Items[1].Form != "correram" {
		t.Fatalf("the lemma does not say which form it was: %+v", res.Items[1])
	}
}

func TestDictionaryLookup_WhatTheWordIsComesBeforeWhatItIsAFormOf(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	// "livro" is a noun and a form of "livrar": the noun first, then the form, then the lemma it is a form of.
	if got := kinds(lookup(t, s, "pt", "livro")); got != "entry:livro:noun entry:livro:verb lemma:livrar:verb" {
		t.Fatalf("items: %s", got)
	}
}

func TestDictionaryLookup_AFormThatHasNoEntryOfItsOwnFindsItsLemmaByTheFormsItLists(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	res := lookup(t, s, "pt", "corro")
	if got := kinds(res); got != "lemma:correr:verb" {
		t.Fatalf("items: %s", got)
	}
	if res.Items[0].Form != "corro" || strings.Join(res.Items[0].Tags, ",") != "first-person,singular,present" {
		t.Fatalf("form: %+v", res.Items[0])
	}
	if got := kinds(lookup(t, s, "pt", "Correndo")); got != "lemma:correr:verb" {
		t.Fatalf("by another form: %s", got)
	}
}

func TestDictionaryLookup_AWordOfAnotherLanguageIsFoundByTheTranslationsAnEntryLists(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	res := lookup(t, s, "ja", "走る")
	if got := kinds(res); got != "translation:correr:verb" {
		t.Fatalf("items: %s", got)
	}
	if it := res.Items[0]; it.Via != "走る" || it.Sense != "mover-se com rapidez" || it.Entry.Lang != "pt" {
		t.Fatalf("translation: %+v", it)
	}
	// An English word that has its own entry, and is also listed as a translation: the entry first.
	if got := kinds(lookup(t, s, "en", "book")); got != "entry:book:noun" {
		t.Fatalf("book: %s", got)
	}
	if got := kinds(lookup(t, s, "en", "run")); got != "translation:correr:verb" {
		t.Fatalf("run: %s", got)
	}
}

func TestDictionaryLookup_ALemmaIsNotShownTwice(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	// "correr" is found by its forms and by a translation under the same word: it shows once.
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos, tags) VALUES ('wikt-pt', 'pt', 'correram', 'correram', 'correr', 'verb', '{}')`)
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES ('wikt-pt', 'pt', 'correram', 'correram', 'pt', 'correr')`)
	res := lookup(t, s, "pt", "correram")
	count := 0
	for _, it := range res.Items {
		if it.Entry.Word == "correr" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("correr shown %d times: %s", count, kinds(res))
	}
}

func TestDictionaryLookup_ALemmaThatIsItselfAFormIsNotWhatTheWordComesFrom(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos, tags) VALUES ('wikt-pt', 'pt', 'xyz', 'xyz', 'livro', 'noun', '{}')`)
	// "livro" has an entry that is a noun and one that is a form of livrar: only the noun is a lemma.
	if got := kinds(lookup(t, s, "pt", "xyz")); got != "lemma:livro:noun" {
		t.Fatalf("items: %s", got)
	}
}

func TestDictionaryLookup_OnlyWhatIsReadyIsLookedUp(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-xx', 'installing', 'x')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-xx', 'pt', 'correr', 'correr', '{"senses":[{"glosses":["de outro"]}]}')`)
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma) VALUES ('wikt-xx', 'pt', 'corro', 'corro', 'outro')`)
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES ('wikt-xx', 'ja', '走る', '走る', 'pt', 'correr')`)
	if got := kinds(lookup(t, s, "pt", "correr")); got != "entry:correr:verb" {
		t.Fatalf("entries: %s", got)
	}
	if got := kinds(lookup(t, s, "pt", "corro")); got != "lemma:correr:verb" {
		t.Fatalf("forms: %s", got)
	}
	if got := kinds(lookup(t, s, "ja", "走る")); got != "translation:correr:verb" {
		t.Fatalf("links: %s", got)
	}
	var n int
	for _, it := range lookup(t, s, "pt", "correr").Items {
		if strings.Contains(string(it.Entry.Data), "de outro") {
			n++
		}
	}
	if n != 0 {
		t.Fatal("an entry of a package that is not ready was shown")
	}
}

func TestDictionaryLookup_SaysWhereWhatItFoundComesFrom(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	res := lookup(t, s, "pt", "correram")
	if len(res.Sources) != 1 {
		t.Fatalf("sources: %+v", res.Sources)
	}
	src := res.Sources[0]
	if src.Package != "wikt-pt" || src.Name != "Wikcionário em português" || !strings.Contains(src.License, "CC BY-SA") || !strings.Contains(src.LicenseUrl, "creativecommons.org") || !strings.Contains(src.SourceUrl, "kaikki.org") || src.Source == "" {
		t.Fatalf("source: %+v", src)
	}
	if res := lookup(t, s, "pt", "naoexiste"); len(res.Sources) != 0 || len(res.Items) != 0 || !res.Installed {
		t.Fatalf("nothing found: %+v", res)
	}
}

func TestDictionaryLookup_TakesTheLanguageAsABaseCode(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	for _, lang := range []string{"pt-BR", "pt_BR", "PT", "pt-br"} {
		if got := kinds(lookup(t, s, lang, "correr")); got != "entry:correr:verb" {
			t.Errorf("%s: %s", lang, got)
		}
	}
	if res := lookup(t, s, "pt-BR", "correr"); res.Lang != "pt" {
		t.Fatalf("the language answered: %s", res.Lang)
	}
}

func TestDictionaryLookup_BringsNoMoreThanACardCanHold(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	for i := 0; i < 20; i++ {
		s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', 'pt', 'manga', 'manga', $1, '{"senses":[{"glosses":["x"]}]}')`, fmt.Sprintf("pos%d", i))
	}
	if n := len(lookup(t, s, "pt", "manga").Items); n != 12 {
		t.Fatalf("entries: %d", n)
	}
	for i := 0; i < 15; i++ {
		s.exec(fmt.Sprintf(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', 'pt', 'alvo%d', 'alvo%d', 'noun', '{"senses":[{"glosses":["x"]}]}')`, i, i))
		s.exec(fmt.Sprintf(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES ('wikt-pt', 'ja', 'muito', 'muito', 'pt', 'alvo%d')`, i))
	}
	if n := len(lookup(t, s, "ja", "muito").Items); n != 10 {
		t.Fatalf("translations: %d", n)
	}
}

func TestDictionaryLookup_RefusesWhatIsNotALookup(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	long := strings.Repeat("a", 81)
	for _, q := range []string{
		"", "word=casa", "lang=pt", "lang=pt&word=", "lang=pt&word=%20%20", "lang=pt&word=...", "lang=pt&word=%C2%AB%C2%BB",
		"lang=&word=casa", "lang=p&word=casa", "lang=portuguese&word=casa", "lang=..%2Fx&word=casa", "lang=pt%27&word=casa", "lang=1t&word=casa",
		"lang=pt&word=" + long,
	} {
		if rec := s.do(ana, "GET", "/dictionary?"+q, ""); rec.Code != 400 {
			t.Errorf("%q: got %d %s", q, rec.Code, rec.Body.String())
		}
	}
	if rec := s.do(ana, "GET", "/dictionary?lang=pt&word="+strings.Repeat("a", 80), ""); rec.Code != 200 {
		t.Errorf("eighty letters: %d", rec.Code)
	}
}

func TestDictionaryLookup_TheWordIsAnArgumentNotSQL(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	for _, word := range []string{"'; DROP TABLE dictionary_entries; --", "casa' OR '1'='1", `a%b_c\`} {
		if rec := s.do(ana, "GET", "/dictionary?lang=pt&word="+url.QueryEscape(word), ""); rec.Code != 200 {
			t.Errorf("%q: %d", word, rec.Code)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM dictionary_entries`); got != "7" {
		t.Fatalf("the table: %s", got)
	}
}

func TestDictionaryLookup_BringsNoMoreLemmasThanACardCanHold(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	for i := 0; i < 10; i++ {
		s.exec(fmt.Sprintf(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', 'pt', 'lema%02d', 'lema%02d', 'noun', '{"senses":[{"glosses":["x"]}]}')`, i, i))
		s.exec(fmt.Sprintf(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos) VALUES ('wikt-pt', 'pt', 'muitas', 'muitas', 'lema%02d', 'noun')`, i))
	}
	if n := len(lookup(t, s, "pt", "muitas").Items); n != 8 {
		t.Fatalf("lemmas from forms: %d", n)
	}
	// An entry that is a form of ten words.
	var words []string
	for i := 0; i < 10; i++ {
		words = append(words, fmt.Sprintf(`{"word":"lema%02d"}`, i))
	}
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', 'pt', 'forma', 'forma', 'verb', $1::jsonb)`,
		`{"senses":[{"glosses":["forma de muitos"],"form_of":[`+strings.Join(words, ",")+`]}]}`)
	res := lookup(t, s, "pt", "forma")
	lemmas := 0
	for _, it := range res.Items {
		if it.Kind == "lemma" {
			lemmas++
		}
	}
	if lemmas != 8 {
		t.Fatalf("lemmas from the senses: %d (%s)", lemmas, kinds(res))
	}
}

func TestDictionaryLookup_AWordThatTwoLemmasListAsAFormFindsBothInOrder(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos, tags) VALUES
		('wikt-pt', 'pt', 'xyz', 'xyz', 'livro', 'noun', '{plural}'),
		('wikt-pt', 'pt', 'xyz', 'xyz', 'correr', 'verb', '{plural}'),
		('wikt-pt', 'pt', 'xyz', 'xyz', 'correr', 'verb', '{plural}')`)
	res := lookup(t, s, "pt", "xyz")
	if got := kinds(res); got != "lemma:correr:verb lemma:livro:noun" {
		t.Fatalf("items: %s", got)
	}
	if len(res.Items[0].Tags) != 1 || res.Items[0].Tags[0] != "plural" {
		t.Fatalf("tags, listed twice: %v", res.Items[0].Tags)
	}
}

func TestDictionaryLookup_AnEntryThatIsAlreadyThereIsNotAddedAsATranslation(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	// "correr" is an entry of its own and is also listed as a translation of itself, and of a form of another word.
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES
		('wikt-pt', 'pt', 'correr', 'correr', 'pt', 'correr'),
		('wikt-pt', 'pt', 'correr', 'correr', 'pt', 'correram')`)
	if got := kinds(lookup(t, s, "pt", "correr")); got != "entry:correr:verb" {
		t.Fatalf("items: %s", got)
	}
}

func TestDictionaryLookup_OnlyTheEntryOfTheWordItselfIsALemma(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	// Another word that reads the same ("córrer" and "correr"): the lemma is the word the dictionary says, not what it reads as.
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', 'pt', 'córrer', 'correr', 'verb', '{"senses":[{"glosses":["outra"]}]}')`)
	if got := kinds(lookup(t, s, "pt", "corro")); got != "lemma:correr:verb" {
		t.Fatalf("by forms: %s", got)
	}
	if got := kinds(lookup(t, s, "ja", "走る")); got != "translation:correr:verb" {
		t.Fatalf("by translation: %s", got)
	}
}

func TestDictionaryLookup_WhatIsNotReadyIsNotLookedUpByItsFormsOrLinksEither(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-xx', 'installing', 'x')`)
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma) VALUES ('wikt-xx', 'pt', 'zzz', 'zzz', 'correr')`)
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES ('wikt-xx', 'ja', 'xyz', 'xyz', 'pt', 'correr')`)
	if res := lookup(t, s, "pt", "zzz"); len(res.Items) != 0 {
		t.Fatalf("a form of a package that is not ready: %s", kinds(res))
	}
	if res := lookup(t, s, "ja", "xyz"); len(res.Items) != 0 {
		t.Fatalf("a link of a package that is not ready: %s", kinds(res))
	}
}

func TestDictionaryLookup_SaysTheSourceOfTheDictionaryTheEntryCameFrom(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-fr', 'ready', 'x')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-fr', 'fr', 'maison', 'maison', 'noun', '{"senses":[{"glosses":["bâtiment"]}]}')`)
	res := lookup(t, s, "fr", "maison")
	if len(res.Sources) != 1 || res.Sources[0].Package != "wikt-fr" || res.Sources[0].Name != "Wikcionário em francês" {
		t.Fatalf("sources: %+v", res.Sources)
	}
	// An entry of a package the catalog does not know has nothing to say of its source.
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('desconhecido', 'ready', 'x')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('desconhecido', 'fr', 'chat', 'chat', 'noun', '{"senses":[{"glosses":["gato"]}]}')`)
	if res := lookup(t, s, "fr", "chat"); len(res.Items) != 1 || len(res.Sources) != 0 {
		t.Fatalf("unknown package: %+v", res)
	}
}

func TestDictionaryLookup_AnswersWithTheWordWithoutTheSpacesAroundIt(t *testing.T) {
	s := newCatalogStack(t)
	stockedDictionary(t, s)
	if res := lookup(t, s, "pt", "  correr 	"); res.Word != "correr" {
		t.Fatalf("word: %q", res.Word)
	}
	rec := s.do(ana, "GET", "/dictionary?lang=pt&word=", "")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "Informe a palavra") {
		t.Fatalf("no word: %d %s", rec.Code, rec.Body.String())
	}
	rec = s.do(ana, "GET", "/dictionary?lang=pt&word=...", "")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "Nenhuma palavra para procurar") {
		t.Fatalf("no letters: %d %s", rec.Code, rec.Body.String())
	}
}
