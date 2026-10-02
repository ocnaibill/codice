// Package versions puts the files of one book under one work by hand (#37, DEC-028): an edition moves
// from a work to another (Join), or goes back to a work of its own (Split), and a pair of works an
// admin said are not the same is remembered (NotTheSame).
//
// Nothing here deletes: the work that is left with no edition is retired (reversible, DEC-038), its files
// are the ones that moved, and each file keeps its id, so its reading positions, notes and everything
// else that points at the file are untouched. What belongs to the work as a whole (the history of what
// people finished, the "whole work finished" mark, favorites, tags, identifiers, authors, the positions
// they accepted, the relations they drew in the graph) is carried to the work that stays, because it used to be lost with the work that
// went (dupes.Link deleted it, and all of that went with it).
package versions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

var (
	ErrNotFound    = errors.New("work or edition not found")
	ErrSameWork    = errors.New("a work cannot be joined to itself")
	ErrRetired     = errors.New("a retired work cannot be joined: restore it first")
	ErrOnlyEdition = errors.New("the only edition of a work cannot be separated from it")
)

// JoinResult says what moved.
type JoinResult struct {
	Editions int
}

// lockActive locks the works, in order of id so two decisions cannot wait for each other, and checks
// that each exists and is not retired.
func lockActive(ctx context.Context, tx *sql.Tx, ids ...int) error {
	lo, hi := ids[0], ids[len(ids)-1]
	if lo > hi {
		lo, hi = hi, lo
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, retired_at IS NOT NULL FROM works WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, lo, hi)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[int]bool{}
	for rows.Next() {
		var id int
		var retired bool
		if err := rows.Scan(&id, &retired); err != nil {
			return err
		}
		if retired {
			return ErrRetired
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if !found[id] {
			return ErrNotFound
		}
	}
	return nil
}

// ensurePrimary gives a work that has editions but no primary one the oldest of them.
func ensurePrimary(ctx context.Context, tx *sql.Tx, work int) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE editions SET is_primary = TRUE
		WHERE id = (SELECT id FROM editions WHERE work_id = $1 ORDER BY id LIMIT 1)
		  AND NOT EXISTS (SELECT 1 FROM editions WHERE work_id = $1 AND is_primary)`, work)
	return err
}

// carryFavorites gives the work that receives an edition the favorites of the one it leaves: a person who
// had the book among their favorites still has it.
func carryFavorites(ctx context.Context, tx *sql.Tx, to, from int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO favorites (user_id, work_id, created_at) SELECT user_id, $1, created_at FROM favorites WHERE work_id = $2 ON CONFLICT DO NOTHING`, to, from)
	return err
}

// carryWhole copies what describes a work as a whole, from the one that goes to the one that stays,
// without overwriting what the other already has.
func carryWhole(ctx context.Context, tx *sql.Tx, to, from int) error {
	if err := carryFavorites(ctx, tx, to, from); err != nil {
		return err
	}
	for _, q := range []string{
		`INSERT INTO work_tags (work_id, tag_id) SELECT $1, tag_id FROM work_tags WHERE work_id = $2 ON CONFLICT DO NOTHING`,
		`INSERT INTO work_identifiers (work_id, identifier_type, identifier_value)
		   SELECT $1, identifier_type, identifier_value FROM work_identifiers WHERE work_id = $2 ON CONFLICT DO NOTHING`,
		// Authors come after the ones the work already has, in their own order.
		`INSERT INTO work_contributors (work_id, person_id, role, position)
		   SELECT $1, c.person_id, c.role,
		          c.position + COALESCE((SELECT max(position) + 1 FROM work_contributors WHERE work_id = $1), 0)
		   FROM work_contributors c WHERE c.work_id = $2 ON CONFLICT DO NOTHING`,
	} {
		if _, err := tx.ExecContext(ctx, q, to, from); err != nil {
			return err
		}
	}
	return nil
}

