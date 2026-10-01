package handlers

import (
	"log"
	"net/http"
	"strings"
)

// maxQueuedWorks is how many works the queue of suggestions lists: it is a queue to work through, and the
// next ones show up as these are decided.
var maxQueuedWorks = 200

// QueuedWork is a work with suggestions waiting for a decision.
type QueuedWork struct {
	WorkID  int      `json:"workId"`
	Title   string   `json:"title"`
	Author  string   `json:"author"`
	Pending int      `json:"pending"`
	Fields  []string `json:"fields"`
}

// SuggestionQueue lists the works that have suggestions nobody decided yet, the ones waiting the longest first
// (owner and admin, #70). A retired work has none to decide: it is not in the library.
func (h *LibraryHandler) SuggestionQueue(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT w.id, w.original_title, COALESCE(a.name, ''), count(*), string_agg(DISTINCT c.field, ',' ORDER BY c.field)
		FROM metadata_candidates c
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		LEFT JOIN LATERAL (`+firstAuthorSQL+`) a ON TRUE
		WHERE c.state = 'pending'
		GROUP BY w.id, w.original_title, a.name
		ORDER BY min(c.created_at), w.id
		LIMIT $1`, maxQueuedWorks)
	if err != nil {
		log.Println("Error listing the queue of suggestions:", err)
		http.Error(w, "Error listing suggestions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []QueuedWork{}
	for rows.Next() {
		var q QueuedWork
		var fields string
		if err := rows.Scan(&q.WorkID, &q.Title, &q.Author, &q.Pending, &fields); err != nil {
			log.Println("Error reading the queue of suggestions:", err)
			http.Error(w, "Error reading suggestions", http.StatusInternalServerError)
			return
		}
		q.Fields = strings.Split(fields, ",")
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Error reading suggestions", http.StatusInternalServerError)
		return
	}
	var total int
	if err := h.DB.QueryRow(`
		SELECT count(DISTINCT c.work_id) FROM metadata_candidates c
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL WHERE c.state = 'pending'`).Scan(&total); err != nil {
		http.Error(w, "Error counting suggestions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": total})
}
