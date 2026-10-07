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
	// The type narrows the list too (the screen of referenced files asks for the scans only, so a flood of
	// ingestion jobs cannot push a running scan out of the page).
	s.exec(`INSERT INTO jobs (type, payload, state) VALUES ('scan', '{"root_id": 1}', 'running')`)
	get("?type=scan")
	if len(list.Data) != 1 || list.Data[0].State != "running" {
		t.Fatalf("scans = %+v", list.Data)
	}
	get("?type=scan&state=pending")
	if len(list.Data) != 0 {
		t.Fatalf("a state and a type that do not meet = %+v", list.Data)
	}
	get("?type=scan&limit=1")
	if len(list.Data) != 1 {
		t.Fatalf("limit with a type = %+v", list.Data)
	}
	s.exec(`DELETE FROM jobs WHERE type = 'scan'`)
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

func TestJobsAdmin_RerunAllFailedAtOnce(t *testing.T) {
	s := newCatalogStack(t)
	w1 := s.addWork("Um", "Ana", "um.epub", "epub")
	w2 := s.addWork("Dois", "Ana", "dois.pdf", "pdf")
	w3 := s.addWork("Três", "Ana", "tres.epub", "epub")
	w4 := s.addWork("Quatro", "Ana", "quatro.epub", "epub")
	w5 := s.addWork("Cinco", "Ana", "cinco.epub", "epub")
	job := func(kind string, work any, state, payload string) int64 {
		var id int64
		err := s.db.QueryRow(`INSERT INTO jobs (type, work_id, payload, state, attempts, last_error, error_kind, finished_at)
			VALUES ($1, $2, $3::jsonb, $4::varchar, 3, CASE WHEN $4 = 'failed' THEN 'it did not work' END, CASE WHEN $4 = 'failed' THEN 'permanent' END,
			        CASE WHEN $4 IN ('failed', 'succeeded', 'cancelled') THEN now() END) RETURNING id`, kind, work, payload, state).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ingest := job("ingest", w1, "failed", `{"file_path": "/old/place/um.epub"}`)
	ocr := job("ocr", w2, "failed", `{}`)
	oldText := job("extract_text", w3, "failed", `{}`)
	newText := job("extract_text", w3, "failed", `{}`)
	busyFailed := job("ingest", w4, "failed", `{}`)
	busyLive := job("ingest", w4, "pending", `{}`)
	scan := job("scan", nil, "failed", `{"root_id": 1}`)
	done := job("extract_text", w5, "succeeded", `{}`)
	cancelled := job("ocr", w5, "cancelled", `{}`)

	rec := s.do(admin, "POST", "/admin/jobs/rerun-failed", "")
	var r struct{ Requeued, Left int }
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || r.Requeued != 3 || r.Left != 3 {
		t.Fatalf("%d %s, want 3 requeued and 3 left", rec.Code, rec.Body.String())
	}
	state := func(id int64) string {
		return s.scalar(`SELECT state || '/' || attempts || '/' || COALESCE(last_error, '-') FROM jobs WHERE id = $1`, id)
	}
	for name, id := range map[string]int64{"ingest": ingest, "ocr": ocr, "newest text": newText} {
		if got := state(id); got != "pending/0/-" {
			t.Errorf("%s = %q, want pending/0/-", name, got)
		}
	}
	// What it must not touch: the older failed job of the same kind (the newest returns), a work that already has a live
	// job of that kind, a job with no work, and what is not failed.
	for name, id := range map[string]int64{"older text": oldText, "work with a live job": busyFailed, "scan": scan} {
		if got := state(id); got != "failed/3/it did not work" {
			t.Errorf("%s = %q, want it left failed", name, got)
		}
	}
	if state(busyLive) != "pending/3/-" || state(done) != "succeeded/3/-" || state(cancelled) != "cancelled/3/-" {
		t.Errorf("a job that was not failed changed: %s %s %s", state(busyLive), state(done), state(cancelled))
	}
	// The ingestion looks for its file where it is now, not where it was.
	want := s.storage + "/" + s.scalar(`SELECT l.path FROM work_primary wp JOIN storage_locations l ON l.file_id = wp.file_id WHERE wp.work_id = $1`, w1)
	if got := s.scalar(`SELECT payload->>'file_path' FROM jobs WHERE id = $1`, ingest); got != want {
		t.Errorf("file_path = %q, want %q", got, want)
	}
	// It is audited, with what it did.
	if got := s.scalar(`SELECT details->>'requeued' || '/' || (details->>'left') FROM audit_log WHERE action = 'job.rerun_failed'`); got != "3/3" {
		t.Errorf("audit = %q, want 3/3", got)
	}

	// Asking again has nothing left that it may touch, and says so without an audit entry of nothing.
	rec = s.do(admin, "POST", "/admin/jobs/rerun-failed", "")
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || r.Requeued != 0 || r.Left != 3 {
		t.Errorf("second call: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'job.rerun_failed'`); got != "1" {
		t.Errorf("%s audit entries, want 1", got)
	}
}
