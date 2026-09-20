package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// Work represents the structure sent to the frontend. In the catalog a work is
// shown through its primary edition and file; GET /works/{id} also lists every
// edition and file.
type Work struct {
	ID              int           `json:"id"`
	Title           string        `json:"title"`
	Author          string        `json:"author"`
	CoverURL        string        `json:"coverUrl"`
	FileURL         string        `json:"fileUrl,omitempty"`
	FileID          *int64        `json:"fileId,omitempty"`
	Format          string        `json:"format,omitempty"`
	Series          string        `json:"series,omitempty"`
	SeriesIndex     float64       `json:"seriesIndex,omitempty"`
	MediaStatus     string        `json:"mediaStatus,omitempty"`
	Tags            []string      `json:"tags"`
	ReadingProgress string        `json:"readingProgress,omitempty"`
	PercentComplete float64       `json:"percentComplete"`
	Completed       bool          `json:"completed"`
	IsFavorite      bool          `json:"isFavorite"`
	Retired         bool          `json:"retired,omitempty"`
	Editions        []Edition     `json:"editions,omitempty"`
	Metadata        *WorkMetadata `json:"metadata,omitempty"`
	// Continue is the version of the work that counts for the calling user (DEC-079): the one they
	// opened last among those they have begun, an unfinished one before a finished one. Its position
	// and percentage are also the ones the card shows. Null until they have read.
	Continue *ContinueFile `json:"continue"`
	// InProgress: there is a version to continue, and the work was not marked as finished.
	InProgress bool `json:"inProgress"`
	// Finished: the caller marked the whole work as finished (DEC-080).
	Finished bool `json:"finished"`
	// FileCount is how many files of the work can be opened: with more than one there is a choice.
	FileCount int `json:"fileCount"`
	// Completions, only in the detail: how many times the caller finished it, and in what format.
	Completions *CompletionSummary `json:"completions,omitempty"`
}

// CompletionSummary is the history of finishing a work (DEC-080): "finished 2 times, 1 in EPUB and
// 1 in PDF". A file finished again after being reopened counts again.
type CompletionSummary struct {
	Total    int            `json:"total"`
	ByFormat map[string]int `json:"byFormat"`
}

// ContinueFile is the file the caller read most recently in a work, with their position in it.
type ContinueFile struct {
	FileID          int64   `json:"fileId"`
	Format          string  `json:"format,omitempty"`
	Language        string  `json:"language,omitempty"`
	URL             string  `json:"url,omitempty"`
	Position        string  `json:"position,omitempty"`
	PercentComplete float64 `json:"percentComplete"`
	Completed       bool    `json:"completed"`
}

// Edition is one publication of a work: its language, publisher and date, and
// the files (formats) available for it.
type Edition struct {
	ID              int        `json:"id"`
	Title           string     `json:"title"`
	Language        string     `json:"language,omitempty"`
	Publisher       string     `json:"publisher,omitempty"`
	PublicationDate string     `json:"publicationDate,omitempty"`
	ISBN            string     `json:"isbn,omitempty"`
	IsPrimary       bool       `json:"isPrimary"`
	Files           []FileInfo `json:"files"`
}

// FileInfo is one digital manifestation of an edition. Progress is the calling
// user's, per file: the EPUB and the PDF of a book keep separate positions.
type FileInfo struct {
	ID              int64   `json:"id"`
	Format          string  `json:"format,omitempty"`
	SizeBytes       *int64  `json:"sizeBytes,omitempty"`
	Availability    string  `json:"availability"`
	URL             string  `json:"url,omitempty"`
	PercentComplete float64 `json:"percentComplete"`
	Completed       bool    `json:"completed"`
	// TextStatus says what became of reading the file's text: ready (there is text, TextSegments
	// passages of it), empty (nothing to read: a scan), unsupported (this kind of file has no text) or
	// failed. Empty until the text has been looked at.
	TextStatus   string `json:"textStatus,omitempty"`
	TextSegments int    `json:"textSegments,omitempty"`
	// Started is true when the calling user has a saved position in this file, even if the
	// viewer could not say how far along it is (an EPUB has no fixed page count).
	Started bool `json:"started"`
	// NeedsOCR is set for a PDF with pages that carry no text (RF-019);
	// PagesWithoutText lists them, numbered from 1.
	NeedsOCR         bool  `json:"needsOcr,omitempty"`
	PagesWithoutText []int `json:"pagesWithoutText,omitempty"`
}

