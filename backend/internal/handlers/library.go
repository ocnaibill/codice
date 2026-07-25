package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Work represents the structure sent to the frontend
type Work struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	CoverURL string `json:"coverUrl"`
}

// LibraryHandler stores the database connection
type LibraryHandler struct {
	DB *sql.DB
}

// GetWorks fetches works from PostgreSQL
func (h *LibraryHandler) GetWorks(w http.ResponseWriter, r *http.Request) {
	// Query performs a safe JOIN considering author or cover can be null
	query := `
		SELECT 
			w.id, 
			w.original_title, 
			COALESCE(p.name, 'Unknown Author') as author, 
			COALESCE(e.cover_url, '') as cover_url
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
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
		if err := rows.Scan(&work.ID, &work.Title, &work.Author, &work.CoverURL); err != nil {
			http.Error(w, "Error reading data", http.StatusInternalServerError)
			return
		}
		
		// Fallback cover if database returns empty string
		if work.CoverURL == "" {
			work.CoverURL = "https://via.placeholder.com/300x450/1f2937/d1d5db?text=No+Cover"
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