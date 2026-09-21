package equivalence

import (
	"regexp"
	"strconv"
)

// Chapter is one division of a file's text that has a name: a chapter of an EPUB, a heading of a
// Markdown file. A PDF has none, because its segments only know their page.
type Chapter struct {
	Key           string // what tells it apart inside its file (the chapter's href, the heading)
	Title         string // as the file writes it; may be empty
	FirstSequence int    // where it starts, in reading order
	Locator       []byte // the address of its first segment
	Excerpt       string // the start of its text, to show
	number        int
	hasNumber     bool
}

var (
	// "Book One", "Livro Primeiro" and "Part 2" number the big divisions of a book the way "Chapter 3"
	// numbers the small ones.
	chapterWord = regexp.MustCompile(`(?i)\b(?:cap[ií]tulo|capitulo|chapter|chapitre|kapitel|capitolo|cap\.?|livro|libro|livre|book|buch|libro|parte|part|partie)\s+([0-9]+|[ivxlcdm]+|[\p{L}]+)\b`)
	numberWords = map[string]int{
		// Portuguese, English, Spanish and French, one to twelve: what a chapter is called in the
		// books that write it out.
		"um": 1, "uma": 1, "dois": 2, "duas": 2, "tres": 3, "quatro": 4, "cinco": 5, "seis": 6, "sete": 7, "oito": 8, "nove": 9, "dez": 10, "onze": 11, "doze": 12,
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
		"uno": 1, "dos": 2, "cuatro": 4, "siete": 7, "ocho": 8, "nueve": 9, "diez": 10, "once": 11, "doce": 12,
		"deux": 2, "trois": 3, "quatre": 4, "sept": 7, "huit": 8, "neuf": 9, "dix": 10, "onze_fr": 11, "douze": 12,
		// The ordinals a book's parts are numbered with ("Livro Primeiro", "Book the Second"), one to ten.
		"primeiro": 1, "primeira": 1, "segundo": 2, "segunda": 2, "terceiro": 3, "terceira": 3, "quarto": 4, "quarta": 4, "quinto": 5, "quinta": 5,
		"sexto": 6, "sexta": 6, "setimo": 7, "setima": 7, "oitavo": 8, "oitava": 8, "nono": 9, "nona": 9, "decimo": 10, "decima": 10,
		"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10,
		"primero": 1, "primera": 1, "tercero": 3, "tercera": 3, "cuarto": 4, "cuarta": 4, "septimo": 7, "octavo": 8, "noveno": 9,
		"premier": 1, "premiere": 1, "deuxieme": 2, "troisieme": 3, "quatrieme": 4, "cinquieme": 5, "sixieme": 6,
	}
	romans = map[rune]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100, 'd': 500, 'm': 1000}
)

// ChapterNumber reads the number out of a title that says "Capítulo 3", "Chapter III" or "Capítulo
// três", in the languages above. A title with no such word has no number: a bare "3" or "III" might
// be anything.
func ChapterNumber(title string) (int, bool) {
	match := chapterWord.FindStringSubmatch(title)
	if match == nil {
		return 0, false
	}
	word := Fold(match[1])
	if n, err := strconv.Atoi(word); err == nil && n > 0 {
		return n, true
	}
	if n, ok := numberWords[word]; ok {
		return n, true
	}
	if n, ok := roman(word); ok {
		return n, true
	}
	return 0, false
}

func roman(s string) (int, bool) {
	if s == "" || len(s) > 8 {
		return 0, false
	}
	total, prev := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		v, ok := romans[rune(s[i])]
		if !ok {
			return 0, false
		}
		if v < prev {
			total -= v
		} else {
			total += v
			prev = v
		}
	}
	return total, total > 0
}

func (c *Chapter) prepare() {
	c.number, c.hasNumber = ChapterNumber(c.Title)
}

// SameTitle says whether two chapters are called the same, ignoring case, accents and punctuation.
func SameTitle(a, b string) bool {
	fa, fb := Fold(a), Fold(b)
	return fa != "" && fa == fb
}

// EqualCount reports whether two lists of chapters are as long as each other and long enough to mean
// something: two books that both have twelve chapters probably divide the same way, two that both
// have one prove nothing.
func EqualCount(a, b []Chapter) bool {
	return len(a) == len(b) && len(a) >= minChaptersToCount
}
