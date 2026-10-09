package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// referenceIn makes the work's primary file one that lives in a directory the owner authorised, instead of in the managed storage.
func (s *catalogStack) referenceIn(work int, root, rel string) {
	s.t.Helper()
	s.exec(`UPDATE storage_locations SET mode = 'referenced', root = $1, path = $2 WHERE file_id = $3`, root, rel, s.primaryFile(work))
}

// referencedWork is a work whose only file is in the directory.
func (s *catalogStack) referencedWork(root, title string) int {
	s.t.Helper()
	id := s.addWork(title, "Someone", title+".epub", "epub")
	s.referenceIn(id, root, title+".epub")
	return id
}

func (s *catalogStack) rootOf(path string) StorageRoot {
	s.t.Helper()
	var list struct {
		Roots []StorageRoot `json:"roots"`
	}
	if err := json.Unmarshal(s.do(admin, "GET", "/admin/storage/roots", "").Body.Bytes(), &list); err != nil {
		s.t.Fatal(err)
	}
	for _, r := range list.Roots {
		if r.Path == path {
			return r
		}
	}
	s.t.Fatalf("no root %q in %+v", path, list.Roots)
	return StorageRoot{}
}

type purgeAnswer struct {
	Purged    int `json:"purged"`
	Remaining int `json:"remaining"`
}

func (s *catalogStack) purgeRetired(root StorageRoot) purgeAnswer {
	s.t.Helper()
	rec := s.do(admin, "POST", fmt.Sprintf("/admin/storage/roots/%d/purge-retired", root.ID), "")
	if rec.Code != 200 {
		s.t.Fatalf("purge-retired: %d %s", rec.Code, rec.Body.String())
	}
	var out purgeAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func authorisedDir(t *testing.T, s *catalogStack) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if rec := s.addRoot(admin, dir); rec.Code != 201 {
		t.Fatalf("authorising %s: %d %s", dir, rec.Code, rec.Body.String())
	}
	return dir
}

func TestRoots_EachOneSaysWhatTheCatalogKeepsInItAndWhatOfThatIsRetired(t *testing.T) {
	s := newCatalogStack(t)
	dir := authorisedDir(t, s)
	empty := authorisedDir(t, s)
	s.referencedWork(dir, "Active")
	gone := s.referencedWork(dir, "Gone")
	// A retired work with two files in the directory counts one work and two files.
	two := s.referencedWork(dir, "Two")
	second := s.addFile(int(s.primaryEdition(two)), "pdf", "Two.pdf", "managed")
	s.exec(`UPDATE storage_locations SET mode = 'referenced', root = $1 WHERE file_id = $2`, dir, second)
	// A retired work that also keeps a file the server stores is not purgeable from here: that file is not in the directory.
	mixed := s.referencedWork(dir, "Mixed")
	s.addFile(int(s.primaryEdition(mixed)), "pdf", "Mixed.pdf", "managed")
	for _, id := range []int{gone, two, mixed} {
		s.retire(id)
	}

	got := s.rootOf(dir)
	want := StorageRoot{Path: dir, Files: 1, RetiredFiles: 4, RetiredWorks: 3, PurgeableWorks: 2}
	got.ID = 0
	if got != want {
		t.Errorf("the directory = %+v, want %+v", got, want)
	}
	if e := s.rootOf(empty); e.Files != 0 || e.RetiredFiles != 0 || e.RetiredWorks != 0 || e.PurgeableWorks != 0 {
		t.Errorf("an empty directory = %+v", e)
	}
}

func TestRoots_RemovingOneThatStillHasFilesSaysWhichAreInTheCatalogAndWhichAreFromRetiredWorks(t *testing.T) {
	s := newCatalogStack(t)
	dir := authorisedDir(t, s)
	root := s.rootOf(dir)
	remove := func() (int, string) {
		rec := s.do(admin, "DELETE", fmt.Sprintf("/admin/storage/roots/%d", root.ID), "")
		return rec.Code, strings.TrimSpace(rec.Body.String())
	}

	s.referencedWork(dir, "Active")
	gone := s.referencedWork(dir, "Gone")
	other := s.referencedWork(dir, "Other")
	s.retire(gone)
	s.retire(other)
	if code, msg := remove(); code != 409 || msg != "Files in this directory are still catalogued: 1 in the catalog, 2 from retired works" {
		t.Errorf("both kinds: %d %q", code, msg)
	}
	s.exec(`DELETE FROM works WHERE original_title = 'Active'`)
	if code, msg := remove(); code != 409 || msg != "Files in this directory are still catalogued: 0 in the catalog, 2 from retired works" {
		t.Errorf("only retired: %d %q", code, msg)
	}
}

