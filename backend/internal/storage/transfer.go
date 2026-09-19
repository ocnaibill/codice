package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	// ErrNotReferenced: the file is not a referenced one, so there is nothing to transfer.
	ErrNotReferenced = errors.New("the file is not in a referenced location")
	// ErrSourceMissing: the original is not where it was catalogued.
	ErrSourceMissing = errors.New("the original file is missing")
	// ErrSourceChanged: the original no longer has the content that was catalogued.
	ErrSourceChanged = errors.New("the original file changed since it was catalogued")
)

// Transferrer moves referenced files into the managed storage. Managed import
// from a server directory follows the same rules (DEC-033, DEC-034):
//
//  1. the original must still have the catalogued content;
//  2. it is copied to a staging file while hashing, flushed, and compared;
//  3. it is published at its layout path, and in ONE transaction the file is
//     switched to its managed location and the original is recorded as waiting
//     for removal;
//  4. only then is the original removed, and only if it has not changed since
//     it was read. If it cannot be removed the transfer still stands and the
//     original stays on the pending list with the reason.
//
// A failure at any point keeps at least one valid copy.
type Transferrer struct {
	Mover *Mover
	// BeforeRemoval, when set, runs just before the original is examined for
	// removal. It exists for tests that need the original to change at that
	// exact moment; it is nil in production.
	BeforeRemoval func()
}

// TransferResult says how a transfer ended.
type TransferResult struct {
	Target         string `json:"target"`
	OriginRemoved  bool   `json:"originRemoved"`
	CleanupPending bool   `json:"cleanupPending"`
}

// MoveToManaged transfers one referenced file into the managed storage.
func (t *Transferrer) MoveToManaged(ctx context.Context, fileID int64) (*TransferResult, error) {
	m := t.Mover
	files, err := m.loadFiles(ctx, fileQuery{mode: "referenced", fileID: fileID})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNotReferenced
	}
	fm := files[0]
	source, ok := AbsPath("", "referenced", fm.root, fm.path)
	if !ok {
		return nil, ErrUnsafePath
	}
	// The directory may have changed since it was catalogued: only follow what
	// still resolves inside its root.
	realSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSourceMissing, fm.path)
	}
	if realRoot, rerr := filepath.EvalSymlinks(fm.root); rerr != nil || !within(realSource, realRoot) {
		return nil, ErrSourceChanged
	}
	before, err := os.Stat(realSource)
	if err != nil || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrSourceMissing, fm.path)
	}

	// Choose where it goes: the layout path, or its id-suffixed variant.
	plan, err := m.plan(ctx, []fileMeta{{fileID: fm.fileID, workID: fm.workID, path: "", meta: fm.meta, title: fm.title}})
	if err != nil {
		return nil, err
	}
	if len(plan.Moves) != 1 {
		return nil, fmt.Errorf("%w: no free destination", ErrTargetTaken)
	}
	target := plan.Moves[0].To

	// Copy to staging, hashing as it goes.
	staging := filepath.Join(m.Root, ".staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(staging, "xfer-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	published := false
	defer func() {
		tmp.Close()
		if !published {
			os.Remove(tmpPath)
		}
	}()
	src, err := os.Open(realSource)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceMissing, err)
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), src)
	src.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("copying the file: %w", copyErr)
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if fm.sha != "" && sum != fm.sha {
		m.DB.ExecContext(ctx, `UPDATE storage_locations SET state = 'conflict' WHERE file_id = $1 AND mode = 'referenced'`, fm.fileID)
		return nil, ErrSourceChanged
	}

	// Publish at the destination. An existing file is never overwritten.
	if exists(m.full(target)) {
		return nil, fmt.Errorf("%w: %s", ErrTargetTaken, target)
	}
	if err := os.MkdirAll(filepath.Dir(m.full(target)), 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, m.full(target)); err != nil {
		return nil, err
	}
	published = true
	undo := func() { os.Remove(m.full(target)); m.pruneEmptyDirs(filepath.Dir(m.full(target))) }

	// One transaction: managed location, and the original waiting for removal.
	if err := t.switchToManaged(ctx, fm, target, sum, source); err != nil {
		undo()
		return nil, err
	}

	res := &TransferResult{Target: target}
	if t.BeforeRemoval != nil {
		t.BeforeRemoval()
	}
	res.OriginRemoved = RemoveVerifiedOrigin(ctx, m.DB, source, sum, before)
	res.CleanupPending = !res.OriginRemoved
	return res, nil
}

