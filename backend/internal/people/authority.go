package people

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
)

// An identifier is what a reference source calls a person: a scheme ("openlibrary") and a value ("OL79034A").
// Both are bounded, and anything that does not look like one is left out rather than stored.
var (
	schemeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	valueRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
)

// IDsFor is what a suggestion's evidence says the identifiers of name are. The evidence lists everyone the
// source credits on the record; the identifiers are those of the credit whose name is this one, and only when
// exactly one credit is (two credits with the same name say nothing about which is which). A name the record
// does not credit has none: an identifier is never taken from a neighbour.
func IDsFor(evidence []byte, name string) map[string]string {
	var e struct {
		Credits []struct {
			Name string            `json:"name"`
			IDs  map[string]string `json:"ids"`
		} `json:"credits"`
	}
	if len(evidence) == 0 || json.Unmarshal(evidence, &e) != nil {
		return nil
	}
	want := Key(name)
	if want == "" {
		return nil
	}
	var found map[string]string
	matches := 0
	for _, c := range e.Credits {
		if Key(c.Name) == want {
			matches++
			found = c.IDs
		}
	}
	if matches != 1 {
		return nil
	}
	out := map[string]string{}
	for scheme, value := range found {
		if schemeRe.MatchString(scheme) && valueRe.MatchString(value) {
			out[scheme] = value
		}
	}
	return out
}

// RecordAuthority keeps the identifiers of a person and, for each, proposes a merge with every other person
// that holds the same one. The proposal is what a human decides; a pair already decided stays decided, and one
// only suggested for sharing words is promoted, because the shared key says more.
func RecordAuthority(ctx context.Context, tx *sql.Tx, personID int, source string, ids map[string]string) error {
	for scheme, value := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO person_authority (person_id, scheme, value, source) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, personID, scheme, value, source); err != nil {
			return err
		}
		evidence, _ := json.Marshal(map[string]string{"scheme": scheme, "value": value})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO person_merge_candidates (person_a, person_b, reason, evidence)
			SELECT LEAST($1::int, o.person_id), GREATEST($1::int, o.person_id), 'authority', $4::jsonb
			FROM person_authority o WHERE o.scheme = $2 AND o.value = $3 AND o.person_id <> $1
			ON CONFLICT (person_a, person_b) DO UPDATE
			  SET reason = 'authority', evidence = EXCLUDED.evidence
			  WHERE person_merge_candidates.state = 'pending'`, personID, scheme, value, string(evidence)); err != nil {
			return err
		}
	}
	return nil
}
