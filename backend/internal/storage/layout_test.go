package storage

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuild_FollowsTheSpecExample(t *testing.T) {
	got := Build(Meta{Authors: []string{"Frank Herbert"}, Title: "Duna", Language: "pt", Publisher: "Aleph", Year: "2017", Format: "epub"}, "")
	if want := "Frank Herbert/Duna/Português — Aleph — 2017/Duna.epub"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuild_LeavesOutWhatIsMissingWithoutPlaceholders(t *testing.T) {
	cases := []struct {
		name string
		m    Meta
		want string
	}{
		{"no publisher", Meta{Authors: []string{"A"}, Title: "T", Language: "en", Year: "1999", Format: "pdf"}, "A/T/Inglês — 1999/T.pdf"},
		{"only a year", Meta{Authors: []string{"A"}, Title: "T", Year: "2001-05-03", Format: "epub"}, "A/T/2001/T.epub"},
		{"nothing about the edition", Meta{Authors: []string{"A"}, Title: "T", Format: "epub"}, "A/T/T.epub"},
		{"no author", Meta{Title: "T", Format: "epub"}, "Autor desconhecido/T/T.epub"},
		{"the legacy placeholder author", Meta{Authors: []string{"Unknown Author"}, Title: "T", Format: "epub"}, "Autor desconhecido/T/T.epub"},
		{"no title", Meta{Authors: []string{"A"}, Format: "epub"}, "A/Sem título/Sem título.epub"},
		{"a language code with a region", Meta{Authors: []string{"A"}, Title: "T", Language: "pt-BR", Format: "epub"}, "A/T/Português/T.epub"},
		{"an unknown language keeps its code", Meta{Authors: []string{"A"}, Title: "T", Language: "sw", Format: "epub"}, "A/T/SW/T.epub"},
	}
	for _, c := range cases {
		if got := Build(c.m, ""); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuild_AuthorFolder(t *testing.T) {
	first := Build(Meta{Authors: []string{"Neil Gaiman", "Terry Pratchett"}, Title: "Good Omens", Format: "epub"}, "")
	if !strings.HasPrefix(first, "Neil Gaiman/") {
		t.Errorf("two authors go under the first: %q", first)
	}
	anthology := Build(Meta{Authors: []string{"A", "B", "C", "D"}, Title: "Antologia", Format: "epub"}, "")
	if !strings.HasPrefix(anthology, "Vários autores/") {
		t.Errorf("more than three authors go in the shared folder: %q", anthology)
	}
	if strings.Contains(Build(Meta{Authors: []string{"A", "B", "C"}, Title: "T", Format: "epub"}, ""), "Vários") {
		t.Error("exactly three authors still have their own folder")
	}
}

func TestBuild_SeriesFolderOnlyForComicsAndManga(t *testing.T) {
	comic := Build(Meta{Authors: []string{"Alan Moore"}, Title: "Watchmen", Series: "Watchmen", SeriesIndex: 2, Format: "cbz"}, "")
	if comic != "Watchmen/02 - Watchmen/Watchmen.cbz" {
		t.Errorf("comic: %q", comic)
	}
	if got := Build(Meta{Title: "Especial", Series: "Sandman", SeriesIndex: 1.5, Format: "cbr"}, ""); got != "Sandman/01.5 - Especial/Especial.cbr" {
		t.Errorf("fractional position: %q", got)
	}
	if got := Build(Meta{Title: "Anual", Series: "Sandman", Format: "cbz"}, ""); got != "Sandman/Anual/Anual.cbz" {
		t.Errorf("a series without a number: %q", got)
	}
	// A book in a series stays under its author; the series lives in the catalog.
	if got := Build(Meta{Authors: []string{"Frank Herbert"}, Title: "Duna", Series: "Duna", SeriesIndex: 1, Format: "epub"}, ""); !strings.HasPrefix(got, "Frank Herbert/Duna/") {
		t.Errorf("a book in a series: %q", got)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		`AC/DC: Live?`:      "AC DC Live",
		`a<b>c|d"e*f\g`:     "a b c d e f g",
		"  ..Duna..  ":      "Duna",
		"con":               "con_",
		"NUL.txt":           "NUL.txt_",
		"":                  "_",
		"...":               "_",
		"Ação  e   Reação":  "Ação e Reação",
		"Fim\x00do\x1fnome": "Fim do nome",
		"São Paulo":         "São Paulo",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	// Decomposed accents (a + combining tilde) become the composed form.
	if got := Sanitize("Ação"); got != "Ação" {
		t.Errorf("normalisation: %q", got)
	}
}

func TestBuild_NeverProducesAnUnsafePath(t *testing.T) {
	nasty := []string{"../../etc/passwd", "/absolute", `..\..\windows`, "a/../b", "  ", "...", "con", "nul", "title\x00hidden"}
	for _, s := range nasty {
		p := Build(Meta{Authors: []string{s}, Title: s, Publisher: s, Format: "epub"}, "")
		for _, seg := range strings.Split(p, "/") {
			if seg == "" || seg == "." || seg == ".." {
				t.Errorf("%q produced an unsafe segment in %q", s, p)
			}
		}
		if strings.HasPrefix(p, "/") || strings.Contains(p, "\x00") || strings.Contains(p, `\`) {
			t.Errorf("%q produced %q", s, p)
		}
	}
}

func TestBuild_ShortensLongPathsButKeepsSuffixAndExtension(t *testing.T) {
	long := strings.Repeat("Título muito longo ", 30)
	m := Meta{Authors: []string{strings.Repeat("Autor ", 40)}, Title: long, Language: "pt", Publisher: strings.Repeat("Editora ", 20), Year: "2017", Format: "epub"}
	p := Build(m, "00ff")
	if n := utf8.RuneCountInString(p); n > MaxPathLen+60 { // author and edition folders are capped separately
		t.Errorf("path is %d characters: %q", n, p)
	}
	if !strings.HasSuffix(p, " [00ff].epub") {
		t.Errorf("the tie-breaker and the extension must survive shortening: %q", p)
	}
	if !utf8.ValidString(p) {
		t.Error("shortening split a character")
	}
	// A short title is never touched.
	if got := Build(Meta{Authors: []string{"A"}, Title: "Curto", Format: "epub"}, ""); got != "A/Curto/Curto.epub" {
		t.Errorf("%q", got)
	}
}

func TestResolve_TiesAreBrokenByFileIdNotByArrivalOrder(t *testing.T) {
	m := Meta{Authors: []string{"A"}, Title: "Duna", Format: "epub"}
	taken := map[string]bool{"A/Duna/Duna.epub": true}
	exists := func(p string) bool { return taken[p] }

	got := Resolve(m, 0xa3f9, exists)
	if got != "A/Duna/Duna [a3f9].epub" {
		t.Errorf("got %q", got)
	}
	// Same file id, same answer, however many times it is asked.
	if again := Resolve(m, 0xa3f9, exists); again != got {
		t.Errorf("not stable: %q then %q", got, again)
	}
	// A free path is used as it is.
	if got := Resolve(m, 7, func(string) bool { return false }); got != "A/Duna/Duna.epub" {
		t.Errorf("free path: %q", got)
	}
	// Different ids never share a suffix.
	if Suffix(1) == Suffix(2) || Suffix(0x1234) == Suffix(0x12345) {
		t.Error("suffixes collide")
	}
}
