package equivalence

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// book builds a file's outline and its segments: every node with a size gets segments of that many
// characters, in order, and says which node they are in.
type bookNode struct {
	title string
	depth int
	chars int
	part  string
}

func book(nodes ...bookNode) ([]Node, []Segment) {
	var out []Node
	var segs []Segment
	for i, n := range nodes {
		part := n.part
		if part == "" {
			part = PartBody
		}
		out = append(out, Node{Title: n.title, Depth: n.depth, Chars: n.chars, Part: part})
		for left := n.chars; left > 0; left -= 700 {
			size := left
			if size > 700 {
				size = 700
			}
			seq := len(segs)
			segs = append(segs, Segment{
				Sequence: seq, Node: i, Part: part, Text: strings.Repeat("x", size),
				Locator: json.RawMessage(fmt.Sprintf(`{"type":"epub","href":"n%d.xhtml"}`, i)),
			})
		}
	}
	return out, segs
}

func chapters28(prefix string, first int) []bookNode {
	var out []bookNode
	for i := 1; i <= 28; i++ {
		out = append(out, bookNode{title: fmt.Sprintf("%s %d", prefix, first+i-1), chars: 1400})
	}
	return out
}

func titles(d *Division) string {
	var t []string
	for _, u := range d.Units {
		t = append(t, u.Title)
	}
	return strings.Join(t, "|")
}

func TestBuildDivision_CutsTheStoryAtADepth(t *testing.T) {
	nodes, segs := book(
		bookNode{"Prefácio", 0, 1400, PartFront},
		bookNode{"Livro Um", 0, 700, ""}, bookNode{"Capítulo 1", 1, 1400, ""}, bookNode{"Capítulo 2", 1, 1400, ""},
		bookNode{"Livro Dois", 0, 700, ""}, bookNode{"Capítulo 3", 1, 1400, ""},
		bookNode{"Apêndice", 0, 1400, PartBack},
	)
	if got := titles(BuildDivision(nodes, segs, 1)); got != "Capítulo 1|Capítulo 2|Capítulo 3" {
		t.Errorf("chapters: %s", got)
	}
	parts := BuildDivision(nodes, segs, 0)
	if got := titles(parts); got != "Livro Um|Livro Dois" {
		t.Errorf("parts: %s", got)
	}
	if parts.Units[0].Chars != 700+1400+1400 {
		t.Errorf("a part holds everything under it: %d", parts.Units[0].Chars)
	}
}

func TestBuildDivision_ATitleWithNoTextIsNotAChapterAndTheOnesAfterItMoveUp(t *testing.T) {
	nodes, segs := book(bookNode{"Título", 0, 40, ""}, bookNode{"Capítulo 1", 0, 1400, ""}, bookNode{"Capítulo 2", 0, 1400, ""})
	div := BuildDivision(nodes, segs, 0)
	if titles(div) != "Capítulo 1|Capítulo 2" {
		t.Fatalf("got %s", titles(div))
	}
	if i, ok := div.UnitOf(segs[len(segs)-1].Sequence); !ok || i != 1 {
		t.Errorf("the last segment should be in the second unit, got %d %v", i, ok)
	}
	if _, ok := div.UnitOf(0); ok {
		t.Error("the title's own segment is in no unit")
	}
}

func TestBuildDivision_NumbersComeFromTheTitleOrFromTheOnlyEntryItIsUnder(t *testing.T) {
	nodes, segs := book(
		bookNode{"CHAPTER I.", 0, 10, ""}, bookNode{"THE GUN CLUB.", 1, 1400, ""},
		bookNode{"CHAPTER II.", 0, 10, ""}, bookNode{"PRESIDENT BARBICANE'S COMMUNICATION.", 1, 1400, ""},
	)
	div := BuildDivision(nodes, segs, 1)
	if !div.Units[0].HasNumber || div.Units[0].Number != 1 || div.Units[1].Number != 2 {
		t.Errorf("the number is in the entry above: %+v", div.Units)
	}
}

