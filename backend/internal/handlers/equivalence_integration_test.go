package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

type equivalenceCandidate struct {
	Precision  string          `json:"precision"`
	Confidence string          `json:"confidence"`
	Method     string          `json:"method"`
	Locator    json.RawMessage `json:"locator"`
	Section    string          `json:"section"`
	Excerpt    string          `json:"excerpt"`
	Evidence   map[string]any  `json:"evidence"`
}

type equivalenceResponse struct {
	Status        string                 `json:"status"`
	Candidates    []equivalenceCandidate `json:"candidates"`
	SourceExcerpt string                 `json:"sourceExcerpt"`
	SourceSection string                 `json:"sourceSection"`
}

func (s *catalogStack) equivalent(a actor, dest, src int64) (int, equivalenceResponse) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=%d", dest, src), "")
	var out equivalenceResponse
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const passage = `A cidade de Constantinopla, cercada havia semanas pelos turcos otomanos, finalmente caiu no ano de 1453, encerrando o que restava do Império Bizantino e mudando para sempre o curso da história.`
const rewordedPassage = `No ano de 1453, depois de longas semanas de cerco pelos turcos otomanos, a cidade de Constantinopla enfim caiu, encerrando o que restava do Império Bizantino e mudando para sempre o curso da história.`
const unrelatedPassage = `O mercado de especiarias em Veneza prosperou durante todo o século XV, atraindo comerciantes de toda a Europa e do Oriente, com rotas que atravessavam o Mediterrâneo inteiro sem cessar.`

func TestEquivalence_FindsTheSamePassageInAnotherFormat(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()

	s.index(epub, segment{text: "Capítulo um introdutório, sem relação com o que vem a seguir na obra inteira.", locator: `{"type":"epub","href":"c1.xhtml"}`})
	s.index(pdf,
		segment{text: unrelatedPassage, locator: `{"type":"pdf","page":0}`},
		segment{text: passage, locator: `{"type":"pdf","page":1}`},
	)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)
	// Ana is really reading the passage about Constantinople, further along than the segment
	// that was indexed for that chapter; the locator alone is what nearestSegment has to work
	// with, so give the chapter itself that text instead.
	s.exec(`UPDATE document_segments SET text = $1 WHERE file_id = $2`, passage, epub)

	code, r := s.equivalent(ana, pdf, epub)
	if code != 200 || r.Status != "found" || len(r.Candidates) != 1 {
		t.Fatalf("%d %+v", code, r)
	}
	c := r.Candidates[0]
	if c.Method != "text" || c.Confidence != "high" {
		t.Errorf("candidate = %+v", c)
	}
	var loc map[string]any
	json.Unmarshal(c.Locator, &loc)
	if loc["page"] != float64(1) {
		t.Errorf("locator = %s", c.Locator)
	}
	if r.SourceExcerpt == "" {
		t.Error("the source excerpt is missing, for the prompt to say where the person was")
	}
	_ = work
}

func TestEquivalence_MatchesAChapterByItsTitleWhenTheWordingDiffersEntirely(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Obra", "X", "obra.epub", "epub")
	source := s.primaryFile(work)
	otherEd := s.addEdition(work, "en")
	dest := s.addFile(otherEd, "epub", "obra.en.epub", "managed")

	s.index(source,
		segment{text: "Um parágrafo qualquer de abertura, sem nada que se destaque.", section: "Introdução", locator: `{"type":"epub","href":"intro.xhtml"}`},
		segment{text: "Um dia, o sol nasceu diferente e ninguém soube explicar o motivo exato.", section: "Capítulo Um: A Partida", locator: `{"type":"epub","href":"c1.xhtml"}`},
	)
	s.index(dest,
		segment{text: "One morning, everything felt unusually still across the whole valley below.", section: "Chapter One: The Departure", locator: `{"type":"epub","href":"c1-en.xhtml"}`},
	)
	s.progress(ana, "PUT", source, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)

	code, r := s.equivalent(ana, dest, source)
	if code != 200 || r.Status != "found" || len(r.Candidates) != 1 {
		t.Fatalf("%d %+v", code, r)
	}
	if c := r.Candidates[0]; c.Method != "structure" || c.Precision != "chapter" || c.Section != "Chapter One: The Departure" {
		t.Errorf("candidate = %+v", c)
	}
}

