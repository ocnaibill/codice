package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// NotesHandler stores the database connection
type NotesHandler struct {
	DB *sql.DB
}

// Note represents a quote/annotation a user saved against a work. Its title and
// author come from the reference stored on the note, so it stays complete when
// the work is retired or deleted; SourceAvailable then turns false and WorkID
// is null (RF-039).
type Note struct {
	ID              int    `json:"id"`
	WorkID          *int   `json:"workId"`
	WorkTitle       string `json:"workTitle"`
	WorkAuthor      string `json:"workAuthor"`
	Quote           string `json:"quote"`
	CreatedAt       string `json:"createdAt"`
	SourceAvailable bool   `json:"sourceAvailable"`
}

// CreateNoteRequest is the payload for saving a new quote/annotation
type CreateNoteRequest struct {
	Quote string `json:"quote"`
}

// CreateNote saves a quote/annotation against a work for the current user
func (h *NotesHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
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

	// The bibliographic reference is copied from the work by the database when
	// the note is created. Notes can only be added to works the caller can see.
	var id int
	err := h.DB.QueryRow(
		`INSERT INTO notes (user_id, work_id, quote)
		 SELECT $1, w.id, $3 FROM works w WHERE w.id = $2 AND w.retired_at IS NULL
		 RETURNING id`,
		userID, workID, req.Quote,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
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

	// LEFT JOIN: a note is listed whether or not its work still exists.
	query := `
		SELECT n.id, n.work_id, n.source_title, COALESCE(n.source_author, 'Unknown Author'),
		       n.quote, n.created_at, (w.id IS NOT NULL AND w.retired_at IS NULL)
		FROM notes n
		LEFT JOIN works w ON w.id = n.work_id
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
		var workID sql.NullInt64
		if err := rows.Scan(&n.ID, &workID, &n.WorkTitle, &n.WorkAuthor, &n.Quote, &n.CreatedAt, &n.SourceAvailable); err != nil {
			log.Println("Error scanning note:", err)
			continue
		}
		if workID.Valid {
			id := int(workID.Int64)
			n.WorkID = &id
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
