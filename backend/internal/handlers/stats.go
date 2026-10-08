package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// StatsHandler stores the database connection
type StatsHandler struct {
	DB *sql.DB
}

// FormatBreakdown groups a count into the four shelves the dashboard displays: "livros" (epub/pdf/txt/md),
// "quadrinhos", "mangas" and "audio" (audio formats). A comic or a manga is a matter of the kind of the work (#187), not
// of the format: a work marked as a manga is on the manga shelf in any format, one marked as a comic is on the comic
// shelf, and one with no kind is a comic when its file is a CBZ or a CBR, so nothing a library already had goes missing.
type FormatBreakdown struct {
	Livros     int `json:"livros"`
	Quadrinhos int `json:"quadrinhos"`
	Mangas     int `json:"mangas"`
	Audio      int `json:"audio"`
}

// DashboardStats is the payload for GET /stats
type DashboardStats struct {
	WorksTotal          int             `json:"worksTotal"`
	CatalogedPercent    int             `json:"catalogedPercent"`
	LibraryBreakdown    FormatBreakdown `json:"libraryBreakdown"`
	InProgressCount     int             `json:"inProgressCount"`
	InProgressBreakdown FormatBreakdown `json:"inProgressBreakdown"`
	CompletedThisMonth  int             `json:"completedThisMonth"`
	CompletedBreakdown  FormatBreakdown `json:"completedBreakdown"`
	TotalReadingSeconds int             `json:"totalReadingSeconds"`
}

const bookFormats = "('epub','pdf','txt','md')"
const comicFormats = "('cbz','cbr')"
const audioFormats = "('mp3','m4a','m4b','ogg','wav','flac')"

// shelfCondition is the SQL condition that puts a work on a shelf ("ebooks", "comics", "mangas" or "audio"), given the
// expression of the format being counted; the work is `w`. A work with a kind is on the shelf of the kind and on no
// other; one with no kind is on the shelf of its format. An unknown shelf has no condition.
func shelfCondition(shelf, formatExpr string) string {
	return shelfConditionOf("w", shelf, formatExpr)
}

// shelfConditionOf is shelfCondition for a work that goes by another alias.
func shelfConditionOf(work, shelf, formatExpr string) string {
	format := "LOWER(" + formatExpr + ")"
	kind := work + ".comic_kind"
	switch shelf {
	case "ebooks":
		return "(" + kind + " IS NULL AND " + format + " IN " + bookFormats + ")"
	case "comics":
		return "(" + kind + " = 'comic' OR (" + kind + " IS NULL AND " + format + " IN " + comicFormats + "))"
	case "mangas":
		return kind + " = 'manga'"
	case "audio":
		return "(" + kind + " IS NULL AND " + format + " IN " + audioFormats + ")"
	}
	return ""
}

// scanFormatBreakdown counts available (not retired) works by the format of
// their primary file. joinSQL adds joins, such as the caller's progress.
func scanFormatBreakdown(db *sql.DB, joinSQL string, args ...interface{}) (FormatBreakdown, int, error) {
	return scanFormatBreakdownBy(db, "wp.file_format", joinSQL, "", args...)
}

// scanFormatBreakdownBy counts works by the format given by formatExpr, after joinSQL and
// only those that satisfy where (a condition, or empty).
func scanFormatBreakdownBy(db *sql.DB, formatExpr, joinSQL, where string, args ...interface{}) (FormatBreakdown, int, error) {
	var b FormatBreakdown
	if where != "" {
		where = " AND " + where
	}
	query := `
		SELECT
			COUNT(*) FILTER (WHERE ` + shelfCondition("ebooks", formatExpr) + `),
			COUNT(*) FILTER (WHERE ` + shelfCondition("comics", formatExpr) + `),
			COUNT(*) FILTER (WHERE ` + shelfCondition("mangas", formatExpr) + `),
			COUNT(*) FILTER (WHERE ` + shelfCondition("audio", formatExpr) + `),
			COUNT(*)
		FROM works w
		LEFT JOIN work_primary wp ON wp.work_id = w.id
		` + joinSQL + `
		WHERE w.retired_at IS NULL` + where
	var total int
	err := db.QueryRow(query, args...).Scan(&b.Livros, &b.Quadrinhos, &b.Mangas, &b.Audio, &total)
	return b, total, err
}

