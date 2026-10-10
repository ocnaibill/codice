package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// Looking a person up on Wikidata by name, for staff to choose who the author is (DEC-168). The worker is who asks the provider; the page asks
// for a search, reads the candidates and says which one is the person. Nothing is linked on its own: a name can be anybody.
const (
	jobProfileSearch = "profile_search"
	maxSearchQuery   = 200
)

var wikidataIDRe = regexp.MustCompile(`^Q[0-9]{1,12}$`)

// ProfileCandidate is a person Wikidata found, with what tells one from another.
type ProfileCandidate struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Born        string `json:"born,omitempty"`
	Died        string `json:"died,omitempty"`
	Photo       bool   `json:"photo"`
	Wikipedia   bool   `json:"wikipedia"`
}

// ProfileSearch is the last search made for a person.
type ProfileSearch struct {
	State   string             `json:"state"`
	Query   string             `json:"query"`
	Results []ProfileCandidate `json:"results"`
}

// RequestSearch answers POST /admin/people/{id}/profile/search {"query": "..."} (staff): asks the worker to look the person up by their name, or
// by the text given. Asking again while a search of that person waits is the same request.
func (h *PeopleHandler) RequestSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Person not found", http.StatusNotFound)
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
	var name string
	if err := tx.QueryRowContext(r.Context(), `SELECT name FROM person WHERE id = $1`, id).Scan(&name); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Println("Error reading a person:", err)
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if query == "" {
		query = name
	}
	actor := currentUserID(r)
	// One statement under a lock, so two requests at once are one job.
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('profile_search'))`); err != nil {
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO person_profile_searches (person_id, query, state, results, requested_at, finished_at)
		VALUES ($1, $2, 'pending', '[]'::jsonb, now(), NULL)
		ON CONFLICT (person_id) DO UPDATE SET query = EXCLUDED.query, state = 'pending', results = '[]'::jsonb, requested_at = now(), finished_at = NULL`,
		id, query); err != nil {
		log.Println("Error keeping a search:", err)
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	var jobID int64
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO jobs (type, payload, priority, created_by)
		SELECT $1::text, jsonb_build_object('person', $2::int, 'query', $3::text), $4::int, NULLIF($5, '')::uuid
		WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE type = $1::text AND state IN ('pending', 'running') AND (payload->>'person')::int = $2)
		RETURNING id`, jobProfileSearch, id, query, jobs.PriorityManual, actor).Scan(&jobID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Println("Error queueing a search:", err)
		http.Error(w, "Error asking for the search", http.StatusInternalServerError)
		return
	}
	if err == nil {
		if err := audit.Record(r.Context(), tx, actor, "person.profile.search", "person", strconv.Itoa(id), map[string]any{"query": query}); err != nil {
			log.Println("Could not audit person.profile.search:", err)
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
	writeJSON(w, http.StatusAccepted, ProfileSearch{State: "pending", Query: query, Results: []ProfileCandidate{}})
}

// loadSearch is the last search of a person; nil when none was made.
func (h *PeopleHandler) loadSearch(r *http.Request, id int) (*ProfileSearch, error) {
	var s ProfileSearch
	var raw []byte
	err := h.DB.QueryRowContext(r.Context(), `SELECT state, query, results FROM person_profile_searches WHERE person_id = $1`, id).Scan(&s.State, &s.Query, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.Results); err != nil || s.Results == nil {
		s.Results = []ProfileCandidate{}
	}
	return &s, nil
}

// SearchResult answers GET /admin/people/{id}/profile/search (staff): the last search of the person, or 404 when none was made.
func (h *PeopleHandler) SearchResult(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Search not found", http.StatusNotFound)
		return
	}
	s, err := h.loadSearch(r, id)
	if err != nil {
		log.Println("Error reading a search:", err)
		http.Error(w, "Error reading the search", http.StatusInternalServerError)
		return
	}
	if s == nil {
		http.Error(w, "Search not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// LinkProfile answers POST /admin/people/{id}/profile/link {"wikidataId": "Q123"} (staff): the person is the one the candidate says. Only a
// candidate of the last search can be chosen, and the identifier is what the worker reads the profile by. A profile that was written by hand
// is kept (it is discarded on purpose, not by a choice of another author); one read for another identifier is replaced.
func (h *PeopleHandler) LinkProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	var req struct {
		WikidataID string `json:"wikidataId"`
	}
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil || !wikidataIDRe.MatchString(req.WikidataID) {
		http.Error(w, "wikidataId is a Wikidata identifier, like Q6984190", http.StatusBadRequest)
		return
	}
	search, err := h.loadSearch(r, id)
	if err != nil {
		log.Println("Error reading a search:", err)
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	chosen := false
	if search != nil && search.State == "done" {
		for _, c := range search.Results {
			if c.ID == req.WikidataID {
				chosen = true
			}
		}
	}
	if !chosen {
		http.Error(w, "Escolha uma das pessoas da busca.", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var manual sql.NullBool
	var wasID sql.NullString
	if err := tx.QueryRowContext(r.Context(), `SELECT manual, wikidata_id FROM person_profile WHERE person_id = $1 FOR UPDATE`, id).Scan(&manual, &wasID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Println("Error reading a profile:", err)
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	var known bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM person WHERE id = $1)`, id).Scan(&known); err != nil || !known {
		if err != nil {
			http.Error(w, "Error linking the profile", http.StatusInternalServerError)
			return
		}
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	actor := currentUserID(r)
	// The identifier the person held before (the wrong one, if they are choosing again) is not theirs any more.
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM person_authority WHERE person_id = $1 AND scheme = 'wikidata' AND value <> $2`, id, req.WikidataID); err != nil {
		log.Println("Error replacing an identifier:", err)
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	if err := people.RecordAuthority(r.Context(), tx, id, "Escolhido na página da pessoa", map[string]string{"wikidata": req.WikidataID}); err != nil {
		log.Println("Error keeping an identifier:", err)
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	outcome := "queued"
	switch {
	case manual.Valid && manual.Bool:
		outcome = "kept"
	default:
		// A profile read for another identifier goes; and a lookup remembered for this one (missing, failed, done) is asked again.
		if manual.Valid && wasID.Valid && wasID.String == req.WikidataID {
			outcome = "present"
		} else if _, err := tx.ExecContext(r.Context(), `DELETE FROM person_profile WHERE person_id = $1`, id); err != nil {
			log.Println("Error replacing a profile:", err)
			http.Error(w, "Error linking the profile", http.StatusInternalServerError)
			return
		}
		if outcome == "queued" {
			if _, err := tx.ExecContext(r.Context(), `DELETE FROM authority_lookups WHERE source = 'wikidata' AND key = $1`, req.WikidataID); err != nil {
				log.Println("Error forgetting a lookup:", err)
				http.Error(w, "Error linking the profile", http.StatusInternalServerError)
				return
			}
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "person.profile.link", "person", strconv.Itoa(id), map[string]any{"wikidataId": req.WikidataID, "outcome": outcome}); err != nil {
		log.Println("Could not audit person.profile.link:", err)
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error linking the profile", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"profile": outcome})
}
