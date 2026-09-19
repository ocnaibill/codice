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

// FormatBreakdown groups a count into the three shelf categories the
// dashboard displays: "livros" (epub/pdf/txt/md), "mangas" (cbz/cbr) and
// "audio" (audio formats).
type FormatBreakdown struct {
	Livros int `json:"livros"`
	Mangas int `json:"mangas"`
	Audio  int `json:"audio"`
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

func scanFormatBreakdown(db *sql.DB, whereSQL string, args ...interface{}) (FormatBreakdown, int, error) {
	var b FormatBreakdown
	query := `
		SELECT
			COUNT(*) FILTER (WHERE LOWER(w.format) IN ` + bookFormats + `),
			COUNT(*) FILTER (WHERE LOWER(w.format) IN ` + comicFormats + `),
			COUNT(*) FILTER (WHERE LOWER(w.format) IN ` + audioFormats + `),
			COUNT(*)
		FROM works w
		` + whereSQL
	var total int
	err := db.QueryRow(query, args...).Scan(&b.Livros, &b.Mangas, &b.Audio, &total)
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
			LEFT JOIN editions e ON w.id = e.work_id
			WHERE w.author_id IS NOT NULL AND COALESCE(e.cover_url, '') <> ''
		`).Scan(&catalogedCount)
		if err != nil {
			log.Println("Error computing cataloged percent:", err)
			http.Error(w, "Error computing stats", http.StatusInternalServerError)
			return
		}
		stats.CatalogedPercent = int((float64(catalogedCount) / float64(worksTotal)) * 100)
	}

	inProgressBreakdown, inProgressTotal, err := scanFormatBreakdown(
		h.DB,
		`JOIN user_progress up ON up.work_id = w.id AND up.user_id = $1 AND up.progress <> '' AND up.completed_at IS NULL`,
		userID,
	)
	if err != nil {
		log.Println("Error computing in-progress breakdown:", err)
		http.Error(w, "Error computing stats", http.StatusInternalServerError)
		return
	}
	stats.InProgressBreakdown = inProgressBreakdown
	stats.InProgressCount = inProgressTotal

	completedBreakdown, completedTotal, err := scanFormatBreakdown(
		h.DB,
		`JOIN user_progress up ON up.work_id = w.id AND up.user_id = $1
		 AND up.completed_at IS NOT NULL
		 AND date_trunc('month', up.completed_at) = date_trunc('month', CURRENT_TIMESTAMP)`,
		userID,
	)
	if err != nil {
		log.Println("Error computing completed-this-month breakdown:", err)
		http.Error(w, "Error computing stats", http.StatusInternalServerError)
		return
	}
	stats.CompletedBreakdown = completedBreakdown
	stats.CompletedThisMonth = completedTotal

	var totalSeconds sql.NullInt64
	err = h.DB.QueryRow(`SELECT SUM(reading_seconds) FROM user_progress WHERE user_id = $1`, userID).Scan(&totalSeconds)
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
