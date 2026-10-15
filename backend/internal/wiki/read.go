package wiki

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

func recordAudit(ctx context.Context, tx *sql.Tx, userID string, projectID int64, b *Bundle, replaced bool) error {
	pages := 0
	for _, e := range b.Entities {
		if e.Page {
			pages++
		}
	}
	return audit.Record(ctx, tx, userID, "wiki.import", "wiki", strconv.FormatInt(projectID, 10), map[string]any{
		"books": len(b.Books), "entities": len(b.Entities), "pages": pages, "provider": b.Provider, "model": b.Model,
		"remote": b.Remote, "partial": b.Partial, "replaced": replaced,
	})
}

// Delete removes a project and everything in it.
func Delete(ctx context.Context, db *sql.DB, projectID int64, userID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// What the model answered for this wiki holds quotes of the books: it goes with the wiki.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM wiki_llm_cache
		WHERE work_id = (SELECT work_id FROM wiki_projects WHERE id = $1)
		   OR collection_id = (SELECT collection_id FROM wiki_projects WHERE id = $1)`, projectID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM wiki_projects WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := audit.Record(ctx, tx, userID, "wiki.delete", "wiki", strconv.FormatInt(projectID, 10), nil); err != nil {
		return err
	}
	return tx.Commit()
}

// BookRow is a stored book with its chapters.
type BookRow struct {
	Position int          `json:"position"`
	WorkID   int64        `json:"workId"`
	FileID   *int64       `json:"fileId"`
	Side     bool         `json:"side"`
	Title    string       `json:"title"`
	Chapters []ChapterRow `json:"-"`
}

// Project is a stored wiki and its books, as the read side needs it.
type Project struct {
	ID           int64     `json:"id"`
	WorkID       *int64    `json:"workId"`
	CollectionID *int64    `json:"collectionId"`
	State        string    `json:"state"`
	Review       string    `json:"review"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Remote       bool      `json:"remote"`
	GeneratedAt  time.Time `json:"generatedAt"`
	Books        []BookRow `json:"books"`
}

// ProjectFor is the wiki a work belongs to: its own, or its series'.
func ProjectFor(ctx context.Context, db *sql.DB, workID int64) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `
		SELECT p.id FROM wiki_projects p
		WHERE p.work_id = $1
		   OR p.collection_id IN (SELECT collection_id FROM collection_works WHERE official AND work_id = $1)
		   OR p.id IN (SELECT project_id FROM wiki_books WHERE work_id = $1)
		ORDER BY (p.work_id = $1) DESC NULLS LAST LIMIT 1`, workID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// LoadProject reads a project and its books and chapters.
