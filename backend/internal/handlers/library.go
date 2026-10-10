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
	"github.com/ocnaibill/codice/backend/internal/metaproviders"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/people"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// Work represents the structure sent to the frontend. In the catalog a work is
// shown through its primary edition and file; GET /works/{id} also lists every
// edition and file.
type Work struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	// Authors are the authors one by one (the id of the person and the name as the account is shown it), for the card to link each to
	// the page of the person; Author is the same names in one text.
	Authors  []WorkAuthor `json:"authors"`
	CoverURL string       `json:"coverUrl"`
	// Collapsed: this work stands for its whole series in a grid (#187): the work is the newest of the series, and the card is the series'.
	Collapsed       *SeriesCard   `json:"collapsed,omitempty"`
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
	// FormatCount is how many different formats those files have, whatever their number: a book with an EPUB
	// and a PDF has two, one with the EPUBs of two editions has one. The card says "2 formatos" instead of
	// naming only one of them.
	FormatCount int `json:"formatCount"`
	// Completions, only in the detail: how many times the caller finished it, and in what format.
	Completions *CompletionSummary `json:"completions,omitempty"`
	// ReadLater, only in the detail: the caller put it aside in their list "Ler depois" (DEC-152).
	ReadLater bool `json:"readLater"`
	// Rating, only in the detail: the stars the caller gave it, from 1 to 5, or 0 when they gave none (DEC-154).
	Rating int `json:"rating"`
}

// CompletionSummary is the history of finishing a work (DEC-080): "finished 2 times, 1 in EPUB and
// 1 in PDF". A file finished again after being reopened counts again.
type CompletionSummary struct {
	Total    int            `json:"total"`
	ByFormat map[string]int `json:"byFormat"`
}

// ContinueFile is the file the caller read most recently in a work, with their position in it.
type ContinueFile struct {
	FileID   int64  `json:"fileId"`
	Format   string `json:"format,omitempty"`
	Language string `json:"language,omitempty"`
	// Title is the title that owner or admin wrote for the edition of this file, which is the name of the work while it is the one
	// being read; empty when nobody wrote one.
	Title           string  `json:"title,omitempty"`
	URL             string  `json:"url,omitempty"`
	Position        string  `json:"position,omitempty"`
	PercentComplete float64 `json:"percentComplete"`
	Completed       bool    `json:"completed"`
	// Chapter, UnitIndex and UnitTotal say where that position is in words (DEC-148): the chapter's title, and which unit of how many (the
	// page of a PDF or a comic, the position of an EPUB). The reader that saved the position sent them, and a position saved without them has none.
	Chapter   string `json:"chapter,omitempty"`
	UnitIndex int    `json:"unitIndex,omitempty"`
	UnitTotal int    `json:"unitTotal,omitempty"`
	// RemainingSeconds, only in the detail, is how long is left of the file at the pace of the caller in it (DEC-155); absent until they have
	// read enough for the pace to be worth saying.
	RemainingSeconds int `json:"remainingSeconds,omitempty"`
}

const (
	// An estimate of the time left is said only after this much reading and this far through the file: before it the pace is a guess.
	remainingMinSeconds = 600
	remainingMinPercent = 5.0
)

// estimateRemaining is how many seconds are left of a file, at the pace the person has had in it: the time spent reading, over the part of the
// file it got them through. It says 0 when it cannot say: too little reading or too little of the file, or a file that is finished.
func estimateRemaining(readingSeconds int, percent float64) int {
	if readingSeconds < remainingMinSeconds || percent < remainingMinPercent || percent >= 100 {
		return 0
	}
	left := float64(readingSeconds) * (100 - percent) / percent
	if left > 3600*1000 { // more than a thousand hours is not an estimate of a book
		return 0
	}
	return int(left + 0.5)
}

