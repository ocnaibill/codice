// Package graph holds the rules of the manual graph (#83, DEC-109): which types of relation there are, between which
// kinds of node each can be drawn, and how the names of a person's concepts are compared. It has no database: what is
// stored and who may see it is in the handlers.
package graph

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Kinds of node.
const (
	Work    = "work"
	Concept = "concept"
	Note    = "note"
)

// Kinds are the nodes a relation can join, for now. Author, tag and quotation come later.
var Kinds = []string{Work, Concept, Note}

// Origins of a relation.
const (
	Manual   = "manual"
	Wikilink = "wikilink"
)

// Type is a kind of relation. Every one is something the person says, never something deduced (the specification, 19:
// similarity is no proof of agreement, opposition or causality). The list is fixed: a person does not make their own,
// for now, so that the vocabulary does not scatter; the comment on each relation carries the nuance.
type Type struct {
	Key       string // what is stored
	Label     string // how it reads from its source ("trata de")
	Inverse   string // how it reads from its target ("é tratado em"); empty for a type with no direction
	Symmetric bool   // it reads the same from both ends
	Pairs     []Pair // the kinds it can join: source, target
}

// Pair is the kinds of the two ends of a relation, source first.
type Pair struct{ Source, Target string }

func anyPair() []Pair {
	var out []Pair
	for _, a := range Kinds {
		for _, b := range Kinds {
			out = append(out, Pair{a, b})
		}
	}
	return out
}

// Types are the nine types, in the order they are offered.
var Types = []Type{
	{Key: "related", Label: "relacionado a", Symmetric: true, Pairs: anyPair()},
	{Key: "part_of", Label: "é parte de", Inverse: "tem como parte", Pairs: []Pair{{Concept, Concept}}},
	{Key: "kind_of", Label: "é um tipo de", Inverse: "tem como tipo", Pairs: []Pair{{Concept, Concept}}},
	{Key: "about", Label: "trata de", Inverse: "é tratado em", Pairs: []Pair{{Work, Concept}}},
	{Key: "exemplifies", Label: "exemplifica", Inverse: "é exemplificado por", Pairs: []Pair{{Note, Concept}, {Work, Concept}}},
	{Key: "defines", Label: "define", Inverse: "é definido em", Pairs: []Pair{{Note, Concept}}},
	{Key: "in_dialogue_with", Label: "dialoga com", Symmetric: true, Pairs: []Pair{{Work, Work}, {Note, Note}}},
	{Key: "opposes", Label: "se opõe a", Symmetric: true, Pairs: anyPair()},
	{Key: "mentions", Label: "menciona", Inverse: "é mencionado em", Pairs: []Pair{{Note, Concept}}},
}

// TypeByKey finds a type, or reports that there is none.
func TypeByKey(key string) (Type, bool) {
	for _, t := range Types {
		if t.Key == key {
			return t, true
		}
	}
	return Type{}, false
}

// Allows says whether the type can join a source of one kind to a target of another.
func (t Type) Allows(source, target string) bool {
	for _, p := range t.Pairs {
		if p.Source == source && p.Target == target {
			return true
		}
	}
	return false
}

// ValidKind says whether it is a kind of node.
func ValidKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Key is what a concept name (or alias) reads as, for comparing: no case, no accents, punctuation and spacing as one
// space. "Inteligência  Artificial!" and "inteligencia artificial" are the same name. Unlike a title it keeps what is
// in parentheses: "Estoicismo (escola)" is not "Estoicismo".
func Key(name string) string {
	var b strings.Builder
	space := true
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		default:
			if !space {
				b.WriteRune(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