// GetStats aggregates library-wide and per-user reading stats for the
// dashboard: total preserved works, how many are mid-read, how many were
// finished this month, and real accumulated reading time from the reader's
// heartbeat pings.
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	var stats DashboardStats

	libraryBreakdown, worksTotal, err := scanFormatBreakdown(h.DB, "")
	if err != nil {
		log.Println("Error computing library breakdown:", err)
		http.Error(w, "Error computing stats", http.StatusInternalServerError)
		return
	}
	stats.LibraryBreakdown = libraryBreakdown
	stats.WorksTotal = worksTotal

	if worksTotal > 0 {
		var catalogedCount int
		err := h.DB.QueryRow(`
			SELECT COUNT(*)
			FROM works w
			LEFT JOIN work_primary wp ON wp.work_id = w.id
			WHERE w.retired_at IS NULL
			  AND EXISTS (SELECT 1 FROM work_contributors c WHERE c.work_id = w.id AND c.role = 'author')
			  AND COALESCE(wp.cover_url, '') <> ''
		`).Scan(&catalogedCount)
		if err != nil {
			log.Println("Error computing cataloged percent:", err)
			http.Error(w, "Error computing stats", http.StatusInternalServerError)
			return
		}
		stats.CatalogedPercent = int((float64(catalogedCount) / float64(worksTotal)) * 100)
	}

	// A work is in progress, or finished, by the file its reader touched last (DEC-077), counted
	// under that file's format: reading the English EPUB of a book whose main file is a PDF makes
	// it an ebook in progress.
	inProgressBreakdown, inProgressTotal, err := scanFormatBreakdownBy(
		h.DB, "lastrp.format", lastReadJoin,
		"lastrp.file_id IS NOT NULL AND lastrp.completed_at IS NULL AND wrs.work_id IS NULL", userID,
	)
	if err != nil {
		log.Println("Error computing in-progress breakdown:", err)
		http.Error(w, "Error computing stats", http.StatusInternalServerError)
		return
	}
	stats.InProgressBreakdown = inProgressBreakdown
	stats.InProgressCount = inProgressTotal

	// Finished this month is counted from the history, one per work however many times or in how
	// many formats it was finished (under the format of the latest), so finishing the EPUB and the
	// PDF of one book in a month is one book (DEC-080).
	var completedBreakdown FormatBreakdown
	var completedTotal int
	err = h.DB.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE `+shelfCondition("ebooks", "format")+`),
			COUNT(*) FILTER (WHERE `+shelfCondition("comics", "format")+`),
			COUNT(*) FILTER (WHERE `+shelfCondition("mangas", "format")+`),
			COUNT(*) FILTER (WHERE `+shelfCondition("audio", "format")+`),
			COUNT(*)
		FROM (
			SELECT DISTINCT ON (c.work_id) c.format, w.comic_kind
			FROM reading_completions c
			JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
			WHERE c.user_id = $1 AND date_trunc('month', c.completed_at) = date_trunc('month', CURRENT_TIMESTAMP)
			ORDER BY c.work_id, c.completed_at DESC
		) w`, userID).Scan(&completedBreakdown.Livros, &completedBreakdown.Quadrinhos, &completedBreakdown.Mangas, &completedBreakdown.Audio, &completedTotal)
	if err != nil {
		log.Println("Error computing completed-this-month breakdown:", err)
		http.Error(w, "Error computing stats", http.StatusInternalServerError)
		return
	}
	stats.CompletedBreakdown = completedBreakdown
	stats.CompletedThisMonth = completedTotal

	var totalSeconds sql.NullInt64
	err = h.DB.QueryRow(`SELECT SUM(reading_seconds) FROM reading_progress WHERE user_id = $1`, userID).Scan(&totalSeconds)
	if err != nil {
		log.Println("Error computing total reading seconds:", err)
		http.Error(w, "Error computing stats", http.StatusInternalServerError)
		return
	}
	if totalSeconds.Valid {
		stats.TotalReadingSeconds = int(totalSeconds.Int64)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
