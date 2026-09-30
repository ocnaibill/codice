package people

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// How a name is shown, and how works are sorted by author (#64, DEC-094). It is only presentation: what is
// stored is never changed by it.
const (
	GivenFirst  = "given_first"  // Frank Herbert
	FamilyFirst = "family_first" // Herbert, Frank

	orderKey = "name_order"
)

// ValidOrder says whether s is one of the two orders.
func ValidOrder(s string) bool { return s == GivenFirst || s == FamilyFirst }

// LibraryOrder is the library's default (the owner's choice); given names first until the owner says otherwise.
func LibraryOrder(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) string {
	var raw []byte
	if err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, orderKey).Scan(&raw); err != nil {
		return GivenFirst
	}
	var v string
	if json.Unmarshal(raw, &v) != nil || !ValidOrder(v) {
		return GivenFirst
	}
	return v
}

// SetLibraryOrder records the library's default.
func SetLibraryOrder(ctx context.Context, db *sql.DB, order, actor string) error {
	if !ValidOrder(order) {
		return errors.New("name order must be given_first or family_first")
	}
	value, _ := json.Marshal(order)
	_, err := db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_by) VALUES ($1, $2, NULLIF($3, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		orderKey, value, actor)
	return err
}

// Preference is what an account chose, and what actually applies to it.
type Preference struct {
	Choice    string `json:"choice"`    // the account's own choice, or "" for the library's default
	Library   string `json:"library"`   // the library's default
	Effective string `json:"effective"` // what applies: the choice if there is one, else the library's
}

// OrderFor is the order that applies to an account.
func OrderFor(ctx context.Context, db *sql.DB, userID string) Preference {
	p := Preference{Library: LibraryOrder(ctx, db)}
	var choice sql.NullString
	if userID != "" {
		db.QueryRowContext(ctx, `SELECT name_order FROM users WHERE id = $1`, userID).Scan(&choice)
	}
	p.Choice = choice.String
	p.Effective = p.Library
	if ValidOrder(p.Choice) {
		p.Effective = p.Choice
	}
	return p
}

// SetChoice records an account's own choice; an empty one goes back to the library's default.
func SetChoice(ctx context.Context, db *sql.DB, userID, order string) error {
	if order != "" && !ValidOrder(order) {
		return errors.New("name order must be given_first, family_first or empty")
	}
	_, err := db.ExecContext(ctx, `UPDATE users SET name_order = NULLIF($2, '') WHERE id = $1`, userID, order)
	return err
}
