package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type progressBody struct {
	FileID          int64           `json:"fileId"`
	Locator         json.RawMessage `json:"locator"`
	LocatorVersion  *int            `json:"locatorVersion"`
	Position        string          `json:"position"`
	Percent         float64         `json:"percent"`
	Completed       bool            `json:"completed"`
	Revision        int64           `json:"revision"`
	Device          string          `json:"device"`
	UpdatedAt       *time.Time      `json:"updatedAt"`
	ClientUpdatedAt *time.Time      `json:"clientUpdatedAt"`
}

func (s *catalogStack) progress(a actor, method string, file int64, body string) (int, progressBody) {
	s.t.Helper()
	rec := s.do(a, method, fmt.Sprintf("/progress/files/%d", file), body)
	var out progressBody
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (s *catalogStack) bookWithTwoFiles() (work int, epub, pdf int64) {
	s.t.Helper()
	work = s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	epub = s.primaryFile(work)
	pdf = s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, work)), "pdf", "duna.pdf", "managed")
	return
}

func TestFileProgress_SavesAndReturnsTheLocatorInCanonicalForm(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()

	code, st := s.progress(ana, "GET", epub, "")
	if code != 200 || st.Revision != 0 || st.Locator != nil && string(st.Locator) != "null" || st.UpdatedAt != nil {
		t.Fatalf("nothing saved yet: %d %+v", code, st)
	}

	code, st = s.progress(ana, "PUT", epub, `{ "locator": {"progression":0.4, "type":"epub", "cfi":"epubcfi(/6/4!/4)"}, "percent": 33.5, "device":"  Leitor da sala " }`)
	if code != 200 || st.Revision != 1 || st.Percent != 33.5 || st.Device != "Leitor da sala" {
		t.Fatalf("first save: %d %+v", code, st)
	}
	// JSONB does not keep the order of keys, so the content is compared, not the text.
	var got map[string]any
	json.Unmarshal(st.Locator, &got)
	if len(got) != 3 || got["type"] != "epub" || got["cfi"] != "epubcfi(/6/4!/4)" || got["progression"] != 0.4 {
		t.Errorf("the stored form is the canonical one, got %s", st.Locator)
	}
	if st.LocatorVersion == nil || *st.LocatorVersion != 1 || st.Position != "epubcfi(/6/4!/4)" || st.UpdatedAt == nil {
		t.Errorf("version, legacy position and server time: %+v", st)
	}

	// Not sending percent or completed keeps what was saved.
	code, st = s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"ch9.xhtml"}}`)
	if code != 200 || st.Revision != 2 || st.Percent != 33.5 || st.Completed || st.Position != "ch9.xhtml" {
		t.Errorf("second save: %d %+v", code, st)
	}
}

func TestFileProgress_ACompletionCanBeMarkedAndReopened(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	loc := `"locator":{"type":"epub","href":"fim.xhtml"}`

	if _, st := s.progress(ana, "PUT", epub, `{`+loc+`,"percent":250,"completed":true}`); !st.Completed || st.Percent != 100 {
		t.Errorf("finished, with the percent held to 100: %+v", st)
	}
	if _, st := s.progress(ana, "PUT", epub, `{`+loc+`}`); !st.Completed {
		t.Errorf("a write that says nothing about it keeps it finished: %+v", st)
	}
	if _, st := s.progress(ana, "PUT", epub, `{`+loc+`,"completed":false,"percent":-5}`); st.Completed || st.Percent != 0 {
		t.Errorf("reopened: %+v", st)
	}
}

func TestFileProgress_TheSheetKnowsWhichFilesWereStartedEvenWithoutAPercentage(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()

	// An EPUB viewer knows where the reader is but not how far along it is.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"ch2.xhtml"}}`)

	started := func(a actor) map[int64]bool {
		w, _ := s.detail(a, work)
		out := map[int64]bool{}
		for _, e := range w.Editions {
			for _, f := range e.Files {
				out[f.ID] = f.Started
			}
		}
		return out
	}
	if got := started(ana); !got[epub] || got[pdf] {
		t.Errorf("ana started only the epub: %v", got)
	}
	if got := started(bob); got[epub] || got[pdf] {
		t.Errorf("bob started nothing, and must not learn that ana did: %v", got)
	}
}

