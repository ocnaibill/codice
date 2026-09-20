package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/ownership"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// RestoreOptions describes one restoration.
type RestoreOptions struct {
	DatabaseURL string
	StorageRoot string
	In          io.Reader
	Passphrase  string
	TmpDir      string
	// Overwrite lets the restore replace a database that already holds data. The old
	// database is never destroyed: it is renamed and kept.
	Overwrite bool
	// SkipHash checks that files exist and have the right size, without reading them all.
	SkipHash bool
	Now      func() time.Time
}

// RestoreResult reports what was restored and what could not be confirmed.
type RestoreResult struct {
	Manifest        Manifest
	KeptDatabase    string // the previous database, kept under this name (empty if there was none)
	FilesRestored   int
	FilesSkipped    int // already there with the same content
	FilesVerified   int
	FilesMissing    int
	FilesMismatched int
	Referenced      int // files that live in the operator's own folders: not checked here
	Duration        time.Duration
	Notes           []string
	// PackageBytes is what was read. The four times below add up to Duration (give or
	// take the connection setup) and say where a restore spends its time.
	PackageBytes int64
	Timings      RestoreTimings
}

// RestoreTimings splits a restore into its phases, to be scaled to a bigger instance:
// reading is proportional to the package size, files to the bytes checked, the database
// to the dump.
type RestoreTimings struct {
	Read     time.Duration // reading, decrypting and checking every byte of the package
	Files    time.Duration // bringing the files in and checking each against its hash
	Database time.Duration // restoring the database, checking it and swapping it in
	Finish   time.Duration // migrating the schema and recording the restore
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// databaseExists and friends run on the maintenance connection (the "postgres" database),
// never on the target itself, so that the target can be renamed.
func databaseExists(ctx context.Context, admin *sql.DB, name string) (bool, error) {
	var n int
	err := admin.QueryRowContext(ctx, `SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&n)
	return n > 0, err
}

// targetState looks at the target database: how many tables it has and whether any
// hold people or books.
func targetState(ctx context.Context, c pgConn) (tables int, hasData bool, err error) {
	db, err := c.open()
	if err != nil {
		return 0, false, err
	}
	defer db.Close()
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tables); err != nil {
		return
	}
	for _, t := range []string{"users", "works"} {
		var exists bool
		if err = db.QueryRowContext(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, t).Scan(&exists); err != nil {
			return
		}
		if !exists {
			continue
		}
		var n int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM `+quoteIdent(t)).Scan(&n); err != nil {
			return
		}
		if n > 0 {
			hasData = true
		}
	}
	return
}

func restoreInto(ctx context.Context, c pgConn, dump string, single bool) error {
	args := []string{"--no-owner", "--no-privileges", "--exit-on-error", "--dbname=" + c.dbname}
	if single {
		args = append(args, "--single-transaction")
	}
	f, err := os.Open(dump)
	if err != nil {
		return err
	}
	defer f.Close()
	return c.run(ctx, "pg_restore", f, nil, args...)
}

// listDump asks pg_restore to read the whole table of contents of a dump.
func listDump(ctx context.Context, c pgConn, dump string, out io.Writer) error {
	f, err := os.Open(dump)
	if err != nil {
		return err
	}
	defer f.Close()
	return c.run(ctx, "pg_restore", f, out, "--list")
}

