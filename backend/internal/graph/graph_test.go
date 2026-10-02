package graph

import "testing"

func TestTypes_AreTheNineOfTheDesignInThatOrder(t *testing.T) {
	var keys []string
	for _, ty := range Types {
		keys = append(keys, ty.Key)
	}
	want := []string{"related", "part_of", "kind_of", "about", "exemplifies", "defines", "in_dialogue_with", "opposes", "mentions"}
	if len(keys) != len(want) {
		t.Fatalf("%v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("%v, want %v", keys, want)
		}
	}
}

func TestTypes_EachSaysHowItReadsFromBothEnds(t *testing.T) {
	for _, ty := range Types {
		if ty.Label == "" || len(ty.Pairs) == 0 {
			t.Errorf("%s has no label or no kinds", ty.Key)
		}
		if ty.Symmetric && ty.Inverse != "" {
			t.Errorf("%s reads the same from both ends and has an inverse", ty.Key)
		}
		if !ty.Symmetric && ty.Inverse == "" {
			t.Errorf("%s has a direction and no inverse", ty.Key)
		}
	}
	about, _ := TypeByKey("about")
	if about.Label != "trata de" || about.Inverse != "é tratado em" {
		t.Errorf("%+v", about)
	}
}

func TestType_SymmetricOnesAreTheOnesTheDatabaseTreatsAsUndirected(t *testing.T) {
	// The partial unique index of migration 00041 lists these three, by key.
	got := map[string]bool{}
	for _, ty := range Types {
		if ty.Symmetric {
			got[ty.Key] = true
		}
	}
	want := map[string]bool{"related": true, "in_dialogue_with": true, "opposes": true}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("%s is not symmetric", k)
		}
	}
}

func TestType_AllowsOnlyTheKindsOfItsDesign(t *testing.T) {
	cases := []struct {
		key, source, target string
		ok                  bool
	}{
		{"related", Work, Concept, true}, {"related", Note, Note, true}, {"related", Concept, Work, true},
		{"opposes", Concept, Concept, true}, {"opposes", Note, Work, true},
		{"part_of", Concept, Concept, true}, {"part_of", Work, Concept, false}, {"part_of", Concept, Work, false},
		{"kind_of", Concept, Concept, true}, {"kind_of", Note, Concept, false},
		{"about", Work, Concept, true}, {"about", Concept, Work, false}, {"about", Note, Concept, false}, {"about", Work, Work, false},
		{"exemplifies", Note, Concept, true}, {"exemplifies", Work, Concept, true}, {"exemplifies", Concept, Concept, false},
		{"defines", Note, Concept, true}, {"defines", Work, Concept, false},
		{"in_dialogue_with", Work, Work, true}, {"in_dialogue_with", Note, Note, true}, {"in_dialogue_with", Work, Note, false},
		{"in_dialogue_with", Concept, Concept, false},
		{"mentions", Note, Concept, true}, {"mentions", Note, Work, false}, {"mentions", Concept, Note, false},
	}
	for _, c := range cases {
		ty, ok := TypeByKey(c.key)
		if !ok {
			t.Fatalf("no type %s", c.key)
		}
		if got := ty.Allows(c.source, c.target); got != c.ok {
			t.Errorf("%s from %s to %s: %v, want %v", c.key, c.source, c.target, got, c.ok)
		}
	}
	if _, ok := TypeByKey("made_up"); ok {
		t.Error("a type that is not on the list")
	}
	if _, ok := TypeByKey(""); ok {
		t.Error("an empty type")
	}
}

func TestValidKind(t *testing.T) {
	for _, k := range []string{"work", "concept", "note"} {
		if !ValidKind(k) {
			t.Errorf("%s", k)
		}
	}
	for _, k := range []string{"", "author", "tag", "Work", "quote"} {
		if ValidKind(k) {
			t.Errorf("%q", k)
		}
	}
}

func TestKey_IsWhatANameReadsAsWithNoCaseAccentOrPunctuation(t *testing.T) {
	same := [][2]string{
		{"Inteligência  Artificial!", "inteligencia artificial"},
		{"  ESTOICISMO ", "estoicismo"},
		{"Ação, reação", "acao reacao"},
		{"a-b", "a b"},
		{"a\tb\nc", "a b c"},
	}
	for _, p := range same {
		if Key(p[0]) != Key(p[1]) || Key(p[0]) == "" {
			t.Errorf("%q and %q: %q %q", p[0], p[1], Key(p[0]), Key(p[1]))
		}
	}
	// What is in parentheses is part of the name: unlike a title, a concept is not told apart from its note of edition.
	if Key("Estoicismo (escola)") == Key("Estoicismo") {
		t.Error("the parentheses were dropped")
	}
	if Key("IA") == Key("AI") {
		t.Error("different names")
	}
	for _, empty := range []string{"", "   ", "!!!", "…"} {
		if Key(empty) != "" {
			t.Errorf("%q: %q", empty, Key(empty))
		}
	}
	if Key("Neuromancer 2") == Key("Neuromancer") {
		t.Error("digits count")
	}
}
