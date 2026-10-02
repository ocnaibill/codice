package dupes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/equivalence"
	"github.com/ocnaibill/codice/backend/internal/fingerprint"
)

// A pair of works whose files hold the same text is proposed with reason "content" (#38): a book in two formats,
// or two editions of one text, found by the words and not by the title or the author. It is a proposal like the
// others and nothing is joined without a person (DEC-029).
const reasonContent = "content"

// bodyOf is the text of a file's published generation that counts: with an outline, the segments of the body of
// the book (a licence, a preface, an appendix are what many files of a source share, and do not tell the text);
// without one, all of it. It also returns the generation it read and the hash of the file it was made from.
func bodyOf(ctx context.Context, db *sql.DB, fileID int64) (words []string, generation int, sha string, ok bool, err error) {
	var structure []byte
	var source sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT generation, source_sha256, structure FROM text_extractions
		WHERE file_id = $1 AND status = 'ready' AND generation > 0`, fileID).Scan(&generation, &source, &structure)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, "", false, nil
	}
	if err != nil {
		return nil, 0, "", false, err
	}
	var nodes []equivalence.Node
	if len(structure) > 0 && json.Unmarshal(structure, &nodes) != nil {
		nodes = nil // an outline that cannot be read is no outline
	}
	rows, err := db.QueryContext(ctx, `
		SELECT text, node FROM document_segments WHERE file_id = $1 AND generation = $2 ORDER BY sequence`, fileID, generation)
	if err != nil {
		return nil, 0, "", false, err
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		var node sql.NullInt64
		if err := rows.Scan(&text, &node); err != nil {
			return nil, 0, "", false, err
		}
		if len(nodes) > 0 && (!node.Valid || int(node.Int64) >= len(nodes) || nodes[node.Int64].Part != equivalence.PartBody) {
			continue
		}
		words = append(words, equivalence.Words(text)...)
	}
	return words, generation, source.String, true, rows.Err()
}

// ensureFingerprint makes the fingerprint of a file when it has none or the one it has is of another text or of
// another version of the method. It reports whether the file has published text at all.
func ensureFingerprint(ctx context.Context, db *sql.DB, fileID int64) (bool, error) {
	var generation int
	var source sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT generation, source_sha256 FROM text_extractions WHERE file_id = $1 AND status = 'ready' AND generation > 0`, fileID).
		Scan(&generation, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var current bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM text_fingerprints WHERE file_id = $1 AND generation = $2 AND method = $3
		               AND source_sha256 IS NOT DISTINCT FROM $4)`, fileID, generation, fingerprint.Version, nullable(source)).Scan(&current); err != nil {
		return false, err
	}
	if current {
		return true, nil
	}
	words, generation, sha, ok, err := bodyOf(ctx, db, fileID)
	if err != nil || !ok {
		return false, err
	}
	sample := fingerprint.Of(words)
	if sample == nil {
		sample = []int64{}
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO text_fingerprints (file_id, generation, source_sha256, method, words, sample) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (file_id) DO UPDATE SET generation = EXCLUDED.generation, source_sha256 = EXCLUDED.source_sha256,
			method = EXCLUDED.method, words = EXCLUDED.words, sample = EXCLUDED.sample`,
		fileID, generation, nullable(sql.NullString{String: sha, Valid: sha != ""}), fingerprint.Version, len(words), pq.Array(sample))
	return err == nil, err
}

func nullable(s sql.NullString) any {
	if !s.Valid || s.String == "" {
		return nil
	}
	return s.String
}

// sameText is a file of another work that holds the same text as one of this work's.
type sameText struct {
	work    int
	mine    int64 // the file of the work being looked at
	theirs  int64
	overlap fingerprint.Overlap
	mineN   int
	theirsN int
	words   [2]int // of mine, of theirs
}

