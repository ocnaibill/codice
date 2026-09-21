package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
	"github.com/ocnaibill/codice/backend/internal/locator"
)

// EquivalenceHandler finds, and records the acceptance of, an equivalent position between two
// files of the same work (RF-042, DEC-030): the same passage in another format, edition or
// language. It is opt-in: nothing here changes a file's own progress, and a suggestion is worked
// out live from the text already indexed (backend/internal/equivalence), never stored ahead of
// time or trusted across a change in either file's text.
type EquivalenceHandler struct {
	DB *sql.DB
}

type fileHandle struct {
	workID       int
	format       string
	availability string
}

// aFile loads what deciding an equivalence needs to know about one file: which work it belongs
// to (so both sides are checked to be of the *same* work), its format (which locator applies)
// and whether it can still be opened.
func (h *EquivalenceHandler) aFile(ctx context.Context, id int64) (fileHandle, error) {
	var f fileHandle
	err := h.DB.QueryRowContext(ctx, `
		SELECT e.work_id, COALESCE(f.format, ''), f.availability
		FROM files f JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id
		WHERE f.id = $1 AND w.retired_at IS NULL`, id).Scan(&f.workID, &f.format, &f.availability)
	return f, err
}

const segmentColumns = `s.id, s.sequence, COALESCE(s.section, ''), s.origin, s.text, s.locator, s.locator_version`

// publishedSegments returns one file's currently published text, in reading order.
func publishedSegments(ctx context.Context, db *sql.DB, fileID int64) ([]equivalence.Segment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+segmentColumns+`
		FROM document_segments s
		JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
		WHERE s.file_id = $1
		ORDER BY s.sequence`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []equivalence.Segment
	for rows.Next() {
		var seg equivalence.Segment
		var origin string
		var loc []byte
		var ver int
		if err := rows.Scan(&seg.ID, &seg.Sequence, &seg.Section, &origin, &seg.Text, &loc, &ver); err != nil {
			return nil, err
		}
		seg.Locator = json.RawMessage(loc)
		seg.Chapter = chapterKey(loc, seg.Section)
		out = append(out, seg)
	}
	return out, rows.Err()
}

// chapterKey is what tells two segments' chapters apart inside one file: an EPUB's chapter (its
// href), or a Markdown segment's heading. A PDF's segments have neither, so its "chapters" are
// always empty and the structural heuristic simply does not apply to it: two pages that happen
// to carry the same heading text are not one chapter, or evidence of anything.
func chapterKey(loc []byte, section string) string {
	var head struct {
		Type string `json:"type"`
		Href string `json:"href"`
	}
	if json.Unmarshal(loc, &head) != nil {
		return ""
	}
	switch head.Type {
	case "epub":
		return head.Href
	case "text":
		return section
	default:
		return ""
	}
}

// chapters groups a file's segments into the chapters they fall in, in the order they first
// appear. A segment with no chapter key (chapterKey returns "") is not part of any chapter.
func chapters(segments []equivalence.Segment) ([]equivalence.Chapter, map[int]int) {
	var list []equivalence.Chapter
	index := map[string]int{}
	segmentChapter := map[int]int{} // segment sequence -> index into list
	for _, s := range segments {
		if s.Chapter == "" {
			continue
		}
		i, ok := index[s.Chapter]
		if !ok {
			list = append(list, equivalence.Chapter{Key: s.Chapter, Title: s.Section, FirstSequence: s.Sequence, Locator: s.Locator, Excerpt: equivalence.Excerpt(s.Text, 200)})
			i = len(list) - 1
			index[s.Chapter] = i
		}
		segmentChapter[s.Sequence] = i
	}
	return list, segmentChapter
}