func TestBuildDivision_APartDoesNotLendItsNumberToEachOfItsChapters(t *testing.T) {
	nodes, segs := book(bookNode{"Book One", 0, 10, ""}, bookNode{"The Boy", 1, 1400, ""}, bookNode{"The Duke", 1, 1400, ""})
	for _, u := range BuildDivision(nodes, segs, 1).Units {
		if u.HasNumber {
			t.Errorf("%q has no number of its own: %+v", u.Title, u)
		}
	}
}

func TestBuildDivision_TheNumbersStartingOverBeginAnotherRun(t *testing.T) {
	nodes := append(chapters28("Chapter", 1)[:3], chapters28("Chapter", 1)[:2]...)
	var list []bookNode
	list = append(list, nodes...)
	n, s := book(list...)
	div := BuildDivision(n, s, 0)
	var runs []int
	for _, u := range div.Units {
		runs = append(runs, u.Run)
	}
	if fmt.Sprint(runs) != "[0 0 0 1 1]" {
		t.Errorf("runs %v", runs)
	}
}

func TestBuildDivision_ADivisionWithNoNumberBeforeTheNumbersStartOverOpensTheNextRun(t *testing.T) {
	list := append(chapters28("Chapter", 1)[:3], bookNode{"RECAPITULATING THE FIRST PART", 0, 1400, ""})
	list = append(list, chapters28("Chapter", 1)[:2]...)
	n, s := book(list...)
	var runs []int
	for _, u := range BuildDivision(n, s, 0).Units {
		runs = append(runs, u.Run)
	}
	if fmt.Sprint(runs) != "[0 0 0 1 1 1]" {
		t.Errorf("the preface of the second book belongs to it: %v", runs)
	}
	// ...but one in the middle of a run stays in it.
	list = []bookNode{{"Chapter 1", 0, 1400, ""}, {"Interlúdio", 0, 1400, ""}, {"Chapter 2", 0, 1400, ""}, {"Chapter 3", 0, 1400, ""}}
	n, s = book(list...)
	runs = nil
	for _, u := range BuildDivision(n, s, 0).Units {
		runs = append(runs, u.Run)
	}
	if fmt.Sprint(runs) != "[0 0 0 0]" {
		t.Errorf("an unnumbered division inside a run stays in it: %v", runs)
	}
}

func TestBuildDivision_TheTextAParentHoldsBetweenItsChildrenIsNoChapter(t *testing.T) {
	nodes, segs := book(bookNode{"Livro Um", 0, 1400, ""}, bookNode{"Capítulo 1", 1, 1400, ""}, bookNode{"Capítulo 2", 1, 1400, ""})
	div := BuildDivision(nodes, segs, 1)
	if titles(div) != "Capítulo 1|Capítulo 2" {
		t.Fatalf("got %s", titles(div))
	}
	if _, ok := div.UnitOf(0); ok {
		t.Error("the book's own opening text belongs to no chapter")
	}
}

func TestDepths_IncludeTheOnesAboveTheNodesThatHoldText(t *testing.T) {
	nodes, segs := book(bookNode{"Parte", 0, 0, ""}, bookNode{"Capítulo", 1, 1400, ""})
	// The part holds no text of its own (it opens where its first chapter does), but it is a level.
	if got := fmt.Sprint(Depths(nodes, segs)); got != "[0 1]" {
		t.Errorf("depths %s", got)
	}
}

func flat(prefix string, n int) []bookNode {
	var out []bookNode
	for i := 1; i <= n; i++ {
		out = append(out, bookNode{title: fmt.Sprintf("%s %d", prefix, i), chars: 1400})
	}
	return out
}

func mustAlign(t *testing.T, a, b []bookNode, at int) *Alignment {
	t.Helper()
	sn, ss := book(a...)
	dn, ds := book(b...)
	return Align(sn, ss, dn, ds, at)
}

