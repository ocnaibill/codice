package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/storage"
)

func referencedLibrary(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	put := func(rel, from string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, corpusFile(t, from), 0o644)
	}
	put("Quadrinhos/Watchmen 01.cbz", "cbz_ltr.cbz")
	put("Notas/notas pessoais.txt", "notas.txt")
	put("Livros/Duna.epub", "epub_acentos.epub")
	return dir
}

func (s *catalogStack) addRoot(a actor, path string) *httptest.ResponseRecorder {
	return s.do(a, "POST", "/admin/storage/roots", fmt.Sprintf(`{"path":%q}`, path))
}

func TestRoots_TheOwnerAuthorisesDirectoriesWithSafeguards(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)

	if rec := s.addRoot(admin, lib); rec.Code != 201 {
		t.Fatalf("add root: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Roots   []StorageRoot
		Managed string
	}
	json.Unmarshal(s.do(ana, "GET", "/admin/storage/roots", "").Body.Bytes(), &list)
	if len(list.Roots) != 1 || list.Roots[0].Path != lib || list.Managed != s.storage {
		t.Errorf("list = %+v", list)
	}

	cases := map[string]struct {
		path string
		want int
	}{
		"a relative path":                 {"relative/dir", 400},
		"a directory that does not exist": {filepath.Join(lib, "nope"), 400},
		"a file, not a directory":         {filepath.Join(lib, "Livros", "Duna.epub"), 400},
		"the managed storage itself":      {s.storage, 400},
		"inside the managed storage":      {filepath.Join(s.storage, "covers"), 400},
		"a parent of the managed storage": {filepath.Dir(s.storage), 400},
		"the same root again":             {lib, 409},
		"a directory inside a root":       {filepath.Join(lib, "Livros"), 409},
	}
	os.MkdirAll(filepath.Join(s.storage, "covers"), 0o755)
	for name, c := range cases {
		if rec := s.addRoot(admin, c.path); rec.Code != c.want {
			t.Errorf("%s: got %d, want %d (%s)", name, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
	}
	// Nesting is refused in both directions (checked apart from the shared test folders).
	base, _ := filepath.EvalSymlinks(t.TempDir())
	inner := filepath.Join(base, "a", "b")
	os.MkdirAll(inner, 0o755)
	if rec := s.addRoot(admin, inner); rec.Code != 201 {
		t.Fatalf("nested setup: %d", rec.Code)
	}
	if rec := s.addRoot(admin, filepath.Join(base, "a")); rec.Code != 409 {
		t.Errorf("a directory containing a root: %d, want 409", rec.Code)
	}
	// A symlink is stored by the real path it leads to, so it cannot be used to sneak a second view of the same place.
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(lib, link)
	if rec := s.addRoot(admin, link); rec.Code != 409 {
		t.Errorf("a symlink to an existing root: %d, want 409", rec.Code)
	}
	other, _ := filepath.EvalSymlinks(t.TempDir())
	linkOther := filepath.Join(t.TempDir(), "link2")
	os.Symlink(other, linkOther)
	rec := s.addRoot(admin, linkOther)
	var added StorageRoot
	json.Unmarshal(rec.Body.Bytes(), &added)
	if rec.Code != 201 || added.Path != other {
		t.Errorf("a symlinked root: %d stored %q, want the real path %q", rec.Code, added.Path, other)
	}

	// Removing is refused while catalogued files still live there.
	var first StorageRoot
	json.Unmarshal(s.do(admin, "GET", "/admin/storage/roots", "").Body.Bytes(), &list)
	first = list.Roots[0]
	scanner := &storage.Scanner{DB: s.db}
	if _, err := scanner.Scan(context.Background(), lib, "", idAdmin); err != nil {
		t.Fatal(err)
	}
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/storage/roots/%d", first.ID), "").Code; code != 409 {
		t.Errorf("removing a root with catalogued files: %d, want 409", code)
	}
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/storage/roots/%d", added.ID), "").Code; code != 204 {
		t.Errorf("removing an empty root: %d, want 204", code)
	}
	if code := s.do(admin, "DELETE", "/admin/storage/roots/999", "").Code; code != 404 {
		t.Errorf("unknown root: %d, want 404", code)
	}
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'storage.root_%'`); !strings.HasPrefix(got, "storage.root_add,storage.root_add") || !strings.HasSuffix(got, "storage.root_remove") {
		t.Errorf("audit = %q", got)
	}
}

func TestScanEndpoint_QueuesOneJobPerDirectoryInsideAuthorisedRoots(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	s.addRoot(admin, lib)
	rootID := s.scalar(`SELECT id FROM storage_roots`)

	body := fmt.Sprintf(`{"rootId":%s}`, rootID)
	rec := s.do(admin, "POST", "/admin/library/scan", body)
	if rec.Code != 202 {
		t.Fatalf("scan: %d %s", rec.Code, rec.Body.String())
	}
	var first struct {
		JobID int64 `json:"job_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &first)
	// Asking again while it is waiting returns the same job.
	var again struct {
		JobID int64 `json:"job_id"`
	}
	json.Unmarshal(s.do(admin, "POST", "/admin/library/scan", body).Body.Bytes(), &again)
	if again.JobID != first.JobID {
		t.Errorf("a second identical scan created job %d instead of reusing %d", again.JobID, first.JobID)
	}
	// A different subdirectory is a different scan.
	sub := s.do(admin, "POST", "/admin/library/scan", fmt.Sprintf(`{"rootId":%s,"subdir":"Livros"}`, rootID))
	var other struct {
		JobID int64 `json:"job_id"`
	}
	json.Unmarshal(sub.Body.Bytes(), &other)
	if sub.Code != 202 || other.JobID == first.JobID {
		t.Errorf("subdirectory scan: %d %d", sub.Code, other.JobID)
	}
	if got := s.scalar(`SELECT state || '/' || type || '/' || priority FROM jobs WHERE id = $1`, first.JobID); got != "pending/scan/10" {
		t.Errorf("job = %q", got)
	}

	// Nothing outside the authorised root can be scanned.
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"an unknown root":          {`{"rootId":999}`, 404},
		"no root":                  {`{}`, 400},
		"a parent directory":       {fmt.Sprintf(`{"rootId":%s,"subdir":"../"}`, rootID), 400},
		"an absolute subdirectory": {fmt.Sprintf(`{"rootId":%s,"subdir":"/etc"}`, rootID), 400},
	} {
		if code := s.do(admin, "POST", "/admin/library/scan", tc.body).Code; code != tc.want {
			t.Errorf("%s: %d, want %d", name, code, tc.want)
		}
	}
}

