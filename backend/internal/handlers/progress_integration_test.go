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
	rec := s.do(a, method, fmt.Sprintf("/files/%d/progress", file), body)
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
	if rec := s.do(ana, "GET", "/files/abc/progress", ""); rec.Code != http.StatusNotFound {
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
