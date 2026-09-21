package equivalence

import "testing"

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