func LoadProject(ctx context.Context, db *sql.DB, id int64) (*Project, error) {
	p := &Project{ID: id}
	var work, coll sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT work_id, collection_id, state, review, provider, model, remote, generated_at FROM wiki_projects WHERE id = $1`, id).
		Scan(&work, &coll, &p.State, &p.Review, &p.Provider, &p.Model, &p.Remote, &p.GeneratedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if work.Valid {
		p.WorkID = &work.Int64
	}
	if coll.Valid {
		p.CollectionID = &coll.Int64
	}
	rows, err := db.QueryContext(ctx, `SELECT position, work_id, file_id, side, title FROM wiki_books WHERE project_id = $1 ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var b BookRow
		var file sql.NullInt64
		if err := rows.Scan(&b.Position, &b.WorkID, &file, &b.Side, &b.Title); err != nil {
			rows.Close()
			return nil, err
		}
		if file.Valid {
			b.FileID = &file.Int64
		}
		p.Books = append(p.Books, b)
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT book, chapter, title, part, first_sequence, editorial FROM wiki_chapters WHERE project_id = $1 ORDER BY book, chapter`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var book int
		var c ChapterRow
		var first sql.NullInt64
		if err := rows.Scan(&book, &c.Chapter, &c.Title, &c.Part, &first, &c.Editorial); err != nil {
			return nil, err
		}
		if first.Valid {
			v := int(first.Int64)
			c.FirstSequence = &v
		}
		if book >= 0 && book < len(p.Books) {
			p.Books[book].Chapters = append(p.Books[book].Chapters, c)
		}
	}
	return p, rows.Err()
}

func (p *Project) side(book int) bool {
	return book >= 0 && book < len(p.Books) && p.Books[book].Side
}

// Sees says whether the cursor shows a point of this project.
func (p *Project) Sees(c Cursor, book, chapter int) bool {
	if book < 0 || book >= len(p.Books) {
		return false
	}
	return c.Sees(p.side(book), book, chapter)
}

func (p *Project) chapter(book, chapter int) *ChapterRow {
	if book < 0 || book >= len(p.Books) {
		return nil
	}
	for i := range p.Books[book].Chapters {
		if p.Books[book].Chapters[i].Chapter == chapter {
			return &p.Books[book].Chapters[i]
		}
	}
	return nil
}

// Label names a point for a reader: the book, the part and the chapter.
func (p *Project) Label(book, chapter int) string {
	if book < 0 || book >= len(p.Books) {
		return ""
	}
	out := p.Books[book].Title
	if c := p.chapter(book, chapter); c != nil {
		if c.Part != "" && c.Part != "Final" && c.Part != "Abertura" {
			out += ", " + c.Part
		}
		out += " · " + c.Title
	}
	return out
}

// ChapterView is a chapter as the cursor control shows it: past the reader's own point only the number is given, since a
// chapter's title can tell what happens in it.
type ChapterView struct {
	Chapter int    `json:"chapter"`
	Title   string `json:"title"`
	Part    string `json:"part"`
	Read    bool   `json:"read"`
}

// BookView is a book of the project as the cursor control shows it.
type BookView struct {
	Position int           `json:"position"`
	WorkID   int64         `json:"workId"`
	FileID   *int64        `json:"fileId"`
	Side     bool          `json:"side"`
	Title    string        `json:"title"`
	Chapters []ChapterView `json:"chapters"`
}

// Books lists the books and their story chapters (editorial ones left out), titled up to the reader's own point.
func (p *Project) BooksFor(own Cursor) []BookView {
	out := []BookView{}
	for i, b := range p.Books {
		v := BookView{Position: i, WorkID: b.WorkID, FileID: b.FileID, Side: b.Side, Title: b.Title, Chapters: []ChapterView{}}
		for _, c := range b.Chapters {
			if c.Editorial {
				continue
			}
			read := own.Sees(b.Side, i, c.Chapter) && !b.Side
			cv := ChapterView{Chapter: c.Chapter, Read: read}
			if read || b.Side {
				cv.Title, cv.Part = c.Title, c.Part
			}
			v.Chapters = append(v.Chapters, cv)
		}
		out = append(out, v)
	}
	return out
}

// IndexEntry is one entity in the index of a wiki.
type IndexEntry struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Page        bool     `json:"page"`
	Appearances int      `json:"appearances"`
	Aliases     []string `json:"aliases"`
}

type pointRow struct {
	entity  int64
	book    int
	chapter int
}

func (p *Project) appearances(ctx context.Context, db *sql.DB) ([]pointRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.entity_id, a.book, a.chapter FROM wiki_appearances a JOIN wiki_entities e ON e.id = a.entity_id WHERE e.project_id = $1`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pointRow
	for rows.Next() {
		var r pointRow
		if err := rows.Scan(&r.entity, &r.book, &r.chapter); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type entityRow struct {
	id         int64
	slug, name string
	kind       string
	page       bool
}

func (p *Project) entities(ctx context.Context, db *sql.DB) (map[int64]entityRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, slug, name, kind, page FROM wiki_entities WHERE project_id = $1`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]entityRow{}
	for rows.Next() {
		var e entityRow
		if err := rows.Scan(&e.id, &e.slug, &e.name, &e.kind, &e.page); err != nil {
			return nil, err
		}
		out[e.id] = e
	}
	return out, rows.Err()
}

// visibleCounts is how many chapters each entity appeared in up to the cursor; an entity not in it has not appeared yet.
func (p *Project) visibleCounts(ctx context.Context, db *sql.DB, c Cursor) (map[int64]int, error) {
	points, err := p.appearances(ctx, db)
	if err != nil {
		return nil, err
	}
	out := map[int64]int{}
	for _, r := range points {
		if p.Sees(c, r.book, r.chapter) {
			out[r.entity]++
		}
	}
	return out, nil
}

// Index lists the entities that appeared up to the cursor, with the names they already had there.
func (p *Project) Index(ctx context.Context, db *sql.DB, c Cursor) ([]IndexEntry, error) {
	counts, err := p.visibleCounts(ctx, db, c)
	if err != nil {
		return nil, err
	}
	ents, err := p.entities(ctx, db)
	if err != nil {
		return nil, err
	}
	aliases, err := p.visibleAliases(ctx, db, c, 0)
	if err != nil {
		return nil, err
	}
	out := []IndexEntry{}
	for id, n := range counts {
		e := ents[id]
		al := aliases[id]
		if al == nil {
			al = []string{}
		}
		out = append(out, IndexEntry{Slug: e.slug, Name: e.name, Kind: e.kind, Page: e.page, Appearances: n, Aliases: al})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page
		}
		if out[i].Appearances != out[j].Appearances {
			return out[i].Appearances > out[j].Appearances
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (p *Project) visibleAliases(ctx context.Context, db *sql.DB, c Cursor, only int64) (map[int64][]string, error) {
	q := `SELECT a.entity_id, a.alias, a.book, a.chapter FROM wiki_aliases a JOIN wiki_entities e ON e.id = a.entity_id WHERE e.project_id = $1`
	args := []any{p.ID}
	if only != 0 {
		q += ` AND a.entity_id = $2`
		args = append(args, only)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY a.book, a.chapter, a.alias`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var alias string
		var book, chapter int
		if err := rows.Scan(&id, &alias, &book, &chapter); err != nil {
			return nil, err
		}
		if p.Sees(c, book, chapter) {
			out[id] = append(out[id], alias)
		}
	}
	return out, rows.Err()
}

// GraphNode and GraphEdge are what the graph shows up to the cursor.
type GraphNode struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Page        bool   `json:"page"`
	Appearances int    `json:"appearances"`
}

