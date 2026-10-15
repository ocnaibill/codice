package wiki

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// Who is who, decided by people (DEC-181): joining two entities, keeping them apart, and taking a mention away from an entity. The
// decisions are rules of the work or series, applied now to the wiki that exists and again to every generation that replaces it.

// Key is what two renderings of a name share: no accents, no case, no punctuation, single spaces.
func Key(s string) string { return fold(s) }

// Rule kinds.
const (
	RuleSame     = "same"
	RuleDistinct = "distinct"
	RuleMention  = "mention"
)

// IdentityRule is one decision.
type IdentityRule struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	A         string    `json:"a"`
	B         string    `json:"b"`
	DecidedAt time.Time `json:"decidedAt"`
}

// ErrIdentity is a decision that cannot be made as asked; the message says why, for the person.
type ErrIdentity struct{ Msg string }

func (e ErrIdentity) Error() string { return e.Msg }

type scopeCols struct {
	work       sql.NullInt64
	collection sql.NullInt64
}

func projectScope(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, projectID int64) (scopeCols, error) {
	var s scopeCols
	err := q.QueryRowContext(ctx, `SELECT work_id, collection_id FROM wiki_projects WHERE id = $1`, projectID).Scan(&s.work, &s.collection)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

type entityRef struct {
	id       int64
	slug     string
	name     string
	page     bool
	chapters int
}

// entitiesByKey maps every name and alias (folded) of the project to the entities that carry it. A key two entities carry is ambiguous.
func entitiesByKey(ctx context.Context, tx *sql.Tx, projectID int64) (map[string][]entityRef, map[int64]entityRef, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT e.id, e.slug, e.name, e.page, (SELECT count(*) FROM wiki_appearances a WHERE a.entity_id = e.id)
		FROM wiki_entities e WHERE e.project_id = $1`, projectID)
	if err != nil {
		return nil, nil, err
	}
	byID := map[int64]entityRef{}
	byKey := map[string][]entityRef{}
	for rows.Next() {
		var r entityRef
		if err := rows.Scan(&r.id, &r.slug, &r.name, &r.page, &r.chapters); err != nil {
			rows.Close()
			return nil, nil, err
		}
		byID[r.id] = r
		byKey[fold(r.name)] = append(byKey[fold(r.name)], r)
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT a.entity_id, a.alias FROM wiki_aliases a JOIN wiki_entities e ON e.id = a.entity_id WHERE e.project_id = $1`, projectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var alias string
		if err := rows.Scan(&id, &alias); err != nil {
			return nil, nil, err
		}
		k := fold(alias)
		if r, ok := byID[id]; ok && k != "" {
			dup := false
			for _, x := range byKey[k] {
				dup = dup || x.id == id
			}
			if !dup {
				byKey[k] = append(byKey[k], r)
			}
		}
	}
	return byKey, byID, rows.Err()
}

// find says which entity a key is, when it is only one.
func find(byKey map[string][]entityRef, key string) (entityRef, bool) {
	list := byKey[key]
	if len(list) != 1 {
		return entityRef{}, false
	}
	return list[0], true
}

