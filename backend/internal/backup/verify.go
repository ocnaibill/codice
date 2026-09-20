package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// VerifyOptions describes a check of a package.
type VerifyOptions struct {
	In          io.Reader
	Passphrase  string
	TmpDir      string
	DatabaseURL string
	// Deep restores the dump into a temporary database on the server, compares what came
	// back with what the package says, and drops it: a rehearsal of the restore, with its
	// real duration. It needs permission to create databases.
	Deep bool
}

// VerifyResult reports what the check found.
type VerifyResult struct {
	Manifest     Manifest
	DumpEntries  int
	Encrypted    bool
	Reading      time.Duration
	PackageBytes int64
	Deep         *DeepResult
}

// DeepResult is the outcome of the rehearsal.
type DeepResult struct {
	Duration time.Duration
	Restored Counts
}

// Verify reads a package end to end (sizes and sha256 of every member against the manifest,
// format, schema), confirms the database dump can be read, and optionally rehearses the
// restore. It changes nothing.
func Verify(ctx context.Context, o VerifyOptions) (VerifyResult, error) {
	var res VerifyResult
	start := time.Now()
	pkg, err := Read(o.In, ReadOptions{Passphrase: o.Passphrase, TmpDir: o.TmpDir})
	if err != nil {
		return res, err
	}
	defer pkg.Close()
	res.Manifest, res.Encrypted, res.Reading, res.PackageBytes = pkg.Manifest, pkg.Encrypted, time.Since(start), pkg.Bytes

	conn, err := parseConn(o.DatabaseURL)
	if err != nil && (o.Deep || o.DatabaseURL != "") {
		return res, err
	}
	var listing bytes.Buffer
	lister := conn
	if err != nil { // no DATABASE_URL: the list needs no connection
		lister = pgConn{}
	}
	if err := listDump(ctx, lister, pkg.DumpPath, &listing); err != nil {
		return res, fmt.Errorf("%w: the database dump cannot be read (%v)", ErrCorrupt, err)
	}
	for _, line := range strings.Split(listing.String(), "\n") {
		if line != "" && !strings.HasPrefix(line, ";") {
			res.DumpEntries++
		}
	}
	if !o.Deep {
		return res, nil
	}

	t0 := time.Now()
	admin, err := conn.withDB("postgres").open()
	if err != nil {
		return res, err
	}
	defer admin.Close()
	var serverVersion string
	if err := admin.QueryRowContext(ctx, `SHOW server_version`).Scan(&serverVersion); err != nil {
		return res, err
	}
	if major(pkg.Manifest.ServerVersion) > major(serverVersion) {
		return res, fmt.Errorf("%w (package %d, server %d)", ErrServerTooOld, major(pkg.Manifest.ServerVersion), major(serverVersion))
	}
	name := "codice_verify_" + randHex(5)
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+quoteIdent(name)+` TEMPLATE template0`); err != nil {
		return res, fmt.Errorf("cannot create a scratch database for the rehearsal (it needs permission to create databases): %w", err)
	}
	defer admin.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+quoteIdent(name)+` WITH (FORCE)`)
	scratch := conn.withDB(name)
	if err := restoreInto(ctx, scratch, pkg.DumpPath, false); err != nil {
		return res, err
	}
	sdb, err := scratch.open()
	if err != nil {
		return res, err
	}
	defer sdb.Close()
	got, err := countsOf(ctx, sdb)
	if err != nil {
		return res, err
	}
	res.Deep = &DeepResult{Duration: time.Since(t0), Restored: got}
	if got != pkg.Manifest.Counts {
		return res, fmt.Errorf("%w: the rehearsal restored %+v but the package says %+v", ErrCorrupt, got, pkg.Manifest.Counts)
	}
	return res, nil
}
