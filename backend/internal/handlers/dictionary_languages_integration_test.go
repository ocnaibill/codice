package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

type installedLanguages struct {
	Words       []string `json:"words"`
	Definitions []string `json:"definitions"`
}

func askLanguages(t *testing.T, s *catalogStack) installedLanguages {
	t.Helper()
	rec := s.do(ana, "GET", "/dictionary/languages", "")
	if rec.Code != 200 {
		t.Fatalf("languages: %d %s", rec.Code, rec.Body)
	}
	var out installedLanguages
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDictionaryLanguages_NothingInstalledOffersNothing(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "GET", "/dictionary/languages", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"words":[],"definitions":[]}` {
		t.Fatalf("nothing: %d %s", rec.Code, rec.Body)
	}
}

func TestDictionaryLanguages_WordsAreThoseOfTheEntriesAndOfWhatTheTranslationsFind(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, languages, link_languages) VALUES
		('wikt-pt', 'ready', 'x', '{pt,en,es}', '{ja,pt}')`)
	got := askLanguages(t, s)
	if strings.Join(got.Words, ",") != "en,es,ja,pt" {
		t.Fatalf("words: %v", got.Words)
	}
}

func TestDictionaryLanguages_DefinitionsAreThoseOfTheEditionsThatAreInstalled(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, languages) VALUES
		('wikt-pt', 'ready', 'x', '{pt}'), ('wikt-ja', 'ready', 'x', '{ja}'), ('wikt-it', 'ready', 'x', '{it}')`)
	got := askLanguages(t, s)
	if strings.Join(got.Definitions, ",") != "it,ja,pt" || strings.Join(got.Words, ",") != "it,ja,pt" {
		t.Fatalf("%+v", got)
	}
}

func TestDictionaryLanguages_TheSimpleEditionDefinesInEnglish(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, languages) VALUES ('wikt-simple', 'ready', 'x', '{en}')`)
	if got := askLanguages(t, s); strings.Join(got.Definitions, ",") != "en" {
		t.Fatalf("%+v", got)
	}
}

func TestDictionaryLanguages_OnlyWhatIsReadyCounts(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, languages, link_languages) VALUES
		('wikt-pt', 'ready', 'x', '{pt}', '{}'),
		('wikt-ja', 'installing', 'x', '{ja}', '{ko}'),
		('wikt-it', 'failed', 'x', '{it}', '{}')`)
	got := askLanguages(t, s)
	if strings.Join(got.Words, ",") != "pt" || strings.Join(got.Definitions, ",") != "pt" {
		t.Fatalf("%+v", got)
	}
}

func TestDictionaryLanguages_ARemovedPackageTakesItsLanguagesAway(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, languages) VALUES ('wikt-pt', 'ready', 'x', '{pt}'), ('wikt-ja', 'ready', 'x', '{ja}')`)
	s.exec(`DELETE FROM dictionary_packages WHERE id = 'wikt-ja'`)
	if got := askLanguages(t, s); strings.Join(got.Words, ",") != "pt" {
		t.Fatalf("%+v", got)
	}
}

func TestDictionaryLanguages_APackageOutsideTheCatalogHasWordsButNoEdition(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, languages) VALUES ('antigo', 'ready', 'x', '{pt}')`)
	if got := askLanguages(t, s); strings.Join(got.Words, ",") != "pt" || len(got.Definitions) != 0 {
		t.Fatalf("%+v", got)
	}
}
