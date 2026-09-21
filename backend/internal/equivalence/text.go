// Package equivalence finds, in one version of a work, the place that matches a place in another
// version: the same passage in the PDF and the EPUB, the same chapter in another edition or in a
// translation (RF-042, spec 18.2). It only reads text that is already stored (document segments) and
// uses no model: word sequences, the structure of the chapters, and names and numbers.
//
// A candidate always points to content that was found in the destination, never to a position worked
// out from a percentage. It says how sure it is and why, so a person can decide; when there is no
// evidence, there is no candidate. Resemblance is not equivalence, and the thresholds here are a first
// guess to be measured (QA-028), not approved limits.
package equivalence

import (
	"hash/fnv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Words is the text as a list of lower-case words with the accents taken off: "Ação, coração!" is
// ["acao", "coracao"]. A word is a run of letters or digits.
func Words(text string) []string {
	var words []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, r := range norm.NFD.String(text) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// an accent that came apart from its letter
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			current.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return words
}

// Fold is one word as Words would write it, for comparing titles and names.
func Fold(text string) string { return strings.Join(Words(text), " ") }

// Shingles are the sequences of n consecutive words of a text, each as a hash. Two texts that share
// many of them share passages, whatever the accents, the case, the punctuation or where a page or a
// chapter breaks.
func Shingles(words []string, n int) map[uint64]struct{} {
	set := map[uint64]struct{}{}
	for i := 0; i+n <= len(words); i++ {
		h := fnv.New64a()
		for _, w := range words[i : i+n] {
			h.Write([]byte(w))
			h.Write([]byte{0})
		}
		set[h.Sum64()] = struct{}{}
	}
	return set
}

// Shared is how many of the source's shingles are also in the candidate, and what fraction of the
// source that is. The fraction is of the source, not of the candidate: a page holds only part of the
// chunk it is being compared with, and a chunk holds only part of the page.
func Shared(source, candidate map[uint64]struct{}) (int, float64) {
	if len(source) == 0 {
		return 0, 0
	}
	n := 0
	for h := range source {
		if _, ok := candidate[h]; ok {
			n++
		}
	}
	return n, float64(n) / float64(len(source))
}

// Anchors are the words that survive a translation: names (capitalized words that do not start a
// sentence) and numbers of two digits or more, folded. "Constantinopla caiu em 1453" gives
// {constantinopla?, 1453}: a name at the start of a sentence is not told from any other capital,
// so it is left out rather than guessed.
func Anchors(text string) map[string]struct{} {
	anchors := map[string]struct{}{}
	runes := []rune(text)
	startOfSentence := true
	for i := 0; i < len(runes); {
		r := runes[i]
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if strings.ContainsRune(".!?…\n", r) {
				startOfSentence = true
			}
			i++
			continue
		}
		j := i
		for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '\'' || runes[j] == '’') {
			j++
		}
		token := asciiDigits(string(runes[i:j]))
		switch {
		case isNumber(token) && len(token) >= 2:
			anchors[token] = struct{}{}
		case unicode.IsUpper(runes[i]) && len(runes[i:j]) >= 4 && !startOfSentence && !isAllUpper(token):
			anchors[Fold(token)] = struct{}{}
		}
		startOfSentence = false
		i = j
	}
	return anchors
}

// asciiDigits writes the digits of Arabic (٠-٩) and of Persian and Urdu (۰-۹) as 0-9, so that a
// number is the same number in a version that writes it with the other digits.
func asciiDigits(s string) string {
	changed := false
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= '٠' && r <= '٩':
			r, changed = '0'+(r-'٠'), true
		case r >= '۰' && r <= '۹':
			r, changed = '0'+(r-'۰'), true
		}
		out = append(out, r)
	}
	if !changed {
		return s
	}
	return string(out)
}

func isNumber(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

func isAllUpper(s string) bool {
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return letters > 1
}

// Excerpt is the start of a text, cut at a word, for showing to a person.
func Excerpt(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	cut := max
	for cut > max/2 && runes[cut] != ' ' {
		cut--
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}