// Edition is one publication of a work: its language, publisher and date, and
// the files (formats) available for it.
type Edition struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	// TitleSet: the title was written by owner or admin, and is the name of the work while this edition is the one read. Otherwise it is
	// what the file brought (its title, or its file name), which is only a name to be found by.
	TitleSet        bool       `json:"titleSet"`
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
	TextStatus string `json:"textStatus,omitempty"`
	// Protected: the file asks for a password to be opened (a PDF): the server could not read its text nor draw its cover, and the reader asks the person for the password.
	Protected    bool `json:"protected,omitempty"`
	TextSegments int  `json:"textSegments,omitempty"`
	// Started is true when the calling user has a saved position in this file, even if the
	// viewer could not say how far along it is (an EPUB has no fixed page count).
	Started bool `json:"started"`
	// NeedsOCR is set for a PDF with pages that carry no text (RF-019);
	// PagesWithoutText lists them, numbered from 1.
	NeedsOCR         bool  `json:"needsOcr,omitempty"`
	PagesWithoutText []int `json:"pagesWithoutText,omitempty"`
	// DeclaredMode is how a comic file says it is read: "rtl" or "webtoon" (#19). Empty when it says nothing.
	DeclaredMode string `json:"declaredMode,omitempty"`
	// OCR says what became of the pages of a scan that have no text layer (#24); absent when the file has none.
	OCR *OCRProgress `json:"ocr,omitempty"`
}