// LibraryHandler stores the database connection
type LibraryHandler struct {
	DB    *sql.DB
	Trash *storage.Trash // optional: built from the storage path when nil
}

// currentUserID extracts the authenticated user id from context. AuthMiddleware
// guarantees it on protected routes; if it is ever missing this returns "" and
// never a substitute identity, so the query matches nothing instead of acting
// as another user.
func currentUserID(r *http.Request) string {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	return userID
}

// workIDParam reads the {id} URL parameter as a work id.
func workIDParam(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	return id, err == nil && id > 0
}

// cardColumns are the columns of a Work as the catalog shows it. $1 is the
// calling user, for progress and favorites.
const cardColumns = `
	w.id,
	w.original_title,
	` + authorLabel + `,
	COALESCE(wp.cover_url, ''),
	wp.file_path,
	COALESCE(wp.file_format, ''),
	COALESCE(w.series, ''),
	COALESCE(w.series_index, 0),
	COALESCE(w.media_status, 'READY'),
	COALESCE((SELECT array_agg(t.name ORDER BY t.name) FROM work_tags wt JOIN tags t ON t.id = wt.tag_id WHERE wt.work_id = w.id), '{}'),
	COALESCE(rp.position, ''),
	COALESCE(rp.percent_complete, 0),
	(rp.completed_at IS NOT NULL),
	(f.user_id IS NOT NULL),
	wp.file_id,
	(w.retired_at IS NOT NULL),
	COALESCE(wp.file_mode, ''),
	lastrp.file_id, COALESCE(lastrp.format, ''), lastrp.path, COALESCE(lastrp.mode, ''),
	COALESCE(lastrp.position, ''), COALESCE(lastrp.percent_complete, 0), (lastrp.completed_at IS NOT NULL),
	COALESCE(lastrp.language, ''), (wrs.work_id IS NOT NULL),
	(SELECT count(*) FROM files fc JOIN editions ec ON ec.id = fc.edition_id
	  WHERE ec.work_id = w.id AND fc.availability = 'available')`

// hasPosition is the condition for a reading_progress row (alias a) that says where the person
// is, or that they finished. A row can exist for less: counting seconds of reading creates one
// with no position, and opening a file is not the same as having begun it.
func hasPosition(a string) string {
	return "(" + a + ".position <> '' OR " + a + ".locator IS NOT NULL OR " + a + ".percent_complete > 0 OR " + a + ".completed_at IS NOT NULL)"
}

// cardJoins add the calling user's progress on the primary file and favorite flag.
var cardJoins = `
	LEFT JOIN reading_progress rp ON rp.file_id = wp.file_id AND rp.user_id = $1
	LEFT JOIN favorites f ON f.work_id = w.id AND f.user_id = $1` + lastReadJoin