// carryRelations moves the relations that people drew to the work that goes, to the work that stays (#83): they were
// about the book, and the book is now under the other work. A relation the person already had with the work that
// stays, or that is the same one once moved (a pair with no direction, in the other order), is kept once; one between
// the two works joined would join the work to itself, and goes.
func carryRelations(ctx context.Context, tx *sql.Tx, to, from int) error {
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id, origin, comment, source_label, target_label, created_at, updated_at)
		   SELECT user_id, 'work', $1::bigint, type, target_kind, target_id, origin, comment, source_label, target_label, created_at, now()
		   FROM relations WHERE source_kind = 'work' AND source_id = $2 AND NOT (target_kind = 'work' AND target_id = $1)
		   ON CONFLICT DO NOTHING`, []any{to, from}},
		{`DELETE FROM relations WHERE source_kind = 'work' AND source_id = $1`, []any{from}},
		{`INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id, origin, comment, source_label, target_label, created_at, updated_at)
		   SELECT user_id, source_kind, source_id, type, 'work', $1::bigint, origin, comment, source_label, target_label, created_at, now()
		   FROM relations WHERE target_kind = 'work' AND target_id = $2 AND NOT (source_kind = 'work' AND source_id = $1)
		   ON CONFLICT DO NOTHING`, []any{to, from}},
		{`DELETE FROM relations WHERE target_kind = 'work' AND target_id = $1`, []any{from}},
	} {
		if _, err := tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return err
		}
	}
	return nil
}

// JoinTx moves every edition of source under target and retires source. Whoever calls it decides
// whether and how to audit it.
func JoinTx(ctx context.Context, tx *sql.Tx, target, source int, actor string) (JoinResult, error) {
	if target == source {
		return JoinResult{}, ErrSameWork
	}
	if err := lockActive(ctx, tx, target, source); err != nil {
		return JoinResult{}, err
	}
	var res JoinResult
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM editions WHERE work_id = $1`, source).Scan(&res.Editions); err != nil {
		return res, err
	}
	for _, q := range []string{
		// The editions, remembering where they came from; the work that stays keeps its primary edition.
		`UPDATE editions SET work_id = $1, is_primary = FALSE, former_work_id = $2 WHERE work_id = $2`,
		// What people did with the work as a whole.
		`UPDATE notes SET work_id = $1 WHERE work_id = $2`,
		`UPDATE reading_completions SET work_id = $1 WHERE work_id = $2`,
		`UPDATE equivalent_position_acceptances SET work_id = $1 WHERE work_id = $2`,
		// "Finished" if either was: the earliest date a person said so.
		`INSERT INTO work_reading_state (user_id, work_id, finished_at)
		   SELECT user_id, $1, finished_at FROM work_reading_state WHERE work_id = $2
		   ON CONFLICT (user_id, work_id) DO UPDATE SET finished_at = LEAST(work_reading_state.finished_at, EXCLUDED.finished_at)`,
	} {
		if _, err := tx.ExecContext(ctx, q, target, source); err != nil {
			return res, err
		}
	}
	if err := carryWhole(ctx, tx, target, source); err != nil {
		return res, err
	}
	if err := carryRelations(ctx, tx, target, source); err != nil {
		return res, err
	}
	if err := ensurePrimary(ctx, tx, target); err != nil {
		return res, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE works SET retired_at = now(), retired_by = NULLIF($2, '')::uuid WHERE id = $1`, source, actor); err != nil {
		return res, err
	}
	lo, hi := target, source
	if lo > hi {
		lo, hi = hi, lo
	}
	// The pair, if it was proposed, is decided.
	_, err := tx.ExecContext(ctx, `
		UPDATE duplicate_candidates SET state = 'linked', decided_at = now(), decided_by = NULLIF($3, '')::uuid
		WHERE work_a = $1 AND work_b = $2 AND state = 'pending'`, lo, hi, actor)
	return res, err
}

// Join puts every edition of source under target, in one transaction, and audits it.
func Join(ctx context.Context, db *sql.DB, target, source int, actor string) (JoinResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return JoinResult{}, err
	}
	defer tx.Rollback()
	res, err := JoinTx(ctx, tx, target, source, actor)
	if err != nil {
		return res, err
	}
	if err := audit.Record(ctx, tx, actor, "work.join", "work", fmt.Sprint(target),
		map[string]any{"joined": source, "editions": res.Editions}); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

// SplitResult says where the edition went.
type SplitResult struct {
	WorkID   int  // the work it is in now
	Restored bool // it went back to the work it came from, which was retired and is back
}

// Split takes an edition out of its work. It goes back to the work it came from, if there is one (and
// that work is restored if it was retired), and otherwise to a new work with the title and the authors
// of the one it leaves. The notes and the completions that are about its files go with it; what belongs
// to the work as a whole stays where it is (favorites, tags and authors are copied).
func Split(ctx context.Context, db *sql.DB, edition int, actor string) (SplitResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SplitResult{}, err
	}
	defer tx.Rollback()

	var from int
	var former sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT work_id, former_work_id FROM editions WHERE id = $1`, edition).Scan(&from, &former)
	if errors.Is(err, sql.ErrNoRows) {
		return SplitResult{}, ErrNotFound
	}
	if err != nil {
		return SplitResult{}, err
	}
	if err := lockActive(ctx, tx, from, from); err != nil {
		return SplitResult{}, err
	}
	var editions int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM editions WHERE work_id = $1`, from).Scan(&editions); err != nil {
		return SplitResult{}, err
	}
	if editions < 2 {
		return SplitResult{}, ErrOnlyEdition
	}

	var res SplitResult
	if former.Valid && int(former.Int64) != from {
		var retired bool
		err := tx.QueryRowContext(ctx, `SELECT retired_at IS NOT NULL FROM works WHERE id = $1 FOR UPDATE`, former.Int64).Scan(&retired)
		if err == nil {
			res.WorkID = int(former.Int64)
			if retired {
				res.Restored = true
				if _, err := tx.ExecContext(ctx, `UPDATE works SET retired_at = NULL, retired_by = NULL WHERE id = $1`, res.WorkID); err != nil {
					return res, err
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return res, err
		}
	}
	if res.WorkID == 0 {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO works (original_title, media_status) SELECT original_title, 'READY' FROM works WHERE id = $1 RETURNING id`, from).Scan(&res.WorkID); err != nil {
			return res, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_contributors (work_id, person_id, role, position)
			SELECT $1, person_id, role, position FROM work_contributors WHERE work_id = $2`, res.WorkID, from); err != nil {
			return res, err
		}
	}

	files := `SELECT id FROM files WHERE edition_id = $1`
	for _, q := range []string{
		`UPDATE editions SET work_id = $2, is_primary = FALSE, former_work_id = NULL WHERE id = $1`,
		`UPDATE notes SET work_id = $2 WHERE file_id IN (` + files + `)`,
		`UPDATE reading_completions SET work_id = $2 WHERE file_id IN (` + files + `)`,
	} {
		if _, err := tx.ExecContext(ctx, q, edition, res.WorkID); err != nil {
			return res, err
		}
	}
	// Only the favorites: the tags, identifiers and authors describe the book the edition is leaving, not
	// the one it goes to (a work it came from still has its own).
	if err := carryFavorites(ctx, tx, res.WorkID, from); err != nil {
		return res, err
	}
	for _, w := range []int{res.WorkID, from} {
		if err := ensurePrimary(ctx, tx, w); err != nil {
			return res, err
		}
	}
	if err := audit.Record(ctx, tx, actor, "work.split", "edition", fmt.Sprint(edition),
		map[string]any{"from": from, "to": res.WorkID, "restored": res.Restored}); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

// NotTheSame records that two works are not the same book, so no scan proposes the pair again and the
// decision of a person outlasts any guess of the system.
func NotTheSame(ctx context.Context, db *sql.DB, a, b int, actor string) error {
	if a == b {
		return ErrSameWork
	}
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM works WHERE id IN ($1, $2)`, lo, hi).Scan(&n); err != nil {
		return err
	}
	if n != 2 {
		return ErrNotFound
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO duplicate_candidates (work_a, work_b, reason, state, decided_at, decided_by)
		VALUES ($1, $2, 'manual', 'dismissed', now(), NULLIF($3, '')::uuid)
		ON CONFLICT (work_a, work_b) DO UPDATE SET state = 'dismissed', decided_at = now(), decided_by = NULLIF($3, '')::uuid`, lo, hi, actor); err != nil {
		return err
	}
	return audit.Record(ctx, db, actor, "work.not_same", "work", fmt.Sprint(lo), map[string]any{"other": hi})
}
