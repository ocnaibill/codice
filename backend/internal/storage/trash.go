package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// TrashDir is where trashed files live, inside the managed storage.
const TrashDir = ".trash"

// ErrNotTrashable: the file has no managed location that can be trashed.
// Referenced files are never trashed: they belong to someone else's directory.
var ErrNotTrashable = errors.New("only available managed files can be trashed")

// ErrNoSuchItem: the trash has no such item.
var ErrNoSuchItem = errors.New("no such item in the trash")

// Trash is the recoverable trash for managed files.
type Trash struct {
	DB   *sql.DB
	Root string
}

// Policy is the automatic cleanup setting. It is off by default and only the
// owner changes it (DEC-042). Days is counted from the moment an item enters.
type Policy struct {
	Enabled bool `json:"enabled"`
	Days    int  `json:"days"`
}

const policyKey = "trash.policy"

// DefaultPolicy: off, with the suggested 30 days ready for when it is enabled (DEC-043).
var DefaultPolicy = Policy{Enabled: false, Days: 30}

// GetPolicy reads the policy in force.
func (t *Trash) GetPolicy(ctx context.Context) (Policy, error) {
	var raw []byte
	err := t.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, policyKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPolicy, nil
	}
	if err != nil {
		return DefaultPolicy, err
	}
	p := DefaultPolicy
	if json.Unmarshal(raw, &p) != nil || p.Days <= 0 {
		return DefaultPolicy, nil
	}
	return p, nil
}

// SetPolicy stores the policy. It only applies to items that enter the trash
// afterwards; existing items keep the date they were given (DEC-043).
func (t *Trash) SetPolicy(ctx context.Context, p Policy, actor string) error {
	if p.Days < 1 || p.Days > 3650 {
		return fmt.Errorf("days must be between 1 and 3650")
	}
	body, _ := json.Marshal(p)
	_, err := t.DB.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_by) VALUES ($1, $2, NULLIF($3, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		policyKey, body, actor)
	return err
}

func (t *Trash) full(rel string) string { return filepath.Join(t.Root, filepath.FromSlash(rel)) }

// purgeAfter is the expiry for an item entering now, or nil when the policy is off.
func (t *Trash) purgeAfter(ctx context.Context) any {
	p, err := t.GetPolicy(ctx)
	if err != nil || !p.Enabled {
		return nil
	}
	return time.Now().Add(time.Duration(p.Days) * 24 * time.Hour)
}