// lastReadJoin adds, for the work w ($1 is the caller): wrs, the mark "the whole work is finished",
// and lastrp, the version that counts (DEC-079). A version counts only after the person has begun it
// (a position, a percentage or a completion: opening is not beginning) and only if it can still be
// opened. Of those, one that is not finished comes before one that is, and then the one opened last
// wins; the finished ones are what is left when nothing is in progress.
var lastReadJoin = `
	LEFT JOIN work_reading_state wrs ON wrs.work_id = w.id AND wrs.user_id = $1
	LEFT JOIN LATERAL (
		SELECT r.file_id, r.position, r.percent_complete, r.completed_at, f2.format, e2.language, l.path, l.mode
		FROM reading_progress r
		JOIN files f2 ON f2.id = r.file_id
		JOIN editions e2 ON e2.id = f2.edition_id
		LEFT JOIN LATERAL (SELECT path, mode FROM storage_locations WHERE file_id = f2.id ORDER BY id LIMIT 1) l ON TRUE
		WHERE e2.work_id = w.id AND r.user_id = $1 AND f2.availability = 'available'
		  AND ` + hasPosition("r") + `
		ORDER BY (r.completed_at IS NOT NULL),
		         GREATEST(COALESCE(r.last_opened_at, '-infinity'::timestamptz), r.updated_at) DESC, r.file_id
		LIMIT 1
	) lastrp ON TRUE`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWork(row rowScanner) (Work, error) {
	var work Work
	var filePath sql.NullString
	var fileID sql.NullInt64
	var mode string
	var lastFile sql.NullInt64
	var lastPath sql.NullString
	var last ContinueFile
	var lastMode string
	var finished bool
	err := row.Scan(
		&work.ID, &work.Title, &work.Author, &work.CoverURL, &filePath, &work.Format,
		&work.Series, &work.SeriesIndex, &work.MediaStatus, pq.Array(&work.Tags),
		&work.ReadingProgress, &work.PercentComplete, &work.Completed, &work.IsFavorite,
		&fileID, &work.Retired, &mode,
		&lastFile, &last.Format, &lastPath, &lastMode, &last.Position, &last.PercentComplete, &last.Completed,
		&last.Language, &finished, &work.FileCount,
	)
	if err != nil {
		return work, err
	}
	work.FileURL = fileHref(fileID, mode, filePath.String)
	if lastFile.Valid {
		last.FileID = lastFile.Int64
		last.URL = fileHref(lastFile, lastMode, lastPath.String)
		work.Continue = &last
		// The card speaks for the version that counts, not for the primary file (DEC-079).
		work.ReadingProgress, work.PercentComplete, work.Completed = last.Position, last.PercentComplete, last.Completed
	}
	work.Finished = finished
	work.InProgress = work.Continue != nil && !work.Continue.Completed && !finished
	if fileID.Valid {
		work.FileID = &fileID.Int64
	}
	if work.CoverURL == "" {
		work.CoverURL = "/covers/placeholder.svg"
	}
	if work.Tags == nil {
		work.Tags = []string{}
	}
	return work, nil
}

// GetWorks fetches works from PostgreSQL with server-side pagination, search,
// and optional filters: inProgress=true (has unfinished reading progress) and
// favorite=true (marked as favorite by the caller). Retired works are hidden;
// owner and admin can list them with retired=true.
func (h *LibraryHandler) GetWorks(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)

	page := 1
	limit := 50
	search := r.URL.Query().Get("search")
	inProgressOnly := r.URL.Query().Get("inProgress") == "true"
	favoriteOnly := r.URL.Query().Get("favorite") == "true"
	formatGroup := r.URL.Query().Get("formatGroup") // "ebooks" | "comics" | "audio"
	retiredOnly := isStaffRequest(r) && r.URL.Query().Get("retired") == "true"

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	offset := (page - 1) * limit

	var whereClauses []string
	var args []interface{}
	args = append(args, userID) // $1 always the current user, used by the joins
	argIdx := 2

	if retiredOnly {
		whereClauses = append(whereClauses, "w.retired_at IS NOT NULL")
	} else {
		whereClauses = append(whereClauses, "w.retired_at IS NULL")
	}
	if search != "" {
		placeholder := fmt.Sprintf("$%d", argIdx)
		whereClauses = append(whereClauses, "(LOWER(w.original_title) LIKE LOWER("+placeholder+") OR "+authorMatches(placeholder)+")")
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if inProgressOnly {
		// In progress is decided by the version that counts, in whatever edition or format: reading
		// the English EPUB of a book whose primary file is the Portuguese one counts, and a work
		// marked as finished is not.
		whereClauses = append(whereClauses, "(lastrp.file_id IS NOT NULL AND lastrp.completed_at IS NULL AND wrs.work_id IS NULL)")
	}
	if favoriteOnly {
		whereClauses = append(whereClauses, "f.user_id IS NOT NULL")
	}
	switch formatGroup {
	case "ebooks":
		whereClauses = append(whereClauses, "LOWER(wp.file_format) IN "+bookFormats)
	case "comics":
		whereClauses = append(whereClauses, "LOWER(wp.file_format) IN "+comicFormats)
	case "audio":
		whereClauses = append(whereClauses, "LOWER(wp.file_format) IN "+audioFormats)
	}
	whereSQL := " WHERE " + strings.Join(whereClauses, " AND ")

	var totalCount int
	if err := h.DB.QueryRow("SELECT COUNT(*) "+catalogFrom+cardJoins+whereSQL, args...).Scan(&totalCount); err != nil {
		log.Println("Error counting works:", err)
		http.Error(w, "Error counting works", http.StatusInternalServerError)
		return
	}

	query := "SELECT " + cardColumns + catalogFrom + cardJoins + whereSQL +
		fmt.Sprintf(" ORDER BY w.id DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		log.Println("Error fetching works:", err)
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	works := []Work{}
	for rows.Next() {
		work, err := scanWork(rows)
		if err != nil {
			log.Println("Error scanning work:", err)
			continue
		}
		works = append(works, work)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":       works,
		"total":      totalCount,
		"page":       page,
		"limit":      limit,
		"totalPages": (totalCount + limit - 1) / limit,
	})
}