func TestAlign_ChaptersThatAreTheSameInNumberAndInTheirNumbersAreVerified(t *testing.T) {
	a := mustAlign(t, flat("Capítulo", 28), flat("Chapter", 28), 6*2+1) // the 7th chapter (two segments each)
	if a == nil || !a.Verified || a.Basis != BasisCount || a.SourceUnit != 6 || a.DestUnit != 6 {
		t.Fatalf("got %+v", a)
	}
	if c := a.Candidate(); c.Confidence != High || c.Precision != ChapterOnly || c.Method != MethodStructure {
		t.Errorf("candidate %+v", c)
	}
}

func TestAlign_FindsTheLevelWhereTheTwoBooksHaveTheSameDivisions(t *testing.T) {
	// One edition has only the three parts; the other has the parts and the chapters under them.
	few := []bookNode{{"Livro Primeiro: Duna", 0, 4200, ""}, {"Livro Segundo: Muad'Dib", 0, 4200, ""}, {"Livro Terceiro: O Profeta", 0, 4200, ""}}
	many := []bookNode{{"Book One: Dune", 0, 10, ""}}
	for i := 1; i <= 6; i++ {
		many = append(many, bookNode{fmt.Sprintf("Chapter %02d", i), 1, 700, ""})
	}
	many = append(many, bookNode{"Book Two: Muad'Dib", 0, 10, ""})
	for i := 7; i <= 12; i++ {
		many = append(many, bookNode{fmt.Sprintf("Chapter %02d", i), 1, 700, ""})
	}
	many = append(many, bookNode{"Book Three: The Prophet", 0, 10, ""})
	for i := 13; i <= 18; i++ {
		many = append(many, bookNode{fmt.Sprintf("Chapter %02d", i), 1, 700, ""})
	}
	// A position in the second part of the edition with only parts.
	a := mustAlign(t, few, many, 6+2)
	if a == nil || !a.Verified || a.SourceUnit != 1 || a.Dest.Units[a.DestUnit].Title != "Book Two: Muad'Dib" {
		t.Fatalf("got %+v", a)
	}
}

func TestAlign_NumbersThatDisagreeAreNotTheSameDivisions(t *testing.T) {
	// Twenty-eight and twenty-eight, but one is numbered 2 to 29: they are not the same chapters.
	a := mustAlign(t, flat("Capítulo", 28), func() []bookNode {
		var out []bookNode
		for i := 0; i < 28; i++ {
			out = append(out, bookNode{title: fmt.Sprintf("Chapter %d", 30+i), chars: 1400})
		}
		return out
	}(), 4)
	if a != nil {
		t.Fatalf("the numbers say these do not line up: %+v", a)
	}
}

func TestAlign_AVerifiedDivisionThatIsAThirdOfTheBookIsTooBigToBeHigh(t *testing.T) {
	parts := func(word string) []bookNode {
		return []bookNode{{word + " 1", 0, 4200, ""}, {word + " 2", 0, 4200, ""}, {word + " 3", 0, 4200, ""}}
	}
	a := mustAlign(t, parts("Livro"), parts("Book"), 6+1)
	if a == nil || !a.Verified {
		t.Fatalf("got %+v", a)
	}
	c := a.Candidate()
	if c.Confidence != Medium || c.Evidence["unitShare"].(float64) < 0.3 {
		t.Errorf("the right part, but only the part: %+v", c)
	}
}

func TestAlign_WithoutNumbersOnlyTheCountIsKnownAndItIsNotVerified(t *testing.T) {
	var src, dst []bookNode
	for i := 0; i < 10; i++ {
		src = append(src, bookNode{title: fmt.Sprintf("O que ele viu %d vezes", i), chars: 1400})
		dst = append(dst, bookNode{title: fmt.Sprintf("What he saw, part %s", strings.Repeat("x", i)), chars: 1400})
	}
	a := mustAlign(t, src, dst, 4)
	if a == nil || a.Verified || a.Basis != BasisCount || a.DestUnit != 2 {
		t.Fatalf("got %+v", a)
	}
	if c := a.Candidate(); c.Confidence != Medium {
		t.Errorf("a count that no number backs is medium at most: %+v", c)
	}
}

