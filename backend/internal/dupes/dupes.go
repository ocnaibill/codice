// Package dupes finds works that may be the same book and lets an admin decide.
// It only proposes: title, author or ISBN in common is a hint, not proof, so a
// pair is never merged without a person's decision (DEC-029).
package dupes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"golang.org/x/text/unicode/norm"
)

var nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// NormalizeTitle folds a title so that "A Guerra dos Tronos!" and
// "a guerra dos tronos" compare equal: case, accents, punctuation and spacing
// are ignored, and so is a trailing edition note in parentheses or brackets.
func NormalizeTitle(s string) string {
	s = regexp.MustCompile(`[\(\[][^\)\]]*[\)\]]`).ReplaceAllString(s, " ")
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(nonAlnum.ReplaceAllString(b.String(), " "))
}

// NormalizeISBN keeps digits and a trailing X and accepts only 10 or 13 characters,
// so "978-85-7657-313-5" and "9788576573135" match, while noise does not.
func NormalizeISBN(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= '0' && r <= '9') || r == 'X' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) != 10 && len(out) != 13 {
		return ""
	}
	return out
}

type work struct {
	id     int
	title  string
	author string
	isbns  []string
}

// loadWorks reads the works that can be compared: active ones, with their first
// author and every ISBN they carry (on their editions and in their identifiers).
func loadWorks(ctx context.Context, db *sql.DB) ([]work, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT w.id, w.original_title,
		       COALESCE((SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
		                 WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.position, p.name LIMIT 1), ''),
		       COALESCE((SELECT array_agg(v) FROM (
		           SELECT e.isbn AS v FROM editions e WHERE e.work_id = w.id AND e.isbn IS NOT NULL
		           UNION SELECT wi.identifier_value FROM work_identifiers wi WHERE wi.work_id = w.id AND wi.identifier_type = 'isbn'
		       ) x), '{}')
		FROM works w WHERE w.retired_at IS NULL ORDER BY w.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []work
	for rows.Next() {
		var w work
		var raw sql.RawBytes
		if err := rows.Scan(&w.id, &w.title, &w.author, &raw); err != nil {
			return nil, err
		}
		w.isbns = parseTextArray(string(raw))
		out = append(out, w)
	}
	return out, rows.Err()
}

// parseTextArray reads a PostgreSQL text[] literal such as {a,"b c"}.
func parseTextArray(s string) []string {
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		out = append(out, strings.Trim(part, `"`))
	}
	return out
}

func related(a, b work) string {
	seen := map[string]bool{}
	for _, i := range a.isbns {
		if n := NormalizeISBN(i); n != "" {
			seen[n] = true
		}
	}
	for _, i := range b.isbns {
		if n := NormalizeISBN(i); n != "" && seen[n] {
			return "isbn"
		}
	}
	// A title alone is too weak a hint (many books share one); it needs an author
	// as well, and an unknown author never counts.
	ta, tb := NormalizeTitle(a.title), NormalizeTitle(b.title)
	aa, ab := NormalizeTitle(a.author), NormalizeTitle(b.author)
	if ta != "" && ta == tb && aa != "" && aa == ab {
		return "title_author"
	}
	return ""
}

func insertPair(ctx context.Context, db *sql.DB, a, b work, reason string) (bool, error) {
	lo, hi := a.id, b.id
	if lo > hi {
		lo, hi = hi, lo
	}
	res, err := db.ExecContext(ctx, `INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, $3) ON CONFLICT (work_a, work_b) DO NOTHING`, lo, hi, reason)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Detect compares one work with all the others and records new possible
// duplicates. A pair that already exists, whatever its state, is left alone, so a
// dismissed pair does not come back. It returns how many new pairs it found.
func Detect(ctx context.Context, db *sql.DB, workID int) (int, error) {
	works, err := loadWorks(ctx, db)
	if err != nil {
		return 0, err
	}
	var me *work
	for i := range works {
		if works[i].id == workID {
			me = &works[i]
		}
	}
	if me == nil {
		return 0, nil // retired or gone: nothing to compare
	}
	found := 0
	for _, other := range works {
		if other.id == me.id {
			continue
		}
		if reason := related(*me, other); reason != "" {
			if added, err := insertPair(ctx, db, *me, other, reason); err != nil {
				return found, err
			} else if added {
				found++
			}
		}
	}
	return found, nil
}

// DetectAll compares every pair of active works.
func DetectAll(ctx context.Context, db *sql.DB) (int, error) {
	works, err := loadWorks(ctx, db)
	if err != nil {
		return 0, err
	}
	found := 0
	for i := range works {
		for j := i + 1; j < len(works); j++ {
			if reason := related(works[i], works[j]); reason != "" {
				if added, err := insertPair(ctx, db, works[i], works[j], reason); err != nil {
					return found, err
				} else if added {
					found++
				}
			}
		}
	}
	return found, nil
}

// Candidate is a pair waiting for a decision.
type Candidate struct {
	ID     int64   `json:"id"`
	Reason string  `json:"reason"`
	A      Summary `json:"a"`
	B      Summary `json:"b"`
}

// Summary is what an admin needs to tell two works apart.
type Summary struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	Formats  []string `json:"formats"`
	Language string   `json:"language,omitempty"`
	ISBN     string   `json:"isbn,omitempty"`
}

