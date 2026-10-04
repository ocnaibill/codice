package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

const (
	lastKey         = "last_backup"
	lastVerifiedKey = "last_backup_verified"
)

// Record notes that a package was made, for the interface to show (UI-19) and the audit log. path
// is where the package was written, or "" when it went to standard output.
func Record(ctx context.Context, db *sql.DB, res Result, path string) error {
	last := LastBackup{At: res.Manifest.CreatedAt, Bytes: res.Bytes, Files: res.Manifest.Files.Included,
		IncludesFiles: res.Manifest.IncludesFiles, Encrypted: res.Encrypted, Path: path}
	if path != "" {
		last.Name = filepath.Base(path)
	}
	body, _ := json.Marshal(last)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, lastKey, body); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, "", "backup.create", "instance", "database", map[string]any{
		"bytes": res.Bytes, "encrypted": res.Encrypted, "includesFiles": res.Manifest.IncludesFiles,
		"files": res.Manifest.Files.Total, "included": res.Manifest.Files.Included,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// Last returns the latest package recorded on this instance, or nil.
func Last(ctx context.Context, db *sql.DB) (*LastBackup, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, lastKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l LastBackup
	if json.Unmarshal(raw, &l) != nil {
		return nil, nil
	}
	return &l, nil
}

// RecordVerified notes that the package called name was checked, for the interface to show.
func RecordVerified(ctx context.Context, db *sql.DB, name string, deep bool) error {
	body, _ := json.Marshal(Verified{Name: name, At: time.Now().UTC(), Deep: deep})
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, lastVerifiedKey, body); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, "", "backup.verify", "instance", "database", map[string]any{"name": name, "deep": deep}); err != nil {
		return err
	}
	return tx.Commit()
}

// LastVerified returns the latest check recorded on this instance, or nil.
func LastVerified(ctx context.Context, db *sql.DB) (*Verified, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, lastVerifiedKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v Verified
	if json.Unmarshal(raw, &v) != nil {
		return nil, nil
	}
	return &v, nil
}
