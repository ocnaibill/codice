package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

type bridgeResult struct {
	Items   []lookupItem `json:"items"`
	Sources []struct {
		Package string `json:"package"`
	} `json:"sources"`
	Bridge *struct {
		Via        string `json:"via"`
		From       string `json:"from"`
		To         string `json:"to"`
		Available  bool   `json:"available"`
		Candidates []struct {
			English string   `json:"english"`
			Words   []string `json:"words"`
		} `json:"candidates"`
	} `json:"bridge"`
}

func lookupBridge(t *testing.T, s *catalogStack, lang, prefer, word string) bridgeResult {
	t.Helper()
	rec := s.do(ana, "GET", "/dictionary?lang="+lang+"&prefer="+url.QueryEscape(prefer)+"&word="+url.QueryEscape(word), "")
	if rec.Code != 200 {
		t.Fatalf("lookup: %d %s", rec.Code, rec.Body.String())
	}
	var res bridgeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func candidatesOf(res bridgeResult) string {
	if res.Bridge == nil {
		return "no bridge"
	}
	var out []string
	for _, c := range res.Bridge.Candidates {
		out = append(out, c.English+"="+strings.Join(c.Words, "/"))
	}
	return strings.Join(out, " ")
}

// bridgeWorld is a Japanese dictionary with 走る and no word of Portuguese, an English one whose "run" lists 走る and the
// Portuguese words, and a Portuguese one whose entries list "run".
func bridgeWorld(t *testing.T) *catalogStack {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-ja', 'ready', 'x'), ('wikt-en', 'ready', 'x'), ('wikt-pt', 'ready', 'x')`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES
		('wikt-ja', 'ja', '走る', '走る', 'verb', '{"senses":[{"glosses":["はしる"]}]}'),
		('wikt-en', 'en', 'run', 'run', 'verb', '{"senses":[{"glosses":["to move fast"]}]}'),
		('wikt-pt', 'pt', 'correr', 'correr', 'verb', '{"senses":[{"glosses":["mover-se"]}]}')`)
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES
		('wikt-en', 'ja', '走る', '走る', 'en', 'run'),
		('wikt-en', 'ja', '走る', '走る', 'en', 'race'),
		('wikt-en', 'pt', 'correr', 'correr', 'en', 'run'),
		('wikt-en', 'pt', 'andar', 'andar', 'en', 'run'),
		('wikt-en', 'pt', 'competir', 'competir', 'en', 'race'),
		('wikt-ja', 'en', 'run', 'run', 'ja', '走る'),
		('wikt-pt', 'en', 'run', 'run', 'pt', 'correr'),
		('wikt-pt', 'en', 'run', 'run', 'pt', 'dirigir')`)
	return s
}

func TestDictionaryBridge_WhenTheTwoLanguagesAreNotLinkedItGoesThroughEnglishAndSaysSo(t *testing.T) {
	s := bridgeWorld(t)
	res := lookupBridge(t, s, "ja", "pt", "走る")
	if res.Bridge == nil || res.Bridge.Via != "en" || res.Bridge.From != "ja" || res.Bridge.To != "pt" || !res.Bridge.Available {
		t.Fatalf("bridge: %+v", res.Bridge)
	}
	// "run" is said by the English dictionary and by the Japanese entry: first. The words said twice come first within it.
	if got := candidatesOf(res); got != "run=correr/andar/dirigir race=competir" {
		t.Fatalf("candidates: %s", got)
	}
	// the Japanese entry is there all the same: the bridge adds to what the dictionaries have, it does not replace it
	if len(res.Items) == 0 || res.Items[0].Entry.Package != "wikt-ja" {
		t.Fatalf("items: %+v", res.Items)
	}
}

func TestDictionaryBridge_CreditsTheDictionariesItWentThrough(t *testing.T) {
	s := bridgeWorld(t)
	var got []string
	for _, src := range lookupBridge(t, s, "ja", "pt", "走る").Sources {
		got = append(got, src.Package)
	}
	if strings.Join(got, ",") != "wikt-ja,wikt-en,wikt-pt" {
		t.Fatalf("sources: %v", got)
	}
}

