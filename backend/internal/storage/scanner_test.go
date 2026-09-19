package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/storage"
)

func corpus(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "corpus", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// library builds a directory of the synthetic corpus, as a person's own library.
func library(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	put := func(rel, from string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, corpus(t, from), 0o644)
	}
	put("Livros/Duna.epub", "epub_acentos.epub")
	put("Livros/Cópia de Duna.epub", "epub_duplicata.epub") // identical bytes, another name
	put("Quadrinhos/Watchmen 01.cbz", "cbz_ltr.cbz")
	put("Quadrinhos/Ação e Reação/02.cbz", "cbz_rtl.cbz")
	put("Documentos/manual.pdf", "pdf_digital.pdf")
	put("Livros/quebrado.epub", "epub_corrompido.epub") // not what it says
	put("Livros/falso.pdf", "falso.pdf")                // not what it says
	put("Livros/capa.jpg", "notas.txt")                 // unsupported extension: ignored
	return dir
}

func scan(t *testing.T, e *env, root, subdir string) *storage.ScanReport {
	t.Helper()
	r, err := (&storage.Scanner{DB: e.db}).Scan(ctx, root, subdir, "")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return r
}

func TestScan_CataloguesFilesWhereTheyAreAndTouchesNothing(t *testing.T) {
	e := newEnv(t)
	root := library(t)

	before := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			before[p] = info.ModTime().String() + "|" + string(b)
		}
		return nil
	})

	r := scan(t, e, root, "")
	// 7 supported files: 4 valid and distinct, 1 duplicate, 2 that are not what they claim.
	if r.Scanned != 7 || r.Added != 4 || r.Duplicates != 1 || r.Rejected != 2 || r.Missing != 0 {
		t.Fatalf("report = %+v", r)
	}

	// Nothing was moved, renamed, rewritten or added to the user's directory.
	after := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			after[p] = info.ModTime().String() + "|" + string(b)
		}
		return nil
	})
	if len(after) != len(before) {
		t.Errorf("the library changed: %d files before, %d after", len(before), len(after))
	}
	for p, v := range before {
		if after[p] != v {
			t.Errorf("%s was modified", p)
		}
	}

	// Each accepted file is one work with a referenced location and its hash.
	if got := e.scalar(`SELECT count(*) FROM storage_locations WHERE mode = 'referenced' AND root = $1 AND state = 'ok'`, root); got != "4" {
		t.Errorf("referenced locations = %s", got)
	}
	if got := e.scalar(`SELECT count(*) FROM storage_locations WHERE mode = 'managed'`); got != "0" {
		t.Errorf("a managed location was created: %s", got)
	}
	if got := e.scalar(`SELECT count(*) FROM files WHERE sha256 IS NOT NULL AND size_bytes > 0`); got != "4" {
		t.Errorf("files with hash and size = %s", got)
	}
	if got := e.scalar(`SELECT path FROM storage_locations WHERE path LIKE '%Reação%'`); got != "Quadrinhos/Ação e Reação/02.cbz" {
		t.Errorf("a nested path with accents = %q", got)
	}
	// Of two identical files the first in directory order is catalogued (deterministic).
	// The name in the directory is a placeholder title; the worker replaces it after reading the file.
	if got := e.scalar(`SELECT original_title FROM works WHERE id = (SELECT work_id FROM work_primary WHERE file_path = 'Livros/Cópia de Duna.epub')`); got != "Cópia de Duna.epub" {
		t.Errorf("placeholder title = %q", got)
	}
	if got := e.scalar(`SELECT count(*) FROM storage_locations WHERE path = 'Livros/Duna.epub'`); got != "0" {
		t.Error("the duplicate was catalogued too")
	}
	// One ingestion job per work, on the original path, at batch priority.
	if got := e.scalar(`SELECT count(*) || '/' || min(priority) || '/' || max(priority) FROM jobs WHERE type = 'ingest'`); got != "4/0/0" {
		t.Errorf("jobs = %s", got)
	}
	if got := e.scalar(`SELECT payload->>'file_path' FROM jobs WHERE payload->>'file_path' LIKE '%Watchmen 01.cbz'`); got != filepath.Join(root, "Quadrinhos", "Watchmen 01.cbz") {
		t.Errorf("job path = %q", got)
	}

	// A second scan finds nothing new.
	again := scan(t, e, root, "")
	if again.Added != 0 || again.Duplicates != 1 || again.Missing != 0 || again.Changed != 0 {
		t.Errorf("second scan = %+v", again)
	}
	if got := e.scalar(`SELECT count(*) FROM works`); got != "4" {
		t.Errorf("works after two scans = %s, want 4", got)
	}
	// The scan is audited.
	if got := e.scalar(`SELECT count(*) FROM audit_log WHERE action = 'library.scan'`); got != "2" {
		t.Errorf("audit entries = %s", got)
	}
}

