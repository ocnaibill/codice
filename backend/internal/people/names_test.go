package people

import "testing"

func TestSuggestParts(t *testing.T) {
	cases := []struct {
		in     string
		family string
		given  string
		ok     bool
	}{
		{"Frank Herbert", "Herbert", "Frank", true},
		{"  Frank   Herbert ", "Herbert", "Frank", true},
		{"Andrew S. Tanenbaum", "Tanenbaum", "Andrew S.", true},
		{"J. R. R. Tolkien", "Tolkien", "J. R. R.", true},
		// The last word is the proposal: a compound surname is for the person who confirms to correct.
		{"Gabriel García Márquez", "Márquez", "Gabriel García", true},
		// The catalogue's way, with no role: what comes before the comma.
		{"Herbert, Frank", "Herbert", "Frank", true},
		{"García Márquez, Gabriel", "García Márquez", "Gabriel", true},
		{"Herbert,Frank", "Herbert", "Frank", true},
		// Nothing to propose.
		{"Plato", "", "", false},
		{"", "", "", false},
		{"   ", "", "", false},
		{"Herbert, Frank, Jr", "", "", false},
		{"Herbert,", "", "", false},
		{"Rubem Fonseca Jr.", "", "", false},
		{"Rubem Fonseca Júnior", "", "", false},
		{"Martin Luther King Jr", "", "", false},
		{"Carlos Drummond Filho", "", "", false},
		{"Henry Ford III", "", "", false},
		{"Alan Moore; Dave Gibbons", "", "", false},
		{"Brian K. Vaughan & Fiona Staples", "", "", false},
		{"Alan Moore / Dave Gibbons", "", "", false},
		{"Neal and Jay Kristoff", "", "", false},
		{"Maria e José Silva", "", "", false},
		{"José Ortega y Gasset", "", "", false},
		{"Pierre et Marie Curie", "", "", false},
		{"Karl und Rosa Luxemburg", "", "", false},
	}
	for _, c := range cases {
		got, ok := SuggestParts(c.in)
		if ok != c.ok || got.Family != c.family || got.Given != c.given {
			t.Errorf("SuggestParts(%q) = %+v, %v; want %q, %q, %v", c.in, got, ok, c.family, c.given, c.ok)
		}
	}
}

func TestSuggestParts_AWordThatMerelyContainsAConnectorIsNotOne(t *testing.T) {
	// "Andre" contains "and", "Eça" and "Beatriz" contain "e": only the whole word between spaces is a connector.
	for _, name := range []string{"Andre Agassi", "Eça de Queirós", "Beatriz Nascimento", "Yara Yanez", "Etienne Dupont", "Undine Smith"} {
		if _, ok := SuggestParts(name); !ok {
			t.Errorf("SuggestParts(%q) proposed nothing", name)
		}
	}
}