type GraphLabel struct {
	Label   string `json:"label"`
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
}

type GraphEdge struct {
	A      string       `json:"a"`
	B      string       `json:"b"`
	Weight int          `json:"weight"`
	Labels []GraphLabel `json:"labels"`
}

const maxEdgeLabels = 12

// Graph is the graph up to the cursor: the entities that appeared, and the relations recorded between them so far.
func (p *Project) Graph(ctx context.Context, db *sql.DB, c Cursor) ([]GraphNode, []GraphEdge, error) {
	counts, err := p.visibleCounts(ctx, db, c)
	if err != nil {
		return nil, nil, err
	}
	ents, err := p.entities(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	nodes := []GraphNode{}
	for id, n := range counts {
		e := ents[id]
		nodes = append(nodes, GraphNode{Slug: e.slug, Name: e.name, Kind: e.kind, Page: e.page, Appearances: n})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Appearances > nodes[j].Appearances || (nodes[i].Appearances == nodes[j].Appearances && nodes[i].Slug < nodes[j].Slug)
	})
	rows, err := db.QueryContext(ctx, `SELECT a_id, b_id, label, book, chapter FROM wiki_edges WHERE project_id = $1 ORDER BY book, chapter`, p.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type key struct{ a, b int64 }
	byPair := map[key]*GraphEdge{}
	var order []key
	for rows.Next() {
		var a, b int64
		var g GraphLabel
		if err := rows.Scan(&a, &b, &g.Label, &g.Book, &g.Chapter); err != nil {
			return nil, nil, err
		}
		if !p.Sees(c, g.Book, g.Chapter) || counts[a] == 0 || counts[b] == 0 {
			continue
		}
		if a > b {
			a, b = b, a
		}
		k := key{a, b}
		e := byPair[k]
		if e == nil {
			e = &GraphEdge{A: ents[a].slug, B: ents[b].slug, Labels: []GraphLabel{}}
			byPair[k] = e
			order = append(order, k)
		}
		e.Weight++
		if len(e.Labels) < maxEdgeLabels {
			e.Labels = append(e.Labels, g)
		}
	}
	edges := make([]GraphEdge, 0, len(order))
	for _, k := range order {
		edges = append(edges, *byPair[k])
	}
	return nodes, edges, rows.Err()
}
