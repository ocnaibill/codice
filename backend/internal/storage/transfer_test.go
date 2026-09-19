package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/storage"
)

const anaID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

// referenced returns an environment where one book is catalogued in place, plus
// the id of its file, the absolute path of the original, and the library root.
func referenced(t *testing.T) (e *env, fileID int64, origin, root string) {
	t.Helper()
	e = newEnv(t)
	root, _ = filepath.EvalSymlinks(t.TempDir())
	origin = filepath.Join(root, "Livros", "Duna.epub")
	os.MkdirAll(filepath.Dir(origin), 0o755)
	os.WriteFile(origin, corpus(t, "epub_acentos.epub"), 0o644)
	r := scan(t, e, root, "")
	if r.Added != 1 {
		t.Fatalf("scan = %+v", r)
	}
	if err := e.db.QueryRow(`SELECT file_id FROM work_primary WHERE file_path = 'Livros/Duna.epub'`).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	// As if the worker had read the metadata.
	e.exec(`UPDATE works SET original_title = 'Duna' WHERE id = (SELECT work_id FROM work_primary WHERE file_id = $1)`, fileID)
	e.exec(`INSERT INTO person (name) VALUES ('Frank Herbert') ON CONFLICT DO NOTHING`)
	e.exec(`INSERT INTO work_contributors (work_id, person_id, role, position)
		SELECT w.id, p.id, 'author', 0 FROM works w, person p WHERE w.original_title = 'Duna' AND p.name = 'Frank Herbert'
		ON CONFLICT DO NOTHING`)
	return
}