// TrashFile sends one managed file to the trash. The work, the file and its
// history stay in the database, so it can be restored.
func (t *Trash) TrashFile(ctx context.Context, fileID int64, actor string) error {
	var locID int64
	var rel string
	var size sql.NullInt64
	err := t.DB.QueryRowContext(ctx, `
		SELECT l.id, l.path, f.size_bytes FROM storage_locations l JOIN files f ON f.id = l.file_id
		WHERE f.id = $1 AND l.mode = 'managed' AND l.state = 'ok' ORDER BY l.id LIMIT 1`, fileID).Scan(&locID, &rel, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotTrashable
	}
	if err != nil {
		return err
	}
	src := t.full(rel)
	info, err := os.Stat(src)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: the file is not on disk", ErrNotTrashable)
	}
	trashRel := path.Join(TrashDir, fmt.Sprint(fileID), path.Base(rel))
	if err := os.MkdirAll(filepath.Dir(t.full(trashRel)), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, t.full(trashRel)); err != nil {
		return err
	}
	undo := func() { os.Rename(t.full(trashRel), src) }

	tx, err := t.DB.BeginTx(ctx, nil)
	if err != nil {
		undo()
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE storage_locations SET state = 'trashed' WHERE id = $1`, locID); err != nil {
		undo()
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE files SET availability = 'missing' WHERE id = $1`, fileID); err != nil {
		undo()
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO trash_items (file_id, kind, original_path, trash_path, size_bytes, trashed_by, purge_after)
		VALUES ($1, 'file', $2, $3, $4, NULLIF($5, '')::uuid, $6)`,
		fileID, rel, trashRel, info.Size(), actor, t.purgeAfter(ctx)); err != nil {
		undo()
		return err
	}
	if err := tx.Commit(); err != nil {
		undo()
		return err
	}
	t.pruneEmpty(filepath.Dir(src))
	return nil
}

// TrashWork sends every managed file of a work to the trash and reports how many.
func (t *Trash) TrashWork(ctx context.Context, workID int, actor string) (int, error) {
	rows, err := t.DB.QueryContext(ctx, `
		SELECT f.id FROM files f JOIN editions e ON e.id = f.edition_id
		JOIN storage_locations l ON l.file_id = f.id AND l.mode = 'managed' AND l.state = 'ok'
		WHERE e.work_id = $1 ORDER BY f.id`, workID)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if err := t.TrashFile(ctx, id, actor); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (t *Trash) pruneEmpty(dir string) { (&Mover{Root: t.Root}).pruneEmptyDirs(dir) }

// Item is one entry of the trash.
type Item struct {
	ID           int64      `json:"id"`
	Kind         string     `json:"kind"`
	FileID       *int64     `json:"fileId"`
	WorkID       *int       `json:"workId"`
	WorkTitle    string     `json:"workTitle,omitempty"`
	OriginalPath string     `json:"originalPath"`
	SizeBytes    int64      `json:"sizeBytes"`
	TrashedAt    time.Time  `json:"trashedAt"`
	PurgeAfter   *time.Time `json:"purgeAfter"`
}

// List returns the trash newest first and the space it occupies.
func (t *Trash) List(ctx context.Context) ([]Item, int64, error) {
	rows, err := t.DB.QueryContext(ctx, `
		SELECT ti.id, ti.kind, ti.file_id, e.work_id, COALESCE(w.original_title, ''), ti.original_path,
		       ti.size_bytes, ti.trashed_at, ti.purge_after
		FROM trash_items ti
		LEFT JOIN files f ON f.id = ti.file_id
		LEFT JOIN editions e ON e.id = f.edition_id
		LEFT JOIN works w ON w.id = e.work_id
		ORDER BY ti.id DESC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []Item{}
	var total int64
	for rows.Next() {
		var it Item
		var fid sql.NullInt64
		var wid sql.NullInt64
		if err := rows.Scan(&it.ID, &it.Kind, &fid, &wid, &it.WorkTitle, &it.OriginalPath, &it.SizeBytes, &it.TrashedAt, &it.PurgeAfter); err != nil {
			return nil, 0, err
		}
		if fid.Valid {
			it.FileID = &fid.Int64
		}
		if wid.Valid {
			w := int(wid.Int64)
			it.WorkID = &w
		}
		total += it.SizeBytes
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// Restore puts an item back where it was. If that place is now taken, the file
// takes the id-derived name instead of replacing what is there.
func (t *Trash) Restore(ctx context.Context, itemID int64) (string, error) {
	var fileID sql.NullInt64
	var kind, original, trashPath string
	err := t.DB.QueryRowContext(ctx, `SELECT file_id, kind, original_path, trash_path FROM trash_items WHERE id = $1`, itemID).
		Scan(&fileID, &kind, &original, &trashPath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoSuchItem
	}
	if err != nil {
		return "", err
	}
	if !exists(t.full(trashPath)) {
		return "", fmt.Errorf("%w: the file is no longer in the trash folder", ErrNotTrashable)
	}
	target := original
	var taken bool
	t.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM storage_locations WHERE mode = 'managed' AND path = $1 AND state <> 'trashed')`, target).Scan(&taken)
	if taken || exists(t.full(target)) {
		ext := path.Ext(target)
		target = strings.TrimSuffix(target, ext) + " [" + Suffix(fileID.Int64+itemID) + "]" + ext
		if exists(t.full(target)) {
			return "", fmt.Errorf("%w: %s", ErrTargetTaken, target)
		}
	}
	if err := os.MkdirAll(filepath.Dir(t.full(target)), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(t.full(trashPath), t.full(target)); err != nil {
		return "", err
	}
	tx, err := t.DB.BeginTx(ctx, nil)
	if err != nil {
		os.Rename(t.full(target), t.full(trashPath))
		return "", err
	}
	defer tx.Rollback()
	if fileID.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE storage_locations SET state = 'ok', path = $1 WHERE file_id = $2 AND mode = 'managed'`, target, fileID.Int64); err != nil {
			os.Rename(t.full(target), t.full(trashPath))
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE files SET availability = 'available' WHERE id = $1`, fileID.Int64); err != nil {
			os.Rename(t.full(target), t.full(trashPath))
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM trash_items WHERE id = $1`, itemID); err != nil {
		os.Rename(t.full(target), t.full(trashPath))
		return "", err
	}
	if err := tx.Commit(); err != nil {
		os.Rename(t.full(target), t.full(trashPath))
		return "", err
	}
	t.pruneEmpty(filepath.Dir(t.full(trashPath)))
	return target, nil
}

