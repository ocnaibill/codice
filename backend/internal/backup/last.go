package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

const lastKey = "last_backup"

// Record notes that a package was made, for the interface to show (UI-19) and the audit log.
func Record(ctx context.Context, db *sql.DB, res Result) error {
	last := LastBackup{At: res.Manifest.CreatedAt, Bytes: res.Bytes, Files: res.Manifest.Files.Included,
		IncludesFiles: res.Manifest.IncludesFiles, Encrypted: res.Encrypted}
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
