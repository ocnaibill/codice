package people

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// maxGroup is how many spellings of one name are compared with each other: more than that is not a person
// written in several ways, it is a name that many people have.
const maxGroup = 6

var (
	ErrNotFound = errors.New("candidate not found")
	ErrBadKeep  = errors.New("keep must be one of the two people of the pair")
)

// Detect proposes the pairs of people whose names are made of the same words, in any order ("Herbert,
// Frank" and "Frank Herbert"), and those who hold the same identifier of a reference source. A pair that already exists, whatever its state, is left alone, so one that
// was dismissed does not come back. It returns how many new pairs it found.
func Detect(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM person ORDER BY id`)
	if err != nil {
		return 0, err
	}
	groups := map[string][]int{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return 0, err
		}
		if k := Key(name); k != "" {
			groups[k] = append(groups[k], id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(groups))
	for k, ids := range groups {
		if len(ids) > 1 && len(ids) <= maxGroup {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	found := 0
	for _, k := range keys {
		ids := groups[k]
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				res, err := db.ExecContext(ctx, `INSERT INTO person_merge_candidates (person_a, person_b) VALUES ($1, $2) ON CONFLICT DO NOTHING`, ids[i], ids[j])
				if err != nil {
					return found, err
				}
				if n, _ := res.RowsAffected(); n > 0 {
					found++
				}
			}
		}
	}
	shared, err := detectShared(ctx, db)
	return found + shared, err
}

// detectShared proposes the people who hold the same identifier from a reference source (a key of Open Library,
// a Wikidata, VIAF or ISNI identifier): the strongest reason to think two spellings are one person, so it
// promotes a pair that was only proposed for sharing words and names the identifier as the evidence. A pair
// that was dismissed stays dismissed. When two share several, the evidence is the one that says the most. It
// returns how many new pairs it found.
func detectShared(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `
		INSERT INTO person_merge_candidates (person_a, person_b, reason, evidence)
		SELECT DISTINCT ON (a.person_id, b.person_id)
		       a.person_id, b.person_id, 'authority', jsonb_build_object('scheme', a.scheme, 'value', a.value)
		FROM person_authority a
		JOIN person_authority b ON b.scheme = a.scheme AND b.value = a.value AND a.person_id < b.person_id
		ORDER BY a.person_id, b.person_id, COALESCE(array_position(ARRAY['wikidata', 'viaf', 'isni', 'openlibrary'], a.scheme::text), 99), a.scheme
		ON CONFLICT (person_a, person_b) DO UPDATE SET reason = 'authority', evidence = EXCLUDED.evidence
		  WHERE person_merge_candidates.state = 'pending' AND person_merge_candidates.reason <> 'authority'
		RETURNING (xmax = 0)`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var inserted bool
		if err := rows.Scan(&inserted); err != nil {
			return found, err
		}
		if inserted {
			found++
		}
	}
	return found, rows.Err()
}

// Person is one side of a pair, with enough to tell who it is.
type Person struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Works   int      `json:"works"`
	Titles  []string `json:"titles"`
	// Authorities are the identifiers a reference source has for the person ("openlibrary:OL79034A").
	Authorities []string `json:"authorities"`
}

// Candidate is a pair waiting for a decision.
type Candidate struct {
	ID int64 `json:"id"`
	// Reason is why the pair was proposed: "authority" (both hold the same identifier from a reference source,
	// which Evidence names) or "words" (their names are made of the same words).
	Reason   string          `json:"reason"`
	Evidence json.RawMessage `json:"evidence"`
	A        Person          `json:"a"`
	B        Person          `json:"b"`
}

func person(ctx context.Context, db *sql.DB, id int) (Person, error) {
	p := Person{ID: id, Aliases: []string{}, Titles: []string{}, Authorities: []string{}}
	if err := db.QueryRowContext(ctx, `SELECT name FROM person WHERE id = $1`, id).Scan(&p.Name); err != nil {
		return p, err
	}
	rows, err := db.QueryContext(ctx, `SELECT alias FROM person_alias WHERE person_id = $1 ORDER BY alias`, id)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return p, err
		}
		p.Aliases = append(p.Aliases, a)
	}
	rows.Close()
	ids, err := db.QueryContext(ctx, `SELECT scheme || ':' || value FROM person_authority WHERE person_id = $1 ORDER BY scheme, value`, id)
	if err != nil {
		return p, err
	}
	for ids.Next() {
		var a string
		if err := ids.Scan(&a); err != nil {
			ids.Close()
			return p, err
		}
		p.Authorities = append(p.Authorities, a)
	}
	ids.Close()
	if err := db.QueryRowContext(ctx, `
		SELECT count(DISTINCT c.work_id) FROM work_contributors c JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		WHERE c.person_id = $1`, id).Scan(&p.Works); err != nil {
		return p, err
	}
	titles, err := db.QueryContext(ctx, `
		SELECT w.original_title FROM work_contributors c JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		WHERE c.person_id = $1 GROUP BY w.id, w.original_title ORDER BY w.id LIMIT 3`, id)
	if err != nil {
		return p, err
	}
	defer titles.Close()
	for titles.Next() {
		var t string
		if err := titles.Scan(&t); err != nil {
			return p, err
		}
		p.Titles = append(p.Titles, t)
	}
	return p, titles.Err()
}

// ListPending returns the pairs waiting for a decision.
func ListPending(ctx context.Context, db *sql.DB) ([]Candidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, person_a, person_b, reason, evidence FROM person_merge_candidates WHERE state = 'pending'
		ORDER BY (reason = 'authority') DESC, id`)
	if err != nil {
		return nil, err
	}
	type pair struct {
		id       int64
		a, b     int
		reason   string
		evidence []byte
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.a, &p.b, &p.reason, &p.evidence); err != nil {
			rows.Close()
			return nil, err
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	out := []Candidate{}
	for _, p := range pairs {
		a, err := person(ctx, db, p.a)
		if err != nil {
			return nil, err
		}
		b, err := person(ctx, db, p.b)
		if err != nil {
			return nil, err
		}
		out = append(out, Candidate{ID: p.id, Reason: p.reason, Evidence: p.evidence, A: a, B: b})
	}
	return out, nil
}

