package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

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

// ---- The owner's two buttons (DEC-123) ----

const ownerPassword = "a senha do dono"

// panelHandler is a handler whose panel is set up with a folder that has packages in it.
func panelHandler(t *testing.T, packages ...string) (*BackupAdminHandler, string) {
	t.Helper()
	db := migratedDB(t)
	dir := t.TempDir()
	for _, n := range packages {
		os.WriteFile(filepath.Join(dir, n), []byte("0123456789"), 0o600)
	}
	pass := filepath.Join(t.TempDir(), "frase")
	os.WriteFile(pass, []byte("uma frase longa e boa"), 0o600)
	h := &BackupAdminHandler{DB: db, StoragePath: t.TempDir(), Panel: backup.Panel{DB: db, Dir: dir, PassphraseFile: pass}}

	hash, err := bcrypt.GenerateFromPassword([]byte(ownerPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.QueryRow(`INSERT INTO users (username, email, role, password_hash) VALUES ('dono', 'dono@x', 'owner', $1) RETURNING id`, string(hash)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return h, id
}

func postPanel(t *testing.T, h *BackupAdminHandler, action func(http.ResponseWriter, *http.Request), userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/admin/backup/x", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.UserRoleKey, "owner")
	rec := httptest.NewRecorder()
	action(rec, req.WithContext(ctx))
	return rec
}

func jobCount(t *testing.T, h *BackupAdminHandler, where string) int {
	t.Helper()
	var n int
	if err := h.DB.QueryRow(`SELECT count(*) FROM jobs WHERE ` + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackupPanel_IsOffAndSaysSoWhenNotSetUp(t *testing.T) {
	db := migratedDB(t)
	h := &BackupAdminHandler{DB: db, StoragePath: t.TempDir()}
	panel, _ := getBackupAs(t, h, "owner")["panel"].(map[string]any)
	if panel["enabled"] != false || panel["job"] != nil || len(panel["packages"].([]any)) != 0 {
		t.Errorf("panel = %v", panel)
	}
	if _, has := panel["dir"]; has {
		t.Error("no folder to show")
	}
	rec := postPanel(t, h, h.Run, "u", `{"password":"x"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("run when off: %d, want 409", rec.Code)
	}
	if n := jobCount(t, h, "true"); n != 0 {
		t.Errorf("a job was queued although the panel is off: %d", n)
	}
}

func TestBackupPanel_ShowsThePackagesAndTheFolderOnlyToTheOwner(t *testing.T) {
	h, _ := panelHandler(t, "codice-backup-20261001-030000.tar.age", "codice-backup-20261002-030000.tar.age", "outro.txt")
	owner, _ := getBackupAs(t, h, "owner")["panel"].(map[string]any)
	if owner["enabled"] != true || owner["dir"] != h.Panel.Dir {
		t.Errorf("owner: %v", owner)
	}
	list, _ := owner["packages"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "codice-backup-20261002-030000.tar.age" {
		t.Errorf("packages = %v", list)
	}
	admin, _ := getBackupAs(t, h, "admin")["panel"].(map[string]any)
	if admin["enabled"] != true || len(admin["packages"].([]any)) != 2 {
		t.Errorf("admin should see that it exists and the names: %v", admin)
	}
	if _, has := admin["dir"]; has {
		t.Errorf("the folder of the server is the owner's: %v", admin)
	}
}

func TestBackupPanel_ShowsHowTheLastJobWentAndTheErrorOnlyToTheOwner(t *testing.T) {
	h, _ := panelHandler(t)
	insert := func(jobType, state, payload, lastErr string) {
		if _, err := h.DB.Exec(`INSERT INTO jobs (type, state, payload, last_error, started_at, finished_at) VALUES ($1, $2::varchar, $3::jsonb, NULLIF($4, ''), now(), CASE WHEN $2::varchar IN ('succeeded','failed') THEN now() END)`,
			jobType, state, payload, lastErr); err != nil {
			t.Fatal(err)
		}
	}
	insert("extract_text", "failed", `{}`, "not a backup job: ignored")
	insert(JobBackup, "succeeded", `{}`, "")
	insert(JobVerifyBackup, "failed", `{"name":"codice-backup-20261001-030000.tar.age"}`, "pg_restore: /usr/bin/pg_restore exploded")

	ownerJob, _ := getBackupAs(t, h, "owner")["panel"].(map[string]any)["job"].(map[string]any)
	if ownerJob["type"] != JobVerifyBackup || ownerJob["state"] != "failed" || ownerJob["name"] != "codice-backup-20261001-030000.tar.age" {
		t.Fatalf("the latest of the two buttons' jobs: %v", ownerJob)
	}
	if !strings.Contains(ownerJob["error"].(string), "exploded") || ownerJob["finishedAt"] == nil {
		t.Errorf("the owner sees what went wrong: %v", ownerJob)
	}
	adminJob, _ := getBackupAs(t, h, "admin")["panel"].(map[string]any)["job"].(map[string]any)
	if _, has := adminJob["error"]; has {
		t.Errorf("an administrator does not get the text of the error: %v", adminJob)
	}
	if adminJob["state"] != "failed" {
		t.Errorf("but sees that it failed: %v", adminJob)
	}
}

func TestBackupRun_AsksTheOwnerPasswordAgainAndQueuesOneAttemptAudited(t *testing.T) {
	h, owner := panelHandler(t)

	for name, body := range map[string]string{"none": `{}`, "empty": `{"password":""}`, "garbage": `nope`} {
		if rec := postPanel(t, h, h.Run, owner, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if rec := postPanel(t, h, h.Run, owner, `{"password":"errada"}`); rec.Code != http.StatusForbidden {
		t.Errorf("wrong password: %d, want 403", rec.Code)
	}
	if n := jobCount(t, h, "true"); n != 0 {
		t.Fatalf("a job was queued without the right password: %d", n)
	}

	rec := postPanel(t, h, h.Run, owner, `{"password":"`+ownerPassword+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("right password: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]float64
	json.Unmarshal(rec.Body.Bytes(), &got)
	if jobCount(t, h, "type = 'backup' AND state = 'pending' AND max_attempts = 1 AND created_by = '"+owner+"' AND id = "+strconv.Itoa(int(got["job_id"]))) != 1 {
		t.Errorf("the job is not what was asked: %v", got)
	}
	var audited int
	h.DB.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'backup.requested' AND actor_id = $1`, owner).Scan(&audited)
	if audited != 1 {
		t.Errorf("the request was not audited: %d", audited)
	}
}

func TestBackupRun_AnAccountWithNoLocalPasswordNeverPasses(t *testing.T) {
	h, _ := panelHandler(t)
	var id string
	h.DB.QueryRow(`INSERT INTO users (username, email, role) VALUES ('sem-senha', 's@x', 'reader') RETURNING id`).Scan(&id)
	if id == "" {
		t.Fatal("the account was not made")
	}
	if rec := postPanel(t, h, h.Run, id, `{"password":"qualquer"}`); rec.Code != http.StatusForbidden {
		t.Errorf("no local password: %d, want 403", rec.Code)
	}
}

func TestBackupRun_OnlyOneJobOfEitherKindLivesAtATime(t *testing.T) {
	h, owner := panelHandler(t, "codice-backup-20261001-030000.tar.age")
	ok := `"password":"` + ownerPassword + `"`

	first := postPanel(t, h, h.Run, owner, `{`+ok+`}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first: %d", first.Code)
	}
	for name, action := range map[string]func(http.ResponseWriter, *http.Request){"run": h.Run, "verify": h.Verify} {
		rec := postPanel(t, h, action, owner, `{`+ok+`,"name":"codice-backup-20261001-030000.tar.age"}`)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s while one is pending: %d, want 409", name, rec.Code)
		}
		var body map[string]any
		json.Unmarshal(rec.Body.Bytes(), &body)
		if body["job_id"] == nil {
			t.Errorf("%s: the answer must say which job is running: %v", name, body)
		}
	}
	if n := jobCount(t, h, "true"); n != 1 {
		t.Errorf("jobs = %d, want 1", n)
	}

	// Once it is over, another can be asked.
	h.DB.Exec(`UPDATE jobs SET state = 'succeeded'`)
	if rec := postPanel(t, h, h.Run, owner, `{`+ok+`}`); rec.Code != http.StatusAccepted {
		t.Errorf("after it finished: %d, want 202", rec.Code)
	}
}

func TestBackupVerify_ChecksOnlyAPackageOfTheFolder(t *testing.T) {
	h, owner := panelHandler(t, "codice-backup-20261001-030000.tar.age")
	ok := `"password":"` + ownerPassword + `"`
	for _, name := range []string{"", "nao-existe.tar", "codice-backup-20261009-030000.tar.age", "../codice-backup-20261001-030000.tar.age", "/etc/passwd"} {
		body, _ := json.Marshal(map[string]string{"password": ownerPassword, "name": name})
		if rec := postPanel(t, h, h.Verify, owner, string(body)); rec.Code != http.StatusNotFound {
			t.Errorf("name %q: %d, want 404", name, rec.Code)
		}
	}
	if n := jobCount(t, h, "true"); n != 0 {
		t.Fatalf("a job for a package that is not there: %d", n)
	}
	if rec := postPanel(t, h, h.Verify, owner, `{"password":"errada","name":"codice-backup-20261001-030000.tar.age"}`); rec.Code != http.StatusForbidden {
		t.Errorf("wrong password: %d, want 403", rec.Code)
	}

	rec := postPanel(t, h, h.Verify, owner, `{`+ok+`,"name":"codice-backup-20261001-030000.tar.age"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("right: %d %s", rec.Code, rec.Body.String())
	}
	if jobCount(t, h, "type = 'verify_backup' AND payload->>'name' = 'codice-backup-20261001-030000.tar.age' AND max_attempts = 1") != 1 {
		t.Error("the verify job is not what was asked")
	}
	var details string
	h.DB.QueryRow(`SELECT details::text FROM audit_log WHERE action = 'backup.requested'`).Scan(&details)
	if !strings.Contains(details, "codice-backup-20261001-030000.tar.age") {
		t.Errorf("the audit does not say which package: %s", details)
	}
}
