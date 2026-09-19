package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lib/pq"
)

var (
	// ErrTargetTaken: another file already lives at the destination, in the
	// database or on disk. Nothing is overwritten (spec 12.1).
	ErrTargetTaken = errors.New("the destination is already taken")
	// ErrUnsafePath: the path would leave the storage root.
	ErrUnsafePath = errors.New("the path is not a safe relative path")
	// ErrNotMovable: the file has no managed location, or it is not in a state that can move.
	ErrNotMovable = errors.New("the file cannot be moved")
)

// Mover puts managed files at their layout path and keeps the database and the
// disk in step, even if the process dies half way.
type Mover struct {
	DB   *sql.DB
	Root string // the managed storage directory; every managed path is relative to it
}

// SafeRel cleans a relative path and refuses anything that could leave its root.
func SafeRel(rel string) (string, bool) {
	rel = path.Clean(filepath.ToSlash(rel))
	if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) || strings.ContainsRune(rel, 0) {
		return "", false
	}
	return rel, true
}

// AbsPath is where a stored file really is. A managed location is relative to the
// storage directory and a referenced one to its root; either way the relative
// part must be a safe path that stays inside it.
func AbsPath(storageRoot, mode, locRoot, rel string) (string, bool) {
	rel, ok := SafeRel(rel)
	if !ok {
		return "", false
	}
	base := storageRoot
	if mode == "referenced" {
		base = locRoot
	}
	if base == "" {
		return "", false
	}
	return filepath.Join(base, filepath.FromSlash(rel)), true
}

func (m *Mover) full(rel string) string { return filepath.Join(m.Root, filepath.FromSlash(rel)) }

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// Move relocates one managed file to target (relative to the root).
//
// The intended destination is recorded first, then the file is renamed, then the
// database is confirmed. If the process stops between these steps, Recover
// settles the outcome from what is on disk. An existing file is never overwritten.
func (m *Mover) Move(ctx context.Context, fileID int64, target string) error {
	target, ok := SafeRel(target)
	if !ok {
		return ErrUnsafePath
	}

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var locID int64
	var from, state string
	err = tx.QueryRowContext(ctx, `
		SELECT id, path, state FROM storage_locations
		WHERE file_id = $1 AND mode = 'managed' ORDER BY id LIMIT 1 FOR UPDATE`, fileID).Scan(&locID, &from, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no managed location", ErrNotMovable)
	}
	if err != nil {
		return err
	}
	if state != "ok" {
		return fmt.Errorf("%w: the location is %s", ErrNotMovable, state)
	}
	if from == target {
		if _, err := tx.ExecContext(ctx, `UPDATE files SET organized_at = COALESCE(organized_at, now()) WHERE id = $1`, fileID); err != nil {
			return err
		}
		return tx.Commit()
	}

	var taken bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM storage_locations WHERE mode = 'managed' AND path = $1 AND id <> $2)`,
		target, locID).Scan(&taken); err != nil {
		return err
	}
	if taken || exists(m.full(target)) {
		return fmt.Errorf("%w: %s", ErrTargetTaken, target)
	}
	if !exists(m.full(from)) {
		return fmt.Errorf("%w: the file is not on disk (%s)", ErrNotMovable, from)
	}

	// 1. Say what is about to happen.
	if _, err := tx.ExecContext(ctx, `UPDATE storage_locations SET state = 'moving', moving_to = $1 WHERE id = $2`, target, locID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// 2. Move the file.
	if err := os.MkdirAll(filepath.Dir(m.full(target)), 0o755); err == nil {
		err = os.Rename(m.full(from), m.full(target))
		if err != nil {
			m.revert(ctx, locID)
			return fmt.Errorf("moving the file: %w", err)
		}
	} else {
		m.revert(ctx, locID)
		return fmt.Errorf("creating the destination folder: %w", err)
	}

	// 3. Confirm it in the database. If this fails the location stays 'moving'
	// and Recover finishes the job from what is on disk.
	if err := m.finalize(ctx, locID, fileID, target); err != nil {
		return err
	}
	m.pruneEmptyDirs(filepath.Dir(m.full(from)))
	return nil
}

func (m *Mover) revert(ctx context.Context, locID int64) {
	m.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'ok', moving_to = NULL WHERE id = $1`, locID)
}

