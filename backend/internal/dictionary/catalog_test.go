package dictionary

import (
	"strings"
	"testing"
)

func TestCatalog_EveryPackageIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Catalog {
		if p.ID == "" || !strings.HasPrefix(p.ID, "wikt-") || seen[p.ID] {
			t.Errorf("package id %q is empty, not a Wiktionary one, or repeated", p.ID)
		}
		seen[p.ID] = true
		if p.Name == "" || p.Description == "" || p.Edition == "" || p.License == "" || p.LicenseURL == "" || p.Source == "" || p.SourceURL == "" {
			t.Errorf("%s: a field that the owner must be shown is empty: %+v", p.ID, p)
		}
		if p.DownloadBytes <= 0 {
			t.Errorf("%s: no size to show", p.ID)
		}
		if len(p.Languages) == 0 {
			t.Errorf("%s: says no language", p.ID)
		}
		for _, l := range p.Languages {
			if l.Code == "" || (l.Level != Complete && l.Level != Partial && l.Level != Weak) {
				t.Errorf("%s: language %+v", p.ID, l)
			}
		}
		if err := p.Validate(); err != nil {
			t.Errorf("%v", err)
		}
		if p.ID != "wikt-en" && !strings.HasSuffix(p.URL, "/"+p.Edition+"/"+p.Edition+"-extract.jsonl.gz") { // English is the one by language of the words
			t.Errorf("%s: the address %s is not the extract of the %s edition", p.ID, p.URL, p.Edition)
		}
	}
}

func TestCatalog_OnlyWhatWasMeasuredSaysWhatItTakesInTheDatabase(t *testing.T) {
	for _, p := range Catalog {
		if p.StorageBytes < 0 {
			t.Errorf("%s: %d", p.ID, p.StorageBytes)
		}
		if p.StorageBytes > 0 && p.StorageBytes < p.DownloadBytes {
			t.Errorf("%s: it takes less in the database (%d) than it weighs compressed (%d)", p.ID, p.StorageBytes, p.DownloadBytes)
		}
	}
	if pt, _ := Find("wikt-pt"); pt.StorageBytes != 326_000_000 {
		t.Fatalf("the Portuguese edition was measured at 326 MB: %d", pt.StorageBytes)
	}
	if fr, _ := Find("wikt-fr"); fr.StorageBytes != 0 {
		t.Fatalf("the French one was not measured: %d", fr.StorageBytes)
	}
}

func TestCatalog_EveryEditionTheSourcePublishesCanBeInstalled(t *testing.T) {
	if len(Catalog) != 21 {
		t.Fatalf("%d packages", len(Catalog))
	}
	for _, p := range Catalog {
		if !p.Installable {
			t.Errorf("%s cannot be installed", p.ID)
		}
		if p.ID != "wikt-"+p.Edition {
			t.Errorf("%s is the edition %s", p.ID, p.Edition)
		}
	}
}

func TestCatalog_PortugueseFirstThenTheOthersByTheNameOfTheLanguage(t *testing.T) {
	var names []string
	for _, p := range Catalog {
		names = append(names, strings.TrimPrefix(p.Name, "Wikcionário em "))
	}
	want := []string{"português", "alemão", "chinês", "coreano", "curdo", "espanhol", "francês", "grego", "holandês", "indonésio",
		"inglês", "inglês simples", "italiano", "japonês", "malaio", "polonês", "russo", "tailandês", "tcheco", "turco", "vietnamita"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names: %v", names)
	}
}

func TestCatalog_EachPackageKeepsTheWordsOfItsOwnLanguage(t *testing.T) {
	for _, p := range Catalog {
		if len(p.Headwords) == 0 {
			t.Errorf("%s keeps no words", p.ID)
		}
		listed := map[string]bool{}
		for _, h := range p.Headwords {
			listed[h] = true
		}
		for _, l := range p.Languages {
			if !listed[l.Code] {
				t.Errorf("%s says it covers %s and does not keep its words", p.ID, l.Code)
			}
		}
	}
	for _, code := range []string{"de", "ja", "zh", "it", "fr", "ru", "ko"} {
		p, _ := Find("wikt-" + code)
		if len(p.Headwords) != 1 || p.Headwords[0] != code {
			t.Errorf("%s keeps %v", code, p.Headwords)
		}
	}
	if pt, _ := Find("wikt-pt"); strings.Join(pt.Headwords, ",") != "pt,en,es,fr,de,it,ja,zh" {
		t.Errorf("the Portuguese edition keeps the languages of the library: %v", pt.Headwords)
	}
	// The simple edition is the Wiktionary in a simpler English: it defines English words.
	if simple, _ := Find("wikt-simple"); len(simple.Headwords) != 1 || simple.Headwords[0] != "en" || simple.Languages[0].Code != "en" {
		t.Errorf("simple: %+v", simple)
	}
}

