// Package dictionary is what the server knows of the dictionaries the owner may install (#109, DEC-115): a fixed catalog
// of packages, each from a known address. A dictionary is third-party data (the Wiktionary, extracted by Wiktextract and
// published at kaikki.org); the project does not ship it nor ask anywhere else for it. The worker downloads a package only
// when the owner asks, and only from an address in this catalog, never from one a request supplies.
package dictionary

import (
	"fmt"
	"net/url"
)

// Level says how well a package covers the words of a language (measured on the data, see
// docs/Codice_Levantamento_Dicionario_2026-10-03.md).
type Level string

const (
	// Complete: the words and their inflected forms are there, with definitions in the language of the package.
	Complete Level = "complete"
	// Partial: the common words are there, with their translation; inflected forms are patchy.
	Partial Level = "partial"
	// Weak: a few words, and words found through the translations of another language's entries.
	Weak Level = "weak"
)

// Language is a language a package has words of, and how well.
type Language struct {
	Code  string `json:"code"`
	Level Level  `json:"level"`
}

// Package is a dictionary the owner can install.
type Package struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Edition is the Wiktionary the package is extracted from, by its language code: its definitions are in that language.
	Edition string `json:"edition"`
	URL     string `json:"-"`
	// DownloadBytes is what is downloaded (compressed), as the server said when this was written: an approximation, the
	// real size is read when the download starts.
	DownloadBytes int64 `json:"downloadBytes"`
	// StorageBytes is about what the package takes in the database once imported, when it has been measured (it is much
	// more than the download); 0 where it was not.
	StorageBytes int64 `json:"storageBytes,omitempty"`
	// Headwords are the languages whose words the worker keeps from the package (the language of the edition, and for the
	// Portuguese one the languages of the library): the rest of the file is let go, which keeps what it takes in the database
	// to what is read.
	Headwords  []string   `json:"headwords"`
	Languages  []Language `json:"languages"`
	License    string     `json:"license"`
	LicenseURL string     `json:"licenseUrl"`
	Source     string     `json:"source"`
	SourceURL  string     `json:"sourceUrl"`
	// Installable is whether this version of the server can import the package. The others are listed so that the owner
	// sees what is coming, and cannot be installed.
	Installable bool `json:"installable"`
}

// Host is the only host a package is downloaded from.
const Host = "kaikki.org"

const (
	licenseName = "CC BY-SA 4.0 e GFDL"
	licenseURL  = "https://creativecommons.org/licenses/by-sa/4.0/deed.pt-br"
	sourceName  = "Wikcionário, extraído com o Wiktextract (kaikki.org)"
	sourceURL   = "https://kaikki.org/dictionary/rawdata.html"
)

func edition(code string) string {
	return fmt.Sprintf("https://%s/dictionary/downloads/%s/%s-extract.jsonl.gz", Host, code, code)
}

// wiktionary is one edition of the Wiktionary that kaikki.org publishes: the one in a language, whose definitions are in it.
type wiktionary struct {
	code string // of the edition, as in the address
	// headword is the language of the words the edition defines when it is not the language of the edition (the "simple"
	// edition defines English words).
	headword string
	name     string // of the language, in Portuguese
	bytes    int64  // compressed, as the server said
}

// editions are the Wiktionaries kaikki.org extracts (the English one is a package of its own, which is not here: it is
// 80 times bigger and is the bridge, not a dictionary to read in). Sorted by the name of the language in Portuguese.
var editions = []wiktionary{
	{"de", "", "alemão", 308_579_949},
	{"zh", "", "chinês", 233_265_421},
	{"ko", "", "coreano", 26_833_091},
	{"ku", "", "curdo", 57_931_901},
	{"es", "", "espanhol", 102_038_407},
	{"fr", "", "francês", 733_854_991},
	{"el", "", "grego", 113_503_515},
	{"nl", "", "holandês", 133_385_673},
	{"id", "", "indonésio", 2_832_536},
	{"simple", "en", "inglês simples", 4_719_269},
	{"it", "", "italiano", 43_612_281},
	{"ja", "", "japonês", 64_066_672},
	{"ms", "", "malaio", 6_163_151},
	{"pl", "", "polonês", 137_385_920},
	{"ru", "", "russo", 306_932_298},
	{"th", "", "tailandês", 75_885_107},
	{"cs", "", "tcheco", 39_231_427},
	{"tr", "", "turco", 44_063_619},
	{"vi", "", "vietnamita", 33_774_098},
}

// big is where a package is big enough for the owner to be told it takes a while.
const big = 200 * 1024 * 1024

func ownEdition(w wiktionary) Package {
	language := w.headword
	if language == "" {
		language = w.code
	}
	description := "Definições em " + w.name + ", para quem lê em " + w.name + "."
	if w.code == "simple" {
		description = "Definições em inglês simples (a edição \"simple\" do Wikcionário), um dicionário pequeno de inglês."
	}
	if w.bytes >= big {
		description += " É grande: leva um tempo para baixar e importar."
	}
	return Package{
		ID: "wikt-" + w.code, Name: "Wikcionário em " + w.name, Description: description,
		Edition: w.code, URL: edition(w.code), DownloadBytes: w.bytes,
		Headwords: []string{language}, Languages: []Language{{language, Complete}},
		License: licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL, Installable: true,
	}
}

func buildCatalog() []Package {
	out := []Package{{
		ID: "wikt-pt", Name: "Wikcionário em português",
		Description: "Definições em português e tradução para o português. Cobre o português por inteiro, com as formas flexionadas, e traz as demais línguas como tradução.",
		Edition:     "pt", URL: edition("pt"), DownloadBytes: 37_158_613, StorageBytes: 326_000_000,
		Headwords: []string{"pt", "en", "es", "fr", "de", "it", "ja", "zh"},
		Languages: []Language{{"pt", Complete}, {"en", Partial}, {"es", Weak}, {"fr", Weak}, {"de", Weak}, {"it", Weak}, {"ja", Weak}, {"zh", Weak}},
		License:   licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL, Installable: true,
	}}
	for _, w := range editions {
		out = append(out, ownEdition(w))
	}
	return out
}

// Catalog lists the packages, in the order they are shown: the Portuguese edition first, then the others by the name of
// the language. The sizes are of the compressed files.
var Catalog = buildCatalog()

// Find returns the package with that id.
func Find(id string) (Package, bool) {
	for _, p := range Catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Package{}, false
}

// Validate checks that the package is downloaded from where it should be: an https address on Host, and nowhere else.
func (p Package) Validate() error {
	u, err := url.Parse(p.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() != Host || u.User != nil || u.Port() != "" {
		return fmt.Errorf("the package %s is not at an address the server may download from", p.ID)
	}
	return nil
}
