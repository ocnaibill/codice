// Package metaproviders keeps which external metadata providers the owner allowed (DEC-045, #68). Asking a
// provider sends the title of a work to a third party, so nothing is asked unless the owner turned that provider
// on, one by one. The worker reads the same setting before every request.
package metaproviders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// SettingKey is where the choice is kept: {"openlibrary": true, ...}. A provider that is not in it is off.
const SettingKey = "metadata.providers"

// ErrUnknown is for a provider the library does not have.
var ErrUnknown = errors.New("unknown metadata provider")

// Info says what a provider is and what asking it sends, for the owner to decide knowing.
type Info struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Sends lists what leaves the instance on every request: "title" is the title of the work.
	Sends []string `json:"sends"`
	// NeedsKey is whether the provider only works with an API key set in the environment of the worker.
	NeedsKey bool `json:"needsKey"`
}

// Known are the providers the worker can ask, in the order it asks them.
var Known = []Info{
	{ID: "google_books", Name: "Google Books", Sends: []string{"title"}},
	{ID: "openlibrary", Name: "Open Library", Sends: []string{"title"}},
	{ID: "comicvine", Name: "ComicVine", Sends: []string{"title"}, NeedsKey: true},
}

// Status is a provider and whether it is on.
type Status struct {
	Info
	Enabled bool `json:"enabled"`
}

func known(id string) bool {
	for _, k := range Known {
		if k.ID == id {
			return true
		}
	}
	return false
}

func chosen(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}) (map[string]bool, error) {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, SettingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var all map[string]any
	if json.Unmarshal(raw, &all) != nil {
		return map[string]bool{}, nil // not a choice: nothing is on
	}
	out := map[string]bool{}
	for id, v := range all {
		if b, ok := v.(bool); ok && b { // only a plain yes counts
			out[id] = true
		}
	}
	return out, nil
}

// List is every provider with whether it is on: all of them are off until the owner says otherwise.
func List(ctx context.Context, db *sql.DB) ([]Status, error) {
	on, err := chosen(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(Known))
	for _, k := range Known {
		out = append(out, Status{Info: k, Enabled: on[k.ID]})
	}
	return out, nil
}

// AnyEnabled is whether some provider may be asked.
func AnyEnabled(ctx context.Context, db *sql.DB) (bool, error) {
	list, err := List(ctx, db)
	if err != nil {
		return false, err
	}
	for _, s := range list {
		if s.Enabled {
			return true, nil
		}
	}
	return false, nil
}

// Set turns one provider on or off, leaving the others as they are, and records who did it.
func Set(ctx context.Context, db *sql.DB, id string, enabled bool, actor string) error {
	if !known(id) {
		return ErrUnknown
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// One statement, so two providers changed at once do not undo each other. Something in the setting that is not
	// a choice is replaced by one.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_by) VALUES ($1, jsonb_build_object($2::text, $3::boolean), NULLIF($4, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = CASE WHEN jsonb_typeof(settings.value) = 'object' THEN settings.value ELSE '{}'::jsonb END
		                                       || jsonb_build_object($2::text, $3::boolean),
		                                 updated_by = NULLIF($4, '')::uuid, updated_at = now()`,
		SettingKey, id, enabled, actor); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, actor, "providers.set", "metadata_provider", id, map[string]any{"enabled": enabled}); err != nil {
		return err
	}
	return tx.Commit()
}
