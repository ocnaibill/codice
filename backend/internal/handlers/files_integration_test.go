package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestFiles_AreServedOnlyWhenTheCatalogOwnsThem(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "1_duna.epub", "epub")
	os.WriteFile(filepath.Join(s.storage, "1_duna.epub"), []byte("0123456789"), 0o644)
	// On disk but owned by no work, and a sibling directory.
	os.WriteFile(filepath.Join(s.storage, "orphan.epub"), []byte("orphan"), 0o644)
	os.MkdirAll(filepath.Join(s.storage, "covers"), 0o755)
	os.WriteFile(filepath.Join(s.storage, "covers", "duna.jpg"), []byte("jpeg"), 0o644)
	os.WriteFile(filepath.Join(s.storage, "covers", "placeholder.svg"), []byte("<svg/>"), 0o644)
	s.exec(`UPDATE editions SET cover_url = '/covers/duna.jpg' WHERE work_id = $1`, duna)

	r := chi.NewRouter()
	r.Use(identityFromHeaders)
	r.Method("GET", "/files/*", &FilesHandler{Root: s.storage, Lookup: NewFileLookup(s.db)})
	r.Method("GET", "/covers/*", &FilesHandler{Root: filepath.Join(s.storage, "covers"), Lookup: NewCoverLookup(s.db)})

	get := func(a actor, target string, hdr ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", target, nil)
		req.Header.Set("X-Test-User", a.id)
		req.Header.Set("X-Test-Role", a.role)
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// An owned file is served, with Range support.
	if rec := get(ana, "/files/1_duna.epub"); rec.Code != 200 || rec.Body.String() != "0123456789" {
		t.Fatalf("owned file: %d %q", rec.Code, rec.Body.String())
	}
	if rec := get(ana, "/files/1_duna.epub", "Range", "bytes=2-5"); rec.Code != http.StatusPartialContent || rec.Body.String() != "2345" {
		t.Errorf("range request: %d %q", rec.Code, rec.Body.String())
	}

	// Anything the catalog does not own is invisible, even if it exists on disk.
	for _, target := range []string{
		"/files/orphan.epub",         // on disk, in no work
		"/files/",                    // directory listing
		"/files/covers",              // a directory
		"/files/covers/duna.jpg",     // covers are not files of a work
		"/files/../etc/passwd",       // traversal
		"/files/%2e%2e/etc/passwd",   // encoded traversal
		"/files/nope.epub",           // unknown
		"/files/1_duna.epub/../../x", // cleaned traversal
	} {
		if rec := get(ana, target); rec.Code != 404 {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}

	// A retired work's file is hidden from readers and open to staff.
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", duna), "")
	if rec := get(ana, "/files/1_duna.epub"); rec.Code != 404 {
		t.Errorf("retired work's file for a reader: %d, want 404", rec.Code)
	}
	if rec := get(admin, "/files/1_duna.epub"); rec.Code != 200 {
		t.Errorf("retired work's file for staff: %d, want 200", rec.Code)
	}
	s.do(admin, "POST", fmt.Sprintf("/works/%d/restore", duna), "")
	if rec := get(ana, "/files/1_duna.epub"); rec.Code != 200 {
		t.Errorf("restored work's file: %d, want 200", rec.Code)
	}

	// Covers: served, hidden with a retired work, placeholder always served.
	if rec := get(ana, "/covers/duna.jpg"); rec.Code != 200 {
		t.Errorf("cover: %d", rec.Code)
	}
	if rec := get(ana, "/covers/placeholder.svg"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Errorf("placeholder: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", duna), "")
	if rec := get(ana, "/covers/duna.jpg"); rec.Code != 404 {
		t.Errorf("cover of a retired work for a reader: %d, want 404", rec.Code)
	}
	if rec := get(admin, "/covers/duna.jpg"); rec.Code != 200 {
		t.Errorf("cover of a retired work for staff: %d", rec.Code)
	}
	if rec := get(ana, "/covers/"); rec.Code != 404 {
		t.Errorf("covers directory listing: %d, want 404", rec.Code)
	}
}
