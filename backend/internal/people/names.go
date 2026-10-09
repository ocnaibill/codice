package people

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// Suggestion is the division of a name into surname and given names that the system proposes. It is only a proposal: which word
// is the surname depends on the culture ("Gabriel García Márquez", "Ursula K. Le Guin"), so nothing is saved until someone from the
// staff confirms it, changing the words if it is not right (DEC-094, DEC-139).
type Suggestion struct {
	Family string `json:"family"`
	Given  string `json:"given"`
}

// suffixes follow a name without being part of any surname, so the last word says nothing about it.
var suffixes = map[string]bool{"jr": true, "sr": true, "junior": true, "filho": true, "neto": true, "sobrinho": true, "ii": true, "iii": true, "iv": true}

// SuggestParts proposes the surname of a name written the way people say it. A name written with one comma is the catalogue's
// way ("Herbert, Frank": what comes before the comma is the surname); otherwise the last word is. It proposes nothing for a name
// of one word, for a line that seems to name several people, and for a name that ends in a suffix, which are for a person to read.
func SuggestParts(name string) (Suggestion, bool) {
	clean := strings.Join(strings.Fields(name), " ")
	if clean == "" || strings.ContainsAny(clean, ";/&") {
		return Suggestion{}, false
	}
	for _, and := range []string{" and ", " e ", " y ", " et ", " und "} {
		if strings.Contains(strings.ToLower(clean), and) {
			return Suggestion{}, false
		}
	}
	if strings.Contains(clean, ",") {
		parts := splitParts(clean)
		if len(parts) != 2 {
			return Suggestion{}, false
		}
		return Suggestion{Family: parts[0], Given: parts[1]}, true
	}
	words := strings.Fields(clean)
	if len(words) < 2 || suffixes[fold(words[len(words)-1])] {
		return Suggestion{}, false
	}
	last := len(words) - 1
	return Suggestion{Family: words[last], Given: strings.Join(words[:last], " ")}, true
}

// NameRow is a person in the lists of names, with how many works they have. In the list of names to divide it carries the
// division proposed; in the list of the ones already dealt with, the division they have (or that they have none).
type NameRow struct {
	ID         int         `json:"id"`
	Name       string      `json:"name"`
	Works      int         `json:"works"`
	Titles     []string    `json:"titles"`
	Suggestion *Suggestion `json:"suggestion"`
	Family     string      `json:"family,omitempty"`
	Given      string      `json:"given,omitempty"`
	Undivided  bool        `json:"undivided,omitempty"`
}

// NamesPage is one page of them, the ones with most works first.
type NamesPage struct {
	Data       []NameRow `json:"data"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	Limit      int       `json:"limit"`
	TotalPages int       `json:"totalPages"`
}

// ListNames lists the people with at least one work in the catalog, narrowed to the names that contain `q` (without regard to
// case or accents). By default they are the ones whose name has more than one word and whose surname nobody has told apart, nor
// said that there is none; with `dealtWith` they are the ones whose surname was told apart or who were said to have none.
func ListNames(ctx context.Context, db *sql.DB, dealtWith bool, q string, page, limit int) (NamesPage, error) {
	out := NamesPage{Data: []NameRow{}, Page: page, Limit: limit}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(q)) + "%"
	state := `p.family_name IS NULL AND NOT p.name_undivided AND array_length(regexp_split_to_array(btrim(p.name), '\s+'), 1) > 1`
	if dealtWith {
		state = `(p.family_name IS NOT NULL OR p.name_undivided)`
	}
	pending := `
		FROM person p
		WHERE ` + state + `
		  AND unaccent(p.name) ILIKE unaccent($1)
		  AND EXISTS (SELECT 1 FROM work_contributors c JOIN works w ON w.id = c.work_id WHERE c.person_id = p.id AND w.retired_at IS NULL)`
	if err := db.QueryRowContext(ctx, `SELECT count(*) `+pending, like).Scan(&out.Total); err != nil {
		return out, err
	}
	out.TotalPages = (out.Total + limit - 1) / limit
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.name, COALESCE(p.family_name, ''), COALESCE(p.given_name, ''), p.name_undivided,
		       (SELECT count(DISTINCT c.work_id) FROM work_contributors c JOIN works w ON w.id = c.work_id WHERE c.person_id = p.id AND w.retired_at IS NULL) AS works,
		       COALESCE((SELECT array_agg(t) FROM (
		           SELECT w.original_title AS t FROM work_contributors c JOIN works w ON w.id = c.work_id
		           WHERE c.person_id = p.id AND w.retired_at IS NULL GROUP BY w.id, w.original_title ORDER BY w.id LIMIT 3) x), '{}')
		`+pending+`
		ORDER BY works DESC, p.name, p.id
		LIMIT $2 OFFSET $3`, like, limit, (page-1)*limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var n NameRow
		var titles pq.StringArray
		if err := rows.Scan(&n.ID, &n.Name, &n.Family, &n.Given, &n.Undivided, &n.Works, &titles); err != nil {
			return out, err
		}
		n.Titles = []string(titles)
		if !dealtWith {
			if s, ok := SuggestParts(n.Name); ok {
				n.Suggestion = &s
			}
		}
		out.Data = append(out.Data, n)
	}
	return out, rows.Err()
}

// SetUndivided records that a name has no surname to tell apart (or takes that back). It clears any division the person had:
// the two cannot both be true.
func SetUndivided(ctx context.Context, db *sql.DB, id int, undivided bool, actor string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE person SET name_undivided = $2, family_name = CASE WHEN $2 THEN NULL ELSE family_name END,
		                  given_name = CASE WHEN $2 THEN NULL ELSE given_name END
		WHERE id = $1`, id, undivided)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return audit.Record(ctx, db, actor, "person.undivided", "person", fmt.Sprint(id), map[string]any{"undivided": undivided})
}
