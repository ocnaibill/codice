// Package storage decides where managed files live on disk and moves them
// safely. The layout is spec DEC-065: Author/Work/Language — Publisher — Year/File.
// The path is only a location; identity lives in the database (DEC-036).
package storage

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// MaxPathLen is the approximate limit of a relative path, in characters. It is
// conservative on purpose: it protects people who reach the files over SMB or
// download them on Windows (spec 12.1).
const MaxPathLen = 200

// maxAuthorsInFolder: a work with more credited authors (an anthology) goes in
// the "Vários autores" folder. All of them stay linked in the catalog.
const maxAuthorsInFolder = 3

const (
	unknownAuthor  = "Autor desconhecido"
	manyAuthors    = "Vários autores"
	minTitleRunes  = 12
	componentRunes = 120
)

// Meta is what the layout needs to know about a file.
type Meta struct {
	Authors     []string // credited authors, first is the folder author
	Title       string
	Language    string // code such as "pt"
	Publisher   string
	Year        string // publication date; only the year is used
	Series      string
	SeriesIndex float64
	Format      string // lower-case extension without the dot, e.g. "epub"
}

var (
	illegalChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f\x7f]`)
	spaces       = regexp.MustCompile(`\s+`)
	yearPattern  = regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`)
	reserved     = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true}
)

func init() {
	for i := 1; i <= 9; i++ {
		reserved[fmt.Sprintf("com%d", i)] = true
		reserved[fmt.Sprintf("lpt%d", i)] = true
	}
}

// Sanitize makes a string safe as one path component on every common
// filesystem: characters that are not allowed become spaces, Unicode is
// normalised (NFC) with accents kept, and leading or trailing dots and spaces,
// which Windows and some tools mishandle, are removed. It never returns an
// empty string.
func Sanitize(s string) string {
	s = norm.NFC.String(s)
	s = illegalChars.ReplaceAllString(s, " ")
	s = spaces.ReplaceAllString(s, " ")
	s = strings.Trim(s, " .")
	if reserved[strings.ToLower(s)] || reserved[strings.ToLower(strings.SplitN(s, ".", 2)[0])] {
		s += "_"
	}
	if s == "" {
		return "_"
	}
	return s
}

// truncateRunes cuts s to n characters without splitting one, and re-trims.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.Trim(string(r[:n]), " .")
}

var languageNames = map[string]string{
	"pt": "Português", "en": "Inglês", "es": "Espanhol", "fr": "Francês", "de": "Alemão",
	"it": "Italiano", "ja": "Japonês", "ko": "Coreano", "zh": "Chinês", "ru": "Russo",
	"la": "Latim", "nl": "Holandês", "pl": "Polonês", "sv": "Sueco", "tr": "Turco",
}

// languageLabel shows a language by name when known, as in the spec's example,
// and otherwise by its code. "pt-BR" and "pt_br" are the Portuguese.
func languageLabel(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	base := strings.ToLower(strings.FieldsFunc(code, func(r rune) bool { return r == '-' || r == '_' })[0])
	if name, ok := languageNames[base]; ok {
		return name
	}
	return strings.ToUpper(code)
}

func yearOf(s string) string {
	return yearPattern.FindString(s)
}

// authorFolder is the folder of the first credited author, "Vários autores" for
// anthologies, and "Autor desconhecido" when none is known.
func authorFolder(authors []string) string {
	var clean []string
	for _, a := range authors {
		if a = strings.TrimSpace(a); a != "" && !strings.EqualFold(a, "Unknown Author") {
			clean = append(clean, a)
		}
	}
	switch {
	case len(clean) == 0:
		return unknownAuthor
	case len(clean) > maxAuthorsInFolder:
		return manyAuthors
	}
	return Sanitize(truncateRunes(clean[0], componentRunes))
}

// editionFolder is "Language — Publisher — Year", leaving out what is missing.
// It is empty when nothing is known.
func editionFolder(m Meta) string {
	var parts []string
	for _, p := range []string{languageLabel(m.Language), strings.TrimSpace(m.Publisher), yearOf(m.Year)} {
		if p != "" {
			parts = append(parts, truncateRunes(Sanitize(p), 60))
		}
	}
	return strings.Join(parts, " — ")
}

// seriesPosition formats a series number as "01" or "01.5".
func seriesPosition(n float64) string {
	whole := int(n)
	if n == float64(whole) {
		return fmt.Sprintf("%02d", whole)
	}
	return fmt.Sprintf("%02d.%s", whole, strings.TrimLeft(strconv.FormatFloat(n-float64(whole), 'f', -1, 64), "0."))
}

var comicFormats = map[string]bool{"cbz": true, "cbr": true}

// Build returns the relative path (forward slashes) for a file. suffix, when not
// empty, is added before the extension to keep two files apart ("Duna [a3f9]").
// The path is shortened, by cutting the title, to about MaxPathLen characters.
func Build(m Meta, suffix string) string {
	ext := strings.ToLower(strings.TrimPrefix(m.Format, "."))
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = "Sem título"
	}
	tail := ""
	if suffix != "" {
		tail = " [" + suffix + "]"
	}

	for n := componentRunes; ; n -= 10 {
		t := Sanitize(truncateRunes(title, n))
		file := t + tail
		if ext != "" {
			file += "." + ext
		}

		var parts []string
		if comicFormats[ext] && strings.TrimSpace(m.Series) != "" {
			// Comics and manga in a series: Series/NN - Title (spec 12.1).
			folder := t
			if m.SeriesIndex > 0 {
				folder = seriesPosition(m.SeriesIndex) + " - " + t
			}
			parts = []string{Sanitize(truncateRunes(m.Series, componentRunes)), folder, file}
		} else {
			parts = []string{authorFolder(m.Authors), t}
			if ed := editionFolder(m); ed != "" {
				parts = append(parts, ed)
			}
			parts = append(parts, file)
		}
		p := path.Join(parts...)
		if len([]rune(p)) <= MaxPathLen || n <= minTitleRunes {
			return p
		}
	}
}

// Suffix is the short, stable tie-breaker derived from a file id.
func Suffix(fileID int64) string {
	return fmt.Sprintf("%04x", fileID)
}

// Resolve picks the final path: the plain one, or the one with the id suffix
// when the plain one is taken. It never returns a path that exists (exists is
// asked about each candidate), and does not depend on the order files arrive in.
func Resolve(m Meta, fileID int64, exists func(string) bool) string {
	if p := Build(m, ""); !exists(p) {
		return p
	}
	return Build(m, Suffix(fileID))
}