func countsOf(ctx context.Context, db *sql.DB) (c Counts, err error) {
	err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM works),
		(SELECT count(*) FROM files), (SELECT count(*) FROM notes)`).Scan(&c.Users, &c.Works, &c.Files, &c.Notes)
	return
}

// installStaged moves the files read from the package into the storage directory. It
// first checks that none would replace a different file: nothing is moved on a conflict.
func installStaged(stage, root string, res *RestoreResult) error {
	type move struct{ from, to string }
	var moves []move
	var conflicts []string
	err := filepath.Walk(stage, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(stage, p)
		to := filepath.Join(root, rel)
		if existing, serr := os.Stat(to); serr == nil {
			a, _, e1 := sumFile(p)
			b, _, e2 := sumFile(to)
			if e1 == nil && e2 == nil && a == b && existing.Mode().IsRegular() {
				res.FilesSkipped++
				return nil
			}
			conflicts = append(conflicts, filepath.ToSlash(rel))
			return nil
		}
		moves = append(moves, move{p, to})
		return nil
	})
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		if len(conflicts) > 5 {
			conflicts = append(conflicts[:5], "…")
		}
		return fmt.Errorf("these files already exist with different content and would be overwritten: %s", strings.Join(conflicts, ", "))
	}
	for _, m := range moves {
		if err := os.MkdirAll(filepath.Dir(m.to), 0o755); err != nil {
			return err
		}
		if err := os.Rename(m.from, m.to); err != nil {
			return err
		}
		res.FilesRestored++
	}
	return nil
}

// checkFiles confirms the managed files against the recorded list: present, right size,
// and (unless skipHash) the recorded sha256. It returns the ids that are missing.
func checkFiles(root string, entries []FileEntry, skipHash bool, res *RestoreResult) (missing []int64) {
	for _, e := range entries {
		if e.Mode != "managed" {
			res.Referenced++
			continue
		}
		rel, ok := storage.SafeRel(e.Bytes)
		if !ok {
			res.FilesMissing++
			missing = append(missing, e.FileID)
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			res.FilesMissing++
			missing = append(missing, e.FileID)
			continue
		}
		if e.Size > 0 && info.Size() != e.Size {
			res.FilesMismatched++
			continue
		}
		if !skipHash && e.SHA256 != "" {
			if sum, _, err := sumFile(full); err != nil || sum != e.SHA256 {
				res.FilesMismatched++
				continue
			}
		}
		res.FilesVerified++
	}
	return missing
}

// Restore verifies the package completely, then restores it. The order is what keeps the
// instance safe: (1) read and verify every byte, touching only temporary places;
// (2) refuse a target that has data unless told to overwrite, or one still in use;
// (3) bring the files in, additively, refusing to replace different ones; (4) restore the
// database into a NEW database, check it, and only then swap the names, keeping the old
// database. A failure before the swap leaves the instance exactly as it was.
func Restore(ctx context.Context, o RestoreOptions) (res RestoreResult, err error) {
	start := time.Now()
	if o.Now == nil {
		o.Now = time.Now
	}
	target, err := parseConn(o.DatabaseURL)
	if err != nil {
		return res, err
	}
	for _, t := range []string{"pg_restore"} {
		if _, err := tool(t); err != nil {
			return res, err
		}
	}
	if err := os.MkdirAll(o.StorageRoot, 0o755); err != nil {
		return res, err
	}
	stage := filepath.Join(o.StorageRoot, ".restore-"+randHex(6))
	defer os.RemoveAll(stage)

	tRead := time.Now()
	pkg, err := Read(o.In, ReadOptions{Passphrase: o.Passphrase, TmpDir: o.TmpDir, StageDir: stage})
	if err != nil {
		return res, err
	}
	defer pkg.Close()
	res.Manifest, res.PackageBytes = pkg.Manifest, pkg.Bytes
	res.Timings.Read = time.Since(tRead)

	// The database dump must at least be listable before anything is changed.
	if err := listDump(ctx, target, pkg.DumpPath, io.Discard); err != nil {
		return res, fmt.Errorf("%w: the database dump cannot be read (%v)", ErrCorrupt, err)
	}

	adminConn := target.withDB("postgres")
	admin, err := adminConn.open()
	if err != nil {
		return res, err
	}
	defer admin.Close()
	exists, err := databaseExists(ctx, admin, target.dbname)
	if err != nil {
		return res, fmt.Errorf("cannot reach the database server: %w", err)
	}
	// Older into newer is fine; a package from a newer PostgreSQL will not load into an older one.
	var serverVersion string
	if err := admin.QueryRowContext(ctx, `SHOW server_version`).Scan(&serverVersion); err != nil {
		return res, err
	}
	if major(pkg.Manifest.ServerVersion) > major(serverVersion) {
		return res, fmt.Errorf("%w (package %d, server %d)", ErrServerTooOld, major(pkg.Manifest.ServerVersion), major(serverVersion))
	}
	tables, hasData := 0, false
	if exists {
		if tables, hasData, err = targetState(ctx, target); err != nil {
			return res, err
		}
		if hasData && !o.Overwrite {
			return res, ErrNotEmpty
		}
		// Our own connection to the target has just closed; give the server a moment to
		// notice, then insist that nobody else is there.
		var who string
		for attempt := 0; attempt < 15; attempt++ {
			rows, qerr := admin.QueryContext(ctx, `SELECT COALESCE(NULLIF(application_name, ''), '(no name)') || ' as ' || usename
				FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid() AND backend_type = 'client backend'`, target.dbname)
			if qerr != nil {
				return res, qerr
			}
			var names []string
			for rows.Next() {
				var n string
				rows.Scan(&n)
				names = append(names, n)
			}
			rows.Close()
			who = strings.Join(names, ", ")
			if len(names) == 0 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if who != "" {
			return res, fmt.Errorf("%w (connected: %s)", ErrBusy, who)
		}
	}

	// Files first: they only add. Orphans left by a later failure are harmless.
	tFiles := time.Now()
	if pkg.Manifest.IncludesFiles {
		if _, serr := os.Stat(stage); serr == nil {
			if err := installStaged(stage, o.StorageRoot, &res); err != nil {
				return res, err
			}
		}
	}
	missing := checkFiles(o.StorageRoot, pkg.Files, o.SkipHash, &res)
	res.Timings.Files = time.Since(tFiles)
	tDB := time.Now()

	// The database: into a new one, verified, then swapped in.
	switch {
	case !exists:
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+quoteIdent(target.dbname)+` TEMPLATE template0`); err != nil {
			return res, err
		}
		if err := restoreInto(ctx, target, pkg.DumpPath, true); err != nil {
			return res, err
		}
	case tables == 0:
		if err := restoreInto(ctx, target, pkg.DumpPath, true); err != nil {
			return res, err
		}
	default:
		scratchName := fmt.Sprintf("%s_restore_%s", target.dbname, randHex(4))
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+quoteIdent(scratchName)+` TEMPLATE template0`); err != nil {
			return res, err
		}
		scratch := target.withDB(scratchName)
		dropScratch := func() {
			admin.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+quoteIdent(scratchName)+` WITH (FORCE)`)
		}
		if err := restoreInto(ctx, scratch, pkg.DumpPath, false); err != nil {
			dropScratch()
			return res, err
		}
		sdb, err := scratch.open()
		if err != nil {
			dropScratch()
			return res, err
		}
		got, cerr := countsOf(ctx, sdb)
		sdb.Close()
		if cerr != nil || got != pkg.Manifest.Counts {
			dropScratch()
			return res, fmt.Errorf("%w: the restored copy does not match what the package says (%+v, expected %+v)", ErrCorrupt, got, pkg.Manifest.Counts)
		}
		kept := fmt.Sprintf("%s_before_restore_%s", target.dbname, o.Now().UTC().Format("20060102150405"))
		if _, err := admin.ExecContext(ctx, `ALTER DATABASE `+quoteIdent(target.dbname)+` RENAME TO `+quoteIdent(kept)); err != nil {
			dropScratch()
			return res, fmt.Errorf("cannot set the current database aside (is anything still connected?): %w", err)
		}
		if _, err := admin.ExecContext(ctx, `ALTER DATABASE `+quoteIdent(scratchName)+` RENAME TO `+quoteIdent(target.dbname)); err != nil {
			admin.ExecContext(context.Background(), `ALTER DATABASE `+quoteIdent(kept)+` RENAME TO `+quoteIdent(target.dbname))
			dropScratch()
			return res, err
		}
		res.KeptDatabase = kept
	}

	res.Timings.Database = time.Since(tDB)
	tFinish := time.Now()
	// The restored instance: bring the schema forward, and make the credentials safe.
	db, err := target.open()
	if err != nil {
		return res, err
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		return res, fmt.Errorf("the database was restored but could not be migrated to this version: %w", err)
	}
	if err := afterRestore(ctx, db, pkg, missing, &res); err != nil {
		return res, err
	}
	res.Timings.Finish = time.Since(tFinish)
	res.Duration = time.Since(start)
	return res, nil
}

// afterRestore records the restore and marks what could not be confirmed. Sessions, app
// tokens, invitations and reset links were never in the package, so no revoked credential
// comes back to life; the owner is told at the next sign-in.
func afterRestore(ctx context.Context, db *sql.DB, pkg *Package, missing []int64, res *RestoreResult) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range missing {
		if _, err := tx.ExecContext(ctx, `UPDATE storage_locations SET state = 'missing' WHERE file_id = $1 AND state = 'ok'`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE files SET availability = 'missing' WHERE id = $1 AND availability = 'available'`, id); err != nil {
			return err
		}
	}
	details := map[string]any{
		"backupCreatedAt": pkg.Manifest.CreatedAt, "schemaVersion": pkg.Manifest.SchemaVersion,
		"filesMissing": res.FilesMissing, "filesMismatched": res.FilesMismatched,
	}
	if err := audit.Record(ctx, tx, "", "backup.restore", "instance", "database", details); err != nil {
		return err
	}
	var ownerID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE role = 'owner' LIMIT 1`).Scan(&ownerID)
	if err == nil {
		if err := ownership.Notice(ctx, tx, ownerID, "instance_restored", map[string]any{"backupCreatedAt": pkg.Manifest.CreatedAt}); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return tx.Commit()
}
