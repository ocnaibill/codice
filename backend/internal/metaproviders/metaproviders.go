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
	// Sends lists what leaves the instance: "title" is the title of the work, on every analysis; "isbn" is the ISBN the file
	// carries, when it has one, for the providers that find a book by it (DEC-142); "author_key" is
	// the key Open Library gave an author an administrator accepted, to ask it for the identifiers it knows;
	// "page_title" is the title of the page Wikidata says the work has on Wikipedia, which is all Wikipedia is asked.
	Sends []string `json:"sends"`
	// Key says whether the provider takes an API key from the environment of the worker: "optional" (it works
	// without, within a lower limit), "required" (it does not work without) or empty (it has none).
	Key string `json:"key"`
}

// Known are the providers the worker can ask, in the order it asks them.
var Known = []Info{
	{ID: "google_books", Name: "Google Books", Sends: []string{"title", "isbn"}, Key: "required"},
	{ID: "openlibrary", Name: "Open Library", Sends: []string{"title", "isbn", "author_key"}},
	{ID: "comicvine", Name: "ComicVine", Sends: []string{"title"}, Key: "required"},
	{ID: "anilist", Name: "AniList", Sends: []string{"title"}},
	{ID: "mangadex", Name: "MangaDex", Sends: []string{"title"}},
	{ID: "wikidata", Name: "Wikidata", Sends: []string{"title"}},
	{ID: "wikipedia", Name: "Wikipedia", Sends: []string{"page_title"}},
}

// KeysSettingKey is where the worker tells which API keys it has: {"comicvine": true, ...}. It tells whether each
// is set and never the key, which stays in the environment of the worker.
const KeysSettingKey = "metadata.providers.keys"

// ErrNoKey is for turning on a provider that cannot work without an API key the worker does not have.
var ErrNoKey = errors.New("the provider needs an API key that is not configured")

// Status is a provider and whether it is on.
type Status struct {
	Info
	Enabled bool `json:"enabled"`
	// KeyConfigured is whether the worker has the API key: null when the provider has none or the worker has not
	// said yet (it says at start).
	KeyConfigured *bool `json:"keyConfigured"`
}

func find(id string) (Info, bool) {
	for _, k := range Known {
		if k.ID == id {
			return k, true
		}
	}
	return Info{}, false
}

// read is the plain booleans of a setting: only a JSON true or false counts, anything else is as if absent.
func read(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, key string) (map[string]bool, error) {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&raw)
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
		if b, ok := v.(bool); ok {
			out[id] = b
		}
	}
	return out, nil
}

// chosen are the providers the owner turned on: only a plain true counts.
func chosen(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}) (map[string]bool, error) {
	all, err := read(ctx, q, SettingKey)
	if err != nil {
		return nil, err
	}
	on := map[string]bool{}
	for id, b := range all {
		if b {
			on[id] = true
		}
	}
	return on, nil
}

// List is every provider with whether it is on: all of them are off until the owner says otherwise.
func List(ctx context.Context, db *sql.DB) ([]Status, error) {
	on, err := chosen(ctx, db)
	if err != nil {
		return nil, err
	}
	keys, err := read(ctx, db, KeysSettingKey)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(Known))
	for _, k := range Known {
		st := Status{Info: k, Enabled: on[k.ID]}
		if has, said := keys[k.ID]; said && k.Key != "" {
			st.KeyConfigured = &has
		}
		out = append(out, st)
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
	info, ok := find(id)
	if !ok {
		return ErrUnknown
	}
	if enabled && info.Key == "required" {
		// Turning on what cannot work only to find out later: the worker says whether it has the key. If it has not
		// said yet, it is allowed, and the administration shows that it is not known.
		keys, err := read(ctx, db, KeysSettingKey)
		if err != nil {
			return err
		}
		if has, said := keys[id]; said && !has {
			return ErrNoKey
		}
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
