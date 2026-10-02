package equivalence

import (
	"fmt"
	"strings"
	"testing"
)

// name is a person of a story, the same in every language: a capital letter and letters only, so that Anchors takes it.
func name(story string, id int) string {
	letters := []byte("abcdefghijklmnopqrstuvwxyz")
	out := []byte{}
	for n := id + 1; n > 0; n /= 26 {
		out = append(out, letters[n%26])
	}
	return "Pers" + story + string(out)
}

// telling is a book of n segments in some language: each segment is a scene that names four people, two of whom come
// from the scene before (a story goes on). `frame` is the words around the names, which no other language shares.
func telling(story, frame string, n int) File {
	var f File
	for i := 0; i < n; i++ {
		names := []string{name(story, 3*i), name(story, 3*i+1), name(story, 3*i+2)}
		if i > 0 {
			names = append(names, name(story, 3*(i-1)))
		}
		f.Segments = append(f.Segments, Segment{
			Sequence: i, Locator: []byte(fmt.Sprintf(`{"n":%d}`, i)),
			Text: fmt.Sprintf("%s %d %s %s %s.", frame, i, strings.Join(names[:2], " "), frame+"x", strings.Join(names[2:], " ")),
		})
	}
	return f
}

// reframe is the same book cut again: n segments with the same names in the same order, as another edition cuts it.
func reframe(story, frame string, n, scenes int) File {
	whole := telling(story, "x", scenes)
	var f File
	for i := 0; i < n; i++ {
		lo, hi := i*scenes/n, (i+1)*scenes/n
		if hi == lo {
			hi = lo + 1
		}
		var names []string
		for _, s := range whole.Segments[lo:hi] {
			for _, w := range strings.Fields(s.Text) {
				if strings.HasPrefix(w, "Pers") {
					names = append(names, w)
				}
			}
		}
		f.Segments = append(f.Segments, Segment{Sequence: i, Locator: []byte(`{}`), Text: fmt.Sprintf("%s %d %s.", frame, i, strings.Join(names, " "))})
	}
	return f
}

func TestReadParallel_ATranslationIsFoundEndToEndInOrder(t *testing.T) {
	pt, en := telling("a", "a historia seguiu e depois chegou", 150), telling("a", "the story went on and then came", 150)
	got := ReadParallel(pt, en, ParallelSamples)
	if !got.Translation() || got.Hits < 50 || got.Order < 0.99 || got.Samples != ParallelSamples || got.Smaller != 150 || got.Larger != 150 {
		t.Errorf("%+v", got)
	}
	if again := ReadParallel(en, pt, ParallelSamples); !again.Translation() || again.Hits < 50 {
		t.Errorf("the other way round: %+v", again)
	}
}

func TestReadParallel_AnotherCutOfTheSameBookIsStillTheSameBook(t *testing.T) {
	// The other edition cuts the book into 90 segments where the first has 150: a name of a scene is in a neighbour.
	a, b := telling("b", "uma lingua", 150), reframe("b", "another language", 90, 150)
	if got := ReadParallel(a, b, ParallelSamples); !got.Translation() {
		t.Errorf("%+v", got)
	}
}

func TestReadParallel_TheLargerFileMayHoldMoreThanTheSmallerOne(t *testing.T) {
	small := telling("c", "um idioma", 120)
	big := telling("c", "otro idioma", 120)
	for i := 0; i < 100; i++ { // and the book that follows, in the same file
		s := telling("z", "otro idioma", 100).Segments[i]
		s.Sequence = 120 + i
		big.Segments = append(big.Segments, s)
	}
	got := ReadParallel(small, big, ParallelSamples)
	if !got.Translation() || got.Smaller != 120 || got.Larger != 220 {
		t.Errorf("%+v", got)
	}
	if got := ReadParallel(big, small, ParallelSamples); !got.Translation() || got.Smaller != 120 {
		t.Errorf("the order of the arguments does not matter: %+v", got)
	}
}

func TestReadParallel_ABookThatSharesOnlySomePeopleIsNotTheSameBook(t *testing.T) {
	// A sequel: the cast of the first book comes into it now and then, in scenes of its own, where the story takes it:
	// not scene by scene along the first book.
	first := telling("d", "um idioma", 150)
	sequel := telling("e", "um idioma", 150)
	for i := 0; i < 150; i += 2 {
		old := (i*7 + 3) % 150
		sequel.Segments[i].Text += " " + name("d", 3*old) + " " + name("d", 3*old+1) + " " + name("d", 3*old+2) + " " + name("d", 3*(old-1)+300)
	}
	got := ReadParallel(first, sequel, ParallelSamples)
	if got.Translation() {
		t.Errorf("a sequel was taken for a translation: %+v", got)
	}
}

