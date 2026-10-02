package equivalence

import (
	"fmt"
	"strings"
	"testing"
)

func embedded(id int64, sequence int, vector []float32) Segment {
	return Segment{ID: id, Sequence: sequence, Text: "semantic passage", Embedding: vector,
		EmbeddingProvider: "test", EmbeddingModel: "multilingual", EmbeddingRevision: "r1", EmbeddingPreprocessing: 1}
}

func TestBySemantic_RequiresAMutualNearestCompatibleMatch(t *testing.T) {
	source := embedded(1, 4, []float32{1, 0})
	sources := []Segment{source, embedded(2, 5, []float32{0, 1})}
	destination := []Segment{embedded(10, 8, []float32{.99, .01}), embedded(11, 9, []float32{0, 1})}
	got := BySemantic(source, destination, sources)
	if len(got) != 1 || got[0].Method != MethodSemantic || got[0].sequence != 8 || got[0].Evidence["reverse"] != "agrees" {
		t.Fatalf("got %+v", got)
	}
	destination[0].EmbeddingModel = "another-model"
	if got := BySemantic(source, destination, sources); len(got) != 0 {
		t.Fatalf("incompatible vectors were compared: %+v", got)
	}
}

func TestBySemantic_RejectsAOneWayNearestNeighbour(t *testing.T) {
	source := embedded(1, 4, []float32{1, 0})
	other := embedded(2, 5, []float32{.999, .001})
	destination := []Segment{embedded(10, 8, []float32{.99, .01})}
	if got := BySemantic(source, destination, []Segment{source, other}); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func seg(seq int, chapter, text string) Segment {
	return Segment{ID: int64(seq), Sequence: seq, Chapter: chapter, Text: text, Locator: []byte(`{}`)}
}

const passageA = "A cidade de Constantinopla, cercada havia semanas pelos turcos otomanos, finalmente caiu no ano de 1453, encerrando o que restava do Império Bizantino e mudando para sempre o curso da história europeia e do Mediterrâneo oriental."

func TestByPassage_FindsAnAlmostIdenticalPassageWithHighConfidence(t *testing.T) {
	source := seg(10, "c3", passageA)
	// A layout- or OCR-level difference (a hyphenation, a stray space), not a rewording.
	sameWording := seg(40, "c4", "A cidade de Constantinopla, cercada havia semanas pelos turcos otomanos,  finalmente caiu no ano de 1453, encerrando o que restava do Império Bizantino e mudando para sempre o curso da história europeia e do Mediterrâneo oriental.")
	unrelated := seg(41, "c5", "O mercado de especiarias em Veneza prosperou durante todo o século XV, atraindo comerciantes de toda a Europa e do Oriente, com rotas que atravessavam o Mediterrâneo inteiro.")

	got := ByPassage(source, []Segment{unrelated, sameWording})
	if len(got) != 1 {
		t.Fatalf("candidates = %+v", got)
	}
	if got[0].Locator == nil || got[0].Method != MethodText || got[0].Precision != Passage {
		t.Errorf("candidate = %+v", got[0])
	}
	if got[0].Confidence != High {
		t.Errorf("nearly the same wording should score high: %+v", got[0])
	}
}

func TestByPassage_FindsARewordedPassageWithMediumConfidence(t *testing.T) {
	source := seg(10, "c3", passageA)
	reworded := seg(40, "c4", "No ano de 1453, depois de semanas de cerco pelos turcos otomanos, a cidade de Constantinopla finalmente caiu, encerrando o que restava do Império Bizantino e mudando para sempre o curso da história europeia e do Mediterrâneo oriental, segundo os cronistas da época.")
	got := ByPassage(source, []Segment{reworded})
	if len(got) != 1 || got[0].Confidence != Medium {
		t.Fatalf("a rewording that keeps some sequences intact scores medium: %+v", got)
	}
}

func TestByPassage_TooLittleSourceTextYieldsNothing(t *testing.T) {
	source := seg(1, "c1", "Isso é curto.")
	if got := ByPassage(source, []Segment{seg(2, "c2", "Isso é curto e nada mais.")}); got != nil {
		t.Errorf("too little text to compare must yield nothing, got %+v", got)
	}
}

func TestByPassage_UnrelatedTextYieldsNothing(t *testing.T) {
	source := seg(10, "c1", passageA)
	unrelated := seg(11, "c2", "O gato dormiu a tarde inteira no sofá da sala, sem se importar com o barulho da rua ou com as pessoas que iam e vinham pela casa.")
	if got := ByPassage(source, []Segment{unrelated}); got != nil {
		t.Errorf("unrelated text must yield nothing, got %+v", got)
	}
}

func TestByPassage_DropsANeighbourOfABetterCandidateAndCapsTheList(t *testing.T) {
	source := seg(10, "c1", passageA)
	var candidates []Segment
	for i := 0; i < 6; i++ {
		candidates = append(candidates, seg(20+i, "cx", passageA)) // identical, adjacent-ish segments
	}
	got := ByPassage(source, candidates)
	if len(got) > maxTextCandidates {
		t.Errorf("too many candidates: %d", len(got))
	}
	for i := 0; i < len(got); i++ {
		for j := i + 1; j < len(got); j++ {
			if abs(got[i].sequence-got[j].sequence) <= adjacentSegments {
				t.Errorf("two kept candidates are the same passage: %+v %+v", got[i], got[j])
			}
		}
	}
}

func TestByAnchors_FindsATranslationByTheNumbersThatSurviveIt(t *testing.T) {
	// Names get translated too (Maomé/Mehmed, Constantinopla/Constantinople): what reliably
	// survives across languages is numbers, so this is where the heuristic earns its keep.
	source := seg(1, "c1", "No dia 29 de maio de 1453, cerca de 200 soldados e 12 navios otomanos entraram na cidade cercada.")
	translation := seg(2, "c9", "On the 29th of May 1453, about 200 soldiers and 12 Ottoman ships entered the besieged city.")
	unrelated := seg(90, "c20", "The harvest that year in Flanders was poor, and the merchants of Bruges struggled through a difficult winter season.")

	got := ByAnchors(source, []Segment{unrelated, translation})
	if len(got) != 1 || got[0].Method != MethodAnchors || got[0].chapter != "c9" {
		t.Fatalf("an unrelated candidate must be rejected by the threshold, not just placed second: %+v", got)
	}
}

func TestByAnchors_TooFewNamesInTheSourceYieldsNothing(t *testing.T) {
	source := seg(1, "c1", "Ele caminhou devagar até a porta e parou ali por um instante, pensativo.")
	if got := ByAnchors(source, []Segment{seg(2, "c2", "Ele caminhou devagar até a porta e parou.")}); got != nil {
		t.Errorf("too few anchors must yield nothing, got %+v", got)
	}
}

func TestByStructure_MatchesByTitleThenNumberThenPosition(t *testing.T) {
	src := []Chapter{{Key: "s1", Title: "Introdução", FirstSequence: 0}, {Key: "s2", Title: "Capítulo Um: A Partida", FirstSequence: 5}, {Key: "s3", Title: "Capítulo Dois", FirstSequence: 12}}

	byTitle := []Chapter{{Key: "d1", Title: "Prólogo", FirstSequence: 0}, {Key: "d2", Title: "capitulo um: a partida", FirstSequence: 8}}
	c := ByStructure(1, src, byTitle)
	if c == nil || c.chapter != "d2" || c.Confidence != High || c.Evidence["reason"] != "same title" {
		t.Fatalf("by title: %+v", c)
	}

	byNumber := []Chapter{{Key: "d1", Title: "Chapter 1", FirstSequence: 0}, {Key: "d2", Title: "Chapter 2: The Fall", FirstSequence: 9}}
	c = ByStructure(2, src, byNumber) // "Capítulo Dois" -> number 2
	if c == nil || c.chapter != "d2" || c.Evidence["reason"] != "same chapter number" {
		t.Fatalf("by number: %+v", c)
	}

	byPosition := []Chapter{{Key: "d1", Title: "One", FirstSequence: 0}, {Key: "d2", Title: "Two", FirstSequence: 6}, {Key: "d3", Title: "Three", FirstSequence: 13}}
	c = ByStructure(1, src, byPosition)
	if c == nil || c.chapter != "d2" || c.Confidence != Low || c.Evidence["reason"] != "same number of chapters" {
		t.Fatalf("by position: %+v", c)
	}
}

func TestByStructure_TwoChaptersWithTheSameTitleIsNotAMatch(t *testing.T) {
	src := []Chapter{{Key: "s1", Title: "Nota do Autor", FirstSequence: 0}}
	dst := []Chapter{{Key: "d1", Title: "Nota do Autor", FirstSequence: 0}, {Key: "d2", Title: "Nota do Autor", FirstSequence: 40}}
	if c := ByStructure(0, src, dst); c != nil {
		t.Errorf("an ambiguous title must not be chosen: %+v", c)
	}
}

func TestByStructure_UnequalChapterCountsWithNoTitleOrNumberFindsNothing(t *testing.T) {
	src := []Chapter{{Key: "s1", Title: "Um dia qualquer", FirstSequence: 0}}
	dst := []Chapter{{Key: "d1", Title: "Um dia diferente", FirstSequence: 0}, {Key: "d2", Title: "Outro dia", FirstSequence: 10}}
	if c := ByStructure(0, src, dst); c != nil {
		t.Errorf("no evidence must yield nothing, got %+v", c)
	}
}

func TestByStructure_OutOfRangeIndexIsNotAPanic(t *testing.T) {
	if c := ByStructure(5, []Chapter{{Key: "s1"}}, []Chapter{{Key: "d1"}}); c != nil {
		t.Errorf("out of range: %+v", c)
	}
	if c := ByStructure(0, nil, []Chapter{{Key: "d1"}}); c != nil {
		t.Errorf("empty source: %+v", c)
	}
}

func TestCombine_OneStrongCandidateIsFound(t *testing.T) {
	high := Candidate{Confidence: High, Score: 0.9, chapter: "c2", Evidence: map[string]any{}}
	answer := Combine([]Candidate{high}, nil, nil)
	if answer.Status != Found || len(answer.Candidates) != 1 {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestCombine_TwoCandidatesInTheSameChapterAgreeAndAreFound(t *testing.T) {
	// Same confidence, same chapter: without the agreement rule this would be ambiguous.
	a := Candidate{Confidence: Medium, Score: 0.5, chapter: "c2", Evidence: map[string]any{}}
	b := Candidate{Confidence: Medium, Score: 0.45, chapter: "c2", Evidence: map[string]any{}}
	answer := Combine([]Candidate{a, b}, nil, nil)
	if answer.Status != Found {
		t.Errorf("two candidates of equal confidence pointing at the same chapter agree: %+v", answer)
	}
}

func TestCombine_TwoCandidatesOfEqualConfidenceInDifferentChaptersAreAmbiguous(t *testing.T) {
	a := Candidate{Confidence: Medium, Score: 0.5, chapter: "c2", Evidence: map[string]any{}}
	b := Candidate{Confidence: Medium, Score: 0.45, chapter: "c7", Evidence: map[string]any{}}
	answer := Combine([]Candidate{a, b}, nil, nil)
	if answer.Status != Ambiguous || len(answer.Candidates) != 2 {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestCombine_NoEvidenceAtAllIsNotFound(t *testing.T) {
	if answer := Combine(nil, nil, nil); answer.Status != NotFound || answer.Candidates != nil {
		t.Errorf("answer = %+v", answer)
	}
}

func TestCombine_AnchorsAreUsedOnlyWhenThereIsNoTextMatch(t *testing.T) {
	textMatch := Candidate{Confidence: Medium, Score: 0.5, chapter: "c1", Evidence: map[string]any{}}
	anchorMatch := Candidate{Confidence: High, Score: 0.9, chapter: "c9", Evidence: map[string]any{}}
	answer := Combine([]Candidate{textMatch}, []Candidate{anchorMatch}, nil)
	if len(answer.Candidates) != 1 || answer.Candidates[0].chapter != "c1" {
		t.Fatalf("a text match on wording takes precedence over an unused anchor match: %+v", answer)
	}
	answer = Combine(nil, []Candidate{anchorMatch}, nil)
	if answer.Status != Found || answer.Candidates[0].chapter != "c9" {
		t.Fatalf("with no text match, anchors are used: %+v", answer)
	}
}

func TestCombine_AChapterCandidateThatAgreesIsMarkedNotDuplicated(t *testing.T) {
	passage := Candidate{Confidence: Medium, Score: 0.5, chapter: "c2", Evidence: map[string]any{}}
	chapter := &Candidate{Confidence: Low, Score: 1, chapter: "c2", Evidence: map[string]any{}}
	answer := Combine([]Candidate{passage}, nil, chapter)
	if len(answer.Candidates) != 1 {
		t.Fatalf("the chapter candidate agrees with the passage: must not be added again: %+v", answer)
	}
	if answer.Candidates[0].Evidence["chapterAgrees"] != true {
		t.Errorf("the agreement is recorded: %+v", answer.Candidates[0])
	}
}

func TestCombine_AChapterCandidateThatDisagreesIsAddedAsItsOwn(t *testing.T) {
	passage := Candidate{Confidence: Low, Score: 0.31, chapter: "c2", Evidence: map[string]any{}}
	chapter := &Candidate{Confidence: Low, Score: 1, chapter: "c9", Evidence: map[string]any{}}
	answer := Combine([]Candidate{passage}, nil, chapter)
	if len(answer.Candidates) != 2 {
		t.Fatalf("a disagreeing chapter candidate is its own: %+v", answer)
	}
}

// said is a sentence that names the given people (a name after the first word is what Anchors takes).
func said(names ...string) string {
	return "Naquela tarde a conversa continuou entre " + strings.Join(names, ", ") + " e todos os outros presentes."
}

// told is the same kind of sentence in another language: no words in common but the names.
func told(names ...string) string {
	return "That afternoon the talk went on among " + strings.Join(names, ", ") + " and all of the rest who were there."
}

// crowd is n segments, every one of which names the same four people: the cast of the book.
func crowd(n int) []Segment {
	var out []Segment
	for i := 0; i < n; i++ {
		out = append(out, seg(100+i, "c", said("Alfredo", "Bernardo", "Cassandra", "Dionisio")))
	}
	return out
}

func TestByAnchors_ANameThatIsEverywhereSaysLittleAndARareOneSaysALot(t *testing.T) {
	// The source names the cast and three rare people. Counted equally, every page of the cast shares four of
	// seven names and is as good a candidate as the page of the three rare ones.
	source := seg(1, "s", said("Alfredo", "Bernardo", "Cassandra", "Dionisio", "Evaristo", "Florentina", "Gumercindo"))
	pool := append(crowd(25), seg(7, "target", said("Evaristo", "Florentina", "Gumercindo", "Alfredo")))
	got := ByAnchors(source, pool)
	if len(got) != 1 || got[0].chapter != "target" {
		t.Fatalf("only the page with the rare names is the passage: %+v", got)
	}
}

func TestByAnchors_APageThatIsAListOfCharactersHasNoPrecision(t *testing.T) {
	source := seg(1, "s", said("Alfredo", "Bernardo", "Cassandra", "Dionisio"))
	var list []string
	for i := 0; i < 40; i++ {
		list = append(list, fmt.Sprintf("Personagem%c%c", 'A'+i%26, 'a'+i/26))
	}
	appendix := seg(900, "appendix", said(append([]string{"Alfredo", "Bernardo", "Cassandra", "Dionisio"}, list...)...))
	pool := append(crowd(6)[:0], appendix, seg(5, "other", said("Eufrasia", "Gaspar", "Hortensio", "Ildefonso")))
	if got := ByAnchors(source, pool); len(got) != 0 {
		t.Fatalf("a page that names everybody is not the passage that names four: %+v", got)
	}
	// The same four names on a page that is about them are the passage.
	pool = append(pool, seg(6, "scene", said("Alfredo", "Bernardo", "Cassandra", "Dionisio")))
	got := ByAnchors(source, pool)
	if len(got) != 1 || got[0].chapter != "scene" {
		t.Fatalf("got %+v", got)
	}
	if p := got[0].Evidence["precision"].(float64); p < minAnchorPrecision {
		t.Errorf("precision %v", p)
	}
}

func TestByAnchors_NamesTheDestinationNeverMentionsCountAgainstTheCandidate(t *testing.T) {
	// A translation keeps its names. Three of six names found, three nowhere in the book: not the same book.
	source := seg(1, "s", said("Alfredo", "Bernardo", "Cassandra", "Dionisio", "Evaristo", "Florentina"))
	pool := []Segment{seg(7, "x", said("Alfredo", "Bernardo", "Cassandra", "Gaspar")), seg(8, "y", said("Hortensio", "Ildefonso", "Gaspar", "Jeronimo"))}
	if got := ByAnchors(source, pool); len(got) != 0 {
		t.Fatalf("the sibling book shares part of the cast, not the passage: %+v", got)
	}
	// With the others present somewhere (just not here), the same three are enough.
	pool = append(pool, seg(9, "z", said("Dionisio", "Evaristo", "Florentina", "Gaspar")))
	if got := ByAnchors(source, pool); len(got) == 0 {
		t.Fatalf("names that are in the book count for what they are: %+v", got)
	}
}

func TestByAnchors_ConfidenceDependsOnHowMuchOfTheCandidateTheNamesAre(t *testing.T) {
	source := seg(1, "s", said("Alfredo", "Bernardo", "Cassandra", "Dionisio"))
	tight := seg(7, "tight", said("Alfredo", "Bernardo", "Cassandra", "Dionisio"))
	got := ByAnchors(source, []Segment{tight, seg(8, "other", said("Gaspar", "Hortensio", "Ildefonso", "Jeronimo"))})
	if len(got) != 1 || got[0].Confidence != Medium {
		t.Fatalf("every name of the source, most of the page: %+v", got)
	}
	// Four of its names among fourteen others: found, but not trusted as far.
	var others []string
	for i := 0; i < 14; i++ {
		others = append(others, fmt.Sprintf("Figura%c", 'A'+i))
	}
	diluted := seg(7, "diluted", said(append([]string{"Alfredo", "Bernardo", "Cassandra", "Dionisio"}, others...)...))
	got = ByAnchors(source, []Segment{diluted, seg(8, "other", said("Eufrasia", "Orestes", "Pafuncio", "Quiteria"))})
	if len(got) != 1 || got[0].Confidence != Low {
		t.Fatalf("a fifth of the page is the source's names: %+v", got)
	}
}

func TestWindowAround_AddsTheNeighboursTextAndKeepsTheSegmentsOwn(t *testing.T) {
	pool := []Segment{seg(4, "c", "quatro"), seg(5, "c", "cinco"), seg(6, "c", "seis"), seg(9, "c", "nove")}
	w := windows(pool)
	if w[1].Text != "quatro\ncinco\nseis" || w[1].shown() != "cinco" || w[1].Sequence != 5 || w[1].Chapter != "c" {
		t.Errorf("a window is the segment and its two neighbours: %+v", w[1])
	}
	if w[0].Text != "quatro\ncinco" || w[3].Text != "nove" || w[3].shown() != "nove" {
		t.Errorf("the edges, and a segment with no neighbours: %q %q", w[0].Text, w[3].Text)
	}
	if pool[1].Text != "cinco" || pool[1].own != "" {
		t.Error("the pool was changed")
	}
	// A neighbour that is not in the pool lends nothing: the segment 7 of another chapter is not here.
	if got := windows([]Segment{seg(5, "c", "cinco"), seg(7, "d", "sete")}); got[0].Text != "cinco" || got[1].Text != "sete" {
		t.Errorf("only neighbours in the pool: %+v", got)
	}
}

func TestByAnchors_ThreeNamesAreTooFewToSayWhichPassageItIs(t *testing.T) {
	source := seg(1, "s", said("Alfredo", "Bernardo", "Cassandra"))
	if got := ByAnchors(source, []Segment{seg(2, "t", told("Alfredo", "Bernardo", "Cassandra"))}); got != nil {
		t.Errorf("%+v", got)
	}
	source = seg(1, "s", said("Alfredo", "Bernardo", "Cassandra", "Dionisio"))
	if got := ByAnchors(source, []Segment{seg(2, "t", told("Alfredo", "Bernardo", "Cassandra", "Dionisio"))}); len(got) != 1 {
		t.Errorf("four are enough: %+v", got)
	}
}