func TestTransfer_CopiesVerifiesPublishesAndRemovesTheOriginal(t *testing.T) {
	e, fileID, origin, _ := referenced(t)
	work := e.scalar(`SELECT work_id FROM work_primary WHERE file_id = $1`, fileID)
	// Personal data on the file must survive: the file keeps its identity.
	e.exec(`INSERT INTO users (id, username, email, role) VALUES ($1, 'ana', 'a@x', 'reader')`, anaID)
	e.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, 'uma frase')`, anaID, work)
	e.exec(`INSERT INTO reading_progress (user_id, file_id, position) VALUES ($1, $2, 'cap-4')`, anaID, fileID)
	want := corpus(t, "epub_acentos.epub")

	res, err := (&storage.Transferrer{Mover: e.mover}).MoveToManaged(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OriginRemoved || res.CleanupPending || res.Target != "Frank Herbert/Duna/Duna.epub" {
		t.Fatalf("result = %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(e.root, "Frank Herbert", "Duna", "Duna.epub"))
	if string(got) != string(want) {
		t.Error("the managed copy is not the same bytes")
	}
	if _, err := os.Stat(origin); !os.IsNotExist(err) {
		t.Error("the original was not removed")
	}
	if got := e.scalar(`SELECT mode || '|' || COALESCE(root, 'NULL') || '|' || path || '|' || state FROM storage_locations WHERE file_id = $1`, fileID); got != "managed|NULL|Frank Herbert/Duna/Duna.epub|ok" {
		t.Errorf("location = %q", got)
	}
	// The same file, so notes and progress are still attached.
	if e.scalar(`SELECT count(*) FROM notes WHERE work_id = $1`, work) != "1" || e.scalar(`SELECT position FROM reading_progress WHERE file_id = $1`, fileID) != "cap-4" {
		t.Error("personal data was lost in the transfer")
	}
	if got := e.scalar(`SELECT file_path FROM work_primary WHERE work_id = $1`, work); got != "Frank Herbert/Duna/Duna.epub" {
		t.Errorf("work_primary = %q", got)
	}
	if got := e.scalar(`SELECT organized_at IS NOT NULL FROM files WHERE id = $1`, fileID); got != "true" {
		t.Error("not marked organized")
	}
	if n := e.scalar(`SELECT count(*) FROM storage_cleanups`); n != "0" {
		t.Errorf("pending cleanups = %s after a clean transfer", n)
	}
	// A second attempt is refused: it is not a referenced file any more.
	if _, err := (&storage.Transferrer{Mover: e.mover}).MoveToManaged(ctx, fileID); err != storage.ErrNotReferenced {
		t.Errorf("transferring twice: %v", err)
	}
	// The scanner no longer finds anything to add, and the directory is not disturbed.
	if _, err := os.Stat(filepath.Dir(origin)); err != nil {
		t.Error("the user's folder was removed")
	}
}

func TestTransfer_RefusesAnOriginalThatChangedBeforeTheCopy(t *testing.T) {
	e, fileID, origin, _ := referenced(t)
	os.WriteFile(origin, append(corpus(t, "epub_acentos.epub"), []byte("edited later")...), 0o644)

	_, err := (&storage.Transferrer{Mover: e.mover}).MoveToManaged(ctx, fileID)
	if err != storage.ErrSourceChanged {
		t.Fatalf("got %v, want ErrSourceChanged", err)
	}
	if _, serr := os.Stat(origin); serr != nil {
		t.Error("the original was touched")
	}
	if got := e.scalar(`SELECT mode || '|' || state FROM storage_locations WHERE file_id = $1`, fileID); got != "referenced|conflict" {
		t.Errorf("location = %q, want it flagged for review and still referenced", got)
	}
	if entries, _ := os.ReadDir(e.root); len(entries) != 0 && !(len(entries) == 1 && entries[0].Name() == ".staging") {
		t.Errorf("the storage was left with %v", entries)
	}
	if staging, _ := os.ReadDir(filepath.Join(e.root, ".staging")); len(staging) != 0 {
		t.Errorf("staging debris: %v", staging)
	}
}

func TestTransfer_KeepsAnOriginalThatChangesWhileItIsBeingMoved(t *testing.T) {
	e, fileID, origin, _ := referenced(t)
	tr := &storage.Transferrer{Mover: e.mover}
	// The person edits the original after it was read but before it could be removed.
	tr.BeforeRemoval = func() {
		os.WriteFile(origin, append(corpus(t, "epub_acentos.epub"), []byte("edited during the transfer")...), 0o644)
	}
	res, err := tr.MoveToManaged(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	if res.OriginRemoved || !res.CleanupPending {
		t.Fatalf("result = %+v: an original that changed must be kept", res)
	}
	if b, _ := os.ReadFile(origin); !strings.HasSuffix(string(b), "edited during the transfer") {
		t.Error("the person's edit was lost")
	}
	// The transfer itself stands: the library has its own good copy.
	if e.scalar(`SELECT mode FROM storage_locations WHERE file_id = $1`, fileID) != "managed" || !e.has("Frank Herbert/Duna/Duna.epub") {
		t.Error("the managed copy is missing")
	}
	if got := e.scalar(`SELECT reason FROM storage_cleanups WHERE path = $1`, origin); !strings.Contains(got, "changed") {
		t.Errorf("reason = %q", got)
	}
	// A retry does not delete it either: it is not the file that was copied.
	removed, remaining, _ := storage.RetryCleanups(ctx, e.db)
	if removed != 0 || remaining != 1 {
		t.Errorf("retry = %d removed, %d remaining", removed, remaining)
	}
	if _, err := os.Stat(origin); err != nil {
		t.Error("a retry deleted a file that had changed")
	}
}

func TestTransfer_AnOriginalThatCannotBeRemovedIsListedAndRetried(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	e, fileID, origin, _ := referenced(t)
	dir := filepath.Dir(origin)
	os.Chmod(dir, 0o555) // the folder is read-only: the file cannot be deleted
	defer os.Chmod(dir, 0o755)

	res, err := (&storage.Transferrer{Mover: e.mover}).MoveToManaged(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	if res.OriginRemoved || !res.CleanupPending {
		t.Fatalf("result = %+v", res)
	}
	// The transfer is not reported as fully done, and says why.
	pending, _ := storage.PendingCleanups(ctx, e.db)
	if len(pending) != 1 || pending[0].Path != origin || !strings.Contains(pending[0].Reason, "could not be removed") {
		t.Fatalf("pending = %+v", pending)
	}
	if e.scalar(`SELECT mode FROM storage_locations WHERE file_id = $1`, fileID) != "managed" {
		t.Error("the file was not switched to the managed storage")
	}

	// Once the folder is writable again, a retry finishes the job.
	os.Chmod(dir, 0o755)
	removed, remaining, err := storage.RetryCleanups(ctx, e.db)
	if err != nil || removed != 1 || remaining != 0 {
		t.Fatalf("retry = %d %d %v", removed, remaining, err)
	}
	if _, err := os.Stat(origin); !os.IsNotExist(err) {
		t.Error("the original is still there")
	}
	if left, _ := storage.PendingCleanups(ctx, e.db); len(left) != 0 {
		t.Errorf("the list still has %+v", left)
	}
}

func TestTransfer_NeverOverwritesAnExistingFile(t *testing.T) {
	e, fileID, _, _ := referenced(t)

	// Something already sits at the layout path, unknown to the database: the file
	// takes the id-derived name instead of replacing it.
	e.write("Frank Herbert/Duna/Duna.epub", "someone else's file")
	res, err := (&storage.Transferrer{Mover: e.mover}).MoveToManaged(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Target, "Frank Herbert/Duna/Duna [") || !strings.HasSuffix(res.Target, "].epub") {
		t.Errorf("target = %q", res.Target)
	}
	if b, _ := os.ReadFile(filepath.Join(e.root, "Frank Herbert/Duna/Duna.epub")); string(b) != "someone else's file" {
		t.Error("an existing file was overwritten")
	}
}

func TestTransfer_AMissingOrSwappedOriginalChangesNothing(t *testing.T) {
	e, fileID, origin, _ := referenced(t)
	tr := &storage.Transferrer{Mover: e.mover}

	// The original goes missing.
	saved, _ := os.ReadFile(origin)
	os.Remove(origin)
	if _, err := tr.MoveToManaged(ctx, fileID); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("missing original: %v", err)
	}

	// It is swapped for a link that leaves its root.
	secret := filepath.Join(t.TempDir(), "secret.epub")
	os.WriteFile(secret, []byte("private"), 0o644)
	os.Symlink(secret, origin)
	if _, err := tr.MoveToManaged(ctx, fileID); err != storage.ErrSourceChanged {
		t.Errorf("a link leaving the root: %v, want ErrSourceChanged", err)
	}
	if b, _ := os.ReadFile(secret); string(b) != "private" {
		t.Error("the file behind the link was touched")
	}
	if e.scalar(`SELECT mode FROM storage_locations WHERE file_id = $1`, fileID) != "referenced" {
		t.Error("a refused transfer changed the location")
	}
	if _, err := tr.MoveToManaged(ctx, 999999); err != storage.ErrNotReferenced {
		t.Errorf("an unknown file: %v", err)
	}
	_ = saved
}
