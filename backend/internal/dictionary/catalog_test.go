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
		if !strings.HasSuffix(p.URL, "/"+p.Edition+"/"+p.Edition+"-extract.jsonl.gz") {
			t.Errorf("%s: the address %s is not the extract of the %s edition", p.ID, p.URL, p.Edition)
		}
	}
}

func TestCatalog_OnlyThePortugueseEditionCanBeInstalledForNow(t *testing.T) {
	var installable []string
	for _, p := range Catalog {
		if p.Installable {
			installable = append(installable, p.ID)
		}
	}
	if len(installable) != 1 || installable[0] != "wikt-pt" {
		t.Fatalf("installable: %v", installable)
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
