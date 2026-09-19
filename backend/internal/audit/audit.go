// Package audit records administrative actions: who did what to what. Entries
// never carry secrets (plan section 10).
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Execer is satisfied by *sql.DB and *sql.Tx, so an entry can be written in the
// same transaction as the action it describes.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Record stores one entry. actorID may be empty for system actions. The
// username is copied so the entry stays readable after the account is deleted.
func Record(ctx context.Context, q Execer, actorID, action, targetType, targetID string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO audit_log (actor_id, actor_username, action, target_type, target_id, details)
		VALUES (NULLIF($1, '')::uuid,
		        (SELECT username FROM users WHERE id = NULLIF($1, '')::uuid),
		        $2, $3, $4, $5)`,
		actorID, action, targetType, targetID, payload)
	return err
}