// finalize records that the file is now at target. The legacy column
// works.file_path is kept in step for code that still reads it.
func (m *Mover) finalize(ctx context.Context, locID, fileID int64, target string) error {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE storage_locations SET path = $1, state = 'ok', moving_to = NULL WHERE id = $2`, target, locID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE files SET organized_at = now() WHERE id = $1`, fileID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE works SET file_path = $1
		WHERE id = (SELECT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = $2)
		  AND (SELECT file_id FROM work_primary WHERE work_id = works.id) = $2`, target, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// pruneEmptyDirs removes the folders a move left empty, never the root itself.
func (m *Mover) pruneEmptyDirs(dir string) {
	root := filepath.Clean(m.Root)
	for dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)) {
		if err := os.Remove(dir); err != nil { // fails while it still has content: exactly what we want
			return
		}
		dir = filepath.Dir(dir)
	}
}

// RecoverResult counts how interrupted moves were settled.
type RecoverResult struct{ Completed, Reverted, Conflicts, Missing int }

// Recover settles moves that were interrupted, by comparing the disk with the
// recorded intention: the file arrived (finish it), never left (undo it), is in
// both places (flag a conflict for an admin) or in neither (mark it missing).
func (m *Mover) Recover(ctx context.Context) (RecoverResult, error) {
	var res RecoverResult
	rows, err := m.DB.QueryContext(ctx, `SELECT id, file_id, path, moving_to FROM storage_locations WHERE state = 'moving'`)
	if err != nil {
		return res, err
	}
	type pending struct {
		id, fileID int64
		from, to   string
	}
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.fileID, &p.from, &p.to); err != nil {
			rows.Close()
			return res, err
		}
		list = append(list, p)
	}
	rows.Close()

	for _, p := range list {
		fromThere, toThere := exists(m.full(p.from)), exists(m.full(p.to))
		switch {
		case toThere && !fromThere:
			if err := m.finalize(ctx, p.id, p.fileID, p.to); err != nil {
				return res, err
			}
			res.Completed++
		case fromThere && !toThere:
			m.revert(ctx, p.id)
			res.Reverted++
		case fromThere && toThere:
			m.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'conflict' WHERE id = $1`, p.id)
			res.Conflicts++
		default:
			m.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'missing', moving_to = NULL WHERE id = $1`, p.id)
			res.Missing++
		}
	}
	return res, nil
}

// PlannedMove is one relocation a reorganization would make.
type PlannedMove struct {
	FileID int64  `json:"fileId"`
	WorkID int    `json:"workId"`
	Title  string `json:"title"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Plan is the preview of a reorganization: what would move and where.
type Plan struct {
	Moves     []PlannedMove `json:"moves"`
	Unchanged int           `json:"unchanged"`
	Skipped   []string      `json:"skipped"`
	Hash      string        `json:"hash"`
}

type fileMeta struct {
	fileID int64
	workID int
	path   string
	root   string // only for referenced files
	sha    string
	size   int64
	meta   Meta
	title  string
}

// fileQuery selects files. mode is "managed" unless set; fileID 0 means any.
type fileQuery struct {
	mode            string
	workID          int
	fileID          int64
	onlyUnorganized bool
}