// OCRProgress is how far the reading of the pages without text has got: how many there are, how many were read (a blank
// page counts as read) and how many could not be, and whether the work is waiting for the engine ("queued") or being
// read ("reading").
type OCRProgress struct {
	Pages  int    `json:"pages"`
	Read   int    `json:"read"`
	Failed int    `json:"failed"`
	State  string `json:"state,omitempty"`
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

// cardColumnsFor are the columns of a Work as the catalog shows it, with the author in the order the caller
// prefers.
func cardColumnsFor(order string) string {
	return strings.Replace(strings.Replace(cardColumns, authorLabel, authorLabelFor(order), 1), "au.person_names", personNamesFor(order), 1)
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
	COALESCE(lastrp.language, ''), COALESCE(lastrp.title, ''), (wrs.work_id IS NOT NULL),
	COALESCE(lastrp.chapter, ''), COALESCE(lastrp.unit_index, 0), COALESCE(lastrp.unit_total, 0),
	(SELECT count(*) FROM files fc JOIN editions ec ON ec.id = fc.edition_id
	  WHERE ec.work_id = w.id AND fc.availability = 'available'),
	(SELECT count(DISTINCT lower(fc.format)) FROM files fc JOIN editions ec ON ec.id = fc.edition_id
	  WHERE ec.work_id = w.id AND fc.availability = 'available'),
	COALESCE(au.ids, '{}'), COALESCE(au.person_names, '{}')`

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
		SELECT r.file_id, r.position, r.percent_complete, r.completed_at, r.chapter, r.unit_index, r.unit_total, f2.format, e2.language, l.path, l.mode,
		       CASE WHEN e2.title_manual THEN e2.title END AS title
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
	var authorIDs pq.Int64Array
	var authorNames pq.StringArray
	err := row.Scan(
		&work.ID, &work.Title, &work.Author, &work.CoverURL, &filePath, &work.Format,
		&work.Series, &work.SeriesIndex, &work.MediaStatus, pq.Array(&work.Tags),
		&work.ReadingProgress, &work.PercentComplete, &work.Completed, &work.IsFavorite,
		&fileID, &work.Retired, &mode,
		&lastFile, &last.Format, &lastPath, &lastMode, &last.Position, &last.PercentComplete, &last.Completed,
		&last.Language, &last.Title, &finished, &last.Chapter, &last.UnitIndex, &last.UnitTotal, &work.FileCount, &work.FormatCount, &authorIDs, &authorNames,
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
	work.Authors = workAuthorsOf(authorIDs, authorNames)
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
	formatGroup := r.URL.Query().Get("formatGroup")             // "ebooks" | "comics" | "mangas" | "audio"
	collapseSeries := r.URL.Query().Get("series") == "collapse" // the grid of the library, which shows a series as one card (#187)
	person, role := r.URL.Query().Get("person"), r.URL.Query().Get("role")
	category := r.URL.Query().Get("category") // the works in a category, and in the ones under it (DEC-140)
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
		whereClauses = append(whereClauses, "("+titleMatches(placeholder)+" OR "+authorMatches(placeholder)+")")
		args = append(args, catalogSearchPattern(search))
		argIdx++
	}
	if person != "" || role != "" {
		// The works a person is credited on, with the role if one is asked (#186). What is not a person or a role finds nothing,
		// rather than everything.
		id, err := strconv.Atoi(person)
		if (person != "" && (err != nil || id <= 0)) || (role != "" && !contributorRoles[role]) {
			whereClauses = append(whereClauses, "FALSE")
		} else {
			cond := "EXISTS (SELECT 1 FROM work_contributors wc WHERE wc.work_id = w.id"
			if person != "" {
				args = append(args, id)
				cond += fmt.Sprintf(" AND wc.person_id = $%d", argIdx)
				argIdx++
			}
			if role != "" {
				args = append(args, role)
				cond += fmt.Sprintf(" AND wc.role = $%d", argIdx)
				argIdx++
			}
			whereClauses = append(whereClauses, cond+")")
		}
	}
	if category != "" {
		// What is not a category finds nothing, rather than everything (a number that is none finds no work).
		id, err := strconv.ParseInt(category, 10, 64)
		if err != nil {
			whereClauses = append(whereClauses, "FALSE")
		} else {
			args = append(args, id)
			whereClauses = append(whereClauses, fmt.Sprintf(`w.id IN (SELECT wc.work_id FROM work_categories wc WHERE wc.category_id IN (
				WITH RECURSIVE down AS (SELECT id FROM categories WHERE id = $%d UNION ALL SELECT c.id FROM categories c JOIN down d ON c.parent_id = d.id)
				SELECT id FROM down))`, argIdx))
			argIdx++
		}
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
	if cond := shelfCondition(formatGroup, "wp.file_format"); cond != "" {
		whereClauses = append(whereClauses, cond)
	}
	// A series is one card only in the plain grid: a search, a person, a category, the reading in progress, the favorites and the trash
	// show the works, each by itself. With no work that has a unit there is nothing to put together.
	collapseSeries = collapseSeries && search == "" && person == "" && role == "" && category == "" && !inProgressOnly && !favoriteOnly && !retiredOnly && hasSeriesWorks(h.DB)
	if collapseSeries {
		whereClauses = append(whereClauses, seriesRepresentative(formatGroup))
	}
	// The page is chosen before the cards are built: counting and ordering run on the cheapest FROM the filters allow
	// (often only `works`), and only the works of the page get their card (author, progress, counts, tags). Building
	// the card first made every request pay for all the works before the offset: with 10 000 works the last page
	// took half a second (docs/Codice_Teste_de_Escala_2026-10-04.md).
	sortKey := r.URL.Query().Get("sort")
	pageFrom := " FROM works w"
	if catalogNeedsJoins(sortKey, inProgressOnly, favoriteOnly, formatGroup) {
		pageFrom = catalogFrom + cardJoins
	} else {
		// $1 is the caller's id; without the joins nothing else would mention it, and the server refuses a
		// parameter whose type it cannot tell.
		whereClauses = append(whereClauses, "$1::text IS NOT NULL")
	}
	if collapseSeries {
		pageFrom += seriesLateral
	}
	whereSQL := " WHERE " + strings.Join(whereClauses, " AND ")

	var totalCount int
	if err := h.DB.QueryRow("SELECT COUNT(*)"+pageFrom+whereSQL, args...).Scan(&totalCount); err != nil {
		log.Println("Error counting works:", err)
		http.Error(w, "Error counting works", http.StatusInternalServerError)
		return
	}

	order := people.OrderFor(r.Context(), h.DB, userID).Effective
	pageArgs := append(append([]interface{}{}, args...), limit, offset)
	idColumns, titleExpr := "w.id, 0", "w.original_title"
	if collapseSeries {
		idColumns, titleExpr = "w.id, COALESCE(sc.collection_id, 0)", "COALESCE(sc.name, w.original_title)"
	}
	idRows, err := h.DB.Query("SELECT "+idColumns+pageFrom+whereSQL+
		fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", catalogOrderByTitled(sortKey, order, titleExpr), argIdx, argIdx+1), pageArgs...)
	if err != nil {
		log.Println("Error choosing the page of works:", err)
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	var pageIDs []int
	seriesOf := map[int]int64{} // the work of the page that stands for a series -> the collection
	for idRows.Next() {
		var id int
		var series int64
		if err := idRows.Scan(&id, &series); err != nil {
			idRows.Close()
			log.Println("Error scanning the page of works:", err)
			http.Error(w, "Error fetching works", http.StatusInternalServerError)
			return
		}
		pageIDs = append(pageIDs, id)
		if series != 0 {
			seriesOf[id] = series
		}
	}
	idRows.Close()
	if err := idRows.Err(); err != nil {
		log.Println("Error choosing the page of works:", err)
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}

	works := []Work{}
	if len(pageIDs) > 0 {
		rows, err := h.DB.Query("SELECT "+cardColumnsFor(order)+catalogFrom+cardJoins+" WHERE w.id = ANY($2)", userID, pq.Array(pageIDs))
		if err != nil {
			log.Println("Error fetching works:", err)
			http.Error(w, "Error fetching works", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		byID := make(map[int]Work, len(pageIDs))
		for rows.Next() {
			work, err := scanWork(rows)
			if err != nil {
				log.Println("Error scanning work:", err)
				continue
			}
			byID[work.ID] = work
		}
		// The order is the one the page was chosen in, not whatever the second query happens to return.
		for _, id := range pageIDs {
			if work, ok := byID[id]; ok {
				works = append(works, work)
			}
		}
	}

	if len(seriesOf) > 0 {
		var collections []int64
		for _, id := range seriesOf {
			collections = append(collections, id)
		}
		cards, err := loadSeriesCards(h.DB, collections, userID)
		if err != nil {
			log.Println("Error reading the series of the page:", err)
			http.Error(w, "Error fetching works", http.StatusInternalServerError)
			return
		}
		for i := range works {
			if card := cards[seriesOf[works[i].ID]]; card != nil {
				works[i].Collapsed = card
				if card.CoverURL != "" {
					works[i].CoverURL = card.CoverURL
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":       works,
		"total":      totalCount,
		"series":     collapseSeries, // the total counts cards, a series being one
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

	order := people.OrderFor(r.Context(), h.DB, userID).Effective
	work, err := scanWork(h.DB.QueryRow("SELECT "+cardColumnsFor(order)+catalogFrom+cardJoins+" WHERE w.id = $2", userID, id))
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

	if c := work.Continue; c != nil && !c.Completed {
		var seconds int
		var percent float64
		err := h.DB.QueryRowContext(r.Context(),
			`SELECT reading_seconds, percent_complete FROM reading_progress WHERE user_id = $1::uuid AND file_id = $2`, userID, c.FileID).Scan(&seconds, &percent)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Println("Error fetching the pace:", err)
			http.Error(w, "Error fetching book", http.StatusInternalServerError)
			return
		}
		c.RemainingSeconds = estimateRemaining(seconds, percent)
	}

	if work.Completions, err = h.loadCompletions(r, userID, id); err != nil {
		log.Println("Error fetching completions:", err)
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM collection_works cw JOIN collections c ON c.id = cw.collection_id
		               WHERE c.owner_id = $1::uuid AND c.system_key = $2 AND cw.work_id = $3)`,
		userID, readLaterKey, id).Scan(&work.ReadLater); err != nil {
		log.Println("Error fetching the read-later mark:", err)
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	if err := h.DB.QueryRowContext(r.Context(), `SELECT COALESCE((SELECT stars FROM work_ratings WHERE user_id = $1::uuid AND work_id = $2), 0)`, userID, id).Scan(&work.Rating); err != nil {
		log.Println("Error fetching the rating:", err)
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	meta, err := loadMetadata(r.Context(), h.DB, id, order)
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
		SELECT e.id, COALESCE(e.title, ''), e.title_manual, COALESCE(e.language, ''), COALESCE(e.publisher, ''),
		       COALESCE(e.publication_date, ''), COALESCE(e.isbn, ''), e.is_primary,
		       f.id, COALESCE(f.format, ''), f.size_bytes, f.availability, l.path, l.mode,
		       COALESCE(rp.percent_complete, 0), (rp.completed_at IS NOT NULL), COALESCE(`+hasPosition("rp")+`, FALSE),
		       COALESCE(tl.needs_ocr, FALSE), tl.pages_without_text,
		       COALESCE(tx.status, ''), COALESCE(tx.segment_count, 0), COALESCE(f.declared_mode, ''),
		       cardinality(tl.pages_without_text), oc.read, oc.failed,
		       (SELECT CASE j.state WHEN 'running' THEN 'reading' ELSE 'queued' END FROM jobs j
		        WHERE j.type = 'ocr' AND j.work_id = e.work_id AND j.state IN ('pending', 'running') ORDER BY j.id LIMIT 1),
		       COALESCE(f.protected, FALSE)
		FROM editions e
		LEFT JOIN files f ON f.edition_id = e.id
		LEFT JOIN text_layers tl ON tl.file_id = f.id
		LEFT JOIN text_extractions tx ON tx.file_id = f.id
		LEFT JOIN LATERAL (
			SELECT count(*) FILTER (WHERE p.state IN ('done', 'blank')) AS read, count(*) FILTER (WHERE p.state = 'failed') AS failed
			FROM ocr_pages p WHERE p.file_id = f.id AND p.source_sha256 IS NOT DISTINCT FROM f.sha256
			  AND p.page + 1 = ANY (tl.pages_without_text)
		) oc ON tl.needs_ocr
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
		var declaredMode string
		var ocrPages, ocrRead, ocrFailed sql.NullInt64
		var ocrState sql.NullString
		var protected bool
		if err := rows.Scan(&e.ID, &e.Title, &e.TitleSet, &e.Language, &e.Publisher, &e.PublicationDate, &e.ISBN, &e.IsPrimary,
			&fileID, &format, &size, &availability, &filePath, &mode, &percent, &completed, &started, &needsOCR, &missing, &textStatus, &textSegments, &declaredMode, &ocrPages, &ocrRead, &ocrFailed, &ocrState, &protected); err != nil {
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
				TextStatus: textStatus, TextSegments: textSegments, DeclaredMode: declaredMode, Protected: protected}
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
			if needsOCR && ocrPages.Valid {
				fi.OCR = &OCRProgress{Pages: int(ocrPages.Int64), Read: int(ocrRead.Int64), Failed: int(ocrFailed.Int64), State: ocrState.String}
			}
			editions[i].Files = append(editions[i].Files, fi)
		}
	}
	return editions, rows.Err()
}

