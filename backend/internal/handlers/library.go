package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
)

// Work represents the structure sent to the frontend
type Work struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	CoverURL string   `json:"coverUrl"`
	FileURL  string   `json:"fileUrl,omitempty"`
	Tags     []string `json:"tags"`
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
			work.CoverURL = "https://via.placeholder.com/300x450/1f2937/d1d5db?text=No+Cover"
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

// GetWorkByID fetches a single work by its ID along with tags
func (h *LibraryHandler) GetWorkByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filePath sql.NullString
	var work Work

	query := `
		SELECT 
			w.id, 
			w.original_title, 
			COALESCE(p.name, 'Unknown Author') as author, 
			COALESCE(e.cover_url, '') as cover_url,
			w.file_path,
			COALESCE(array_agg(t.name) FILTER (WHERE t.name IS NOT NULL), '{}') as tags
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		LEFT JOIN work_tags wt ON w.id = wt.work_id
		LEFT JOIN tags t ON wt.tag_id = t.id
		WHERE w.id = $1
		GROUP BY w.id, w.original_title, p.name, e.cover_url, w.file_path
	`

	err := h.DB.QueryRow(query, id).Scan(&work.ID, &work.Title, &work.Author, &work.CoverURL, &filePath, pq.Array(&work.Tags))
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching book", http.StatusInternalServerError)
		return
	}

	if work.CoverURL == "" {
		work.CoverURL = "https://via.placeholder.com/300x450/1f2937/d1d5db?text=No+Cover"
	}

	if work.Tags == nil {
		work.Tags = []string{}
	}

	if filePath.Valid && filePath.String != "" {
		work.FileURL = "http://localhost:8080/files/" + filePath.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(work)
}