func TestAlign_ASequelBoundInTheSameFileIsAnotherRun(t *testing.T) {
	// The Portuguese has the first book (5 chapters); the other edition binds it with a second one.
	both := append(flat("Chapter", 5), flat("Chapter", 4)...)
	a := mustAlign(t, flat("Capítulo", 5), both, 2*2+1)
	if a == nil || a.DestUnit != 2 || !a.Verified {
		t.Fatalf("the first run of the same length is the one: %+v", a)
	}
	// The same in the other direction: a position in the sequel of the bound edition has no
	// counterpart in the edition with only the first book.
	sn, ss := book(both...)
	dn, ds := book(flat("Capítulo", 5)...)
	if got := Align(sn, ss, dn, ds, 6*2+1); got != nil {
		t.Errorf("a run of four has no run of four to match: %+v", got)
	}
}

func TestAlign_TwoRunsThatCouldBeTheOneAreNoMatch(t *testing.T) {
	both := append(flat("Chapter", 5), flat("Chapter", 5)...)
	if a := mustAlign(t, flat("Capítulo", 5), both, 3); a != nil {
		t.Fatalf("which of the two books? no guess: %+v", a)
	}
}

func TestAlign_ANumberIsUsedWhenTheCountsDiffer(t *testing.T) {
	// An abridgement: it has chapters 1, 2, 3, 5, 6 of the seven of the source.
	var short []bookNode
	for _, n := range []int{1, 2, 3, 5, 6} {
		short = append(short, bookNode{title: fmt.Sprintf("Chapter %d", n), chars: 1400})
	}
	a := mustAlign(t, flat("Capítulo", 7), short, 4*2) // the fifth chapter
	if a == nil || a.Basis != BasisNumber || a.Dest.Units[a.DestUnit].Number != 5 {
		t.Fatalf("got %+v", a)
	}
	if c := a.Candidate(); c.Confidence != Low {
		t.Errorf("a number alone is low: %+v", c)
	}
	if a := mustAlign(t, flat("Capítulo", 7), short, 3*2); a != nil {
		t.Errorf("the fourth chapter is not in the abridgement: %+v", a)
	}
}

func TestAlign_TooFewDivisionsProveNothing(t *testing.T) {
	unnumbered := func(name string) []bookNode {
		return []bookNode{{name + " a", 0, 1400, ""}, {name + " b", 0, 1400, ""}}
	}
	if a := mustAlign(t, unnumbered("Uma"), unnumbered("One"), 0); a != nil {
		t.Fatalf("two and two prove nothing: %+v", a)
	}
}

func TestAlign_APositionOutsideTheStoryHasNoUnit(t *testing.T) {
	nodes := append([]bookNode{{"Prefácio", 0, 1400, PartFront}}, flat("Capítulo", 5)...)
	sn, ss := book(nodes...)
	dn, ds := book(flat("Chapter", 5)...)
	if a := Align(sn, ss, dn, ds, 0); a != nil {
		t.Fatalf("the preface is not a chapter: %+v", a)
	}
}

func TestAlign_NoOutlineIsNoAlignment(t *testing.T) {
	_, ss := book(flat("Capítulo", 5)...)
	dn, ds := book(flat("Chapter", 5)...)
	if Align(nil, ss, dn, ds, 0) != nil || Align(dn, ds, nil, ss, 0) != nil {
		t.Fatal("a file with no outline has nothing to align")
	}
}

func words(prefix string, n int) string {
	var w []string
	for i := 0; i < n; i++ {
		w = append(w, fmt.Sprintf("%s%d", prefix, i))
	}
	return strings.Join(w, " ")
}

func TestReverse_TheWayBackFindsWhereThePersonWas(t *testing.T) {
	passage := words("palavra", 60)
	source := []Segment{
		{Sequence: 4, Text: words("outra", 60)},
		{Sequence: 5, Text: passage},
		{Sequence: 6, Text: words("terceira", 60)},
	}
	original := source[1]
	if got := Reverse(original, Segment{Sequence: 9, Text: passage}, source); got != Agrees {
		t.Errorf("same passage: %v", got)
	}
	// A candidate whose way back leads to another place in the source.
	distant := Segment{Sequence: 40, Text: words("longe", 60)}
	if got := Reverse(distant, Segment{Sequence: 9, Text: passage}, source); got != Disagrees {
		t.Errorf("a candidate that leads back elsewhere: %v", got)
	}
	// Nothing comes back: nothing is learned.
	if got := Reverse(original, Segment{Sequence: 9, Text: "curto"}, source); got != Unknown {
		t.Errorf("too little to walk back: %v", got)
	}
}

