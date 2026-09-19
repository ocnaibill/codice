package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

// Work represents the structure sent to the frontend
type Work struct {
	ID              int      `json:"id"`
	Title           string   `json:"title"`
	Author          string   `json:"author"`
	CoverURL        string   `json:"coverUrl"`
	FileURL         string   `json:"fileUrl,omitempty"`
	Format          string   `json:"format,omitempty"`
	Series          string   `json:"series,omitempty"`
	SeriesIndex     float64  `json:"seriesIndex,omitempty"`
	MediaStatus     string   `json:"mediaStatus,omitempty"`
	Tags            []string `json:"tags"`
	ReadingProgress string   `json:"readingProgress,omitempty"`
	PercentComplete float64  `json:"percentComplete"`
	Completed       bool     `json:"completed"`
	IsFavorite      bool     `json:"isFavorite"`
}

// LibraryHandler stores the database connection
type LibraryHandler struct {
	DB *sql.DB
}

// currentUserID extracts the authenticated user id from context. AuthMiddleware
// guarantees it on protected routes; if it is ever missing this returns "" and
// never a substitute identity, so the query matches nothing instead of acting
// as another user.
func currentUserID(r *http.Request) string {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	return userID
}

// GetWorks fetches works from PostgreSQL with aggregated tags, server-side
// pagination, search, and optional filters: inProgress=true (has unfinished
// reading progress) and favorite=true (marked as favorite by the caller).
func (h *LibraryHandler) GetWorks(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)

	page := 1
	limit := 50
	search := r.URL.Query().Get("search")
	inProgressOnly := r.URL.Query().Get("inProgress") == "true"
	favoriteOnly := r.URL.Query().Get("favorite") == "true"
	formatGroup := r.URL.Query().Get("formatGroup") // "ebooks" | "comics" | "audio"

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
	args = append(args, userID) // $1 always the current user, used by both joins
	argIdx := 2

	if search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(w.original_title) LIKE LOWER($%d) OR LOWER(COALESCE(p.name, '')) LIKE LOWER($%d))", argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if inProgressOnly {
		whereClauses = append(whereClauses, "(up.progress IS NOT NULL AND up.completed_at IS NULL)")
	}
	if favoriteOnly {
		whereClauses = append(whereClauses, "f.user_id IS NOT NULL")
	}
	switch formatGroup {
	case "ebooks":
		whereClauses = append(whereClauses, "LOWER(w.format) IN "+bookFormats)
	case "comics":
		whereClauses = append(whereClauses, "LOWER(w.format) IN "+comicFormats)
	case "audio":
		whereClauses = append(whereClauses, "LOWER(w.format) IN "+audioFormats)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := `
		SELECT COUNT(DISTINCT w.id)
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN user_progress up ON w.id = up.work_id AND up.user_id = $1
		LEFT JOIN favorites f ON w.id = f.work_id AND f.user_id = $1
	` + whereSQL

	var totalCount int
	if err := h.DB.QueryRow(countQuery, args...).Scan(&totalCount); err != nil {
		log.Println("Error counting works:", err)
		http.Error(w, "Error counting works", http.StatusInternalServerError)
		return
	}

	query := `
		SELECT
			w.id,
			w.original_title,
			COALESCE(p.name, 'Unknown Author') as author,
			COALESCE(e.cover_url, '') as cover_url,
			w.file_path,
			COALESCE(w.format, '') as format,
			COALESCE(w.series, '') as series,
			COALESCE(w.series_index, 0) as series_index,
			COALESCE(w.media_status, 'READY') as media_status,
			COALESCE(array_agg(t.name) FILTER (WHERE t.name IS NOT NULL), '{}') as tags,
			COALESCE(up.progress, '') as reading_progress,
			COALESCE(up.percent_complete, 0) as percent_complete,
			(up.completed_at IS NOT NULL) as completed,
			(f.user_id IS NOT NULL) as is_favorite
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		LEFT JOIN work_tags wt ON w.id = wt.work_id
		LEFT JOIN tags t ON wt.tag_id = t.id
		LEFT JOIN user_progress up ON w.id = up.work_id AND up.user_id = $1
		LEFT JOIN favorites f ON w.id = f.work_id AND f.user_id = $1
	` + whereSQL + `
		GROUP BY w.id, w.original_title, p.name, e.cover_url, w.file_path, w.format, w.series, w.series_index, w.media_status, up.progress, up.percent_complete, up.completed_at, f.user_id
		ORDER BY w.id DESC
	`

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		log.Println("Error fetching works:", err)
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var works []Work
	for rows.Next() {
		var work Work
		var filePath sql.NullString
		err := rows.Scan(
			&work.ID, &work.Title, &work.Author, &work.CoverURL, &filePath, &work.Format,
			&work.Series, &work.SeriesIndex, &work.MediaStatus, pq.Array(&work.Tags),
			&work.ReadingProgress, &work.PercentComplete, &work.Completed, &work.IsFavorite,
		)
		if err != nil {
			log.Println("Error scanning work:", err)
			continue
		}

		if filePath.Valid && filePath.String != "" {
			work.FileURL = "/files/" + filePath.String
		}

		if work.CoverURL == "" {
			work.CoverURL = "/covers/placeholder.svg"
		}

		if work.Tags == nil {
			work.Tags = []string{}
		}

		works = append(works, work)
	}

	if works == nil {
		works = []Work{}
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

// GetWorkByID fetches a single work by its ID along with tags and per-user reading progress
func (h *LibraryHandler) GetWorkByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := currentUserID(r)

	var filePath sql.NullString
	var progress sql.NullString
	var work Work

	query := `
		SELECT
			w.id,
			w.original_title,
			COALESCE(p.name, 'Unknown Author') as author,
			COALESCE(e.cover_url, '') as cover_url,
			w.file_path,
			COALESCE(w.format, '') as format,
			COALESCE(w.series, '') as series,
			COALESCE(w.series_index, 0) as series_index,
			COALESCE(w.media_status, 'READY') as media_status,
			up.progress,
			COALESCE(up.percent_complete, 0) as percent_complete,
			(up.completed_at IS NOT NULL) as completed,
			(f.user_id IS NOT NULL) as is_favorite,
			COALESCE(array_agg(t.name) FILTER (WHERE t.name IS NOT NULL), '{}') as tags
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		LEFT JOIN user_progress up ON w.id = up.work_id AND up.user_id = $2
		LEFT JOIN favorites f ON w.id = f.work_id AND f.user_id = $2
		LEFT JOIN work_tags wt ON w.id = wt.work_id
		LEFT JOIN tags t ON wt.tag_id = t.id
		WHERE w.id = $1
		GROUP BY w.id, w.original_title, p.name, e.cover_url, w.file_path, w.format, w.series, w.series_index, w.media_status, up.progress, up.percent_complete, up.completed_at, f.user_id
	`

	err := h.DB.QueryRow(query, id, userID).Scan(
		&work.ID, &work.Title, &work.Author, &work.CoverURL, &filePath, &work.Format,
		&work.Series, &work.SeriesIndex, &work.MediaStatus, &progress, &work.PercentComplete,
		&work.Completed, &work.IsFavorite, pq.Array(&work.Tags),
	)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	if work.CoverURL == "" {
		work.CoverURL = "/covers/placeholder.svg"
	}

	if work.Tags == nil {
		work.Tags = []string{}
	}

	if progress.Valid {
		work.ReadingProgress = progress.String
	}

	if filePath.Valid && filePath.String != "" {
		work.FileURL = "/files/" + filePath.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(work)
}

// UpdateWorkRequest represents the payload for updating work metadata
type UpdateWorkRequest struct {
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Tags   []string `json:"tags"`
}

// UpdateWork updates title, resolves author, and syncs tags within an atomic database transaction
func (h *LibraryHandler) UpdateWork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Resolve Author
	var authorID int
	authorName := req.Author
	if authorName == "" {
		authorName = "Unknown Author"
	}

	err = tx.QueryRow("SELECT id FROM person WHERE name = $1", authorName).Scan(&authorID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = tx.QueryRow("INSERT INTO person (name) VALUES ($1) RETURNING id", authorName).Scan(&authorID)
			if err != nil {
				http.Error(w, "Error creating author record", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Error querying author record", http.StatusInternalServerError)
			return
		}
	}

	// 2. Update Work record with new Title and Author ID
	_, err = tx.Exec("UPDATE works SET original_title = $1, author_id = $2 WHERE id = $3", req.Title, authorID, id)
	if err != nil {
		http.Error(w, "Error updating work record", http.StatusInternalServerError)
		return
	}

	// 3. Sync Tags (delete existing relations and re-insert new ones)
	_, err = tx.Exec("DELETE FROM work_tags WHERE work_id = $1", id)
	if err != nil {
		http.Error(w, "Error resetting work tags", http.StatusInternalServerError)
		return
	}

	for _, tagName := range req.Tags {
		if tagName == "" {
			continue
		}
		var tagID int
		err = tx.QueryRow("SELECT id FROM tags WHERE name = $1", tagName).Scan(&tagID)
		if err != nil {
			if err == sql.ErrNoRows {
				err = tx.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", tagName).Scan(&tagID)
				if err != nil {
					http.Error(w, "Error creating tag record", http.StatusInternalServerError)
					return
				}
			} else {
				http.Error(w, "Error querying tag record", http.StatusInternalServerError)
				return
			}
		}

		_, err = tx.Exec("INSERT INTO work_tags (work_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, tagID)
		if err != nil {
			http.Error(w, "Error linking work tag", http.StatusInternalServerError)
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
type ProgressRequest struct {
	Progress  string   `json:"progress"`
	Percent   *float64 `json:"percent,omitempty"`
	Completed *bool    `json:"completed,omitempty"`
}

// UpdateProgress updates the reading progress location (and optionally
// percent/completion) for a work, isolated by user_id.
func (h *LibraryHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	workID := chi.URLParam(r, "id")
	userID := currentUserID(r)

	var req ProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
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

	query := `
		INSERT INTO user_progress (user_id, work_id, progress, percent_complete, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, CASE WHEN $5 THEN CURRENT_TIMESTAMP ELSE NULL END, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, work_id)
		DO UPDATE SET
			progress = EXCLUDED.progress,
			percent_complete = CASE WHEN $6 THEN EXCLUDED.percent_complete ELSE user_progress.percent_complete END,
			completed_at = CASE
				WHEN $5 THEN CURRENT_TIMESTAMP
				WHEN $7 THEN NULL
				ELSE user_progress.completed_at
			END,
			updated_at = CURRENT_TIMESTAMP;
	`

	percentProvided := req.Percent != nil
	completedExplicitlyFalse := req.Completed != nil && !*req.Completed

	_, err := h.DB.Exec(query, userID, workID, req.Progress, percent, completed, percentProvided, completedExplicitlyFalse)
	if err != nil {
		log.Println("Error saving reading progress:", err)
		http.Error(w, "Error saving isolated user reading progress", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HeartbeatRequest reports how many seconds of active reading happened
// since the reader's last heartbeat tick.
type HeartbeatRequest struct {
	Seconds float64 `json:"seconds"`
}

// ReadingHeartbeat accumulates real reading time for a work. Seconds are
// clamped to a small window so a stalled tab or clock skew can't inflate
// the total — the reader is expected to call this roughly every 20-30s
// while the document is open and the tab is visible.
func (h *LibraryHandler) ReadingHeartbeat(w http.ResponseWriter, r *http.Request) {
	workID := chi.URLParam(r, "id")
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

	query := `
		INSERT INTO user_progress (user_id, work_id, progress, reading_seconds, updated_at)
		VALUES ($1, $2, '', $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, work_id)
		DO UPDATE SET reading_seconds = user_progress.reading_seconds + $3, updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := h.DB.Exec(query, userID, workID, seconds); err != nil {
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

// DeleteWork removes a work from PostgreSQL within a transaction and deletes its physical files from server disk
func (h *LibraryHandler) DeleteWork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 1. Retrieve file_path and cover_url before database deletion
	var filePath sql.NullString
	var coverURL sql.NullString

	query := `
		SELECT w.file_path, e.cover_url
		FROM works w
		LEFT JOIN editions e ON w.id = e.work_id
		WHERE w.id = $1
	`
	err := h.DB.QueryRow(query, id).Scan(&filePath, &coverURL)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching book files", http.StatusInternalServerError)
		return
	}

	// 2. Atomic Database Transaction Cleanup
	tx, err := h.DB.Begin()
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err = tx.Exec("DELETE FROM work_tags WHERE work_id = $1", id); err != nil {
		http.Error(w, "Error deleting work tags", http.StatusInternalServerError)
		return
	}
	if _, err = tx.Exec("DELETE FROM user_progress WHERE work_id = $1", id); err != nil {
		http.Error(w, "Error deleting user progress", http.StatusInternalServerError)
		return
	}
	if _, err = tx.Exec("DELETE FROM editions WHERE work_id = $1", id); err != nil {
		http.Error(w, "Error deleting editions", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec("DELETE FROM works WHERE id = $1", id)
	if err != nil {
		http.Error(w, "Error deleting work from database", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "Error committing deletion transaction", http.StatusInternalServerError)
		return
	}

	// 3. Physical Server Disk Cleanup
	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	if filePath.Valid && filePath.String != "" {
		os.Remove(filepath.Join(storagePath, filePath.String))
	}

	if coverURL.Valid && coverURL.String != "" {
		coverFilename := path.Base(coverURL.String)
		os.Remove(filepath.Join(storagePath, "covers", coverFilename))
	}

	w.WriteHeader(http.StatusOK)
}