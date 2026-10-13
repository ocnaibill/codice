package handlers

import (
	"fmt"
	"testing"
)

// readByOCR pretends the service read some pages of the first file of the work.
func (s *catalogStack) readByOCR(work int, model, state string, pages ...int) {
	s.t.Helper()
	file := s.primaryFile(work)
	regions := `[{"box":[1,2,3,4],"text":"OLÁ","conf":98}]`
	if state != "done" {
		regions = `[]`
	}
	for _, p := range pages {
		s.exec(`INSERT INTO comic_ocr_pages (file_id, page, source_sha256, state, regions, width, height, model, engine)
			VALUES ($1, $2, NULL, $3, $4::jsonb, 100, 100, $5, 'paddleocr')`, file, p, state, regions, model)
	}
}

func (s *catalogStack) textJobs() map[int]string {
	s.t.Helper()
	rows, err := s.db.Query(`SELECT work_id, payload::text FROM jobs WHERE type = 'extract_text' AND state = 'pending'`)
	if err != nil {
		s.t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var payload string
		rows.Scan(&id, &payload)
		out[id] = payload
	}
	return out
}

func TestComicText_TheDirectionOfASeriesMakesTheTextOfItsReadComicsAgain(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter", "chapter")
	elsewhere := s.addWork("Solto", "Autor", "solto.cbz", "cbz")
	s.readByOCR(ids[0], "mobile", "done", 0, 1)
	s.readByOCR(ids[1], "mobile", "blank", 0)   // read, and nothing was said in it: there is no text to put in order
	s.readByOCR(elsewhere, "mobile", "done", 0) // read, but of no series that changes
	path := fmt.Sprintf("/collections/%d", col)

	if rec := s.do(admin, "PATCH", path, `{"readingDirection":"rtl"}`); rec.Code != 200 {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	jobs := s.textJobs()
	if len(jobs) != 1 || jobs[ids[0]] == "" {
		t.Fatalf("jobs = %v, want only the work of the series that has speech", jobs)
	}
	if got := jobs[ids[0]]; got != `{"force": true}` {
		t.Errorf("payload = %s", got)
	}
	// Saying it again while the job waits is the same job.
	if rec := s.do(admin, "PATCH", path, `{"readingDirection":"ltr"}`); rec.Code != 200 {
		t.Fatalf("again: %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'extract_text' AND state = 'pending'`); n != "1" {
		t.Errorf("jobs after saying it again = %s", n)
	}
}

func TestComicText_ChangingOnlyTheDescriptionOrTheStatusOfASeriesAsksForNothing(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter")
	s.readByOCR(ids[0], "mobile", "done", 0)
	path := fmt.Sprintf("/collections/%d", col)
	if rec := s.do(admin, "PATCH", path, `{"description":"Dark fantasy.","publicationStatus":"ongoing"}`); rec.Code != 200 {
		t.Fatalf("describe: %d %s", rec.Code, rec.Body.String())
	}
	if jobs := s.textJobs(); len(jobs) != 0 {
		t.Errorf("jobs = %v", jobs)
	}
}

func TestComicText_ARetiredWorkIsNotQueued(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter")
	s.readByOCR(ids[0], "mobile", "done", 0)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[0])
	s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", col), `{"readingDirection":"rtl"}`)
	if jobs := s.textJobs(); len(jobs) != 0 {
		t.Errorf("jobs = %v", jobs)
	}
}

func TestComicText_TheKindOfAWorkDecidesTheOrderWhenNothingElseDoes(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Mangá", "Autor", "m.cbz", "cbz")
	other := s.addWork("Outro", "Autor", "o.cbz", "cbz")
	s.readByOCR(duna, "mobile", "done", 0)
	s.readByOCR(other, "mobile", "done", 0)

	// Another field changes: the order of the balloons is not touched.
	if rec := s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"Mangá","author":"Autor","description":"Nova sinopse."}`); rec.Code != 200 {
		t.Fatalf("describe: %d %s", rec.Code, rec.Body.String())
	}
	if jobs := s.textJobs(); len(jobs) != 0 {
		t.Fatalf("jobs after another field = %v", jobs)
	}
	if rec := s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"Mangá","author":"Autor","comic_kind":"manga"}`); rec.Code != 200 {
		t.Fatalf("kind: %d %s", rec.Code, rec.Body.String())
	}
	if jobs := s.textJobs(); len(jobs) != 1 || jobs[duna] == "" {
		t.Errorf("jobs after the kind = %v, want only the work that changed", jobs)
	}
}

func TestComicOCRReread_ReadsAgainWhatAnotherModelReadAndWhatFailed(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Mangá", "Autor", "m.cbz", "cbz")
	s.readByOCR(duna, "mobile", "done", 0, 1)
	s.readByOCR(duna, "mobile", "blank", 2)
	s.readByOCR(duna, "server", "done", 3)
	s.readByOCR(duna, "mobile", "failed", 4)
	s.readByOCR(duna, "server", "failed", 5) // the model that is chosen failed too: after the cause is dealt with, it is read again

	// No model is chosen, though the service is running: there is nothing to read with, and nothing is "another model's".
	s.beat("comic_ocr.worker", `{"model":"server","state":"ready","pages":1}`)
	if got := s.models(); got.ComicOcr.OtherModelPages != 0 {
		t.Fatalf("with no model chosen, pages of another model = %d, want 0", got.ComicOcr.OtherModelPages)
	}
	if rec := s.do(admin, "POST", "/admin/models/comic-ocr/reread", ``); rec.Code != 409 {
		t.Fatalf("no model: %d", rec.Code)
	}
	s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"server"}`)
	if got := s.models(); got.ComicOcr.OtherModelPages != 3 || got.ComicOcr.FailedPages != 2 {
		t.Fatalf("pages of another model = %d (want 3), failed = %d (want 2)", got.ComicOcr.OtherModelPages, got.ComicOcr.FailedPages)
	}

	rec := s.do(admin, "POST", "/admin/models/comic-ocr/reread", ``)
	if rec.Code != 200 {
		t.Fatalf("reread: %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT count(*) FROM comic_ocr_pages`); n != "1" {
		t.Errorf("pages left = %s, want only the one the chosen model read", n)
	}
	if n := s.scalar(`SELECT page FROM comic_ocr_pages`); n != "3" {
		t.Errorf("the page that is left = %s, want 3", n)
	}
	if got := s.models(); got.ComicOcr.OtherModelPages != 0 || got.ComicOcr.FailedPages != 0 {
		t.Errorf("still to read again = %d other, %d failed", got.ComicOcr.OtherModelPages, got.ComicOcr.FailedPages)
	}
	if n := s.scalar(`SELECT details->>'pages' || '/' || (details->>'otherModel') || '/' || (details->>'failed') FROM audit_log WHERE action = 'models.comic_ocr.reread'`); n != "5/3/2" {
		t.Errorf("audit pages/other/failed = %q", n)
	}
}

func TestComicOCRReread_NeedsTheServiceThatReadsRunning(t *testing.T) {
	s := newCatalogStack(t)
	s.beat("comic_ocr.worker", `{"model":"mobile","state":"ready","pages":0}`)
	s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"mobile"}`)
	s.exec(`UPDATE settings SET updated_at = now() - interval '10 minutes' WHERE key = 'comic_ocr.worker'`)
	if rec := s.do(admin, "POST", "/admin/models/comic-ocr/reread", ``); rec.Code != 409 {
		t.Errorf("silent service: %d", rec.Code)
	}
}
