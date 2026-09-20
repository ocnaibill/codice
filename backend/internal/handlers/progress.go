package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/locator"
)

// ProgressHandler serves (under /progress/files/{id}, apart from /files/*, which serves the
// library's own files) the reading position of one file for the calling user. The file is
// the unit (DEC-030): the EPUB and the PDF of one book keep separate positions, and nothing
// here reads or writes another user's.
type ProgressHandler struct {
	DB *sql.DB
}

// ProgressState is a position as the client sees it. Revision is 0 when nothing was saved.
// Position is the plain text older readers wrote; Locator is null until a locator is saved.
type ProgressState struct {
	FileID          int64           `json:"fileId"`
	Locator         json.RawMessage `json:"locator"`
	LocatorVersion  *int            `json:"locatorVersion"`
	Position        string          `json:"position"`
	Percent         float64         `json:"percent"`
	Completed       bool            `json:"completed"`
	Revision        int64           `json:"revision"`
	Device          string          `json:"device,omitempty"`
	UpdatedAt       *time.Time      `json:"updatedAt"`       // when the server received it
	ClientUpdatedAt *time.Time      `json:"clientUpdatedAt"` // when the device says it happened
}

// PutProgressRequest saves a position. Percent and Completed are pointers so that "not sent"
// is not "zero": a viewer that does not know the percentage must not erase the saved one.
// BaseRevision is the revision the device last saw: if another device saved since, the write
// is refused with the current state instead of overwriting it silently.
type PutProgressRequest struct {
	Locator      json.RawMessage `json:"locator"`
	Percent      *float64        `json:"percent"`
	Completed    *bool           `json:"completed"`
	Device       string          `json:"device"`
	ClientTime   *time.Time      `json:"clientTime"`
	BaseRevision *int64          `json:"baseRevision"`
}

func fileIDParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// fileFormat returns the format of a file of an available work, or sql.ErrNoRows.
func (h *ProgressHandler) fileFormat(r *http.Request, fileID int64) (string, error) {
	var format sql.NullString
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT f.format FROM files f
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE f.id = $1 AND w.retired_at IS NULL`, fileID).Scan(&format)
	return format.String, err
}

const progressColumns = `COALESCE(rp.position, ''), rp.locator, rp.locator_version, COALESCE(rp.percent_complete, 0),
	(rp.completed_at IS NOT NULL), COALESCE(rp.revision, 0), COALESCE(rp.device, ''), rp.updated_at, rp.client_updated_at`

func scanProgress(row *sql.Row, fileID int64) (ProgressState, error) {
	st := ProgressState{FileID: fileID}
	var loc []byte
	var ver sql.NullInt64
	var updated, client sql.NullTime
	err := row.Scan(&st.Position, &loc, &ver, &st.Percent, &st.Completed, &st.Revision, &st.Device, &updated, &client)
	if err != nil {
		return st, err
	}
	if loc != nil {
		st.Locator = json.RawMessage(loc)
	}
	if ver.Valid {
		v := int(ver.Int64)
		st.LocatorVersion = &v
	}
	if updated.Valid {
		st.UpdatedAt = &updated.Time
	}
	if client.Valid {
		st.ClientUpdatedAt = &client.Time
	}
	return st, nil
}

func (h *ProgressHandler) load(r *http.Request, userID string, fileID int64) (ProgressState, error) {
	return scanProgress(h.DB.QueryRowContext(r.Context(),
		`SELECT `+progressColumns+` FROM (SELECT 1) one
		 LEFT JOIN reading_progress rp ON rp.user_id = $1 AND rp.file_id = $2`, userID, fileID), fileID)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Get returns the caller's position in a file, or an empty one (revision 0).
func (h *ProgressHandler) Get(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if _, err := h.fileFormat(r, fileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		log.Println("Error checking file for progress:", err)
		http.Error(w, "Error reading progress", http.StatusInternalServerError)
		return
	}
	st, err := h.load(r, currentUserID(r), fileID)
	if err != nil {
		log.Println("Error reading progress:", err)
		http.Error(w, "Error reading progress", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Put saves the caller's position in a file. The locator must be of the kind that fits the
// file's format, and is stored in its canonical form.
func (h *ProgressHandler) Put(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var req PutProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	format, err := h.fileFormat(r, fileID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error checking file for progress:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	canonical, err := locator.Validate(format, req.Locator)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var percent sql.NullFloat64
	if req.Percent != nil {
		if math.IsNaN(*req.Percent) || math.IsInf(*req.Percent, 0) {
			http.Error(w, "percent is not a number", http.StatusBadRequest)
			return
		}
		percent = sql.NullFloat64{Float64: math.Max(0, math.Min(100, *req.Percent)), Valid: true}
	}
	device := strings.TrimSpace(req.Device)
	if len(device) > 100 {
		device = device[:100]
	}
	var completed sql.NullBool // NULL keeps it, TRUE marks it finished now, FALSE reopens it
	if req.Completed != nil {
		completed = sql.NullBool{Bool: *req.Completed, Valid: true}
	}
	var base sql.NullInt64
	if req.BaseRevision != nil {
		base = sql.NullInt64{Int64: *req.BaseRevision, Valid: true}
	}

	// One statement decides: the update only happens if nobody moved the revision since
	// the device last looked. No row back means the write was refused.
	var revision int64
	err = h.DB.QueryRowContext(r.Context(), `
		INSERT INTO reading_progress AS rp (user_id, file_id, position, locator, locator_version,
		                                    percent_complete, completed_at, device, client_updated_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, COALESCE($6::float8, 0), CASE WHEN $7::boolean THEN now() END, NULLIF($8, ''), $9, now())
		ON CONFLICT (user_id, file_id) DO UPDATE SET
			position = EXCLUDED.position,
			locator = EXCLUDED.locator,
			locator_version = EXCLUDED.locator_version,
			percent_complete = COALESCE($6::float8, rp.percent_complete),
			-- A finished file stays finished: the first date is kept, and only an explicit false reopens it.
			completed_at = CASE WHEN $7::boolean IS NULL THEN rp.completed_at WHEN $7::boolean THEN COALESCE(rp.completed_at, now()) END,
			device = EXCLUDED.device,
			client_updated_at = EXCLUDED.client_updated_at,
			revision = rp.revision + 1,
			updated_at = now()
		WHERE $10::bigint IS NULL OR rp.revision = $10
		RETURNING revision`,
		userID, fileID, locator.Position(canonical), string(canonical), locator.Version,
		percent, completed, device, req.ClientTime, base).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		st, lerr := h.load(r, userID, fileID)
		if lerr != nil {
			log.Println("Error reading progress after a conflict:", lerr)
			http.Error(w, "Error saving progress", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusConflict, st)
		return
	}
	if err != nil {
		log.Println("Error saving progress:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}

	st, err := h.load(r, userID, fileID)
	if err != nil {
		log.Println("Error reading saved progress:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	if !st.Completed {
		h.reopenWork(r, userID, fileID) // reading on: the work is no longer "finished"
	}
	writeJSON(w, http.StatusOK, st)
}

// reopenWork takes the mark "the whole work is finished" off the work a file belongs to (DEC-080).
// It is what reading on does: the person is back in the book. A finished file that only keeps
// saving its last page does not.
func (h *ProgressHandler) reopenWork(r *http.Request, userID string, fileID int64) {
	if _, err := h.DB.ExecContext(r.Context(), `
		DELETE FROM work_reading_state
		WHERE user_id = $1 AND work_id = (SELECT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = $2)`,
		userID, fileID); err != nil {
		log.Println("Error clearing the finished mark:", err)
	}
}

// PutCompletionRequest marks a file finished or reopens it.
type PutCompletionRequest struct {
	Completed *bool `json:"completed"`
	// Restart, with completed false, is "read it again": the position and percentage go too, so the
	// file is one nobody has begun, and finishing it again counts as another time (DEC-080).
	Restart bool `json:"restart"`
}

// SetCompletion marks the caller's file as finished, or reopens it (DEC-077): the explicit
// action, for a book finished elsewhere or one to read again. It needs no locator. Finishing
// sets the percentage to 100; reopening a file that was at 100 takes it back to 0, and keeps the
// position, so it can be picked up where it was or read from the start.
func (h *ProgressHandler) SetCompletion(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req PutCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Completed == nil || (req.Restart && *req.Completed) {
		http.Error(w, "completed is true or false; restart only goes with false", http.StatusBadRequest)
		return
	}
	if _, err := h.fileFormat(r, fileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		log.Println("Error checking file for completion:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	userID := currentUserID(r)
	_, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO reading_progress AS rp (user_id, file_id, position, percent_complete, completed_at, updated_at)
		VALUES ($1, $2, '', CASE WHEN $3::boolean THEN 100 ELSE 0 END, CASE WHEN $3::boolean THEN now() END, now())
		ON CONFLICT (user_id, file_id) DO UPDATE SET
			completed_at = CASE WHEN $3::boolean THEN COALESCE(rp.completed_at, now()) END,
			percent_complete = CASE WHEN $3::boolean THEN 100 WHEN $4::boolean OR rp.percent_complete >= 100 THEN 0 ELSE rp.percent_complete END,
			position = CASE WHEN $4::boolean THEN '' ELSE rp.position END,
			locator = CASE WHEN $4::boolean THEN NULL ELSE rp.locator END,
			locator_version = CASE WHEN $4::boolean THEN NULL ELSE rp.locator_version END,
			revision = rp.revision + 1,
			updated_at = now()`, userID, fileID, *req.Completed, req.Restart)
	if err != nil {
		log.Println("Error saving completion:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	if !*req.Completed {
		h.reopenWork(r, userID, fileID)
	}
	st, err := h.load(r, userID, fileID)
	if err != nil {
		log.Println("Error reading saved completion:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Opened records that the caller opened a file (DEC-079): which version is the latest one they
// opened, even if they closed it without moving. It creates the row if there is none, changes
// neither the revision nor the position, and a file only opened is still one nobody has begun.
func (h *ProgressHandler) Opened(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if _, err := h.fileFormat(r, fileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		log.Println("Error checking file for an opening:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO reading_progress AS rp (user_id, file_id, position, last_opened_at, updated_at)
		VALUES ($1, $2, '', now(), now())
		ON CONFLICT (user_id, file_id) DO UPDATE SET last_opened_at = now()`, currentUserID(r), fileID); err != nil {
		log.Println("Error saving an opening:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PutFinishedRequest marks a whole work as finished, or takes the mark off.
type PutFinishedRequest struct {
	Finished *bool `json:"finished"`
}

// SetWorkFinished marks the whole work as finished for the caller (DEC-080): every version leaves
// Continue Reading, and none of them is said to have been read to the end. The person asked for it,
// typically after finishing one version while another was still in progress.
func (h *ProgressHandler) SetWorkFinished(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req PutFinishedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Finished == nil {
		http.Error(w, "finished is true or false", http.StatusBadRequest)
		return
	}
	var exists bool
	if err := h.DB.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM works WHERE id = $1 AND retired_at IS NULL)`, workID).Scan(&exists); err != nil {
		log.Println("Error checking work for the finished mark:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	userID := currentUserID(r)
	var err error
	if *req.Finished {
		_, err = h.DB.ExecContext(r.Context(), `
			INSERT INTO work_reading_state (user_id, work_id) VALUES ($1, $2)
			ON CONFLICT (user_id, work_id) DO UPDATE SET finished_at = now()`, userID, workID)
	} else {
		_, err = h.DB.ExecContext(r.Context(), `DELETE FROM work_reading_state WHERE user_id = $1 AND work_id = $2`, userID, workID)
	}
	if err != nil {
		log.Println("Error saving the finished mark:", err)
		http.Error(w, "Error saving progress", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"finished": *req.Finished})
}
