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
