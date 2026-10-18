package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
)

// PlaceView is one of the two places of a conflict, as the screen that asks which to keep draws it: how far, when, who, and where in words.
// What the server cannot say of a place stays empty (a file with no text index has no chapter and no passage; an EPUB has no page).
type PlaceView struct {
	Percent float64   `json:"percent"`
	At      time.Time `json:"at"`
	Device  string    `json:"device"`
	Chapter string    `json:"chapter,omitempty"`
	Snippet string    `json:"snippet,omitempty"`
	// Page is from 1, only for a PDF or a comic (an EPUB has no fixed pages).
	Page int `json:"page,omitempty"`
}

// PlaceComparison is the place the web has and the one a device sent that was kept apart (DEC-191).
type PlaceComparison struct {
	FileID int64      `json:"fileId"`
	Format string     `json:"format"`
	Web    *PlaceView `json:"web"`
	Device *PlaceView `json:"device"`
	// Notes is how many notes the caller has in this file.
	Notes int `json:"notes"`
	// Gap is how far apart the two are, in percentage points (always positive), and in pages when both are pages of the same kind.
	GapPercent float64 `json:"gapPercent"`
	GapPages   int     `json:"gapPages,omitempty"`
}

const snippetRunes = 220

// Compare says where the web is and where the device that was kept apart is, each in words, so that the person chooses with both in front.
func (h *ProgressHandler) Compare(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	format, err := h.fileFormat(r, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		log.Println("Error checking file to compare places:", err)
		http.Error(w, "Error reading progress", http.StatusInternalServerError)
		return
	}
	st, err := h.load(r, currentUserID(r), fileID)
	if err != nil {
		log.Println("Error reading progress to compare places:", err)
		http.Error(w, "Error reading progress", http.StatusInternalServerError)
		return
	}
	out := PlaceComparison{FileID: fileID, Format: format}
	nodes, segments, _, _ := loadOutlineIndex(r.Context(), h.DB, fileID)
	if st.Locator != nil && st.UpdatedAt != nil {
		v := placeViewOf(r.Context(), h.DB, fileID, nodes, segments, st.Locator)
		v.Percent, v.At, v.Device = st.Percent, *st.UpdatedAt, st.Device
		out.Web = &v
	}
	if st.Pending != nil {
		v := placeViewOf(r.Context(), h.DB, fileID, nodes, segments, st.Pending.Locator)
		v.Percent, v.At, v.Device = st.Pending.Percent, st.Pending.At, st.Pending.Device
		out.Device = &v
	}
	if out.Web != nil && out.Device != nil {
		gap := out.Device.Percent - out.Web.Percent
		if gap < 0 {
			gap = -gap
		}
		out.GapPercent = gap
		if out.Web.Page > 0 && out.Device.Page > 0 {
			pages := out.Device.Page - out.Web.Page
			if pages < 0 {
				pages = -pages
			}
			out.GapPages = pages
		}
	}
	_ = h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM notes WHERE user_id = $1 AND file_id = $2`, currentUserID(r), fileID).Scan(&out.Notes)
	writeJSON(w, http.StatusOK, out)
}

// placeViewOf says in words where a locator is: the chapter of the outline and the passage of the text index nearest to it, and the page
// of a PDF or a comic.
func placeViewOf(ctx context.Context, db *sql.DB, fileID int64, nodes []equivalence.Node, segments []equivalence.Segment, raw json.RawMessage) PlaceView {
	var v PlaceView
	var head struct {
		Type  string `json:"type"`
		Page  *int   `json:"page"`
		Index *int   `json:"index"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return v
	}
	switch head.Type {
	case "pdf":
		if head.Page != nil {
			v.Page = *head.Page + 1
		}
	case "image":
		if head.Index != nil {
			v.Page = *head.Index + 1
		}
	}
	if len(segments) == 0 {
		return v
	}
	seg := nearestSegment(segments, raw)
	if seg == nil {
		return v
	}
	if seg.Node >= 0 && seg.Node < len(nodes) {
		v.Chapter = strings.TrimSpace(nodes[seg.Node].Title)
	}
	var text string
	if err := db.QueryRowContext(ctx, `
		SELECT s.text FROM document_segments s
		JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1 AND s.sequence = $2`, fileID, seg.Sequence).Scan(&text); err == nil {
		v.Snippet = shortPassage(text, snippetRunes)
	}
	return v
}

// shortPassage is a passage in one line, cut at a word with an ellipsis when it is long.
func shortPassage(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	cut := []rune(s)[:limit]
	out := string(cut)
	if i := strings.LastIndex(out, " "); i > limit/2 {
		out = out[:i]
	}
	return strings.TrimRight(out, " ,;:.") + "…"
}
