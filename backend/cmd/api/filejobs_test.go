package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/backup"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/storage"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

func corpusBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "corpus", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func scalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return s.String
}

// The file jobs, exactly as the API process runs them: a queue runner with the
// real handlers, through scanning a directory and transferring its files.
func TestFileJobs_ScanThenTransferThroughTheQueue(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	managed, _ := filepath.EvalSymlinks(t.TempDir())
	lib, _ := filepath.EvalSymlinks(t.TempDir())
	mover := &storage.Mover{DB: db, Root: managed}
	handlers := fileJobHandlers(db, mover, backup.Panel{}, &handlers.DataExportHandler{})
	types := []string{}
	for k := range handlers {
		types = append(types, k)
	}
	runner := &jobs.Runner{DB: db, Owner: "api-test", Types: types, MaxRunning: 1, LeaseSeconds: 60,
		Handlers: handlers, Heartbeat: 20 * time.Millisecond}
	ctx := context.Background()
	drain := func() int {
		n := 0
		for {
			took, err := runner.RunOnce(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !took {
				return n
			}
			n++
		}
	}

	os.MkdirAll(filepath.Join(lib, "Livros"), 0o755)
	os.WriteFile(filepath.Join(lib, "Livros", "Duna.epub"), corpusBytes(t, "epub_acentos.epub"), 0o644)
	os.WriteFile(filepath.Join(lib, "Livros", "quebrado.epub"), corpusBytes(t, "epub_corrompido.epub"), 0o644)
	var rootID int
	db.QueryRow(`INSERT INTO storage_roots (path) VALUES ($1) RETURNING id`, lib).Scan(&rootID)

	// A scan job catalogues the good file in place and rejects the bad one.
	db.Exec(`INSERT INTO jobs (type, payload) VALUES ('scan', jsonb_build_object('root_id', $1::int, 'subdir', ''))`, rootID)
	if n := drain(); n != 1 {
		t.Fatalf("jobs run = %d", n)
	}
	if got := scalar(t, db, `SELECT state FROM jobs WHERE type = 'scan'`); got != "succeeded" {
		t.Fatalf("scan job = %s", got)
	}
	if got := scalar(t, db, `SELECT count(*) FROM storage_locations WHERE mode = 'referenced'`); got != "1" {
		t.Fatalf("referenced locations = %s", got)
	}

	// A scan of a root that is no longer authorised fails for good: retrying cannot help.
	db.Exec(`INSERT INTO jobs (type, payload) VALUES ('scan', jsonb_build_object('root_id', 99999, 'subdir', ''))`)
	drain()
	if got := scalar(t, db, `SELECT state || '/' || error_kind || '/' || attempts FROM jobs WHERE payload->>'root_id' = '99999'`); got != "failed/permanent/1" {
		t.Errorf("unauthorised root = %q", got)
	}

	// The transfer job moves the file into the managed storage and removes the original.
	fileID := scalar(t, db, `SELECT file_id FROM work_primary WHERE file_mode = 'referenced'`)
	workID := scalar(t, db, `SELECT work_id FROM work_primary WHERE file_mode = 'referenced'`)
	db.Exec(`INSERT INTO jobs (type, work_id, payload) VALUES ('transfer', $1, jsonb_build_object('file_id', $2::bigint))`, workID, fileID)
	drain()
	if got := scalar(t, db, `SELECT state FROM jobs WHERE type = 'transfer'`); got != "succeeded" {
		t.Fatalf("transfer job = %s (%s)", got, scalar(t, db, `SELECT last_error FROM jobs WHERE type = 'transfer'`))
	}
	if got := scalar(t, db, `SELECT mode FROM storage_locations WHERE file_id = $1`, fileID); got != "managed" {
		t.Errorf("mode = %s", got)
	}
	if _, err := os.Stat(filepath.Join(lib, "Livros", "Duna.epub")); !os.IsNotExist(err) {
		t.Error("the original was not removed")
	}
	if _, err := os.Stat(filepath.Join(lib, "Livros", "quebrado.epub")); err != nil {
		t.Error("a file that was never catalogued was touched")
	}

	// Transferring a file that is already managed is a permanent failure, not a retry loop.
	db.Exec(`UPDATE jobs SET state = 'failed' WHERE type = 'transfer'`)
	db.Exec(`INSERT INTO jobs (type, work_id, payload) VALUES ('transfer', $1, jsonb_build_object('file_id', $2::bigint))`, workID, fileID)
	drain()
	if got := scalar(t, db, `SELECT error_kind || '/' || attempts FROM jobs WHERE type = 'transfer' AND state = 'failed' ORDER BY id DESC LIMIT 1`); got != "permanent/1" {
		t.Errorf("transferring twice = %q", got)
	}
	_ = strings.TrimSpace
}

// The owner's two backup buttons run as jobs of the API process (DEC-123). What the server's own set-up
// makes impossible fails for good at the first attempt: trying again cannot fix a missing passphrase.
func TestBackupJobs_FailForGoodWhenTheServerIsNotSetUpForThem(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	panelOff := backup.Panel{DB: db}
	panelNoPass := backup.Panel{DB: db, Dir: dir, PassphraseFile: filepath.Join(t.TempDir(), "nao-existe")}
	ctx := context.Background()

	run := func(panel backup.Panel, jobType, payload string) string {
		t.Helper()
		db.Exec(`DELETE FROM jobs`)
		handlers := fileJobHandlers(db, &storage.Mover{DB: db, Root: t.TempDir()}, panel, &handlers.DataExportHandler{})
		for _, typ := range []string{"backup", "verify_backup"} {
			if handlers[typ] == nil {
				t.Fatalf("no handler for %s", typ)
			}
		}
		// Three attempts allowed: a permanent error must not use them up.
		if _, err := db.Exec(`INSERT INTO jobs (type, payload, max_attempts) VALUES ($1, $2::jsonb, 3)`, jobType, payload); err != nil {
			t.Fatal(err)
		}
		runner := &jobs.Runner{DB: db, Owner: "api-test", Types: []string{jobType}, MaxRunning: 1, LeaseSeconds: 60, Handlers: handlers, Heartbeat: 20 * time.Millisecond}
		if took, err := runner.RunOnce(ctx); err != nil || !took {
			t.Fatalf("RunOnce: %v %v", took, err)
		}
		return scalar(t, db, `SELECT state || '/' || COALESCE(error_kind, '') || '/' || attempts || '/' || COALESCE(last_error, '') FROM jobs`)
	}

	cases := []struct {
		name    string
		panel   backup.Panel
		typ     string
		payload string
		want    string
	}{
		{"backup, panel off", panelOff, "backup", `{}`, "failed/permanent/1/" + backup.ErrPanelOff.Error()},
		{"backup, no passphrase", panelNoPass, "backup", `{}`, "failed/permanent/1/" + backup.ErrNoPassphrase.Error()},
		{"verify, panel off", panelOff, "verify_backup", `{"name":"codice-backup-20261001-030000.tar.age"}`, "failed/permanent/1/" + backup.ErrPanelOff.Error()},
		{"verify, no package named", panelNoPass, "verify_backup", `{}`, "failed/permanent/1/the job has no package"},
		{"verify, a package that is not there", backup.Panel{DB: db, Dir: dir, PassphraseFile: "x"}, "verify_backup", `{"name":"codice-backup-20261001-030000.tar"}`, "failed/permanent/1/" + backup.ErrNotAPackage.Error()},
	}
	for _, c := range cases {
		if got := run(c.panel, c.typ, c.payload); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// "Exportar meus dados" runs as a job of the API process: the request becomes a file, and a request that is gone fails for good.
func TestExportJob_MakesTheFileThroughTheQueue(t *testing.T) {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	exports := &handlers.DataExportHandler{DB: db, Dir: filepath.Join(t.TempDir(), "exports")}
	runner := &jobs.Runner{DB: db, Owner: "api-test", Types: []string{handlers.JobExportData}, MaxRunning: 1, LeaseSeconds: 60,
		Handlers: fileJobHandlers(db, &storage.Mover{DB: db, Root: t.TempDir()}, backup.Panel{}, exports), Heartbeat: 20 * time.Millisecond}

	var user, id string
	db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('ana', 'a@x', 'reader') RETURNING id`).Scan(&user)
	db.QueryRow(`INSERT INTO data_exports (user_id) VALUES ($1) RETURNING id`, user).Scan(&id)
	db.Exec(`INSERT INTO jobs (type, payload, max_attempts) VALUES ('export_data', jsonb_build_object('export_id', $1::text), 3)`, id)
	if took, err := runner.RunOnce(context.Background()); err != nil || !took {
		t.Fatalf("RunOnce: %v %v", took, err)
	}
	if got := scalar(t, db, `SELECT state FROM data_exports WHERE id = $1`, id); got != "ready" {
		t.Fatalf("export = %s", got)
	}
	if info, err := os.Stat(filepath.Join(exports.Dir, id+".zip")); err != nil || info.Size() == 0 {
		t.Errorf("the file: %v %v", info, err)
	}
	if got := scalar(t, db, `SELECT state FROM jobs WHERE type = 'export_data'`); got != "succeeded" {
		t.Errorf("job = %s", got)
	}

	// A request that is gone, or a job with no request, fails once and for good.
	for name, payload := range map[string]string{
		"a request that is gone": `{"export_id": "00000000-0000-0000-0000-000000000000"}`,
		"no request named":       `{}`,
	} {
		db.Exec(`DELETE FROM jobs`)
		db.Exec(`INSERT INTO jobs (type, payload, max_attempts) VALUES ('export_data', $1::jsonb, 3)`, payload)
		runner.RunOnce(context.Background())
		if got := scalar(t, db, `SELECT state || '/' || error_kind || '/' || attempts FROM jobs`); got != "failed/permanent/1" {
			t.Errorf("%s: %s", name, got)
		}
	}
}
