package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// Provenance sources recorded next to a field's value.
const (
	sourceManual = "manual"
)

// WorkMetadata is the descriptive metadata of a work with the state of each
// field: which fields are locked against automatic changes and where each
// current value came from.
type WorkMetadata struct {
	Series          string  `json:"series"`
	SeriesIndex     float64 `json:"seriesIndex"`
	ISBN            string  `json:"isbn"`
	Publisher       string  `json:"publisher"`
	Language        string  `json:"language"`
	PublicationDate string  `json:"publicationDate"`
	// OriginalYear is the year the work was first published (DEC-156), null when nobody knows; negative is before the common era.
	OriginalYear *int   `json:"originalYear"`
	Description  string `json:"description"`
	// Unit and ComicKind are chosen by hand (#187): the unit a comic or manga work is of its series, and whether it is a comic or a
	// manga. Empty when nobody said.
	Unit      string `json:"unit"`
	ComicKind string `json:"comicKind"`
	// VolumeNumber is the bound volume that collected a chapter and StoryArc the arc it is in (DEC-169); empty when nobody said.
	VolumeNumber *float64 `json:"volumeNumber"`
	StoryArc     string   `json:"storyArc"`
	// SeriesDirection is how the official collection the work is in says it is read (DEC-166): "ltr", "rtl" or "webtoon"; empty when
	// the work is in none or nobody said.
	SeriesDirection string `json:"seriesDirection"`
	// FirstAuthor is the name of the work's first author as it is stored, not as an account is shown it
	// (a work with two authors is shown "A, B", and "Herbert, Frank" is how a surname-first account sees
	// "Frank Herbert"): what an edit of the author starts from.
	FirstAuthor string `json:"firstAuthor"`
	// AlternativeTitles are the other names of the work (#185): the ones kept for it, and the titles of its editions.
	AlternativeTitles []AlternativeTitle `json:"alternativeTitles"`
	// Contributors are the people credited on the work, by role and place (#185): the authors, the first being the main one.
	Contributors []Contributor `json:"contributors"`
	// Categories are the places of the tree of categories that the work is in (DEC-140), each with its path ("Mangá › Seinen").
	Categories []WorkCategory    `json:"categories"`
	Locks      map[string]bool   `json:"locks"`
	Sources    map[string]string `json:"sources"`
}

