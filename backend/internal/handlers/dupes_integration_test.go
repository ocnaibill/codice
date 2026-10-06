package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestDuplicatesAPI_ReviewDismissAndLink(t *testing.T) {
	s := newCatalogStack(t)
	epub := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	pdf := s.addWork("Duna", "Frank Herbert", "b.pdf", "pdf")
	solo := s.addWork("Neuromancer", "William Gibson", "c.epub", "epub")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", pdf), `{"quote":"da Ana"}`)

	// Comparing is a job (it looks at every pair); run its work directly here.
	if rec := s.do(admin, "POST", "/admin/duplicates/scan", ""); rec.Code != 202 {
		t.Fatalf("scan: %d", rec.Code)
	}
	if code := s.do(admin, "POST", "/admin/duplicates/scan", "").Code; code != 202 || s.scalar(`SELECT count(*) FROM jobs WHERE type = 'dedupe'`) != "1" {
		t.Errorf("asking twice must reuse the waiting job")
	}
	s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`, epub, pdf)

	var list struct {
		Data []struct {
			ID     int64
			Reason string
			A, B   struct {
				ID      int
				Formats []string
			}
		}
	}
	json.Unmarshal(s.do(admin, "GET", "/admin/duplicates", "").Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].A.ID != epub || list.Data[0].B.Formats[0] != "pdf" {
		t.Fatalf("list = %+v", list)
	}
	id := list.Data[0].ID
	path := func(verb string) string { return fmt.Sprintf("/admin/duplicates/%d/%s", id, verb) }

	// Linking is irreversible: it needs the work to keep and an explicit confirmation.
	if code := s.do(admin, "POST", path("link"), `{"keep":`+fmt.Sprint(epub)+`}`).Code; code != 400 {
		t.Errorf("without confirm: %d, want 400", code)
	}
	if code := s.do(admin, "POST", path("link"), fmt.Sprintf(`{"keep":%d,"confirm":true}`, solo)).Code; code != 400 {
		t.Errorf("keeping a work outside the pair: %d, want 400", code)
	}
	if s.scalar(`SELECT count(*) FROM works`) != "3" {
		t.Fatal("a refused link changed something")
	}
	if code := s.do(admin, "POST", "/admin/duplicates/999/dismiss", "").Code; code != 404 {
		t.Errorf("unknown: %d", code)
	}

	if code := s.do(admin, "POST", path("link"), fmt.Sprintf(`{"keep":%d,"confirm":true}`, epub)).Code; code != 204 {
		t.Fatalf("link: %d", code)
	}
	w, _ := s.detail(ana, epub)
	if len(w.Editions) != 2 {
		t.Errorf("editions after linking = %d", len(w.Editions))
	}
	var notes struct{ Data []Note }
	json.Unmarshal(s.do(ana, "GET", "/notes", "").Body.Bytes(), &notes)
	if len(notes.Data) != 1 || notes.Data[0].WorkID == nil || *notes.Data[0].WorkID != epub {
		t.Errorf("the note did not follow: %+v", notes.Data)
	}
	if _, code := s.detail(ana, pdf); code != 404 {
		t.Errorf("the absorbed work is still there: %d", code)
	}
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'duplicate.%'`); got != "duplicate.link" {
		t.Errorf("audit = %q", got)
	}

	// Dismissing works on another pair.
	s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'isbn')`, epub, solo)
	var next int64
	s.db.QueryRow(`SELECT id FROM duplicate_candidates WHERE state = 'pending'`).Scan(&next)
	if code := s.do(admin, "POST", fmt.Sprintf("/admin/duplicates/%d/dismiss", next), "").Code; code != 204 {
		t.Errorf("dismiss: %d", code)
	}
	json.Unmarshal(s.do(admin, "GET", "/admin/duplicates", "").Body.Bytes(), &list)
	if len(list.Data) != 0 {
		t.Errorf("a dismissed pair is still listed")
	}
}

func TestDuplicatesAPI_ListsInPagesWithTheTotal(t *testing.T) {
	s := newCatalogStack(t)
	for i := 0; i < 5; i++ {
		a := s.addWork(fmt.Sprintf("Livro %d", i), "Ana", fmt.Sprintf("a%d.epub", i), "epub")
		b := s.addWork(fmt.Sprintf("Livro %d", i), "Ana", fmt.Sprintf("b%d.pdf", i), "pdf")
		s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`, a, b)
	}
	type page struct {
		Data  []struct{ ID int64 }
		Total int
		More  bool
	}
	get := func(query string) (page, int) {
		var p page
		rec := s.do(admin, "GET", "/admin/duplicates"+query, "")
		json.Unmarshal(rec.Body.Bytes(), &p)
		return p, rec.Code
	}

	first, code := get("?limit=2")
	if code != 200 || len(first.Data) != 2 || first.Total != 5 || !first.More {
		t.Fatalf("first = %d %+v", code, first)
	}
	second, _ := get(fmt.Sprintf("?limit=2&after=%d", first.Data[1].ID))
	if len(second.Data) != 2 || second.Data[0].ID <= first.Data[1].ID || !second.More {
		t.Fatalf("second = %+v", second)
	}
	last, _ := get(fmt.Sprintf("?limit=2&after=%d", second.Data[1].ID))
	if len(last.Data) != 1 || last.More || last.Total != 5 {
		t.Fatalf("last = %+v", last)
	}

	// Without a limit the answer is a page, not the whole list; nonsense falls back to the defaults.
	for _, query := range []string{"", "?limit=0", "?limit=abc&after=-4", "?limit=-1"} {
		if p, code := get(query); code != 200 || len(p.Data) != 5 || p.More {
			t.Errorf("%q = %d %+v", query, code, p)
		}
	}
	// The page has a ceiling.
	a := s.addWork("Base", "Ana", "base.epub", "epub")
	for i := 0; i < duplicatesMaxPage; i++ {
		s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'content')`,
			a, s.addWork(fmt.Sprintf("Outro %d", i), "Ana", fmt.Sprintf("o%d.pdf", i), "pdf"))
	}
	if p, _ := get("?limit=100000"); len(p.Data) != duplicatesMaxPage || !p.More || p.Total != 5+duplicatesMaxPage {
		t.Errorf("limit is not capped: %d rows, more=%v, total=%d", len(p.Data), p.More, p.Total)
	}
	if p, _ := get(""); len(p.Data) != duplicatesPage || !p.More {
		t.Errorf("default page: %d rows, more=%v", len(p.Data), p.More)
	}
}

func TestOCRListing_ReportsPagesWithoutText(t *testing.T) {
	s := newCatalogStack(t)
	scanned := s.addWork("Escaneado", "Ana", "e.pdf", "pdf")
	digital := s.addWork("Digital", "Ana", "d.pdf", "pdf")
	fileOf := func(work int) string {
		return s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, work))
	}
	s.exec(`INSERT INTO text_layers (file_id, page_count, pages_without_text, needs_ocr) VALUES ($1, 5, '{2,3}', TRUE)`, fileOf(scanned))
	s.exec(`INSERT INTO text_layers (file_id, page_count, pages_without_text, needs_ocr) VALUES ($1, 5, '{}', FALSE)`, fileOf(digital))

	var list struct {
		Data []struct {
			WorkID           int
			PagesWithoutText []int
		}
	}
	json.Unmarshal(s.do(admin, "GET", "/admin/ocr", "").Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].WorkID != scanned || len(list.Data[0].PagesWithoutText) != 2 {
		t.Fatalf("listing = %+v", list)
	}

	w, _ := s.detail(ana, scanned)
	f := w.Editions[0].Files[0]
	if !f.NeedsOCR || len(f.PagesWithoutText) != 2 || f.PagesWithoutText[0] != 2 {
		t.Errorf("work detail file = %+v", f)
	}
	if d, _ := s.detail(ana, digital); d.Editions[0].Files[0].NeedsOCR {
		t.Errorf("a PDF with a text layer is flagged")
	}
}

// Linking used to delete the absorbed work, and what hung from it by cascade went with it: the history
// of what people finished, their "whole work finished" mark, the record of equivalent positions they
// accepted and its authors. The files and their own progress survived, which hid it.
func TestDuplicatesAPI_LinkKeepsWhatThePeopleHadDoneWithTheAbsorbedWork(t *testing.T) {
	s := newCatalogStack(t)
	keep := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	absorbed := s.addWork("Dune", "Herbert, Frank, author", "b.pdf", "pdf")
	pdf := s.primaryFile(absorbed)
	if code := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", pdf), `{"completed":true}`).Code; code != 200 {
		t.Fatalf("completion: %d", code)
	}
	if code := s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", absorbed), `{"finished":true}`).Code; code != 200 {
		t.Fatalf("finished: %d", code)
	}
	s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`, keep, absorbed)
	var id int64
	s.db.QueryRow(`SELECT id FROM duplicate_candidates`).Scan(&id)

	if code := s.do(admin, "POST", fmt.Sprintf("/admin/duplicates/%d/link", id), fmt.Sprintf(`{"keep":%d,"confirm":true}`, keep)).Code; code != 204 {
		t.Fatalf("link: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM reading_completions WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("the history of what was finished: %s completions on the kept work, want 1", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_reading_state WHERE work_id = $1 AND finished_at IS NOT NULL`, keep); got != "1" {
		t.Errorf("the whole-work-finished mark was lost: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_contributors WHERE work_id = $1`, keep); got != "2" {
		t.Errorf("the authors of both works: %s, want 2", got)
	}
}
