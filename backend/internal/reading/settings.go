// Package reading is how a person wants the text of a book to look (#106): each choice is one of a short closed list, so that
// every combination can be read (no color, no font, no size is typed by hand), and the same lists are the frontend's
// (frontend/src/features/reader/epubThemes.js, which a test there checks against this file).
package reading

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// The closed lists of the choices.
var (
	Themes   = []string{"branco", "papel", "sepia", "cinza", "escuro", "preto"}
	Fonts    = []string{"livro", "serifada", "sem-serifa", "dislexia"}
	Spacings = []string{"livro", "media", "ampla"}
	Margins  = []string{"livro", "estreita", "media", "larga"}
)

// The size of the letter is a percentage of the usual one, in steps.
const (
	SizeMin  = 80
	SizeMax  = 200
	SizeStep = 10
)

// Settings is one person's choice. Every field is required: a choice is made of all of them, which the reader fills with
// what it has when the person changes one.
type Settings struct {
	Theme   string `json:"theme"`
	Font    string `json:"font"`
	Size    int    `json:"size"`
	Spacing string `json:"spacing"`
	Margins string `json:"margins"`
	Justify bool   `json:"justify"`
}

// ErrInvalid is a choice that is not one of the lists.
var ErrInvalid = errors.New("the reading preferences are not valid")

func in(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// Validate says whether every choice is one the lists have.
func (s Settings) Validate() error {
	switch {
	case !in(Themes, s.Theme):
		return fmt.Errorf("%w: theme", ErrInvalid)
	case !in(Fonts, s.Font):
		return fmt.Errorf("%w: font", ErrInvalid)
	case s.Size < SizeMin || s.Size > SizeMax || (s.Size-SizeMin)%SizeStep != 0:
		return fmt.Errorf("%w: size", ErrInvalid)
	case !in(Spacings, s.Spacing):
		return fmt.Errorf("%w: spacing", ErrInvalid)
	case !in(Margins, s.Margins):
		return fmt.Errorf("%w: margins", ErrInvalid)
	}
	return nil
}

// Parse reads a choice from JSON, refusing a field it does not know and a value that is not in the lists.
func Parse(raw []byte) (Settings, error) {
	var s Settings
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if dec.More() {
		return Settings{}, fmt.Errorf("%w: more than one value", ErrInvalid)
	}
	return s, s.Validate()
}

// Get is the choice of an account, or nil when it has made none (the reader then has its own defaults).
func Get(ctx context.Context, db *sql.DB, userID string) (*Settings, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT reader_prefs FROM users WHERE id = $1`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s, err := Parse(raw)
	if err != nil {
		return nil, nil // what is kept and is no longer valid (a list changed) is as if there were none
	}
	return &s, nil
}

// Set keeps the choice of an account; nil takes it away.
func Set(ctx context.Context, db *sql.DB, userID string, s *Settings) error {
	if s == nil {
		_, err := db.ExecContext(ctx, `UPDATE users SET reader_prefs = NULL WHERE id = $1`, userID)
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE users SET reader_prefs = $2::jsonb WHERE id = $1`, userID, string(raw))
	return err
}
