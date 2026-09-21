package equivalence

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// story builds a file of `chapters` chapters of two segments each (1400 characters), with an outline,
// then lets the test say what the words of some segments are.
func story(title string, chapters int, extra ...bookNode) File {
	var list []bookNode
	for i := 1; i <= chapters; i++ {
		list = append(list, bookNode{title: fmt.Sprintf("%s %d", title, i), chars: 1400})
	}
	list = append(list, extra...)
	nodes, segs := book(list...)
	return File{Nodes: nodes, Segments: segs}
}

func say(f File, sequence int, text string) {
	f.Segments[sequence].Text = text
}

const (
	passageOne = "a expedição partiu de manhã com cinco homens e um cão que não largava o velho cavalo cinzento durante toda a longa travessia do vale até a casa de pedra junto ao rio"
	namesEN    = "the meeting was held by Barbicane with Nicholl and Ardan near Baltimore after Maston spoke in 1866 and again in 1867 before the whole club"
	namesPT    = "a reunião foi feita por Barbicane com Nicholl e Ardan perto de Baltimore depois que Maston falou em 1866 e de novo em 1867 diante de todo o clube"
)

func firstCandidate(t *testing.T, a Answer) Candidate {
	t.Helper()
	if len(a.Candidates) == 0 {
		t.Fatalf("no candidate: %+v", a)
	}
	return a.Candidates[0]
}

func TestFind_TheSamePassageInTheChapterTheOutlinesAlign(t *testing.T) {
	src, dst := story("Capítulo", 5), story("Chapter", 5)
	say(src, 4, passageOne) // chapter 3
	say(dst, 4, passageOne)
	say(dst, 0, passageOne) // the same words again, in chapter 1 of the other version: a refrain
	a := Find(src, dst, src.Segments[4])
	c := firstCandidate(t, a)
	if a.Status != Found || c.Method != MethodText || c.sequence != 4 || c.Evidence["reverse"] != "agrees" {
		t.Fatalf("got %+v", a)
	}
	if len(a.Candidates) != 1 {
		t.Errorf("the refrain in chapter 1 is not in the chapter the outlines point to: %+v", a.Candidates)
	}
}

func TestFind_WithoutTheOutlineTheRefrainIsAmbiguous(t *testing.T) {
	src, dst := story("Capítulo", 5), story("Chapter", 5)
	say(src, 4, passageOne)
	say(dst, 4, passageOne)
	say(dst, 0, passageOne)
	src.Nodes, dst.Nodes = nil, nil
	for i := range src.Segments {
		src.Segments[i].Part, dst.Segments[i].Part = "", ""
	}
	if a := Find(src, dst, src.Segments[4]); a.Status != Ambiguous {
		t.Fatalf("with nothing to tell the chapters apart, two identical passages are two answers: %+v", a)
	}
}

func TestFind_ABackMatterPageThatNamesEveryoneIsNeverTheAnswer(t *testing.T) {
	appendix := bookNode{"Apêndice: quem é quem", 0, 700, PartBack}
	src, dst := story("Capítulo", 5), story("Chapter", 5, appendix)
	say(src, 4, namesPT)
	say(dst, len(dst.Segments)-1, "who is who: Barbicane, Nicholl, Ardan, Baltimore, Maston, 1866, 1867, and Michel")
	a := Find(src, dst, src.Segments[4])
	for _, c := range a.Candidates {
		if c.sequence == len(dst.Segments)-1 {
			t.Fatalf("the appendix is not the story: %+v", a)
		}
	}
	// The translation in its chapter is found, by names, and the outline says which chapter.
	say(dst, 4, namesEN)
	a = Find(src, dst, src.Segments[4])
	if c := firstCandidate(t, a); a.Status != Found || c.sequence != 4 || c.Method != MethodAnchors {
		t.Fatalf("got %+v", a)
	}
}