func TestAlign_ANumberThatTheDestinationRepeatsIsNoMatch(t *testing.T) {
	// The destination has two chapters 5 (a sequel bound with it): which one? no guess.
	dst := []bookNode{{"Chapter 1", 0, 1400, ""}, {"Chapter 5", 0, 1400, ""}, {"Chapter 2", 0, 1400, ""}, {"Chapter 5", 0, 1400, ""}}
	if a := mustAlign(t, flat("Capítulo", 7), dst, 4*2); a != nil {
		t.Fatalf("got %+v", a)
	}
}

func TestReverse_ANeighbouringSegmentIsTheSamePlace(t *testing.T) {
	passage := words("palavra", 60)
	source := []Segment{{Sequence: 20, Text: passage}}
	if got := Reverse(Segment{Sequence: 21, Text: "estava um ao lado"}, Segment{Sequence: 9, Text: passage}, source); got != Agrees {
		t.Errorf("one segment away is the same passage cut differently: %v", got)
	}
	if got := Reverse(Segment{Sequence: 24, Text: "estava longe"}, Segment{Sequence: 9, Text: passage}, source); got != Disagrees {
		t.Errorf("four away is somewhere else: %v", got)
	}
}

func partsAndChapters(partTitle, chapterTitle string, parts, perPart int, numberChapters bool) []bookNode {
	var out []bookNode
	n := 0
	for p := 1; p <= parts; p++ {
		out = append(out, bookNode{title: fmt.Sprintf("%s %d", partTitle, p), depth: 0, chars: 10})
		for c := 1; c <= perPart; c++ {
			n++
			title := fmt.Sprintf("%s %d", chapterTitle, n)
			if !numberChapters {
				title = fmt.Sprintf("%s %s", chapterTitle, strings.Repeat("z", n))
			}
			out = append(out, bookNode{title: title, depth: 1, chars: 1400})
		}
	}
	return out
}

func TestAlign_WhenTwoLevelsLineUpTheFinerOneIsUsed(t *testing.T) {
	src := partsAndChapters("Livro", "Capítulo", 3, 4, true)
	dst := partsAndChapters("Book", "Chapter", 3, 4, true)
	sn, ss := book(src...)
	dn, ds := book(dst...)
	// a segment in the 7th chapter
	at := 0
	for _, s := range ss {
		if sn[s.Node].Title == "Capítulo 7" {
			at = s.Sequence
			break
		}
	}
	a := Align(sn, ss, dn, ds, at)
	if a == nil || a.Units != 12 || a.Dest.Units[a.DestUnit].Title != "Chapter 7" {
		t.Fatalf("the chapters (12) are finer than the parts (3): %+v", a)
	}
}

func TestAlign_ALevelWhoseNumbersAgreeBeatsAFinerOneThatOnlyHasTheCount(t *testing.T) {
	// The parts are numbered and agree; the chapters have no numbers at all: the parts are the ones
	// that are verified, even though the chapters are more of them.
	src := partsAndChapters("Livro", "O caso do", 3, 4, false)
	dst := partsAndChapters("Book", "The affair of", 3, 4, false)
	sn, ss := book(src...)
	dn, ds := book(dst...)
	at := ss[len(ss)-1].Sequence
	a := Align(sn, ss, dn, ds, at)
	if a == nil || !a.Verified || a.Units != 3 || a.Dest.Units[a.DestUnit].Title != "Book 3" {
		t.Fatalf("got %+v", a)
	}
}

// A chapter of `size` characters in the middle of a book of five.
func fiveChapters(word string, size int) []bookNode {
	var out []bookNode
	for i := 1; i <= 12; i++ {
		chars := 1400
		if i == 3 {
			chars = size
		}
		out = append(out, bookNode{title: fmt.Sprintf("%s %d", word, i), chars: chars})
	}
	return out
}

