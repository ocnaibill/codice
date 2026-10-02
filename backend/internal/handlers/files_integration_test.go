package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// A browser sends a comma as %2C and a semicolon as %3B, and authors are written "Seitz, Tim" and
// "Aho; Lam": a file under such a name has to be found.
func TestFiles_WithACommaOrASemicolonInThePathAreServed(t *testing.T) {
	s := newCatalogStack(t)
	for _, rel := range []string{
		"Justin Seitz, Tim Arnold/Black Hat Python/Português — No Starch Press — 2021/Black Hat Python.epub",
		"Alfred V. Aho; Monica S. Lam/Compilers/Inglês — 2006/Compilers.pdf",
		"Autor/100% Verdade/Edição #1 (a+b).epub",
		"Autor/O Livro [1]/livro.epub",
		"Autor/100%/livro.epub", // a name that holds an encoded-looking text
		"Autor/a%2Cb/livro.epub",
		"Justin Seitz,/a%2Cb/livro.epub", // a comma and an encoded-looking text in one path
	} {
		full := filepath.Join(s.storage, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("conteúdo de "+rel), 0o644)
	}
	r := chi.NewRouter()
	r.Use(identityFromHeaders)
	r.Method("GET", "/files/*", &FilesHandler{Root: s.storage, Lookup: func(_ context.Context, rel string) (FileAccess, error) {
		if _, err := os.Stat(filepath.Join(s.storage, filepath.FromSlash(rel))); err != nil {
			return FileAccess{}, ErrFileNotFound
		}
		return FileAccess{}, nil
	}})
	get := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", target, nil)
		req.Header.Set("X-Test-User", ana.id)
		req.Header.Set("X-Test-Role", ana.role)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	// escape is what the front end does: every segment through encodeURIComponent.
	escape := func(rel string) string {
		parts := strings.Split(rel, "/")
		for i, p := range parts {
			e := url.QueryEscape(p)
			e = strings.ReplaceAll(e, "+", "%20")
			// encodeURIComponent leaves ! ' ( ) * alone and escapes everything else, so a comma is %2C.
			parts[i] = e
		}
		return "/files/" + strings.Join(parts, "/")
	}
	for _, c := range []struct{ rel, target string }{
		{"Justin Seitz, Tim Arnold/Black Hat Python/Português — No Starch Press — 2021/Black Hat Python.epub", ""},
		{"Alfred V. Aho; Monica S. Lam/Compilers/Inglês — 2006/Compilers.pdf", ""},
		{"Autor/100% Verdade/Edição #1 (a+b).epub", ""},
		{"Autor/O Livro [1]/livro.epub", ""},
		{"Autor/100%/livro.epub", ""},
		{"Justin Seitz,/a%2Cb/livro.epub", ""}, // %2C and %252C together: decoded once, not twice
		// The comma written as it comes, and encoded by hand in the one place.
		{"Justin Seitz, Tim Arnold/Black Hat Python/Português — No Starch Press — 2021/Black Hat Python.epub", "/files/Justin%20Seitz,%20Tim%20Arnold/Black%20Hat%20Python/Portugu%C3%AAs%20%E2%80%94%20No%20Starch%20Press%20%E2%80%94%202021/Black%20Hat%20Python.epub"},
	} {
		target := c.target
		if target == "" {
			target = escape(c.rel)
		}
		if rec := get(target); rec.Code != 200 || rec.Body.String() != "conteúdo de "+c.rel {
			t.Errorf("GET %s = %d %q", target, rec.Code, rec.Body.String())
		}
	}
	// The decoding happens once: %252C is a percent sign and "252C", not a comma, so it is not the file "a,b".
	if rec := get("/files/Autor/a%252Cb/livro.epub"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "a%2Cb") {
		t.Errorf("a name holding an encoded text: %d %q", rec.Code, rec.Body.String())
	}
	// A path that does not exist is still not found, and what climbs out of the storage is refused whatever its encoding.
	for _, target := range []string{
		"/files/Justin%20Seitz%2C%20Tim%20Arnold/nope.epub",
		"/files/..%2F..%2Fetc%2Fpasswd",
		"/files/Autor%2F..%2F..%2Fetc%2Fpasswd",
		"/files/%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"/files/Autor%2Fa%5C..%5Cb",
		"/files/Autor%2C",
	} {
		if rec := get(target); rec.Code != 404 {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}