func TestScan_SkipsSymlinksAndDuplicatesOfManagedFiles(t *testing.T) {
	e := newEnv(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("private data"), 0o644)
	os.Symlink(secret, filepath.Join(root, "innocent.txt"))
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "elsewhere.epub"), corpus(t, "epub_alterado.epub"), 0o644)
	os.Symlink(outside, filepath.Join(root, "linked-dir"))

	// The bytes of a book the library already manages.
	os.WriteFile(filepath.Join(root, "Duna.epub"), corpus(t, "epub_acentos.epub"), 0o644)
	_, file := e.addWork("Duna", "Frank Herbert", "ab_Duna.epub", "epub", "", "", "")
	e.exec(`UPDATE files SET sha256 = (SELECT encode(sha256($1::bytea), 'hex')) WHERE id = $2`, corpus(t, "epub_acentos.epub"), file)

	r := scan(t, e, root, "")
	if r.Scanned != 1 || r.Added != 0 || r.Duplicates != 1 {
		t.Errorf("report = %+v (a symlink must not be followed, and bytes already stored are a duplicate)", r)
	}
	if got := e.scalar(`SELECT count(*) FROM storage_locations WHERE mode = 'referenced'`); got != "0" {
		t.Errorf("referenced locations = %s", got)
	}
}

func TestScan_MissingReappearedAndChanged(t *testing.T) {
	e := newEnv(t)
	root := library(t)
	scan(t, e, root, "")
	watch := "Quadrinhos/Watchmen 01.cbz"
	work := e.scalar(`SELECT work_id FROM work_primary WHERE file_path = $1`, watch)
	fileID := e.scalar(`SELECT file_id FROM work_primary WHERE file_path = $1`, watch)

	// Personal data attached to the file: it must survive the file going away.
	e.exec(`INSERT INTO users (id, username, email, role) VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'ana', 'a@x', 'reader')`)
	e.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', $1, 'uma frase')`, work)
	e.exec(`INSERT INTO reading_progress (user_id, file_id, position) VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', $1, 'p-3')`, fileID)

	// The file disappears.
	p := filepath.Join(root, filepath.FromSlash(watch))
	saved, _ := os.ReadFile(p)
	os.Remove(p)
	r := scan(t, e, root, "")
	if r.Missing != 1 {
		t.Fatalf("report = %+v", r)
	}
	if got := e.scalar(`SELECT l.state || '/' || f.availability FROM storage_locations l JOIN files f ON f.id = l.file_id WHERE l.path = $1`, watch); got != "missing/missing" {
		t.Errorf("state = %q", got)
	}
	if e.scalar(`SELECT count(*) FROM notes WHERE work_id = $1`, work) != "1" || e.scalar(`SELECT count(*) FROM reading_progress WHERE file_id = $1`, fileID) != "1" {
		t.Error("a missing file must keep its notes and progress")
	}
	if got := e.scalar(`SELECT count(*) FROM works WHERE id = $1`, work); got != "1" {
		t.Error("the work was deleted when its file went away")
	}

	// It comes back.
	os.WriteFile(p, saved, 0o644)
	r = scan(t, e, root, "")
	if r.Reappeared != 1 || r.Missing != 0 {
		t.Fatalf("report = %+v", r)
	}
	if got := e.scalar(`SELECT l.state || '/' || f.availability FROM storage_locations l JOIN files f ON f.id = l.file_id WHERE l.path = $1`, watch); got != "ok/available" {
		t.Errorf("state after it came back = %q", got)
	}

	// Its content changes (a different file is put under the same name).
	os.WriteFile(p, append(saved, []byte("EXTRA BYTES THAT CHANGE THE FILE")...), 0o644)
	r = scan(t, e, root, "")
	if r.Changed != 1 {
		t.Fatalf("report = %+v", r)
	}
	if got := e.scalar(`SELECT state FROM storage_locations WHERE path = $1`, watch); got != "conflict" {
		t.Errorf("a changed file must wait for review: %q", got)
	}
	if e.scalar(`SELECT count(*) FROM works`) != "4" {
		t.Error("a changed file must not create a new work behind the scenes")
	}
	// Scanning again does not flip it back or report it again.
	if again := scan(t, e, root, ""); again.Changed != 0 || again.Reappeared != 0 {
		t.Errorf("repeat = %+v", again)
	}
}