// MergeEntities puts drop into keep: where it appears, its names (its own name becomes an alias from its first appearance), its states, the
// sections keep does not have, its ties and the edges of the graph; whatever pointed to drop points to keep. Returns the entity that stays.
func mergeEntities(ctx context.Context, tx *sql.Tx, projectID int64, a, b entityRef) (entityRef, error) {
	keep, drop := a, b
	// The one with a page of its own stays; then the one seen in more chapters; then the first name in order.
	switch {
	case b.page && !a.page:
		keep, drop = b, a
	case a.page == b.page && (b.chapters > a.chapters || (b.chapters == a.chapters && b.slug < a.slug)):
		keep, drop = b, a
	}
	exec := func(q string, args ...any) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	}
	// Its name, from the first place it was seen.
	if err := exec(`
		INSERT INTO wiki_aliases (entity_id, alias, book, chapter)
		SELECT $1, $3, book, chapter FROM wiki_appearances WHERE entity_id = $2 ORDER BY book, chapter LIMIT 1
		ON CONFLICT DO NOTHING`, keep.id, drop.id, drop.name); err != nil {
		return keep, err
	}
	for _, q := range []string{
		`INSERT INTO wiki_appearances (entity_id, book, chapter) SELECT $1, book, chapter FROM wiki_appearances WHERE entity_id = $2 ON CONFLICT DO NOTHING`,
		`INSERT INTO wiki_aliases (entity_id, alias, book, chapter) SELECT $1, alias, book, chapter FROM wiki_aliases WHERE entity_id = $2 ON CONFLICT DO NOTHING`,
		`INSERT INTO wiki_states (entity_id, book, chapter, text) SELECT $1, book, chapter, text FROM wiki_states WHERE entity_id = $2 ON CONFLICT DO NOTHING`,
	} {
		if err := exec(q, keep.id, drop.id); err != nil {
			return keep, err
		}
	}
	// The name of keep is not its own alias.
	if err := exec(`DELETE FROM wiki_aliases WHERE entity_id = $1 AND lower(alias) = lower($2)`, keep.id, keep.name); err != nil {
		return keep, err
	}
	if err := exec(`
		UPDATE wiki_sections s SET entity_id = $1
		WHERE s.entity_id = $2 AND NOT EXISTS (
			SELECT 1 FROM wiki_sections k WHERE k.entity_id = $1 AND k.kind = s.kind AND COALESCE(k.book, -1) = COALESCE(s.book, -1)
			  AND COALESCE(k.chapter_from, 0) = COALESCE(s.chapter_from, 0) AND k.part = s.part)`, keep.id, drop.id); err != nil {
		return keep, err
	}
	if err := exec(`
		UPDATE wiki_relations r SET entity_id = $1
		WHERE r.entity_id = $2 AND NOT EXISTS (
			SELECT 1 FROM wiki_relations k WHERE k.entity_id = $1 AND lower(k.other_name) = lower(r.other_name) AND k.grp = r.grp)`, keep.id, drop.id); err != nil {
		return keep, err
	}
	if err := exec(`UPDATE wiki_relations SET other_id = $1 WHERE other_id = $2`, keep.id, drop.id); err != nil {
		return keep, err
	}
	if err := exec(`DELETE FROM wiki_relations WHERE entity_id = $1 AND other_id = $1`, keep.id); err != nil {
		return keep, err
	}
	if err := exec(`UPDATE wiki_edges SET a_id = $1 WHERE a_id = $2`, keep.id, drop.id); err != nil {
		return keep, err
	}
	if err := exec(`UPDATE wiki_edges SET b_id = $1 WHERE b_id = $2`, keep.id, drop.id); err != nil {
		return keep, err
	}
	if err := exec(`DELETE FROM wiki_edges WHERE project_id = $1 AND a_id = b_id`, projectID); err != nil {
		return keep, err
	}
	// A suggestion about drop is about keep now; one between the two is answered.
	if err := exec(`UPDATE wiki_suggestions SET entity_slug = $2 WHERE project_id = $1 AND entity_slug = $3`, projectID, keep.slug, drop.slug); err != nil {
		return keep, err
	}
	if err := exec(`UPDATE wiki_suggestions SET other_slug = $2 WHERE project_id = $1 AND other_slug = $3`, projectID, keep.slug, drop.slug); err != nil {
		return keep, err
	}
	if err := exec(`UPDATE wiki_suggestions SET state = 'accepted', decided_at = COALESCE(decided_at, now())
	                WHERE project_id = $1 AND kind = 'merge' AND entity_slug = other_slug AND state = 'open'`, projectID); err != nil {
		return keep, err
	}
	if err := exec(`DELETE FROM wiki_entities WHERE id = $1`, drop.id); err != nil {
		return keep, err
	}
	if drop.page && !keep.page {
		if err := exec(`UPDATE wiki_entities SET page = true WHERE id = $1`, keep.id); err != nil {
			return keep, err
		}
	}
	return keep, nil
}