// detectContent compares the files of one work, with published text, with the files of every other active work.
// It returns how many new pairs it proposed.
func detectContent(ctx context.Context, db *sql.DB, workID int) (int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.id FROM files f JOIN editions e ON e.id = f.edition_id WHERE e.work_id = $1 ORDER BY f.id`, workID)
	if err != nil {
		return 0, err
	}
	var files []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		files = append(files, id)
	}
	rows.Close()

	best := map[int]sameText{} // the best match for each other work
	for _, id := range files {
		if ok, err := ensureFingerprint(ctx, db, id); err != nil {
			return 0, err
		} else if !ok {
			continue
		}
		var mine pq.Int64Array
		var myWords int
		if err := db.QueryRowContext(ctx, `SELECT sample, words FROM text_fingerprints WHERE file_id = $1`, id).Scan(&mine, &myWords); err != nil {
			return 0, err
		}
		// The files that hold at least the share of this one's hashes that "the same text" needs, found hash by hash
		// through the index: comparing whole arrays with && reads every fingerprint (twelve seconds over four thousand
		// files), this reads the few that have a hash in common (a few thousandths of a second).
		need := int(math.Ceil(fingerprint.SameContainment * float64(len(mine))))
		found, err := db.QueryContext(ctx, `
			WITH hits AS (
				SELECT o.file_id FROM unnest($1::bigint[]) AS h(hash) JOIN text_fingerprints o ON o.sample @> ARRAY[h.hash]
				GROUP BY o.file_id HAVING count(*) >= $3
			)
			SELECT o.file_id, e.work_id, o.words, o.sample
			FROM hits JOIN text_fingerprints o ON o.file_id = hits.file_id
			JOIN text_extractions te ON te.file_id = o.file_id AND te.generation = o.generation AND te.status = 'ready'
			JOIN files f ON f.id = o.file_id
			JOIN editions e ON e.id = f.edition_id
			JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
			WHERE e.work_id <> $2`, pq.Array([]int64(mine)), workID, need)
		if err != nil {
			return 0, err
		}
		for found.Next() {
			var other sameText
			var sample pq.Int64Array
			if err := found.Scan(&other.theirs, &other.work, &other.words[1], &sample); err != nil {
				found.Close()
				return 0, err
			}
			o := fingerprint.Compare(mine, sample)
			if !o.Same() {
				continue
			}
			other.mine, other.overlap, other.mineN, other.theirsN, other.words[0] = id, o, len(mine), len(sample), myWords
			if prev, ok := best[other.work]; !ok || min(o.OfA, o.OfB) > min(prev.overlap.OfA, prev.overlap.OfB) {
				best[other.work] = other
			}
		}
		found.Close()
		if err := found.Err(); err != nil {
			return 0, err
		}
	}

	others := make([]int, 0, len(best))
	for w := range best {
		others = append(others, w)
	}
	sort.Ints(others)
	added := 0
	for _, w := range others {
		isNew, err := proposeContent(ctx, db, workID, w, best[w])
		if err != nil {
			return added, err
		}
		if isNew {
			added++
		}
	}
	return added, nil
}

// proposeContent records the pair with what the files share. A pair that is already waiting for a decision for
// another reason keeps it and gains the evidence; one that was decided is left alone, so a pair that was said
// not to be the same work does not come back.
func proposeContent(ctx context.Context, db *sql.DB, mine, theirs int, m sameText) (bool, error) {
	lo, hi := mine, theirs
	a, b := m.overlap.OfA, m.overlap.OfB // of the work being looked at, of the other
	fileA, fileB, hashesA, hashesB, wordsA, wordsB := m.mine, m.theirs, m.mineN, m.theirsN, m.words[0], m.words[1]
	if lo > hi {
		lo, hi = hi, lo
		a, b = b, a
		fileA, fileB, hashesA, hashesB, wordsA, wordsB = fileB, fileA, hashesB, hashesA, wordsB, wordsA
	}
	evidence, _ := json.Marshal(map[string]any{"content": map[string]any{
		"fileA": fileA, "fileB": fileB, "ofA": round3(a), "ofB": round3(b), "shared": m.overlap.Shared,
		"hashesA": hashesA, "hashesB": hashesB, "wordsA": wordsA, "wordsB": wordsB, "method": fingerprint.Version,
	}})
	var inserted bool
	err := db.QueryRowContext(ctx, `
		INSERT INTO duplicate_candidates (work_a, work_b, reason, evidence) VALUES ($1, $2, $3, $4)
		ON CONFLICT (work_a, work_b) DO UPDATE
			SET evidence = COALESCE(duplicate_candidates.evidence, '{}'::jsonb) || EXCLUDED.evidence
			WHERE duplicate_candidates.state = 'pending'
		RETURNING (xmax = 0)`, lo, hi, reasonContent, evidence).Scan(&inserted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // decided before: left as it is
	}
	return inserted, err
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }
