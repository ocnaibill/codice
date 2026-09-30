package people

import "testing"

// The same vectors are in worker/tests/test_people.py and the migration that fixed the stored names.
var Vectors = []struct{ In, Want string }{
	{"Herbert, Frank, author", "Frank Herbert"},
	{"Herbert, Frank, Author", "Frank Herbert"},
	{"  Herbert ,  Frank ,  author  ", "Frank Herbert"},
	{"Herbert, Frank, author.", "Frank Herbert"},
	{"García Márquez, Gabriel, autor", "Gabriel García Márquez"},
	{"Saint-Exupéry, Antoine de, auteur", "Antoine de Saint-Exupéry"},
	{"Schoenherr, John, illustrator", "John Schoenherr"},
	{"Macedo, Henrique de, tradutor", "Henrique de Macedo"},
	{"Frank Herbert, author", "Frank Herbert"},
	{"Frank Herbert (author)", "Frank Herbert"},
	{"Herbert, Frank (author)", "Frank Herbert"},
	{"Frank Herbert", "Frank Herbert"},
	{"Herbert, Frank", "Herbert, Frank"},                                                     // no role: not clear, left for a person to decide
	{"Plato, Aristotle", "Plato, Aristotle"},                                                 // a list of two
	{"Frank Herbert, Brian Herbert", "Frank Herbert, Brian Herbert"},                         // a list
	{"Herbert, Frank, Schoenherr, John, author", "Herbert, Frank, Schoenherr, John, author"}, // more than a name: not touched
	{"King, Martin Luther, Jr., author", "King, Martin Luther, Jr., author"},
	{"author", "author"}, // a lone word is a name, whatever it means
	{"", ""},
}

func TestNormalizeName(t *testing.T) {
	for _, v := range Vectors {
		got, changed := NormalizeName(v.In)
		if got != v.Want {
			t.Errorf("NormalizeName(%q) = %q, want %q", v.In, got, v.Want)
		}
		if changed != (got != v.In) {
			t.Errorf("NormalizeName(%q) changed = %v for %q", v.In, changed, got)
		}
	}
}

func TestSameName(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Herbert, Frank", "Frank Herbert", true},
		{"Herbert, Frank, author", "Frank Herbert", true},
		{"GARCÍA MÁRQUEZ, Gabriel", "Gabriel Garcia Marquez", true},
		{"Frank Herbert", "Brian Herbert", false},
		{"Herbert", "Frank Herbert", false}, // one word less is not the same name
		{"", "", false},
		{"author", "editor", false},
	} {
		if SameName(c.a, c.b) != c.want || SameName(c.b, c.a) != c.want {
			t.Errorf("SameName(%q, %q) should be %v", c.a, c.b, c.want)
		}
	}
}
