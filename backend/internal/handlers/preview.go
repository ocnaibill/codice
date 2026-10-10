package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
)

// previewDefault and previewMax are how many segments of the text index a preview shows at once.
const (
	previewDefault = 6
	previewMax     = 12
)

// PreviewHandler answers with a few passages of a file in a row, from the text index (DEC-153): the page of a work shows them as a plain preview of
// where the person is, without opening the reader.
type PreviewHandler struct {
	DB *sql.DB
}

// PreviewSegment is one passage of the text index.
type PreviewSegment struct {
	Sequence int    `json:"sequence"`
	Text     string `json:"text"`
	// Chapter is the title of the entry of the outline that the passage is in; empty when the file has no outline.
	Chapter string `json:"chapter"`
	// Part is "front", "body" or "back" with an outline, and empty without one.
	Part    string          `json:"part"`
	Locator json.RawMessage `json:"locator"`
}

// PreviewResponse is a window of the passages of a file, and how to move to the ones before and after it.
type PreviewResponse struct {
	FileID int64 `json:"fileId"`
	// Total is how many passages the file has in the index.
	Total    int              `json:"total"`
	Segments []PreviewSegment `json:"segments"`
	// From is the sequence the window starts at. PrevFrom and NextFrom are where the windows before and after start, null at the ends.
	From     int  `json:"from"`
	PrevFrom *int `json:"prevFrom"`
	NextFrom *int `json:"nextFrom"`
	// Current is the sequence of the passage the caller's saved place is in, or null when they have none.
	Current *int `json:"current"`
}

// Get returns a window of the file's passages. With no `from`, it is the one that holds the caller's saved place (or the start of the file when
// they have none); `count` is how many passages (6 to 12). A file with no text index has an empty window.
func (h *PreviewHandler) Get(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	var found bool
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM files f JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id
		               WHERE f.id = $1 AND w.retired_at IS NULL)`, fileID).Scan(&found); err != nil {
		log.Println("Error checking file for preview:", err)
		http.Error(w, "Error reading the preview", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	count := previewDefault
	if v, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && v > 0 {
		count = min(v, previewMax)
	}
	from := -1
	if raw := r.URL.Query().Get("from"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			http.Error(w, "from is a position from 0", http.StatusBadRequest)
			return
		}
		from = v
	}

	out := PreviewResponse{FileID: fileID, Segments: []PreviewSegment{}}
	var total int
	var last sql.NullInt64
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT count(*), max(s.sequence)
		FROM document_segments s JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1`, fileID).Scan(&total, &last); err != nil {
		log.Println("Error counting the preview:", err)
		http.Error(w, "Error reading the preview", http.StatusInternalServerError)
		return
	}
	out.Total = total
	if total == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}

	// Where the caller is: the segment nearest to their saved place.
	var saved []byte
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT locator FROM reading_progress WHERE user_id = $1 AND file_id = $2 AND locator IS NOT NULL`,
		currentUserID(r), fileID).Scan(&saved); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Println("Error reading the place for the preview:", err)
		http.Error(w, "Error reading the preview", http.StatusInternalServerError)
		return
	}
	if len(saved) > 0 {
		places, err := loadPlaces(r, h.DB, fileID)
		if err != nil {
			log.Println("Error reading the places for the preview:", err)
			http.Error(w, "Error reading the preview", http.StatusInternalServerError)
			return
		}
		if seg := nearestSegment(places, json.RawMessage(saved)); seg != nil {
			cur := seg.Sequence
			out.Current = &cur
		}
	}
	if from < 0 {
		from = 0
		if out.Current != nil {
			// the window that holds the place, with the place near its start (a passage of lead-in)
			from = max(*out.Current-1, 0)
		}
	}
	out.From = from

	var nodes []equivalence.Node
	var structure []byte
	if err := h.DB.QueryRowContext(r.Context(), `SELECT structure FROM text_extractions WHERE file_id = $1`, fileID).Scan(&structure); err == nil && len(structure) > 0 {
		if json.Unmarshal(structure, &nodes) != nil {
			nodes = nil
		}
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT s.sequence, s.text, s.node, s.locator
		FROM document_segments s JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1 AND s.sequence >= $2
		ORDER BY s.sequence LIMIT $3`, fileID, from, count)
	if err != nil {
		log.Println("Error reading the preview:", err)
		http.Error(w, "Error reading the preview", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var seg PreviewSegment
		var node sql.NullInt64
		var loc []byte
		if err := rows.Scan(&seg.Sequence, &seg.Text, &node, &loc); err != nil {
			log.Println("Error reading a passage of the preview:", err)
			http.Error(w, "Error reading the preview", http.StatusInternalServerError)
			return
		}
		seg.Locator = json.RawMessage(loc)
		if node.Valid && int(node.Int64) < len(nodes) {
			seg.Chapter = nodes[node.Int64].Title
			seg.Part = nodes[node.Int64].Part
		}
		out.Segments = append(out.Segments, seg)
	}
	if err := rows.Err(); err != nil {
		log.Println("Error reading the preview:", err)
		http.Error(w, "Error reading the preview", http.StatusInternalServerError)
		return
	}
	if from > 0 {
		prev := max(from-count, 0)
		out.PrevFrom = &prev
	}
	if n := len(out.Segments); n > 0 && last.Valid && out.Segments[n-1].Sequence < int(last.Int64) {
		next := out.Segments[n-1].Sequence + 1
		out.NextFrom = &next
	}
	writeJSON(w, http.StatusOK, out)
}

// loadPlaces reads where each passage of a file is, with no text: its sequence and where it opens, in reading order.
func loadPlaces(r *http.Request, db *sql.DB, fileID int64) ([]equivalence.Segment, error) {
	rows, err := db.QueryContext(r.Context(), `
		SELECT s.sequence, s.locator
		FROM document_segments s JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1 ORDER BY s.sequence`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []equivalence.Segment
	for rows.Next() {
		var seg equivalence.Segment
		var loc []byte
		if err := rows.Scan(&seg.Sequence, &loc); err != nil {
			return nil, err
		}
		seg.Locator = json.RawMessage(loc)
		out = append(out, seg)
	}
	return out, rows.Err()
}