func TestFind_InsideAChapterAVerifiedOutlineRaisesWhatTheWayBackConfirms(t *testing.T) {
	src, dst := story("Capítulo", 5), story("Chapter", 5)
	say(src, 4, namesPT)
	say(dst, 4, namesEN)
	c := firstCandidate(t, Find(src, dst, src.Segments[4]))
	// Names alone are low or medium; inside the chapter the numbers of the titles confirm, and the
	// way back leads to the same passage: high.
	if c.Method != MethodAnchors || c.Confidence != High || c.Evidence["reverse"] != "agrees" {
		t.Fatalf("got %+v", c)
	}
}

func TestFind_ACandidateThatLeadsBackElsewhereIsDroppedWhenItRestsOnNamesAlone(t *testing.T) {
	// No outlines, so nothing but the words decides. The destination segment holds the source
	// passage's names and, besides, those of a passage far away in the source, which it resembles
	// more: from it the way back leads there.
	far := "Croghan Impey Ashton Bloomsbury Tampa Gibraltar Sumatra Rocky Hills Stones Quebec Havana Lisboa Porto Faro Braga Viseu Leiria Tomar Elvas Beja Evora Sines Mafra Setubal Aveiro Coimbra Guarda Chaves"
	src := File{Segments: []Segment{{Sequence: 2, Text: namesEN}, {Sequence: 30, Text: far}}}
	dst := File{Segments: []Segment{{Sequence: 7, Text: namesPT + " " + far}}}
	if a := Find(src, dst, src.Segments[0]); a.Status != NotFound {
		t.Fatalf("got %+v", a)
	}
	// The same destination, when the way back does lead to the source passage, is offered.
	dst.Segments[0].Text = namesPT
	if a := Find(src, dst, src.Segments[0]); a.Status != Found || firstCandidate(t, a).Evidence["reverse"] != "agrees" {
		t.Fatalf("got %+v", a)
	}
}

func TestFind_WhenNoPassageIsFoundTheChapterTheOutlinesPointToIsOffered(t *testing.T) {
	src, dst := story("Capítulo", 12), story("Chapter", 12) // twelve: a chapter is a small part of the book
	say(src, 4, "Um texto sem nomes nem números para ligar a nada do outro lado da tradução.")
	say(dst, 4, "Some text with no names or numbers to link to anything in the other version.")
	a := Find(src, dst, src.Segments[4])
	c := firstCandidate(t, a)
	if a.Status != Found || c.Method != MethodStructure || c.Precision != ChapterOnly || c.Confidence != High || c.Section != "Chapter 3" {
		t.Fatalf("got %+v", a)
	}
}

func TestFind_APositionInAPrefaceIsLookedForInThePrefaceOnly(t *testing.T) {
	pref := bookNode{"Prefácio", 0, 1400, PartFront}
	srcNodes, srcSegs := book(append([]bookNode{pref}, story2("Capítulo", 5)...)...)
	dstNodes, dstSegs := book(append([]bookNode{{"Preface", 0, 1400, PartFront}}, story2("Chapter", 5)...)...)
	src, dst := File{Nodes: srcNodes, Segments: srcSegs}, File{Nodes: dstNodes, Segments: dstSegs}
	say(src, 0, namesPT)
	say(dst, 0, namesEN)
	say(dst, 6, namesEN) // the same names in a chapter of the story
	a := Find(src, dst, src.Segments[0])
	if c := firstCandidate(t, a); c.sequence != 0 || len(a.Candidates) != 1 {
		t.Fatalf("a preface is matched with a preface: %+v", a)
	}
}

func story2(title string, chapters int) []bookNode {
	var list []bookNode
	for i := 1; i <= chapters; i++ {
		list = append(list, bookNode{title: fmt.Sprintf("%s %d", title, i), chars: 1400})
	}
	return list
}

