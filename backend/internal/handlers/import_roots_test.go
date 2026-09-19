package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importFixture lays out a storage dir, its default import root, an extra
// allowed root, and a directory that is NOT allowed.
type importFixture struct {
	storage, defaultRoot, extraRoot, outside string
}

func newImportFixture(t *testing.T) importFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := importFixture{
		storage:     filepath.Join(base, "storage"),
		defaultRoot: filepath.Join(base, "storage", "import"),
		extraRoot:   filepath.Join(base, "library"),
		outside:     filepath.Join(base, "secrets"),
	}
	for _, d := range []string{f.defaultRoot, f.extraRoot, f.outside, filepath.Join(f.extraRoot, "sci-fi")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CODICE_STORAGE_PATH", f.storage)
	t.Setenv("CODICE_IMPORT_ROOTS", f.extraRoot)
	return f
}

func TestResolveImportDir(t *testing.T) {
	f := newImportFixture(t)
	roots := importRoots(f.storage)

	// A symlink inside an allowed root that points outside must not be a way out.
	escape := filepath.Join(f.extraRoot, "escape")
	if err := os.Symlink(f.outside, escape); err != nil {
		t.Fatal(err)
	}
	// A sibling whose name merely starts with the root's name is not inside it.
	evil := f.defaultRoot + "-evil"
	os.MkdirAll(evil, 0o755)

	allowed := map[string]string{
		"empty means the default root": "",
		"the default root itself":      f.defaultRoot,
		"an extra root":                f.extraRoot,
		"a subdirectory of a root":     filepath.Join(f.extraRoot, "sci-fi"),
		"relative to the default root": ".",
	}
	for name, req := range allowed {
		if _, err := resolveImportDir(req, roots); err != nil {
			t.Errorf("%s (%q): unexpected error %v", name, req, err)
		}
	}

	denied := map[string]string{
		"absolute path outside every root": f.outside,
		"the filesystem root":              string(filepath.Separator),
		"parent of a root":                 f.storage,
		"dot-dot escape":                   filepath.Join(f.defaultRoot, "..", "..", "secrets"),
		"relative dot-dot escape":          "../../secrets",
		"sibling sharing the name prefix":  evil,
		"symlink leading outside":          escape,
	}
	for name, req := range denied {
		_, err := resolveImportDir(req, roots)
		if !errors.Is(err, errOutsideImportRoots) {
			t.Errorf("%s (%q): got %v, want errOutsideImportRoots", name, req, err)
		}
	}

	if _, err := resolveImportDir(filepath.Join(f.extraRoot, "missing"), roots); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing directory inside a root: got %v, want ErrNotExist", err)
	}
}

func TestImportRoots_FromEnvironment(t *testing.T) {
	f := newImportFixture(t)

	t.Setenv("CODICE_IMPORT_ROOTS", strings.Join([]string{f.extraRoot, "", "relative/ignored", f.outside}, string(os.PathListSeparator)))
	roots := importRoots(f.storage)

	if roots[0] != f.defaultRoot {
		t.Errorf("first root = %q, want the default import dir", roots[0])
	}
	got := strings.Join(roots, "|")
	if !strings.Contains(got, f.extraRoot) || !strings.Contains(got, f.outside) {
		t.Errorf("configured roots missing: %v", roots)
	}
	if strings.Contains(got, "relative") || len(roots) != 3 {
		t.Errorf("relative and empty entries must be ignored: %v", roots)
	}

	t.Setenv("CODICE_IMPORT_ROOTS", "")
	if roots := importRoots(f.storage); len(roots) != 1 {
		t.Errorf("without configuration only the default root exists: %v", roots)
	}
}

func bulkImport(dir string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(BulkImportRequest{Directory: dir})
	rec := httptest.NewRecorder()
	// No database or Redis: every case below must end before touching them.
	(&UploadHandler{}).HandleBulkImport(rec, httptest.NewRequest("POST", "/works/bulk-import", strings.NewReader(string(body))))
	return rec
}

func TestBulkImport_RefusesDirectoriesOutsideTheRoots(t *testing.T) {
	f := newImportFixture(t)
	os.WriteFile(filepath.Join(f.outside, "secret.pdf"), []byte("%PDF-1.4"), 0o644)

	for _, dir := range []string{f.outside, "/etc", string(filepath.Separator), f.storage} {
		if rec := bulkImport(dir); rec.Code != http.StatusForbidden {
			t.Errorf("%q: got %d, want 403", dir, rec.Code)
		}
	}
}

func TestBulkImport_DoesNotCreateDirectoriesItWasAskedFor(t *testing.T) {
	f := newImportFixture(t)
	missing := filepath.Join(f.extraRoot, "created-by-request")

	if rec := bulkImport(missing); rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("a requested directory was created on the server")
	}
}

func TestBulkImport_CreatesOnlyTheDefaultImportDirectory(t *testing.T) {
	f := newImportFixture(t)
	os.RemoveAll(f.defaultRoot)

	rec := bulkImport("")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(f.defaultRoot); err != nil {
		t.Errorf("default import directory was not created: %v", err)
	}
}

func TestBulkImport_SkipsSymlinkedFiles(t *testing.T) {
	f := newImportFixture(t)
	secret := filepath.Join(f.outside, "shadow.txt")
	os.WriteFile(secret, []byte("root:x:0:0"), 0o600)
	// Looks like an importable file, but is a way to read something else.
	if err := os.Symlink(secret, filepath.Join(f.defaultRoot, "innocent.pdf")); err != nil {
		t.Fatal(err)
	}

	rec := bulkImport(f.defaultRoot)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var resp BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Scanned != 0 || resp.Enqueued != 0 {
		t.Errorf("a symlink was imported: %+v", resp)
	}
	entries, _ := os.ReadDir(f.storage)
	for _, e := range entries {
		if e.Name() != "import" {
			t.Errorf("unexpected file copied into storage: %s", e.Name())
		}
	}
}
