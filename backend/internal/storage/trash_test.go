package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/storage"
)

func trashEnv(t *testing.T) (*env, *storage.Trash) {
	e := newEnv(t)
	return e, &storage.Trash{DB: e.db, Root: e.root}
}

func TestTrash_SendingAFileKeepsItsRecordAndRestoringBringsItBack(t *testing.T) {
	e, tr := trashEnv(t)
	work, file := e.addWork("Duna", "Frank Herbert", "Frank Herbert/Duna/Duna.epub", "epub", "", "", "")
	e.exec(`INSERT INTO users (id, username, email, role) VALUES ($1, 'ana', 'a@x', 'reader')`, anaID)
	e.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, 'uma frase')`, anaID, work)
	e.exec(`INSERT INTO reading_progress (user_id, file_id, position) VALUES ($1, $2, 'cap-2')`, anaID, file)

	if err := tr.TrashFile(ctx, file, ""); err != nil {
		t.Fatal(err)
	}
	if e.has("Frank Herbert/Duna/Duna.epub") || !e.has(".trash/"+itoa64(file)+"/Duna.epub") {
		t.Fatal("the bytes did not move to the trash folder")
	}
	// The record stays: nothing is gone for good.
	if got := e.scalar(`SELECT l.state || '|' || f.availability FROM storage_locations l JOIN files f ON f.id = l.file_id WHERE f.id = $1`, file); got != "trashed|missing" {
		t.Errorf("state = %q", got)
	}
	if e.scalar(`SELECT count(*) FROM works WHERE id = $1`, work) != "1" || e.scalar(`SELECT count(*) FROM notes WHERE work_id = $1`, work) != "1" {
		t.Error("the work or its notes were affected by trashing a file")
	}
	items, total, _ := tr.List(ctx)
	if len(items) != 1 || items[0].WorkTitle != "Duna" || total != int64(len("content of Frank Herbert/Duna/Duna.epub")) {
		t.Errorf("list = %+v total=%d", items, total)
	}
	// Trashed files are not served (the catalog only serves locations that are 'ok').
	if e.scalar(`SELECT count(*) FROM storage_locations WHERE file_id = $1 AND state = 'ok'`, file) != "0" {
		t.Error("a trashed file still counts as available")
	}
	// A second attempt has nothing to trash.
	if err := tr.TrashFile(ctx, file, ""); err != storage.ErrNotTrashable {
		t.Errorf("trashing twice: %v", err)
	}

	target, err := tr.Restore(ctx, items[0].ID)
	if err != nil || target != "Frank Herbert/Duna/Duna.epub" {
		t.Fatalf("restore: %q %v", target, err)
	}
	if !e.has(target) || e.has(".trash") {
		t.Error("the file is not back, or the trash folder was left behind")
	}
	if got := e.scalar(`SELECT l.state || '|' || f.availability FROM storage_locations l JOIN files f ON f.id = l.file_id WHERE f.id = $1`, file); got != "ok|available" {
		t.Errorf("after restore = %q", got)
	}
	if e.scalar(`SELECT position FROM reading_progress WHERE file_id = $1`, file) != "cap-2" {
		t.Error("restoring lost the reading position")
	}
	if left, _, _ := tr.List(ctx); len(left) != 0 {
		t.Errorf("the item is still listed: %+v", left)
	}
}

func itoa64(n int64) string { return strings.TrimSpace(strings.Repeat(" ", 0) + fmtInt(n)) }

func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestTrash_RestoreNeverOverwritesWhatTookItsPlace(t *testing.T) {
	e, tr := trashEnv(t)
	_, file := e.addWork("Duna", "X", "X/Duna.epub", "epub", "", "", "")
	tr.TrashFile(ctx, file, "")
	// Someone put another file where it used to be.
	e.write("X/Duna.epub", "the newcomer")

	items, _, _ := tr.List(ctx)
	target, err := tr.Restore(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if target == "X/Duna.epub" || !strings.HasPrefix(target, "X/Duna [") {
		t.Errorf("restored over the newcomer: %q", target)
	}
	if b, _ := os.ReadFile(filepath.Join(e.root, "X/Duna.epub")); string(b) != "the newcomer" {
		t.Error("the file that took its place was overwritten")
	}
	if got := e.scalar(`SELECT path FROM storage_locations WHERE file_id = $1`, file); got != target {
		t.Errorf("the location = %q, want %q", got, target)
	}
}

func TestTrash_DeletingForGoodIsTheOnlyPlaceBytesAreDestroyed(t *testing.T) {
	e, tr := trashEnv(t)
	work, file := e.addWork("Duna", "X", "X/Duna.epub", "epub", "", "", "")
	e.exec(`INSERT INTO users (id, username, email, role) VALUES ($1, 'ana', 'a@x', 'reader')`, anaID)
	e.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, 'uma frase')`, anaID, work)
	e.exec(`UPDATE editions SET cover_url = '/covers/duna.jpg' WHERE work_id = $1`, work)
	e.write("covers/duna.jpg", "jpeg")

	tr.TrashFile(ctx, file, "")
	items, _, _ := tr.List(ctx)

	// A work that is still in the catalog keeps its record even if its last file is deleted.
	if _, err := tr.Delete(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
	if e.has(".trash") {
		t.Error("the bytes were not destroyed")
	}
	if e.scalar(`SELECT count(*) FROM works WHERE id = $1`, work) != "1" || e.scalar(`SELECT count(*) FROM files WHERE id = $1`, file) != "0" {
		t.Error("an active work must keep its record; only the file record goes")
	}

	// A RETIRED work whose last file is deleted is deleted too; its notes survive with their reference.
	work2, file2 := e.addWork("Outra", "Y", "Y/Outra.epub", "epub", "", "", "")
	e.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, 'da outra')`, anaID, work2)
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work2)
	e.exec(`UPDATE editions SET cover_url = '/covers/outra.jpg' WHERE work_id = $1`, work2)
	e.write("covers/outra.jpg", "jpeg")
	tr.TrashFile(ctx, file2, "")
	items, _, _ = tr.List(ctx)
	if _, err := tr.Delete(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
	if e.scalar(`SELECT count(*) FROM works WHERE id = $1`, work2) != "0" {
		t.Error("a retired work with no files left should be deleted")
	}
	if got := e.scalar(`SELECT count(*) FROM notes WHERE quote = 'da outra' AND work_id IS NULL AND source_title = 'Outra'`); got != "1" {
		t.Errorf("the note did not survive with its reference: %s", got)
	}
	if e.has("covers/outra.jpg") || !e.has("covers/duna.jpg") {
		t.Error("a deleted work's cover must go, another work's must stay")
	}
	if _, err := tr.Delete(ctx, 999999); err != storage.ErrNoSuchItem {
		t.Errorf("unknown item: %v", err)
	}
}

func TestTrash_ReferencedFilesAreNeverTrashed(t *testing.T) {
	e, tr := trashEnv(t)
	root := library(t)
	scan(t, e, root, "")
	var id int64
	e.db.QueryRow(`SELECT file_id FROM work_primary LIMIT 1`).Scan(&id)
	if err := tr.TrashFile(ctx, id, ""); err != storage.ErrNotTrashable {
		t.Errorf("trashing a referenced file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Livros", "Duna.epub")); err != nil && !os.IsNotExist(err) {
		t.Error(err)
	}
}

func TestTrash_AutomaticCleanupIsOffByDefaultAndOnlyAppliesToNewItems(t *testing.T) {
	e, tr := trashEnv(t)
	p, _ := tr.GetPolicy(ctx)
	if p.Enabled || p.Days != 30 {
		t.Fatalf("default policy = %+v, want off with 30 days ready", p)
	}
	_, a := e.addWork("A", "X", "X/A.epub", "epub", "", "", "")
	_, b := e.addWork("B", "X", "X/B.epub", "epub", "", "", "")
	_, c := e.addWork("C", "X", "X/C.epub", "epub", "", "", "")

	// Disabled: nothing ever expires.
	tr.TrashFile(ctx, a, "")
	if e.scalar(`SELECT purge_after IS NULL FROM trash_items WHERE file_id = $1`, a) != "true" {
		t.Error("an item got an expiry while the cleanup was off")
	}

	// Enabling it does not touch what is already in the trash (DEC-043)...
	if err := tr.SetPolicy(ctx, storage.Policy{Enabled: true, Days: 7}, ""); err != nil {
		t.Fatal(err)
	}
	if e.scalar(`SELECT purge_after IS NULL FROM trash_items WHERE file_id = $1`, a) != "true" {
		t.Error("enabling the policy changed an existing item")
	}
	// ...but new items get their date counted from their own entry.
	tr.TrashFile(ctx, b, "")
	if got := e.scalar(`SELECT (purge_after - trashed_at) BETWEEN interval '6 days 23 hours' AND interval '7 days 1 hour' FROM trash_items WHERE file_id = $1`, b); got != "true" {
		t.Errorf("expiry is not entry + 7 days")
	}
	// Changing the number of days does not move the date of existing items either.
	tr.SetPolicy(ctx, storage.Policy{Enabled: true, Days: 1}, "")
	if got := e.scalar(`SELECT (purge_after - trashed_at) > interval '6 days' FROM trash_items WHERE file_id = $1`, b); got != "true" {
		t.Error("shortening the policy rewrote an existing item's date")
	}
	if err := tr.SetPolicy(ctx, storage.Policy{Enabled: true, Days: 0}, ""); err == nil {
		t.Error("a policy of zero days was accepted")
	}

	// Applying it to the existing items is a separate, explicit action, with a preview
	// that counts what is already due.
	e.exec(`UPDATE trash_items SET trashed_at = now() - interval '40 days' WHERE file_id = $1`, a)
	prev, _ := tr.PreviewPolicy(ctx, 30)
	if prev.Items != 2 || prev.AlreadyDue != 1 {
		t.Errorf("preview = %+v", prev)
	}
	n, _ := tr.ApplyPolicy(ctx, 30)
	if n != 2 {
		t.Errorf("applied to %d", n)
	}
	// The sweep deletes what is due, and only that; an item restored meanwhile is safe.
	tr.TrashFile(ctx, c, "")
	e.exec(`UPDATE trash_items SET purge_after = now() - interval '1 hour' WHERE file_id = $1`, c)
	items, _, _ := tr.List(ctx)
	for _, it := range items {
		if it.FileID != nil && *it.FileID == c {
			tr.Restore(ctx, it.ID) // restored just before the sweep
		}
	}
	removed, err := tr.PurgeExpired(ctx)
	if err != nil || removed != 1 {
		t.Fatalf("sweep removed %d (%v), want only the one past due", removed, err)
	}
	if e.scalar(`SELECT count(*) FROM trash_items`) != "1" || !e.has("X/C.epub") {
		t.Error("the sweep touched an item that was not due or was restored")
	}
}

func TestTrash_OrphansAreListedAndTrashedNotDeleted(t *testing.T) {
	e, tr := trashEnv(t)
	e.addWork("Known", "X", "X/Known.epub", "epub", "", "", "")
	e.write("stray-old.epub", "left by a crash")
	e.write("covers/c.jpg", "cover")
	e.write("cache/x", "page cache")
	e.write("stray-new.epub", "being ingested right now")
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(e.root, "stray-old.epub"), old, old)
	os.Chtimes(filepath.Join(e.root, "X", "Known.epub"), old, old)

	orphans, err := tr.FindOrphans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans[0].Path != "stray-old.epub" {
		t.Fatalf("orphans = %+v (a known file, covers, cache and files younger than a day are not orphans)", orphans)
	}

	// Only paths that are still orphans are moved; anything else named is ignored.
	moved, err := tr.TrashOrphans(ctx, []string{"stray-old.epub", "X/Known.epub", "../escape"}, "")
	if err != nil || moved != 1 {
		t.Fatalf("moved %d (%v)", moved, err)
	}
	if e.has("stray-old.epub") || !e.has("X/Known.epub") {
		t.Error("the orphan stayed, or a known file was moved")
	}
	items, _, _ := tr.List(ctx)
	if len(items) != 1 || items[0].Kind != "orphan" || items[0].OriginalPath != "stray-old.epub" {
		t.Errorf("trash = %+v", items)
	}
	// It is recoverable like any other deletion.
	if target, err := tr.Restore(ctx, items[0].ID); err != nil || target != "stray-old.epub" || !e.has("stray-old.epub") {
		t.Errorf("restoring an orphan: %q %v", target, err)
	}
}