func TestPurgeRetired_DeletesOnlyTheRecordsOfRetiredWorksThatKeepNoBytesAndTouchesNoFile(t *testing.T) {
	s := newCatalogStack(t)
	dir := authorisedDir(t, s)
	elsewhere := authorisedDir(t, s)
	onDisk := filepath.Join(dir, "Gone.epub")
	if err := os.WriteFile(onDisk, []byte("the bytes of the owner"), 0o644); err != nil {
		t.Fatal(err)
	}

	active := s.referencedWork(dir, "Active")
	gone := s.referencedWork(dir, "Gone")
	mixed := s.referencedWork(dir, "Mixed")
	s.addFile(int(s.primaryEdition(mixed)), "pdf", "Mixed.pdf", "managed")
	away := s.referencedWork(elsewhere, "Away")
	notRetired := s.referencedWork(dir, "Not retired")
	for _, id := range []int{gone, mixed, away} {
		s.retire(id)
	}
	// The notes of a deleted work stay, without the work.
	s.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, 'a thought')`, idAdmin, gone)

	got := s.purgeRetired(s.rootOf(dir))
	if got.Purged != 1 || got.Remaining != 0 {
		t.Errorf("answer = %+v, want 1 purged and none waiting", got)
	}
	exists := func(id int) bool { return s.scalar(`SELECT count(*) FROM works WHERE id = $1`, id) == "1" }
	for name, c := range map[string]struct {
		id   int
		kept bool
	}{
		"in the catalog":                        {active, true},
		"not retired":                           {notRetired, true},
		"retired, keeping a file on the server": {mixed, true},
		"retired, in another directory":         {away, true},
		"retired, nothing on the server":        {gone, false},
	} {
		if exists(c.id) != c.kept {
			t.Errorf("%s: exists = %v, want %v", name, exists(c.id), c.kept)
		}
	}
	if _, err := os.Stat(onDisk); err != nil {
		t.Errorf("the owner's file in the directory was touched: %v", err)
	}
	if n := s.scalar(`SELECT count(*) FROM trash_items`); n != "0" {
		t.Errorf("%s file(s) went to the trash; nothing should", n)
	}
	if n := s.scalar(`SELECT count(*) FROM notes WHERE quote = 'a thought'`); n != "1" {
		t.Errorf("the note was lost: %s", n)
	}
	if a := s.scalar(`SELECT details->>'purged' FROM audit_log WHERE action = 'storage.retired_purge'`); a != "1" {
		t.Errorf("audit says purged = %q", a)
	}
	// Nothing left to delete: asking again changes nothing and leaves no entry.
	if again := s.purgeRetired(s.rootOf(dir)); again.Purged != 0 || again.Remaining != 0 {
		t.Errorf("a second call = %+v", again)
	}
	if n := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'storage.retired_purge'`); n != "1" {
		t.Errorf("%s audit entries", n)
	}
}

func TestPurgeRetired_WorksThroughAFolderOfThousandsInBatchesAndThenTheFolderCanBeRemoved(t *testing.T) {
	was := purgeBatch
	purgeBatch = 3
	t.Cleanup(func() { purgeBatch = was })
	s := newCatalogStack(t)
	dir := authorisedDir(t, s)
	// Works of the catalog come first by number: a batch that took them would use itself up on works it must not delete.
	for i := 0; i < 4; i++ {
		s.referencedWork(dir, fmt.Sprintf("Active %d", i))
	}
	for i := 0; i < 7; i++ {
		s.retire(s.referencedWork(dir, fmt.Sprintf("Book %03d", i)))
	}
	root := s.rootOf(dir)
	if root.PurgeableWorks != 7 {
		t.Fatalf("purgeable = %d, want 7", root.PurgeableWorks)
	}
	for i, want := range []purgeAnswer{{3, 4}, {3, 1}, {1, 0}, {0, 0}} {
		if got := s.purgeRetired(root); got != want {
			t.Errorf("call %d = %+v, want %+v", i+1, got, want)
		}
	}
	if left := s.rootOf(dir); left.Files != 4 || left.RetiredFiles != 0 || left.RetiredWorks != 0 {
		t.Errorf("the directory holds %+v, want only the 4 works of the catalog", left)
	}
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/storage/roots/%d", root.ID), "").Code; code != 409 {
		t.Errorf("removing it with works in the catalog: %d, want 409", code)
	}
	s.exec(`DELETE FROM works`)
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/storage/roots/%d", root.ID), "").Code; code != 204 {
		t.Errorf("removing the directory after: %d, want 204", code)
	}
}

func TestPurgeRetired_AnUnknownDirectoryIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	for _, id := range []string{"999", "0", "x"} {
		if code := s.do(admin, "POST", "/admin/storage/roots/"+id+"/purge-retired", "").Code; code != 404 {
			t.Errorf("root %s: %d, want 404", id, code)
		}
	}
}