// removeMention takes a name away from an entity: the alias goes. Its own name is not a mention that can go.
func removeMention(ctx context.Context, tx *sql.Tx, e entityRef, mentionKey string) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT alias FROM wiki_aliases WHERE entity_id = $1`, e.id)
	if err != nil {
		return 0, err
	}
	var gone []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return 0, err
		}
		if fold(a) == mentionKey {
			gone = append(gone, a)
		}
	}
	rows.Close()
	for _, a := range gone {
		if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_aliases WHERE entity_id = $1 AND alias = $2`, e.id, a); err != nil {
			return 0, err
		}
	}
	return len(gone), nil
}

// Decision is what the staff asks for on a wiki.
type Decision struct {
	Action     string `json:"action"` // merge, distinct or mention
	Entity     string `json:"entity"`
	Other      string `json:"other"`
	Mention    string `json:"mention"`
	Suggestion int64  `json:"suggestion"`
}

func saveRule(ctx context.Context, tx *sql.Tx, s scopeCols, kind, aName, bName, userID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO wiki_identity_rules (work_id, collection_id, kind, a_name, a_key, b_name, b_key, decided_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid)
		ON CONFLICT DO NOTHING`, s.work, s.collection, kind, aName, fold(aName), bName, fold(bName), userID)
	return err
}

// Decide applies a decision to the wiki and keeps it as a rule of the work or series. It returns the slug of the entity the page should
// show afterwards: the one asked about, or the one it was joined into.
func Decide(ctx context.Context, db *sql.DB, projectID int64, d Decision, userID string) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	scope, err := projectScope(ctx, tx, projectID)
	if err != nil {
		return "", err
	}
	var a, b entityRef
	shown := d.Entity
	load := func(slug string) (entityRef, error) {
		var r entityRef
		err := tx.QueryRowContext(ctx, `
			SELECT e.id, e.slug, e.name, e.page, (SELECT count(*) FROM wiki_appearances x WHERE x.entity_id = e.id)
			FROM wiki_entities e WHERE e.project_id = $1 AND e.slug = $2`, projectID, slug).Scan(&r.id, &r.slug, &r.name, &r.page, &r.chapters)
		if errors.Is(err, sql.ErrNoRows) {
			return r, ErrNotFound
		}
		return r, err
	}
	if a, err = load(d.Entity); err != nil {
		return "", err
	}
	switch d.Action {
	case "merge", "distinct":
		if b, err = load(d.Other); err != nil {
			return "", err
		}
		if a.id == b.id {
			return "", ErrIdentity{"Escolha outra entidade: uma não pode ser juntada nem separada dela mesma."}
		}
		if d.Action == "merge" {
			if err := saveRule(ctx, tx, scope, RuleSame, a.name, b.name, userID); err != nil {
				return "", err
			}
			kept, err := mergeEntities(ctx, tx, projectID, a, b)
			if err != nil {
				return "", err
			}
			shown = kept.slug
		} else {
			if err := saveRule(ctx, tx, scope, RuleDistinct, a.name, b.name, userID); err != nil {
				return "", err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE wiki_suggestions SET state = 'declined', decided_by = NULLIF($4, '')::uuid, decided_at = now()
				WHERE project_id = $1 AND kind = 'merge' AND state = 'open'
				  AND ((entity_slug = $2 AND other_slug = $3) OR (entity_slug = $3 AND other_slug = $2))`, projectID, a.slug, b.slug, userID); err != nil {
				return "", err
			}
		}
	case "mention":
		key := fold(d.Mention)
		if key == "" {
			return "", ErrIdentity{"Diga qual menção sair da entidade."}
		}
		if key == fold(a.name) {
			return "", ErrIdentity{"O nome da própria entidade não sai: separe uma de suas menções (os outros nomes)."}
		}
		n, err := removeMention(ctx, tx, a, key)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", ErrIdentity{"Essa menção não é um dos nomes da entidade."}
		}
		if err := saveRule(ctx, tx, scope, RuleMention, a.name, d.Mention, userID); err != nil {
			return "", err
		}
	default:
		return "", ErrIdentity{"Decisão desconhecida."}
	}
	if d.Suggestion > 0 {
		state := "accepted"
		if d.Action == "distinct" {
			state = "declined"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE wiki_suggestions SET state = $3, decided_by = NULLIF($4, '')::uuid, decided_at = now()
			WHERE project_id = $1 AND id = $2 AND state = 'open'`, projectID, d.Suggestion, state, userID); err != nil {
			return "", err
		}
	}
	if err := audit.Record(ctx, tx, userID, "wiki.identity."+d.Action, "wiki", strconv.FormatInt(projectID, 10),
		map[string]any{"entity": d.Entity, "other": d.Other, "mention": d.Mention}); err != nil {
		return "", err
	}
	return shown, tx.Commit()
}

// Rules lists the decisions of the work or series the project is of.
func Rules(ctx context.Context, db *sql.DB, projectID int64) ([]IdentityRule, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.id, r.kind, r.a_name, r.b_name, r.decided_at FROM wiki_identity_rules r
		JOIN wiki_projects p ON (r.work_id IS NOT NULL AND r.work_id = p.work_id) OR (r.collection_id IS NOT NULL AND r.collection_id = p.collection_id)
		WHERE p.id = $1 ORDER BY r.decided_at DESC, r.id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IdentityRule{}
	for rows.Next() {
		var r IdentityRule
		if err := rows.Scan(&r.ID, &r.Kind, &r.A, &r.B, &r.DecidedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ForgetRule takes a decision back: the next generation no longer follows it. What was merged in the wiki that exists stays merged
// until the wiki is generated again.
func ForgetRule(ctx context.Context, db *sql.DB, projectID, ruleID int64, userID string) error {
	res, err := db.ExecContext(ctx, `
		DELETE FROM wiki_identity_rules r USING wiki_projects p
		WHERE r.id = $2 AND p.id = $1 AND ((r.work_id IS NOT NULL AND r.work_id = p.work_id) OR (r.collection_id IS NOT NULL AND r.collection_id = p.collection_id))`,
		projectID, ruleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return audit.Record(ctx, db, userID, "wiki.identity.forget", "wiki", strconv.FormatInt(projectID, 10), map[string]any{"rule": ruleID})
}

// applyRules puts the decisions of the work or series on a project that was just written: the entities are found by name, and what the
// staff joined is joined again, what they took away is taken away again, and the suggestions they answered are answered.
func applyRules(ctx context.Context, tx *sql.Tx, projectID int64) error {
	scope, err := projectScope(ctx, tx, projectID)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT kind, a_key, b_key FROM wiki_identity_rules
		WHERE (work_id IS NOT NULL AND work_id = $1) OR (collection_id IS NOT NULL AND collection_id = $2)
		ORDER BY CASE kind WHEN 'same' THEN 0 ELSE 1 END, id`, scope.work, scope.collection)
	if err != nil {
		return err
	}
	type rule struct{ kind, a, b string }
	var rules []rule
	for rows.Next() {
		var r rule
		if err := rows.Scan(&r.kind, &r.a, &r.b); err != nil {
			rows.Close()
			return err
		}
		rules = append(rules, r)
	}
	rows.Close()
	if len(rules) == 0 {
		return nil
	}
	for _, r := range rules {
		byKey, _, err := entitiesByKey(ctx, tx, projectID)
		if err != nil {
			return err
		}
		a, okA := find(byKey, r.a)
		if !okA {
			continue
		}
		switch r.kind {
		case RuleSame:
			if b, ok := find(byKey, r.b); ok && b.id != a.id {
				if _, err := mergeEntities(ctx, tx, projectID, a, b); err != nil {
					return err
				}
			}
		case RuleMention:
			if _, err := removeMention(ctx, tx, a, r.b); err != nil {
				return err
			}
		case RuleDistinct:
			if b, ok := find(byKey, r.b); ok && b.id != a.id {
				if _, err := tx.ExecContext(ctx, `
					UPDATE wiki_suggestions SET state = 'declined', decided_at = COALESCE(decided_at, now())
					WHERE project_id = $1 AND kind = 'merge' AND state = 'open'
					  AND ((entity_slug = $2 AND other_slug = $3) OR (entity_slug = $3 AND other_slug = $2))`, projectID, a.slug, b.slug); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
