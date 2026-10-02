package graph

import (
	"encoding/json"
	"os"
	"testing"
)

type linkCase struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	Links []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	} `json:"links"`
}

// The cases are shared with the client's reader (frontend/src/features/notes/wikilinks.test.js): the two must read a
// text the same way, or a link would be shown one way and kept the other.
func TestLinksSharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/wikilinks.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []linkCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 30 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := Links(c.Text)
			if len(got) != len(c.Links) {
				t.Fatalf("got %d links %+v, want %d", len(got), got, len(c.Links))
			}
			for i, want := range c.Links {
				if got[i].Name != want.Name || got[i].Label != want.Label {
					t.Errorf("link %d is %q|%q, want %q|%q", i, got[i].Name, got[i].Label, want.Name, want.Label)
				}
				if c.Text[got[i].Start:got[i].Start+2] != "[[" || c.Text[got[i].End-2:got[i].End] != "]]" {
					t.Errorf("link %d: offsets %d-%d do not span the brackets: %q", i, got[i].Start, got[i].End, c.Text[got[i].Start:got[i].End])
				}
			}
		})
	}
}