// GetWorkByID fetches a single work with all its editions and files, and the
// caller's progress on each file. A retired work is visible to owner and admin only.
func (h *LibraryHandler) GetWorkByID(w http.ResponseWriter, r *http.Request) {
	id, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)

	work, err := scanWork(h.DB.QueryRow("SELECT "+cardColumns+catalogFrom+cardJoins+" WHERE w.id = $2", userID, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}
	if work.Retired && !isStaffRequest(r) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}

	editions, err := h.loadEditions(id, userID)
	if err != nil {
		log.Println("Error fetching editions:", err)
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}
	work.Editions = editions

	if work.Completions, err = h.loadCompletions(r, userID, id); err != nil {
		log.Println("Error fetching completions:", err)
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	meta, err := loadMetadata(r.Context(), h.DB, id)
	if err != nil {
		log.Println("Error fetching metadata:", err)
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}
	work.Metadata = meta

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(work)
}

// loadEditions returns every edition of a work with its files, primary first.
func (h *LibraryHandler) loadEditions(workID int, userID string) ([]Edition, error) {
	rows, err := h.DB.Query(`
		SELECT e.id, COALESCE(e.title, ''), COALESCE(e.language, ''), COALESCE(e.publisher, ''),
		       COALESCE(e.publication_date, ''), COALESCE(e.isbn, ''), e.is_primary,
		       f.id, COALESCE(f.format, ''), f.size_bytes, f.availability, l.path, l.mode,
		       COALESCE(rp.percent_complete, 0), (rp.completed_at IS NOT NULL), COALESCE(`+hasPosition("rp")+`, FALSE),
		       COALESCE(tl.needs_ocr, FALSE), tl.pages_without_text,
		       COALESCE(tx.status, ''), COALESCE(tx.segment_count, 0)
		FROM editions e
		LEFT JOIN files f ON f.edition_id = e.id
		LEFT JOIN text_layers tl ON tl.file_id = f.id
		LEFT JOIN text_extractions tx ON tx.file_id = f.id
		LEFT JOIN LATERAL (
			SELECT path, mode FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1
		) l ON TRUE
		LEFT JOIN reading_progress rp ON rp.file_id = f.id AND rp.user_id = $2
		WHERE e.work_id = $1
		ORDER BY e.is_primary DESC, e.id, f.id`, workID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	editions := []Edition{}
	index := map[int]int{}
	for rows.Next() {
		var e Edition
		var fileID sql.NullInt64
		var size sql.NullInt64
		var format, availability, filePath, mode sql.NullString
		var percent float64
		var completed, started sql.NullBool
		var needsOCR bool
		var missing pq.Int64Array
		var textStatus string
		var textSegments int
		if err := rows.Scan(&e.ID, &e.Title, &e.Language, &e.Publisher, &e.PublicationDate, &e.ISBN, &e.IsPrimary,
			&fileID, &format, &size, &availability, &filePath, &mode, &percent, &completed, &started, &needsOCR, &missing, &textStatus, &textSegments); err != nil {
			return nil, err
		}
		i, seen := index[e.ID]
		if !seen {
			e.Files = []FileInfo{}
			editions = append(editions, e)
			i = len(editions) - 1
			index[e.ID] = i
		}
		if fileID.Valid {
			fi := FileInfo{ID: fileID.Int64, Format: format.String, Availability: availability.String,
				PercentComplete: percent, Completed: completed.Bool, Started: started.Bool,
				TextStatus: textStatus, TextSegments: textSegments}
			if size.Valid {
				fi.SizeBytes = &size.Int64
			}
			fi.URL = fileHref(fileID, mode.String, filePath.String)
			if needsOCR {
				fi.NeedsOCR = true
				for _, n := range missing {
					fi.PagesWithoutText = append(fi.PagesWithoutText, int(n))
				}
			}
			editions[i].Files = append(editions[i].Files, fi)
		}
	}
	return editions, rows.Err()
}

// UpdateWorkRequest is the payload for editing a work. Title and author are
// always sent. Every other field is optional: a field that is absent is left as
// it is, while an empty string clears it. The *_lock flags protect (or release)
// a field against automatic changes.
type UpdateWorkRequest struct {
	Title           string   `json:"title"`
	Author          string   `json:"author"`
	Tags            []string `json:"tags"`
	Series          *string  `json:"series"`
	SeriesIndex     *float64 `json:"series_index"`
	ISBN            *string  `json:"isbn"`
	Publisher       *string  `json:"publisher"`
	Language        *string  `json:"language"`
	PublicationDate *string  `json:"publication_date"`
	Description     *string  `json:"description"`

	TitleLock           *bool `json:"title_lock"`
	AuthorLock          *bool `json:"author_lock"`
	SeriesLock          *bool `json:"series_lock"`
	CoverLock           *bool `json:"cover_lock"`
	ISBNLock            *bool `json:"isbn_lock"`
	PublisherLock       *bool `json:"publisher_lock"`
	LanguageLock        *bool `json:"language_lock"`
	PublicationDateLock *bool `json:"publication_date_lock"`
	DescriptionLock     *bool `json:"description_lock"`
}

// UpdateWork edits a work's descriptive metadata and tags in one transaction. A
// field the admin actually changes becomes confirmed (RN-008): it is locked
// against automatic enrichment and its provenance is recorded as manual. The
// bibliographic reference kept on the user's notes follows a confirmed title or
// author while the work is available (DEC-040).
func (h *LibraryHandler) UpdateWork(w http.ResponseWriter, r *http.Request) {
	id, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}

	var req UpdateWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	cur, retired, err := readWorkFields(tx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error querying work", http.StatusInternalServerError)
		return
	}

	next := cur
	next.Title = req.Title
	next.Author = strings.TrimSpace(req.Author)
	if next.Author == "" {
		next.Author = "Unknown Author"
	}
	if req.Series != nil {
		next.Series = strings.TrimSpace(*req.Series)
	}
	if req.SeriesIndex != nil {
		next.SeriesIndex = *req.SeriesIndex
	}
	if req.ISBN != nil {
		next.ISBN = strings.TrimSpace(*req.ISBN)
	}
	if req.Publisher != nil {
		next.Publisher = strings.TrimSpace(*req.Publisher)
	}
	if req.Language != nil {
		next.Language = strings.TrimSpace(*req.Language)
	}
	if req.PublicationDate != nil {
		next.PublicationDate = strings.TrimSpace(*req.PublicationDate)
	}
	if req.Description != nil {
		next.Description = strings.TrimSpace(*req.Description)
	}

	locks := map[string]*bool{
		"title": req.TitleLock, "author": req.AuthorLock, "series": req.SeriesLock, "cover": req.CoverLock,
		"isbn": req.ISBNLock, "publisher": req.PublisherLock, "language": req.LanguageLock,
		"publication_date": req.PublicationDateLock, "description": req.DescriptionLock,
	}
	actor := currentUserID(r)
	changes, err := applyWorkFields(r.Context(), tx, id, actor, sourceManual, cur, retired, next, locks)
	if err != nil {
		log.Println("Error updating work:", err)
		http.Error(w, "Error updating work record", http.StatusInternalServerError)
		return
	}

	// Sync Tags (delete existing relations and re-insert new ones)
	if _, err = tx.Exec("DELETE FROM work_tags WHERE work_id = $1", id); err != nil {
		http.Error(w, "Error resetting work tags", http.StatusInternalServerError)
		return
	}
	if err := addTags(tx, id, req.Tags); err != nil {
		http.Error(w, "Error linking work tags", http.StatusInternalServerError)
		return
	}

	if len(changes) > 0 {
		if err := audit.Record(r.Context(), tx, actor, "work.update", "work", strconv.Itoa(id), auditDetails(changes)); err != nil {
			http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "Error committing database transaction", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ProgressRequest represents the payload for updating reading progress.
// Percent and Completed are pointers so we can tell "not sent" apart from
// "sent as zero/false" — a viewer that doesn't know percent yet (e.g. a
// text file with no pagination) shouldn't overwrite a previously saved one.
// FileID selects which file of the work the position belongs to; without it
// the work's primary file is used.
type ProgressRequest struct {
	Progress  string   `json:"progress"`
	Percent   *float64 `json:"percent,omitempty"`
	Completed *bool    `json:"completed,omitempty"`
	FileID    *int64   `json:"fileId,omitempty"`
}

var errNoReadableFile = errors.New("work has no readable file")

// resolveProgressFile returns the file a progress write applies to: the one
// named, if it belongs to the (available) work, otherwise the work's primary file.
func (h *LibraryHandler) resolveProgressFile(workID int, fileID *int64) (int64, error) {
	var id sql.NullInt64
	var err error
	if fileID != nil {
		err = h.DB.QueryRow(`
			SELECT f.id FROM files f
			JOIN editions e ON e.id = f.edition_id
			JOIN works w ON w.id = e.work_id
			WHERE f.id = $1 AND w.id = $2 AND w.retired_at IS NULL`, *fileID, workID).Scan(&id)
	} else {
		err = h.DB.QueryRow(`
			SELECT wp.file_id FROM works w JOIN work_primary wp ON wp.work_id = w.id
			WHERE w.id = $1 AND w.retired_at IS NULL`, workID).Scan(&id)
	}
	if err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, errNoReadableFile
	}
	return id.Int64, nil
}

func progressError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errNoReadableFile) {
		http.Error(w, "Book or file not found", http.StatusNotFound)
		return
	}
	log.Println("Error resolving file for progress:", err)
	http.Error(w, "Error saving reading progress", http.StatusInternalServerError)
}