// Delete removes one item for good. This is the only place bytes are destroyed.
// If that leaves a retired work with no files at all, the work itself is deleted
// (its notes stay, unlinked, with their bibliographic reference: RF-039) and so
// are the covers it owned.
func (t *Trash) Delete(ctx context.Context, itemID int64) (freed int64, err error) {
	var fileID sql.NullInt64
	var trashPath string
	var size int64
	err = t.DB.QueryRowContext(ctx, `SELECT file_id, trash_path, size_bytes FROM trash_items WHERE id = $1`, itemID).Scan(&fileID, &trashPath, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoSuchItem
	}
	if err != nil {
		return 0, err
	}
	if rmErr := os.Remove(t.full(trashPath)); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return 0, rmErr
	}

	tx, err := t.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var workID sql.NullInt64
	var covers []string
	if fileID.Valid {
		tx.QueryRowContext(ctx, `SELECT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = $1`, fileID.Int64).Scan(&workID)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM trash_items WHERE id = $1`, itemID); err != nil {
		return 0, err
	}
	if fileID.Valid {
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id = $1`, fileID.Int64); err != nil {
			return 0, err
		}
	}
	if workID.Valid {
		var derr error
		covers, derr = deleteRetiredWorkIfNoBytes(ctx, tx, int(workID.Int64))
		if derr != nil {
			return 0, derr
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, c := range covers {
		if base := path.Base(c); base != "placeholder.svg" {
			os.Remove(filepath.Join(t.Root, "covers", base))
		}
	}
	t.pruneEmpty(filepath.Dir(t.full(trashPath)))
	return size, nil
}

// deleteRetiredWorkIfNoBytes deletes a retired work once nothing of it is stored
// by the server any more (no managed file, and none waiting in the trash). Its
// notes stay, unlinked, with their bibliographic reference (RF-039). Files that
// are only referenced from elsewhere are never touched. It returns the covers to
// remove from disk after the commit.
func deleteRetiredWorkIfNoBytes(ctx context.Context, tx *sql.Tx, workID int) ([]string, error) {
	var retired, bytesLeft bool
	if err := tx.QueryRowContext(ctx, `
		SELECT w.retired_at IS NOT NULL,
		       EXISTS (SELECT 1 FROM storage_locations l JOIN files f ON f.id = l.file_id JOIN editions e ON e.id = f.edition_id
		               WHERE e.work_id = w.id AND l.mode = 'managed' AND l.state IN ('ok', 'moving', 'trashed', 'conflict'))
		FROM works w WHERE w.id = $1`, workID).Scan(&retired, &bytesLeft); err != nil {
		return nil, err
	}
	if !retired || bytesLeft {
		return nil, nil
	}
	var covers []string
	rows, err := tx.QueryContext(ctx, `SELECT cover_url FROM editions WHERE work_id = $1 AND COALESCE(cover_url, '') <> ''`, workID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			covers = append(covers, c)
		}
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM works WHERE id = $1`, workID); err != nil {
		return nil, err
	}
	return covers, nil
}

// PurgeWork is what "delete for good" means for a retired work: its managed files
// go to the trash (recoverable), and if the server stores none of its bytes the
// record is deleted at once. It returns how many files went to the trash and
// whether the work record was deleted.
func (t *Trash) PurgeWork(ctx context.Context, workID int, actor string) (trashed int, deleted bool, err error) {
	trashed, err = t.TrashWork(ctx, workID, actor)
	if err != nil {
		return trashed, false, err
	}
	tx, err := t.DB.BeginTx(ctx, nil)
	if err != nil {
		return trashed, false, err
	}
	defer tx.Rollback()
	covers, err := deleteRetiredWorkIfNoBytes(ctx, tx, workID)
	if err != nil {
		return trashed, false, err
	}
	var still bool
	tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM works WHERE id = $1)`, workID).Scan(&still)
	if err := tx.Commit(); err != nil {
		return trashed, false, err
	}
	for _, c := range covers {
		if base := path.Base(c); base != "placeholder.svg" {
			os.Remove(filepath.Join(t.Root, "covers", base))
		}
	}
	return trashed, !still, nil
}