func TestCatalog_SaysWhenAPackageIsBig(t *testing.T) {
	for _, p := range Catalog {
		big := p.DownloadBytes >= 200*1024*1024
		says := strings.Contains(p.Description, "É grande")
		if big != says {
			t.Errorf("%s: %d bytes, says it is big: %v", p.ID, p.DownloadBytes, says)
		}
	}
	for _, id := range []string{"wikt-fr", "wikt-de", "wikt-zh", "wikt-ru", "wikt-en"} {
		if p, _ := Find(id); !strings.Contains(p.Description, "É grande") {
			t.Errorf("%s: %s", id, p.Description)
		}
	}
	if p, _ := Find("wikt-ja"); strings.Contains(p.Description, "É grande") {
		t.Error("ja is not big")
	}
}

func TestCatalog_TheSizesAreThoseTheSourceSaid(t *testing.T) {
	for id, want := range map[string]int64{"wikt-en": 523_338_320, "wikt-pt": 37_158_613, "wikt-ja": 64_066_672, "wikt-it": 43_612_281, "wikt-fr": 733_854_991, "wikt-simple": 4_719_269} {
		if p, _ := Find(id); p.DownloadBytes != want {
			t.Errorf("%s: %d", id, p.DownloadBytes)
		}
	}
}

func TestCatalog_ThePortugueseEditionIsCompleteForPortugueseAndNothingElse(t *testing.T) {
	p, _ := Find("wikt-pt")
	levels := map[string]Level{}
	for _, l := range p.Languages {
		levels[l.Code] = l.Level
	}
	if levels["pt"] != Complete || levels["en"] != Partial {
		t.Fatalf("levels: %v", levels)
	}
	for _, code := range []string{"es", "fr", "de", "it", "ja", "zh"} {
		if levels[code] != Weak {
			t.Errorf("%s is %q, measured weak", code, levels[code])
		}
	}
}

func TestFind(t *testing.T) {
	if p, ok := Find("wikt-de"); !ok || p.Edition != "de" {
		t.Fatalf("find: %+v %v", p, ok)
	}
	if _, ok := Find("wikt-xx"); ok {
		t.Fatal("an unknown package was found")
	}
	if _, ok := Find(""); ok {
		t.Fatal("the empty id was found")
	}
}

func TestValidate_OnlyAnHTTPSAddressOnTheHost(t *testing.T) {
	good := Package{ID: "x", URL: "https://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"http://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz",
		"https://evil.example/dictionary/downloads/pt/pt-extract.jsonl.gz",
		"https://kaikki.org.evil.example/x.gz",
		"https://user@kaikki.org/x.gz",
		"https://kaikki.org:8443/x.gz",
		"https://127.0.0.1/x.gz",
		"file:///etc/passwd",
		"",
		"://nope",
	} {
		if err := (Package{ID: "x", URL: bad}).Validate(); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestCatalog_TheEnglishPackageIsTheDictionaryOfEnglishAndTheBridge(t *testing.T) {
	p, ok := Find("wikt-en")
	if !ok || !p.Installable {
		t.Fatalf("wikt-en: %+v", p)
	}
	if p.URL != "https://kaikki.org/dictionary/English/kaikki.org-dictionary-English.jsonl.gz" || p.Validate() != nil {
		t.Errorf("address: %s (%v)", p.URL, p.Validate())
	}
	if p.Edition != "en" || strings.Join(p.Headwords, ",") != "en" || len(p.Languages) != 1 || p.Languages[0].Code != "en" {
		t.Errorf("it is the English words, defined in English: %+v", p)
	}
	// it keeps the translations into every language the catalog has words of: that is what the bridge walks
	if got := strings.Join(p.Translations, ","); got != "cs,de,el,en,es,fr,id,it,ja,ko,ku,ms,nl,pl,pt,ru,th,tr,vi,zh" {
		t.Errorf("translations: %s", got)
	}
	if !strings.Contains(p.Description, "ponte") {
		t.Errorf("it says it is the bridge: %s", p.Description)
	}
	for _, other := range Catalog {
		if other.ID != "wikt-en" && len(other.Translations) != 0 {
			t.Errorf("%s keeps the translations the library keeps, not %v", other.ID, other.Translations)
		}
	}
}

func TestCatalog_EveryAddressIsOneThatMayBeDownloadedFrom(t *testing.T) {
	for _, p := range Catalog {
		if err := p.Validate(); err != nil {
			t.Errorf("%s: %v", p.ID, err)
		}
	}
}