func TestFind_WithoutAnyOutlineAFileIsNotTakenForAChapterAsFarAsAnOutlineIs(t *testing.T) {
	mk := func(prefix string) File {
		var segs []Segment
		for i := 0; i < 6; i++ {
			segs = append(segs, Segment{
				Sequence: i, Section: fmt.Sprintf("%s %d", prefix, i+1), Chapter: fmt.Sprintf("f%d.xhtml", i), Text: "texto sem ligação nenhuma",
				Locator: json.RawMessage(fmt.Sprintf(`{"type":"epub","href":"f%d.xhtml"}`, i)),
			})
		}
		return File{Segments: segs}
	}
	src, dst := mk("Capítulo"), mk("Chapter")
	a := Find(src, dst, src.Segments[2])
	c := firstCandidate(t, a)
	if c.Method != MethodStructure || c.Confidence == High {
		t.Fatalf("a file that is a chapter by name is at most medium without an outline to say so: %+v", c)
	}
}

func TestFind_NothingInCommonIsNotFound(t *testing.T) {
	unnumbered := func(word string, n int) File {
		var list []bookNode
		for i := 0; i < n; i++ {
			list = append(list, bookNode{title: fmt.Sprintf("%s %s", word, strings.Repeat("z", i+1)), chars: 1400})
		}
		nodes, segs := book(list...)
		return File{Nodes: nodes, Segments: segs}
	}
	src, dst := unnumbered("O caso", 4), unnumbered("Der Fall", 5) // 4 and 5, and no numbers: the outlines do not line up
	say(src, 2, "sem nomes")
	if a := Find(src, dst, src.Segments[2]); a.Status != NotFound {
		t.Fatalf("got %+v", a)
	}
}

func TestFind_TheStorySaysWhereAPositionInAnUnnumberedButEqualBookIs(t *testing.T) {
	// Same count and no numbers to check it: the chapter is offered, but the words are looked for in
	// the whole story, since a count alone might be wrong.
	mk := func(word string) File {
		var list []bookNode
		for i := 0; i < 6; i++ {
			list = append(list, bookNode{title: fmt.Sprintf("%s %s", word, strings.Repeat("z", i+1)), chars: 1400})
		}
		nodes, segs := book(list...)
		return File{Nodes: nodes, Segments: segs}
	}
	src, dst := mk("O caso"), mk("The case")
	say(src, 4, passageOne)
	say(dst, 8, passageOne) // in chapter 5 of the destination, not chapter 3 where the count points
	a := Find(src, dst, src.Segments[4])
	c := firstCandidate(t, a)
	if c.Method != MethodText || c.sequence != 8 {
		t.Fatalf("the words are looked for everywhere in the story when the alignment is only a count: %+v", a)
	}
}

const shortPassage = "a expedição partiu de manhã com cinco homens e um cão que não largava o velho cavalo cinzento"

func TestFind_AVerifiedOutlineLooksOnlyInsideTheChapterEvenAtATitleBetweenChapters(t *testing.T) {
	mk := func(word string) File {
		nodes, segs := book(
			bookNode{word + " 1", 0, 1400, ""}, bookNode{word + " 2", 0, 1400, ""}, bookNode{"nota", 0, 60, ""},
			bookNode{word + " 3", 0, 1400, ""}, bookNode{word + " 4", 0, 1400, ""},
		)
		return File{Nodes: nodes, Segments: segs}
	}
	src, dst := mk("Capítulo"), mk("Chapter")
	// chapters 1-2 are segments 0-3, the note is 4, chapter 3 is 5-6
	say(src, 5, shortPassage)
	say(dst, 5, shortPassage)
	say(dst, 4, shortPassage) // the note: too short to be a chapter, and in no chapter at all
	a := Find(src, dst, src.Segments[5])
	if len(a.Candidates) != 1 || a.Candidates[0].sequence != 5 {
		t.Fatalf("text that belongs to no chapter is not in the chapter the outlines point to: %+v", a)
	}
}

