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
	StorageBytes int64      `json:"storageBytes,omitempty"`
	Languages    []Language `json:"languages"`
	License      string     `json:"license"`
	LicenseURL   string     `json:"licenseUrl"`
	Source       string     `json:"source"`
	SourceURL    string     `json:"sourceUrl"`
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

// Catalog lists the packages, in the order they are shown. The sizes are of the compressed files.
var Catalog = []Package{
	{
		ID: "wikt-pt", Name: "Wikcionário em português",
		Description: "Definições em português e tradução para o português. É o dicionário da primeira versão: cobre o português por inteiro, com as formas flexionadas, e traz as demais línguas como tradução.",
		Edition:     "pt", URL: edition("pt"), DownloadBytes: 37_158_613, StorageBytes: 326_000_000,
		Languages: []Language{{"pt", Complete}, {"en", Partial}, {"es", Weak}, {"fr", Weak}, {"de", Weak}, {"it", Weak}, {"ja", Weak}, {"zh", Weak}},
		License:   licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL, Installable: true,
	},
	{
		ID: "wikt-it", Name: "Wikcionário em italiano",
		Description: "Definições em italiano, para quem lê em italiano.",
		Edition:     "it", URL: edition("it"), DownloadBytes: 43_612_281,
		Languages: []Language{{"it", Complete}}, License: licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL,
	},
	{
		ID: "wikt-ja", Name: "Wikcionário em japonês",
		Description: "Definições em japonês, com as leituras, para quem lê em japonês.",
		Edition:     "ja", URL: edition("ja"), DownloadBytes: 64_066_672,
		Languages: []Language{{"ja", Complete}}, License: licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL,
	},
	{
		ID: "wikt-zh", Name: "Wikcionário em chinês",
		Description: "Definições em chinês (simplificado e tradicional), com a leitura em pinyin.",
		Edition:     "zh", URL: edition("zh"), DownloadBytes: 233_265_421,
		Languages: []Language{{"zh", Complete}}, License: licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL,
	},
	{
		ID: "wikt-de", Name: "Wikcionário em alemão",
		Description: "Definições em alemão, para quem lê em alemão.",
		Edition:     "de", URL: edition("de"), DownloadBytes: 308_579_949,
		Languages: []Language{{"de", Complete}}, License: licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL,
	},
	{
		ID: "wikt-fr", Name: "Wikcionário em francês",
		Description: "Definições em francês, para quem lê em francês. É grande: leva um tempo para baixar e importar.",
		Edition:     "fr", URL: edition("fr"), DownloadBytes: 733_854_991,
		Languages: []Language{{"fr", Complete}}, License: licenseName, LicenseURL: licenseURL, Source: sourceName, SourceURL: sourceURL,
	},
}

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