func TestDictionaryBridge_IsNotTriedWhenTheLanguagesAreLinkedDirectly(t *testing.T) {
	s := bridgeWorld(t)
	// the Portuguese dictionary lists 走る under correr: that is a direct link from Japanese to Portuguese
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES ('wikt-pt', 'ja', '走る', '走る', 'pt', 'correr')`)
	res := lookupBridge(t, s, "ja", "pt", "走る")
	if res.Bridge != nil {
		t.Fatalf("a direct link was there: %s", candidatesOf(res))
	}
}

func TestDictionaryBridge_AnEntryThatListsATranslationIntoTheWantedLanguageIsADirectLink(t *testing.T) {
	s := bridgeWorld(t)
	s.exec(`UPDATE dictionary_entries SET data = '{"senses":[{"glosses":["はしる"]}],"translations":[{"lang":"pt","word":"correr"}]}' WHERE word = '走る'`)
	if res := lookupBridge(t, s, "ja", "pt", "走る"); res.Bridge != nil {
		t.Fatalf("the entry lists Portuguese: %s", candidatesOf(res))
	}
	// one that lists only another language is not
	s.exec(`UPDATE dictionary_entries SET data = '{"senses":[{"glosses":["はしる"]}],"translations":[{"lang":"fr","word":"courir"}]}' WHERE word = '走る'`)
	if res := lookupBridge(t, s, "ja", "pt", "走る"); res.Bridge == nil {
		t.Fatal("a translation into French is not one into Portuguese")
	}
}

func TestDictionaryBridge_AnEnglishEntryThatListsPortugueseIsStillTheBridgeAndNotADirectLink(t *testing.T) {
	s := bridgeWorld(t)
	// the English entry for "run" lists the word, and its own data lists Portuguese translations (the real file does)
	s.exec(`UPDATE dictionary_entries SET data = '{"senses":[{"glosses":["to move fast"]}],"translations":[{"lang":"pt","word":"correr"}]}' WHERE package_id = 'wikt-en'`)
	res := lookupBridge(t, s, "ja", "pt", "走る")
	if res.Bridge == nil || candidatesOf(res) != "run=correr/andar/dirigir race=competir" {
		t.Fatalf("the word is listed under English, and that is said: %s", candidatesOf(res))
	}
	var english bool
	for _, it := range res.Items {
		english = english || it.Entry.Package == "wikt-en"
	}
	if !english {
		t.Fatal("the English entry is shown too, as what the word is listed under")
	}
}

func TestDictionaryBridge_AWordOfItsOwnLanguageDefinedInTheWantedOneIsADirectLink(t *testing.T) {
	s := bridgeWorld(t)
	// the Portuguese Wiktionary has an entry of the Japanese word, with the definition in Portuguese
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES ('wikt-pt', 'ja', '走る', '走る', 'verb', '{"senses":[{"glosses":["correr"]}]}')`)
	if res := lookupBridge(t, s, "ja", "pt", "走る"); res.Bridge != nil {
		t.Fatalf("defined in Portuguese: %s", candidatesOf(res))
	}
}

func TestDictionaryBridge_IsNotTriedWhenThereIsNothingToBridge(t *testing.T) {
	s := bridgeWorld(t)
	for _, c := range []struct{ lang, prefer, why string }{
		{"ja", "", "no language wanted"},
		{"ja", "ja", "the language of the word is the one wanted"},
		{"en", "pt", "English is not bridged to"},
		{"ja", "en", "English is the one wanted"},
	} {
		if res := lookupBridge(t, s, c.lang, c.prefer, "run"); res.Bridge != nil {
			t.Errorf("%s: %s", c.why, candidatesOf(res))
		}
	}
}

func TestDictionaryBridge_EnglishIsNotBridgedToEvenWhenThePortugueseDictionaryHasNothingOfTheWord(t *testing.T) {
	s := bridgeWorld(t)
	s.exec(`DELETE FROM dictionary_packages WHERE id = 'wikt-pt'`)
	if res := lookupBridge(t, s, "en", "pt", "run"); res.Bridge != nil {
		t.Fatalf("a word of English is not bridged through English: %s", candidatesOf(res))
	}
}

