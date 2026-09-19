package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrashAPI_DestroyingNeedsConfirmationAndPolicyIsExplicit(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	b := s.addWork("B", "X", "b.epub", "epub")
	for _, f := range []string{"a.epub", "b.epub"} {
		os.WriteFile(filepath.Join(s.storage, f), []byte("bytes of "+f), 0o644)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", a), "")
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", b), "")
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d?purge=true", a), "")
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d?purge=true", b), "")

	var list struct {
		Items      []struct{ ID, SizeBytes int64 }
		TotalBytes int64
		Policy     struct {
			Enabled bool
			Days    int
		}
	}
	json.Unmarshal(s.do(admin, "GET", "/admin/trash", "").Body.Bytes(), &list)
	if len(list.Items) != 2 || list.TotalBytes != int64(len("bytes of a.epub")+len("bytes of b.epub")) || list.Policy.Enabled || list.Policy.Days != 30 {
		t.Fatalf("trash = %+v (space used is reported and the automatic cleanup is off)", list)
	}

	id := list.Items[0].ID
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/trash/%d", id), "").Code; code != 400 {
		t.Errorf("deleting without confirming: %d, want 400", code)
	}
	if s.scalar(`SELECT count(*) FROM trash_items`) != "2" {
		t.Error("an unconfirmed delete removed something")
	}
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/trash/%d?confirm=true", id), "").Code; code != 204 {
		t.Errorf("confirmed delete: %d", code)
	}
	if code := s.do(admin, "DELETE", "/admin/trash/999999?confirm=true", "").Code; code != 404 {
		t.Errorf("unknown item: %d", code)
	}
	if code := s.do(admin, "POST", "/admin/trash/abc/restore", "").Code; code != 404 {
		t.Errorf("malformed id: %d", code)
	}

	// The policy: a bad value is refused, enabling it applies only to new items, applying to old ones needs confirmation.
	if code := s.do(admin, "PUT", "/admin/trash/policy", `{"enabled":true,"days":0}`).Code; code != 400 {
		t.Errorf("zero days: %d, want 400", code)
	}
	if code := s.do(admin, "PUT", "/admin/trash/policy", `{"enabled":true,"days":7}`).Code; code != 200 {
		t.Fatalf("set policy: %d", code)
	}
	if s.scalar(`SELECT count(*) FROM trash_items WHERE purge_after IS NOT NULL`) != "0" {
		t.Error("enabling the policy changed items already in the trash")
	}
	s.exec(`UPDATE trash_items SET trashed_at = now() - interval '30 days'`)
	var prev struct{ Items, AlreadyDue int }
	json.Unmarshal(s.do(admin, "GET", "/admin/trash/policy/preview?days=7", "").Body.Bytes(), &prev)
	if prev.Items != 1 || prev.AlreadyDue != 1 {
		t.Errorf("preview = %+v", prev)
	}
	if code := s.do(admin, "POST", "/admin/trash/policy/apply", `{"days":7}`).Code; code != 400 {
		t.Errorf("applying without confirmation: %d, want 400", code)
	}
	if code := s.do(admin, "POST", "/admin/trash/policy/apply", `{"days":7,"confirm":true}`).Code; code != 200 {
		t.Errorf("apply: %d", code)
	}
	if s.scalar(`SELECT count(*) FROM trash_items WHERE purge_after IS NOT NULL`) != "1" {
		t.Error("the policy was not applied to the existing item")
	}
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'trash.%'`); got != "trash.delete,trash.policy,trash.policy_apply" {
		t.Errorf("audit = %q", got)
	}
}

func TestOrphansAPI_ListsAndMovesToTheTrash(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Known", "X", "known.epub", "epub")
	os.WriteFile(filepath.Join(s.storage, "known.epub"), []byte("k"), 0o644)
	os.WriteFile(filepath.Join(s.storage, "stray.epub"), []byte("left by a crash"), 0o644)
	old := time.Now().Add(-72 * time.Hour)
	os.Chtimes(filepath.Join(s.storage, "stray.epub"), old, old)

	var out struct{ Data []struct{ Path string } }
	json.Unmarshal(s.do(admin, "GET", "/admin/storage/orphans", "").Body.Bytes(), &out)
	if len(out.Data) != 1 || out.Data[0].Path != "stray.epub" {
		t.Fatalf("orphans = %+v", out.Data)
	}
	if code := s.do(admin, "POST", "/admin/storage/orphans/trash", `{"paths":[]}`).Code; code != 400 {
		t.Errorf("no paths: %d", code)
	}
	var moved map[string]int
	json.Unmarshal(s.do(admin, "POST", "/admin/storage/orphans/trash", `{"paths":["stray.epub","known.epub"]}`).Body.Bytes(), &moved)
	if moved["moved"] != 1 {
		t.Errorf("moved = %v (a known file must never be moved)", moved)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "known.epub")); err != nil {
		t.Error("a known file was moved")
	}
	if s.scalar(`SELECT count(*) FROM trash_items WHERE kind = 'orphan'`) != "1" {
		t.Error("the orphan is not in the trash")
	}
}
