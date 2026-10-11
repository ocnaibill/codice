package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/jobs"
)

// Looking for the cover of a work among the providers that are on (DEC-174). The worker is who asks them and who keeps the pictures, in a folder
// that expires; the page asks for a search, reads what was found and shows the pictures from here. Nothing is put on the work by it: a person
// chooses one, cuts it to 2:3 and sends it as a cover of their own (DEC-173).
const jobCoverSearch = "cover_search"

// candidateFileRe is what the worker names the pictures it keeps: <work>_<n>_<8 hex>.<jpg|png|webp>. Nothing else is served from the folder.
var candidateFileRe = regexp.MustCompile(`^([0-9]{1,10})_[0-9]{1,3}_[0-9a-f]{8}\.(jpg|png|webp)$`)

// CoverCandidate is a cover a provider has for the work.
type CoverCandidate struct {
	File     string `json:"file"`
	Provider string `json:"provider"`
	Source   string `json:"source"`
	Title    string `json:"title"`
	Author   string `json:"author"`
}

// CoverSearch is the last search made for a work.
type CoverSearch struct {
	State   string           `json:"state"`
	Query   string           `json:"query"`
	Results []CoverCandidate `json:"results"`
}

// RequestSearch answers POST /works/{id}/cover/search {"query": "..."} (staff): asks the worker for the covers the providers have for the work,
// by what the work says (title, author, ISBN) or by the text given. Asking again while a search of that work waits is the same request.
func (h *WorkCoverHandler) RequestSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
	}
	query := strings.Join(strings.Fields(req.Query), " ")
	if utf8.RuneCountInString(query) > maxSearchQuery {
		http.Error(w, "A busca tem até 200 caracteres.", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var retired bool
	if err := tx.QueryRowContext(r.Context(), `SELECT retired_at IS NOT NULL FROM works WHERE id = $1`, id).Scan(&retired); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Println("Error reading a work:", err)
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if retired {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}
	actor := currentUserID(r)
	// One statement under a lock, so two requests at once are one job.
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('cover_search'))`); err != nil {
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO work_cover_searches (work_id, query, state, results, requested_at, finished_at)
		VALUES ($1, $2, 'pending', '[]'::jsonb, now(), NULL)
		ON CONFLICT (work_id) DO UPDATE SET query = EXCLUDED.query, state = 'pending', results = '[]'::jsonb, requested_at = now(), finished_at = NULL`,
		id, query); err != nil {
		log.Println("Error keeping a search:", err)
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	var jobID int64
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO jobs (type, payload, priority, created_by)
		SELECT $1::text, jsonb_build_object('work', $2::int, 'query', $3::text), $4::int, NULLIF($5, '')::uuid
		WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE type = $1::text AND state IN ('pending', 'running') AND (payload->>'work')::int = $2)
		RETURNING id`, jobCoverSearch, id, query, jobs.PriorityManual, actor).Scan(&jobID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Println("Error queueing a search:", err)
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if err == nil {
		if err := audit.Record(r.Context(), tx, actor, "work.cover.search", "work", strconv.Itoa(id), map[string]any{"query": query}); err != nil {
			log.Println("Could not audit work.cover.search:", err)
			http.Error(w, "Error asking for the search", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if jobID != 0 {
		jobs.Notify(r.Context(), h.RedisClient, jobID) // a failure only means the workers find it on their next poll
	}
	writeJSON(w, http.StatusAccepted, CoverSearch{State: "pending", Query: query, Results: []CoverCandidate{}})
}

// SearchResult answers GET /works/{id}/cover/search (staff): the last search of the work, or 404 when none was made.
func (h *WorkCoverHandler) SearchResult(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Search not found", http.StatusNotFound)
		return
	}
	var s CoverSearch
	var raw []byte
	err := h.DB.QueryRowContext(r.Context(), `SELECT state, query, results FROM work_cover_searches WHERE work_id = $1`, id).Scan(&s.State, &s.Query, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Search not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading a search:", err)
		http.Error(w, "Error reading the search", http.StatusInternalServerError)
		return
	}
	if err := json.Unmarshal(raw, &s.Results); err != nil || s.Results == nil {
		s.Results = []CoverCandidate{}
	}
	writeJSON(w, http.StatusOK, s)
}

// Candidate answers GET /works/{id}/cover/candidates/{file} (staff): a picture the last search of the work kept. Only what the search of this
// work lists is served, so the folder cannot be read by guessing names, and a picture of another work is not reachable from this one.
func (h *WorkCoverHandler) Candidate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	file := chi.URLParam(r, "file")
	m := candidateFileRe.FindStringSubmatch(file)
	if !ok || m == nil || m[1] != strconv.Itoa(id) || h.SearchDir == "" {
		http.Error(w, "Cover not found", http.StatusNotFound)
		return
	}
	var listed bool
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM work_cover_searches s, jsonb_array_elements(s.results) e
		               WHERE s.work_id = $1 AND s.state = 'done' AND e->>'file' = $2)`, id, file).Scan(&listed)
	if err != nil {
		log.Println("Error reading a search:", err)
		http.Error(w, "Error reading the cover", http.StatusInternalServerError)
		return
	}
	if !listed {
		http.Error(w, "Cover not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(filepath.Join(h.SearchDir, file))
	if err != nil {
		// The folder expires: a search that is a few hours old lists pictures that are gone.
		http.Error(w, "Cover not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "Cover not found", http.StatusNotFound)
		return
	}
	types := map[string]string{".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp"}
	w.Header().Set("Content-Type", types[filepath.Ext(file)])
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, file, info.ModTime(), f)
}