// UpdateProgress updates the reading position (and optionally percent and
// completion) of one file of a work, isolated by user.
func (h *LibraryHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)

	var req ProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	fileID, err := h.resolveProgressFile(workID, req.FileID)
	if err != nil {
		progressError(w, err)
		return
	}

	percent := 0.0
	if req.Percent != nil {
		percent = *req.Percent
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
	}

	completed := req.Completed != nil && *req.Completed
	percentProvided := req.Percent != nil
	completedExplicitlyFalse := req.Completed != nil && !*req.Completed

	query := `
		INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, CASE WHEN $5 THEN now() ELSE NULL END, now())
		ON CONFLICT (user_id, file_id)
		DO UPDATE SET
			position = EXCLUDED.position,
			-- A plain-text position says nothing a saved locator could still be right about.
			locator = NULL,
			locator_version = NULL,
			percent_complete = CASE WHEN $6 THEN EXCLUDED.percent_complete ELSE reading_progress.percent_complete END,
			completed_at = CASE
				WHEN $5 THEN COALESCE(reading_progress.completed_at, now())
				WHEN $7 THEN NULL
				ELSE reading_progress.completed_at
			END,
			revision = reading_progress.revision + 1,
			updated_at = now();
	`
	if _, err := h.DB.Exec(query, userID, fileID, req.Progress, percent, completed, percentProvided, completedExplicitlyFalse); err != nil {
		log.Println("Error saving reading progress:", err)
		http.Error(w, "Error saving isolated user reading progress", http.StatusInternalServerError)
		return
	}
	if !completed {
		(&ProgressHandler{DB: h.DB}).reopenWork(r, userID, fileID)
	}

	w.WriteHeader(http.StatusOK)
}