// loadFiles reads what the layout needs about every file, or about one work's
// files that are not organized yet.
func (m *Mover) loadFiles(ctx context.Context, q fileQuery) ([]fileMeta, error) {
	if q.mode == "" {
		q.mode = "managed"
	}
	rows, err := m.DB.QueryContext(ctx, `
		SELECT f.id, e.work_id, l.path, COALESCE(l.root, ''), COALESCE(f.sha256, ''), COALESCE(f.size_bytes, 0),
		       w.original_title, COALESCE(e.language, ''), COALESCE(e.publisher, ''),
		       COALESCE(e.publication_date, ''), COALESCE(w.series, ''), COALESCE(w.series_index, 0), COALESCE(f.format, ''),
		       COALESCE((SELECT array_agg(p.name ORDER BY c.position, p.name)
		                 FROM work_contributors c JOIN person p ON p.id = c.person_id
		                 WHERE c.work_id = e.work_id AND c.role = 'author'), '{}')
		FROM storage_locations l
		JOIN files f ON f.id = l.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE l.mode = $1 AND l.state = 'ok'
		  AND ($2 = 0 OR e.work_id = $2)
		  AND ($3 = 0 OR f.id = $3)
		  AND (NOT $4 OR f.organized_at IS NULL)
		ORDER BY f.id`, q.mode, q.workID, q.fileID, q.onlyUnorganized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileMeta
	for rows.Next() {
		var fm fileMeta
		var authors []string
		var format string
		if err := rows.Scan(&fm.fileID, &fm.workID, &fm.path, &fm.root, &fm.sha, &fm.size, &fm.title, &fm.meta.Language, &fm.meta.Publisher,
			&fm.meta.Year, &fm.meta.Series, &fm.meta.SeriesIndex, &format, pq.Array(&authors)); err != nil {
			return nil, err
		}
		fm.meta.Authors = authors
		fm.meta.Title = fm.title
		fm.meta.Format = format
		if fm.meta.Format == "" {
			fm.meta.Format = strings.TrimPrefix(strings.ToLower(path.Ext(fm.path)), ".")
		}
		out = append(out, fm)
	}
	return out, rows.Err()
}

// plan computes destinations for the given files. Destinations already used by
// another file, planned earlier, or present on disk are never chosen: a suffix
// derived from the file id breaks the tie, so the result does not depend on the
// order files arrive in.
func (m *Mover) plan(ctx context.Context, files []fileMeta) (*Plan, error) {
	taken := map[string]bool{}
	rows, err := m.DB.QueryContext(ctx, `SELECT path FROM storage_locations WHERE mode = 'managed'`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		taken[p] = true
	}
	rows.Close()

	p := &Plan{Moves: []PlannedMove{}, Skipped: []string{}}
	for _, f := range files {
		busy := func(candidate string) bool {
			return candidate != f.path && (taken[candidate] || exists(m.full(candidate)))
		}
		target := Resolve(f.meta, f.fileID, busy)
		if busy(target) {
			p.Skipped = append(p.Skipped, fmt.Sprintf("file %d: no free destination", f.fileID))
			continue
		}
		if target == f.path {
			p.Unchanged++
			continue
		}
		taken[target] = true
		p.Moves = append(p.Moves, PlannedMove{FileID: f.fileID, WorkID: f.workID, Title: f.title, From: f.path, To: target})
	}
	sum := sha256.Sum256(mustJSON(p.Moves))
	p.Hash = hex.EncodeToString(sum[:])
	return p, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// Plan previews reorganizing every managed file to match its current metadata
// (RF-044). Nothing is moved.
func (m *Mover) Plan(ctx context.Context) (*Plan, error) {
	files, err := m.loadFiles(ctx, fileQuery{})
	if err != nil {
		return nil, err
	}
	return m.plan(ctx, files)
}

// Failure is a move that could not be made, with the reason.
type Failure struct {
	FileID int64  `json:"fileId"`
	Error  string `json:"error"`
}

// ApplyResult reports what a reorganization did.
type ApplyResult struct {
	Moved    int       `json:"moved"`
	Failures []Failure `json:"failures"`
}

// ErrPlanChanged: the library changed since the preview.
var ErrPlanChanged = errors.New("the plan changed since the preview; preview again")

// Apply executes a reorganization the admin previewed. It plans again first and
// refuses to run if the result differs from what was shown, so the admin only
// ever confirms what they saw (spec 12.1, "revalidar a prévia").
func (m *Mover) Apply(ctx context.Context, previewHash string) (*ApplyResult, error) {
	p, err := m.Plan(ctx)
	if err != nil {
		return nil, err
	}
	if p.Hash != previewHash {
		return nil, ErrPlanChanged
	}
	res := &ApplyResult{Failures: []Failure{}}
	for _, mv := range p.Moves {
		if err := m.Move(ctx, mv.FileID, mv.To); err != nil {
			res.Failures = append(res.Failures, Failure{FileID: mv.FileID, Error: err.Error()})
			continue
		}
		res.Moved++
	}
	return res, nil
}

// OrganizeWork puts the not-yet-organized managed files of a work at their
// layout path. It is what runs after a file is first analysed (the initial
// layout is applied at incorporation, spec 12.1).
func (m *Mover) OrganizeWork(ctx context.Context, workID int) error {
	files, err := m.loadFiles(ctx, fileQuery{workID: workID, onlyUnorganized: true})
	if err != nil {
		return err
	}
	p, err := m.plan(ctx, files)
	if err != nil {
		return err
	}
	moves := map[int64]string{}
	for _, mv := range p.Moves {
		moves[mv.FileID] = mv.To
	}
	for _, f := range files {
		target, ok := moves[f.fileID]
		if !ok {
			target = f.path // already in place: only record that it is organized
		}
		if err := m.Move(ctx, f.fileID, target); err != nil {
			return err
		}
	}
	return nil
}
