package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/filecheck"
	"github.com/ocnaibill/codice/backend/internal/jobs"
)

// Scanner catalogues files where they are, in a directory the owner authorised
// (referenced mode, DEC-032). It never moves, renames or changes those files.
type Scanner struct {
	DB *sql.DB
}

// ScanReport says what a scan found.
type ScanReport struct {
	Scanned    int      `json:"scanned"`
	Added      int      `json:"added"`
	Duplicates int      `json:"duplicates"`
	Rejected   int      `json:"rejected"`
	Changed    int      `json:"changed"`
	Missing    int      `json:"missing"`
	Reappeared int      `json:"reappeared"`
	Problems   []string `json:"problems"`
}

const maxProblems = 20

func (r *ScanReport) problem(format string, args ...any) {
	if len(r.Problems) < maxProblems {
		r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
	}
}

// Scan walks subdir (relative to root, "" for all of it) and brings the catalog
// in line with what is there:
//   - a new valid file becomes a work with a referenced location, and an
//     ingestion job is queued to read its metadata;
//   - bytes already in the library are not catalogued again (DEC-027);
//   - a file that disappeared is marked missing, keeping its notes and history,
//     and one that comes back is marked available again;
//   - a file whose content changed is flagged for review instead of silently
//     inheriting the notes of the old content (spec 12.2).
func (s *Scanner) Scan(ctx context.Context, root, subdir, actor string) (*ScanReport, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("the root is not readable: %w", err)
	}
	dir := realRoot
	if subdir != "" {
		clean, ok := SafeRel(subdir)
		if !ok {
			return nil, ErrUnsafePath
		}
		dir, err = filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(clean)))
		if err != nil || !within(dir, realRoot) {
			return nil, ErrUnsafePath
		}
	}

	report := &ScanReport{Problems: []string{}}
	seen := map[string]bool{}

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			report.problem("%s: %v", p, err)
			return nil
		}
		// Only regular files: a symlink named like a book could expose anything
		// the server can read.
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !filecheck.Supported[ext] {
			return nil
		}
		rel, err := filepath.Rel(realRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		report.Scanned++
		s.visit(ctx, realRoot, rel, p, ext, actor, report)
		return nil
	})
	if walkErr != nil {
		return report, walkErr
	}

	s.markMissing(ctx, realRoot, dir, seen, report)

	if err := audit.Record(ctx, s.DB, actor, "library.scan", "storage_root", realRoot, map[string]any{
		"subdir": subdir, "scanned": report.Scanned, "added": report.Added, "duplicates": report.Duplicates,
		"rejected": report.Rejected, "changed": report.Changed, "missing": report.Missing, "reappeared": report.Reappeared,
	}); err != nil {
		report.problem("could not audit the scan: %v", err)
	}
	return report, nil
}

func within(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// visit handles one file found on disk.
func (s *Scanner) visit(ctx context.Context, root, rel, abs, ext, actor string, report *ScanReport) {
	var locID, fileID int64
	var state string
	var sum sql.NullString
	var size sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.id, l.state, f.id, f.sha256, f.size_bytes
		FROM storage_locations l JOIN files f ON f.id = l.file_id
		WHERE l.mode = 'referenced' AND l.root = $1 AND l.path = $2`, root, rel).Scan(&locID, &state, &fileID, &sum, &size)

	switch {
	case err == nil:
		s.recheck(ctx, locID, fileID, state, sum.String, size.Int64, abs, rel, report)
	case errors.Is(err, sql.ErrNoRows):
		s.catalogue(ctx, root, rel, abs, ext, actor, report)
	default:
		report.problem("%s: %v", rel, err)
	}
}

// recheck looks at a file that is already catalogued: has it come back, or changed?
func (s *Scanner) recheck(ctx context.Context, locID, fileID int64, state, sum string, size int64, abs, rel string, report *ScanReport) {
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	if state == "conflict" {
		return // already waiting for a person to look at it
	}
	if size > 0 && info.Size() != size {
		// The size differs: confirm with the hash before calling it a change.
		if now, _, herr := filecheck.HashFile(abs); herr == nil && now != sum {
			s.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'conflict' WHERE id = $1`, locID)
			report.Changed++
			report.problem("%s: the content changed since it was catalogued; review it", rel)
			return
		}
	}
	if state == "missing" {
		s.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'ok' WHERE id = $1`, locID)
		s.DB.ExecContext(ctx, `UPDATE files SET availability = 'available' WHERE id = $1`, fileID)
		report.Reappeared++
	}
}

// catalogue adds a file that is not in the library yet.
func (s *Scanner) catalogue(ctx context.Context, root, rel, abs, ext, actor string, report *ScanReport) {
	if err := filecheck.Validate(abs, ext); err != nil {
		report.Rejected++
		report.problem("%s: %v", rel, err)
		return
	}
	sum, size, err := filecheck.HashFile(abs)
	if err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	var existing int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM files WHERE sha256 = $1`, sum).Scan(&existing); err == nil && existing > 0 {
		report.Duplicates++
		return
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	defer tx.Rollback()

	var workID, editionID int
	var fileID int64
	name := path.Base(rel)
	format := strings.TrimPrefix(ext, ".")
	if err := tx.QueryRowContext(ctx, `INSERT INTO works (original_title) VALUES ($1) RETURNING id`, name).Scan(&workID); err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO editions (work_id, title, is_primary) VALUES ($1, $2, TRUE) RETURNING id`, workID, name).Scan(&editionID); err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO files (edition_id, format, sha256, size_bytes) VALUES ($1, $2, $3, $4) RETURNING id`,
		editionID, format, sum, size).Scan(&fileID); err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23505" {
			report.Duplicates++ // the same bytes were catalogued while this scan ran
			return
		}
		report.problem("%s: %v", rel, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO storage_locations (file_id, mode, root, path) VALUES ($1, 'referenced', $2, $3)`, fileID, root, rel); err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	if _, err := jobs.Enqueue(ctx, tx, jobs.TypeIngest, workID, map[string]any{"file_path": abs}, jobs.PriorityBatch, actor); err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	if err := tx.Commit(); err != nil {
		report.problem("%s: %v", rel, err)
		return
	}
	report.Added++
}

// markMissing flags catalogued files, inside the scanned directory, that were not
// found. Their notes, progress and metadata stay.
func (s *Scanner) markMissing(ctx context.Context, root, dir string, seen map[string]bool, report *ScanReport) {
	prefix := ""
	if dir != root {
		rel, _ := filepath.Rel(root, dir)
		prefix = filepath.ToSlash(rel)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT l.id, l.file_id, l.path FROM storage_locations l
		WHERE l.mode = 'referenced' AND l.root = $1 AND l.state = 'ok'
		  AND ($2 = '' OR starts_with(l.path, $2 || '/'))`, root, prefix)
	if err != nil {
		report.problem("looking for missing files: %v", err)
		return
	}
	type gone struct {
		locID, fileID int64
	}
	var lost []gone
	for rows.Next() {
		var g gone
		var rel string
		if rows.Scan(&g.locID, &g.fileID, &rel) == nil && !seen[rel] {
			lost = append(lost, g)
		}
	}
	rows.Close()
	for _, g := range lost {
		s.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'missing' WHERE id = $1`, g.locID)
		s.DB.ExecContext(ctx, `UPDATE files SET availability = 'missing' WHERE id = $1`, g.fileID)
		report.Missing++
	}
}
