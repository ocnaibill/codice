package equivalence

import (
	"strings"
	"testing"
)

// scene is a segment that names the given people.
func scene(sequence int, names ...string) Segment {
	return Segment{Sequence: sequence, Text: "Naquela tarde a conversa continuou entre " + strings.Join(names, ", ") + " e todos os outros presentes."}
}

func TestRareNames_AreTheOnesInAFewSegmentsNotInOneAndNotInAll(t *testing.T) {
	var segments []Segment
	for i := 0; i < 100; i++ {
		names := []string{"Heroina"} // in every segment: says nothing
		if i%2 == 0 {
			names = append(names, "Escudeiro") // in half: still too common
		}
		if i < 4 {
			names = append(names, "Vilao") // in four of a hundred: rare
		}
		if i == 10 {
			names = append(names, "Passante") // said once
		}
		if i == 20 || i == 21 {
			names = append(names, "Mensageiro") // twice
		}
		segments = append(segments, scene(i, names...))
	}
	got := RareNames(segments)
	if strings.Join(got, ",") != "mensageiro,vilao" {
		t.Errorf("%v", got)
	}
}

func TestRareNames_ANumberCountsAndNamesAreFolded(t *testing.T) {
	segments := []Segment{
		{Sequence: 0, Text: "Em 1866 chegou Édouard ao porto de Marselha"},
		{Sequence: 1, Text: "Ao fim de 1866 partiu Édouard de volta"},
		{Sequence: 2, Text: "Nada mais se soube de Édouard depois"},
	}
	got := RareNames(segments)
	// In a book of three segments a name may be in two of them, and "1866" and "edouard" are, once folded.
	if len(got) == 0 || got[0] != "1866" {
		t.Fatalf("%v", got)
	}
	for _, n := range got {
		if n == "Édouard" {
			t.Error("not folded")
		}
	}
}

// manyNames is a book of 2,000 names that are each in two segments, and a few that are in three.
func manyNames(commonest int) []Segment {
	label := func(i int) string {
		letters := []byte("abcdefghijklmnopqrstuvwxyz")
		out := []byte{}
		for n := i + 1; n > 0; n /= 26 {
			out = append(out, letters[n%26])
		}
		return "Pers" + string(out)
	}
	var segments []Segment
	for i := 0; i < 2000+commonest; i++ {
		count := 2
		if i < commonest {
			count = 3
		}
		for k := 0; k < count; k++ {
			segments = append(segments, scene(len(segments), label(i)))
		}
	}
	return segments
}

func TestRareNames_IsTheSameListEveryTimeAndBoundedAt1500(t *testing.T) {
	segments := manyNames(0)
	a, b := RareNames(segments), RareNames(segments)
	if len(a) != 1500 || strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("%d names", len(a))
	}
	for i := 1; i < len(a); i++ {
		if a[i-1] >= a[i] {
			t.Fatal("not sorted")
		}
	}
}

func TestRareNames_WhenThereAreTooManyTheCommonestAreKept(t *testing.T) {
	// 400 names in three segments and 2,000 in two: of 2,400, the 400 commoner are kept, with 1,100 of the others.
	segments := manyNames(400)
	got := map[string]bool{}
	for _, n := range RareNames(segments) {
		got[n] = true
	}
	if len(got) != 1500 {
		t.Fatalf("%d names", len(got))
	}
	// The three-segment names are the first 400: each appears in segments 0..1199, three at a time.
	for i := 0; i < 400; i++ {
		text := segments[3*i].Text
		name := strings.ToLower(strings.TrimSpace(text[strings.Index(text, "Pers"):strings.Index(text, " e todos")]))
		if !got[name] {
			t.Fatalf("the name %q, in three segments, was left out", name)
		}
	}
}

func TestRareNames_NothingForNoSegmentsOrNoNames(t *testing.T) {
	if got := RareNames(nil); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if got := RareNames([]Segment{{Text: "um texto sem nomes nem números nenhuns"}, {Text: "outro igual a esse"}}); len(got) != 0 {
		t.Errorf("%v", got)
	}
}
