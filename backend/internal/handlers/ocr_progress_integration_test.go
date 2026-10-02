package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// How far reading the pages of a scan has got (#24): in the list of the administration, in the sheet of the work, and the
// request to try again the pages that failed.

const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// scanned makes a work whose PDF has the given pages (from 1) without a text layer.
func (s *catalogStack) scanned(title string, pages ...int) (work int, file string) {
	s.t.Helper()
	work = s.addWork(title, "Ana", strings.ToLower(title)+".pdf", "pdf")
	file = s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, work))
	// The first scan of a test has shaA; one made after it gets a hash of its own (the column is unique).
	s.exec(`UPDATE files SET sha256 = CASE WHEN EXISTS (SELECT 1 FROM files WHERE sha256 = $2 AND id <> $1)
		THEN md5($1::text) || md5($1::text) ELSE $2 END WHERE id = $1`, file, shaA)
	list := make([]string, len(pages))
	for i, p := range pages {
		list[i] = fmt.Sprint(p)
	}
	s.exec(fmt.Sprintf(`INSERT INTO text_layers (file_id, page_count, pages_without_text, needs_ocr) VALUES ($1, 10, '{%s}', TRUE)`, strings.Join(list, ",")), file)
	return
}

func (s *catalogStack) readPage(file string, page int, state, sha, language string) {
	s.t.Helper()
	text := ""
	if state == "done" {
		text = "texto lido"
	}
	s.exec(`INSERT INTO ocr_pages (file_id, page, source_sha256, state, text, engine, language) VALUES ($1, $2, $3, $4, $5, 'tesseract', $6)`,
		file, page, sha, state, text, language)
}

type ocrItem struct {
	WorkID           int      `json:"workId"`
	Title            string   `json:"title"`
	PageCount        int      `json:"pageCount"`
	PagesWithoutText []int    `json:"pagesWithoutText"`
	Read             int      `json:"read"`
	Failed           int      `json:"failed"`
	State            string   `json:"state"`
	Languages        []string `json:"languages"`
	Language         string   `json:"language"`
	LanguageSource   string   `json:"languageSource"`
}

func (s *catalogStack) ocrList() []ocrItem {
	s.t.Helper()
	var out struct{ Data []ocrItem }
	rec := s.do(admin, "GET", "/admin/ocr", "")
	if rec.Code != 200 {
		s.t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Data
}

func TestOCRList_SaysHowFarEachScanHasGot(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.scanned("Escaneado", 1, 2, 3, 4, 5)
	s.readPage(file, 0, "done", shaA, "por+eng")
	s.readPage(file, 1, "blank", shaA, "por+eng")
	s.readPage(file, 2, "failed", shaA, "por+eng")
	// Not counted: a page of another version of the file, and one that is not among those without text.
	s.readPage(file, 3, "done", shaB, "por+eng")
	s.readPage(file, 8, "done", shaA, "spa")
	s.readPage(file, 4, "blank", shaA, "") // a page with no language recorded says nothing about the languages

	items := s.ocrList()
	if len(items) != 1 {
		t.Fatalf("items: %+v", items)
	}
	it := items[0]
	if it.WorkID != work || it.PageCount != 10 || len(it.PagesWithoutText) != 5 || it.Read != 3 || it.Failed != 1 || it.State != "" {
		t.Errorf("progress: %+v", it)
	}
	if len(it.Languages) != 1 || it.Languages[0] != "por+eng" {
		t.Errorf("the languages it was read in are those of the pages that count: %v", it.Languages)
	}
}

func TestOCRList_ANeverReadScanHasNothingReadAndAnEmptyListOfLanguages(t *testing.T) {
	s := newCatalogStack(t)
	s.scanned("Escaneado", 1, 2)
	it := s.ocrList()[0]
	if it.Read != 0 || it.Failed != 0 || it.State != "" || it.Languages == nil || len(it.Languages) != 0 {
		t.Errorf("%+v", it)
	}
}

func TestOCRList_SaysWhetherTheWorkIsWaitingOrBeingRead(t *testing.T) {
	s := newCatalogStack(t)
	work, _ := s.scanned("Escaneado", 1)
	for state, want := range map[string]string{"pending": "queued", "running": "reading"} {
		s.exec(`DELETE FROM jobs`)
		s.exec(`INSERT INTO jobs (type, work_id, state) VALUES ('ocr', $1, $2)`, work, state)
		if got := s.ocrList()[0].State; got != want {
			t.Errorf("job %s: state %q, want %q", state, got, want)
		}
	}
	// Another kind of job of the work, or an OCR job that ended, says nothing about the reading.
	s.exec(`DELETE FROM jobs`)
	s.exec(`INSERT INTO jobs (type, work_id, state) VALUES ('extract_text', $1, 'running'), ('ocr', $1, 'succeeded'), ('ocr', $1, 'failed')`, work)
	if got := s.ocrList()[0].State; got != "" {
		t.Errorf("state %q", got)
	}
}

func TestOCRList_NotTheWorksThatAreRetired(t *testing.T) {
	s := newCatalogStack(t)
	work, _ := s.scanned("Aposentado", 1)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if items := s.ocrList(); len(items) != 0 {
		t.Errorf("%+v", items)
	}
}

func TestWorkDetail_TheFileSaysHowFarItsScanHasGot(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.scanned("Escaneado", 1, 2, 3)
	s.readPage(file, 0, "done", shaA, "por")
	s.readPage(file, 1, "failed", shaA, "por")
	s.readPage(file, 2, "done", shaB, "por") // of another version of the file
	// The first job of the work in line is of another kind: it says nothing about reading the pages.
	s.exec(`INSERT INTO jobs (type, work_id, state) VALUES ('extract_text', $1, 'pending')`, work)
	s.exec(`INSERT INTO jobs (type, work_id, state) VALUES ('ocr', $1, 'running')`, work)

	w, _ := s.detail(ana, work)
	got := w.Editions[0].Files[0].OCR
	if got == nil || got.Pages != 3 || got.Read != 1 || got.Failed != 1 || got.State != "reading" {
		t.Fatalf("ocr: %+v", got)
	}
	if body := s.do(ana, "GET", fmt.Sprintf("/works/%d", work), "").Body.String(); !strings.Contains(body, `"ocr":{"pages":3,"read":1,"failed":1,"state":"reading"}`) {
		t.Errorf("the JSON the sheet reads: %.400s", body)
	}
}

func TestWorkDetail_AFileThatIsNotAScanHasNoOCR(t *testing.T) {
	s := newCatalogStack(t)
	digital := s.addWork("Digital", "Ana", "d.pdf", "pdf")
	file := s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, digital))
	s.exec(`INSERT INTO text_layers (file_id, page_count, pages_without_text, needs_ocr) VALUES ($1, 5, '{}', FALSE)`, file)
	plain := s.addWork("Livro", "Ana", "l.epub", "epub")
	for _, work := range []int{digital, plain} {
		if body := s.do(ana, "GET", fmt.Sprintf("/works/%d", work), "").Body.String(); strings.Contains(body, `"ocr"`) {
			t.Errorf("work %d: %.300s", work, body)
		}
	}
}

