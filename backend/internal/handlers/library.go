package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"

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
	Tags            []string `json:"tags"`
	ReadingProgress string   `json:"readingProgress,omitempty"`
}

// LibraryHandler stores the database connection
type LibraryHandler struct {
	DB *sql.DB
}

// GetWorks fetches works from PostgreSQL with aggregated tags
func (h *LibraryHandler) GetWorks(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT 
			w.id, 
			w.original_title, 
			COALESCE(p.name, 'Unknown Author') as author, 
			COALESCE(e.cover_url, '') as cover_url,
			COALESCE(array_agg(t.name) FILTER (WHERE t.name IS NOT NULL), '{}') as tags
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		LEFT JOIN work_tags wt ON w.id = wt.work_id
		LEFT JOIN tags t ON wt.tag_id = t.id
		GROUP BY w.id, w.original_title, p.name, e.cover_url
		ORDER BY w.id DESC
	`

	rows, err := h.DB.Query(query)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var works []Work
	for rows.Next() {
		var work Work
		if err := rows.Scan(&work.ID, &work.Title, &work.Author, &work.CoverURL, pq.Array(&work.Tags)); err != nil {
			http.Error(w, "Error reading data", http.StatusInternalServerError)
			return
		}

		// Fallback cover if database returns empty string
		if work.CoverURL == "" {
			work.CoverURL = "/covers/placeholder.svg"
		}

		if work.Tags == nil {
			work.Tags = []string{}
		}

		works = append(works, work)
	}

	// Prevent returning null (returns empty array)
	if works == nil {
		works = []Work{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(works)
}

// GetWorkByID fetches a single work by its ID along with tags and per-user reading progress
func (h *LibraryHandler) GetWorkByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		userID = middleware.DefaultDevUserID
	}

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
			up.progress,
			COALESCE(array_agg(t.name) FILTER (WHERE t.name IS NOT NULL), '{}') as tags
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		LEFT JOIN user_progress up ON w.id = up.work_id AND up.user_id = $2
		LEFT JOIN work_tags wt ON w.id = wt.work_id
		LEFT JOIN tags t ON wt.tag_id = t.id
		WHERE w.id = $1
		GROUP BY w.id, w.original_title, p.name, e.cover_url, w.file_path, up.progress
	`

	err := h.DB.QueryRow(query, id, userID).Scan(&work.ID, &work.Title, &work.Author, &work.CoverURL, &filePath, &progress, pq.Array(&work.Tags))
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

// ProgressRequest represents the payload for updating reading progress
type ProgressRequest struct {
	Progress string `json:"progress"`
}

// UpdateProgress updates the reading progress location for a work isolated by user_id
func (h *LibraryHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	workID := chi.URLParam(r, "id")

	userID, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok || userID == "" {
		userID = middleware.DefaultDevUserID
	}

	var req ProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO user_progress (user_id, work_id, progress, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, work_id) 
		DO UPDATE SET progress = EXCLUDED.progress, updated_at = CURRENT_TIMESTAMP;
	`

	_, err := h.DB.Exec(query, userID, workID, req.Progress)
	if err != nil {
		http.Error(w, "Error saving isolated user reading progress", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

	tx.Exec("DELETE FROM work_tags WHERE work_id = $1", id)
	tx.Exec("DELETE FROM user_progress WHERE work_id = $1", id)
	tx.Exec("DELETE FROM editions WHERE work_id = $1", id)

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