func TestFileProgress_EachFileKeepsItsOwnPosition(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()

	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"ch3.xhtml"},"percent":30}`)
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":199},"percent":80}`)

	_, e := s.progress(ana, "GET", epub, "")
	_, p := s.progress(ana, "GET", pdf, "")
	if e.Percent != 30 || e.Position != "ch3.xhtml" || p.Percent != 80 || p.Position != "200" {
		t.Errorf("epub %+v\npdf %+v", e, p)
	}
}

func TestFileProgress_TwoUsersNeverSeeEachOthersPosition(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()

	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"segredo.xhtml"},"percent":70,"completed":true,"device":"celular da ana"}`)

	code, st := s.progress(bob, "GET", epub, "")
	if code != 200 || st.Revision != 0 || st.Position != "" || st.Percent != 0 || st.Completed || st.Device != "" || len(st.Locator) > 4 {
		t.Errorf("bob must see an empty position, got %d %+v", code, st)
	}
	// Bob writing does not touch Ana's, and revisions count separately.
	if _, st := s.progress(bob, "PUT", epub, `{"locator":{"type":"epub","href":"a.xhtml"}}`); st.Revision != 1 {
		t.Errorf("bob's first save is his revision 1, got %+v", st)
	}
	if _, st := s.progress(ana, "GET", epub, ""); st.Position != "segredo.xhtml" || st.Revision != 1 || !st.Completed {
		t.Errorf("ana's position changed: %+v", st)
	}
}

func TestFileProgress_ALocatorOfTheWrongKindIsRefusedAndNothingIsStored(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()

	bad := map[string]struct {
		file int64
		body string
	}{
		"an epub locator for a pdf":   {pdf, `{"locator":{"type":"epub","cfi":"x"}}`},
		"a pdf locator for an epub":   {epub, `{"locator":{"type":"pdf","page":3}}`},
		"no locator":                  {epub, `{"percent":10}`},
		"an unknown field":            {epub, `{"locator":{"type":"epub","href":"a","x":1}}`},
		"a page that is not a number": {pdf, `{"locator":{"type":"pdf","page":"3"}}`},
		"not JSON":                    {epub, `nope`},
		"too large":                   {epub, `{"locator":{"type":"epub","href":"a"},"device":"` + strings.Repeat("d", 20000) + `"}`},
	}
	for name, c := range bad {
		if code, _ := s.progress(ana, "PUT", c.file, c.body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if n := s.scalar(`SELECT count(*) FROM reading_progress`); n != "0" {
		t.Errorf("a refused write left %s rows", n)
	}
}

func TestFileProgress_AStaleDeviceDoesNotOverwriteANewerPosition(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	at := func(href string, base string) string {
		return `{"locator":{"type":"epub","href":"` + href + `"}` + base + `}`
	}

	// The phone saves; the tablet had last seen nothing (revision 0) and saves after.
	if code, st := s.progress(ana, "PUT", epub, at("phone.xhtml", `,"baseRevision":0`)); code != 200 || st.Revision != 1 {
		t.Fatalf("first: %d %+v", code, st)
	}
	code, st := s.progress(ana, "PUT", epub, at("tablet.xhtml", `,"baseRevision":0`))
	if code != http.StatusConflict || st.Position != "phone.xhtml" || st.Revision != 1 {
		t.Fatalf("a stale write must be refused with the current state: %d %+v", code, st)
	}
	if got := s.scalar(`SELECT position FROM reading_progress WHERE user_id = $1`, idAna); got != "phone.xhtml" {
		t.Errorf("stored %q", got)
	}
	// Having seen the current revision, the tablet may go on; so may a client that sends none.
	if code, st := s.progress(ana, "PUT", epub, at("tablet.xhtml", `,"baseRevision":1`)); code != 200 || st.Revision != 2 {
		t.Errorf("with the current base: %d %+v", code, st)
	}
	if code, st := s.progress(ana, "PUT", epub, at("last.xhtml", ``)); code != 200 || st.Revision != 3 {
		t.Errorf("without a base the last write wins: %d %+v", code, st)
	}
}

func TestFileProgress_TheClientClockIsKeptApartFromTheServers(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()

	_, st := s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"a"},"clientTime":"2001-02-03T04:05:06Z"}`)
	if st.ClientUpdatedAt == nil || st.ClientUpdatedAt.Year() != 2001 || st.UpdatedAt == nil || time.Since(*st.UpdatedAt) > time.Minute {
		t.Errorf("the device time is information, the server time is when it arrived: %+v", st)
	}
	if code, _ := s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"a"},"clientTime":"ontem"}`); code != http.StatusBadRequest {
		t.Errorf("a time that is not a time: %d", code)
	}
}

func TestFileProgress_ARetiredOrMissingFileIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, _ := s.bookWithTwoFiles()
	body := `{"locator":{"type":"epub","href":"a"}}`

	if code, _ := s.progress(ana, "PUT", 999999, body); code != http.StatusNotFound {
		t.Errorf("unknown file: %d", code)
	}
	if rec := s.do(ana, "GET", "/progress/files/abc", ""); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	for _, m := range []string{"GET", "PUT"} {
		if code, _ := s.progress(ana, m, epub, body); code != http.StatusNotFound {
			t.Errorf("%s on a retired work: %d", m, code)
		}
	}
}

func TestFileProgress_ThePlainTextWriteOfTheFirstReadersReplacesTheLocator(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, _ := s.bookWithTwoFiles()

	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"ch1.xhtml"}}`)
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", work), `{"progress":"epubcfi(/6/8)","fileId":`+fmt.Sprint(epub)+`}`); rec.Code != 200 {
		t.Fatalf("legacy write: %d", rec.Code)
	}
	code, st := s.progress(ana, "GET", epub, "")
	if code != 200 || st.Position != "epubcfi(/6/8)" || st.LocatorVersion != nil || len(st.Locator) > 4 || st.Revision != 2 {
		t.Errorf("the old locator would now point somewhere else: %d %+v", code, st)
	}
}

