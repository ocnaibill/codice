package people

import (
	"reflect"
	"testing"
)

func TestIDsFor(t *testing.T) {
	evidence := `{"query":"Dune","credits":[
		{"name":"Frank Herbert","ids":{"openlibrary":"OL79034A"}},
		{"name":"John Schoenherr","role":"illustrator"},
		{"name":"Twin Name","ids":{"openlibrary":"OL1A"}},
		{"name":"Twin Name","ids":{"openlibrary":"OL2A"}},
		{"name":"Bad Keys","ids":{"Open Library":"OL3A","openlibrary":"has space","ok_scheme":"OL4A"}}]}`
	cases := []struct {
		label, evidence, name string
		want                  map[string]string
	}{
		{"the credit of that name", evidence, "Frank Herbert", map[string]string{"openlibrary": "OL79034A"}},
		{"whichever way the name is written", evidence, "Herbert, Frank", map[string]string{"openlibrary": "OL79034A"}},
		{"a credit without identifiers", evidence, "John Schoenherr", map[string]string{}},
		{"a name the record does not credit gets none from a neighbour", evidence, "Brian Herbert", nil},
		{"two credits with the same name say nothing about which is which", evidence, "Twin Name", nil},
		{"a scheme or value that does not look like one is left out", evidence, "Bad Keys", map[string]string{"ok_scheme": "OL4A"}},
		{"no evidence", ``, "Frank Herbert", nil},
		{"evidence that is not JSON", `not json`, "Frank Herbert", nil},
		{"evidence without credits", `{"query":"x"}`, "Frank Herbert", nil},
		{"an empty name", evidence, "  ", nil},
	}
	for _, c := range cases {
		if got := IDsFor([]byte(c.evidence), c.name); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: IDsFor(%q) = %v, want %v", c.label, c.name, got, c.want)
		}
	}
}