// nearestSegment is the segment of source that the given locator falls in or nearest to: the
// "trecho ao redor da posição de origem" the specification asks for. Its meaning depends on the
// locator's own kind, so only the formats the locator package addresses with a stored place
// (epub, pdf, text) are handled; the rest have nothing to match against.
func nearestSegment(segments []equivalence.Segment, raw json.RawMessage) *equivalence.Segment {
	var head struct {
		Type        string   `json:"type"`
		Href        string   `json:"href"`
		Progression *float64 `json:"progression"`
		Page        *int     `json:"page"`
		Offset      *int     `json:"offset"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return nil
	}
	switch head.Type {
	case "epub":
		// A file can hold many chapters' worth of text, so the place inside it (the progression a
		// segment starts at) chooses the segment: the last one starting at or before it.
		var best *equivalence.Segment
		for i := range segments {
			var loc struct {
				Href        string   `json:"href"`
				Progression *float64 `json:"progression"`
			}
			if json.Unmarshal(segments[i].Locator, &loc) != nil || loc.Href != head.Href {
				continue
			}
			if best == nil {
				best = &segments[i] // segments are in reading order: the start of the file
			}
			if head.Progression == nil {
				break
			}
			if loc.Progression != nil && *loc.Progression <= *head.Progression {
				best = &segments[i]
			}
		}
		return best
	case "pdf":
		if head.Page == nil {
			return nil
		}
		var best *equivalence.Segment
		bestDist := -1
		for i := range segments {
			var loc struct {
				Page int `json:"page"`
			}
			if json.Unmarshal(segments[i].Locator, &loc) != nil {
				continue
			}
			d := loc.Page - *head.Page
			if d < 0 {
				d = -d
			}
			if bestDist == -1 || d < bestDist {
				best, bestDist = &segments[i], d
				if d == 0 {
					break
				}
			}
		}
		return best
	case "text":
		if head.Offset == nil {
			return nil
		}
		var best *equivalence.Segment
		for i := range segments {
			var loc struct {
				Offset int `json:"offset"`
			}
			if json.Unmarshal(segments[i].Locator, &loc) != nil {
				continue
			}
			if loc.Offset <= *head.Offset {
				best = &segments[i] // segments are ordered: the last one starting at or before it
			}
		}
		if best == nil && len(segments) > 0 {
			best = &segments[0]
		}
		return best
	default:
		return nil
	}
}

// equivalenceOut is one candidate as the client sees it.
type equivalenceOut struct {
	Precision  string          `json:"precision"`
	Confidence string          `json:"confidence"`
	Method     string          `json:"method"`
	Locator    json.RawMessage `json:"locator"`
	Section    string          `json:"section,omitempty"`
	Excerpt    string          `json:"excerpt"`
	Evidence   map[string]any  `json:"evidence"`
}

// Find looks, in the file named by "from" (a version the caller has a position in), for the
// place that answers to the file in the path (the one being opened): status "found" (the first
// candidate is it), "ambiguous" (more than one, the caller chooses) or "not_found" (nothing
// pointed at content actually there, and nothing was made up). It also returns where the search
// started, for the prompt to say "you were at â€¦".
func (h *EquivalenceHandler) Find(w http.ResponseWriter, r *http.Request) {
	destID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	srcID, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil || srcID <= 0 {
		http.Error(w, "from is the file id you are reading now", http.StatusBadRequest)
		return
	}
	if srcID == destID {
		http.Error(w, "from must be a different file", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	dest, err := h.aFile(ctx, destID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	src, err2 := h.aFile(ctx, srcID)
	if err == nil {
		err = err2
	}
	if errors.Is(err, sql.ErrNoRows) || (err == nil && src.workID != dest.workID) {
		http.Error(w, "That is not a file of the same work", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Println("Error reading files for an equivalent position:", err)
		http.Error(w, "Error finding an equivalent position", http.StatusInternalServerError)
		return
	}
	if dest.availability != "available" || src.availability != "available" {
		writeJSON(w, http.StatusOK, map[string]any{"status": equivalence.NotFound})
		return
	}

	userID := currentUserID(r)
	progress, err := h.DB.QueryContext(ctx, `SELECT locator FROM reading_progress WHERE user_id = $1 AND file_id = $2`, userID, srcID)
	var sourceLocator []byte
	if err == nil {
		if progress.Next() {
			progress.Scan(&sourceLocator)
		}
		progress.Close()
		err = progress.Err()
	}
	if err != nil {
		log.Println("Error reading the source position:", err)
		http.Error(w, "Error finding an equivalent position", http.StatusInternalServerError)
		return
	}
	if len(sourceLocator) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": equivalence.NotFound})
		return
	}

	srcSegments, err := publishedSegments(ctx, h.DB, srcID)
	if err == nil {
		var more []equivalence.Segment
		more, err = publishedSegments(ctx, h.DB, destID)
		if err == nil {
			h.respond(w, sourceLocator, srcSegments, more)
			return
		}
	}
	log.Println("Error reading segments for an equivalent position:", err)
	http.Error(w, "Error finding an equivalent position", http.StatusInternalServerError)
}

func (h *EquivalenceHandler) respond(w http.ResponseWriter, sourceLocator json.RawMessage, srcSegments, destSegments []equivalence.Segment) {
	source := nearestSegment(srcSegments, sourceLocator)
	if source == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": equivalence.NotFound})
		return
	}
	srcChapters, srcIndex := chapters(srcSegments)
	destChapters, _ := chapters(destSegments)

	passages := equivalence.ByPassage(*source, destSegments)
	var anchors []equivalence.Candidate
	if len(passages) == 0 {
		anchors = equivalence.ByAnchors(*source, destSegments)
	}
	var chapterCandidate *equivalence.Candidate
	if i, ok := srcIndex[source.Sequence]; ok {
		chapterCandidate = equivalence.ByStructure(i, srcChapters, destChapters)
	}
	answer := equivalence.Combine(passages, anchors, chapterCandidate)

	out := make([]equivalenceOut, len(answer.Candidates))
	for i, c := range answer.Candidates {
		out[i] = equivalenceOut{Precision: c.Precision, Confidence: c.Confidence, Method: c.Method, Locator: c.Locator, Section: c.Section, Excerpt: c.Excerpt, Evidence: c.Evidence}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        answer.Status,
		"candidates":    out,
		"sourceExcerpt": equivalence.Excerpt(source.Text, 200),
		"sourceSection": source.Section,
	})
}

// AcceptRequest is what the caller chose to jump to.
type AcceptRequest struct {
	SourceFileID int64           `json:"sourceFileId"`
	Locator      json.RawMessage `json:"locator"`
	Method       string          `json:"method"`
	Confidence   string          `json:"confidence"`
	Precision    string          `json:"precision"`
}

// Accept records that the caller took a suggested position (for history: nothing here moves any
// progress, which the client saves the ordinary way once it opens the file there). It preserves
// the source: only the destination's own progress will ever change from this.
func (h *EquivalenceHandler) Accept(w http.ResponseWriter, r *http.Request) {
	destID, ok := fileIDParam(r)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req AcceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Locator) == 0 ||
		(req.Method != equivalence.MethodText && req.Method != equivalence.MethodAnchors && req.Method != equivalence.MethodStructure) ||
		(req.Confidence != equivalence.Low && req.Confidence != equivalence.Medium && req.Confidence != equivalence.High) ||
		(req.Precision != equivalence.Passage && req.Precision != equivalence.ChapterOnly) {
		http.Error(w, "invalid acceptance", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	dest, err := h.aFile(ctx, destID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading the destination file:", err)
		http.Error(w, "Error recording the acceptance", http.StatusInternalServerError)
		return
	}
	userID := currentUserID(r)
	var srcID sql.NullInt64
	if req.SourceFileID > 0 {
		srcID = sql.NullInt64{Int64: req.SourceFileID, Valid: true}
	}
	_, err = h.DB.ExecContext(ctx, `
		INSERT INTO equivalent_position_acceptances
			(user_id, work_id, source_file_id, source_locator, source_locator_version,
			 destination_file_id, destination_locator, destination_locator_version, method, confidence, match_precision)
		SELECT $1, $2, $3, rp.locator, rp.locator_version, $4, $5::jsonb, $6, $7, $8, $9
		FROM (SELECT 1) one
		LEFT JOIN reading_progress rp ON rp.user_id = $1 AND rp.file_id = $3`,
		userID, dest.workID, srcID, destID, string(req.Locator), locator.Version, req.Method, req.Confidence, req.Precision)
	if err != nil {
		log.Println("Error recording an equivalent-position acceptance:", err)
		http.Error(w, "Error recording the acceptance", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
