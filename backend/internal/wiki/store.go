package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/lib/pq"
	"golang.org/x/text/unicode/norm"
)

// ErrNotFound is a project, a book file or an entity that is not there.
var ErrNotFound = errors.New("not found")

// InputError is a bundle that cannot be stored as it is: the message says why, in words for the person importing it.
type InputError struct{ Msg string }

func (e InputError) Error() string { return e.Msg }

func inputErr(format string, a ...any) error { return InputError{Msg: fmt.Sprintf(format, a...)} }

type segment struct {
	sequence int
	href     string
	text     string // folded, for finding quotes
}

type resolvedBook struct {
	fileID   int64
	workID   int64
	segments []segment
}

func cleanHref(h string) string {
	if u, err := url.PathUnescape(h); err == nil {
		h = u
	}
	h = strings.SplitN(h, "#", 2)[0]
	return strings.TrimPrefix(path.Clean("/"+h), "/")
}

// sameResource says whether two hrefs name the same resource of an EPUB: the bundle may give the path inside the archive
// ("OEBPS/Text/c1.xhtml") where the stored segments give it relative to the package document ("Text/c1.xhtml").
func sameResource(a, b string) bool {
	a, b = cleanHref(a), cleanHref(b)
	return a == b || strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// fold makes two renderings of the same words equal: no accents, no case, no punctuation, single spaces.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(nonWord.ReplaceAllString(b.String(), " "))
}

