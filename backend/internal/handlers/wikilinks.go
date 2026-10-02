package handlers

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/graph"
)

// [[Concept]] links in the text of a note (#21, DEC-110). The text is the person's and is never changed. What this
// does with it:
//   - a link to a concept the person has is kept as a "mentions" relation from the note to the concept, of origin
//     "wikilink": it comes with the link and goes with it, and the person does not edit it (the text is where it is
//     changed);
//   - a link to a concept that does not exist is pending: nothing is created, and it is shown as such. It is the
//     person who creates the concept, and the link resolves at that moment.
//
// Whether a link is resolved is always read now (a note is read with the concepts its links point to, or null). The
// relations are kept as the text and the concepts change: when the note is saved, and when a concept is created,
// renamed or given another alias, for the notes that cite one of the names that changed.

// queryer is a database or a transaction.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// LinkOut is the concept a [[link]] of a note points to, as it is read.
type LinkOut struct {
	ConceptID   int64  `json:"conceptId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// linkDescription is how much of a concept's description a note carries along: enough for a hint.
const linkDescription = 280

// linkKeys is what the links of a text read as, without repeats.
func linkKeys(links []graph.Link) []string {
	var keys []string
	seen := map[string]bool{}
	for _, l := range links {
		if k := graph.Key(l.Name); !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// conceptsByKey finds the person's concepts that answer to the keys.
func conceptsByKey(ctx context.Context, q queryer, user string, keys []string) (map[string]LinkOut, error) {
	out := map[string]LinkOut{}
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT k.key, c.id, c.name, c.description
		FROM concept_keys k JOIN concepts c ON c.id = k.concept_id
		WHERE k.user_id = $1 AND c.user_id = $1 AND k.key = ANY($2)`, user, pq.Array(keys))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var l LinkOut
		if err := rows.Scan(&key, &l.ConceptID, &l.Name, &l.Description); err != nil {
			return nil, err
		}
		l.Description = clip(l.Description, linkDescription)
		out[key] = l
	}
	return out, rows.Err()
}

// attachLinks gives each note the concepts its links point to, by the name as it was written; null for one that does
// not exist (pending). One query for all the notes.
func attachLinks(ctx context.Context, q queryer, user string, notes []Note) error {
	var keys []string
	cited := make([][]graph.Link, len(notes))
	for i, n := range notes {
		cited[i] = graph.Links(n.Body)
		keys = append(keys, linkKeys(cited[i])...)
	}
	found, err := conceptsByKey(ctx, q, user, keys)
	if err != nil {
		return err
	}
	for i := range notes {
		if len(cited[i]) == 0 {
			continue
		}
		notes[i].Links = map[string]*LinkOut{}
		for _, l := range cited[i] {
			if c, ok := found[graph.Key(l.Name)]; ok {
				notes[i].Links[l.Name] = &c
			} else {
				notes[i].Links[l.Name] = nil
			}
		}
	}
	return nil
}

// syncNoteLinks makes the wikilink relations of a note what its text says: one "mentions" to each concept the links
// point to, and none to a concept they no longer point to. A relation of the person's own (origin "manual") between
// the same two is left as it is, and stays when the link goes.
func syncNoteLinks(ctx context.Context, q queryer, user string, noteID int64, body string) error {
	found, err := conceptsByKey(ctx, q, user, linkKeys(graph.Links(body)))
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(found))
	for _, c := range found {
		ids = append(ids, c.ConceptID)
	}
	if _, err := q.ExecContext(ctx, `
		DELETE FROM relations
		WHERE user_id = $1 AND origin = 'wikilink' AND source_kind = 'note' AND source_id = $2 AND target_id <> ALL($3::bigint[])`,
		user, noteID, pq.Array(ids)); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	source, _ := json.Marshal(label{Label: excerpt(body)})
	_, err = q.ExecContext(ctx, `
		INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id, origin, source_label, target_label)
		SELECT $1::uuid, 'note', $2::bigint, 'mentions', 'concept', c.id, 'wikilink', $3::jsonb, jsonb_build_object('label', c.name)
		FROM concepts c
		WHERE c.user_id = $1::uuid AND c.id = ANY($4::bigint[])
		  AND (SELECT count(*) FROM relations WHERE user_id = $1::uuid) < $5::bigint
		ON CONFLICT DO NOTHING`,
		user, noteID, string(source), pq.Array(ids), maxRelations)
	return err
}

// resyncNotesCiting brings the wikilink relations of the person's notes up to date after the names that concepts answer
// to changed: the notes that cite one of them are the ones that can be affected.
func resyncNotesCiting(ctx context.Context, q queryer, user string, changed []string) error {
	if len(changed) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, k := range changed {
		want[k] = true
	}
	rows, err := q.QueryContext(ctx, `SELECT id, body FROM notes WHERE user_id = $1 AND body LIKE '%[[%'`, user)
	if err != nil {
		return err
	}
	type cite struct {
		id   int64
		body string
	}
	var affected []cite
	for rows.Next() {
		var c cite
		if err := rows.Scan(&c.id, &c.body); err != nil {
			rows.Close()
			return err
		}
		for _, k := range linkKeys(graph.Links(c.body)) {
			if want[k] {
				affected = append(affected, c)
				break
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range affected {
		if err := syncNoteLinks(ctx, q, user, c.id, c.body); err != nil {
			return err
		}
	}
	return nil
}

// symmetricDifference is the keys that are in one of the two and not in the other.
func symmetricDifference(a, b []string) []string {
	in := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, k := range list {
			m[k] = true
		}
		return m
	}
	ma, mb := in(a), in(b)
	var out []string
	for k := range ma {
		if !mb[k] {
			out = append(out, k)
		}
	}
	for k := range mb {
		if !ma[k] {
			out = append(out, k)
		}
	}
	return out
}