func TestReferencedFiles_AreServedListedAndReadableByID(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	if _, err := (&storage.Scanner{DB: s.db}).Scan(context.Background(), lib, "", idAdmin); err != nil {
		t.Fatal(err)
	}
	find := func(rel string) (workID int, fileID int64) {
		s.t.Helper()
		if err := s.db.QueryRow(`SELECT work_id, file_id FROM work_primary WHERE file_path = $1`, rel).Scan(&workID, &fileID); err != nil {
			s.t.Fatal(err)
		}
		return
	}
	notesWork, notesFile := find("Notas/notas pessoais.txt")
	comicWork, comicFile := find("Quadrinhos/Watchmen 01.cbz")

	// The catalog points at /file/<id>, not at a path under the storage directory.
	w, _ := s.detail(ana, notesWork)
	if w.FileURL != fmt.Sprintf("/file/%d", notesFile) {
		t.Errorf("FileURL = %q", w.FileURL)
	}
	if w.Editions[0].Files[0].URL != w.FileURL {
		t.Errorf("edition file URL = %q", w.Editions[0].Files[0].URL)
	}
	if list := s.list(ana, ""); len(list.Data) != 3 {
		t.Errorf("listing = %d works", len(list.Data))
	}
	if rec := s.do(ana, "GET", "/opds/recent", ""); !strings.Contains(rec.Body.String(), fmt.Sprintf("/file/%d", notesFile)) {
		t.Errorf("OPDS does not link the referenced file by id: %s", rec.Body.String())
	}

	// It is served, with Range support, from where it lives.
	want := string(corpusFile(t, "notas.txt"))
	rec := s.do(ana, "GET", fmt.Sprintf("/file/%d", notesFile), "")
	if rec.Code != 200 || rec.Body.String() != want {
		t.Fatalf("serving by id: %d %q", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/file/%d", notesFile), nil)
	req.Header.Set("Range", "bytes=0-3")
	req.Header.Set("X-Test-User", idAna)
	req.Header.Set("X-Test-Role", "reader")
	part := httptest.NewRecorder()
	s.router.ServeHTTP(part, req)
	if part.Code != http.StatusPartialContent || part.Body.String() != want[:4] {
		t.Errorf("range: %d %q", part.Code, part.Body.String())
	}

	// The readers that open files by work id work for referenced files too.
	if rec := s.do(ana, "GET", fmt.Sprintf("/works/%d/text", notesWork), ""); rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("text viewer: %d", rec.Code)
	}
	var pages []any
	rec = s.do(ana, "GET", fmt.Sprintf("/works/%d/pages", comicWork), "")
	json.Unmarshal(rec.Body.Bytes(), &pages)
	if rec.Code != 200 || len(pages) == 0 {
		t.Errorf("page listing of a referenced CBZ: %d %s", rec.Code, rec.Body.String())
	}

	// Unknown, malformed, retired and missing files are refused.
	for _, target := range []string{"/file/999999", "/file/abc", "/file/0", "/file/-1"} {
		if code := s.do(ana, "GET", target, "").Code; code != 404 {
			t.Errorf("GET %s = %d, want 404", target, code)
		}
	}
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", notesWork), "")
	if code := s.do(ana, "GET", fmt.Sprintf("/file/%d", notesFile), "").Code; code != 404 {
		t.Errorf("a retired work's file for a reader: %d, want 404", code)
	}
	if code := s.do(admin, "GET", fmt.Sprintf("/file/%d", notesFile), "").Code; code != 200 {
		t.Errorf("a retired work's file for staff: %d, want 200", code)
	}
	s.exec(`UPDATE storage_locations SET state = 'missing' WHERE file_id = $1`, comicFile)
	if code := s.do(ana, "GET", fmt.Sprintf("/file/%d", comicFile), "").Code; code != 404 {
		t.Errorf("a missing file: %d, want 404", code)
	}
	if code := s.do(ana, "GET", fmt.Sprintf("/works/%d/pages", comicWork), "").Code; code == 200 {
		t.Error("the page listing of a missing file was served")
	}
}

func TestReferencedFiles_ASymlinkPlacedLaterCannotLeakOtherFiles(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	(&storage.Scanner{DB: s.db}).Scan(context.Background(), lib, "", idAdmin)
	var fileID int64
	s.db.QueryRow(`SELECT file_id FROM work_primary WHERE file_path = 'Notas/notas pessoais.txt'`).Scan(&fileID)

	// After cataloguing, the file is replaced by a link to something private.
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("private data"), 0o600)
	target := filepath.Join(lib, "Notas", "notas pessoais.txt")
	os.Remove(target)
	os.Symlink(secret, target)

	rec := s.do(ana, "GET", fmt.Sprintf("/file/%d", fileID), "")
	if rec.Code != 404 || strings.Contains(rec.Body.String(), "private") {
		t.Errorf("a symlink out of the root was served: %d %q", rec.Code, rec.Body.String())
	}
	// A link that stays inside the root is fine.
	os.Remove(target)
	os.Symlink(filepath.Join(lib, "Livros", "Duna.epub"), target)
	if code := s.do(ana, "GET", fmt.Sprintf("/file/%d", fileID), "").Code; code != 200 {
		t.Errorf("a link that stays inside the root: %d", code)
	}
}