func TestFind_TheWayBackLooksOnlyInsideTheAlignedChapterOfTheSource(t *testing.T) {
	src, dst := story("Capítulo", 5), story("Chapter", 5)
	full := passageOne
	half := strings.Join(strings.Fields(passageOne)[:20], " ")
	say(src, 4, half) // the source position, in chapter 3, holds part of the passage
	for _, seq := range []int{0, 7, 9} {
		say(src, seq, full) // and the whole passage is in three other places of the source, far from it
	}
	say(dst, 4, full)
	c := firstCandidate(t, Find(src, dst, src.Segments[4]))
	if c.sequence != 4 || c.Confidence != High || c.Evidence["reverse"] != "agrees" {
		t.Fatalf("inside the chapter the way back leads to where the person is: %+v", c)
	}
}

func TestFind_AWordMatchThatLeadsBackElsewhereIsTrustedLess(t *testing.T) {
	half := strings.Join(strings.Fields(passageOne)[:20], " ")
	src := File{Segments: []Segment{{Sequence: 2, Text: half}}}
	for _, seq := range []int{10, 20, 30} {
		src.Segments = append(src.Segments, Segment{Sequence: seq, Text: passageOne})
	}
	dst := File{Segments: []Segment{{Sequence: 7, Text: passageOne}}}
	c := firstCandidate(t, Find(src, dst, src.Segments[0]))
	if c.Method != MethodText || c.Confidence != Medium || c.Evidence["reverse"] != "disagrees" {
		t.Fatalf("the passage is in the destination, but from it three other places come out: %+v", c)
	}
}

func TestFind_ACountThatNoNumberBacksDoesNotRaiseWhatTheWayBackConfirms(t *testing.T) {
	mk := func(word string) File {
		var list []bookNode
		for i := 0; i < 6; i++ {
			list = append(list, bookNode{title: fmt.Sprintf("%s %s", word, strings.Repeat("z", i+1)), chars: 1400})
		}
		nodes, segs := book(list...)
		return File{Nodes: nodes, Segments: segs}
	}
	src, dst := mk("O caso"), mk("The case")
	say(src, 4, namesPT)
	say(dst, 4, namesEN)
	c := firstCandidate(t, Find(src, dst, src.Segments[4]))
	if c.Method != MethodAnchors || c.Evidence["reverse"] != "agrees" || c.Confidence != Medium {
		t.Fatalf("names alone are medium, and a count is no reason to say more: %+v", c)
	}
}

func TestFind_TheOutlineOfOneFileAloneDoesNotMakeTheOtherOneAChapter(t *testing.T) {
	seg := func(seq int, chapter string) Segment {
		return Segment{Sequence: seq, Chapter: chapter, Section: "Capítulo " + chapter, Text: "sem ligação", Node: NoNode}
	}
	src := story("Capítulo", 5) // has an outline
	for i := range src.Segments {
		src.Segments[i].Chapter, src.Segments[i].Section = fmt.Sprintf("f%d", i/2), fmt.Sprintf("Capítulo %d", i/2+1)
	}
	dst := File{}
	for i := 0; i < 6; i++ {
		dst.Segments = append(dst.Segments, seg(i, fmt.Sprintf("f%d", i)))
		dst.Segments[i].Section = fmt.Sprintf("Capítulo %d", i+1) // even a title that matches exactly
	}
	if a := Find(src, dst, src.Segments[4]); a.Status != NotFound {
		t.Fatalf("with an outline on one side only, the other's files are not chapters: %+v", a)
	}
}

func TestFind_WordsAreTheAnswerWhenTheyAreThereAndNamesAreOnlyForWhenTheyAreNot(t *testing.T) {
	src := File{Segments: []Segment{{Sequence: 3, Text: namesEN}}}
	dst := File{Segments: []Segment{{Sequence: 5, Text: namesEN}, {Sequence: 40, Text: namesPT}}}
	if a := Find(src, dst, src.Segments[0]); len(a.Candidates) != 1 || a.Candidates[0].Method != MethodText {
		t.Fatalf("a passage found by its words is not joined by a match of names: %+v", a)
	}
}