// Dismiss records that the two are not the same person, for good.
func Dismiss(ctx context.Context, db *sql.DB, id int64, actor string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE person_merge_candidates SET state = 'dismissed', decided_at = now(), decided_by = NULLIF($2, '')::uuid
		WHERE id = $1 AND state = 'pending'`, id, actor)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return audit.Record(ctx, db, actor, "person.not_same", "person_merge", fmt.Sprint(id), nil)
}

// Merge makes the other person of the pair the same as keep: the works it is an author of become keep's, its
// name and its aliases become aliases of keep, and it is deleted. It cannot be undone, so it runs only on the
// decision of an admin. Nothing that was written is lost: it is all an alias now.
func Merge(ctx context.Context, db *sql.DB, id int64, keep int, actor string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var a, b int
	err = tx.QueryRowContext(ctx, `SELECT person_a, person_b FROM person_merge_candidates WHERE id = $1 AND state = 'pending' FOR UPDATE`, id).Scan(&a, &b)
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
	var otherName, keepName string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM person WHERE id = $1`, other).Scan(&otherName); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT name FROM person WHERE id = $1`, keep).Scan(&keepName); err != nil {
		return err
	}
	var moved int
	if err := tx.QueryRowContext(ctx, `SELECT count(DISTINCT work_id) FROM work_contributors WHERE person_id = $1`, other).Scan(&moved); err != nil {
		return err
	}
	// A human saying these two are the same, with a comma in one of them, says where the surname ends.
	if family, given, ok := SplitFromPair(keepName, otherName); ok {
		if _, err := tx.ExecContext(ctx, `UPDATE person SET family_name = $2, given_name = $3 WHERE id = $1 AND family_name IS NULL`, keep, family, given); err != nil {
			return err
		}
	}
	for _, st := range []struct {
		query string
		args  []any
	}{
		// Its works are keep's, once each: a work that had both keeps the one row it has.
		{`UPDATE work_contributors SET person_id = $1
		   WHERE person_id = $2 AND NOT EXISTS (
		     SELECT 1 FROM work_contributors o WHERE o.work_id = work_contributors.work_id AND o.person_id = $1 AND o.role = work_contributors.role)`, []any{keep, other}},
		{`INSERT INTO person_alias (person_id, alias) SELECT $1, name FROM person WHERE id = $2 ON CONFLICT DO NOTHING`, []any{keep, other}},
		{`INSERT INTO person_alias (person_id, alias) SELECT $1, alias FROM person_alias WHERE person_id = $2 ON CONFLICT DO NOTHING`, []any{keep, other}},
		// The identifiers it had are keep's too.
		{`INSERT INTO person_authority (person_id, scheme, value, source) SELECT $1, scheme, value, source FROM person_authority WHERE person_id = $2 ON CONFLICT DO NOTHING`, []any{keep, other}},
		// What is left of it (the rows of a work that had both) goes with it, and so do its pairs.
		{`DELETE FROM person WHERE id = $1`, []any{other}},
	} {
		if _, err := tx.ExecContext(ctx, st.query, st.args...); err != nil {
			return err
		}
	}
	if err := audit.Record(ctx, tx, actor, "person.merge", "person", fmt.Sprint(keep),
		map[string]any{"kept": keepName, "absorbed": otherName, "works": moved}); err != nil {
		return err
	}
	return tx.Commit()
}

// Resolve is the person a name written in a file stands for, created if it does not exist. The person goes by
// the name people say, what was written is kept as an alias, and the surname and the given names are kept when
// the writing says which is which for certain (#64). What is already known about a person is never replaced.
func Resolve(ctx context.Context, tx *sql.Tx, raw string) (int, error) {
	p := Parse(raw)
	written := strings.Join(strings.Fields(raw), " ")
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM person WHERE name = $1`, p.Name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `INSERT INTO person (name) VALUES ($1) RETURNING id`, p.Name).Scan(&id)
	}
	if err != nil {
		return 0, err
	}
	if p.Family != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE person SET family_name = $2, given_name = $3 WHERE id = $1 AND family_name IS NULL`, id, p.Family, p.Given); err != nil {
			return 0, err
		}
	}
	if p.Changed && written != p.Name {
		if _, err := tx.ExecContext(ctx, `INSERT INTO person_alias (person_id, alias) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, written); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// ErrNotTheName is for parts that are not the words the name has.
var ErrNotTheName = errors.New("family and given must be made of the words of the name")

// SetParts records which words of a person's name are the surname and which are the given names, for when it
// was not known for certain (or was wrong). Empty parts take the division away. It never renames the person.
func SetParts(ctx context.Context, db *sql.DB, id int, family, given, actor string) error {
	family, given = strings.Join(strings.Fields(family), " "), strings.Join(strings.Fields(given), " ")
	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM person WHERE id = $1`, id).Scan(&name); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if family == "" && given == "" {
		if _, err := db.ExecContext(ctx, `UPDATE person SET family_name = NULL, given_name = NULL WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, db, actor, "person.parts", "person", fmt.Sprint(id), map[string]any{"family": "", "given": ""})
	}
	if family == "" || Key(family+" "+given) != Key(name) {
		return ErrNotTheName
	}
	var g any
	if given != "" {
		g = given
	}
	if _, err := db.ExecContext(ctx, `UPDATE person SET family_name = $2, given_name = $3 WHERE id = $1`, id, family, g); err != nil {
		return err
	}
	return audit.Record(ctx, db, actor, "person.parts", "person", fmt.Sprint(id), map[string]any{"family": family, "given": given})
}