// UpdateWorkRequest is the payload for editing a work. The title is always
// sent. Every other field is optional: a field that is absent is left as it
// is, while an empty string clears it. The author is the first author's name:
// absent leaves the authors as they are (what is shown for a work with several
// authors is not a name). The *_lock flags protect (or release) a field against
// automatic changes.
type UpdateWorkRequest struct {
	Title           string   `json:"title"`
	Author          *string  `json:"author"`
	Tags            []string `json:"tags"`
	Series          *string  `json:"series"`
	SeriesIndex     *float64 `json:"series_index"`
	ISBN            *string  `json:"isbn"`
	Publisher       *string  `json:"publisher"`
	Language        *string  `json:"language"`
	PublicationDate *string  `json:"publication_date"`
	Description     *string  `json:"description"`
	// Unit and ComicKind (#187): "volume", "chapter" or "oneshot", and "comic" or "manga"; an empty one clears, an absent one leaves.
	Unit      *string `json:"unit"`
	ComicKind *string `json:"comic_kind"`

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

// validUnit and validComicKind say what the two fields of a comic or manga work (#187) can hold; empty is none.
func validUnit(s string) bool      { return s == "" || s == "volume" || s == "chapter" || s == "oneshot" }
func validComicKind(s string) bool { return s == "" || s == "comic" || s == "manga" }

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
	if req.Author != nil {
		next.Author = strings.TrimSpace(*req.Author)
		if next.Author == "" {
			next.Author = "Unknown Author"
		}
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
	if req.Unit != nil {
		next.Unit = strings.TrimSpace(*req.Unit)
		if !validUnit(next.Unit) {
			http.Error(w, "A unidade é volume, capítulo ou único.", http.StatusBadRequest)
			return
		}
	}
	if req.ComicKind != nil {
		next.ComicKind = strings.TrimSpace(*req.ComicKind)
		if !validComicKind(next.ComicKind) {
			http.Error(w, "O tipo é quadrinho ou mangá.", http.StatusBadRequest)
			return
		}
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
			chapter = NULL,
			unit_index = NULL,
			unit_total = NULL,
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

	// Nothing leaves the instance unless the owner turned a provider on (DEC-045): the worker checks it too.
	on, err := metaproviders.AnyEnabled(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the providers", http.StatusInternalServerError)
		return
	}
	if !on {
		writeJSON(w, http.StatusOK, map[string]any{"results": []ProviderResult{}, "query": query, "providersOff": true})
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
