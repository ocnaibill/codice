package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// Provenance sources recorded next to a field's value.
const (
	sourceManual = "manual"
)

// WorkMetadata is the descriptive metadata of a work with the state of each
// field: which fields are locked against automatic changes and where each
// current value came from.
type WorkMetadata struct {
	Series          string            `json:"series"`
	SeriesIndex     float64           `json:"seriesIndex"`
	ISBN            string            `json:"isbn"`
	Publisher       string            `json:"publisher"`
	Language        string            `json:"language"`
	PublicationDate string            `json:"publicationDate"`
	Description     string            `json:"description"`
	Locks           map[string]bool   `json:"locks"`
	Sources         map[string]string `json:"sources"`
}

// loadMetadata reads the metadata block shown by GET /works/{id}.
func loadMetadata(ctx context.Context, db *sql.DB, workID int) (*WorkMetadata, error) {
	m := &WorkMetadata{Locks: map[string]bool{}, Sources: map[string]string{}}
	var titleL, authorL, seriesL, coverL, isbnL, pubL, langL, dateL, descL bool
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(series, ''), COALESCE(series_index, 0), COALESCE(isbn, ''), COALESCE(publisher, ''),
		       COALESCE(language, ''), COALESCE(publication_date, ''), COALESCE(description, ''),
		       title_lock, author_lock, series_lock, cover_lock,
		       isbn_lock, publisher_lock, language_lock, publication_date_lock, description_lock
		FROM works WHERE id = $1`, workID).Scan(
		&m.Series, &m.SeriesIndex, &m.ISBN, &m.Publisher, &m.Language, &m.PublicationDate, &m.Description,
		&titleL, &authorL, &seriesL, &coverL, &isbnL, &pubL, &langL, &dateL, &descL)
	if err != nil {
		return nil, err
	}
	m.Locks = map[string]bool{
		"title": titleL, "author": authorL, "series": seriesL, "cover": coverL,
		"isbn": isbnL, "publisher": pubL, "language": langL, "publication_date": dateL, "description": descL,
	}
	rows, err := db.QueryContext(ctx, `SELECT field, source FROM work_field_sources WHERE work_id = $1`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f, s string
		if err := rows.Scan(&f, &s); err != nil {
			return nil, err
		}
		m.Sources[f] = s
	}
	return m, rows.Err()
}

// workFields are the editable descriptive fields of a work. Title and author
// are always present; the others are pointers so that "not sent" is not
// mistaken for "clear this field".
type workFields struct {
	Title           string
	Author          string
	Series          string
	SeriesIndex     float64
	ISBN            string
	Publisher       string
	Language        string
	PublicationDate string
	Description     string
}

// fieldChange records one field that actually changed.
type fieldChange struct {
	Field string
	From  string
	To    string
}

// fieldLocks maps a field to the column holding its lock. series_index shares
// the lock of series.
var fieldLocks = map[string]string{
	"title": "title_lock", "author": "author_lock", "series": "series_lock", "cover": "cover_lock",
	"isbn": "isbn_lock", "publisher": "publisher_lock", "language": "language_lock",
	"publication_date": "publication_date_lock", "description": "description_lock",
}

// readWorkFields loads the current values, locking the row for the transaction.
func readWorkFields(tx *sql.Tx, workID int) (workFields, bool, error) {
	var f workFields
	var retired bool
	err := tx.QueryRow(`
		SELECT w.original_title, COALESCE(p.name, 'Unknown Author'), COALESCE(w.series, ''), COALESCE(w.series_index, 0),
		       COALESCE(w.isbn, ''), COALESCE(w.publisher, ''), COALESCE(w.language, ''),
		       COALESCE(w.publication_date, ''), COALESCE(w.description, ''), w.retired_at IS NOT NULL
		FROM works w LEFT JOIN person p ON p.id = w.author_id
		WHERE w.id = $1 FOR UPDATE OF w`, workID).Scan(
		&f.Title, &f.Author, &f.Series, &f.SeriesIndex, &f.ISBN, &f.Publisher, &f.Language,
		&f.PublicationDate, &f.Description, &retired)
	return f, retired, err
}

// applyWorkFields writes the new values, and records what changed. Every field
// that changes becomes confirmed: it is locked against automatic changes and its
// provenance is set to source. A lock the person set explicitly is honoured for
// fields that did not change (so they can protect or release a field), but a
// changed field is always locked. The bibliographic reference on the user's
// notes follows a confirmed title or author while the work is available.
func applyWorkFields(ctx context.Context, tx *sql.Tx, workID int, actor, source string,
	cur workFields, retired bool, next workFields, explicitLocks map[string]*bool) ([]fieldChange, error) {

	var changes []fieldChange
	sets := []string{}
	args := []any{workID}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, col+" = $"+strconv.Itoa(len(args)))
	}
	lockNow := map[string]bool{}
	note := func(field, from, to string) {
		changes = append(changes, fieldChange{Field: field, From: from, To: to})
		lockNow[field] = true
	}

	if next.Title != cur.Title {
		add("original_title", next.Title)
		note("title", cur.Title, next.Title)
	}
	if next.Series != cur.Series {
		add("series", next.Series)
		note("series", cur.Series, next.Series)
	}
	if next.SeriesIndex != cur.SeriesIndex {
		add("series_index", next.SeriesIndex)
		note("series", strconv.FormatFloat(cur.SeriesIndex, 'f', -1, 64), strconv.FormatFloat(next.SeriesIndex, 'f', -1, 64))
	}
	for _, f := range []struct {
		name, col string
		from, to  string
	}{
		{"isbn", "isbn", cur.ISBN, next.ISBN},
		{"publisher", "publisher", cur.Publisher, next.Publisher},
		{"language", "language", cur.Language, next.Language},
		{"publication_date", "publication_date", cur.PublicationDate, next.PublicationDate},
		{"description", "description", cur.Description, next.Description},
	} {
		if f.to != f.from {
			// A cleared field is stored as NULL, not as an empty string.
			add(f.col, sql.NullString{String: f.to, Valid: f.to != ""})
			note(f.name, f.from, f.to)
		}
	}
	if next.Author != cur.Author {
		var authorID int
		err := tx.QueryRowContext(ctx, `SELECT id FROM person WHERE name = $1`, next.Author).Scan(&authorID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `INSERT INTO person (name) VALUES ($1) RETURNING id`, next.Author).Scan(&authorID)
		}
		if err != nil {
			return nil, err
		}
		add("author_id", authorID)
		note("author", cur.Author, next.Author)
	}

	// Locks: changed means confirmed; otherwise an explicit choice, if any.
	for field, col := range fieldLocks {
		switch {
		case lockNow[field]:
			add(col, true)
		case explicitLocks[field] != nil:
			add(col, *explicitLocks[field])
		}
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
		if _, err := tx.ExecContext(ctx, "UPDATE works SET "+strings.Join(sets, ", ")+" WHERE id = $1", args...); err != nil {
			return nil, err
		}
	}

	for _, c := range changes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_field_sources (work_id, field, source, actor_id)
			VALUES ($1, $2, $3, NULLIF($4, '')::uuid)
			ON CONFLICT (work_id, field) DO UPDATE SET source = EXCLUDED.source, actor_id = EXCLUDED.actor_id, updated_at = now()`,
			workID, c.Field, source, actor); err != nil {
			return nil, err
		}
	}

	if (lockNow["title"] || lockNow["author"]) && !retired {
		if _, err := tx.ExecContext(ctx, `
			UPDATE notes SET source_title = $1, source_author = NULLIF($2, 'Unknown Author') WHERE work_id = $3`,
			next.Title, next.Author, workID); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// auditDetails turns changes into audit details. Long text is shortened: the
// log records that a field changed, not a copy of the description.
func auditDetails(changes []fieldChange) map[string]any {
	short := func(s string) string {
		if r := []rune(s); len(r) > 120 {
			return string(r[:120]) + "…"
		}
		return s
	}
	d := map[string]any{}
	for _, c := range changes {
		d[c.Field] = map[string]string{"from": short(c.From), "to": short(c.To)}
	}
	return d
}

// Candidate is a suggestion from an external provider waiting for a decision.
type Candidate struct {
	ID        int64           `json:"id"`
	Field     string          `json:"field"`
	Value     string          `json:"value"`
	Source    string          `json:"source"`
	Evidence  json.RawMessage `json:"evidence"`
	CreatedAt string          `json:"createdAt"`
	Current   string          `json:"current"`
}

// ListCandidates returns the pending suggestions for a work, with the current
// value next to each so the admin can compare.
func (h *LibraryHandler) ListCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var exists bool
	if err := h.DB.QueryRow(`SELECT EXISTS (SELECT 1 FROM works WHERE id = $1)`, id).Scan(&exists); err != nil || !exists {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	rows, err := h.DB.Query(`
		SELECT c.id, c.field, c.value, c.source, c.evidence, c.created_at,
		       CASE c.field
		         WHEN 'title' THEN w.original_title
		         WHEN 'author' THEN COALESCE(p.name, '')
		         WHEN 'series' THEN COALESCE(w.series, '')
		         WHEN 'series_index' THEN COALESCE(w.series_index, 0)::text
		         WHEN 'isbn' THEN COALESCE(w.isbn, '')
		         WHEN 'description' THEN COALESCE(w.description, '')
		         ELSE '' END
		FROM metadata_candidates c
		JOIN works w ON w.id = c.work_id
		LEFT JOIN person p ON p.id = w.author_id
		WHERE c.work_id = $1 AND c.state = 'pending'
		ORDER BY c.field, c.id`, id)
	if err != nil {
		http.Error(w, "Error fetching candidates", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Field, &c.Value, &c.Source, &c.Evidence, &c.CreatedAt, &c.Current); err != nil {
			http.Error(w, "Error reading candidates", http.StatusInternalServerError)
			return
		}
		out = append(out, c)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// AcceptCandidate applies a pending suggestion as a confirmed value: the field
// is locked, its provenance names the provider, and other pending suggestions
// for the same field are dismissed because the field is now settled.
func (h *LibraryHandler) AcceptCandidate(w http.ResponseWriter, r *http.Request) {
	h.decideCandidate(w, r, true)
}

// RejectCandidate dismisses a suggestion. It is remembered, so the same value
// from the same provider is not proposed again.
func (h *LibraryHandler) RejectCandidate(w http.ResponseWriter, r *http.Request) {
	h.decideCandidate(w, r, false)
}

func (h *LibraryHandler) decideCandidate(w http.ResponseWriter, r *http.Request, accept bool) {
	workID, ok := workIDParam(r)
	candID, err := strconv.ParseInt(chi.URLParam(r, "candidateID"), 10, 64)
	if !ok || err != nil {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	actor := currentUserID(r)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	cur, retired, err := readWorkFields(tx, workID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error reading book", http.StatusInternalServerError)
		return
	}

	var field, value, source string
	err = tx.QueryRow(`SELECT field, value, source FROM metadata_candidates
		WHERE id = $1 AND work_id = $2 AND state = 'pending' FOR UPDATE`, candID, workID).Scan(&field, &value, &source)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error reading candidate", http.StatusInternalServerError)
		return
	}

	action := "metadata.reject"
	details := map[string]any{"field": field, "source": source}
	if accept {
		action = "metadata.accept"
		next := cur
		switch field {
		case "title":
			next.Title = value
		case "author":
			next.Author = value
		case "series":
			next.Series = value
		case "series_index":
			v, perr := strconv.ParseFloat(value, 64)
			if perr != nil {
				http.Error(w, "Candidate value is not a number", http.StatusUnprocessableEntity)
				return
			}
			next.SeriesIndex = v
		case "isbn":
			next.ISBN = value
		case "description":
			next.Description = value
		case "tags":
			var names []string
			if json.Unmarshal([]byte(value), &names) != nil {
				http.Error(w, "Candidate tags are malformed", http.StatusUnprocessableEntity)
				return
			}
			if err := addTags(tx, workID, names); err != nil {
				http.Error(w, "Error adding tags", http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "Unsupported candidate field", http.StatusUnprocessableEntity)
			return
		}
		if field != "tags" {
			changes, err := applyWorkFields(r.Context(), tx, workID, actor, source, cur, retired, next, nil)
			if err != nil {
				http.Error(w, "Error applying candidate", http.StatusInternalServerError)
				return
			}
			details["changes"] = auditDetails(changes)
			// The field is settled: dismiss the other proposals for it.
			if _, err := tx.Exec(`UPDATE metadata_candidates SET state = 'rejected', decided_at = now()
				WHERE work_id = $1 AND field = $2 AND state = 'pending' AND id <> $3`, workID, field, candID); err != nil {
				http.Error(w, "Error updating candidates", http.StatusInternalServerError)
				return
			}
		}
	}

	state := "rejected"
	if accept {
		state = "accepted"
	}
	if _, err := tx.Exec(`UPDATE metadata_candidates SET state = $1, decided_at = now(), decided_by = NULLIF($2, '')::uuid WHERE id = $3`,
		state, actor, candID); err != nil {
		http.Error(w, "Error updating candidate", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, actor, action, "work", strconv.Itoa(workID), details); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing decision", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// addTags links tags to a work, creating the ones that do not exist yet.
func addTags(tx *sql.Tx, workID int, names []string) error {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tagID int
		err := tx.QueryRow(`SELECT id FROM tags WHERE name = $1`, name).Scan(&tagID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRow(`INSERT INTO tags (name) VALUES ($1) RETURNING id`, name).Scan(&tagID)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO work_tags (work_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, workID, tagID); err != nil {
			return err
		}
	}
	return nil
}

