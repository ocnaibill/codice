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

func TestContinueReading_FollowsTheVersionThatCountsNotThePrimaryFile(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles() // the epub is the primary file
	other := s.addWork("Outro", "X", "outro.epub", "epub")

	if got := s.list(ana, "?inProgress=true"); len(got.Data) != 0 {
		t.Fatalf("nothing read yet: %v", ids(got.Data))
	}
	if w, _ := s.detail(ana, work); w.Continue != nil || w.InProgress || w.Finished {
		t.Errorf("nothing to continue: %+v", w)
	}

	// Ana reads only the PDF, the file that is not the primary one.
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":41},"percent":42}`)

	got := s.list(ana, "?inProgress=true")
	if len(got.Data) != 1 || got.Data[0].ID != work || !got.Data[0].InProgress {
		t.Fatalf("a work read in a non-primary file is in progress: %v", ids(got.Data))
	}
	c := got.Data[0].Continue
	if c == nil || c.FileID != pdf || c.Format != "pdf" || c.Position != "42" || c.PercentComplete != 42 || c.Completed ||
		!strings.Contains(c.URL, "duna.pdf") {
		t.Errorf("continue = %+v", c)
	}
	// The card shows what the version that counts says (DEC-079), whichever file is the primary.
	if w := got.Data[0]; w.PercentComplete != 42 || w.ReadingProgress != "42" || w.Completed {
		t.Errorf("the card is the counting version's: %+v", w)
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

	// Then she goes back to the EPUB: that is now the version that counts.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"ch2.xhtml"},"percent":10}`)
	if w := s.list(ana, "?inProgress=true").Data[0]; w.Continue.FileID != epub || w.Continue.Format != "epub" || w.PercentComplete != 10 {
		t.Errorf("the latest one opened wins: %+v", w)
	}
}

func TestVersion_OnlyOneThatWasBegunCountsAndOpeningSetsTheOrder(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	comic := s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, work)), "cbz", "duna.cbz", "managed")
	opened := func(a actor, file int64) int {
		return s.do(a, "POST", fmt.Sprintf("/progress/files/%d/opened", file), "").Code
	}
	counting := func() int64 {
		w, _ := s.detail(ana, work)
		if w.Continue == nil {
			return 0
		}
		return w.Continue.FileID
	}

	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"a"},"percent":30}`)
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":5},"percent":60}`)
	if counting() != pdf {
		t.Fatalf("the PDF was moved last: %d", counting())
	}

	// Reopening the EPUB, without moving, makes it the latest one opened.
	before := s.scalar(`SELECT revision FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub)
	if code := opened(ana, epub); code != http.StatusNoContent {
		t.Fatalf("opened: %d", code)
	}
	if counting() != epub {
		t.Errorf("the latest opened, among those begun, counts: %d", counting())
	}
	if after := s.scalar(`SELECT revision FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub); after != before {
		t.Errorf("an opening is not a write of the position: revision %s then %s", before, after)
	}

	// A file that was only opened has not been begun: it does not count, and it is not "started".
	if code := opened(ana, comic); code != http.StatusNoContent {
		t.Fatalf("opened: %d", code)
	}
	if counting() != epub {
		t.Errorf("opening is not beginning: %d", counting())
	}
	w, _ := s.detail(ana, work)
	for _, e := range w.Editions {
		for _, f := range e.Files {
			if f.ID == comic && f.Started {
				t.Errorf("the comic was only opened")
			}
		}
	}
	// Once she moves in it, it counts, being the latest.
	s.progress(ana, "PUT", comic, `{"locator":{"type":"image","index":2},"percent":20}`)
	if counting() != comic {
		t.Errorf("begun, and the latest: %d", counting())
	}

	// It is personal, and only for files that can be read.
	if got := s.list(bob, "?inProgress=true"); len(got.Data) != 0 {
		t.Errorf("bob's opening is his: %v", ids(got.Data))
	}
	if code := opened(bob, epub); code != http.StatusNoContent {
		t.Errorf("bob opens: %d", code)
	}
	if counting() != comic {
		t.Errorf("bob's opening moved ana's version: %d", counting())
	}
	if code := opened(ana, 999999); code != http.StatusNotFound {
		t.Errorf("unknown file: %d", code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code := opened(ana, epub); code != http.StatusNotFound {
		t.Errorf("retired work: %d", code)
	}
}

func TestVersion_FinishingOneLeavesTheOtherInContinueUntilTheWholeWorkIsFinished(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	finish := func(file int64, page string) {
		s.progress(ana, "PUT", file, page+`,"completed":true,"percent":100}`)
	}
	state := func() (Work, bool) {
		w, _ := s.detail(ana, work)
		return w, len(s.list(ana, "?inProgress=true").Data) == 1
	}

	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"a"},"percent":30}`)
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":5},"percent":60}`)
	if w, _ := state(); w.Continue.FileID != pdf {
		t.Fatalf("setup: %+v", w.Continue)
	}

	// The PDF is finished: it leaves Continue, and the EPUB, still in progress, is what is left.
	finish(pdf, `{"locator":{"type":"pdf","page":9}`)
	w, listed := state()
	if !listed || !w.InProgress || w.Continue.FileID != epub || w.PercentComplete != 30 || w.Completed {
		t.Errorf("the other version stays: listed=%v %+v continue=%+v", listed, w, w.Continue)
	}

	// The person says the whole work is finished: nothing of it is in Continue, and nothing is
	// said to have been read to the end that was not.
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", work), `{"finished":true}`); rec.Code != 200 {
		t.Fatalf("finished: %d", rec.Code)
	}
	w, listed = state()
	if listed || w.InProgress || !w.Finished {
		t.Errorf("a finished work is not in progress: listed=%v %+v", listed, w)
	}
	if _, st := s.progress(ana, "GET", epub, ""); st.Completed {
		t.Errorf("the EPUB was not read to the end, and must not become finished: %+v", st)
	}
	if got := s.scalar(`SELECT count(*) FROM reading_completions WHERE user_id = $1 AND work_id = $2`, idAna, work); got != "1" {
		t.Errorf("only the PDF was finished: %s completions", got)
	}
	// A finished file that keeps saving its last page does not undo the mark ...
	finish(pdf, `{"locator":{"type":"pdf","page":9}`)
	if w, _ := state(); !w.Finished {
		t.Errorf("saving a finished file's last page is not reading on")
	}
	// ... reading on in the version that was not finished does.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"b"},"percent":35}`)
	if w, listed := state(); w.Finished || !listed {
		t.Errorf("reading on takes the mark off: listed=%v %+v", listed, w)
	}

	// Once both are finished, nothing is left to continue, and no mark is needed.
	finish(epub, `{"locator":{"type":"epub","href":"fim"}`)
	w, listed = state()
	if listed || w.InProgress || w.Continue == nil || !w.Continue.Completed {
		t.Errorf("everything finished: listed=%v %+v continue=%+v", listed, w, w.Continue)
	}

	// The mark can be taken off by hand, is personal, and needs a work that can be read.
	s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", work), `{"finished":true}`)
	if w, _ := s.detail(bob, work); w.Finished {
		t.Errorf("bob sees ana's mark")
	}
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", work), `{"finished":false}`); rec.Code != 200 {
		t.Errorf("unmark: %d", rec.Code)
	}
	if w, _ := s.detail(ana, work); w.Finished {
		t.Errorf("unmarked")
	}
	for _, body := range []string{`{}`, `nope`, `{"finished":"yes"}`} {
		if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", work), body); rec.Code != 400 {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	if rec := s.do(ana, "PUT", "/progress/works/999999/finished", `{"finished":true}`); rec.Code != 404 {
		t.Errorf("unknown work: %d", rec.Code)
	}
}