func TestScan_ASubdirectoryOnlyJudgesItsOwnFiles(t *testing.T) {
	e := newEnv(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	// "a_b" and "axb": '_' is a wildcard in SQL LIKE and must not make them look alike.
	for _, d := range []string{"a_b", "axb", "other"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "a_b", "one.epub"), corpus(t, "epub_acentos.epub"), 0o644)
	os.WriteFile(filepath.Join(root, "axb", "two.epub"), corpus(t, "epub_alterado.epub"), 0o644)
	os.WriteFile(filepath.Join(root, "other", "three.pdf"), corpus(t, "pdf_digital.pdf"), 0o644)
	if r := scan(t, e, root, ""); r.Added != 3 {
		t.Fatalf("setup scan = %+v", r)
	}

	os.Remove(filepath.Join(root, "axb", "two.epub"))
	os.Remove(filepath.Join(root, "other", "three.pdf"))

	// Scanning only a_b must judge only a_b.
	r := scan(t, e, root, "a_b")
	if r.Missing != 0 {
		t.Errorf("scanning a_b marked %d files missing that belong to other directories", r.Missing)
	}
	if got := e.scalar(`SELECT count(*) FROM storage_locations WHERE state = 'missing'`); got != "0" {
		t.Errorf("missing locations after scanning a_b = %s", got)
	}
	// Scanning the directory that lost a file marks exactly that file.
	if r := scan(t, e, root, "axb"); r.Missing != 1 {
		t.Errorf("scanning axb: %+v", r)
	}
	if got := e.scalar(`SELECT path FROM storage_locations WHERE state = 'missing'`); got != "axb/two.epub" {
		t.Errorf("missing = %q", got)
	}
}

func TestScan_RefusesPathsOutsideTheRootAndStopsWhenCancelled(t *testing.T) {
	e := newEnv(t)
	root := library(t)
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "escape"))

	for _, bad := range []string{"../elsewhere", "/etc", "Livros/../../x", "escape"} {
		if _, err := (&storage.Scanner{DB: e.db}).Scan(ctx, root, bad, ""); err == nil {
			t.Errorf("subdir %q was accepted", bad)
		}
	}
	if _, err := (&storage.Scanner{DB: e.db}).Scan(ctx, filepath.Join(root, "does-not-exist"), "", ""); err == nil {
		t.Error("a missing root was accepted")
	}

	// A cancelled scan reports the cancellation and does not go marking things missing.
	scan(t, e, root, "")
	os.Remove(filepath.Join(root, "Documentos", "manual.pdf"))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := (&storage.Scanner{DB: e.db}).Scan(cancelled, root, "", ""); err == nil {
		t.Error("a cancelled scan reported success")
	}
	if got := e.scalar(`SELECT count(*) FROM storage_locations WHERE state = 'missing'`); got != "0" {
		t.Errorf("a cancelled scan marked %s files missing", got)
	}
}
