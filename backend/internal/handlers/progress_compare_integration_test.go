package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func (s *catalogStack) compare(a actor, file int64) (int, PlaceComparison) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/progress/files/%d/compare", file), "")
	var c PlaceComparison
	json.Unmarshal(rec.Body.Bytes(), &c)
	return rec.Code, c
}

// keepApart puts a place a device sent next to the one of the web, as the stale guard does (DEC-191).
func (s *catalogStack) keepApart(user string, file int64, device, locator string, percent float64) {
	s.t.Helper()
	s.exec(`INSERT INTO kosync_pending (user_id, file_id, device, position, locator, locator_version, percent_complete, kosync_progress, kosync_device_id, kosync_device, created_at)
	        VALUES ($1, $2, $3, 'p', $4::jsonb, 1, $5, 'x', 'DEV1', 'KindleBasic3', now() - interval '18 minutes')`, user, file, device, locator, percent)
}

func TestCompare_EachPlaceIsSaidInWordsFromTheTextIndex(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	file := s.primaryFile(dune)
	s.indexWithOutline(file,
		[]map[string]any{{"title": "Livro II: Capítulo 14", "depth": 0, "chars": 100, "part": "body"}, {"title": "Livro II: Capítulo 18", "depth": 0, "chars": 100, "part": "body"}},
		outlined{text: "A caminhada noturna pelas dunas. Paul recebe o nome secreto de sietch Usul.", locator: `{"type":"epub","href":"Text/c14.xhtml","progression":0.1}`, node: 0},
		outlined{text: strings.Repeat("O homem que consegue estar em dois lugares ao mesmo tempo precisa de uma mente que suporte a divisão. ", 6), locator: `{"type":"epub","href":"Text/c18.xhtml","progression":0.1}`, node: 1},
	)
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), `{"locator":{"type":"epub","cfi":"epubcfi(/6/4)","href":"Text/c14.xhtml","progression":0.2},"percent":50,"device":"Web"}`); rec.Code != 200 {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	s.keepApart(idAna, file, "Kindle de Binaco", `{"type":"epub","href":"Text/c18.xhtml","progression":0.2}`, 64)
	for i := 0; i < 3; i++ {
		s.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color)
		        VALUES ($1, $2, $3, 'highlight', 'q', '', '{}', '{"type":"epub","cfi":"epubcfi(/6/4!/4/2)","href":"Text/c14.xhtml","progression":0.2}'::jsonb, 1, 'sepia')`, idAna, dune, file)
	}

	code, c := s.compare(ana, file)
	if code != 200 || c.Web == nil || c.Device == nil {
		t.Fatalf("compare: %d %+v", code, c)
	}
	if c.Web.Percent != 50 || c.Web.Chapter != "Livro II: Capítulo 14" || !strings.HasPrefix(c.Web.Snippet, "A caminhada noturna") || c.Web.Device != "Web" {
		t.Errorf("web = %+v", c.Web)
	}
	if c.Device.Percent != 64 || c.Device.Device != "Kindle de Binaco" || c.Device.Chapter != "Livro II: Capítulo 18" {
		t.Errorf("device = %+v", c.Device)
	}
	if got := len([]rune(c.Device.Snippet)); got == 0 || got > snippetRunes+1 || !strings.HasSuffix(c.Device.Snippet, "…") {
		t.Errorf("a long passage is cut with an ellipsis: %d %q", got, c.Device.Snippet)
	}
	if c.GapPercent != 14 || c.GapPages != 0 || c.Notes != 3 || c.Web.Page != 0 {
		t.Errorf("gap and notes (an EPUB has no pages) = %+v", c)
	}
	// somebody else's notes and places are not in it
	if _, other := s.compare(bob, file); other.Web != nil || other.Device != nil || other.Notes != 0 {
		t.Errorf("bob = %+v", other)
	}
	if code, _ := s.compare(ana, 999999); code != 404 {
		t.Errorf("no such file: %d", code)
	}
}

func TestCompare_APdfHasPagesAndAGapInPages(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Manual", "Alguém", "m.pdf", "pdf")
	file := s.primaryFile(w)
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), `{"locator":{"type":"pdf","page":203},"percent":50,"device":"Web"}`); rec.Code != 200 {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	s.keepApart(idAna, file, "Kindle", `{"type":"pdf","page":261}`, 64)
	_, c := s.compare(ana, file)
	if c.Web == nil || c.Device == nil || c.Web.Page != 204 || c.Device.Page != 262 || c.GapPages != 58 || c.GapPercent != 14 {
		t.Errorf("compare = %+v / %+v / %+v", c, c.Web, c.Device)
	}
	if c.Web.Chapter != "" || c.Web.Snippet != "" {
		t.Errorf("no text index, no chapter or passage made up: %+v", c.Web)
	}
}

func TestCompare_WithNoPendingPlaceThereIsOnlyTheWeb(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	file := s.primaryFile(w)
	s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), `{"locator":{"type":"epub","cfi":"epubcfi(/6/4)","href":"a.xhtml","progression":0.2},"percent":10,"device":"Web"}`)
	_, c := s.compare(ana, file)
	if c.Web == nil || c.Device != nil || c.GapPercent != 0 {
		t.Errorf("compare = %+v", c)
	}
}
