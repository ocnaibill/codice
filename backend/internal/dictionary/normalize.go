package dictionary

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Normalize is what a word reads as: the form a selected word is looked up by. It says the same as normalize in
// worker/dictionary.py, which makes the form the words are stored under, and both are tested against the same cases
// (testdata/normalize.json): a mismatch would be a word that is in the dictionary and is not found.
//
// An accent that is only a mark on a Latin letter goes, and so does case ("Ação" and "ACAO" read as "acao"). A mark that
// makes a letter of another script stays: が is not か. Width forms are made plain, apostrophes are one, and what is
// around the word and is not a letter, a number or a mark (a full stop, a quotation mark) is not part of it.
func Normalize(text string) string {
	folded := cases.Fold().String(norm.NFKC.String(text))
	folded = strings.NewReplacer("’", "'", "ʼ", "'").Replace(folded)
	letters := make([]rune, 0, len(folded))
	for _, r := range folded {
		letters = append(letters, stripMarks(r))
	}
	start, end := 0, len(letters)
	for start < end && !keeps(letters[start]) {
		start++
	}
	for end > start && !keeps(letters[end-1]) {
		end--
	}
	return string(letters[start:end])
}

// stripMarks drops the accent of a Latin letter and leaves everything else as it is.
func stripMarks(r rune) rune {
	decomposed := []rune(norm.NFD.String(string(r)))
	if len(decomposed) > 1 && unicode.Is(unicode.Latin, decomposed[0]) {
		return decomposed[0]
	}
	return r
}

func keeps(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) }