func TestBulkImport_AcceptsDirectoriesTheOwnerAuthorised(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)

	// Not authorised yet: refused, and nothing is read.
	rec := s.do(admin, "POST", "/works/bulk-import", fmt.Sprintf(`{"directory":%q}`, lib))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("before authorising: %d, want 403", rec.Code)
	}
	s.addRoot(admin, lib)
	rec = s.do(admin, "POST", "/works/bulk-import", fmt.Sprintf(`{"directory":%q}`, lib))
	var r BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || r.Enqueued != 3 {
		t.Fatalf("after authorising: %d %+v", rec.Code, r)
	}
	// Importing copies into the managed storage and leaves the originals alone.
	if got := s.scalar(`SELECT count(*) FROM storage_locations WHERE mode = 'managed'`); got != "3" {
		t.Errorf("managed locations = %s", got)
	}
	if _, err := os.Stat(filepath.Join(lib, "Livros", "Duna.epub")); err != nil {
		t.Error("the original was removed by an import")
	}
}

func TestBulkImport_RemovesOriginalsOnlyWhenAskedAndOnlyWhenVerified(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	s.addRoot(admin, lib)

	rec := s.do(admin, "POST", "/works/bulk-import", fmt.Sprintf(`{"directory":%q,"removeOriginals":true}`, lib))
	var r BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || r.Enqueued != 3 || r.OriginalsRemoved != 3 || r.CleanupPending != 0 {
		t.Fatalf("import as a move: %d %+v", rec.Code, r)
	}
	for _, rel := range []string{"Livros/Duna.epub", "Quadrinhos/Watchmen 01.cbz", "Notas/notas pessoais.txt"} {
		if _, err := os.Stat(filepath.Join(lib, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s was not removed after a verified copy", rel)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM storage_locations WHERE mode = 'managed'`); got != "3" {
		t.Errorf("managed copies = %s", got)
	}
}

func TestBulkImport_AnOriginalThatCannotBeRemovedIsReportedNotLost(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	s.addRoot(admin, lib)
	locked := filepath.Join(lib, "Livros")
	os.Chmod(locked, 0o555)
	defer os.Chmod(locked, 0o755)

	rec := s.do(admin, "POST", "/works/bulk-import", fmt.Sprintf(`{"directory":%q,"removeOriginals":true}`, lib))
	var r BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Enqueued != 3 || r.OriginalsRemoved != 2 || r.CleanupPending != 1 {
		t.Fatalf("report = %+v, want 2 removed and 1 kept", r)
	}
	if _, err := os.Stat(filepath.Join(locked, "Duna.epub")); err != nil {
		t.Error("the original that could not be removed vanished")
	}

	var list struct{ Data []storage.Cleanup }
	json.Unmarshal(s.do(admin, "GET", "/admin/storage/cleanups", "").Body.Bytes(), &list)
	if len(list.Data) != 1 || !strings.HasSuffix(list.Data[0].Path, "Duna.epub") || !strings.Contains(list.Data[0].Reason, "could not be removed") {
		t.Fatalf("pending = %+v", list.Data)
	}
	// Retrying while it is still locked keeps it; after unlocking it finishes.
	var retry map[string]int
	json.Unmarshal(s.do(admin, "POST", "/admin/storage/cleanups/retry", "").Body.Bytes(), &retry)
	if retry["removed"] != 0 || retry["remaining"] != 1 {
		t.Errorf("retry while locked = %v", retry)
	}
	os.Chmod(locked, 0o755)
	json.Unmarshal(s.do(admin, "POST", "/admin/storage/cleanups/retry", "").Body.Bytes(), &retry)
	if retry["removed"] != 1 || retry["remaining"] != 0 {
		t.Errorf("retry after unlocking = %v", retry)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'storage.cleanup_retry'`); got != "2" {
		t.Errorf("audited retries = %s", got)
	}
}

func TestMoveToManagedEndpoint_QueuesOneTransferPerReferencedFile(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	(&storage.Scanner{DB: s.db}).Scan(context.Background(), lib, "", idAdmin)
	var ref int64
	s.db.QueryRow(`SELECT file_id FROM work_primary WHERE file_path = 'Livros/Duna.epub'`).Scan(&ref)
	managedWork := s.addWork("Gerenciada", "X", "m.epub", "epub")
	var managed int64
	s.db.QueryRow(`SELECT file_id FROM work_primary WHERE work_id = $1`, managedWork).Scan(&managed)

	rec := s.do(admin, "POST", "/admin/library/move-to-managed", fmt.Sprintf(`{"fileIds":[%d,%d,999999]}`, ref, managed))
	if rec.Code != 202 {
		t.Fatalf("move-to-managed: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data []struct {
			FileID int64
			JobID  int64
			Error  string
		}
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Data) != 3 || out.Data[0].JobID == 0 || out.Data[1].Error == "" || out.Data[2].Error == "" {
		t.Errorf("answer = %+v (only the referenced file may be queued)", out.Data)
	}
	if got := s.scalar(`SELECT count(*) || '/' || max(type) || '/' || max(priority) FROM jobs WHERE type = 'transfer'`); got != "1/transfer/10" {
		t.Errorf("jobs = %s", got)
	}
	// Asking again while it waits does not queue a second transfer of the same work.
	s.do(admin, "POST", "/admin/library/move-to-managed", fmt.Sprintf(`{"fileIds":[%d]}`, ref))
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'transfer'`); got != "1" {
		t.Errorf("transfer jobs after asking twice = %s", got)
	}
	for _, bad := range []string{`{}`, `{"fileIds":[]}`, `nope`} {
		if code := s.do(admin, "POST", "/admin/library/move-to-managed", bad).Code; code != 400 {
			t.Errorf("body %q: %d, want 400", bad, code)
		}
	}
}