func TestEquivalence_TwoSimilarlyLikelyPassagesAreAmbiguous(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: passage, locator: `{"type":"epub","href":"c1.xhtml"}`})
	// The same passage, reworded twice in unrelated places (a boilerplate note repeated in two
	// volumes): a PDF has no chapters to break the tie by structure, so nothing here says which
	// of the two is the right one.
	s.index(pdf,
		segment{text: rewordedPassage, locator: `{"type":"pdf","page":0}`},
		segment{text: unrelatedPassage},
		segment{text: unrelatedPassage},
		segment{text: rewordedPassage, locator: `{"type":"pdf","page":9}`},
	)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)

	code, r := s.equivalent(ana, pdf, epub)
	if code != 200 || r.Status != "ambiguous" || len(r.Candidates) < 2 {
		t.Fatalf("two equally plausible passages must not be chosen for the person: %d %+v", code, r)
	}
	if r.Candidates[0].Confidence != r.Candidates[1].Confidence {
		t.Errorf("the point of this case is that neither stands out: %+v", r.Candidates)
	}
}

func TestEquivalence_NothingIsInventedWhenThereIsNoEvidence(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: passage, locator: `{"type":"epub","href":"c1.xhtml"}`})
	s.index(pdf, segment{text: unrelatedPassage, locator: `{"type":"pdf","page":0}`})
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)

	code, r := s.equivalent(ana, pdf, epub)
	if code != 200 || r.Status != "not_found" || len(r.Candidates) != 0 {
		t.Errorf("no evidence must not be turned into a guess: %d %+v", code, r)
	}
	_ = work
}

func TestEquivalence_NoPositionInTheSourceIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: passage})
	s.index(pdf, segment{text: passage})
	// Ana never opened the epub: nothing to search from.
	code, r := s.equivalent(ana, pdf, epub)
	if code != 200 || r.Status != "not_found" {
		t.Errorf("%d %+v", code, r)
	}
}

func TestEquivalence_OnlyThePublishedTextIsSearched(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: passage, locator: `{"type":"epub","href":"c1.xhtml"}`})
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)
	// Written but never published: must not be searchable, exactly as in the search index (#17).
	s.writeGeneration(pdf, segment{text: passage})

	code, r := s.equivalent(ana, pdf, epub)
	if code != 200 || r.Status != "not_found" {
		t.Errorf("an unpublished generation must not be matched: %d %+v", code, r)
	}
}

func TestEquivalence_RefusesFilesThatAreNotOfTheSameWorkOrDoNotExist(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	otherFile := s.primaryFile(other)

	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=%d", pdf, otherFile), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("different works: %d", rec.Code)
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=%d", pdf, epub), ""); rec.Code != 200 {
		t.Errorf("same work: %d", rec.Code)
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=%d", pdf, epub), ""); rec.Code == http.StatusBadRequest {
		t.Errorf("a legitimate pair must not be refused")
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/999999/equivalent?from=%d", epub), ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown destination: %d", rec.Code)
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=999999", pdf), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown source: %d", rec.Code)
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=%d", pdf, pdf), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("the same file: %d", rec.Code)
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent", pdf), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no from at all: %d", rec.Code)
	}
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, pdf)
	if code, r := s.equivalent(ana, pdf, epub); code != 200 || r.Status != "not_found" {
		t.Errorf("a file that cannot be opened: %d %+v", code, r)
	}
	s.exec(`UPDATE files SET availability = 'available' WHERE id = $1`, pdf)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, other)
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/equivalent?from=%d", otherFile, epub), ""); rec.Code != http.StatusNotFound {
		t.Errorf("a retired work's file is not found, like everywhere else in the API: %d", rec.Code)
	}
}