func TestCompletions_AreAHistoryByFormatAndRereadingCountsAgain(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	summary := func(a actor) CompletionSummary {
		w, _ := s.detail(a, work)
		if w.Completions == nil {
			t.Fatal("the detail must say how many times")
		}
		return *w.Completions
	}
	done := func(file int64, body string) progressBody {
		rec := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", file), body)
		var out progressBody
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 {
			t.Fatalf("completion %s: %d %s", body, rec.Code, rec.Body)
		}
		return out
	}

	if got := summary(ana); got.Total != 0 || len(got.ByFormat) != 0 {
		t.Fatalf("never finished: %+v", got)
	}
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim"},"completed":true,"percent":100}`)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim"},"completed":true}`) // saving again is not another time
	if got := summary(ana); got.Total != 1 || got.ByFormat["epub"] != 1 {
		t.Errorf("once, in EPUB: %+v", got)
	}
	done(pdf, `{"completed":true}`) // finished elsewhere, marked by hand
	if got := summary(ana); got.Total != 2 || got.ByFormat["epub"] != 1 || got.ByFormat["pdf"] != 1 {
		t.Errorf("once in EPUB, once in PDF: %+v", got)
	}

	// Reopened to be read again: the history stays, and finishing again is one more time.
	st := done(epub, `{"completed":false,"restart":true}`)
	if st.Completed || st.Percent != 0 || st.Position != "" || len(st.Locator) > 4 {
		t.Errorf("a restart is a file nobody has begun: %+v", st)
	}
	if got := summary(ana); got.Total != 2 {
		t.Errorf("reopening does not erase what was finished: %+v", got)
	}
	if w, _ := s.detail(ana, work); w.Continue == nil || w.Continue.FileID != pdf {
		t.Errorf("the restarted file has not been begun again: %+v", w.Continue)
	}
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1"},"percent":5}`)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim"},"completed":true,"percent":100}`)
	if got := summary(ana); got.Total != 3 || got.ByFormat["epub"] != 2 || got.ByFormat["pdf"] != 1 {
		t.Errorf("finished the EPUB twice: %+v", got)
	}

	// restart only goes with reopening; and the history is personal.
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", epub), `{"completed":true,"restart":true}`); rec.Code != 400 {
		t.Errorf("restart with completed: %d", rec.Code)
	}
	if got := summary(bob); got.Total != 0 {
		t.Errorf("bob's history: %+v", got)
	}

	// The format is kept with the event, so it survives the file.
	s.exec(`DELETE FROM files WHERE id = $1`, pdf)
	if got := summary(ana); got.Total != 3 || got.ByFormat["pdf"] != 1 {
		t.Errorf("history after the file is gone: %+v", got)
	}
}