func TestReadParallel_ABookWithNothingInCommonIsNotTheSameBook(t *testing.T) {
	got := ReadParallel(telling("f", "um idioma", 150), telling("g", "otro idioma", 150), ParallelSamples)
	if got.Translation() || got.Hits != 0 || got.Order != -1 {
		t.Errorf("%+v", got)
	}
}

func TestReadParallel_TheSameScenesInAnotherOrderAreNotTheSameBook(t *testing.T) {
	a, b := telling("h", "um idioma", 150), telling("h", "otro idioma", 150)
	for i, j := 0, len(b.Segments)-1; i < j; i, j = i+1, j-1 { // the book read backwards
		b.Segments[i].Text, b.Segments[j].Text = b.Segments[j].Text, b.Segments[i].Text
	}
	got := ReadParallel(a, b, ParallelSamples)
	if got.Translation() || got.Order > 0.5 {
		t.Errorf("%+v", got)
	}
}

func TestReadParallel_OnlyTheBodyOfTheBookIsRead(t *testing.T) {
	a, b := telling("i", "um idioma", 100), telling("i", "otro idioma", 100)
	// Each file has an outline: a front matter and a back matter that name the same people as everything else.
	for _, f := range []*File{&a, &b} {
		for i := range f.Segments {
			f.Segments[i].Node, f.Segments[i].Part = 1, PartBody
		}
		f.Segments[0].Node, f.Segments[0].Part = 0, PartFront
		f.Segments[99].Node, f.Segments[99].Part = 2, PartBack
		f.Nodes = []Node{{Title: "Prefácio", Part: PartFront}, {Title: "Livro", Part: PartBody}, {Title: "Anexo", Part: PartBack}}
	}
	got := ReadParallel(a, b, ParallelSamples)
	if got.Smaller != 98 || got.Larger != 98 || !got.Translation() {
		t.Errorf("%+v", got)
	}
	bare := body(a)
	if len(bare.Nodes) != 0 || bare.Segments[0].Part != "" || bare.Segments[0].Node != NoNode {
		t.Errorf("the comparison does not use the outline: %+v", bare.Segments[0])
	}
}

func TestReadParallel_ThereIsNothingToSayOfFilesTooSmallOrEmpty(t *testing.T) {
	a := telling("j", "um idioma", 150)
	for _, c := range []struct{ a, b File }{{File{}, a}, {a, File{}}, {File{}, File{}}} {
		if got := ReadParallel(c.a, c.b, ParallelSamples); got.Translation() || got.Samples != 0 || got.Order != -1 {
			t.Errorf("%+v", got)
		}
	}
	if got := ReadParallel(a, a, 0); got.Samples != 0 || got.Translation() {
		t.Errorf("no samples: %+v", got)
	}
	// Too short to be read against another, and a book of five scenes is not sampled sixty times.
	if got := ReadParallel(telling("k", "um", 5), telling("k", "otro", 5), ParallelSamples); got.Translation() || got.Samples != 0 {
		t.Errorf("five scenes: %+v", got)
	}
	if got := ReadParallel(telling("k", "um", MinParallelSegments-1), telling("k", "otro", 200), ParallelSamples); got.Samples != 0 {
		t.Errorf("one scene short of the least: %+v", got)
	}
}

func TestReadParallel_AShortFileIsSampledOnceAtEachSegment(t *testing.T) {
	a, b := telling("l", "um idioma", 40), telling("l", "otro idioma", 40)
	got := ReadParallel(a, b, ParallelSamples)
	// Forty segments, the first and the last few left out: every segment in between once, and no more samples than segments.
	if got.Samples != 38 || got.Hits > got.Samples || !got.Translation() {
		t.Errorf("%+v", got)
	}
}

func TestParallel_TranslationNeedsEnoughPassagesEnoughOfTheSampleAndTheOrder(t *testing.T) {
	for _, c := range []struct {
		p    Parallel
		want bool
	}{
		{Parallel{Samples: 60, Hits: 13, Share: 13.0 / 60, Order: 0.88}, true},
		{Parallel{Samples: 60, Hits: 9, Share: 9.0 / 60, Order: 1}, true},
		{Parallel{Samples: 60, Hits: 7, Share: 7.0 / 60, Order: 1}, false},      // too few
		{Parallel{Samples: 20, Hits: 7, Share: 7.0 / 20, Order: 1}, false},      // a big share of a small sample, still too few
		{Parallel{Samples: 40, Hits: 8, Share: 8.0 / 40, Order: 1}, true},       // exactly enough
		{Parallel{Samples: 100, Hits: 11, Share: 0.11, Order: 1}, false},        // enough of them, not enough of the sample
		{Parallel{Samples: 60, Hits: 20, Share: 20.0 / 60, Order: 0.79}, false}, // not in order
		{Parallel{Samples: 60, Hits: 20, Share: 20.0 / 60, Order: -1}, false},
	} {
		if got := c.p.Translation(); got != c.want {
			t.Errorf("%+v: %v", c.p, got)
		}
	}
}

