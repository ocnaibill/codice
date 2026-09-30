// Package people says what a person's name is when a file writes it in a catalogue's way (#36): "Herbert,
// Frank, author" is Frank Herbert. The name that is stored and shown is the one people say; what the file
// wrote is kept as an alias, so nothing is lost and searching for it still finds the work.
//
// The rule is deliberately narrow. Only a role word makes the catalogue's way clear ("Sobrenome, Nome,
// papel", or a role at the end of a name), because "Herbert, Frank" and "Plato, Aristotle" look the same
// and a wrong guess puts a name the wrong way round. Such pairs are left as they are and are offered to
// an administrator as people that may be the same (SameName).
package people

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// roles are the words a catalogue puts after a name to say what the person did, in lower case and with
// no accents (see fold), in the languages the library is expected to have. The worker
// (worker/people.py) and the migration that fixed the names that were already stored keep the same list.
var roles = map[string]bool{
	"author": true, "autor": true, "autora": true, "auteur": true, "autore": true,
	"editor": true, "editora": true, "editeur": true, "editore": true,
	"illustrator": true, "ilustrador": true, "ilustradora": true, "illustrateur": true, "illustratore": true,
	"translator": true, "tradutor": true, "tradutora": true, "traductor": true, "traductora": true, "traducteur": true, "traduttore": true,
	"narrator": true, "narrador": true, "narradora": true, "narrateur": true,
	"contributor": true, "colaborador": true,
}

// fold writes a word with no accents, in lower case and with no punctuation around it.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Trim(b.String(), " .;:()[]")
}

func splitParts(name string) []string {
	var parts []string
	for _, p := range strings.Split(name, ",") {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// stripParenthesisedRole takes "(author)" off the end of a name.
func stripParenthesisedRole(name string) (string, bool) {
	name = strings.TrimSpace(name)
	open := strings.LastIndex(name, "(")
	if open > 0 && strings.HasSuffix(name, ")") && roles[fold(name[open+1:len(name)-1])] {
		return strings.TrimSpace(name[:open]), true
	}
	return name, false
}

// Parsed is what a written name says about a person.
type Parsed struct {
	Name    string // the name people say ("Frank Herbert")
	Family  string // the surname, only when the writing says so for certain
	Given   string // the given names, with Family
	Changed bool   // Name is not what was written
}

// Parse reads a name as a file wrote it. The surname and the given names are told apart **only when the
// writing says which is which for certain**: the catalogue's way with a role ("Herbert, Frank, author").
// Anything else (including "Frank Herbert", where which word is the surname depends on the culture) is
// left undivided, and is shown as it is whatever the order a person prefers (#64).
func Parse(raw string) Parsed {
	clean := strings.Join(strings.Fields(raw), " ")
	if clean == "" {
		return Parsed{Changed: raw != ""}
	}
	name, hadRole := stripParenthesisedRole(clean)
	parts := splitParts(name)
	if len(parts) > 1 && roles[fold(parts[len(parts)-1])] {
		parts, hadRole = parts[:len(parts)-1], true
	}
	if !hadRole {
		return Parsed{Name: clean, Changed: clean != raw}
	}
	switch len(parts) {
	case 1:
		name = parts[0]
		return Parsed{Name: name, Changed: name != raw}
	case 2: // "Sobrenome, Nome"
		name = parts[1] + " " + parts[0]
		return Parsed{Name: name, Family: parts[0], Given: parts[1], Changed: name != raw}
	}
	// more than that is a list, or a name with a suffix: not clear, so not touched
	return Parsed{Name: clean, Changed: clean != raw}
}

// NormalizeName is the name a person goes by, from what a file wrote, and whether it is different from it.
func NormalizeName(raw string) (name string, changed bool) {
	p := Parse(raw)
	return p.Name, p.Changed
}

// SplitFromPair tells the surname from the given names when a person has been written both ways and a
// human decided they are the same ("Herbert, Frank" and "Frank Herbert"): the comma says where the surname
// ends. It is the only case besides a role that is certain, because someone confirmed the two are one.
func SplitFromPair(a, b string) (family, given string, ok bool) {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		comma, plain := pair[0], pair[1]
		parts := splitParts(comma)
		if len(parts) != 2 || strings.Contains(plain, ",") {
			continue
		}
		if Key(comma) != "" && Key(comma) == Key(plain) && fold(parts[1]+" "+parts[0]) == fold(plain) {
			return parts[0], parts[1], true
		}
	}
	return "", "", false
}

// Key is a name as a set of words, whichever order they come in: what two spellings of a person have in
// common.
func Key(name string) string {
	var words []string
	for _, w := range strings.FieldsFunc(fold(strings.ReplaceAll(name, ",", " ")), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !roles[w] {
			words = append(words, w)
		}
	}
	for i := 1; i < len(words); i++ { // insertion sort: a name has a handful of words
		for j := i; j > 0 && words[j] < words[j-1]; j-- {
			words[j], words[j-1] = words[j-1], words[j]
		}
	}
	return strings.Join(words, " ")
}

// SameName says whether two names are made of the same words in any order: "Herbert, Frank" and "Frank
// Herbert". It is a hint for someone to decide on, never proof (two people may share their words).
func SameName(a, b string) bool {
	ka, kb := Key(a), Key(b)
	return ka != "" && ka == kb
}