func TestApproximate_TheSameFractionOfTheChapterWhateverTheLengthOfEachVersion(t *testing.T) {
	// Chapter 3 is 10 segments in one version and 14 in the other (a translation runs longer).
	sn, ss := book(fiveChapters("Capítulo", 7000)...)
	dn, ds := book(fiveChapters("Chapter", 9800)...)
	// the source: the 8th segment of chapter 3 (two segments in each of chapters 1 and 2 come first)
	at := 4 + 7
	a := Align(sn, ss, dn, ds, at)
	if a == nil {
		t.Fatal("no alignment")
	}
	c := a.Approximate(at)
	if c == nil || c.Precision != Approximate || c.Method != MethodStructure {
		t.Fatalf("got %+v", c)
	}
	// (7 + ½) / 10 = 75% through the chapter: the segment of the 14 whose middle is nearest that is the 11th
	if want := 4 + 10; c.Sequence() != want {
		t.Errorf("landed on segment %d, want %d", c.Sequence(), want)
	}
	if got := c.Evidence["unitFraction"].(float64); got != 0.75 {
		t.Errorf("unitFraction %v", got)
	}
	// The first segment of the source chapter is at the start of the destination one.
	if first := a.Approximate(4); first.Sequence() != 4 {
		t.Errorf("start of the chapter landed on %d", first.Sequence())
	}
	// And the last is at its end.
	if last := a.Approximate(4 + 9); last.Sequence() != 4+13 {
		t.Errorf("end of the chapter landed on %d", last.Sequence())
	}
}

func TestApproximate_ItIsOnlyAsSureAsTheAlignmentAndTheSizeOfWhatItAlignedTo(t *testing.T) {
	small := mustAlign(t, flat("Capítulo", 12), flat("Chapter", 12), 2*3)
	if c := small.Approximate(2 * 3); c == nil || c.Confidence != Medium {
		t.Errorf("a short chapter that the numbers confirm is medium at best, an estimate: %+v", c)
	}
	parts := func(word string) []bookNode {
		return []bookNode{{word + " 1", 0, 4200, ""}, {word + " 2", 0, 4200, ""}, {word + " 3", 0, 4200, ""}}
	}
	big := mustAlign(t, parts("Livro"), parts("Book"), 7)
	if c := big.Approximate(7); c == nil || c.Confidence != Low {
		t.Errorf("an estimate inside a third of the book is low: %+v", c)
	}
	byNumber := mustAlign(t, flat("Capítulo", 7), func() []bookNode {
		var short []bookNode
		for _, n := range []int{1, 2, 3, 5, 6} {
			short = append(short, bookNode{title: fmt.Sprintf("Chapter %d", n), chars: 1400})
		}
		return short
	}(), 4*2)
	if c := byNumber.Approximate(4 * 2); c == nil || c.Confidence != Low {
		t.Errorf("a number alone is low: %+v", c)
	}
}

func TestApproximate_NothingWhenTheSourceSegmentIsNotInItsChapter(t *testing.T) {
	a := mustAlign(t, flat("Capítulo", 12), flat("Chapter", 12), 6)
	if c := a.Approximate(9999); c != nil {
		t.Errorf("got %+v", c)
	}
}

func TestApproximate_ACountThatNoNumberBacksIsLowEvenInAShortChapter(t *testing.T) {
	mk := func(word string) []bookNode {
		var list []bookNode
		for i := 0; i < 12; i++ {
			list = append(list, bookNode{title: fmt.Sprintf("%s %s", word, strings.Repeat("z", i+1)), chars: 1400})
		}
		return list
	}
	a := mustAlign(t, mk("O caso"), mk("The case"), 6)
	if a == nil || a.Verified {
		t.Fatalf("expected an unverified count: %+v", a)
	}
	if c := a.Approximate(6); c == nil || c.Confidence != Low {
		t.Errorf("%+v", c)
	}
}
