package handlers

import (
	"database/sql"
	"encoding/json"
	"github.com/ocnaibill/codice/backend/internal/people"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SearchHandler searches the text of the files (RF-018). What it searches is the published text of
// files that can still be opened, of works that are not retired: the text of the catalog, which every
// signed-in person may read, exactly as they may read the files themselves. The notes of a person are
// searched elsewhere, with their owner's scope (GET /notes?q=).
type SearchHandler struct {
	DB *sql.DB
}

// SearchHit is one passage of one file that matches, with where it is.
type SearchHit struct {
	SegmentID  int64  `json:"segmentId"`
	WorkID     int    `json:"workId"`
	WorkTitle  string `json:"workTitle"`
	WorkAuthor string `json:"workAuthor"`
	FileID     int64  `json:"fileId"`
	Format     string `json:"format,omitempty"`
	Language   string `json:"language,omitempty"`
	Section    string `json:"section,omitempty"`
	Origin     string `json:"origin"`
	// Snippet is a piece of the text as it was extracted, accents and all; Matches are the
	// [start, end) positions of the words found in it, counted in characters, for the client to mark.
	Snippet        string          `json:"snippet"`
	Matches        [][2]int        `json:"matches"`
	Locator        json.RawMessage `json:"locator"`
	LocatorVersion int             `json:"locatorVersion"`
	Rank           float64         `json:"rank"`
}

const (
	maxSearchLen  = 200
	searchDefault = 20
	searchMax     = 50
	hitOpen       = '\x02' // where ts_headline marks the start and the end of a match: characters that
	hitClose      = '\x03' // extraction removes from the text, so they cannot be confused with it
)

// What a search can see: the published generation of a file that can still be opened, of a work that is not retired.
const visibleSegments = `
			JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
			JOIN files f ON f.id = s.file_id AND f.availability = 'available'
			JOIN editions e ON e.id = f.edition_id
			JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL`

// The passages that match by the stem of the words, in the language each was indexed with, and the ones that
// match the words as they are written. Each one asks the index of its own vector.
const (
	stemHits = `
			SELECT s.id, w.id AS work_id, f.id AS file_id, s.sequence, q.cfg, ts_rank_cd(s.tsv_stem, q.tsq) AS rank
			FROM q JOIN document_segments s ON s.search_config = q.cfg AND s.tsv_stem @@ q.tsq {visible}
			WHERE TRUE{where}`
	exactHits = `
			SELECT s.id, w.id AS work_id, f.id AS file_id, s.sequence, 'codice_simple'::regconfig AS cfg, ts_rank_cd(s.tsv, ex.tsq) AS rank
			FROM ex JOIN document_segments s ON s.tsv @@ ex.tsq {visible}
			WHERE TRUE{where}`
)

// Search finds passages by the words asked for. The words are read like a web search: quotes make a
// phrase, a minus excludes a word, "or" allows either. Case and accents do not matter.
//
// Without quotes the search also finds the other forms of a word ("correr" finds "corrida", "run" finds
// "running"), by the stem of each word in the language of the edition it is in (#41). With quotes, the
// whole search is exact: the words as written, which is what a phrase is for. The exact search is also
// always part of the other, so the stemming adds hits and never takes one away, and a file whose language
// is not known is searched as it always was.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "q is what to search for", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(q) > maxSearchLen {
		http.Error(w, "The search is too long", http.StatusBadRequest)
		return
	}
	limit, offset := searchDefault, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= searchMax {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}

	args := []any{q}
	where := ""
	for _, f := range []struct{ param, column string }{{"workId", "w.id"}, {"fileId", "f.id"}} {
		if v := r.URL.Query().Get(f.param); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				http.Error(w, f.param+" is a number", http.StatusBadRequest)
				return
			}
			args = append(args, n)
			where += " AND " + f.column + " = $" + strconv.Itoa(len(args))
		}
	}
	// One more than asked for tells whether there is another page, without counting them all.
	args = append(args, limit+1, offset)
	limitArg, offsetArg := "$"+strconv.Itoa(len(args)-1), "$"+strconv.Itoa(len(args))

	// Only the published generation of a file is searched; the headline is made after the page is cut,
	// because it is the expensive part. A passage that matches both ways is one hit, by the better rank.
	exact := strings.Contains(q, `"`)
	matching := exactHits
	if !exact {
		matching = stemHits + " UNION ALL " + exactHits
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		WITH q AS (
			SELECT c.oid::regconfig AS cfg, websearch_to_tsquery(c.oid::regconfig, $1) AS tsq
			FROM pg_ts_config c WHERE c.cfgname LIKE 'codice\_%' AND c.cfgname <> 'codice_simple'
		),
		ex AS (SELECT websearch_to_tsquery('codice_simple', $1) AS tsq),
		hits AS (`+strings.ReplaceAll(strings.ReplaceAll(matching, "{where}", where), "{visible}", visibleSegments)+`),
		best AS (
			SELECT DISTINCT ON (id) id, work_id, file_id, sequence, cfg, rank FROM hits ORDER BY id, rank DESC
		),
		page AS (
			SELECT b.id, b.work_id, b.file_id, b.sequence, b.cfg, b.rank FROM best b
			ORDER BY b.rank DESC, b.work_id, b.file_id, b.sequence
			LIMIT `+limitArg+` OFFSET `+offsetArg+`
		)
		SELECT s.id, w.id, w.original_title, `+authorLabelFor(people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective)+`, f.id,
		       COALESCE(f.format, ''), COALESCE(e.language, ''), COALESCE(s.section, ''), s.origin,
		       ts_headline(page.cfg, s.text, websearch_to_tsquery(page.cfg, $1),
		         'StartSel=' || chr(2) || ', StopSel=' || chr(3) || ', MaxFragments=1, MaxWords=40, MinWords=18, ShortWord=2'),
		       s.locator, s.locator_version, page.rank
		FROM page
		JOIN document_segments s ON s.id = page.id
		JOIN files f ON f.id = page.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = page.work_id
		LEFT JOIN LATERAL (
			SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name) AS names,
			       string_agg(CASE WHEN p.family_name IS NULL THEN p.name ELSE p.family_name || COALESCE(', ' || p.given_name, '') END,
			                  '; ' ORDER BY c.position, p.name) AS names_family
			FROM work_contributors c JOIN person p ON p.id = c.person_id
			WHERE c.work_id = w.id AND c.role = 'author'
		) au ON TRUE
		ORDER BY page.rank DESC, page.work_id, page.file_id, page.sequence`, args...)
	if err != nil {
		log.Println("Error searching:", err)
		http.Error(w, "Error searching", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	hits := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var marked string
		var locator []byte
		if err := rows.Scan(&h.SegmentID, &h.WorkID, &h.WorkTitle, &h.WorkAuthor, &h.FileID, &h.Format, &h.Language,
			&h.Section, &h.Origin, &marked, &locator, &h.LocatorVersion, &h.Rank); err != nil {
			log.Println("Error reading a search hit:", err)
			http.Error(w, "Error searching", http.StatusInternalServerError)
			return
		}
		h.Locator = json.RawMessage(locator)
		h.Snippet, h.Matches = unmark(marked)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		log.Println("Error searching:", err)
		http.Error(w, "Error searching", http.StatusInternalServerError)
		return
	}
	more := len(hits) > limit
	if more {
		hits = hits[:limit]
	}
	mode := "stem"
	if exact {
		mode = "exact"
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": hits, "hasMore": more, "query": q, "mode": mode})
}

// unmark takes the marks ts_headline put around the words found off the text, and says where they
// were, in characters, in the text that is left.
func unmark(marked string) (string, [][2]int) {
	var plain strings.Builder
	matches := [][2]int{}
	position, start := 0, -1
	for _, r := range marked {
		switch r {
		case hitOpen:
			start = position
		case hitClose:
			if start >= 0 {
				matches = append(matches, [2]int{start, position})
				start = -1
			}
		default:
			plain.WriteRune(r)
			position++
		}
	}
	return plain.String(), matches
}
