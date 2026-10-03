package dictionary

import (
	"encoding/json"
	"os"
	"testing"
)

// The cases are shared with the worker (worker/tests/test_dictionary.py reads the same file): a word is stored by the
// worker and looked up here, and both must read it the same way.
func TestNormalize_FollowsTheSharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/normalize.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct{ In, Out string } `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) < 30 {
		t.Fatalf("only %d cases", len(file.Cases))
	}
	for _, c := range file.Cases {
		if got := Normalize(c.In); got != c.Out {
			t.Errorf("Normalize(%q) = %q, want %q", c.In, got, c.Out)
		}
	}
}

func TestNormalize_AMarkThatMakesALetterStays(t *testing.T) {
	for _, pair := range [][2]string{{"か", "が"}, {"は", "ば"}, {"ひ", "ぴ"}} {
		if Normalize(pair[0]) == Normalize(pair[1]) {
			t.Errorf("%s and %s read as one", pair[0], pair[1])
		}
	}
	if Normalize("café") != Normalize("CAFE") {
		t.Error("an accent on a Latin letter should go")
	}
}

func TestNormalize_IsTheSameWhenDoneTwice(t *testing.T) {
	for _, s := range []string{"Ação", "Straße", "l’amour", "走る", "Ｈａｕｓ", "  «Ação!»  "} {
		if once := Normalize(s); Normalize(once) != once {
			t.Errorf("%q: %q then %q", s, once, Normalize(once))
		}
	}
}
