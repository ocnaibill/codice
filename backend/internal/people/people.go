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

// NormalizeName is the name a person goes by, from what a file wrote, and whether it is different from it.
func NormalizeName(raw string) (name string, changed bool) {
	clean := strings.Join(strings.Fields(raw), " ")
	if clean == "" {
		return "", raw != ""
	}
	name, hadRole := stripParenthesisedRole(clean)
	parts := splitParts(name)
	if len(parts) > 1 && roles[fold(parts[len(parts)-1])] {
		parts, hadRole = parts[:len(parts)-1], true
	}
	if !hadRole {
		return clean, clean != raw
	}
	switch len(parts) {
	case 1:
		name = parts[0]
	case 2: // "Sobrenome, Nome"
		name = parts[1] + " " + parts[0]
	default: // more than that is a list, or a name with a suffix: not clear, so not touched
		return clean, clean != raw
	}
	return name, name != raw
}

// key is a name as a set of words, whichever order they come in.
func key(name string) string {
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
	ka, kb := key(a), key(b)
	return ka != "" && ka == kb
}