func TestContinueReading_FollowsTheFileReadLastNotTheBooksPrimaryFile(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles() // the epub is the primary file
	other := s.addWork("Outro", "X", "outro.epub", "epub")

	if got := s.list(ana, "?inProgress=true"); len(got.Data) != 0 {
		t.Fatalf("nothing read yet: %v", ids(got.Data))
	}
	if w, _ := s.detail(ana, work); w.Continue != nil {
		t.Errorf("nothing to continue: %+v", w.Continue)
	}

	// Ana reads only the PDF, the file that is not the primary one.
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":41},"percent":42}`)

	got := s.list(ana, "?inProgress=true")
	if len(got.Data) != 1 || got.Data[0].ID != work {
		t.Fatalf("a work read in a non-primary file is in progress: %v", ids(got.Data))
	}
	c := got.Data[0].Continue
	if c == nil || c.FileID != pdf || c.Format != "pdf" || c.Position != "42" || c.PercentComplete != 42 || c.Completed ||
		!strings.Contains(c.URL, "duna.pdf") {
		t.Errorf("continue = %+v", c)
	}
	// The card itself still speaks for the primary file, as decided.
	if got.Data[0].FileID == nil || *got.Data[0].FileID != epub || got.Data[0].PercentComplete != 0 {
		t.Errorf("the card is the primary file's: %+v", got.Data[0])
	}
	// Reading is personal: Bob has nothing to continue, and the other work is untouched.
	if got := s.list(bob, "?inProgress=true"); len(got.Data) != 0 {
		t.Errorf("bob: %v", ids(got.Data))
	}
	if w, _ := s.detail(bob, work); w.Continue != nil {
		t.Errorf("bob's detail shows ana's reading: %+v", w.Continue)
	}
	if w, _ := s.detail(ana, other); w.Continue != nil {
		t.Errorf("another work: %+v", w.Continue)
	}

	// Then she goes back to the EPUB: that is now the file to continue.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"ch2.xhtml"}}`)
	if c := s.list(ana, "?inProgress=true").Data[0].Continue; c.FileID != epub || c.Format != "epub" {
		t.Errorf("the latest activity wins: %+v", c)
	}

	// Finishing the file read last takes the work out of "in progress", even though the other
	// file was left half way: the person is done with the book, not with a format.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim.xhtml"},"completed":true}`)
	if got := s.list(ana, "?inProgress=true"); len(got.Data) != 0 {
		t.Errorf("finished: %v", ids(got.Data))
	}
	if w, _ := s.detail(ana, work); w.Continue == nil || !w.Continue.Completed {
		t.Errorf("the detail still says which file was last, and that it is finished: %+v", w.Continue)
	}
}

