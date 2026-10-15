package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/wiki"
)

// WikiHandler serves the automatic wiki (DEC-177 to DEC-183): every reader sees it up to their own point of reading, computed here
// from their reading progress and cut on the server (DEC-179); owner and admins import and remove it (DEC-180).
type WikiHandler struct {
	DB *sql.DB
}

const maxWikiBundle = 128 << 20

// ownCursor is how far the caller has read in the project's books: the last main-line book they have a place in, and the chapter
// of that place. The place is the one in the file the wiki was written from; in another file of the same work only "finished"
// (all of it) or the share read (the same share of the story's chapters) is known.
func (h *WikiHandler) ownCursor(ctx context.Context, p *wiki.Project, userID string) (wiki.Cursor, error) {
	cur := wiki.Nowhere
	for i := len(p.Books) - 1; i >= 0; i-- {
		b := p.Books[i]
		if b.Side {
			continue
		}
		chapter, ok, err := h.chapterRead(ctx, b, userID)
		if err != nil {
			return cur, err
		}
		if ok {
			return wiki.Cursor{Book: i, Chapter: chapter}, nil
		}
	}
	return cur, nil
}

func (h *WikiHandler) chapterRead(ctx context.Context, b wiki.BookRow, userID string) (int, bool, error) {
	last := wiki.LastStoryChapter(b.Chapters)
	if b.FileID != nil {
		var loc []byte
		var completed sql.NullTime
		var percent float64
		err := h.DB.QueryRowContext(ctx, `SELECT locator, completed_at, percent_complete FROM reading_progress WHERE user_id = $1 AND file_id = $2`,
			userID, *b.FileID).Scan(&loc, &completed, &percent)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return 0, false, err
		case completed.Valid:
			return last, true, nil
		default:
			if len(loc) > 0 {
				file, err := publishedText(ctx, h.DB, *b.FileID)
				if err != nil {
					return 0, false, err
				}
				if seg := nearestSegment(file.Segments, loc); seg != nil {
					return wiki.ChapterAt(b.Chapters, seg.Sequence), true, nil
				}
			}
			if percent > 0 {
				return shareOf(b.Chapters, percent), true, nil
			}
		}
	}
	// Another file of the same work (another edition, another format).
	var completed bool
	var percent float64
	err := h.DB.QueryRowContext(ctx, `
		SELECT bool_or(rp.completed_at IS NOT NULL), COALESCE(max(rp.percent_complete), 0)
		FROM reading_progress rp JOIN files f ON f.id = rp.file_id JOIN editions e ON e.id = f.edition_id
		WHERE rp.user_id = $1 AND e.work_id = $2 HAVING count(*) > 0`, userID, b.WorkID).Scan(&completed, &percent)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if completed {
		return last, true, nil
	}
	if percent > 0 {
		return shareOf(b.Chapters, percent), true, nil
	}
	return 0, false, nil
}

// shareOf is the story chapter a share of the book (0..1, or 0..100) has reached, rounding down: a share never shows a chapter
// the reader may not have finished.
func shareOf(chapters []wiki.ChapterRow, percent float64) int {
	if percent > 1 {
		percent /= 100
	}
	var story []int
	for _, c := range chapters {
		if !c.Editorial {
			story = append(story, c.Chapter)
		}
	}
	n := int(math.Floor(percent * float64(len(story))))
	if n <= 0 {
		return 0
	}
	if n > len(story) {
		n = len(story)
	}
	return story[n-1]
}

type cursorView struct {
	wiki.Cursor
	Label  string `json:"label"`
	Source string `json:"source"` // progress, manual or none
}

