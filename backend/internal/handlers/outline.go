package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
)

// OutlineHandler answers with the table of contents of a file as the text index has it (DEC-087) and where the caller is in it (DEC-150).
type OutlineHandler struct {
	DB *sql.DB
}

// OutlineChapter is one entry of the outline of a file.
type OutlineChapter struct {
	Title string `json:"title"`
	Depth int    `json:"depth"`
	// Part is where in the book it is: "front", "body" or "back".
	Part string `json:"part"`
	// HasChildren: it is a part or a group of chapters, not a chapter.
	HasChildren bool `json:"hasChildren"`
	// Locator is where the reader opens to go there: the first text it has, or the first of what is under it. Null for an entry that holds no text.
	Locator json.RawMessage `json:"locator"`
	// Percent is how far through the text of the file it starts, from 0 to 100, counted in characters.
	Percent float64 `json:"percent"`
}

// OutlineResponse is the outline of a file and the entry the caller's place is in.
type OutlineResponse struct {
	FileID   int64            `json:"fileId"`
	Chapters []OutlineChapter `json:"chapters"`
	// Current is the index into Chapters of the entry that holds the caller's saved place, or null when they have none or it is not
	// in any entry (before the first one, or in a file that has no outline).
	Current *int `json:"current"`
}

// Get returns the outline of a file the caller can read. A file with no text index, or whose index has no outline, has an empty list: no
// structure is not made up.
func (h *OutlineHandler) Get(w http.ResponseWriter, r *http.Request) {
	fileID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	var found bool
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM files f JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id
		               WHERE f.id = $1 AND w.retired_at IS NULL)`, fileID).Scan(&found); err != nil {
		log.Println("Error checking file for outline:", err)
		http.Error(w, "Error reading the outline", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	out := OutlineResponse{FileID: fileID, Chapters: []OutlineChapter{}}
	nodes, segments, chars, err := loadOutlineIndex(r.Context(), h.DB, fileID)
	if err != nil {
		log.Println("Error reading the outline:", err)
		http.Error(w, "Error reading the outline", http.StatusInternalServerError)
		return
	}
	if len(nodes) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}

	out.Chapters = outlineOf(nodes, segments, chars)
	// Where the caller is: the segment nearest to their saved place, and the entry it is in.
	var saved []byte
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT locator FROM reading_progress WHERE user_id = $1 AND file_id = $2 AND locator IS NOT NULL`,
		currentUserID(r), fileID).Scan(&saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Println("Error reading the place for the outline:", err)
		http.Error(w, "Error reading the outline", http.StatusInternalServerError)
		return
	}
	if len(saved) > 0 {
		if seg := nearestSegment(segments, json.RawMessage(saved)); seg != nil && seg.Node >= 0 {
			current := seg.Node
			out.Current = &current
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// loadOutlineIndex reads what the text index has of a file's outline: its nodes, and for each segment, in reading order, the node it is in, where it
// opens and how many characters it holds (the text itself is left where it is). A file with no index or no outline has no nodes.
func loadOutlineIndex(ctx context.Context, db *sql.DB, fileID int64) ([]equivalence.Node, []equivalence.Segment, []int, error) {
	var structure []byte
	err := db.QueryRowContext(ctx, `SELECT structure FROM text_extractions WHERE file_id = $1`, fileID).Scan(&structure)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, err
	}
	var nodes []equivalence.Node
	if len(structure) > 0 && json.Unmarshal(structure, &nodes) != nil {
		nodes = nil // an outline that cannot be read is no outline
	}
	if len(nodes) == 0 {
		return nil, nil, nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT s.sequence, s.node, s.locator, length(s.text)
		FROM document_segments s
		JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1
		ORDER BY s.sequence`, fileID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	var segments []equivalence.Segment
	var chars []int
	for rows.Next() {
		var seg equivalence.Segment
		var node sql.NullInt64
		var loc []byte
		var length int
		if err := rows.Scan(&seg.Sequence, &node, &loc, &length); err != nil {
			return nil, nil, nil, err
		}
		seg.Locator = json.RawMessage(loc)
		seg.Node = equivalence.NoNode
		if node.Valid && int(node.Int64) < len(nodes) {
			seg.Node = int(node.Int64)
		}
		segments = append(segments, seg)
		chars = append(chars, length)
	}
	return nodes, segments, chars, rows.Err()
}

// outlineOf puts the nodes of an outline in the form the page shows: whether each is a group, where it opens and how far through
// the text it starts. The segments are in reading order, and chars says how many characters each holds.
func outlineOf(nodes []equivalence.Node, segments []equivalence.Segment, chars []int) []OutlineChapter {
	first := make(map[int]equivalence.Segment) // the first segment of each node
	for _, seg := range segments {
		if seg.Node < 0 {
			continue
		}
		if _, ok := first[seg.Node]; !ok {
			first[seg.Node] = seg
		}
	}
	// The characters before each node, to say how far through the text it starts.
	total := 0
	for _, n := range chars {
		total += n
	}
	before := make(map[int]int)
	running := 0
	seen := make(map[int]bool)
	for i, seg := range segments {
		if seg.Node >= 0 && !seen[seg.Node] {
			seen[seg.Node] = true
			before[seg.Node] = running
		}
		running += chars[i]
	}
	chapters := make([]OutlineChapter, len(nodes))
	for i, n := range nodes {
		c := OutlineChapter{Title: n.Title, Depth: n.Depth, Part: n.Part}
		if i+1 < len(nodes) && nodes[i+1].Depth > n.Depth {
			c.HasChildren = true
		}
		// An entry opens at its own first text, or at the first of what is under it (a part opens at its first chapter).
		best, have := first[i]
		start := i
		if !have {
			for k := i + 1; k < len(nodes) && nodes[k].Depth > n.Depth; k++ {
				if s, ok := first[k]; ok && (!have || s.Sequence < best.Sequence) {
					best, have, start = s, true, k
				}
			}
		}
		if have {
			c.Locator = best.Locator
			if total > 0 {
				c.Percent = float64(int(float64(before[start])/float64(total)*1000+0.5)) / 10
			}
		}
		chapters[i] = c
	}
	return chapters
}
