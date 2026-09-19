package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestStorageAdmin_PreviewThenConfirmMovesFilesAndTheyStayReachable(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "aa11_Duna.epub", "epub")
	watch := s.addWork("Watchmen", "Alan Moore", "bb22_w.cbz", "cbz")
	s.exec(`UPDATE works SET language = 'pt', publisher = 'Aleph', publication_date = '2017' WHERE id = $1`, duna)
	for _, f := range []string{"aa11_Duna.epub", "bb22_w.cbz"} {
		os.WriteFile(filepath.Join(s.storage, f), []byte("bytes of "+f), 0o644)
	}

	type preview struct {
		Moves []struct {
			FileID int64
			From   string
			To     string
		}
		Unchanged int
		Hash      string
	}
	var p preview
	rec := s.do(admin, "GET", "/admin/storage/reorganize", "")
	if rec.Code != 200 {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &p)
	if len(p.Moves) != 2 || p.Hash == "" {
		t.Fatalf("preview = %+v", p)
	}
	// Previewing moves nothing.
	if _, err := os.Stat(filepath.Join(s.storage, "aa11_Duna.epub")); err != nil {
		t.Fatal("the preview moved a file")
	}

	// Confirming without, or with a wrong, hash is refused.
	if code := s.do(admin, "POST", "/admin/storage/reorganize", `{}`).Code; code != 400 {
		t.Errorf("no hash: %d, want 400", code)
	}
	if code := s.do(admin, "POST", "/admin/storage/reorganize", `{"hash":"stale"}`).Code; code != 409 {
		t.Errorf("stale hash: %d, want 409", code)
	}

	rec = s.do(admin, "POST", "/admin/storage/reorganize", fmt.Sprintf(`{"hash":%q}`, p.Hash))
	if rec.Code != 200 {
		t.Fatalf("reorganize: %d %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Moved    int
		Failures []any
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Moved != 2 || len(res.Failures) != 0 {
		t.Errorf("result = %+v", res)
	}
	if got := s.scalar(`SELECT string_agg(action || ':' || COALESCE(details->>'moved', ''), ',') FROM audit_log WHERE action = 'storage.reorganize'`); got != "storage.reorganize:2" {
		t.Errorf("audit = %q", got)
	}

	// The catalog now points at the new places, with escaped URLs...
	w, _ := s.detail(ana, duna)
	wantURL := "/files/Frank%20Herbert/Duna/Portugu%C3%AAs%20%E2%80%94%20Aleph%20%E2%80%94%202017/Duna.epub"
	if w.FileURL != wantURL {
		t.Errorf("FileURL = %q, want %q", w.FileURL, wantURL)
	}
	if w2, _ := s.detail(ana, watch); !strings.HasPrefix(w2.FileURL, "/files/Alan%20Moore/Watchmen/") {
		t.Errorf("second FileURL = %q", w2.FileURL)
	}

	// ...and the file is still served from there, through that exact URL.
	r := chi.NewRouter()
	r.Use(identityFromHeaders)
	r.Method("GET", "/files/*", &FilesHandler{Root: s.storage, Lookup: NewFileLookup(s.db)})
	req := httptest.NewRequest("GET", wantURL, nil)
	req.Header.Set("X-Test-User", idAna)
	req.Header.Set("X-Test-Role", "reader")
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	if out.Code != 200 || out.Body.String() != "bytes of aa11_Duna.epub" {
		t.Errorf("serving the moved file: %d %q", out.Code, out.Body.String())
	}
	// The old flat path is no longer a file of the catalog.
	req = httptest.NewRequest("GET", "/files/aa11_Duna.epub", nil)
	req.Header.Set("X-Test-User", idAna)
	req.Header.Set("X-Test-Role", "reader")
	out = httptest.NewRecorder()
	r.ServeHTTP(out, req)
	if out.Code != 404 {
		t.Errorf("the old path: %d, want 404", out.Code)
	}

	// After reorganizing there is nothing left to do.
	json.Unmarshal(s.do(admin, "GET", "/admin/storage/reorganize", "").Body.Bytes(), &p)
	if len(p.Moves) != 0 || p.Unchanged != 2 {
		t.Errorf("second preview = %+v", p)
	}
}

func TestJobRerun_FollowsTheFileToWhereItIsNow(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "aa11_Duna.epub", "epub")
	os.WriteFile(filepath.Join(s.storage, "aa11_Duna.epub"), []byte("x"), 0o644)
	old := filepath.Join(s.storage, "aa11_Duna.epub")

	// An ingestion job that failed while the file was still at its first path.
	s.exec(`INSERT INTO jobs (type, work_id, payload, state, attempts, error_kind, last_error)
		VALUES ('ingest', $1, jsonb_build_object('file_path', $2::text), 'failed', 3, 'temporary', 'boom')`, id, old)
	jobID := s.scalar(`SELECT id FROM jobs WHERE work_id = $1`, id)

	// The file is reorganized afterwards.
	var p struct{ Hash string }
	json.Unmarshal(s.do(admin, "GET", "/admin/storage/reorganize", "").Body.Bytes(), &p)
	s.do(admin, "POST", "/admin/storage/reorganize", fmt.Sprintf(`{"hash":%q}`, p.Hash))
	if _, err := os.Stat(old); err == nil {
		t.Fatal("the file did not move")
	}

	if code := s.do(admin, "POST", "/admin/jobs/"+jobID+"/rerun", "").Code; code != 200 {
		t.Fatalf("rerun: %d", code)
	}
	got := s.scalar(`SELECT payload->>'file_path' FROM jobs WHERE id = $1`, jobID)
	want := filepath.Join(s.storage, "Frank Herbert", "Duna", "Duna.epub")
	if got != want {
		t.Errorf("payload path = %q, want %q (a stale path would fail with \"file not found\")", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("the refreshed path does not exist: %v", err)
	}
}
