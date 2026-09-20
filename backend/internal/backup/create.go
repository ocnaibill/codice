package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// MinPassphrase is the shortest passphrase accepted for encrypting a package.
const MinPassphrase = 8

// CreateOptions describes one backup.
type CreateOptions struct {
	DB           *sql.DB
	DatabaseURL  string
	StorageRoot  string
	Out          io.Writer
	IncludeFiles bool   // also put the managed files and covers in the package
	Passphrase   string // encrypts the package with age when set; the operator keeps it off the server
	TmpDir       string // where the dump waits while the package is assembled (default: the system's)
	Now          func() time.Time
}

// Result says what was made.
type Result struct {
	hashes    map[string]string
	Manifest  Manifest
	Bytes     int64
	Encrypted bool
	Duration  time.Duration
	// Warnings are files that could not be included or did not match their recorded hash.
	Warnings []string
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func sumFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// snapshotFacts reads, inside one snapshot, everything the package records about the database.
func snapshotFacts(ctx context.Context, tx *sql.Tx) (schema int64, server string, counts Counts, files []FileEntry, err error) {
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&schema); err != nil {
		return
	}
	if err = tx.QueryRowContext(ctx, `SHOW server_version`).Scan(&server); err != nil {
		return
	}
	if err = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM works),
		(SELECT count(*) FROM files), (SELECT count(*) FROM notes)`).Scan(&counts.Users, &counts.Works, &counts.Files, &counts.Notes); err != nil {
		return
	}
	rows, qerr := tx.QueryContext(ctx, `
		SELECT f.id, COALESCE(w.original_title, ''), l.mode, COALESCE(l.root, ''), l.path,
		       CASE WHEN l.state = 'trashed' THEN COALESCE(ti.trash_path, l.path) ELSE l.path END,
		       l.state, COALESCE(f.size_bytes, 0), COALESCE(f.sha256, '')
		FROM storage_locations l
		JOIN files f ON f.id = l.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		LEFT JOIN trash_items ti ON ti.file_id = f.id
		ORDER BY f.id, l.id`)
	if qerr != nil {
		err = qerr
		return
	}
	defer rows.Close()
	for rows.Next() {
		var e FileEntry
		if err = rows.Scan(&e.FileID, &e.Title, &e.Mode, &e.Root, &e.Path, &e.Bytes, &e.State, &e.Size, &e.SHA256); err != nil {
			return
		}
		files = append(files, e)
	}
	err = rows.Err()
	return
}

// Create writes a package to o.Out. The database dump and the list of files come from
// the same snapshot, so they agree even if the library changes while it runs.
func Create(ctx context.Context, o CreateOptions) (Result, error) {
	start := time.Now()
	res := Result{Encrypted: o.Passphrase != "", hashes: map[string]string{}}
	if o.Passphrase != "" && len(o.Passphrase) < MinPassphrase {
		return res, fmt.Errorf("the passphrase must have at least %d characters", MinPassphrase)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	conn, err := parseConn(o.DatabaseURL)
	if err != nil {
		return res, err
	}
	if _, err := tool("pg_dump"); err != nil {
		return res, err
	}
	clientMajor, err := toolMajor(ctx, "pg_dump")
	if err != nil {
		return res, err
	}

	// Hold one snapshot open while the dump and the queries read from it.
	tx, err := o.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	var snapshot string
	if err := tx.QueryRowContext(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		return res, err
	}
	schema, server, counts, entries, err := snapshotFacts(ctx, tx)
	if err != nil {
		return res, fmt.Errorf("reading the database: %w", err)
	}
	if sm := major(server); clientMajor > sm {
		return res, fmt.Errorf("%w (pg_dump %d, server %d)", ErrToolTooNew, clientMajor, sm)
	}

	dump, err := os.CreateTemp(o.TmpDir, "codice-backup-*.dump")
	if err != nil {
		return res, err
	}
	dumpPath := dump.Name()
	defer os.Remove(dumpPath)
	args := []string{"--format=custom", "--no-owner", "--no-privileges", "--snapshot=" + snapshot}
	for _, t := range append(append([]string{}, credentialTables...), derivedTables...) {
		args = append(args, "--exclude-table-data=public."+t)
	}
	err = conn.run(ctx, "pg_dump", nil, dump, args...)
	dump.Close()
	if err != nil {
		return res, err
	}
	dumpSum, dumpSize, err := sumFile(dumpPath)
	if err != nil {
		return res, err
	}

	cw := &countingWriter{w: o.Out}
	var sink io.Writer = cw
	var closeEnc func() error
	if o.Passphrase != "" {
		rcpt, err := age.NewScryptRecipient(o.Passphrase)
		if err != nil {
			return res, err
		}
		enc, err := age.Encrypt(cw, rcpt)
		if err != nil {
			return res, err
		}
		sink, closeEnc = enc, enc.Close
	}
	tw := tar.NewWriter(sink)
	members := map[string]Member{}
	modTime := o.Now().UTC()

	// addStream writes one member of known size and records its hash.
	addStream := func(name string, size int64, r io.Reader) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: size, ModTime: modTime, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(tw, h), r)
		if err != nil {
			return err
		}
		if n != size {
			return fmt.Errorf("%s changed while it was being read", name)
		}
		members[name] = Member{Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}
		res.hashes[name] = members[name].SHA256
		return nil
	}

	df, err := os.Open(dumpPath)
	if err != nil {
		return res, err
	}
	err = addStream(MemberDump, dumpSize, df)
	df.Close()
	if err != nil {
		return res, err
	}
	members[MemberDump] = Member{Size: dumpSize, SHA256: dumpSum}

	listing, _ := json.Marshal(entries)
	if err := addStream(MemberFiles, int64(len(listing)), strings.NewReader(string(listing))); err != nil {
		return res, err
	}

	sum := FilesSummary{Total: len(entries)}
	for _, e := range entries {
		sum.Bytes += e.Size
	}
	if o.IncludeFiles {
		if err := includeFiles(o, entries, &sum, &res, addStream); err != nil {
			return res, err
		}
	}

	m := Manifest{
		Format: FormatVersion, Tool: "codice-admin", CreatedAt: modTime, SchemaVersion: schema, ServerVersion: server,
		IncludesFiles: o.IncludeFiles, Counts: counts, Files: sum, Members: members,
	}
	body, _ := json.MarshalIndent(m, "", "  ")
	if err := tw.WriteHeader(&tar.Header{Name: MemberManifest, Mode: 0o600, Size: int64(len(body)), ModTime: modTime, Typeflag: tar.TypeReg}); err != nil {
		return res, err
	}
	if _, err := tw.Write(body); err != nil {
		return res, err
	}
	if err := tw.Close(); err != nil {
		return res, err
	}
	if closeEnc != nil {
		if err := closeEnc(); err != nil {
			return res, err
		}
	}

	res.Manifest, res.Bytes, res.Duration = m, cw.n, time.Since(start)
	return res, nil
}

func (r *Result) membersSum(name string) string { return r.hashes[name] }

// includeFiles copies the managed files, and the covers directory, into the package.
func includeFiles(o CreateOptions, entries []FileEntry, sum *FilesSummary, res *Result, add func(string, int64, io.Reader) error) error {
	root := filepath.Clean(o.StorageRoot)
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Mode != "managed" || seen[e.Bytes] {
			continue
		}
		rel, ok := storage.SafeRel(e.Bytes)
		if !ok {
			res.Warnings = append(res.Warnings, fmt.Sprintf("file %d: unsafe path, not included", e.FileID))
			sum.Missing++
			continue
		}
		seen[e.Bytes] = true
		full := filepath.Join(root, filepath.FromSlash(rel))
		f, err := os.Open(full)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				// A file that is there but cannot be read is not the same as one that is gone: the
				// package would silently lack the library, and a scheduled backup would look fine.
				return fmt.Errorf("file %d (%s) exists but cannot be read (%w): run the backup as the user that owns the library, or leave out --include-files", e.FileID, rel, unwrapPath(err))
			}
			res.Warnings = append(res.Warnings, fmt.Sprintf("file %d (%s): not on disk, not included", e.FileID, rel))
			sum.Missing++
			continue
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			f.Close()
			res.Warnings = append(res.Warnings, fmt.Sprintf("file %d (%s): not a regular file, not included", e.FileID, rel))
			sum.Missing++
			continue
		}
		err = add(MemberStorage+rel, info.Size(), f)
		f.Close()
		if err != nil {
			return err
		}
		if e.SHA256 != "" {
			if m := res.membersSum(MemberStorage + rel); m != "" && m != e.SHA256 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("file %d (%s): its content does not match the hash recorded in the database", e.FileID, rel))
			}
		}
		sum.Included++
	}
	// Covers are derived from the files, but small and quick to include: without them a
	// restored library shows placeholders until every file is analysed again.
	covers := filepath.Join(root, "covers")
	return filepath.WalkDir(covers, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("cover %s cannot be read (%w): run the backup as the user that owns the library, or leave out --include-files", rel, unwrapPath(err))
		}
		defer f.Close()
		return add(MemberStorage+path.Clean(filepath.ToSlash(rel)), info.Size(), f)
	})
}

// unwrapPath drops the path from an *fs.PathError, which the message already names.
func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