// loadMetadata reads the metadata block shown by GET /works/{id}.
func loadMetadata(ctx context.Context, db *sql.DB, workID int, order string) (*WorkMetadata, error) {
	m := &WorkMetadata{Locks: map[string]bool{}, Sources: map[string]string{}, AlternativeTitles: []AlternativeTitle{}, Contributors: []Contributor{}, Categories: []WorkCategory{}}
	var titleL, authorL, seriesL, coverL, isbnL, pubL, langL, dateL, descL, yearL bool
	var year sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(w.series, ''), COALESCE(w.series_index, 0), COALESCE(e.isbn, ''), COALESCE(e.publisher, ''),
		       COALESCE(e.language, ''), COALESCE(e.publication_date, ''), COALESCE(w.description, ''), COALESCE(a.name, ''),
		       COALESCE(w.unit, ''), COALESCE(w.comic_kind, ''), w.original_year, w.volume_number, COALESCE(w.story_arc, ''),
		       COALESCE((SELECT c.reading_direction FROM collection_works cw JOIN collections c ON c.id = cw.collection_id
		                 WHERE cw.work_id = w.id AND cw.official AND c.retired_at IS NULL LIMIT 1), ''),
		       w.title_lock, w.author_lock, w.series_lock, w.cover_lock,
		       w.isbn_lock, w.publisher_lock, w.language_lock, w.publication_date_lock, w.description_lock, w.original_year_lock
		FROM works w LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
		LEFT JOIN LATERAL (`+firstAuthorSQL+`) a ON TRUE
		WHERE w.id = $1`, workID).Scan(
		&m.Series, &m.SeriesIndex, &m.ISBN, &m.Publisher, &m.Language, &m.PublicationDate, &m.Description, &m.FirstAuthor,
		&m.Unit, &m.ComicKind, &year, &m.VolumeNumber, &m.StoryArc, &m.SeriesDirection,
		&titleL, &authorL, &seriesL, &coverL, &isbnL, &pubL, &langL, &dateL, &descL, &yearL)
	if err != nil {
		return nil, err
	}
	if year.Valid {
		y := int(year.Int64)
		m.OriginalYear = &y
	}
	m.Locks = map[string]bool{
		"title": titleL, "author": authorL, "series": seriesL, "cover": coverL,
		"isbn": isbnL, "publisher": pubL, "language": langL, "publication_date": dateL, "description": descL, "original_year": yearL,
	}
	if m.AlternativeTitles, err = loadAlternativeTitles(db, workID); err != nil {
		return nil, err
	}
	if m.Contributors, err = loadContributors(db, workID, order); err != nil {
		return nil, err
	}
	if m.Categories, err = loadWorkCategories(ctx, db, workID); err != nil {
		return nil, err
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
	// OriginalYear is the year the work was first published, as text: "" when nobody knows (DEC-156).
	OriginalYear string
	// Unit and ComicKind say what a comic or manga work is (#187): "volume", "chapter" or "oneshot", and "comic" or "manga".
	// Either may be empty.
	Unit      string
	ComicKind string
	// VolumeNumber and StoryArc say where a chapter sits in its series (DEC-169): "" when nobody said. The number is kept as text, as it is written.
	VolumeNumber string
	StoryArc     string
}

// validYear says whether a text is a year a work can have been first published in: whole, not zero (there is no year 0), from 3000 before the
// common era up to next year. Empty is none, and valid: it clears.
func validYear(s string) bool {
	if s == "" {
		return true
	}
	y, err := strconv.Atoi(s)
	return err == nil && y != 0 && y >= -3000 && y <= time.Now().Year()+1 && strconv.Itoa(y) == s
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
	"publication_date": "publication_date_lock", "description": "description_lock", "original_year": "original_year_lock",
}

// readWorkFields loads the current values, locking the row for the transaction.
func readWorkFields(tx *sql.Tx, workID int) (workFields, bool, error) {
	var f workFields
	var retired bool
	err := tx.QueryRow(`
		SELECT w.original_title, COALESCE(a.name, 'Unknown Author'), COALESCE(w.series, ''), COALESCE(w.series_index, 0),
		       COALESCE(e.isbn, ''), COALESCE(e.publisher, ''), COALESCE(e.language, ''),
		       COALESCE(e.publication_date, ''), COALESCE(w.description, ''), COALESCE(w.unit, ''), COALESCE(w.comic_kind, ''), w.retired_at IS NOT NULL,
		       COALESCE(w.original_year::text, ''), COALESCE(w.volume_number::text, ''), COALESCE(w.story_arc, '')
		FROM works w
		LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
		LEFT JOIN LATERAL (`+firstAuthorSQL+`) a ON TRUE
		WHERE w.id = $1 FOR UPDATE OF w`, workID).Scan(
		&f.Title, &f.Author, &f.Series, &f.SeriesIndex, &f.ISBN, &f.Publisher, &f.Language,
		&f.PublicationDate, &f.Description, &f.Unit, &f.ComicKind, &retired, &f.OriginalYear, &f.VolumeNumber, &f.StoryArc)
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
	// isbn, publisher, language and publication date describe the primary edition.
	editionSets := []string{}
	editionArgs := []any{workID}
	addEdition := func(col string, val any) {
		editionArgs = append(editionArgs, val)
		editionSets = append(editionSets, col+" = $"+strconv.Itoa(len(editionArgs)))
	}
	newAuthor := ""
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
	// The year the work was first published (DEC-156): a column of the work, as a number.
	if next.OriginalYear != cur.OriginalYear {
		var value sql.NullInt64
		if next.OriginalYear != "" {
			y, err := strconv.Atoi(next.OriginalYear)
			if err != nil {
				return nil, err
			}
			value = sql.NullInt64{Int64: int64(y), Valid: true}
		}
		add("original_year", value)
		note("original_year", cur.OriginalYear, next.OriginalYear)
	}
	// What a comic or manga work is (#187): no lock of its own, since nothing but a person writes them.
	for _, f := range []struct{ name, from, to string }{
		{"unit", cur.Unit, next.Unit},
		{"comic_kind", cur.ComicKind, next.ComicKind},
		{"volume_number", cur.VolumeNumber, next.VolumeNumber},
		{"story_arc", cur.StoryArc, next.StoryArc},
	} {
		if f.to != f.from {
			add(f.name, sql.NullString{String: f.to, Valid: f.to != ""})
			note(f.name, f.from, f.to)
		}
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
			value := sql.NullString{String: f.to, Valid: f.to != ""}
			if f.name == "description" {
				add(f.col, value)
			} else {
				addEdition(f.col, value)
			}
			note(f.name, f.from, f.to)
		}
	}
	if next.Author != cur.Author {
		newAuthor = next.Author
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
	if len(editionSets) > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE editions SET "+strings.Join(editionSets, ", ")+" WHERE work_id = $1 AND is_primary", editionArgs...); err != nil {
			return nil, err
		}
	}
	if newAuthor != "" {
		if err := setFirstAuthor(ctx, tx, workID, newAuthor); err != nil {
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
	// Keys are the identifiers accepting the suggestion would keep for a person (DEC-095), worked out by the
	// same rule as accepting, so what the admin is shown is what is done.
	Keys []CandidateKey `json:"keys"`
}

// CandidateKey is an identifier a source gave a person named in a suggestion.
type CandidateKey struct {
	Name   string `json:"name"`
	Scheme string `json:"scheme"`
	Value  string `json:"value"`
}

// candidateKeys lists the identifiers accepting the suggestion would keep: for an author, those of the author;
// for contributors, those of each person listed. Any other field keeps none.
func candidateKeys(field, value string, evidence []byte) []CandidateKey {
	keys := []CandidateKey{}
	var names []string
	switch field {
	case "author":
		names = []string{value}
	case "contributors":
		var items []contributorSuggestion
		if json.Unmarshal([]byte(value), &items) != nil {
			return keys
		}
		for _, it := range items {
			if contributorRoles[it.Role] {
				names = append(names, it.Name)
			}
		}
	}
	for _, name := range names {
		name = strings.Join(strings.Fields(name), " ")
		ids := people.IDsFor(evidence, name)
		schemes := make([]string, 0, len(ids))
		for scheme := range ids {
			schemes = append(schemes, scheme)
		}
		sort.Strings(schemes)
		for _, scheme := range schemes {
			keys = append(keys, CandidateKey{Name: name, Scheme: scheme, Value: ids[scheme]})
		}
	}
	return keys
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
		         WHEN 'author' THEN COALESCE(a.name, '')
		         WHEN 'series' THEN COALESCE(w.series, '')
		         WHEN 'series_index' THEN COALESCE(w.series_index, 0)::text
		         WHEN 'isbn' THEN COALESCE(e.isbn, '')
		         WHEN 'language' THEN COALESCE(e.language, '')
		         WHEN 'publisher' THEN COALESCE(e.publisher, '')
		         WHEN 'publication_date' THEN COALESCE(e.publication_date, '')
		         WHEN 'description' THEN COALESCE(w.description, '')
		         WHEN 'original_year' THEN COALESCE(w.original_year::text, '')
		         WHEN 'contributors' THEN COALESCE((SELECT string_agg(p.name || ' (' || k.role || ')', '; ' ORDER BY k.role, k.position, p.name)
		                                            FROM work_contributors k JOIN person p ON p.id = k.person_id WHERE k.work_id = w.id), '')
		         ELSE '' END
		FROM metadata_candidates c
		JOIN works w ON w.id = c.work_id
		LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
		LEFT JOIN LATERAL (`+firstAuthorSQL+`) a ON TRUE
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
		c.Keys = candidateKeys(c.Field, c.Value, c.Evidence)
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
	var evidence []byte
	err = tx.QueryRow(`SELECT field, value, source, evidence FROM metadata_candidates
		WHERE id = $1 AND work_id = $2 AND state = 'pending' FOR UPDATE`, candID, workID).Scan(&field, &value, &source, &evidence)
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
		case "language":
			next.Language = value
		case "publisher":
			next.Publisher = value
		case "publication_date":
			next.PublicationDate = value
		case "description":
			next.Description = value
		case "original_year":
			if !validYear(value) || value == "" {
				http.Error(w, "Candidate value is not a year", http.StatusUnprocessableEntity)
				return
			}
			next.OriginalYear = value
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
		case "contributors":
			var items []contributorSuggestion
			if json.Unmarshal([]byte(value), &items) != nil {
				http.Error(w, "Candidate contributors are malformed", http.StatusUnprocessableEntity)
				return
			}
			added, err := addContributors(r.Context(), tx, workID, items, source, evidence)
			if err != nil {
				http.Error(w, "Error adding contributors", http.StatusInternalServerError)
				return
			}
			details["added"] = added
		default:
			http.Error(w, "Unsupported candidate field", http.StatusUnprocessableEntity)
			return
		}
		// Tags and contributors only add: they do not settle a field, so other proposals for them stay.
		if field != "tags" && field != "contributors" {
			changes, err := applyWorkFields(r.Context(), tx, workID, actor, source, cur, retired, next, nil)
			if err != nil {
				http.Error(w, "Error applying candidate", http.StatusInternalServerError)
				return
			}
			details["changes"] = auditDetails(changes)
			if field == "author" {
				if err := recordAuthorAuthority(r.Context(), tx, value, source, evidence); err != nil {
					http.Error(w, "Error recording author identity", http.StatusInternalServerError)
					return
				}
			}
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

// maxTagRunes is what tags.name holds (VARCHAR(50)).
const maxTagRunes = 50

// normalizeTag is a tag as the database can hold it: a suggestion is free text, and one the column
// cannot hold must be shortened, not fail the decision. It is cut at a word when it can be and never
// ends on punctuation. The worker does the same to the subjects of a file (analyzer.clean_tag).
func normalizeTag(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	runes := []rune(name)
	if len(runes) <= maxTagRunes {
		return name
	}
	cut := runes[:maxTagRunes]
	for i := len(cut) - 1; i >= maxTagRunes/2; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " ,;:.-–—/")
}

// addTags links tags to a work, creating the ones that do not exist yet.
func addTags(tx *sql.Tx, workID int, names []string) error {
	for _, name := range names {
		name = normalizeTag(name)
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

// firstAuthorSQL selects the name of a work's first author. It expects the work
// to be aliased w and is meant to be used as a LATERAL subquery.
const firstAuthorSQL = `
	SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
	WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.position, p.name LIMIT 1`

// contributorSuggestion is one person a provider credits on a work, with the role the library keeps for them.
type contributorSuggestion struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

var contributorRoles = map[string]bool{"author": true, "translator": true, "narrator": true, "editor": true, "illustrator": true}

const (
	maxSuggestedContributors = 50
	maxContributorNameRunes  = 200
)

// addContributors adds the people of an accepted suggestion to the work, each in their role and after whoever
// the work already has in it. It only adds: nobody the work has is removed or moved, and someone who is
// already there in that role stays as they are. A role the library has no word for, or a name that is empty or
// too long, is left out rather than failing the decision. The identifiers the source gave each name are kept for
// the person, as for an accepted author. It returns who was added.
func addContributors(ctx context.Context, tx *sql.Tx, workID int, items []contributorSuggestion, source string, evidence []byte) ([]string, error) {
	added := []string{}
	if len(items) > maxSuggestedContributors {
		items = items[:maxSuggestedContributors]
	}
	for _, it := range items {
		name := strings.Join(strings.Fields(it.Name), " ")
		if name == "" || len([]rune(name)) > maxContributorNameRunes || !contributorRoles[it.Role] {
			continue
		}
		personID, err := people.Resolve(ctx, tx, name)
		if err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO work_contributors (work_id, person_id, role, position)
			SELECT $1::int, $2::int, $3::varchar, COALESCE((SELECT max(position) + 1 FROM work_contributors WHERE work_id = $1 AND role = $3), 0)
			ON CONFLICT DO NOTHING`, workID, personID, it.Role)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added = append(added, name+" ("+it.Role+")")
		}
		if err := people.RecordAuthority(ctx, tx, personID, source, people.IDsFor(evidence, name)); err != nil {
			return nil, err
		}
	}
	return added, nil
}

// recordAuthorAuthority keeps, for the person an accepted author stands for, the identifiers the source gave
// that author in the record the suggestion came from. A human accepting the name is what ties the identifier
// to the person; a name the record does not credit, or credits twice, brings none.
func recordAuthorAuthority(ctx context.Context, tx *sql.Tx, name, source string, evidence []byte) error {
	ids := people.IDsFor(evidence, name)
	if len(ids) == 0 {
		return nil
	}
	personID, err := people.Resolve(ctx, tx, name)
	if err != nil {
		return err
	}
	return people.RecordAuthority(ctx, tx, personID, source, ids)
}

// setFirstAuthor makes name the work's first author, creating the person if needed. The person goes by
// the name people say ("Herbert, Frank, author" is Frank Herbert) and what was written is kept as an
// alias, so it still finds the work (#36). Other contributors are left as they are.
func setFirstAuthor(ctx context.Context, tx *sql.Tx, workID int, name string) error {
	personID, err := people.Resolve(ctx, tx, name)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_contributors WHERE work_id = $1 AND role = 'author' AND position = 0`, workID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO work_contributors (work_id, person_id, role, position) VALUES ($1, $2, 'author', 0)
		ON CONFLICT DO NOTHING`, workID, personID)
	return err
}
