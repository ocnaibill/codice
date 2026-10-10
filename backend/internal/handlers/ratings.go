package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
)

// RatingsHandler is the stars a person gives a work (DEC-154): one rating per person and work, from 1 to 5, that only they see.
type RatingsHandler struct{ DB *sql.DB }

// Put answers PUT /works/{id}/rating {"stars": 4}: the caller's rating of the work is that, whether there was one or not.
func (h *RatingsHandler) Put(w http.ResponseWriter, r *http.Request) {
	work64, ok := collectionIDParam(r, "id")
	if !ok || work64 > math.MaxInt32 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var req struct {
		Stars *int `json:"stars"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Stars == nil {
		http.Error(w, "stars is a number from 1 to 5", http.StatusBadRequest)
		return
	}
	if *req.Stars < 1 || *req.Stars > 5 {
		http.Error(w, "stars is a number from 1 to 5", http.StatusBadRequest)
		return
	}
	var retired bool
	if err := h.DB.QueryRowContext(r.Context(), `SELECT retired_at IS NOT NULL FROM works WHERE id = $1`, work64).Scan(&retired); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Println("Error checking the work to rate:", err)
		http.Error(w, "Error saving the rating", http.StatusInternalServerError)
		return
	}
	if retired {
		http.Error(w, "A obra está na lixeira.", http.StatusConflict)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO work_ratings (user_id, work_id, stars) VALUES ($1::uuid, $2, $3)
		ON CONFLICT (user_id, work_id) DO UPDATE SET stars = EXCLUDED.stars, updated_at = now()`,
		currentUserID(r), work64, *req.Stars); err != nil {
		log.Println("Error saving the rating:", err)
		http.Error(w, "Error saving the rating", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete answers DELETE /works/{id}/rating: the caller's rating of the work goes. One that was not there is not an error.
func (h *RatingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	work64, ok := collectionIDParam(r, "id")
	if !ok || work64 > math.MaxInt32 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM work_ratings WHERE user_id = $1::uuid AND work_id = $2`, currentUserID(r), work64); err != nil {
		log.Println("Error taking the rating away:", err)
		http.Error(w, "Error saving the rating", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
