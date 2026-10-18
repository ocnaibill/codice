package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/redis/go-redis/v9"
)

// WorkCoverHandler is how staff choose the cover of a work by hand (DEC-173) and go back to the one the file has. The cropping is done by the
// page, to the proportion every cover in the library has (2:3); what arrives here is the JPEG already cut, and it is checked, not trusted.
type WorkCoverHandler struct {
	DB        *sql.DB
	CoversDir string
	// SearchDir is where the worker keeps the covers a search found (DEC-174): a folder that expires, outside the covers of the library.
	SearchDir string
	// RedisClient wakes the workers when a search is asked for; nil only means they find it on their next poll.
	RedisClient *redis.Client
}

// manualCoverPrefix is what names the covers chosen by hand of a work: they are the only ones of the folder that the work owns and that this
// handler may remove (the worker owns the others).
func manualCoverPrefix(workID int) string {
	return fmt.Sprintf("/covers/cover_manual_%d_", workID)
}

// Set answers PUT /works/{id}/cover (staff): the cover of the work, sent as a JPEG in "image". It is locked against the worker from then on,
// and the one that was there is no longer kept if it was also chosen by hand.
func (h *WorkCoverHandler) Set(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	if h.CoversDir == "" {
		http.Error(w, "Covers cannot be kept here", http.StatusInternalServerError)
		return
	}
	data, ok := readManualJPEG(w, r, coverImage)
	if !ok {
		return
	}
	if err := os.MkdirAll(h.CoversDir, 0o755); err != nil {
		log.Println("Error preparing the folder of the covers:", err)
		http.Error(w, "Error saving the cover", http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("cover_manual_%d_%d.jpg", id, time.Now().UnixNano())
	dest := filepath.Join(h.CoversDir, name)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		log.Println("Error writing the cover:", err)
		http.Error(w, "Error saving the cover", http.StatusInternalServerError)
		return
	}
	url := "/covers/" + name

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		os.Remove(dest)
		http.Error(w, "Error saving the cover", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var previous sql.NullString
	err = tx.QueryRowContext(r.Context(), `
		SELECT e.cover_url FROM works w JOIN editions e ON e.work_id = w.id AND e.is_primary
		WHERE w.id = $1 FOR UPDATE OF w, e`, id).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		os.Remove(dest)
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	actor := currentUserID(r)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE editions SET cover_url = $2 WHERE work_id = $1 AND is_primary`, id, url)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE works SET cover_lock = TRUE WHERE id = $1`, id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `
			INSERT INTO work_field_sources (work_id, field, source, actor_id)
			VALUES ($1, 'cover', $2, NULLIF($3, '')::uuid)
			ON CONFLICT (work_id, field) DO UPDATE SET source = EXCLUDED.source, actor_id = EXCLUDED.actor_id, updated_at = now()`,
			id, sourceManual, actor)
	}
	if err == nil {
		err = audit.Record(r.Context(), tx, actor, "work.cover.set", "work", strconv.Itoa(id), map[string]any{"bytes": len(data)})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		os.Remove(dest)
		log.Println("Error saving a cover:", err)
		http.Error(w, "Error saving the cover", http.StatusInternalServerError)
		return
	}
	h.dropManual(id, previous.String)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// Reset answers DELETE /works/{id}/cover (staff): back to the cover the file has. The lock is released, so that a file read again is allowed
// to give another. When the work had no cover of the file, it has none.
func (h *WorkCoverHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error going back to the cover", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var current, auto sql.NullString
	err = tx.QueryRowContext(r.Context(), `
		SELECT e.cover_url, e.auto_cover_url FROM works w JOIN editions e ON e.work_id = w.id AND e.is_primary
		WHERE w.id = $1 FOR UPDATE OF w, e`, id).Scan(&current, &auto)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	actor := currentUserID(r)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE editions SET cover_url = auto_cover_url WHERE work_id = $1 AND is_primary`, id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE works SET cover_lock = FALSE WHERE id = $1`, id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM work_field_sources WHERE work_id = $1 AND field = 'cover' AND source = $2`, id, sourceManual)
	}
	if err == nil {
		err = audit.Record(r.Context(), tx, actor, "work.cover.reset", "work", strconv.Itoa(id), map[string]any{})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Println("Error going back to a cover:", err)
		http.Error(w, "Error going back to the cover", http.StatusInternalServerError)
		return
	}
	h.dropManual(id, current.String)
	url := auto.String
	if url == "" {
		url = "/covers/placeholder.svg"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// dropManual removes the file of a cover chosen by hand of the work that is not the one in use any more. Only the files that carry the name
// of this work's manual covers are touched.
func (h *WorkCoverHandler) dropManual(workID int, old string) {
	if h.CoversDir == "" || old == "" || !strings.HasPrefix(old, manualCoverPrefix(workID)) {
		return
	}
	os.Remove(filepath.Join(h.CoversDir, filepath.Base(old)))
}