// Empty deletes everything in the trash for good.
func (t *Trash) Empty(ctx context.Context) (deleted int, freed int64, err error) {
	items, _, err := t.List(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, it := range items {
		n, derr := t.Delete(ctx, it.ID)
		if derr != nil {
			return deleted, freed, derr
		}
		deleted++
		freed += n
	}
	return deleted, freed, nil
}

// PurgeExpired deletes the items whose time is up. An item is checked again just
// before it is deleted, so one that was restored a moment ago is never touched.
func (t *Trash) PurgeExpired(ctx context.Context) (int, error) {
	rows, err := t.DB.QueryContext(ctx, `SELECT id FROM trash_items WHERE purge_after IS NOT NULL AND purge_after <= now() ORDER BY id`)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		var still bool
		t.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM trash_items WHERE id = $1 AND purge_after <= now())`, id).Scan(&still)
		if !still {
			continue
		}
		if _, err := t.Delete(ctx, id); err == nil {
			n++
		}
	}
	return n, nil
}

// PolicyPreview says what applying a policy to the items already in the trash
// would do, including those that are already past due (DEC-043).
type PolicyPreview struct {
	Items      int `json:"items"`
	AlreadyDue int `json:"alreadyDue"`
}

// PreviewPolicy counts the items a retroactive policy would touch.
func (t *Trash) PreviewPolicy(ctx context.Context, days int) (PolicyPreview, error) {
	var p PolicyPreview
	err := t.DB.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE trashed_at + make_interval(days => $1) <= now()) FROM trash_items`, days).Scan(&p.Items, &p.AlreadyDue)
	return p, err
}

// ApplyPolicy sets the expiry of every item already in the trash to its entry
// date plus days. It is an explicit, separate action from changing the policy.
func (t *Trash) ApplyPolicy(ctx context.Context, days int) (int64, error) {
	res, err := t.DB.ExecContext(ctx, `UPDATE trash_items SET purge_after = trashed_at + make_interval(days => $1)`, days)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Orphan is a file in the managed storage that the catalog knows nothing about,
// typically left by a crash between writing a file and recording it.
type Orphan struct {
	Path      string    `json:"path"`
	SizeBytes int64     `json:"sizeBytes"`
	Modified  time.Time `json:"modified"`
}

// orphanGrace keeps files that are too new to judge: an upload is stored a moment
// before its record is committed.
const orphanGrace = 24 * time.Hour

// FindOrphans lists files under the storage root that no location, trash item or
// cover accounts for and that are older than a day. Nothing is changed.
func (t *Trash) FindOrphans(ctx context.Context) ([]Orphan, error) {
	known := map[string]bool{}
	for _, q := range []string{
		`SELECT path FROM storage_locations WHERE mode = 'managed'`,
		`SELECT trash_path FROM trash_items`,
		`SELECT moving_to FROM storage_locations WHERE moving_to IS NOT NULL`,
	} {
		rows, err := t.DB.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p string
			if rows.Scan(&p) == nil {
				known[p] = true
			}
		}
		rows.Close()
	}
	out := []Orphan{}
	err := filepath.WalkDir(t.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(t.Root, p)
		rel = filepath.ToSlash(rel)
		top := strings.SplitN(rel, "/", 2)[0]
		if top == "covers" || top == "cache" || known[rel] {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil || time.Since(info.ModTime()) < orphanGrace {
			return nil
		}
		out = append(out, Orphan{Path: rel, SizeBytes: info.Size(), Modified: info.ModTime()})
		return nil
	})
	return out, err
}

// TrashOrphans moves the named orphans to the trash (recoverable, like any other
// deletion). Only paths that are still orphans right now are moved.
func (t *Trash) TrashOrphans(ctx context.Context, paths []string, actor string) (int, error) {
	current, err := t.FindOrphans(ctx)
	if err != nil {
		return 0, err
	}
	is := map[string]Orphan{}
	for _, o := range current {
		is[o.Path] = o
	}
	moved := 0
	for _, p := range paths {
		o, ok := is[p]
		if !ok {
			continue
		}
		trashRel := path.Join(TrashDir, "orphans", fmt.Sprintf("%d-%s", time.Now().UnixNano(), path.Base(p)))
		if strings.HasPrefix(p, TrashDir+"/") {
			trashRel = p // already in the trash folder: adopt it where it is
		} else {
			if err := os.MkdirAll(filepath.Dir(t.full(trashRel)), 0o755); err != nil {
				return moved, err
			}
			if err := os.Rename(t.full(p), t.full(trashRel)); err != nil {
				return moved, err
			}
		}
		if _, err := t.DB.ExecContext(ctx, `
			INSERT INTO trash_items (file_id, kind, original_path, trash_path, size_bytes, trashed_by, purge_after)
			VALUES (NULL, 'orphan', $1, $2, $3, NULLIF($4, '')::uuid, $5)`, p, trashRel, o.SizeBytes, actor, t.purgeAfter(ctx)); err != nil {
			if trashRel != p {
				os.Rename(t.full(trashRel), t.full(p))
			}
			return moved, err
		}
		t.pruneEmpty(filepath.Dir(t.full(p)))
		moved++
	}
	return moved, nil
}
