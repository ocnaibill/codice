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

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/dupes"
)

// WorkTitlesHandler is how owner and admin keep the alternative titles of a work (#185, DEC-131).
type WorkTitlesHandler struct{ DB *sql.DB }

const maxWorkTitle = 512

// maxAlternativeTitles bounds what one work can pile up.
const maxAlternativeTitles = 50

// A language is a code like pt, pt-BR or zh-Hant, as the editions already carry them.
var titleLanguage = regexp.MustCompile(`^[A-Za-z]{2,3}([-_][A-Za-z0-9]{2,8})?$`)

// AlternativeTitle is another name of a work. Source is "manual", the name of a provider, or "edition" for the title of one of its
// editions, which is read from there and cannot be removed here (ID is then 0).
type AlternativeTitle struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Language string `json:"language"`
	Source   string `json:"source"`
	// EditionID is the edition a title of source "edition" belongs to.
	EditionID int64 `json:"editionId,omitempty"`
}

// loadAlternativeTitles reads the other names of a work: the ones kept for it, and the titles of its editions that someone wrote and
// that are not the main title nor one of those, each once. The title an edition has only because the file brought it (its file name)
// is not a name to show, though the search still finds the work by it.
func loadAlternativeTitles(db *sql.DB, workID int) ([]AlternativeTitle, error) {
	var main string
	if err := db.QueryRow(`SELECT original_title FROM works WHERE id = $1`, workID).Scan(&main); err != nil {
		return nil, err
	}
	seen := map[string]bool{dupes.NormalizeTitle(main): true}
	out := []AlternativeTitle{}
	rows, err := db.Query(`SELECT id, title, COALESCE(language, ''), source FROM work_titles WHERE work_id = $1 ORDER BY id`, workID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t AlternativeTitle
		if err := rows.Scan(&t.ID, &t.Title, &t.Language, &t.Source); err != nil {
			rows.Close()
			return nil, err
		}
		seen[dupes.NormalizeTitle(t.Title)] = true
		out = append(out, t)
	}
	rows.Close()
	rows, err = db.Query(`SELECT e.id, e.title, COALESCE(e.language, '') FROM editions e WHERE e.work_id = $1 AND e.title_manual AND COALESCE(e.title, '') <> '' ORDER BY e.is_primary DESC, e.id`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t AlternativeTitle
		if err := rows.Scan(&t.EditionID, &t.Title, &t.Language); err != nil {
			return nil, err
		}
		if key := dupes.NormalizeTitle(t.Title); !seen[key] {
			seen[key] = true
			t.Source = "edition"
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

// Add answers POST /works/{id}/titles {"title": "...", "language": "en"}: another name for the work. The language is optional.
// A title the work already goes by (the main one, an alternative one, or the title of one of its editions) is refused.
func (h *WorkTitlesHandler) Add(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var req struct {
		Title    string `json:"title"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	title := strings.Join(strings.Fields(req.Title), " ")
	key := dupes.NormalizeTitle(title)
	if title == "" || key == "" || utf8.RuneCountInString(title) > maxWorkTitle {
		http.Error(w, "O título é obrigatório e tem até 512 caracteres.", http.StatusBadRequest)
		return
	}
	language := strings.TrimSpace(req.Language)
	if language != "" && !titleLanguage.MatchString(language) {
		http.Error(w, "O idioma deve ser um código como pt, en ou pt-BR.", http.StatusBadRequest)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	// The work is locked, so two titles added at once do not both slip past the bound.
	if err := tx.QueryRowContext(r.Context(), `SELECT original_title FROM works WHERE id = $1 FOR UPDATE`, workID).Scan(new(string)); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Println("Error locking a work to add a title:", err)
		http.Error(w, "Error adding the title", http.StatusInternalServerError)
		return
	}
	known, err := h.knownTitles(r, tx, workID)
	if err != nil {
		log.Println("Error reading the titles of a work:", err)
		http.Error(w, "Error adding the title", http.StatusInternalServerError)
		return
	}
	if known.keys[key] {
		http.Error(w, "A obra já tem esse título.", http.StatusConflict)
		return
	}
	if known.alternatives >= maxAlternativeTitles {
		http.Error(w, "A obra já tem "+strconv.Itoa(maxAlternativeTitles)+" títulos alternativos.", http.StatusConflict)
		return
	}
	var id int64
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO work_titles (work_id, title, language, source, title_key) VALUES ($1, $2, NULLIF($3, ''), 'manual', $4) RETURNING id`,
		workID, title, language, key).Scan(&id); err != nil {
		log.Println("Error adding a title:", err)
		http.Error(w, "Error adding the title", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "work.title_add", "work", strconv.Itoa(workID),
		map[string]any{"title": title, "language": language}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error adding the title", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AlternativeTitle{ID: id, Title: title, Language: language, Source: "manual"})
}

type knownTitles struct {
	keys         map[string]bool
	alternatives int
}

// knownTitles are the names the work goes by already, by their keys, and how many alternative titles it has kept.
func (h *WorkTitlesHandler) knownTitles(r *http.Request, tx *sql.Tx, workID int) (knownTitles, error) {
	k := knownTitles{keys: map[string]bool{}}
	rows, err := tx.QueryContext(r.Context(), `
		SELECT original_title, FALSE FROM works WHERE id = $1
		UNION ALL SELECT title, TRUE FROM work_titles WHERE work_id = $1
		UNION ALL SELECT title, FALSE FROM editions WHERE work_id = $1 AND COALESCE(title, '') <> ''`, workID)
	if err != nil {
		return k, err
	}
	defer rows.Close()
	for rows.Next() {
		var title string
		var alternative bool
		if err := rows.Scan(&title, &alternative); err != nil {
			return k, err
		}
		k.keys[dupes.NormalizeTitle(title)] = true
		if alternative {
			k.alternatives++
		}
	}
	return k, rows.Err()
}

// Remove answers DELETE /works/{id}/titles/{titleId}: the work stops going by that name. The title of an edition is not one of
// these: it goes with the edition.
func (h *WorkTitlesHandler) Remove(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	titleID, err := strconv.ParseInt(chi.URLParam(r, "titleId"), 10, 64)
	if !ok || err != nil || titleID <= 0 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var title string
	if err := tx.QueryRowContext(r.Context(), `DELETE FROM work_titles WHERE id = $1 AND work_id = $2 RETURNING title`, titleID, workID).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Title not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Println("Error removing a title:", err)
		http.Error(w, "Error removing the title", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "work.title_remove", "work", strconv.Itoa(workID), map[string]any{"title": title}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error removing the title", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxEditionTitle is the most editions.title holds, in characters.
const maxEditionTitle = 255

// EditEdition answers PATCH /works/{id}/editions/{editionId} {"title": "..."}: the title of one edition of the work, written by owner or
// admin. From then on it is the name of the work while that edition is the one being read, and one of the other names the sheet shows.
// The language of the edition is not changed here: it belongs to what the file is.
func (h *WorkTitlesHandler) EditEdition(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	editionID, err := strconv.ParseInt(chi.URLParam(r, "editionId"), 10, 64)
	if !ok || err != nil || editionID <= 0 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	title := strings.Join(strings.Fields(req.Title), " ")
	if dupes.NormalizeTitle(title) == "" || utf8.RuneCountInString(title) > maxEditionTitle {
		http.Error(w, "O título da edição é obrigatório e tem até 255 caracteres.", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRowContext(r.Context(), `SELECT COALESCE(title, '') FROM editions WHERE id = $1 AND work_id = $2 FOR UPDATE`, editionID, workID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Edition not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading an edition to write its title:", err)
		http.Error(w, "Error writing the title", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE editions SET title = $1, title_manual = TRUE WHERE id = $2`, title, editionID); err != nil {
		log.Println("Error writing the title of an edition:", err)
		http.Error(w, "Error writing the title", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "work.edition_title", "work", strconv.Itoa(workID),
		map[string]any{"edition": editionID, "title": title, "previous": previous}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error writing the title", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": editionID, "title": title, "titleSet": true})
}
