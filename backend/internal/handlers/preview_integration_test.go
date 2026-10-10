package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type previewBody struct {
	FileID   int64 `json:"fileId"`
	Total    int   `json:"total"`
	From     int   `json:"from"`
	PrevFrom *int  `json:"prevFrom"`
	NextFrom *int  `json:"nextFrom"`
	Current  *int  `json:"current"`
	Segments []struct {
		Sequence int    `json:"sequence"`
		Text     string `json:"text"`
		Chapter  string `json:"chapter"`
		Part     string `json:"part"`
	} `json:"segments"`
}

func (s *catalogStack) preview(a actor, file int64, query string) (int, previewBody) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/progress/files/%d/preview%s", file, query), "")
	var out previewBody
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func shown(p *int) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}

func TestPreview_IsAWindowOfThePassagesOfTheFileWithTheChapterOfEach(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.outlinedEPUB(epub) // six passages, sequences 0 to 5

	code, out := s.preview(ana, epub, "")
	if code != 200 || out.Total != 6 || len(out.Segments) != 6 || out.From != 0 || out.PrevFrom != nil || out.NextFrom != nil || out.Current != nil {
		t.Fatalf("the whole file, from the start, with nobody's place: %d %+v", code, out)
	}
	var chapters []string
	for _, seg := range out.Segments {
		chapters = append(chapters, seg.Chapter)
	}
	if strings.Join(chapters, "|") != "Capa|Capítulo 1|Capítulo 1|Capítulo 2|Capítulo 3|Apêndice" {
		t.Errorf("the chapter of each passage: %v", chapters)
	}
	if out.Segments[0].Part != "front" || out.Segments[1].Part != "body" || out.Segments[5].Part != "back" {
		t.Errorf("the part of the book: %+v", out.Segments)
	}
	if out.Segments[1].Text != strings.Repeat("x", 100) {
		t.Errorf("the text is the index's: %q", out.Segments[1].Text)
	}
}

func TestPreview_MovesInWindowsAndSaysWhereTheEndsAre(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.outlinedEPUB(epub)

	_, out := s.preview(ana, epub, "?count=2")
	if len(out.Segments) != 2 || out.Segments[0].Sequence != 0 || shown(out.PrevFrom) != "nil" || shown(out.NextFrom) != "2" {
		t.Errorf("first window: %+v prev %s next %s", out.Segments, shown(out.PrevFrom), shown(out.NextFrom))
	}
	_, out = s.preview(ana, epub, "?from=2&count=2")
	if out.Segments[0].Sequence != 2 || shown(out.PrevFrom) != "0" || shown(out.NextFrom) != "4" {
		t.Errorf("middle window: %+v prev %s next %s", out.Segments, shown(out.PrevFrom), shown(out.NextFrom))
	}
	_, out = s.preview(ana, epub, "?from=4&count=2")
	if len(out.Segments) != 2 || out.Segments[1].Sequence != 5 || shown(out.PrevFrom) != "2" || shown(out.NextFrom) != "nil" {
		t.Errorf("last window: %+v prev %s next %s", out.Segments, shown(out.PrevFrom), shown(out.NextFrom))
	}
	// A window that would start before the beginning starts at it.
	_, out = s.preview(ana, epub, "?from=1&count=4")
	if shown(out.PrevFrom) != "0" {
		t.Errorf("not before the start: %s", shown(out.PrevFrom))
	}
	// Past the end there is nothing, and nothing after it.
	_, out = s.preview(ana, epub, "?from=50&count=2")
	if len(out.Segments) != 0 || out.NextFrom != nil {
		t.Errorf("past the end: %+v", out)
	}
	// What is not a count is the default, and a count past the most is the most.
	if _, out := s.preview(ana, epub, "?count=abc"); len(out.Segments) != 6 {
		t.Errorf("default: %d", len(out.Segments))
	}
	for _, bad := range []string{"?from=-1", "?from=abc"} {
		if code, _ := s.preview(ana, epub, bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
}

func TestPreview_StartsWhereThePersonIsAndOnlyForThem(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.outlinedEPUB(epub)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c3.xhtml","progression":0.1}}`) // the passage 4

	_, out := s.preview(ana, epub, "?count=2")
	if out.Current == nil || *out.Current != 4 || out.From != 3 || len(out.Segments) != 2 || out.Segments[0].Sequence != 3 {
		t.Errorf("a passage of lead-in before the place: %+v", out)
	}
	_, out = s.preview(ana, epub, "?from=0&count=2")
	if out.From != 0 || out.Current == nil || *out.Current != 4 {
		t.Errorf("the window that was asked for, and where the place is: %+v", out)
	}
	_, out = s.preview(bob, epub, "?count=2")
	if out.Current != nil || out.From != 0 {
		t.Errorf("bob has not read it: %+v", out)
	}
}

func TestPreview_ANoIndexIsEmptyAndAFileThatIsNotThereIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	code, out := s.preview(ana, pdf, "")
	if code != 200 || out.Total != 0 || out.Segments == nil || len(out.Segments) != 0 {
		t.Errorf("no index: %d %+v", code, out)
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/preview", pdf), ""); !strings.Contains(rec.Body.String(), `"segments":[]`) {
		t.Errorf("an empty list, not null: %s", rec.Body.String())
	}
	if code, _ := s.preview(ana, 999999, ""); code != http.StatusNotFound {
		t.Errorf("unknown: %d", code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code, _ := s.preview(ana, epub, ""); code != http.StatusNotFound {
		t.Errorf("retired: %d", code)
	}
}

func TestPreview_NeverGivesMoreThanTheMostAtOnce(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	var segs []outlined
	for i := 0; i < 20; i++ {
		segs = append(segs, outlined{text: fmt.Sprintf("passagem %d", i), node: 0, locator: fmt.Sprintf(`{"type":"epub","href":"c.xhtml","progression":%v}`, float64(i)/20)})
	}
	s.indexWithOutline(epub, []map[string]any{{"title": "Único", "depth": 0, "chars": 100, "part": "body"}}, segs...)
	_, out := s.preview(ana, epub, "?count=100")
	if len(out.Segments) != previewMax || out.Total != 20 || shown(out.NextFrom) != fmt.Sprint(previewMax) {
		t.Errorf("at most %d: %d segments, next %s", previewMax, len(out.Segments), shown(out.NextFrom))
	}
	_, out = s.preview(ana, epub, "")
	if len(out.Segments) != previewDefault {
		t.Errorf("the default: %d", len(out.Segments))
	}
}
