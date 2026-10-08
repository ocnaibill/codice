package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// SeriesStep is a work of a series as a button to it names it: the one to read next, or to go on with (#187). The label the
// screen shows ("Vol. 3", "Cap. 27,5") comes from its unit and its position in the collection.
type SeriesStep struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Unit     string   `json:"unit"`
	Position *float64 `json:"position"`
	// Started: the caller has begun this work and has not finished it.
	Started bool `json:"started"`
	// Begun: the caller has read something of the series (this step or another); it is what tells "start" from "go on".
	Begun bool `json:"begun,omitempty"`
}

// seriesEntry is a work of the official collection of a series, in the order of the collection, with what the caller did of it.
type seriesEntry struct {
	SeriesStep
	Done   bool
	LastAt *time.Time
}

// unitOrder is the order the groups of a collection are shown in, and so the order "go on" tries them when the caller has not
// read anything yet: volumes, chapters, one-shots, and the works with no unit last.
var unitOrder = []string{"volume", "chapter", "oneshot", ""}

// loadSeries reads the works of an official collection the caller can open, in the order of the collection, with the caller's
// state on each: finished (the work marked as finished, or every version they began read to the end) or begun. A work in the
// trash, or with no file that is on the disk, is not a step.
func loadSeries(db *sql.DB, collectionID int64, userID string) ([]seriesEntry, error) {
	rows, err := db.Query(`
		SELECT w.id, COALESCE(w.original_title, ''), COALESCE(w.unit, ''), cw.position,
		       (wrs.work_id IS NOT NULL OR (rp.n > 0 AND NOT rp.open)), (rp.n > 0 AND rp.open AND wrs.work_id IS NULL), rp.last_at
		FROM collection_works cw
		JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
		LEFT JOIN work_reading_state wrs ON wrs.work_id = w.id AND wrs.user_id = $1::uuid
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS n, COALESCE(bool_or(r.completed_at IS NULL), FALSE) AS open,
			       MAX(GREATEST(COALESCE(r.last_opened_at, '-infinity'::timestamptz), r.updated_at)) AS last_at
			FROM reading_progress r
			JOIN files f ON f.id = r.file_id AND f.availability = 'available'
			JOIN editions e ON e.id = f.edition_id
			WHERE e.work_id = w.id AND r.user_id = $1::uuid AND `+hasPosition("r")+`
		) rp ON TRUE
		WHERE cw.collection_id = $2 AND cw.official
		  AND EXISTS (SELECT 1 FROM files f JOIN editions e ON e.id = f.edition_id WHERE e.work_id = w.id AND f.availability = 'available')
		ORDER BY cw.position NULLS LAST, COALESCE(w.original_title, cw.label), cw.id`, userID, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []seriesEntry
	for rows.Next() {
		var e seriesEntry
		var lastAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.Title, &e.Unit, &e.Position, &e.Done, &e.Started, &lastAt); err != nil {
			return nil, err
		}
		if lastAt.Valid {
			e.LastAt = &lastAt.Time
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// nextInSeries is the work that follows workID in its own group (the same unit), in the order of the collection: the next
// chapter of a chapter, the next volume of a volume. The last of a group has none, and a work that is not in the list neither.
func nextInSeries(entries []seriesEntry, workID int) *SeriesStep {
	for i, e := range entries {
		if e.ID != workID {
			continue
		}
		for _, n := range entries[i+1:] {
			if n.Unit == e.Unit {
				step := n.SeriesStep
				step.Started = n.Started
				return &step
			}
		}
		return nil
	}
	return nil
}

// continueSeries is the work to go on with: in the group of what the caller read last (or, if they read nothing, in the
// first group that has something left), the one they have begun, else the first they have not finished. When everything
// is finished it is nil. Begun says whether the caller has read anything of the series at all.
func continueSeries(entries []seriesEntry) *SeriesStep {
	var last *seriesEntry
	begun := false
	for i := range entries {
		e := &entries[i]
		if e.Done || e.Started {
			begun = true
		}
		if e.LastAt != nil && (last == nil || e.LastAt.After(*last.LastAt)) {
			last = e
		}
	}
	groups := unitOrder
	if last != nil {
		groups = append([]string{last.Unit}, unitOrder...)
	}
	for _, unit := range groups {
		var first *seriesEntry
		for i := range entries {
			e := &entries[i]
			if e.Unit != unit || e.Done {
				continue
			}
			if e.Started {
				first = e
				break
			}
			if first == nil {
				first = e
			}
		}
		if first != nil {
			step := first.SeriesStep
			step.Begun = begun
			return &step
		}
	}
	return nil
}

// WorkSeries says where a work goes on in its series (#187): the official collection it belongs to and the work that follows
// it in its group, for the reader to offer. A work that is in no collection has neither.
func (h *CollectionsHandler) WorkSeries(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	var collectionID int64
	var name string
	err = h.DB.QueryRow(`
		SELECT c.id, c.name
		FROM collection_works cw
		JOIN collections c ON c.id = cw.collection_id AND c.kind = 'official' AND c.retired_at IS NULL
		WHERE cw.work_id = $1 AND cw.official`, id).Scan(&collectionID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"collection":null,"next":null}`))
		return
	}
	if err != nil {
		log.Println("Error finding the collection of a work:", err)
		http.Error(w, "Error fetching the series", http.StatusInternalServerError)
		return
	}
	entries, err := loadSeries(h.DB, collectionID, userID)
	if err != nil {
		log.Println("Error reading the series of a work:", err)
		http.Error(w, "Error fetching the series", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"collection": map[string]any{"id": collectionID, "name": name},
		"next":       nextInSeries(entries, id),
	})
}