func resolveBooks(ctx context.Context, db *sql.DB, b *Bundle) ([]resolvedBook, error) {
	out := make([]resolvedBook, len(b.Books))
	for i, bk := range b.Books {
		err := db.QueryRowContext(ctx, `SELECT f.id, e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.sha256 = $1`,
			strings.ToLower(bk.SHA256)).Scan(&out[i].fileID, &out[i].workID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, inputErr("o arquivo de %q (hash %s…) não está no acervo", bk.Title, bk.SHA256[:12])
		}
		if err != nil {
			return nil, err
		}
		rows, err := db.QueryContext(ctx, `
			SELECT s.sequence, COALESCE(s.locator->>'href', ''), s.text
			FROM document_segments s
			JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
			WHERE s.file_id = $1 ORDER BY s.sequence`, out[i].fileID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var s segment
			if err := rows.Scan(&s.sequence, &s.href, &s.text); err != nil {
				rows.Close()
				return nil, err
			}
			s.text = fold(s.text)
			out[i].segments = append(out[i].segments, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(out[i].segments) == 0 {
			return nil, inputErr("o texto de %q ainda não foi lido pelo Códice: rode a extração de texto antes de importar a wiki", bk.Title)
		}
	}
	return out, nil
}

// firstSequences is where each chapter of a book starts in its file: the first segment of the resource the chapter starts in.
// A chapter whose resource the file does not have keeps no start, and a reader's place is never placed in it.
func firstSequences(bk Book, rb resolvedBook) map[int]int {
	out := map[int]int{}
	for _, c := range bk.Chapters {
		if c.Href == "" {
			continue
		}
		for _, s := range rb.segments {
			if sameResource(c.Href, s.href) {
				out[c.Chapter] = s.sequence
				break
			}
		}
	}
	return out
}

// quoteSequence is the segment of the chapter the quote is in, for opening the reader there. It looks inside the chapter first,
// then anywhere in the book; nothing is returned when the words are not found.
func quoteSequence(quote string, rb resolvedBook, from, to int) *int {
	q := fold(quote)
	if len([]rune(q)) < 12 {
		return nil
	}
	if r := []rune(q); len(r) > 60 {
		q = string(r[:60])
	}
	for pass := 0; pass < 2; pass++ {
		for _, s := range rb.segments {
			if pass == 0 && (s.sequence < from || (to >= 0 && s.sequence >= to)) {
				continue
			}
			if strings.Contains(s.text, q) {
				seq := s.sequence
				return &seq
			}
		}
	}
	return nil
}

// scope is the work (one book) or the official collection (a series) the bundle belongs to.
func scope(ctx context.Context, db *sql.DB, books []resolvedBook) (workID, collectionID sql.NullInt64, err error) {
	works := map[int64]bool{}
	var ids []int64
	for _, b := range books {
		if !works[b.workID] {
			works[b.workID] = true
			ids = append(ids, b.workID)
		}
	}
	if len(ids) == 1 && len(books) == 1 {
		return sql.NullInt64{Int64: ids[0], Valid: true}, collectionID, nil
	}
	err = db.QueryRowContext(ctx, `
		SELECT cw.collection_id FROM collection_works cw JOIN collections c ON c.id = cw.collection_id
		WHERE cw.official AND c.retired_at IS NULL AND cw.work_id = ANY($1)
		GROUP BY cw.collection_id HAVING COUNT(DISTINCT cw.work_id) = $2
		ORDER BY cw.collection_id LIMIT 1`, pq.Array(ids), len(ids)).Scan(&collectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return workID, collectionID, inputErr("os livros do pacote não estão todos na mesma série do acervo: ponha-os na série antes de importar a wiki")
	}
	return workID, collectionID, err
}

type keptSentence struct {
	slug, text string
	hiddenAt   sql.NullTime
	hiddenBy   sql.NullString
}

type keptDecision struct {
	kind, entity, other, state string
	decidedBy                  sql.NullString
	decidedAt                  sql.NullTime
}

// Import stores a bundle as the wiki of its work or series, replacing what was there, in one transaction. What owner and admins
// decided on the previous generation is kept (DEC-181, RF-052): a sentence marked wrong stays hidden if the new generation writes
// it again, and a decided suggestion keeps its decision.
func Import(ctx context.Context, db *sql.DB, b *Bundle, userID string) (int64, error) {
	if err := b.Validate(); err != nil {
		return 0, InputError{Msg: err.Error()}
	}
	books, err := resolveBooks(ctx, db, b)
	if err != nil {
		return 0, err
	}
	workID, collectionID, err := scope(ctx, db, books)
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var oldID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM wiki_projects WHERE work_id = $1 OR collection_id = $2 FOR UPDATE`,
		workID, collectionID).Scan(&oldID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var hidden []keptSentence
	var decided []keptDecision
	var before map[string]map[SectionKey]bool
	if oldID.Valid {
		var err error
		if before, err = snapshotSections(ctx, tx, oldID.Int64); err != nil {
			return 0, err
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT e.slug, s.text, s.hidden_at, s.hidden_by::text FROM wiki_sentences s
			JOIN wiki_sections sec ON sec.id = s.section_id JOIN wiki_entities e ON e.id = sec.entity_id
			WHERE e.project_id = $1 AND s.hidden_at IS NOT NULL`, oldID.Int64)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var k keptSentence
			if err := rows.Scan(&k.slug, &k.text, &k.hiddenAt, &k.hiddenBy); err != nil {
				rows.Close()
				return 0, err
			}
			hidden = append(hidden, k)
		}
		rows.Close()
		rows, err = tx.QueryContext(ctx, `
			SELECT kind, entity_slug, COALESCE(other_slug, ''), state, decided_by::text, decided_at FROM wiki_suggestions
			WHERE project_id = $1 AND state <> 'open'`, oldID.Int64)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var k keptDecision
			if err := rows.Scan(&k.kind, &k.entity, &k.other, &k.state, &k.decidedBy, &k.decidedAt); err != nil {
				rows.Close()
				return 0, err
			}
			decided = append(decided, k)
		}
		rows.Close()
		if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_projects WHERE id = $1`, oldID.Int64); err != nil {
			return 0, err
		}
	}

	state := "ready"
	if b.Partial {
		state = "partial"
	}
	var projectID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO wiki_projects (collection_id, work_id, state, provider, model, remote, format_version, generated_at, imported_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid) RETURNING id`,
		collectionID, workID, state, b.Provider, b.Model, b.Remote, b.Format, b.GeneratedAt, userID).Scan(&projectID); err != nil {
		return 0, err
	}

	starts := make([]map[int]int, len(b.Books))
	for i, bk := range b.Books {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_books (project_id, position, work_id, file_id, side, title) VALUES ($1, $2, $3, $4, $5, $6)`,
			projectID, i, books[i].workID, books[i].fileID, bk.Side, bk.Title); err != nil {
			return 0, err
		}
		starts[i] = firstSequences(bk, books[i])
		for _, c := range bk.Chapters {
			var first sql.NullInt64
			if s, ok := starts[i][c.Chapter]; ok {
				first = sql.NullInt64{Int64: int64(s), Valid: true}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_chapters (project_id, book, chapter, title, part, first_sequence, editorial) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				projectID, i, c.Chapter, c.Title, c.Part, first, c.Editorial); err != nil {
				return 0, err
			}
		}
	}
	// The segments a chapter covers: from its start to the next known start.
	chapterRange := func(book, chapter int) (int, int) {
		from, ok := starts[book][chapter]
		if !ok {
			return -1, -1
		}
		to := -1
		for c, s := range starts[book] {
			if c > chapter && s > from && (to < 0 || s < to) {
				to = s
			}
		}
		return from, to
	}

	ids := map[string]int64{}
	for _, e := range b.Entities {
		var id int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO wiki_entities (project_id, slug, name, kind, page) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			projectID, e.Slug, e.Name, e.Kind, e.Page).Scan(&id); err != nil {
			return 0, err
		}
		ids[e.Slug] = id
	}
	hiddenOf := map[string]keptSentence{}
	for _, k := range hidden {
		hiddenOf[k.slug+"\x00"+k.text] = k
	}
	for _, e := range b.Entities {
		id := ids[e.Slug]
		for _, p := range dedupPoints(e.Appearances) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_appearances (entity_id, book, chapter) VALUES ($1, $2, $3)`, id, p.Book, p.Chapter); err != nil {
				return 0, err
			}
		}
		seenAlias := map[string]bool{}
		for _, a := range e.Aliases {
			if strings.TrimSpace(a.Alias) == "" || seenAlias[a.Alias] || a.Alias == e.Name {
				continue
			}
			seenAlias[a.Alias] = true
			if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_aliases (entity_id, alias, book, chapter) VALUES ($1, $2, $3, $4)`, id, a.Alias, a.Book, a.Chapter); err != nil {
				return 0, err
			}
		}
		seenState := map[Point]bool{}
		for _, s := range e.States {
			p := Point{s.Book, s.Chapter}
			if seenState[p] || strings.TrimSpace(s.Text) == "" {
				continue
			}
			seenState[p] = true
			if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_states (entity_id, book, chapter, text) VALUES ($1, $2, $3, $4)`, id, s.Book, s.Chapter, s.Text); err != nil {
				return 0, err
			}
		}
		for si, sec := range e.Sections {
			var book, from, to sql.NullInt64
			if sec.Book != nil {
				book = sql.NullInt64{Int64: int64(*sec.Book), Valid: true}
				from = sql.NullInt64{Int64: int64(sec.ChapterFrom), Valid: true}
				to = sql.NullInt64{Int64: int64(sec.ChapterTo), Valid: true}
			}
			var secID int64
			if err := tx.QueryRowContext(ctx, `INSERT INTO wiki_sections (entity_id, kind, book, chapter_from, chapter_to, part, position) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
				id, sec.Kind, book, from, to, sec.Part, si).Scan(&secID); err != nil {
				return 0, err
			}
			for pi, st := range sec.Sentences {
				par := st.Paragraph
				if par < 1 {
					par = 1
				}
				var hAt sql.NullTime
				var hBy sql.NullString
				if k, ok := hiddenOf[e.Slug+"\x00"+st.Text]; ok {
					hAt, hBy = k.hiddenAt, k.hiddenBy
				}
				var stID int64
				if err := tx.QueryRowContext(ctx, `INSERT INTO wiki_sentences (section_id, paragraph, position, text, hidden_at, hidden_by) VALUES ($1, $2, $3, $4, $5, $6::uuid) RETURNING id`,
					secID, par, pi, st.Text, hAt, hBy).Scan(&stID); err != nil {
					return 0, err
				}
				for _, r := range st.Refs {
					var seq *int
					if r.Quote != "" {
						from, to := chapterRange(r.Book, r.Chapter)
						seq = quoteSequence(r.Quote, books[r.Book], from, to)
					}
					if seq == nil {
						if s, ok := starts[r.Book][r.Chapter]; ok {
							seq = &s
						}
					}
					var quote sql.NullString
					if r.Quote != "" {
						quote = sql.NullString{String: r.Quote, Valid: true}
					}
					if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_references (sentence_id, book, chapter, fact, quote, sequence) VALUES ($1, $2, $3, $4, $5, $6)`,
						stID, r.Book, r.Chapter, r.Fact, quote, seq); err != nil {
						return 0, err
					}
				}
			}
		}
		for ri, r := range e.Relations {
			var other sql.NullInt64
			if r.Other != "" {
				other = sql.NullInt64{Int64: ids[r.Other], Valid: true}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_relations (entity_id, other_id, other_name, grp, role, book, chapter, position) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				id, other, r.Name, r.Group, r.Role, r.Book, r.Chapter, ri); err != nil {
				return 0, err
			}
		}
	}
	if len(b.Edges) > 0 {
		stmt, err := tx.PrepareContext(ctx, pq.CopyIn("wiki_edges", "project_id", "a_id", "b_id", "label", "book", "chapter"))
		if err != nil {
			return 0, err
		}
		for _, e := range b.Edges {
			if _, err := stmt.ExecContext(ctx, projectID, ids[e.A], ids[e.B], e.Label, e.Book, e.Chapter); err != nil {
				stmt.Close()
				return 0, err
			}
		}
		if _, err := stmt.ExecContext(ctx); err != nil {
			stmt.Close()
			return 0, err
		}
		if err := stmt.Close(); err != nil {
			return 0, err
		}
	}
	decision := map[string]keptDecision{}
	for _, d := range decided {
		decision[d.kind+"\x00"+d.entity+"\x00"+d.other] = d
	}
	for _, s := range b.Suggestions {
		d, ok := decision[s.Kind+"\x00"+s.Entity+"\x00"+s.Other]
		state := "open"
		if ok {
			state = d.state
		}
		var other sql.NullString
		if s.Other != "" {
			other = sql.NullString{String: s.Other, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_suggestions (project_id, kind, entity_slug, other_slug, evidence, book, chapter, state, decided_by, decided_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::uuid, $10)`,
			projectID, s.Kind, s.Entity, other, s.Evidence, s.Book, s.Chapter, state, d.decidedBy, d.decidedAt); err != nil {
			return 0, err
		}
	}
	// What the staff decided about who is who is still true of this generation (DEC-181).
	if err := applyRules(ctx, tx, projectID); err != nil {
		return 0, err
	}
	// What this update brought that the older content did not have, for the staff to review (DEC-180).
	if oldID.Valid {
		body, err := json.Marshal(diffBundle(before, b))
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE wiki_projects SET changes = $2::jsonb WHERE id = $1`, projectID, body); err != nil {
			return 0, err
		}
	}
	if err := recordAudit(ctx, tx, userID, projectID, b, oldID.Valid); err != nil {
		return 0, err
	}
	return projectID, tx.Commit()
}

func dedupPoints(points []Point) []Point {
	seen := map[Point]bool{}
	var out []Point
	for _, p := range points {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
