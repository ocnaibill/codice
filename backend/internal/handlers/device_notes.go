package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/ocnaibill/codice/backend/internal/locator"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// DeviceNotesHandler is what a reading device does with the annotations of its owner (DEC-199): it sends the highlights and notes made on
// the device, deletes them, and asks which ones the Códice has, so that the two are the same. Only the owner's own notes, only for the files
// whose places the Códice knows how to keep (an EPUB first).
type DeviceNotesHandler struct {
	DB *sql.DB
	// Kosync holds the way a place is said in both languages (the xpointer of the device and the CFI of the reader).
	Kosync      *KosyncHandler
	StorageRoot string
}

var deviceRef = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// deviceNoteFile is a file of a work the caller can see, with what is needed to turn places into places.
type deviceNoteFile struct {
	kosyncFile
	WorkID int
}

func (h *DeviceNotesHandler) file(r *http.Request) (deviceNoteFile, int, string) {
	var f deviceNoteFile
	id, err := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if err != nil || id < 1 {
		return f, http.StatusNotFound, "File not found"
	}
	var mode, root, rel sql.NullString
	err = h.DB.QueryRowContext(r.Context(), `
		SELECT f.id, COALESCE(f.format, ''), e.work_id, l.mode, l.root, l.path
		FROM files f
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
		LEFT JOIN LATERAL (SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1) l ON TRUE
		WHERE f.id = $1 AND f.availability = 'available'`, id).Scan(&f.ID, &f.Format, &f.WorkID, &mode, &root, &rel)
	if errors.Is(err, sql.ErrNoRows) {
		return f, http.StatusNotFound, "File not found"
	}
	if err != nil {
		log.Println("Error finding a file for the notes of a device:", err)
		return f, http.StatusInternalServerError, "Error reading the file"
	}
	f.Format = strings.ToLower(f.Format)
	if rel.Valid {
		if full, ok := storage.AbsPath(h.StorageRoot, mode.String, root.String, rel.String); ok {
			f.FullPath = full
		}
	}
	return f, 0, ""
}

type deviceNoteBody struct {
	Pos0     string `json:"pos0"`
	Pos1     string `json:"pos1"`
	Text     string `json:"text"`
	Note     string `json:"note"`
	Drawer   string `json:"drawer"`
	Chapter  string `json:"chapter"`
	Datetime string `json:"datetime"`
	Device   string `json:"device"`
	// ID is the number the note already has in the Códice, when the device received it from there (it was made in the web): the first time the
	// device changes it, it takes that note as its own (it gets the reference) instead of making another.
	ID int `json:"id"`
}