func TestInOrder_IsTheShareOfPairsInTheOrderOfTheBook(t *testing.T) {
	for _, c := range []struct {
		to   []int
		want float64
	}{
		{[]int{1, 5, 9, 20}, 1},
		{[]int{20, 9, 5, 1}, 0},
		{[]int{1, 20, 5, 9}, 4.0 / 6}, // (1,20) (1,5) (1,9) (5,9) in order; (20,5) (20,9) not
		{[]int{4, 4, 4, 4}, 1},        // two passages in one segment are in order: a longer file cut in fewer pieces
		{[]int{4, 4, 3}, 1.0 / 3},     // (4,4) is in order, (4,3) twice is not
		{[]int{7}, -1},
		{nil, -1},
	} {
		if got := inOrder(c.to); got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("%v: %v, want %v", c.to, got, c.want)
		}
	}
}

func TestBest_IsTheMostTrustedThenTheBestScored(t *testing.T) {
	if _, ok := best(nil); ok {
		t.Error("nothing")
	}
	list := []Candidate{
		{Confidence: Medium, Score: 0.95, sequence: 1},
		{Confidence: High, Score: 0.10, sequence: 2},
		{Confidence: Medium, Score: 0.99, sequence: 3},
		{Confidence: High, Score: 0.20, sequence: 4},
	}
	if c, _ := best(list); c.sequence != 4 {
		t.Errorf("the most trusted, then the best scored: %d", c.sequence)
	}
	if c, _ := best(list[:1]); c.sequence != 1 {
		t.Error("one")
	}
	if c, _ := best([]Candidate{{Confidence: Low, Score: 0.5, sequence: 7}, {Confidence: Low, Score: 0.9, sequence: 8}}); c.sequence != 8 {
		t.Errorf("the best scored: %d", c.sequence)
	}
	if c, _ := best([]Candidate{{Confidence: Medium, Score: 0.5, sequence: 7}, {Confidence: Low, Score: 0.9, sequence: 8}}); c.sequence != 7 {
		t.Errorf("trust before score: %d", c.sequence)
	}
}

// twoNames is a book whose segments name two people each: too few, alone, to tell a segment (see minAnchors).
func twoNames(story, frame string, n int) File {
	var f File
	for i := 0; i < n; i++ {
		f.Segments = append(f.Segments, Segment{Sequence: i, Locator: []byte(`{}`), Text: fmt.Sprintf("%s %d %s %s.", frame, i, name(story, 2*i), name(story, 2*i+1))})
	}
	return f
}

func TestReadParallel_NamesAreReadWithTheirNeighboursOnBothSides(t *testing.T) {
	// Two names a segment: a segment alone says nothing, three together say enough.
	a, b := twoNames("m", "um idioma", 100), twoNames("m", "otro idioma", 100)
	if got := ReadParallel(a, b, ParallelSamples); !got.Translation() || got.Hits < 40 {
		t.Errorf("%+v", got)
	}
}

func TestParallelFinder_APlaceThatLeadsBackElsewhereIsNotTheOne(t *testing.T) {
	a, b := telling("n", "um idioma", 150), telling("n", "otro idioma", 150)
	// The first book has a catalogue scene: forty names of people it brings together.
	var catalogue []string
	for k := 0; k < 40; k++ {
		catalogue = append(catalogue, name("zz", k))
	}
	a.Segments[140].Text += " e depois " + strings.Join(catalogue, " ")
	finder := newParallelFinder(body(a), body(b))
	if at, ok := finder.find(body(a).Segments[75]); !ok || abs(at-75) > 1 {
		t.Fatalf("before: %d %v", at, ok)
	}
	// The scene of the other book that has the names of this passage also names thirty of the catalogue: it is
	// still the passage by what it holds of it (enough of its names, and a fair share of its own), but from it the
	// way back leads to the catalogue, which holds more of what it names than the passage does.
	b.Segments[75].Text += " e depois " + strings.Join(catalogue[:30], " ")
	finder = newParallelFinder(body(a), body(b))
	if at, ok := finder.find(body(a).Segments[75]); ok {
		t.Errorf("a place that leads back to the catalogue was taken for the passage: %d", at)
	}
	if forward := newAnchorIndex(windows(body(b).Segments)).match(windowAround(body(a).Segments[75], sequences(body(a).Segments))); len(forward) == 0 {
		t.Error("the place was not even found from the passage: the test does not reach the way back")
	}
}

func TestParallelFinder_AFindIsFoundByItsWordsWhereTheWordsAreTheSame(t *testing.T) {
	a, b := telling("o", "o mesmo idioma", 150), telling("o", "o mesmo idioma", 150)
	finder := newParallelFinder(body(a), body(b))
	if at, ok := finder.find(body(a).Segments[40]); !ok || at != 40 {
		t.Errorf("%d %v", at, ok)
	}
}