func TestDictionaryBridge_OnlyWhatIsListedAsEnglishIsAnEnglishWord(t *testing.T) {
	s := bridgeWorld(t)
	// Six words of other languages that list 走る, which come before "race" and "run" if they were English words, and
	// would take the places of the six English words the bridge follows.
	for i := 0; i < 6; i++ {
		s.exec(fmt.Sprintf(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES
			('wikt-ja', 'ja', '走る', '走る', 'fr', 'a%d'), ('wikt-ja', 'fr', 'b%d', 'b%d', 'ja', '走る')`, i, i, i))
	}
	if got := candidatesOf(lookupBridge(t, s, "ja", "pt", "走る")); got != "run=correr/andar/dirigir race=competir" {
		t.Fatalf("candidates: %s", got)
	}
}

func TestDictionaryBridge_AnEnglishWordIsFoundByWhatItReadsAsWhateverItsCaseWhereItIsListed(t *testing.T) {
	s := bridgeWorld(t)
	// the English dictionary lists 走る under "Run" with a capital, which the Portuguese one lists as "run"
	s.exec(`UPDATE dictionary_links SET target_word = 'Run' WHERE package_id = 'wikt-en' AND target_word = 'run' AND lang = 'ja'`)
	s.exec(`UPDATE dictionary_links SET target_word = 'Run' WHERE package_id = 'wikt-ja'`)
	if got := candidatesOf(lookupBridge(t, s, "ja", "pt", "走る")); !strings.HasPrefix(got, "Run=correr/dirigir") {
		t.Fatalf("candidates: %s", got)
	}
}

func TestDictionaryBridge_SaysWhenTheEnglishDictionaryIsNotInstalledAndStillUsesWhatTheOthersList(t *testing.T) {
	s := bridgeWorld(t)
	s.exec(`DELETE FROM dictionary_packages WHERE id = 'wikt-en'`)
	res := lookupBridge(t, s, "ja", "pt", "走る")
	if res.Bridge == nil || res.Bridge.Available {
		t.Fatalf("bridge: %+v", res.Bridge)
	}
	// the Japanese entry lists "run", and the Portuguese one lists run under correr and dirigir
	if got := candidatesOf(res); got != "run=correr/dirigir" {
		t.Fatalf("candidates: %s", got)
	}
}

func TestDictionaryBridge_SaysItFoundNothingWhenNoEnglishWordLeadsThere(t *testing.T) {
	s := bridgeWorld(t)
	res := lookupBridge(t, s, "ja", "pt", "食べる")
	if res.Bridge == nil || !res.Bridge.Available || len(res.Bridge.Candidates) != 0 {
		t.Fatalf("bridge: %+v", res.Bridge)
	}
	if res.Bridge.Candidates == nil {
		t.Fatal("candidates are a list, even an empty one")
	}
}

func TestDictionaryBridge_AnEnglishWordThatLeadsToNoWordOfTheWantedLanguageIsNotACandidate(t *testing.T) {
	s := bridgeWorld(t)
	s.exec(`DELETE FROM dictionary_links WHERE word = 'competir'`)
	if got := candidatesOf(lookupBridge(t, s, "ja", "pt", "走る")); got != "run=correr/andar/dirigir" {
		t.Fatalf("candidates: %s", got)
	}
}

func TestDictionaryBridge_AFormGoesThroughItsLemma(t *testing.T) {
	s := bridgeWorld(t)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES
		('wikt-ja', 'ja', '走った', '走った', 'verb', '{"senses":[{"glosses":[],"form_of":[{"word":"走る"}]}]}')`)
	if got := candidatesOf(lookupBridge(t, s, "ja", "pt", "走った")); got != "run=correr/andar/dirigir race=competir" {
		t.Fatalf("the form is of 走る, which leads to English: %s", got)
	}
}

func TestDictionaryBridge_UsesOnlyPackagesThatAreReady(t *testing.T) {
	s := bridgeWorld(t)
	s.exec(`UPDATE dictionary_packages SET state = 'installing' WHERE id = 'wikt-en'`)
	res := lookupBridge(t, s, "ja", "pt", "走る")
	if res.Bridge == nil || res.Bridge.Available {
		t.Fatalf("an English dictionary that is still being installed is not there: %+v", res.Bridge)
	}
	if got := candidatesOf(res); got != "run=correr/dirigir" {
		t.Fatalf("candidates: %s", got)
	}
}

func TestDictionaryBridge_SaysAtMostSixEnglishWordsAndEightOfTheWantedLanguageForEach(t *testing.T) {
	s := bridgeWorld(t)
	for i := 0; i < 10; i++ {
		s.exec(fmt.Sprintf(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES
			('wikt-en', 'ja', '走る', '走る', 'en', 'e%02d'), ('wikt-en', 'pt', 'p%02d', 'p%02d', 'en', 'run'), ('wikt-en', 'pt', 'q', 'q', 'en', 'e%02d')`, i, i, i, i))
	}
	res := lookupBridge(t, s, "ja", "pt", "走る")
	if len(res.Bridge.Candidates) > 6 {
		t.Fatalf("%d English words", len(res.Bridge.Candidates))
	}
	for _, c := range res.Bridge.Candidates {
		if len(c.Words) > 8 {
			t.Fatalf("%s: %d words", c.English, len(c.Words))
		}
	}
	if res.Bridge.Candidates[0].English != "run" || len(res.Bridge.Candidates[0].Words) != 8 {
		t.Fatalf("run: %+v", res.Bridge.Candidates[0])
	}
}

func TestDictionaryBridge_AnEnglishWordIsFoundWhateverItsCase(t *testing.T) {
	s := bridgeWorld(t)
	// the Portuguese Wiktionary lists "Run" with a capital; it reads the same as "run"
	s.exec(`UPDATE dictionary_links SET word = 'Run', norm = 'run' WHERE package_id = 'wikt-pt'`)
	if got := candidatesOf(lookupBridge(t, s, "ja", "pt", "走る")); !strings.HasPrefix(got, "run=correr/andar/dirigir") {
		t.Fatalf("candidates: %s", got)
	}
}