// Put creates the highlight or note a device made, or changes it when the device sends it again (the reference is its identity). The
// color is not the device's to say (it may not even have one): a new note gets the color the Códice starts with, and an edit keeps the one it had.
func (h *DeviceNotesHandler) Put(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	if !deviceRef.MatchString(ref) {
		http.Error(w, "The reference of a note is 8 to 64 letters, digits, - or _", http.StatusBadRequest)
		return
	}
	file, code, msg := h.file(r)
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	if file.Format != "epub" {
		http.Error(w, "The notes of this format are not kept yet", http.StatusUnprocessableEntity)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var req deviceNoteBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	quote, err1 := cleanText(req.Text)
	body, err2 := cleanText(req.Note)
	if err := errors.Join(err1, err2); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	quote = clip(quote, maxQuote)
	if utf8.RuneCountInString(body) > maxBody {
		http.Error(w, fmt.Sprintf("The note is longer than %d characters", maxBody), http.StatusBadRequest)
		return
	}
	if quote == "" && body == "" {
		http.Error(w, "Quote text is required", http.StatusBadRequest)
		return
	}
	if len(req.Pos0) > 600 || len(req.Pos1) > 600 {
		http.Error(w, "A place is too long", http.StatusBadRequest)
		return
	}
	kind := kindMark
	if body != "" {
		kind = kindNote
	}
	// the place: the start of the passage, said the way the Códice keeps it. What does not lead anywhere is kept with no place.
	var loc any
	var ver any
	if req.Pos0 != "" {
		if raw, ok := h.Kosync.locatorOf(file.kosyncFile, req.Pos0); ok {
			if canonical, err := locator.Validate(file.Format, raw); err == nil {
				loc, ver = string(canonical), locator.Version
			}
		}
	}
	var at any
	if t, err := time.Parse(time.RFC3339, req.Datetime); err == nil {
		at = t
	} else if t, err := time.ParseInLocation("2006-01-02 15:04:05", req.Datetime, time.UTC); err == nil {
		at = t
	}
	device := clip(strings.TrimSpace(req.Device), 120)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving the note", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	user := currentUserID(r)
	var id int
	var created bool
	// a note of the Códice that the device is now changing: it becomes the device's (it gets the reference), whoever made it, and keeps its color
	// and tags; nothing is created
	if req.ID > 0 {
		err = tx.QueryRowContext(r.Context(), `
			UPDATE notes SET device_ref = $4, kind = $5, quote = NULLIF($6, ''), body = $7,
				locator = COALESCE($8::jsonb, locator), locator_version = COALESCE($9, locator_version),
				device_pos0 = NULLIF($10, ''), device_pos1 = NULLIF($11, ''), device_name = NULLIF($12, ''), device_at = COALESCE($13, device_at), updated_at = now()
			WHERE id = $1 AND user_id = $2 AND file_id = $3 AND (device_ref IS NULL OR device_ref = $4)
			RETURNING id`, req.ID, user, file.ID, ref, kind, quote, body, loc, ver, req.Pos0, req.Pos1, device, at).Scan(&id)
		if err == nil {
			err = syncNoteLinks(r.Context(), tx, user, int64(id), body)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "created": false})
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			log.Println("Error taking the note of the Códice as the device's:", err)
			http.Error(w, "Error saving the note", http.StatusInternalServerError)
			return
		}
		// there is no such note (it was deleted in the web meanwhile, or it is not this person's): it is made as new, below
		tx.Rollback()
		if tx, err = h.DB.BeginTx(r.Context(), nil); err != nil {
			http.Error(w, "Error saving the note", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
	}
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, locator, locator_version, color, device_ref, device_pos0, device_pos1, device_name, device_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7::jsonb, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14)
		ON CONFLICT (user_id, file_id, device_ref) WHERE device_ref IS NOT NULL DO UPDATE SET
			kind = EXCLUDED.kind, quote = EXCLUDED.quote, body = EXCLUDED.body,
			locator = COALESCE(EXCLUDED.locator, notes.locator), locator_version = COALESCE(EXCLUDED.locator_version, notes.locator_version),
			device_pos0 = EXCLUDED.device_pos0, device_pos1 = EXCLUDED.device_pos1, device_name = EXCLUDED.device_name,
			device_at = COALESCE(EXCLUDED.device_at, notes.device_at), updated_at = now()
		RETURNING id, (xmax = 0)`,
		user, file.WorkID, file.ID, kind, quote, body, loc, ver, defaultColor, ref, req.Pos0, req.Pos1, device, at).Scan(&id, &created)
	if err == nil {
		err = syncNoteLinks(r.Context(), tx, user, int64(id), body)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Println("Error saving the note of a device:", err)
		http.Error(w, "Error saving the note", http.StatusInternalServerError)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"id": id, "created": created})
}

// Delete removes the note a device made and is deleting. Only the one that has the device's reference: a note made in the web, or on
// another device, is never touched by it.
func (h *DeviceNotesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	if !deviceRef.MatchString(ref) {
		http.Error(w, "The reference of a note is 8 to 64 letters, digits, - or _", http.StatusBadRequest)
		return
	}
	file, code, msg := h.file(r)
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	// `?id=` is the number of the note in the Códice, for one the device received from there (made in the web) and is now deleting: the person
	// owns it, and it is what they asked on the device. Without it, only the one that has this device's reference.
	var res sql.Result
	var err error
	if id, perr := strconv.Atoi(r.URL.Query().Get("id")); perr == nil && id > 0 {
		res, err = h.DB.ExecContext(r.Context(), `DELETE FROM notes WHERE user_id = $1 AND file_id = $2 AND id = $3 AND kind <> 'bookmark'`, currentUserID(r), file.ID, id)
	} else {
		res, err = h.DB.ExecContext(r.Context(), `DELETE FROM notes WHERE user_id = $1 AND file_id = $2 AND device_ref = $3`, currentUserID(r), file.ID, ref)
	}
	if err != nil {
		log.Println("Error deleting the note of a device:", err)
		http.Error(w, "Error deleting the note", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deviceNoteOut struct {
	ID        int    `json:"id"`
	Ref       string `json:"ref,omitempty"`
	Kind      string `json:"kind"`
	Quote     string `json:"quote"`
	Body      string `json:"body"`
	Color     string `json:"color"`
	UpdatedAt string `json:"updatedAt"`
	// Pos0 and Pos1 are the exact start and end the device gave (when the note came from a device); Place is the paragraph the note is in, said the
	// way a device understands, for a note made in the web: the device finds the passage in it by its text.
	Pos0  string `json:"pos0,omitempty"`
	Pos1  string `json:"pos1,omitempty"`
	Place string `json:"place,omitempty"`
	// FromDevice: the note was made on a reading device (it came with a reference).
	FromDevice bool `json:"fromDevice"`
}

// List says the highlights and notes the owner has in a file, as a device understands them, and which of the ones it already knows (`known`:
// the numbers it was given) are gone. With these two it makes the device's own list the same as the Códice's.
func (h *DeviceNotesHandler) List(w http.ResponseWriter, r *http.Request) {
	file, code, msg := h.file(r)
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	user := currentUserID(r)
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, COALESCE(device_ref, ''), kind, COALESCE(quote, ''), body, color, updated_at, COALESCE(device_pos0, ''), COALESCE(device_pos1, ''), COALESCE(locator::text, '')
		FROM notes WHERE user_id = $1 AND file_id = $2 AND kind <> 'bookmark' ORDER BY id`, user, file.ID)
	if err != nil {
		log.Println("Error listing the notes for a device:", err)
		http.Error(w, "Error reading the notes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []deviceNoteOut{}
	present := map[int]bool{}
	for rows.Next() {
		var n deviceNoteOut
		var updated time.Time
		var loc string
		if err := rows.Scan(&n.ID, &n.Ref, &n.Kind, &n.Quote, &n.Body, &n.Color, &updated, &n.Pos0, &n.Pos1, &loc); err != nil {
			continue
		}
		n.UpdatedAt = updated.UTC().Format(time.RFC3339)
		n.FromDevice = n.Ref != ""
		if n.Pos0 == "" && loc != "" {
			if place, ok := h.Kosync.placeOf(file.kosyncFile, []byte(loc)); ok {
				n.Place = place
			}
		}
		present[n.ID] = true
		out = append(out, n)
	}
	gone := []int{}
	for _, part := range strings.Split(r.URL.Query().Get("known"), ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && id > 0 && !present[id] {
			gone = append(gone, id)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"notes": out, "gone": gone, "format": file.Format})
}
