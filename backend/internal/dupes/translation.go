package dupes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/equivalence"
)

// A pair of works whose files read as one book in two versions, though they share no run of words, is proposed with
// reason "translation" (#38): the book in another language, or another translation of it. Two steps, so that a
// library is not read against itself: the rare names and numbers of the files tell, from the index, which files
// might be a translation of this one, and only those are read against it, end to end (equivalence.ReadParallel).
const reasonTranslation = "translation"

const (
	// minSharedNames is the fewest rare names two files must have in common to be read against each other, and
	// minSharedNameShare the share of the smaller file's names. A translation keeps 37 to 51% of them (names are
	// not translated); books that have nothing to do with each other, 0 to 20%; the sibling books of a saga, a third
	// or so, which is why this only chooses what is read and decides nothing.
	minSharedNames     = 20
	minSharedNameShare = 0.25
	// maxReadAgainst bounds the files read against one file in one run.
	maxReadAgainst = 8
)

// loadFile is a file's published text as the comparison reads it: its segments in reading order, each with the part
// of the book it is in, and the outline they were published with. Embeddings are not read: no part of this uses them.
func loadFile(ctx context.Context, db *sql.DB, fileID int64) (equivalence.File, error) {
	var file equivalence.File
	var structure []byte
	if err := db.QueryRowContext(ctx, `SELECT structure FROM text_extractions WHERE file_id = $1`, fileID).Scan(&structure); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return file, err
	}
	if len(structure) > 0 && json.Unmarshal(structure, &file.Nodes) != nil {
		file.Nodes = nil // an outline that cannot be read is no outline
	}
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.sequence, s.text, s.node, s.locator
		FROM document_segments s JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1 ORDER BY s.sequence`, fileID)
	if err != nil {
		return file, err
	}
	defer rows.Close()
	for rows.Next() {
		var seg equivalence.Segment
		var node sql.NullInt64
		var locator []byte
		if err := rows.Scan(&seg.ID, &seg.Sequence, &seg.Text, &node, &locator); err != nil {
			return file, err
		}
		seg.Locator = json.RawMessage(locator)
		seg.Node = equivalence.NoNode
		if node.Valid && int(node.Int64) < len(file.Nodes) {
			seg.Node = int(node.Int64)
			seg.Part = file.Nodes[seg.Node].Part
		}
		file.Segments = append(file.Segments, seg)
	}
	return file, rows.Err()
}

// translated is a file of another work that reads as a version of one of this work's files.
type translated struct {
	work, mine, theirs int64
	read               equivalence.Parallel
	shared, minesN     int
	theirsN            int
	languages          [2]string // of mine, of theirs
}

// detectTranslation reads the files of one work against the files of other works that share enough rare names with
// them, and proposes the pairs that read as the same book. It returns how many new pairs it proposed.
func detectTranslation(ctx context.Context, db *sql.DB, workID int) (int, error) {
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

	best := map[int]translated{}
	for _, id := range files {
		if ok, err := ensureFingerprint(ctx, db, id); err != nil {
			return 0, err
		} else if !ok {
			continue
		}
		var mine pq.StringArray
		var language sql.NullString
		if err := db.QueryRowContext(ctx, `
			SELECT t.names, te.language FROM text_fingerprints t JOIN text_extractions te ON te.file_id = t.file_id WHERE t.file_id = $1`, id).
			Scan(&mine, &language); err != nil {
			return 0, err
		}
		if len(mine) < minSharedNames {
			continue
		}
		found, err := db.QueryContext(ctx, `
			WITH hits AS (
				SELECT o.file_id, count(*) AS shared FROM unnest($1::text[]) AS h(name) JOIN text_fingerprints o ON o.names @> ARRAY[h.name]
				GROUP BY o.file_id HAVING count(*) >= $3
			)
			SELECT o.file_id, e.work_id, hits.shared, cardinality(o.names), te.language
			FROM hits JOIN text_fingerprints o ON o.file_id = hits.file_id
			JOIN text_extractions te ON te.file_id = o.file_id AND te.generation = o.generation AND te.status = 'ready'
			JOIN files f ON f.id = o.file_id
			JOIN editions e ON e.id = f.edition_id
			JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
			WHERE e.work_id <> $2
			ORDER BY hits.shared DESC, o.file_id LIMIT $4`, pq.Array([]string(mine)), workID, minSharedNames, maxReadAgainst)
		if err != nil {
			return 0, err
		}
		type candidate struct {
			file, work    int64
			shared, names int
			language      sql.NullString
		}
		var candidates []candidate
		for found.Next() {
			var c candidate
			if err := found.Scan(&c.file, &c.work, &c.shared, &c.names, &c.language); err != nil {
				found.Close()
				return 0, err
			}
			smaller := len(mine)
			if c.names < smaller {
				smaller = c.names
			}
			if smaller > 0 && float64(c.shared)/float64(smaller) >= minSharedNameShare {
				candidates = append(candidates, c)
			}
		}
		found.Close()
		if err := found.Err(); err != nil {
			return 0, err
		}
		var mineFile *equivalence.File
		for _, c := range candidates {
			if undecided, err := readable(ctx, db, workID, int(c.work)); err != nil {
				return 0, err
			} else if !undecided {
				continue
			}
			if mineFile == nil {
				f, err := loadFile(ctx, db, id)
				if err != nil {
					return 0, err
				}
				mineFile = &f
			}
			theirs, err := loadFile(ctx, db, c.file)
			if err != nil {
				return 0, err
			}
			read := equivalence.ReadParallel(*mineFile, theirs, equivalence.ParallelSamples)
			if !read.Translation() {
				continue
			}
			if prev, ok := best[int(c.work)]; !ok || read.Hits > prev.read.Hits {
				best[int(c.work)] = translated{work: c.work, mine: id, theirs: c.file, read: read, shared: c.shared,
					minesN: len(mine), theirsN: c.names, languages: [2]string{language.String, c.language.String}}
			}
		}
	}

	others := make([]int, 0, len(best))
	for w := range best {
		others = append(others, w)
	}
	sort.Ints(others)
	added := 0
	for _, w := range others {
		isNew, err := proposeTranslation(ctx, db, workID, w, best[w])
		if err != nil {
			return added, err
		}
		if isNew {
			added++
		}
	}
	return added, nil
}

// readable says whether the pair of works is worth reading against each other: it was not decided, and it does not
// already say, by its evidence, that it is the same text or a translation (the reading is the expensive step, and it
// is not repeated for what is already known).
func readable(ctx context.Context, db *sql.DB, a, b int) (bool, error) {
	lo, hi, _ := ordered(a, b)
	var state string
	var known bool
	err := db.QueryRowContext(ctx, `
		SELECT state, COALESCE(evidence ? 'content' OR evidence ? 'translation', FALSE) FROM duplicate_candidates WHERE work_a = $1 AND work_b = $2`, lo, hi).
		Scan(&state, &known)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return state == "pending" && !known, nil
}

func proposeTranslation(ctx context.Context, db *sql.DB, mine, theirs int, t translated) (bool, error) {
	lo, hi, swapped := ordered(mine, theirs)
	fileA, fileB, namesA, namesB, languageA, languageB := t.mine, t.theirs, t.minesN, t.theirsN, t.languages[0], t.languages[1]
	if swapped {
		fileA, fileB, namesA, namesB, languageA, languageB = fileB, fileA, namesB, namesA, languageB, languageA
	}
	return propose(ctx, db, lo, hi, reasonTranslation, "translation", map[string]any{
		"fileA": fileA, "fileB": fileB, "hits": t.read.Hits, "samples": t.read.Samples, "order": round3(t.read.Order),
		"sharedNames": t.shared, "namesA": namesA, "namesB": namesB, "languageA": languageA, "languageB": languageB,
		"method": methodTranslation,
	})
}

const methodTranslation = 1