func TestContinueReading_ASourceThatIsGoneIsNotOffered(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"a"}}`)
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":1}}`)
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, pdf)

	// The PDF was read last but is missing from disk: the reader cannot be sent there.
	w, _ := s.detail(ana, work)
	if w.Continue == nil || w.Continue.FileID != epub {
		t.Errorf("continue should fall back to a file that can be opened: %+v", w.Continue)
	}
}

func TestContinueReading_OpeningAFileIsNotBeginningIt(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":3},"percent":10}`)

	// Twenty-five seconds with the EPUB open, without turning a page. That is reading time; it
	// is not a position, so the PDF is still the file to continue.
	rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/reading-heartbeat", work), fmt.Sprintf(`{"seconds":25,"fileId":%d}`, epub))
	if rec.Code != 200 {
		t.Fatalf("heartbeat: %d", rec.Code)
	}
	if got := s.scalar(`SELECT reading_seconds FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub); got != "25" {
		t.Fatalf("the time was counted on the file read: %q", got)
	}
	w, _ := s.detail(ana, work)
	if w.Continue == nil || w.Continue.FileID != pdf {
		t.Errorf("continue moved to a file with no position: %+v", w.Continue)
	}
	for _, e := range w.Editions {
		for _, f := range e.Files {
			if f.ID == epub && f.Started {
				t.Errorf("a file that was only opened is not started")
			}
			if f.ID == pdf && !f.Started {
				t.Errorf("the pdf has a position")
			}
		}
	}
	// And a book with nothing but open time is not "in progress" at all.
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/reading-heartbeat", other), `{"seconds":25}`)
	for _, id := range ids(s.list(ana, "?inProgress=true").Data) {
		if id == other {
			t.Errorf("a book only opened is in progress")
		}
	}
}

func TestCompletion_IsAFactThatOnlyAnExplicitActionUndoes(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	at := func(page int, extra string) string {
		return fmt.Sprintf(`{"locator":{"type":"epub","href":"c%d.xhtml"}%s}`, page, extra)
	}

	s.progress(ana, "PUT", epub, at(9, `,"completed":true,"percent":100`))
	first := s.scalar(`SELECT completed_at::text FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub)
	if first == "" {
		t.Fatal("not completed")
	}

	// Going back and forth, and being told it is finished again, changes nothing about it.
	if _, st := s.progress(ana, "PUT", epub, at(2, ``)); !st.Completed {
		t.Errorf("navigating back must not undo a completion: %+v", st)
	}
	s.progress(ana, "PUT", epub, at(9, `,"completed":true`))
	if got := s.scalar(`SELECT completed_at::text FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub); got != first {
		t.Errorf("the date of the completion is the first one: %s then %s", first, got)
	}
	// Only an explicit false reopens it.
	if _, st := s.progress(ana, "PUT", epub, at(2, `,"completed":false`)); st.Completed {
		t.Errorf("reopened: %+v", st)
	}
}

func TestCompletion_CanBeMarkedAndReopenedWithoutReading(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	done := func(a actor, file int64, body string) (int, progressBody) {
		s.t.Helper()
		rec := s.do(a, "PUT", fmt.Sprintf("/progress/files/%d/completion", file), body)
		var out progressBody
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	// Finished elsewhere: marked without ever opening it.
	code, st := done(ana, epub, `{"completed":true}`)
	if code != 200 || !st.Completed || st.Percent != 100 || st.Revision != 1 {
		t.Fatalf("mark: %d %+v", code, st)
	}
	if got := s.list(ana, "?inProgress=true"); len(got.Data) != 0 {
		t.Errorf("a finished book is not in progress: %v", ids(got.Data))
	}
	if w, _ := s.detail(ana, work); w.Continue == nil || !w.Continue.Completed || w.Continue.FileID != epub {
		t.Errorf("the work knows its last file is finished: %+v", w.Continue)
	}

	// Reopened with nothing else, it is a file nobody has begun.
	code, st = done(ana, epub, `{"completed":false}`)
	if code != 200 || st.Completed || st.Percent != 0 || st.Revision != 2 {
		t.Fatalf("reopen: %d %+v", code, st)
	}
	if w, _ := s.detail(ana, work); w.Continue != nil {
		t.Errorf("nothing to continue after reopening an unread file: %+v", w.Continue)
	}

	// A file that was read to the end keeps its position when reopened, to continue from there.
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":41},"percent":100,"completed":true}`)
	_, st = done(ana, pdf, `{"completed":false}`)
	if st.Completed || st.Percent != 0 || st.Position != "42" {
		t.Errorf("reopen keeps the place: %+v", st)
	}
	if got := s.list(ana, "?inProgress=true"); len(got.Data) != 1 || got.Data[0].Continue.FileID != pdf {
		t.Errorf("reopened, it is in progress again: %+v", got.Data)
	}

	// It is personal, and only for files that can be read.
	if _, st := s.progress(bob, "GET", pdf, ""); st.Revision != 0 || st.Completed {
		t.Errorf("bob's file is untouched: %+v", st)
	}
	if code, _ := done(ana, 999999, `{"completed":true}`); code != http.StatusNotFound {
		t.Errorf("unknown file: %d", code)
	}
	for _, body := range []string{`{}`, `nope`, `{"completed":"yes"}`} {
		if code, _ := done(ana, epub, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, code)
		}
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code, _ := done(ana, epub, `{"completed":true}`); code != http.StatusNotFound {
		t.Errorf("retired work: %d", code)
	}
}

