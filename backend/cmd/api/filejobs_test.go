package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/database"
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
	handlers := fileJobHandlers(db, mover)
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
