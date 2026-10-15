package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// What an update changed and what the staff does about it (DEC-180, RF-052): the entities that are new, the sections that came to the
// ones that already existed, "approve" (it was seen) and "undo" (hide what that entity gained). Marking a sentence wrong is the same
// mechanism, one sentence at a time.

// SectionKey names a section of a page across generations: the same kind, book, first chapter and part is the same section.
type SectionKey struct {
	Kind string `json:"kind"`
	Book int    `json:"book"`
	From int    `json:"from"`
	Part string `json:"part"`
}

// Change is one entity an update touched.
type Change struct {
	Slug  string       `json:"slug"`
	Name  string       `json:"name"`
	Kind  string       `json:"kind"`
	Page  bool         `json:"page"`
	New   bool         `json:"new"`
	Added []SectionKey `json:"added,omitempty"`
	// Labels say, in words, what came: "Quem é, até Parte II", "Biografia, Livro 2 · Parte I".
	Labels []string `json:"labels,omitempty"`
	Undone bool     `json:"undone,omitempty"`
}

const maxChanges = 300

func sectionKeyOf(s Section) SectionKey {
	k := SectionKey{Kind: s.Kind, Book: -1, Part: s.Part}
	if s.Book != nil {
		k.Book = *s.Book
		k.From = s.ChapterFrom
	}
	return k
}

func labelOf(k SectionKey, books []Book) string {
	book := ""
	if k.Book >= 0 && k.Book < len(books) {
		book = books[k.Book].Title
	}
	where := book
	if k.Part != "" {
		if where != "" {
			where += ", "
		}
		where += k.Part
	}
	switch k.Kind {
	case "who":
		return "Quem é, até " + orDash(where)
	case "bio":
		return "Biografia, " + orDash(where)
	case "personality":
		return "Personalidade"
	}
	return k.Kind
}

func orDash(s string) string {
	if s == "" {
		return "o fim"
	}
	return s
}