// cursor is the caller's own cursor, or the one they chose (?book=&chapter=), which may go beyond what they read (DEC-179) and
// changes nothing in their progress. ?side=1 shows the side books.
func (h *WikiHandler) cursor(r *http.Request, p *wiki.Project) (cursorView, wiki.Cursor, error) {
	own, err := h.ownCursor(r.Context(), p, currentUserID(r))
	if err != nil {
		return cursorView{}, own, err
	}
	q := r.URL.Query()
	side := q.Get("side") == "1" || q.Get("side") == "true"
	own.Side = side
	c := own
	source := "progress"
	if own.Book < 0 {
		source = "none"
	}
	if bs, cs := q.Get("book"), q.Get("chapter"); bs != "" && cs != "" {
		book, err1 := strconv.Atoi(bs)
		chapter, err2 := strconv.Atoi(cs)
		if err1 == nil && err2 == nil && book >= 0 && book < len(p.Books) && !p.Books[book].Side && chapter >= 0 {
			c = wiki.Cursor{Book: book, Chapter: chapter, Side: side}
			source = "manual"
		}
	}
	label := ""
	if c.Book >= 0 {
		label = p.Label(c.Book, c.Chapter)
	}
	return cursorView{Cursor: c, Label: label, Source: source}, own, nil
}

