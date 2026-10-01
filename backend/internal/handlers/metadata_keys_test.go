package handlers

import (
	"fmt"
	"strings"
	"testing"
)

func TestCandidateKeys_AreListedInAStableOrderWhateverTheNumberOfSchemes(t *testing.T) {
	ids := []string{}
	for _, s := range []string{"viaf", "isni", "wikidata", "openlibrary", "comicvine", "goodreads", "lccn", "bnf"} {
		ids = append(ids, fmt.Sprintf(`"%s":"K%s"`, s, s))
	}
	evidence := `{"credits":[{"name":"Anne Rice","ids":{` + strings.Join(ids, ",") + `}}]}`
	want := "bnf,comicvine,goodreads,isni,lccn,openlibrary,viaf,wikidata"
	for i := 0; i < 20; i++ {
		var schemes []string
		for _, k := range candidateKeys("author", "Anne Rice", []byte(evidence)) {
			schemes = append(schemes, k.Scheme)
		}
		if got := strings.Join(schemes, ","); got != want {
			t.Fatalf("run %d: schemes = %s, want %s", i, got, want)
		}
	}
}

func TestCandidateKeys_AMalformedListOrAnUnknownFieldKeepsNothingAndNeverPanics(t *testing.T) {
	evidence := []byte(`{"credits":[{"name":"Anne Rice","ids":{"openlibrary":"OL1A"}}]}`)
	for _, c := range []struct{ field, value string }{
		{"contributors", `not json`}, {"contributors", `{"a":1}`}, {"contributors", `[]`}, {"description", "Anne Rice"}, {"", ""},
	} {
		if got := candidateKeys(c.field, c.value, evidence); got == nil || len(got) != 0 {
			t.Errorf("%s %q => %#v", c.field, c.value, got)
		}
	}
	if got := candidateKeys("author", "Anne Rice", nil); got == nil || len(got) != 0 {
		t.Errorf("no evidence => %#v", got)
	}
}