func TestStats_FollowTheFileReadLast(t *testing.T) {
	s := newCatalogStack(t)
	work, _, _ := s.bookWithTwoFiles() // epub primary, plus a pdf
	comic := s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, work)), "cbz", "duna.cbz", "managed")
	stats := func(a actor) DashboardStats {
		var st DashboardStats
		json.Unmarshal(s.do(a, "GET", "/stats", "").Body.Bytes(), &st)
		return st
	}

	// Read only in the comic file, which is not the primary one: it is a comic in progress.
	s.progress(ana, "PUT", comic, `{"locator":{"type":"image","index":3},"percent":20}`)
	got := stats(ana)
	if got.InProgressCount != 1 || got.InProgressBreakdown.Mangas != 1 || got.InProgressBreakdown.Livros != 0 || got.CompletedThisMonth != 0 {
		t.Errorf("in progress: %+v", got)
	}
	if other := stats(bob); other.InProgressCount != 0 || other.CompletedThisMonth != 0 {
		t.Errorf("bob: %+v", other)
	}

	// Finished this month: no longer in progress, and counted where it was read.
	s.progress(ana, "PUT", comic, `{"locator":{"type":"image","index":9},"percent":100,"completed":true}`)
	got = stats(ana)
	if got.InProgressCount != 0 || got.CompletedThisMonth != 1 || got.CompletedBreakdown.Mangas != 1 {
		t.Errorf("completed: %+v", got)
	}

	// Finished last month: it is not in this month's count.
	s.exec(`UPDATE reading_progress SET completed_at = now() - interval '40 days' WHERE user_id = $1 AND file_id = $2`, idAna, comic)
	if got := stats(ana); got.CompletedThisMonth != 0 || got.InProgressCount != 0 {
		t.Errorf("an old completion: %+v", got)
	}
}
