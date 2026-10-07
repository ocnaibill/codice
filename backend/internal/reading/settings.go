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

// Device is the kind of device a choice is for (#180): a screen touched with the fingers, and a computer with a mouse. A phone and
// a computer are read in different places and at different distances, so what looks right on one is not what looks right on the
// other.
type Device string

const (
	Touch   Device = "touch"
	Desktop Device = "desktop"
)

// Valid says whether this is one of the two.
func (d Device) Valid() bool { return d == Touch || d == Desktop }

// Prefs is everything an account has chosen for how the text looks: one choice for each kind of device, and whether they are kept
// the same. A kind that has not been chosen for is nil (the reader then has its own defaults, or the other kind's, once).
type Prefs struct {
	Shared  bool      `json:"shared"`
	Touch   *Settings `json:"touch"`
	Desktop *Settings `json:"desktop"`
}

func (p *Prefs) of(d Device) **Settings {
	if d == Touch {
		return &p.Touch
	}
	return &p.Desktop
}

// Change is what a request asks for. Exactly one of its forms is set:
//   - Settings for Device: the choice of one kind of device (of both, when they are kept the same);
//   - Same: keep both kinds the same (from the choice of From), or let each be its own;
//   - Legacy: a choice without a kind (what the app sent before there were kinds): it is the choice of both.
type Change struct {
	Device   Device
	Settings *Settings
	Same     *bool
	From     Device
	Legacy   *Settings
}

// ParseChange reads a request: refusing a field it does not know, a kind that is not one, and a choice that is not in the lists.
func ParseChange(raw []byte) (Change, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	for _, field := range []string{"theme", "font", "size", "spacing", "margins", "justify"} {
		if _, ok := keys[field]; ok {
			legacy, err := Parse(raw)
			if err != nil {
				return Change{}, err
			}
			return Change{Legacy: &legacy}, nil
		}
	}
	var req struct {
		Device   Device          `json:"device"`
		Settings json.RawMessage `json:"settings"`
		Shared   *bool           `json:"shared"`
		From     Device          `json:"from"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.More() {
		return Change{}, fmt.Errorf("%w: the request", ErrInvalid)
	}
	switch {
	case req.Shared != nil && req.Device == "" && req.Settings == nil:
		if *req.Shared && !req.From.Valid() {
			return Change{}, fmt.Errorf("%w: from", ErrInvalid)
		}
		if !*req.Shared && req.From != "" {
			return Change{}, fmt.Errorf("%w: from", ErrInvalid)
		}
		return Change{Same: req.Shared, From: req.From}, nil
	case req.Shared == nil && req.From == "" && req.Device.Valid() && len(req.Settings) > 0:
		s, err := Parse(req.Settings)
		if err != nil {
			return Change{}, err
		}
		return Change{Device: req.Device, Settings: &s}, nil
	}
	return Change{}, fmt.Errorf("%w: the request", ErrInvalid)
}

func parsePrefs(raw []byte) (*Prefs, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, err
	}
	if _, legacy := keys["theme"]; legacy { // a choice made before there were kinds: it is for both
		s, err := Parse(raw)
		if err != nil {
			return nil, err
		}
		return &Prefs{Touch: &s, Desktop: &s}, nil
	}
	var p Prefs
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	for _, s := range []*Settings{p.Touch, p.Desktop} {
		if s != nil && s.Validate() != nil {
			return nil, ErrInvalid
		}
	}
	return &p, nil
}

// Get is the choice of an account, or nil when it has made none (the reader then has its own defaults).
func Get(ctx context.Context, db *sql.DB, userID string) (*Prefs, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT reader_prefs FROM users WHERE id = $1`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := parsePrefs(raw)
	if err != nil {
		return nil, nil // what is kept and is no longer valid (a list changed) is as if there were none
	}
	if p.Touch == nil && p.Desktop == nil {
		return nil, nil
	}
	return p, nil
}

// Apply keeps a change to the choice of an account, reading and writing it as one step (two devices that change their own kind at
// the same time do not undo each other).
func Apply(ctx context.Context, db *sql.DB, userID string, c Change) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT reader_prefs FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&raw); err != nil {
		return err
	}
	p := &Prefs{}
	if raw != nil {
		if kept, err := parsePrefs(raw); err == nil {
			p = kept
		}
	}
	switch {
	case c.Legacy != nil:
		p.Touch, p.Desktop = c.Legacy, c.Legacy
	case c.Settings != nil:
		*p.of(c.Device) = c.Settings
		if p.Shared {
			p.Touch, p.Desktop = c.Settings, c.Settings
		}
	case c.Same != nil:
		p.Shared = *c.Same
		if p.Shared {
			from := *p.of(c.From)
			if from == nil { // nothing chosen for that kind yet: the other one's is what both have
				from = *p.of(map[Device]Device{Touch: Desktop, Desktop: Touch}[c.From])
			}
			p.Touch, p.Desktop = from, from
		}
	default:
		return fmt.Errorf("%w: nothing to change", ErrInvalid)
	}
	out, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET reader_prefs = $2::jsonb WHERE id = $1`, userID, string(out)); err != nil {
		return err
	}
	return tx.Commit()
}

// Clear takes the choice of an account away.
func Clear(ctx context.Context, db *sql.DB, userID string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET reader_prefs = NULL WHERE id = $1`, userID)
	return err
}
