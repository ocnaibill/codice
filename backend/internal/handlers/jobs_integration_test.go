package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestJobsAdmin_ListRerunAndCancel(t *testing.T) {
	s := newCatalogStack(t)
	// Two uploads produce two pending jobs.
	s.upload(admin, "a.epub", corpusFile(t, "epub_acentos.epub"))
	s.upload(admin, "b.epub", corpusFile(t, "epub_alterado.epub"))

	var list struct {
		Data []struct {
			ID        int64  `json:"id"`
			State     string `json:"state"`
			WorkTitle string `json:"workTitle"`
			Priority  int    `json:"priority"`
			LastError string `json:"lastError"`
			ErrorKind string `json:"errorKind"`
		} `json:"data"`
		Counts map[string]int `json:"counts"`
	}
	get := func(q string) {
		list.Data = nil
		rec := s.do(admin, "GET", "/admin/jobs"+q, "")
		if rec.Code != 200 {
			t.Fatalf("GET /admin/jobs%s: %d", q, rec.Code)
		}
		json.Unmarshal(rec.Body.Bytes(), &list)
	}
	get("")
	if len(list.Data) != 2 || list.Counts["pending"] != 2 || list.Data[0].WorkTitle == "" {
		t.Fatalf("list = %+v", list)
	}
	first, second := list.Data[1].ID, list.Data[0].ID // newest first

	// A worker fails the first permanently with a clear reason; the admin sees it and reruns it.
	s.exec(`SELECT jobs_claim('w', 120, 5)`)
	s.exec(`SELECT jobs_fail($1, 'w', 'permanent', 'the archive has no readable pages')`, first)
	get("?state=failed")
	if len(list.Data) != 1 || list.Data[0].LastError != "the archive has no readable pages" || list.Data[0].ErrorKind != "permanent" {
		t.Fatalf("failed jobs = %+v", list.Data)
	}

	if code := s.do(admin, "POST", fmt.Sprintf("/admin/jobs/%d/rerun", first), "").Code; code != 200 {
		t.Fatalf("rerun: %d", code)
	}
	if got := s.scalar(`SELECT state || '/' || attempts FROM jobs WHERE id = $1`, first); got != "pending/0" {
		t.Errorf("after rerun = %q", got)
	}
	// Cancel the other one.
	if code := s.do(admin, "POST", fmt.Sprintf("/admin/jobs/%d/cancel", second), "").Code; code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	if got := s.scalar(`SELECT state FROM jobs WHERE id = $1`, second); got != "cancelled" {
		t.Errorf("after cancel = %q", got)
	}

	// Wrong states and unknown ids are told apart.
	if code := s.do(admin, "POST", fmt.Sprintf("/admin/jobs/%d/rerun", first), "").Code; code != 409 {
		t.Errorf("rerun of a pending job: %d, want 409", code)
	}
	if code := s.do(admin, "POST", "/admin/jobs/999999/cancel", "").Code; code != 404 {
		t.Errorf("unknown job: %d, want 404", code)
	}
	if code := s.do(admin, "POST", "/admin/jobs/abc/cancel", "").Code; code != 404 {
		t.Errorf("malformed id: %d, want 404", code)
	}

	// Both actions are audited with the actor.
	if got := s.scalar(`SELECT string_agg(action || ':' || target_id || ':' || actor_username, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'job.%'`); got != fmt.Sprintf("job.rerun:%d:adm,job.cancel:%d:adm", first, second) {
		t.Errorf("audit trail = %q", got)
	}
}
