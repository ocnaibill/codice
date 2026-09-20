package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/locator"
)

// NotesHandler serves a person's marginalia: notes, highlights and bookmarks. Everything here
// is scoped to the caller; someone else's note is not found, never forbidden, so its existence
// is not disclosed either.
type NotesHandler struct {
	DB *sql.DB
}

const (
	maxQuote   = 2000  // characters of a saved quotation; a longer one is cut
	maxBody    = 20000 // characters of the person's own text
	maxTags    = 20
	maxTagLen  = 40
	maxSearch  = 200
	exportCap  = 10000
	kindNote   = "note"
	kindMark   = "highlight"
	kindBookmk = "bookmark"
)

// Note is a piece of marginalia. Its title and author come from the reference stored on the
// note, so it stays complete when the work is retired or deleted; SourceAvailable then turns
// false and WorkID is null (RF-039). Body is Markdown, written by the person: clients must show
// it without interpreting HTML in it.
type Note struct {
	ID              int             `json:"id"`
	Kind            string          `json:"kind"`
	WorkID          *int            `json:"workId"`
	WorkTitle       string          `json:"workTitle"`
	WorkAuthor      string          `json:"workAuthor"`
	FileID          *int64          `json:"fileId"`
	FileFormat      string          `json:"fileFormat,omitempty"`
	Quote           string          `json:"quote"`
	Body            string          `json:"body"`
	Tags            []string        `json:"tags"`
	Locator         json.RawMessage `json:"locator"`
	LocatorVersion  *int            `json:"locatorVersion"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	SourceAvailable bool            `json:"sourceAvailable"`
	// FileAvailable says whether the exact file the note points into can still be opened.
	FileAvailable bool `json:"fileAvailable"`
}

// CreateNoteRequest saves marginalia against a work. Quote is the passage kept from the
// source, Body the person's own words (Markdown). FileID and Locator say where in which file:
// a locator needs the file, and is checked against its format.
type CreateNoteRequest struct {
	Kind    string          `json:"kind"`
	Quote   string          `json:"quote"`
	Body    string          `json:"body"`
	Tags    []string        `json:"tags"`
	FileID  *int64          `json:"fileId"`
	Locator json.RawMessage `json:"locator"`
}

// cleanText makes the text a person typed safe to keep: valid UTF-8, no control characters
// other than newlines and tabs, "\n" for line ends, trimmed. It does not touch markup: the
// text is stored as typed and shown as text.
func cleanText(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("text is not valid UTF-8")
	}
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s), nil
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// cleanTags trims and collapses each tag, drops empty ones and repeats (ignoring case), and
// refuses more or longer than the limits: a tag list is small and personal.
func cleanTags(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && !unicode.IsSpace(r) {
				return -1
			}
			return r
		}, t)), " ")
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		if !utf8.ValidString(t) || utf8.RuneCountInString(t) > maxTagLen {
			return nil, fmt.Errorf("a tag has at most %d characters", maxTagLen)
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	if len(out) > maxTags {
		return nil, fmt.Errorf("at most %d tags", maxTags)
	}
	return out, nil
}

func isRaw(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null"
}

// CreateNote saves marginalia against a work, for the caller.
func (h *NotesHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)

	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var req CreateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	quote, err1 := cleanText(req.Quote)
	body, err2 := cleanText(req.Body)
	tags, err3 := cleanTags(req.Tags)
	if err := errors.Join(err1, err2, err3); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	quote = clip(quote, maxQuote)
	if utf8.RuneCountInString(body) > maxBody {
		http.Error(w, fmt.Sprintf("The note is longer than %d characters", maxBody), http.StatusBadRequest)
		return
	}

	kind := req.Kind
	if kind == "" {
		if body != "" {
			kind = kindNote
		} else {
			kind = kindMark
		}
	}
	if kind != kindNote && kind != kindMark && kind != kindBookmk {
		http.Error(w, "kind is note, highlight or bookmark", http.StatusBadRequest)
		return
	}
	hasLocator := isRaw(req.Locator)
	if kind == kindBookmk && !hasLocator {
		http.Error(w, "A bookmark needs a place", http.StatusBadRequest)
		return
	}
	if kind != kindBookmk && quote == "" && body == "" {
		http.Error(w, "Quote text is required", http.StatusBadRequest)
		return
	}
	if hasLocator && req.FileID == nil {
		http.Error(w, "A place needs the file it is in", http.StatusBadRequest)
		return
	}

	// The work must be one the caller can see, and the file one of its files.
	var exists bool
	if err := h.DB.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM works WHERE id = $1 AND retired_at IS NULL)`, workID).Scan(&exists); err != nil {
		log.Println("Error checking work for a note:", err)
		http.Error(w, "Error creating note", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var canonical []byte
	if req.FileID != nil {
		var format sql.NullString
		err := h.DB.QueryRowContext(r.Context(), `
			SELECT f.format FROM files f JOIN editions e ON e.id = f.edition_id
			WHERE f.id = $1 AND e.work_id = $2`, *req.FileID, workID).Scan(&format)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "That file is not one of this book's files", http.StatusBadRequest)
			return
		}
		if err != nil {
			log.Println("Error checking file for a note:", err)
			http.Error(w, "Error creating note", http.StatusInternalServerError)
			return
		}
		if hasLocator {
			if canonical, err = locator.Validate(format.String, req.Locator); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}

	// The bibliographic reference is copied from the work by the database when the note is created.
	var loc, ver any
	if canonical != nil {
		loc, ver = string(canonical), locator.Version
	}
	var id int
	err := h.DB.QueryRowContext(r.Context(), `
		INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8::jsonb, $9)
		RETURNING id`,
		userID, workID, req.FileID, kind, quote, body, pq.Array(tags), loc, ver).Scan(&id)
	if err != nil {
		log.Println("Error creating note:", err)
		http.Error(w, "Error creating note", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// noteFilter is what the list and the export both accept.
type noteFilter struct {
	workID, fileID int64
	kind, tag, q   string
}

func parseNoteFilter(r *http.Request) (noteFilter, error) {
	q := r.URL.Query()
	var f noteFilter
	var err error
	if v := q.Get("workId"); v != "" {
		if f.workID, err = strconv.ParseInt(v, 10, 64); err != nil || f.workID <= 0 {
			return f, errors.New("workId is a number")
		}
	}
	if v := q.Get("fileId"); v != "" {
		if f.fileID, err = strconv.ParseInt(v, 10, 64); err != nil || f.fileID <= 0 {
			return f, errors.New("fileId is a number")
		}
	}
	f.kind = q.Get("kind")
	if f.kind != "" && f.kind != kindNote && f.kind != kindMark && f.kind != kindBookmk {
		return f, errors.New("kind is note, highlight or bookmark")
	}
	f.tag = strings.TrimSpace(q.Get("tag"))
	f.q = strings.TrimSpace(q.Get("q"))
	if utf8.RuneCountInString(f.q) > 200 {
		return f, errors.New("the search is too long")
	}
	return f, nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// where builds the condition of a filter, always starting with the caller: whatever else is
// asked, only their own notes can match.
func (f noteFilter) where(userID string) (string, []any) {
	cond := []string{"n.user_id = $1"}
	args := []any{userID}
	add := func(c string, v any) {
		args = append(args, v)
		cond = append(cond, strings.ReplaceAll(c, "?", "$"+strconv.Itoa(len(args))))
	}
	if f.workID > 0 {
		add("n.source_work_id = ?", f.workID)
	}
	if f.fileID > 0 {
		add("n.file_id = ?", f.fileID)
	}
	if f.kind != "" {
		add("n.kind = ?", f.kind)
	}
	if f.tag != "" {
		add("EXISTS (SELECT 1 FROM unnest(n.tags) t WHERE lower(t) = lower(?))", f.tag)
	}
	if f.q != "" {
		// One pattern, used in the four places it is looked for.
		add(`(n.quote ILIKE ? ESCAPE '\' OR n.body ILIKE ? ESCAPE '\' OR n.source_title ILIKE ? ESCAPE '\'
		      OR array_to_string(n.tags, ' ') ILIKE ? ESCAPE '\')`, "%"+likeEscaper.Replace(f.q)+"%")
	}
	return strings.Join(cond, " AND "), args
}

const noteSelect = `
	SELECT n.id, n.kind, n.work_id, n.source_title, COALESCE(n.source_author, 'Unknown Author'),
	       n.file_id, COALESCE(f.format, ''), COALESCE(n.quote, ''), n.body, n.tags,
	       n.locator, n.locator_version, n.created_at, n.updated_at,
	       (w.id IS NOT NULL AND w.retired_at IS NULL),
	       (f.id IS NOT NULL AND f.availability = 'available' AND w.id IS NOT NULL AND w.retired_at IS NULL)
	FROM notes n
	LEFT JOIN works w ON w.id = n.work_id
	LEFT JOIN files f ON f.id = n.file_id`

func scanNote(rows *sql.Rows) (Note, error) {
	var n Note
	var workID, fileID, ver sql.NullInt64
	var tags pq.StringArray
	var loc []byte
	if err := rows.Scan(&n.ID, &n.Kind, &workID, &n.WorkTitle, &n.WorkAuthor, &fileID, &n.FileFormat,
		&n.Quote, &n.Body, &tags, &loc, &ver, &n.CreatedAt, &n.UpdatedAt, &n.SourceAvailable, &n.FileAvailable); err != nil {
		return n, err
	}
	if workID.Valid {
		id := int(workID.Int64)
		n.WorkID = &id
	}
	if fileID.Valid {
		n.FileID = &fileID.Int64
	}
	if ver.Valid {
		v := int(ver.Int64)
		n.LocatorVersion = &v
	}
	if loc != nil {
		n.Locator = json.RawMessage(loc)
	}
	n.Tags = []string(tags)
	if n.Tags == nil {
		n.Tags = []string{}
	}
	return n, nil
}

// ListNotes returns the caller's marginalia, newest first, filtered by work, file, kind, tag or
// a search text, with the total that matches for paging.
func (h *NotesHandler) ListNotes(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	f, err := parseNoteFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit, offset := 10, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= maxSearch {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}

	cond, args := f.where(userID)
	var total int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM notes n WHERE `+cond, args...).Scan(&total); err != nil {
		log.Println("Error counting notes:", err)
		http.Error(w, "Error listing notes", http.StatusInternalServerError)
		return
	}
	args = append(args, limit, offset)
	rows, err := h.DB.QueryContext(r.Context(), noteSelect+` WHERE `+cond+
		fmt.Sprintf(` ORDER BY n.created_at DESC, n.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		log.Println("Error listing notes:", err)
		http.Error(w, "Error listing notes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	notes := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			log.Println("Error scanning note:", err)
			http.Error(w, "Error listing notes", http.StatusInternalServerError)
			return
		}
		notes = append(notes, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": notes, "total": total})
}

// UpdateNoteRequest edits the words of a note. A field that is absent stays as it is. Where the
// note points, and its kind, do not change: a note is moved by writing another one.
type UpdateNoteRequest struct {
	Quote *string   `json:"quote"`
	Body  *string   `json:"body"`
	Tags  *[]string `json:"tags"`
}

// UpdateNote edits the quotation, text or tags of one of the caller's notes.
func (h *NotesHandler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var req UpdateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	var quote, body sql.NullString
	var tags []string
	if req.Quote != nil {
		q, err := cleanText(*req.Quote)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		quote = sql.NullString{String: clip(q, maxQuote), Valid: true}
	}
	if req.Body != nil {
		b, err := cleanText(*req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(b) > maxBody {
			http.Error(w, fmt.Sprintf("The note is longer than %d characters", maxBody), http.StatusBadRequest)
			return
		}
		body = sql.NullString{String: b, Valid: true}
	}
	if req.Tags != nil {
		if tags, err = cleanTags(*req.Tags); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// The database refuses an edit that would leave a note with nothing in it.
	var updated int
	err = h.DB.QueryRowContext(r.Context(), `
		UPDATE notes SET
			quote = CASE WHEN $2::boolean THEN NULLIF($3, '') ELSE quote END,
			body = CASE WHEN $4::boolean THEN $5 ELSE body END,
			tags = CASE WHEN $6::boolean THEN $7::text[] ELSE tags END,
			updated_at = now()
		WHERE id = $1 AND user_id = $8
		RETURNING id`,
		id, quote.Valid, quote.String, body.Valid, body.String, req.Tags != nil, pq.Array(tags), currentUserID(r)).Scan(&updated)
	var pqErr *pq.Error
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "Note not found", http.StatusNotFound)
	case errors.As(err, &pqErr) && pqErr.Code == "23514":
		http.Error(w, "A note cannot be left empty", http.StatusBadRequest)
	case err != nil:
		log.Println("Error updating note:", err)
		http.Error(w, "Error updating note", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

// DeleteNote removes a note owned by the current user.
func (h *NotesHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, currentUserID(r))
	if err != nil {
		log.Println("Error deleting note:", err)
		http.Error(w, "Error deleting note", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