func TestEquivalence_ItIsPersonalLikeAnyProgress(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: passage, locator: `{"type":"epub","href":"c1.xhtml"}`})
	s.index(pdf, segment{text: rewordedPassage, locator: `{"type":"pdf","page":0}`})
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)

	if code, r := s.equivalent(ana, pdf, epub); code != 200 || r.Status != "found" {
		t.Fatalf("ana: %d %+v", code, r)
	}
	if code, r := s.equivalent(bob, pdf, epub); code != 200 || r.Status != "not_found" {
		t.Errorf("bob never read the epub: %d %+v", code, r)
	}
}

func TestEquivalenceAccept_RecordsTheAcceptanceWithBothLocatorsAndIsPersonal(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)
	body := fmt.Sprintf(`{"sourceFileId":%d,"locator":{"type":"pdf","page":4},"method":"text","confidence":"high","precision":"passage"}`, epub)

	rec := s.do(ana, "POST", fmt.Sprintf("/progress/files/%d/equivalent/accept", pdf), body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	row := s.scalar(`SELECT work_id::text || ':' || source_file_id::text || ':' || (source_locator->>'href') || ':'
	                  || destination_file_id::text || ':' || (destination_locator->>'page') || ':' || method
	                  || ':' || confidence || ':' || match_precision
	                  FROM equivalent_position_acceptances WHERE user_id = $1`, idAna)
	want := fmt.Sprintf("%d:%d:c1.xhtml:%d:4:text:high:passage", work, epub, pdf)
	if row != want {
		t.Errorf("recorded = %q, want %q", row, want)
	}

	// The source's own progress is untouched: acceptance never writes anywhere but the audit trail.
	if _, st := s.progress(ana, "GET", epub, ""); st.Position != "c1.xhtml" {
		t.Errorf("the source locator moved: %+v", st)
	}
	if n := s.scalar(`SELECT count(*) FROM equivalent_position_acceptances WHERE user_id = $1`, idBob); n != "0" {
		t.Errorf("it must be per user")
	}
}

func TestEquivalenceAccept_ValidatesItsInput(t *testing.T) {
	s := newCatalogStack(t)
	_, _, pdf := s.bookWithTwoFiles()
	url := fmt.Sprintf("/progress/files/%d/equivalent/accept", pdf)
	for name, body := range map[string]string{
		"no locator":     `{"method":"text","confidence":"high","precision":"passage"}`,
		"bad method":     `{"locator":{"type":"pdf","page":1},"method":"guess","confidence":"high","precision":"passage"}`,
		"bad confidence": `{"locator":{"type":"pdf","page":1},"method":"text","confidence":"certain","precision":"passage"}`,
		"bad precision":  `{"locator":{"type":"pdf","page":1},"method":"text","confidence":"high","precision":"word"}`,
		"not json":       `nope`,
	} {
		if rec := s.do(ana, "POST", url, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := s.do(ana, "POST", "/progress/files/999999/equivalent/accept", `{"locator":{"type":"pdf","page":1},"method":"text","confidence":"high","precision":"passage"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown file: %d", rec.Code)
	}
}

func TestEquivalenceAccept_AnEstimateInsideTheChapterIsRecordedAsWhatItWas(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml"}}`)
	body := fmt.Sprintf(`{"sourceFileId":%d,"locator":{"type":"pdf","page":4},"method":"structure","confidence":"medium","precision":"approximate"}`, epub)
	if rec := s.do(ana, "POST", fmt.Sprintf("/progress/files/%d/equivalent/accept", pdf), body); rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT match_precision FROM equivalent_position_acceptances WHERE user_id = $1`, idAna); got != "approximate" {
		t.Errorf("recorded precision = %q", got)
	}
}