func (h *WikiHandler) project(w http.ResponseWriter, r *http.Request, id int64) *wiki.Project {
	p, err := wiki.LoadProject(r.Context(), h.DB, id)
	if errors.Is(err, wiki.ErrNotFound) {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return nil
	}
	if err != nil {
		log.Println("Error reading a wiki:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return nil
	}
	return p
}

// ForWork is the wiki of a work (its own or its series'): who wrote it, the books and their chapters for the cursor control,
// the caller's cursor and the index of the entities that appeared up to it. A work with no wiki answers {"wiki": null}.
func (h *WikiHandler) ForWork(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	id, err := wiki.ProjectFor(r.Context(), h.DB, int64(workID))
	if errors.Is(err, wiki.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"wiki": nil})
		return
	}
	if err != nil {
		log.Println("Error finding a wiki:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	p := h.project(w, r, id)
	if p == nil {
		return
	}
	cv, own, err := h.cursor(r, p)
	if err != nil {
		log.Println("Error computing a wiki cursor:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	index, err := p.Index(r.Context(), h.DB, cv.Cursor)
	if err != nil {
		log.Println("Error reading a wiki index:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	ownView := cursorView{Cursor: own, Source: "progress"}
	if own.Book >= 0 {
		ownView.Label = p.Label(own.Book, own.Chapter)
	} else {
		ownView.Source = "none"
	}
	writeJSON(w, http.StatusOK, map[string]any{"wiki": map[string]any{
		"project": p, "books": p.BooksFor(own), "cursor": cv, "own": ownView, "index": index,
		"hasSide": hasSide(p),
	}})
}

func hasSide(p *wiki.Project) bool {
	for _, b := range p.Books {
		if b.Side {
			return true
		}
	}
	return false
}

func projectParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// Entity is the page of an entity up to the cursor. An entity that has not appeared yet for this reader is not found.
func (h *WikiHandler) Entity(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	if !ok {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	p := h.project(w, r, id)
	if p == nil {
		return
	}
	cv, _, err := h.cursor(r, p)
	if err != nil {
		log.Println("Error computing a wiki cursor:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	page, err := p.Page(r.Context(), h.DB, chi.URLParam(r, "slug"), cv.Cursor, isStaffRequest(r))
	if errors.Is(err, wiki.ErrNotFound) {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading a wiki page:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cv, "page": page})
}

// Graph is the graph of the wiki up to the cursor.
func (h *WikiHandler) Graph(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	if !ok {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	p := h.project(w, r, id)
	if p == nil {
		return
	}
	cv, _, err := h.cursor(r, p)
	if err != nil {
		log.Println("Error computing a wiki cursor:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	nodes, edges, err := p.Graph(r.Context(), h.DB, cv.Cursor)
	if err != nil {
		log.Println("Error reading a wiki graph:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cv, "nodes": nodes, "edges": edges})
}

// Import stores a bundle as the wiki of its work or series (owner and admin). The bundle is a file of a multipart form (field
// "bundle"): a series makes a bundle of several megabytes, past the limit of a JSON body.
func (h *WikiHandler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxWikiBundle)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Envie o pacote da wiki como arquivo (campo \"bundle\").", http.StatusBadRequest)
		return
	}
	var b wiki.Bundle
	found := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "Não consegui ler o envio: "+err.Error(), http.StatusBadRequest)
			return
		}
		if part.FormName() != "bundle" {
			part.Close()
			continue
		}
		err = json.NewDecoder(part).Decode(&b)
		part.Close()
		if err != nil {
			http.Error(w, "O pacote da wiki não é um JSON válido: "+err.Error(), http.StatusBadRequest)
			return
		}
		found = true
		break
	}
	if !found {
		http.Error(w, "Envie o pacote da wiki como arquivo (campo \"bundle\").", http.StatusBadRequest)
		return
	}
	id, err := wiki.Import(r.Context(), h.DB, &b, currentUserID(r))
	var bad wiki.InputError
	if errors.As(err, &bad) {
		http.Error(w, bad.Msg, http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		log.Println("Error importing a wiki:", err)
		http.Error(w, "Error importing the wiki", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"projectId": id})
}

// Delete removes a wiki (owner and admin).
func (h *WikiHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	if !ok {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	err := wiki.Delete(r.Context(), h.DB, id, currentUserID(r))
	if errors.Is(err, wiki.ErrNotFound) {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error deleting a wiki:", err)
		http.Error(w, "Error deleting the wiki", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Review is what an update changed, for owner and admins (DEC-180): the entities that came and the sections that were added. It is not
// in the summary every reader gets, since the names of what a later book brings are spoilers.
func (h *WikiHandler) Review(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	if !ok {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	rev, err := wiki.LoadReview(r.Context(), h.DB, id)
	switch {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Wiki not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error reading a wiki review:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, rev)
	}
}

// Approve closes the review of an update: it was seen.
func (h *WikiHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	if !ok {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	switch err := wiki.Approve(r.Context(), h.DB, id, currentUserID(r)); {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Wiki not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error approving a wiki update:", err)
		http.Error(w, "Error saving the review", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// UndoChange hides what one entity gained in the update.
func (h *WikiHandler) UndoChange(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	var req struct {
		Slug string `json:"slug"`
	}
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req) != nil || req.Slug == "" {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	n, err := wiki.Undo(r.Context(), h.DB, id, req.Slug, currentUserID(r))
	switch {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Wiki not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error undoing a wiki change:", err)
		http.Error(w, "Error saving the review", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"hidden": n})
	}
}

// SetSentenceHidden marks a sentence as wrong, or shows it again.
func (h *WikiHandler) SetSentenceHidden(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	sid, err := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	var req struct {
		Hidden bool `json:"hidden"`
	}
	if !ok || err != nil || sid <= 0 || json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&req) != nil {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}
	switch err := wiki.SetSentenceHidden(r.Context(), h.DB, id, sid, req.Hidden, currentUserID(r)); {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Entity not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error hiding a wiki sentence:", err)
		http.Error(w, "Error saving the correction", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// Identity applies a decision about who is who (DEC-181): join two entities, keep them apart, or take a mention away from one.
func (h *WikiHandler) Identity(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	var d wiki.Decision
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&d) != nil {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	var bad wiki.ErrIdentity
	slug, err := wiki.Decide(r.Context(), h.DB, id, d, currentUserID(r))
	switch {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Entity not found", http.StatusNotFound)
	case errors.As(err, &bad):
		http.Error(w, bad.Msg, http.StatusBadRequest)
	case err != nil:
		log.Println("Error deciding a wiki identity:", err)
		http.Error(w, "Error saving the decision", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"slug": slug})
	}
}

// IdentityRules lists the decisions of the work or series.
func (h *WikiHandler) IdentityRules(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	if !ok {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	rules, err := wiki.Rules(r.Context(), h.DB, id)
	if err != nil {
		log.Println("Error listing wiki identity rules:", err)
		http.Error(w, "Error reading the wiki", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

// ForgetIdentityRule takes a decision back for the next generations.
func (h *WikiHandler) ForgetIdentityRule(w http.ResponseWriter, r *http.Request) {
	id, ok := projectParam(r)
	rule, err := strconv.ParseInt(chi.URLParam(r, "rule"), 10, 64)
	if !ok || err != nil || rule <= 0 {
		http.Error(w, "Wiki not found", http.StatusNotFound)
		return
	}
	switch err := wiki.ForgetRule(r.Context(), h.DB, id, rule, currentUserID(r)); {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Wiki not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error forgetting a wiki identity rule:", err)
		http.Error(w, "Error saving the decision", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