// snapshotSections reads the sections of the project that is about to be replaced: slug → the keys it had. Entities with no section are
// there with an empty set, so that "existed before" can be told from "is new".
func snapshotSections(ctx context.Context, tx *sql.Tx, projectID int64) (map[string]map[SectionKey]bool, error) {
	out := map[string]map[SectionKey]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT slug FROM wiki_entities WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			rows.Close()
			return nil, err
		}
		out[slug] = map[SectionKey]bool{}
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `
		SELECT e.slug, sec.kind, COALESCE(sec.book, -1), COALESCE(sec.chapter_from, 0), sec.part
		FROM wiki_sections sec JOIN wiki_entities e ON e.id = sec.entity_id WHERE e.project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var k SectionKey
		if err := rows.Scan(&slug, &k.Kind, &k.Book, &k.From, &k.Part); err != nil {
			return nil, err
		}
		if out[slug] != nil {
			out[slug][k] = true
		}
	}
	return out, rows.Err()
}

// diffBundle says what the bundle brings that the project it replaces did not have. Pages come first, the biggest changes first.
func diffBundle(before map[string]map[SectionKey]bool, b *Bundle) []Change {
	var out []Change
	for _, e := range b.Entities {
		had, existed := before[e.Slug]
		c := Change{Slug: e.Slug, Name: e.Name, Kind: e.Kind, Page: e.Page, New: !existed}
		seen := map[SectionKey]bool{}
		for _, s := range e.Sections {
			k := sectionKeyOf(s)
			if seen[k] || len(s.Sentences) == 0 {
				continue
			}
			seen[k] = true
			if !had[k] {
				c.Added = append(c.Added, k)
				c.Labels = append(c.Labels, labelOf(k, b.Books))
			}
		}
		if c.New && !e.Page {
			continue // a name that is only in the graph is not worth a line of review
		}
		if c.New || len(c.Added) > 0 {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].New != out[j].New {
			return out[i].New
		}
		if out[i].Page != out[j].Page {
			return out[i].Page
		}
		return len(out[i].Added) > len(out[j].Added)
	})
	if len(out) > maxChanges {
		out = out[:maxChanges]
	}
	return out
}

// Review is what the staff sees of an update.
type Review struct {
	State   string   `json:"review"` // none or pending
	Changes []Change `json:"changes"`
}

// LoadReview reads the review of a project.
func LoadReview(ctx context.Context, db *sql.DB, projectID int64) (*Review, error) {
	var state string
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT review, changes FROM wiki_projects WHERE id = $1`, projectID).Scan(&state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r := &Review{State: state, Changes: []Change{}}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &r.Changes); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Approve says the update was seen: the review is closed. What is hidden stays hidden.
func Approve(ctx context.Context, db *sql.DB, projectID int64, userID string) error {
	res, err := db.ExecContext(ctx, `UPDATE wiki_projects SET review = 'none' WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return audit.Record(ctx, db, userID, "wiki.review.approve", "wiki", strconv.FormatInt(projectID, 10), nil)
}

// Undo hides what an entity gained in the update (the sections it added), for everyone: the staff can show a sentence again one by one.
func Undo(ctx context.Context, db *sql.DB, projectID int64, slug, userID string) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT changes FROM wiki_projects WHERE id = $1 FOR UPDATE`, projectID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	var changes []Change
	if err := json.Unmarshal(raw, &changes); err != nil {
		return 0, err
	}
	at := -1
	for i, c := range changes {
		if c.Slug == slug {
			at = i
		}
	}
	if at < 0 {
		return 0, ErrNotFound
	}
	hidden := 0
	for _, k := range changes[at].Added {
		res, err := tx.ExecContext(ctx, `
			UPDATE wiki_sentences s SET hidden_at = now(), hidden_by = NULLIF($6, '')::uuid
			FROM wiki_sections sec JOIN wiki_entities e ON e.id = sec.entity_id
			WHERE s.section_id = sec.id AND e.project_id = $1 AND e.slug = $2 AND s.hidden_at IS NULL
			  AND sec.kind = $3 AND COALESCE(sec.book, -1) = $4 AND COALESCE(sec.chapter_from, 0) = $5 AND sec.part = $7`,
			projectID, slug, k.Kind, k.Book, k.From, userID, k.Part)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		hidden += int(n)
	}
	changes[at].Undone = true
	body, _ := json.Marshal(changes)
	if _, err := tx.ExecContext(ctx, `UPDATE wiki_projects SET changes = $2::jsonb WHERE id = $1`, projectID, body); err != nil {
		return 0, err
	}
	if err := audit.Record(ctx, tx, userID, "wiki.review.undo", "wiki", strconv.FormatInt(projectID, 10),
		map[string]any{"entity": slug, "sentences": hidden}); err != nil {
		return 0, err
	}
	return hidden, tx.Commit()
}

// SetSentenceHidden marks a sentence as wrong (it is shown to no reader, RF-052) or shows it again. A sentence marked wrong is still
// there for the staff, struck through, and survives a new generation that writes the same text.
func SetSentenceHidden(ctx context.Context, db *sql.DB, projectID, sentenceID int64, hidden bool, userID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE wiki_sentences s SET hidden_at = CASE WHEN $3 THEN COALESCE(s.hidden_at, now()) END,
		                            hidden_by = CASE WHEN $3 THEN COALESCE(s.hidden_by, NULLIF($4, '')::uuid) END
		FROM wiki_sections sec JOIN wiki_entities e ON e.id = sec.entity_id
		WHERE s.id = $2 AND s.section_id = sec.id AND e.project_id = $1`, projectID, sentenceID, hidden, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	action := "wiki.sentence.hide"
	if !hidden {
		action = "wiki.sentence.show"
	}
	return audit.Record(ctx, db, userID, action, "wiki", strconv.FormatInt(projectID, 10), map[string]any{"sentence": fmt.Sprint(sentenceID)})
}
