package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// NotesHandler stores the database connection
type NotesHandler struct {
	DB *sql.DB
}

// Note represents a quote/annotation a user saved against a work
type Note struct {
	ID         int    `json:"id"`
	WorkID     int    `json:"workId"`
	WorkTitle  string `json:"workTitle"`
	WorkAuthor string `json:"workAuthor"`
	Quote      string `json:"quote"`
	CreatedAt  string `json:"createdAt"`
}

// CreateNoteRequest is the payload for saving a new quote/annotation
type CreateNoteRequest struct {
	Quote string `json:"quote"`
}

// CreateNote saves a quote/annotation against a work for the current user
func (h *NotesHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	workID := chi.URLParam(r, "id")
	userID := currentUserID(r)

	var req CreateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if len(req.Quote) == 0 {
		http.Error(w, "Quote text is required", http.StatusBadRequest)
		return
	}
	if len(req.Quote) > 2000 {
		req.Quote = req.Quote[:2000]
	}

	var id int
	err := h.DB.QueryRow(
		`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, $3) RETURNING id`,
		userID, workID, req.Quote,
	).Scan(&id)
	if err != nil {
		log.Println("Error creating note:", err)
		http.Error(w, "Error creating note", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"id": id})
}

// ListNotes returns the current user's most recent quotes/annotations,
// each attributed to its work's title and author.
func (h *NotesHandler) ListNotes(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 50 {
			limit = v
		}
	}

	query := `
		SELECT n.id, n.work_id, w.original_title, COALESCE(p.name, 'Unknown Author'), n.quote, n.created_at
		FROM notes n
		JOIN works w ON w.id = n.work_id
		LEFT JOIN person p ON w.author_id = p.id
		WHERE n.user_id = $1
		ORDER BY n.created_at DESC
		LIMIT $2
	`

	rows, err := h.DB.Query(query, userID, limit)
	if err != nil {
		log.Println("Error listing notes:", err)
		http.Error(w, "Error listing notes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	notes := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.WorkID, &n.WorkTitle, &n.WorkAuthor, &n.Quote, &n.CreatedAt); err != nil {
			log.Println("Error scanning note:", err)
			continue
		}
		notes = append(notes, n)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": notes})
}

// DeleteNote removes a note owned by the current user
func (h *NotesHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := currentUserID(r)

	_, err := h.DB.Exec(`DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		log.Println("Error deleting note:", err)
		http.Error(w, "Error deleting note", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
