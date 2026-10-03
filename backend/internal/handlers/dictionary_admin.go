package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/dictionary"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/redis/go-redis/v9"
)

// DictionaryAdminHandler lists the dictionaries the owner may install and installs and removes them (#109, DEC-115). The
// list is the server's catalog (internal/dictionary), never an address from a request: the worker downloads a package
// only from there, and only when the owner asks. Everyone on the staff sees the list; installing and removing are the
// owner's.
type DictionaryAdminHandler struct {
	DB          *sql.DB
	RedisClient *redis.Client
}

// JobTypeDictionary is the kind of job that downloads and imports a package.
const JobTypeDictionary = "dictionary"

type dictionaryItem struct {
	dictionary.Package
	// State is "available" (not installed), "installing", "ready" or "failed".
	State       string     `json:"state"`
	Stage       string     `json:"stage,omitempty"`
	Progress    float64    `json:"progress"`
	BytesDone   int64      `json:"bytesDone"`
	BytesTotal  *int64     `json:"bytesTotal"`
	Entries     int        `json:"entries"`
	Forms       int        `json:"forms"`
	Links       int        `json:"links"`
	InstalledAt *time.Time `json:"installedAt"`
	SourceDate  string     `json:"sourceDate,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// state is what the database has of one package.
type dictionaryState struct {
	state, stage, sourceDate, errText string
	progress                          float64
	bytesDone                         int64
	bytesTotal                        sql.NullInt64
	entries, forms, links             int
	installedAt                       sql.NullTime
}

func (h *DictionaryAdminHandler) states(r *http.Request) (map[string]dictionaryState, error) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, state, stage, progress, bytes_done, bytes_total, COALESCE(source_date, ''), entries, forms, links, error, installed_at
		FROM dictionary_packages`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]dictionaryState{}
	for rows.Next() {
		var id string
		var s dictionaryState
		if err := rows.Scan(&id, &s.state, &s.stage, &s.progress, &s.bytesDone, &s.bytesTotal, &s.sourceDate, &s.entries, &s.forms, &s.links, &s.errText, &s.installedAt); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

func itemOf(p dictionary.Package, s dictionaryState, installed bool) dictionaryItem {
	item := dictionaryItem{Package: p, State: "available"}
	if !installed {
		return item
	}
	item.State, item.Stage, item.Progress, item.BytesDone = s.state, s.stage, s.progress, s.bytesDone
	if s.bytesTotal.Valid {
		item.BytesTotal = &s.bytesTotal.Int64
	}
	item.Entries, item.Forms, item.Links, item.SourceDate, item.Error = s.entries, s.forms, s.links, s.sourceDate, s.errText
	if s.installedAt.Valid {
		item.InstalledAt = &s.installedAt.Time
	}
	return item
}

// List returns the catalog, each package with where its installation is.
func (h *DictionaryAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	states, err := h.states(r)
	if err != nil {
		log.Println("Error listing dictionaries:", err)
		http.Error(w, "Error listing dictionaries", http.StatusInternalServerError)
		return
	}
	items := make([]dictionaryItem, 0, len(dictionary.Catalog))
	for _, p := range dictionary.Catalog {
		s, ok := states[p.ID]
		items = append(items, itemOf(p, s, ok))
	}
	json.NewEncoder(w).Encode(map[string]any{"packages": items})
}

func (h *DictionaryAdminHandler) one(w http.ResponseWriter, r *http.Request, p dictionary.Package, status int) {
	states, err := h.states(r)
	if err != nil {
		http.Error(w, "Error reading the dictionary", http.StatusInternalServerError)
		return
	}
	s, ok := states[p.ID]
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(itemOf(p, s, ok))
}

func (h *DictionaryAdminHandler) catalogPackage(w http.ResponseWriter, r *http.Request) (dictionary.Package, bool) {
	p, ok := dictionary.Find(chi.URLParam(r, "id"))
	if !ok {
		http.Error(w, "Dicionário não encontrado", http.StatusNotFound)
		return p, false
	}
	return p, true
}

// Install asks for a package to be downloaded and imported. The click is the owner's permission: the server downloads
// nothing otherwise. Asking again for one that is being installed is refused; for one that is installed it is an update
// (the new data replaces the old only when it is all in).
func (h *DictionaryAdminHandler) Install(w http.ResponseWriter, r *http.Request) {
	p, ok := h.catalogPackage(w, r)
	if !ok {
		return
	}
	if !p.Installable {
		http.Error(w, "Este dicionário ainda não pode ser instalado nesta versão.", http.StatusConflict)
		return
	}
	if err := p.Validate(); err != nil {
		log.Println("Dictionary catalog:", err)
		http.Error(w, "Este dicionário não tem um endereço permitido.", http.StatusConflict)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error installing the dictionary", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var claimed string
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO dictionary_packages (id, state, stage, progress, source_url, installed_by)
		VALUES ($1, 'installing', 'queued', 0, $2, NULLIF($3, '')::uuid)
		ON CONFLICT (id) DO UPDATE SET state = 'installing', stage = 'queued', progress = 0, bytes_done = 0, bytes_total = NULL,
		       error = '', source_url = EXCLUDED.source_url, installed_by = EXCLUDED.installed_by, updated_at = now()
		WHERE dictionary_packages.state <> 'installing'
		RETURNING id`, p.ID, p.URL, currentUserID(r)).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Este dicionário já está sendo instalado.", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error claiming the dictionary:", err)
		http.Error(w, "Error installing the dictionary", http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(map[string]any{"package": p.ID, "url": p.URL, "edition": p.Edition, "headwords": p.Headwords, "translations": p.Translations})
	var jobID int64
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO jobs (type, payload, priority, created_by) VALUES ($1, $2::jsonb, $3, NULLIF($4, '')::uuid) RETURNING id`,
		JobTypeDictionary, payload, jobs.PriorityManual, currentUserID(r)).Scan(&jobID); err != nil {
		log.Println("Error queueing the dictionary job:", err)
		http.Error(w, "Error queueing the installation", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE dictionary_packages SET job_id = $2 WHERE id = $1`, p.ID, jobID); err != nil {
		http.Error(w, "Error installing the dictionary", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error installing the dictionary", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "dictionary.install", "dictionary", p.ID, map[string]any{"job": jobID, "url": p.URL})
	if err := jobs.Notify(r.Context(), h.RedisClient, jobID); err != nil {
		log.Println("Could not wake the workers (they will find the job on their next poll):", err)
	}
	h.one(w, r, p, http.StatusAccepted)
}

// Cancel stops an installation. What was imported so far is not kept: a package is all of it or none.
func (h *DictionaryAdminHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	p, ok := h.catalogPackage(w, r)
	if !ok {
		return
	}
	var jobID sql.NullInt64
	err := h.DB.QueryRowContext(r.Context(), `SELECT job_id FROM dictionary_packages WHERE id = $1 AND state = 'installing'`, p.ID).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !jobID.Valid) {
		http.Error(w, "Este dicionário não está sendo instalado.", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "Error reading the dictionary", http.StatusInternalServerError)
		return
	}
	if err := jobs.Cancel(r.Context(), h.DB, jobID.Int64); err != nil && !errors.Is(err, jobs.ErrState) && !errors.Is(err, jobs.ErrNotFound) {
		log.Println("Error cancelling the dictionary job:", err)
		http.Error(w, "Error cancelling the installation", http.StatusInternalServerError)
		return
	}
	// A job that was still waiting is cancelled at once, and no worker will say so: the package says it. One that is
	// running says it itself, when it stops.
	if _, err := h.DB.ExecContext(r.Context(), `
		UPDATE dictionary_packages SET state = 'failed', stage = 'cancelled', error = 'Instalação cancelada.', updated_at = now()
		WHERE id = $1 AND state = 'installing' AND EXISTS (SELECT 1 FROM jobs WHERE id = job_id AND state = 'cancelled')`, p.ID); err != nil {
		log.Println("Error marking the dictionary as cancelled:", err)
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "dictionary.cancel", "dictionary", p.ID, map[string]any{"job": jobID.Int64})
	h.one(w, r, p, http.StatusOK)
}

// Remove deletes an installed package with everything it brought. One that is being installed has to be cancelled first.
func (h *DictionaryAdminHandler) Remove(w http.ResponseWriter, r *http.Request) {
	p, ok := h.catalogPackage(w, r)
	if !ok {
		return
	}
	var state string
	err := h.DB.QueryRowContext(r.Context(), `SELECT state FROM dictionary_packages WHERE id = $1`, p.ID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Este dicionário não está instalado.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error reading the dictionary", http.StatusInternalServerError)
		return
	}
	if state == "installing" {
		http.Error(w, "Cancele a instalação antes de remover.", http.StatusConflict)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM dictionary_packages WHERE id = $1 AND state <> 'installing'`, p.ID); err != nil {
		log.Println("Error removing the dictionary:", err)
		http.Error(w, "Error removing the dictionary", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "dictionary.remove", "dictionary", p.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}