// ListPending returns the pairs waiting for a decision, both works still active.
func ListPending(ctx context.Context, db *sql.DB) ([]Candidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.id, d.reason, d.work_a, d.work_b FROM duplicate_candidates d
		JOIN works a ON a.id = d.work_a AND a.retired_at IS NULL
		JOIN works b ON b.id = d.work_b AND b.retired_at IS NULL
		WHERE d.state = 'pending' ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	type pair struct {
		id     int64
		reason string
		a, b   int
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.reason, &p.a, &p.b); err != nil {
			rows.Close()
			return nil, err
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	out := []Candidate{}
	for _, p := range pairs {
		a, err := summary(ctx, db, p.a)
		if err != nil {
			return nil, err
		}
		b, err := summary(ctx, db, p.b)
		if err != nil {
			return nil, err
		}
		out = append(out, Candidate{ID: p.id, Reason: p.reason, A: a, B: b})
	}
	return out, nil
}

func summary(ctx context.Context, db *sql.DB, id int) (Summary, error) {
	s := Summary{ID: id, Formats: []string{}}
	err := db.QueryRowContext(ctx, `
		SELECT w.original_title,
		       COALESCE((SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
		                 WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.position, p.name LIMIT 1), ''),
		       COALESCE((SELECT e.language FROM editions e WHERE e.work_id = w.id AND e.is_primary), ''),
		       COALESCE((SELECT e.isbn FROM editions e WHERE e.work_id = w.id AND e.is_primary), '')
		FROM works w WHERE w.id = $1`, id).Scan(&s.Title, &s.Author, &s.Language, &s.ISBN)
	if err != nil {
		return s, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT COALESCE(f.format, '') FROM files f JOIN editions e ON e.id = f.edition_id WHERE e.work_id = $1 ORDER BY 1`, id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil && f != "" {
			s.Formats = append(s.Formats, f)
		}
	}
	return s, rows.Err()
}

// Errors of a decision.
var (
	ErrNotFound = errors.New("no such pending candidate")
	ErrBadKeep  = errors.New("keep must be one of the two works")
	ErrRetired  = errors.New("both works must be active")
)

// Dismiss records that the pair is not a duplicate, so it is not proposed again.
func Dismiss(ctx context.Context, db *sql.DB, id int64, actor string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE duplicate_candidates SET state = 'dismissed', decided_at = now(), decided_by = NULLIF($2, '')::uuid
		WHERE id = $1 AND state = 'pending'`, id, actor)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return audit.Record(ctx, db, actor, "duplicate.dismiss", "duplicate", fmt.Sprint(id), nil)
}

// Link makes the other work part of keep: its editions and files become
// editions of the kept work (not primary), and its notes, favorites, tags and
// identifiers move over. Reading positions belong to files, which keep their ids,
// so they are untouched. The other work record is then deleted. This is the one
// irreversible step, so it only runs on an explicit decision of an admin.
func Link(ctx context.Context, db *sql.DB, id int64, keep int, actor string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var a, b int
	err = tx.QueryRowContext(ctx, `SELECT work_a, work_b FROM duplicate_candidates WHERE id = $1 AND state = 'pending' FOR UPDATE`, id).Scan(&a, &b)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if keep != a && keep != b {
		return ErrBadKeep
	}
	other := a
	if keep == a {
		other = b
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM works WHERE id IN ($1, $2) AND retired_at IS NULL`, keep, other).Scan(&active); err != nil {
		return err
	}
	if active != 2 {
		return ErrRetired
	}

	var moved int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM editions WHERE work_id = $1`, other).Scan(&moved); err != nil {
		return err
	}
	for _, q := range []string{
		`UPDATE editions SET work_id = $1, is_primary = FALSE WHERE work_id = $2`,
		`UPDATE notes SET work_id = $1 WHERE work_id = $2`,
		`INSERT INTO favorites (user_id, work_id, created_at) SELECT user_id, $1, created_at FROM favorites WHERE work_id = $2 ON CONFLICT DO NOTHING`,
		`INSERT INTO work_tags (work_id, tag_id) SELECT $1, tag_id FROM work_tags WHERE work_id = $2 ON CONFLICT DO NOTHING`,
		`INSERT INTO work_identifiers (work_id, identifier_type, identifier_value)
		   SELECT $1, identifier_type, identifier_value FROM work_identifiers WHERE work_id = $2 ON CONFLICT DO NOTHING`,
	} {
		if _, err := tx.ExecContext(ctx, q, keep, other); err != nil {
			return err
		}
	}
	// The pair record goes with the other work (cascade); the audit entry keeps the story.
	if err := audit.Record(ctx, tx, actor, "duplicate.link", "work", fmt.Sprint(keep),
		map[string]any{"absorbed": other, "editions": moved}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM works WHERE id = $1`, other); err != nil {
		return err
	}
	return tx.Commit()
}