// HeartbeatRequest reports how many seconds of active reading happened
// since the reader's last heartbeat tick.
type HeartbeatRequest struct {
	Seconds float64 `json:"seconds"`
	FileID  *int64  `json:"fileId,omitempty"`
}

// ReadingHeartbeat accumulates real reading time for a file. Seconds are
// clamped to a small window so a stalled tab or clock skew can't inflate
// the total — the reader is expected to call this roughly every 20-30s
// while the document is open and the tab is visible.
func (h *LibraryHandler) ReadingHeartbeat(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	seconds := int(req.Seconds)
	if seconds < 0 {
		seconds = 0
	}
	if seconds > 120 {
		seconds = 120
	}
	if seconds == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	fileID, err := h.resolveProgressFile(workID, req.FileID)
	if err != nil {
		progressError(w, err)
		return
	}

	query := `
		INSERT INTO reading_progress (user_id, file_id, position, reading_seconds, updated_at)
		VALUES ($1, $2, '', $3, now())
		ON CONFLICT (user_id, file_id)
		DO UPDATE SET reading_seconds = reading_progress.reading_seconds + $3, updated_at = now();
	`
	if _, err := h.DB.Exec(query, userID, fileID, seconds); err != nil {
		log.Println("Error saving reading heartbeat:", err)
		http.Error(w, "Error saving reading heartbeat", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// SearchMetadataResponse represents results from a provider metadata search
type SearchMetadataResponse struct {
	Results []ProviderResult `json:"results"`
	Query   string           `json:"query"`
}

// ProviderResult represents a single provider's metadata result
type ProviderResult struct {
	Source          string   `json:"source"`
	Title           string   `json:"title"`
	Author          string   `json:"author"`
	Series          string   `json:"series"`
	SeriesIndex     float64  `json:"series_index"`
	Isbn            string   `json:"isbn"`
	Language        string   `json:"language"`
	Publisher       string   `json:"publisher"`
	PublicationDate string   `json:"publication_date"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	CoverURL        string   `json:"cover_url"`
}

// SearchMetadata proxies a metadata search to the worker HTTP server
func (h *LibraryHandler) SearchMetadata(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	format := r.URL.Query().Get("format")

	if query == "" {
		http.Error(w, "Missing 'q' parameter", http.StatusBadRequest)
		return
	}

	// Proxy to worker search server
	workerURL := os.Getenv("WORKER_SEARCH_URL")
	if workerURL == "" {
		workerURL = "http://localhost:5000/search"
	}

	proxyURL := fmt.Sprintf("%s?q=%s", workerURL, url.QueryEscape(query))
	if format != "" {
		proxyURL += "&format=" + url.QueryEscape(format)
	}

	resp, err := http.Get(proxyURL)
	if err != nil {
		log.Printf("❌ Worker search proxy error: %v", err)
		http.Error(w, "Search service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error reading search response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// DeleteWork retires a work from the catalog (DEC-038): it disappears for
// readers but its files, notes and history stay, and it can be restored. With
// ?purge=true a work that is already retired is deleted for good, files included
// (an extra, explicit step). Notes survive both: they keep their bibliographic
// reference and show the source as unavailable.
func (h *LibraryHandler) DeleteWork(w http.ResponseWriter, r *http.Request) {
	id, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("purge") == "true" {
		h.purgeWork(w, r, id)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var title string
	var alreadyRetired bool
	err = tx.QueryRow(`SELECT original_title, retired_at IS NOT NULL FROM works WHERE id = $1 FOR UPDATE`, id).Scan(&title, &alreadyRetired)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	if !alreadyRetired {
		actor := currentUserID(r)
		if _, err := tx.Exec(`UPDATE works SET retired_at = now(), retired_by = NULLIF($2, '')::uuid WHERE id = $1`, id, actor); err != nil {
			http.Error(w, "Error retiring book", http.StatusInternalServerError)
			return
		}
		if err := audit.Record(r.Context(), tx, actor, "work.retire", "work", strconv.Itoa(id), map[string]any{"title": title}); err != nil {
			http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing retirement", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RestoreWork brings a retired work back to the catalog.
func (h *LibraryHandler) RestoreWork(w http.ResponseWriter, r *http.Request) {
	id, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var title string
	var retired bool
	err = tx.QueryRow(`SELECT original_title, retired_at IS NOT NULL FROM works WHERE id = $1 FOR UPDATE`, id).Scan(&title, &retired)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}
	if retired {
		if _, err := tx.Exec(`UPDATE works SET retired_at = NULL, retired_by = NULL WHERE id = $1`, id); err != nil {
			http.Error(w, "Error restoring book", http.StatusInternalServerError)
			return
		}
		if err := audit.Record(r.Context(), tx, currentUserID(r), "work.restore", "work", strconv.Itoa(id), map[string]any{"title": title}); err != nil {
			http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing restore", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// purgeWork is "delete for good" for a retired work. Its managed files go to the
// trash, where they can still be restored, and are destroyed only when the trash
// is emptied (or by the optional automatic cleanup). If the server stores none of
// the work's bytes the record is deleted at once. Files that are only referenced
// from elsewhere are never touched (RN-004). Notes survive either way.
func (h *LibraryHandler) purgeWork(w http.ResponseWriter, r *http.Request, id int) {
	var title string
	var retired bool
	err := h.DB.QueryRowContext(r.Context(), `SELECT original_title, retired_at IS NOT NULL FROM works WHERE id = $1`, id).Scan(&title, &retired)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}
	if !retired {
		http.Error(w, "Retire the book before deleting it permanently", http.StatusConflict)
		return
	}

	trash := h.Trash
	if trash == nil {
		trash = &storage.Trash{DB: h.DB, Root: resolveStoragePath()}
	}
	trashed, deleted, err := trash.PurgeWork(r.Context(), id, currentUserID(r))
	if err != nil {
		log.Println("Error deleting work:", err)
		http.Error(w, "Error deleting the book", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), h.DB, currentUserID(r), "work.purge", "work", strconv.Itoa(id),
		map[string]any{"title": title, "trashed": trashed, "deleted": deleted}); err != nil {
		log.Println("Could not audit the deletion:", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"trashed": trashed, "deleted": deleted})
}

// insideStorage joins a stored relative path to the storage root and refuses
// anything that would leave it.
func insideStorage(root, rel string) (string, bool) {
	full := filepath.Join(root, rel)
	return full, full != filepath.Clean(root) && isWithin(full, filepath.Clean(root))
}

// loadCompletions counts the times the caller finished a work, by format (DEC-080).
func (h *LibraryHandler) loadCompletions(r *http.Request, userID string, workID int) (*CompletionSummary, error) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT COALESCE(NULLIF(format, ''), '?'), count(*) FROM reading_completions
		WHERE user_id = $1 AND work_id = $2 GROUP BY 1`, userID, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sum := &CompletionSummary{ByFormat: map[string]int{}}
	for rows.Next() {
		var format string
		var n int
		if err := rows.Scan(&format, &n); err != nil {
			return nil, err
		}
		sum.ByFormat[format] = n
		sum.Total += n
	}
	return sum, rows.Err()
}