func TestList_CarriesHowManyFilesThereAreToChooseFrom(t *testing.T) {
	s := newCatalogStack(t)
	work, _, pdf := s.bookWithTwoFiles()
	single := s.addWork("Um só", "X", "um.epub", "epub")

	counts := func() map[int]int {
		out := map[int]int{}
		for _, w := range s.list(ana, "").Data {
			out[w.ID] = w.FileCount
		}
		return out
	}
	if got := counts(); got[work] != 2 || got[single] != 1 {
		t.Errorf("file counts: %v", got)
	}
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, pdf)
	if got := counts(); got[work] != 1 {
		t.Errorf("a file that is gone is not a choice: %v", got)
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
	s.exec(`UPDATE reading_completions SET completed_at = now() - interval '40 days' WHERE user_id = $1 AND file_id = $2`, idAna, comic)
	if got := stats(ana); got.CompletedThisMonth != 0 || got.InProgressCount != 0 {
		t.Errorf("an old completion: %+v", got)
	}
}

func TestStats_AWorkFinishedInTwoFormatsThisMonthCountsOnce(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	stats := func() DashboardStats {
		var st DashboardStats
		json.Unmarshal(s.do(ana, "GET", "/stats", "").Body.Bytes(), &st)
		return st
	}
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim"},"completed":true,"percent":100}`)
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":9},"completed":true,"percent":100}`)
	if got := stats(); got.CompletedThisMonth != 1 {
		t.Errorf("one book, finished in two formats: %+v", got)
	}
	s.progress(ana, "PUT", s.primaryFile(other), `{"locator":{"type":"epub","href":"fim"},"completed":true,"percent":100}`)
	if got := stats(); got.CompletedThisMonth != 2 || got.CompletedBreakdown.Livros != 2 {
		t.Errorf("two books: %+v", got)
	}
}

func TestVersion_ReopeningAFileTakesTheFinishedMarkOffTheWork(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.progress(ana, "PUT", pdf, `{"locator":{"type":"pdf","page":3},"percent":30}`)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim"},"completed":true,"percent":100}`)
	finished := func() bool {
		w, _ := s.detail(ana, work)
		return w.Finished
	}
	for _, body := range []string{`{"completed":false}`, `{"completed":false,"restart":true}`} {
		s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", work), `{"finished":true}`)
		if !finished() {
			t.Fatal("setup: marked")
		}
		s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", epub), body)
		if finished() {
			t.Errorf("reopening (%s) is going back to the book: the mark must go", body)
		}
		s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"fim"},"completed":true}`)
	}
	// Marking a file finished does not take it off: that is the opposite of going back.
	s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", work), `{"finished":true}`)
	s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", pdf), `{"completed":true}`)
	if !finished() {
		t.Errorf("finishing another version is not reading on")
	}
}
