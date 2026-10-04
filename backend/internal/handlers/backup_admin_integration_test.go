package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/backup"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

func TestBackupAdmin_ShowsTheLatestBackupOrNone(t *testing.T) {
	db := migratedDB(t)
	h := &BackupAdminHandler{DB: db}
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		h.Get(rec, httptest.NewRequest("GET", "/admin/backup", nil))
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	if got := get(); got["lastBackup"] != nil {
		t.Errorf("before any backup: %v", got)
	}
	at := time.Now().UTC().Truncate(time.Second)
	if err := backup.Record(t.Context(), db, backup.Result{Bytes: 4096, Encrypted: true,
		Manifest: backup.Manifest{CreatedAt: at, IncludesFiles: true, Files: backup.FilesSummary{Included: 3}}}, ""); err != nil {
		t.Fatal(err)
	}
	last, _ := get()["lastBackup"].(map[string]any)
	if last == nil || last["bytes"] != float64(4096) || last["encrypted"] != true || last["includesFiles"] != true || last["files"] != float64(3) {
		t.Errorf("last = %v", last)
	}
}

// getBackupAs asks the panel as someone with the given role.
func getBackupAs(t *testing.T, h *BackupAdminHandler, role string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/admin/backup", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserRoleKey, role))
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func recordPackage(t *testing.T, h *BackupAdminHandler, path string) {
	t.Helper()
	res := backup.Result{Bytes: 4096, Encrypted: true,
		Manifest: backup.Manifest{CreatedAt: time.Now().UTC(), IncludesFiles: true, Files: backup.FilesSummary{Included: 3}}}
	if err := backup.Record(t.Context(), h.DB, res, path); err != nil {
		t.Fatal(err)
	}
}

func TestBackupAdmin_ThePathIsTheOwnersAndTheNameIsForAllStaff(t *testing.T) {
	db := migratedDB(t)
	storage := t.TempDir()
	h := &BackupAdminHandler{DB: db, StoragePath: storage}
	path := "/mnt/backups/codice/codice-backup-20261004-030000.tar.age"
	recordPackage(t, h, path)

	owner, _ := getBackupAs(t, h, "owner")["lastBackup"].(map[string]any)
	if owner["path"] != path || owner["name"] != "codice-backup-20261004-030000.tar.age" {
		t.Errorf("owner sees %v", owner)
	}
	admin, _ := getBackupAs(t, h, "admin")["lastBackup"].(map[string]any)
	if admin["name"] != "codice-backup-20261004-030000.tar.age" {
		t.Errorf("admin should see the name: %v", admin)
	}
	if _, has := admin["path"]; has {
		t.Errorf("admin must not get the path of the server: %v", admin)
	}
	if _, has := getBackupAs(t, h, "")["lastBackup"].(map[string]any)["path"]; has {
		t.Error("with no role in the context the path must not go out")
	}
}

func TestBackupAdmin_AStreamedPackageHasNoAddress(t *testing.T) {
	db := migratedDB(t)
	h := &BackupAdminHandler{DB: db, StoragePath: t.TempDir()}
	recordPackage(t, h, "")
	last, _ := getBackupAs(t, h, "owner")["lastBackup"].(map[string]any)
	if _, has := last["name"]; has {
		t.Errorf("no name for a package that went to standard output: %v", last)
	}
	if _, has := last["path"]; has {
		t.Errorf("no path either: %v", last)
	}
	if last["verified"] != nil {
		t.Errorf("without a name nothing can be said to have been checked: %v", last)
	}
	// Even a check recorded with no name does not vouch for a package that has none.
	if err := backup.RecordVerified(t.Context(), db, "", true); err != nil {
		t.Fatal(err)
	}
	last, _ = getBackupAs(t, h, "owner")["lastBackup"].(map[string]any)
	if last["verified"] != nil {
		t.Errorf("two missing names are not the same package: %v", last["verified"])
	}
}

func TestBackupAdmin_OnlyACheckOfThisPackageCounts(t *testing.T) {
	db := migratedDB(t)
	h := &BackupAdminHandler{DB: db, StoragePath: t.TempDir()}
	recordPackage(t, h, "/b/codice-backup-A.tar")

	if err := backup.RecordVerified(t.Context(), db, "codice-backup-OLD.tar", true); err != nil {
		t.Fatal(err)
	}
	last, _ := getBackupAs(t, h, "owner")["lastBackup"].(map[string]any)
	if last["verified"] != nil {
		t.Fatalf("a check of another package says nothing about this one: %v", last["verified"])
	}

	if err := backup.RecordVerified(t.Context(), db, "codice-backup-A.tar", false); err != nil {
		t.Fatal(err)
	}
	last, _ = getBackupAs(t, h, "admin")["lastBackup"].(map[string]any)
	v, _ := last["verified"].(map[string]any)
	if v == nil || v["deep"] != false || v["at"] == nil {
		t.Fatalf("verified = %v", last["verified"])
	}
	if err := backup.RecordVerified(t.Context(), db, "codice-backup-A.tar", true); err != nil {
		t.Fatal(err)
	}
	last, _ = getBackupAs(t, h, "admin")["lastBackup"].(map[string]any)
	if v, _ := last["verified"].(map[string]any); v == nil || v["deep"] != true {
		t.Fatalf("the latest check wins: %v", last["verified"])
	}
}

func TestBackupAdmin_SaysWhenThePackageIsOnTheSameDiskAsTheStorage(t *testing.T) {
	db := migratedDB(t)
	storage := t.TempDir()
	h := &BackupAdminHandler{DB: db, StoragePath: storage}

	inside := filepath.Join(storage, "backups")
	os.MkdirAll(inside, 0o755)
	recordPackage(t, h, filepath.Join(inside, "p.tar"))
	last, _ := getBackupAs(t, h, "owner")["lastBackup"].(map[string]any)
	if last["sameDisk"] != true {
		t.Errorf("a package next to the storage: sameDisk = %v, want true", last["sameDisk"])
	}

	recordPackage(t, h, "/surely/not/here/p.tar")
	last, _ = getBackupAs(t, h, "owner")["lastBackup"].(map[string]any)
	if last["sameDisk"] != nil {
		t.Errorf("a package this process cannot see: sameDisk = %v, want null (unknown)", last["sameDisk"])
	}
}

func TestBackupAdmin_ReportsTheFreeSpaceOfTheStorage(t *testing.T) {
	db := migratedDB(t)
	got := getBackupAs(t, &BackupAdminHandler{DB: db, StoragePath: t.TempDir()}, "owner")
	storage, _ := got["storage"].(map[string]any)
	free, total := storage["freeBytes"].(float64), storage["totalBytes"].(float64)
	if total <= 0 || free <= 0 || free > total {
		t.Errorf("storage = %v", storage)
	}
	if got := getBackupAs(t, &BackupAdminHandler{DB: db}, "owner")["storage"]; got != nil {
		t.Errorf("with no storage path there is nothing to measure: %v", got)
	}
}

func TestBackupAdmin_CountsTheQueueAndNamesTheOldestWaiting(t *testing.T) {
	db := migratedDB(t)
	h := &BackupAdminHandler{DB: db}
	empty, _ := getBackupAs(t, h, "owner")["queue"].(map[string]any)
	if empty["pending"] != float64(0) || empty["running"] != float64(0) || empty["failedRecent"] != float64(0) || empty["oldestWaiting"] != nil {
		t.Fatalf("empty queue = %v", empty)
	}

	old := time.Now().UTC().Add(-90 * time.Minute).Truncate(time.Second)
	// A job that is not due yet is not waiting for anyone: alone, it leaves nothing "oldest".
	if _, err := db.Exec(`INSERT INTO jobs (type, state, run_at) VALUES ('extract_text', 'pending', now() + interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	if q, _ := getBackupAs(t, h, "owner")["queue"].(map[string]any); q["pending"] != float64(1) || q["oldestWaiting"] != nil {
		t.Fatalf("only a future job: %v", q)
	}
	insert := func(state string, runAt time.Time, updatedAt time.Time) {
		if _, err := db.Exec(`INSERT INTO jobs (type, state, run_at, updated_at) VALUES ('extract_text', $1, $2, $3)`, state, runAt, updatedAt); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	insert("pending", old, now)                   // waiting for 90 minutes
	insert("pending", now.Add(-time.Minute), now) // waiting, newer
	insert("running", now, now)
	insert("failed", now, now.Add(-time.Hour))    // failed an hour ago
	insert("failed", now, now.Add(-72*time.Hour)) // failed long ago: not today's problem
	insert("succeeded", now, now)
	insert("cancelled", now, now)

	q, _ := getBackupAs(t, h, "owner")["queue"].(map[string]any)
	if q["pending"] != float64(3) || q["running"] != float64(1) || q["failedRecent"] != float64(1) {
		t.Errorf("queue = %v", q)
	}
	waiting, _ := q["oldestWaiting"].(string)
	got, err := time.Parse(time.RFC3339Nano, waiting)
	if err != nil || !got.Equal(old) {
		t.Errorf("oldestWaiting = %q (%v), want %s", waiting, err, old)
	}
}