func TestOCRRetry_AskingForThePagesThatFailedToBeTriedAgain(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.scanned("Escaneado", 1, 2, 3)
	retry := func() int { return s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/ocr/retry", work), "").Code }
	reason := func() string {
		return s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/ocr/retry", work), "").Body.String()
	}

	// With OCR off, a page that failed is not a reason to queue anything: the engine would do nothing with the job.
	s.readPage(file, 1, "failed", shaA, "por")
	if got := retry(); got != 409 || !strings.Contains(reason(), "desligado") || s.scalar(`SELECT count(*) FROM jobs`) != "0" {
		t.Errorf("OCR is off: %d %s", got, reason())
	}
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled": true}')`)
	s.exec(`DELETE FROM ocr_pages`)
	s.readPage(file, 0, "done", shaA, "por")
	if got := retry(); got != 409 || !strings.Contains(reason(), "falhou") {
		t.Errorf("nothing failed: %d %s", got, reason())
	}
	s.readPage(file, 1, "failed", shaA, "por")
	if got := retry(); got != 202 {
		t.Fatalf("a page failed: %d, want 202", got)
	}
	if got := s.scalar(`SELECT type || ':' || priority || ':' || payload::text || ':' || state FROM jobs`); got != `ocr:5:{"retry_failed": true}:pending` {
		t.Errorf("the job: %s", got)
	}
	if got := retry(); got != 200 || s.scalar(`SELECT count(*) FROM jobs`) != "1" {
		t.Errorf("asking twice while it waits is one request: %d", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'ocr.retry'`); got != "1" {
		t.Errorf("audit entries: %s", got)
	}
}

func TestOCRRetry_OnlyForWorksThatAreThere(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled": true}')`)
	work, file := s.scanned("Escaneado", 1)
	s.readPage(file, 0, "failed", shaA, "por")
	if got := s.do(admin, "POST", "/admin/works/99999/ocr/retry", "").Code; got != 404 {
		t.Errorf("a work that does not exist: %d", got)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if got := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/ocr/retry", work), "").Code; got != 404 {
		t.Errorf("a retired work: %d", got)
	}
	if s.scalar(`SELECT count(*) FROM jobs`) != "0" {
		t.Error("a job was queued for a work that is not there")
	}
}

func TestOCRRetry_AFailedPageOfAnotherVersionOfTheFileDoesNotCount(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled": true}')`)
	work, file := s.scanned("Escaneado", 1)
	s.readPage(file, 0, "failed", shaB, "por")
	if got := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/ocr/retry", work), "").Code; got != 409 {
		t.Errorf("%d", got)
	}
}
