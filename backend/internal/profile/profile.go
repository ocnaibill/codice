// Package profile holds what a person says about themselves in their own account: how they want to be called (#179).
package profile

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxDisplayName is the longest name a person may give, in characters (the column is varchar(60)).
const MaxDisplayName = 60

// ErrTooLong: the name is longer than MaxDisplayName.
var ErrTooLong = errors.New("display name is too long")

// CleanDisplayName is a name as it will be kept: the spaces around it and the runs of spaces inside it are one space, and
// no control character, line break, zero-width mark or direction override stays in it (a name is shown in the middle of
// a sentence). An empty result means "call me by my user name". A name longer than MaxDisplayName is refused, and not cut:
// a person who typed it should see that it did not fit.
func CleanDisplayName(name string) (string, error) {
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > MaxDisplayName {
		return "", ErrTooLong
	}
	return name, nil
}

// Name is how an account wants to be called ("" when it wants its user name), and whether it was asked yet.
type Name struct {
	Display string
	Asked   bool
}

// Get reads how an account wants to be called.
func Get(ctx context.Context, db *sql.DB, userID string) (Name, error) {
	var n Name
	var display sql.NullString
	var asked sql.NullTime
	err := db.QueryRowContext(ctx, `SELECT display_name, display_name_asked_at FROM users WHERE id = $1`, userID).Scan(&display, &asked)
	if err != nil {
		return n, err
	}
	return Name{Display: display.String, Asked: asked.Valid}, nil
}

// Set records how an account wants to be called, cleaned; an empty name goes back to the user name. Either way the
// account has been asked.
func Set(ctx context.Context, db *sql.DB, userID, name string) error {
	clean, err := CleanDisplayName(name)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE users SET display_name = NULLIF($2, ''), display_name_asked_at = COALESCE(display_name_asked_at, now())
		WHERE id = $1`, userID, clean)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
