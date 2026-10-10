package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) refreshStatus(work int) string {
	s.t.Helper()
	return s.do(admin, "GET", fmt.Sprintf("/admin/works/%d/metadata-refresh", work), "").Body.String()
}

func TestMetadataRefresh_IsAStaffRequestForOneJobAndNeedsAProviderThatIsOn(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("A nuvem", "Neal Shusterman", "nuvem.epub", "epub")
	s.exec(`DELETE FROM jobs`)
	url := fmt.Sprintf("/admin/works/%d/metadata-refresh", work)

	// Nothing would leave the instance: nothing is queued, and the page is told why.
	rec := s.do(admin, "POST", url, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"providersOff":true`) || !strings.Contains(rec.Body.String(), `"queued":false`) {
		t.Fatalf("no provider on: %d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'match_metadata'`); got != "0" {
		t.Errorf("a job was queued with every provider off: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'metadata.refresh'`); got != "0" {
		t.Errorf("a request that did nothing was audited")
	}

	s.setProvider("openlibrary", `{"enabled":true}`)
	rec = s.do(admin, "POST", url, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("queued: %d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT type || ':' || priority || ':' || (created_by IS NOT NULL) FROM jobs WHERE work_id = $1 AND state = 'pending'`, work); got != "match_metadata:5:true" {
		t.Errorf("the job: %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'metadata.refresh' AND target_id = $1`, fmt.Sprint(work)); got != "1" {
		t.Errorf("audited %s times", got)
	}
	// Asking again while it waits is the same request, and is not audited as another.
	if rec := s.do(admin, "POST", url, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "alreadyQueued") {
		t.Errorf("asking twice: %d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE work_id = $1 AND type = 'match_metadata'`, work); got != "1" {
		t.Errorf("one job: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'metadata.refresh'`); got != "1" {
		t.Errorf("audited %s times", got)
	}
	// It does not stop the other jobs of the work, and does not touch its status.
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE work_id = $1 AND type <> 'match_metadata'`, work); got != "0" {
		t.Errorf("another job appeared: %s", got)
	}
}

func TestMetadataRefresh_ThereIsNoSuchWorkOrItWasRetired(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Aposentado", "X", "a.epub", "epub")
	s.setProvider("openlibrary", `{"enabled":true}`)
	s.exec(`DELETE FROM jobs`)
	if rec := s.do(admin, "POST", "/admin/works/999999/metadata-refresh", ""); rec.Code != http.StatusNotFound {
		t.Errorf("no such work: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", "/admin/works/abc/metadata-refresh", ""); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/metadata-refresh", work), ""); rec.Code != http.StatusNotFound {
		t.Errorf("a retired work: %d", rec.Code)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs`); got != "0" {
		t.Errorf("jobs queued for nothing: %s", got)
	}
}

func TestMetadataRefresh_TheStatusFollowsTheLastSearchOfTheWork(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("A nuvem", "Neal Shusterman", "nuvem.epub", "epub")
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	s.exec(`DELETE FROM jobs`)
	if got := s.refreshStatus(work); !strings.Contains(got, `"job":null`) {
		t.Errorf("never searched: %s", got)
	}
	s.exec(`INSERT INTO jobs (type, work_id, payload, state, last_error, finished_at) VALUES ('match_metadata', $1, '{}', 'failed', 'HTTP 429', now())`, work)
	if got := s.refreshStatus(work); !strings.Contains(got, `"state":"failed"`) || !strings.Contains(got, `"error":"HTTP 429"`) || !strings.Contains(got, "finishedAt") {
		t.Errorf("failed: %s", got)
	}
	s.exec(`INSERT INTO jobs (type, work_id, payload, state) VALUES ('match_metadata', $1, '{}', 'running')`, work)
	got := s.refreshStatus(work)
	if !strings.Contains(got, `"state":"running"`) || strings.Contains(got, "finishedAt") || strings.Contains(got, `"error"`) {
		t.Errorf("running: %s", got)
	}
	s.exec(`UPDATE jobs SET state = 'succeeded', finished_at = now(), payload = '{"result": {"found": true, "source": "Google Books", "title": "A nuvem", "new": 3}}' WHERE work_id = $1 AND state = 'running'`, work)
	got = s.refreshStatus(work)
	if !strings.Contains(got, `"state":"succeeded"`) || !strings.Contains(got, `"new":3`) || !strings.Contains(got, `"source":"Google Books"`) || strings.Contains(got, `"error"`) {
		t.Errorf("done: %s", got)
	}
	// Another work's search, or another kind of job, is not this one's.
	s.exec(`INSERT INTO jobs (type, work_id, payload, state) VALUES ('match_metadata', $1, '{"result": {"new": 9}}', 'succeeded')`, other)
	s.exec(`INSERT INTO jobs (type, work_id, payload, state) VALUES ('extract_text', $1, '{"result": {"new": 8}}', 'succeeded')`, work)
	if got := s.refreshStatus(work); !strings.Contains(got, `"new":3`) {
		t.Errorf("the status of another: %s", got)
	}
	if rec := s.do(admin, "GET", "/admin/works/abc/metadata-refresh", ""); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
}