func (t *Transferrer) switchToManaged(ctx context.Context, fm fileMeta, target, sum, source string) error {
	tx, err := t.Mover.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE storage_locations SET mode = 'managed', root = NULL, path = $1, state = 'ok', moving_to = NULL
		WHERE file_id = $2 AND mode = 'referenced'`, target, fm.fileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("the file was transferred by someone else in the meantime")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE files SET organized_at = now(), sha256 = COALESCE(sha256, $2) WHERE id = $1`, fm.fileID, sum); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO storage_cleanups (path, sha256, file_id, reason)
		VALUES ($1, $2, $3, 'waiting to be removed')
		ON CONFLICT (path) DO UPDATE SET sha256 = EXCLUDED.sha256, file_id = EXCLUDED.file_id`, source, sum, fm.fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveVerifiedOrigin deletes a leftover original, but only if it is still the
// file that was copied: same size and modification time as when it was read, and
// the same content. Otherwise, or if it cannot be deleted, it is kept and the
// reason is recorded on the pending list. It reports whether the file is gone.
func RemoveVerifiedOrigin(ctx context.Context, db *sql.DB, path, sum string, before os.FileInfo) bool {
	pend := func(reason string) bool {
		db.ExecContext(ctx, `
			INSERT INTO storage_cleanups (path, sha256, reason, attempted_at) VALUES ($1, $2, $3, now())
			ON CONFLICT (path) DO UPDATE SET reason = EXCLUDED.reason, attempted_at = now()`, path, sum, reason)
		return false
	}
	now, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		db.ExecContext(ctx, `DELETE FROM storage_cleanups WHERE path = $1`, path)
		return true // already gone
	}
	if err != nil {
		return pend("could not read the original: " + err.Error())
	}
	if before != nil && (now.Size() != before.Size() || !now.ModTime().Equal(before.ModTime())) {
		return pend("the original changed after it was copied; it was kept")
	}
	if got, _, err := hashPath(path); err != nil || got != sum {
		return pend("the original no longer has the content that was copied; it was kept")
	}
	if err := os.Remove(path); err != nil {
		return pend("the original could not be removed: " + err.Error())
	}
	db.ExecContext(ctx, `DELETE FROM storage_cleanups WHERE path = $1`, path)
	return true
}

func hashPath(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// Cleanup is one original still waiting to be removed.
type Cleanup struct {
	ID          int64  `json:"id"`
	Path        string `json:"path"`
	Reason      string `json:"reason"`
	AttemptedAt string `json:"attemptedAt,omitempty"`
}

// PendingCleanups lists originals that could not be removed yet.
func PendingCleanups(ctx context.Context, db *sql.DB) ([]Cleanup, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, path, reason, COALESCE(attempted_at::text, '') FROM storage_cleanups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Cleanup{}
	for rows.Next() {
		var c Cleanup
		if err := rows.Scan(&c.ID, &c.Path, &c.Reason, &c.AttemptedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RetryCleanups tries again to remove every pending original, with the same
// checks as the first time. It returns how many were removed and how many remain.
func RetryCleanups(ctx context.Context, db *sql.DB) (removed, remaining int, err error) {
	rows, err := db.QueryContext(ctx, `SELECT path, sha256 FROM storage_cleanups ORDER BY id`)
	if err != nil {
		return 0, 0, err
	}
	type item struct{ path, sum string }
	var list []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.path, &it.sum) == nil {
			list = append(list, it)
		}
	}
	rows.Close()
	for _, it := range list {
		if RemoveVerifiedOrigin(ctx, db, it.path, it.sum, nil) {
			removed++
		} else {
			remaining++
		}
	}
	return removed, remaining, nil
}
