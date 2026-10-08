package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// WorkContributorsHandler is how owner and admin keep the people credited on a work (#185, DEC-132): the authors (the first is
// the main one), and the translators, narrators, editors and illustrators.
type WorkContributorsHandler struct{ DB *sql.DB }

// Contributor is one person credited on a work, with the role and the place among those of the same role (0 is the first).
type Contributor struct {
	PersonID int `json:"personId"`
	// Name is the name as it is stored; DisplayName is how the account that asks is shown it (surname first or not, #64).
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Position    int    `json:"position"`
}

// roleOrder is the order the roles are listed in.
var roleOrder = []string{"author", "translator", "narrator", "editor", "illustrator"}

// maxWorkContributors bounds the people one work can have credited.
const maxWorkContributors = 50

// displayNameSQL is the name of a person (alias `p`) as an account is shown it: surname first where that is what the account chose and
// the surname is known, the name as it is stored otherwise. `order` is a placeholder holding the order that applies to the account.
func displayNameSQL(p, order string) string {
	return `CASE WHEN ` + order + `::text = 'family_first' AND ` + p + `.family_name IS NOT NULL
		THEN ` + p + `.family_name || COALESCE(', ' || ` + p + `.given_name, '') ELSE ` + p + `.name END`
}

// loadContributors reads who is credited on a work, by role and then by place.
func loadContributors(db *sql.DB, workID int, order string) ([]Contributor, error) {
	rows, err := db.Query(`
		SELECT p.id, p.name, `+displayNameSQL("p", "$3")+`, c.role, c.position
		FROM work_contributors c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = $1
		ORDER BY array_position($2::text[], c.role::text), c.position, p.name`, workID, "{"+strings.Join(roleOrder, ",")+"}", order)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contributor{}
	for rows.Next() {
		var c Contributor
		if err := rows.Scan(&c.PersonID, &c.Name, &c.DisplayName, &c.Role, &c.Position); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (h *WorkContributorsHandler) begin(w http.ResponseWriter, r *http.Request) (*sql.Tx, bool) {
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting database transaction", http.StatusInternalServerError)
		return nil, false
	}
	return tx, true
}

func (h *WorkContributorsHandler) fail(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	log.Println("Error in the contributors of a work ("+what+"):", err)
	http.Error(w, "Error changing the people of the work", http.StatusInternalServerError)
}

// lockWork locks the work for the transaction, so changes made at once to who is credited wait for one another.
func lockWork(ctx context.Context, tx *sql.Tx, workID int) (retired bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT retired_at IS NOT NULL FROM works WHERE id = $1 FOR UPDATE`, workID).Scan(&retired)
	return retired, err
}

// firstAuthor is the main author of a work, 0 if it has none.
func firstAuthor(ctx context.Context, tx *sql.Tx, workID int) (id int, name string, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT p.id, p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = $1 AND c.role = 'author' ORDER BY c.position, p.name LIMIT 1`, workID).Scan(&id, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return id, name, err
}

// renumber puts the places of a role in a row again, from 0, in the order they have (and by name where they tie).
func renumber(ctx context.Context, tx *sql.Tx, workID int, role string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE work_contributors c SET position = r.n
		FROM (SELECT c2.person_id, (row_number() OVER (ORDER BY c2.position, p.name) - 1)::smallint AS n
		      FROM work_contributors c2 JOIN person p ON p.id = c2.person_id
		      WHERE c2.work_id = $1 AND c2.role = $2) r
		WHERE c.work_id = $1 AND c.role = $2 AND c.person_id = r.person_id AND c.position IS DISTINCT FROM r.n`, workID, role)
	return err
}

// confirmFirstAuthor is for when a change moved who the first author is: that is the "author" of the work, so it is confirmed
// by hand like an edit of the field is (locked against the automatic extraction, source "manual"), and what the notes remember
// of the author follows it.
func confirmFirstAuthor(ctx context.Context, tx *sql.Tx, workID int, actor string, before, after int, name string, retired bool) error {
	if before == after {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE works SET author_lock = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, workID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO work_field_sources (work_id, field, source, actor_id) VALUES ($1, 'author', $2, NULLIF($3, '')::uuid)
		ON CONFLICT (work_id, field) DO UPDATE SET source = EXCLUDED.source, actor_id = EXCLUDED.actor_id, updated_at = now()`,
		workID, sourceManual, actor); err != nil {
		return err
	}
	if !retired {
		if _, err := tx.ExecContext(ctx, `UPDATE notes SET source_author = NULLIF($1, '') WHERE work_id = $2`, name, workID); err != nil {
			return err
		}
	}
	return nil
}

// Add answers POST /works/{id}/contributors {"name": "...", "role": "author"}: the person is credited on the work, after those
// who have that role already. A person who has the role on the work is refused.
func (h *WorkContributorsHandler) Add(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	name := strings.Join(strings.Fields(req.Name), " ")
	if name == "" || utf8.RuneCountInString(name) > maxContributorNameRunes {
		http.Error(w, "O nome é obrigatório e tem até 200 caracteres.", http.StatusBadRequest)
		return
	}
	if !contributorRoles[req.Role] {
		http.Error(w, "O papel é autor, tradutor, narrador, editor ou ilustrador.", http.StatusBadRequest)
		return
	}
	actor := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := lockWork(r.Context(), tx, workID)
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	var have int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM work_contributors WHERE work_id = $1`, workID).Scan(&have); err != nil {
		h.fail(w, "add", err)
		return
	}
	if have >= maxWorkContributors {
		http.Error(w, "A obra já tem "+strconv.Itoa(maxWorkContributors)+" pessoas creditadas.", http.StatusConflict)
		return
	}
	beforeID, _, err := firstAuthor(r.Context(), tx, workID)
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	personID, err := people.Resolve(r.Context(), tx, name)
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	var c Contributor
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO work_contributors (work_id, person_id, role, position)
		SELECT $1::int, $2::int, $3::varchar, COALESCE((SELECT max(position) + 1 FROM work_contributors WHERE work_id = $1 AND role = $3), 0)
		ON CONFLICT DO NOTHING
		RETURNING position`, workID, personID, req.Role).Scan(&c.Position)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Essa pessoa já tem esse papel na obra.", http.StatusConflict)
		return
	}
	if err != nil {
		h.fail(w, "add", err)
		return
	}
	if err := tx.QueryRowContext(r.Context(), `SELECT name FROM person WHERE id = $1`, personID).Scan(&c.Name); err != nil {
		h.fail(w, "add", err)
		return
	}
	c.PersonID, c.Role = personID, req.Role
	if req.Role == "author" {
		afterID, afterName, err := firstAuthor(r.Context(), tx, workID)
		if err == nil {
			err = confirmFirstAuthor(r.Context(), tx, workID, actor, beforeID, afterID, afterName, retired)
		}
		if err != nil {
			h.fail(w, "add", err)
			return
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "work.contributor_add", "work", strconv.Itoa(workID), map[string]any{"name": c.Name, "role": c.Role}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "add", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

// Remove answers DELETE /works/{id}/contributors/{personId}/{role}: the person is not credited with that role on the work any
// more. The person stays in the library.
func (h *WorkContributorsHandler) Remove(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	personID, err := strconv.Atoi(chi.URLParam(r, "personId"))
	role := chi.URLParam(r, "role")
	if !ok || err != nil || personID <= 0 || !contributorRoles[role] {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	actor := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := lockWork(r.Context(), tx, workID)
	if err != nil {
		h.fail(w, "remove", err)
		return
	}
	beforeID, _, err := firstAuthor(r.Context(), tx, workID)
	if err != nil {
		h.fail(w, "remove", err)
		return
	}
	var name string
	err = tx.QueryRowContext(r.Context(), `
		DELETE FROM work_contributors WHERE work_id = $1 AND person_id = $2 AND role = $3
		RETURNING (SELECT name FROM person WHERE id = $2)`, workID, personID, role).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Contributor not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.fail(w, "remove", err)
		return
	}
	if err := renumber(r.Context(), tx, workID, role); err != nil {
		h.fail(w, "remove", err)
		return
	}
	if role == "author" {
		afterID, afterName, err := firstAuthor(r.Context(), tx, workID)
		if err == nil {
			err = confirmFirstAuthor(r.Context(), tx, workID, actor, beforeID, afterID, afterName, retired)
		}
		if err != nil {
			h.fail(w, "remove", err)
			return
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "work.contributor_remove", "work", strconv.Itoa(workID), map[string]any{"name": name, "role": role}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "remove", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Order answers PUT /works/{id}/contributors/order {"role": "author", "personIds": [3, 1]}: the people of that role get their
// places in that order. The list must be all the people who have the role on the work, each once.
func (h *WorkContributorsHandler) Order(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var req struct {
		Role      string `json:"role"`
		PersonIDs []int  `json:"personIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if !contributorRoles[req.Role] {
		http.Error(w, "O papel é autor, tradutor, narrador, editor ou ilustrador.", http.StatusBadRequest)
		return
	}
	actor := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	retired, err := lockWork(r.Context(), tx, workID)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	rows, err := tx.QueryContext(r.Context(), `SELECT person_id FROM work_contributors WHERE work_id = $1 AND role = $2`, workID, req.Role)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	have := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			h.fail(w, "order", err)
			return
		}
		have[id] = true
	}
	rows.Close()
	seen := map[int]bool{}
	for _, id := range req.PersonIDs {
		if !have[id] || seen[id] {
			http.Error(w, "A lista deve ter cada pessoa desse papel uma vez, e só elas.", http.StatusBadRequest)
			return
		}
		seen[id] = true
	}
	if len(seen) != len(have) {
		http.Error(w, "A lista deve ter cada pessoa desse papel uma vez, e só elas.", http.StatusBadRequest)
		return
	}
	beforeID, _, err := firstAuthor(r.Context(), tx, workID)
	if err != nil {
		h.fail(w, "order", err)
		return
	}
	for i, id := range req.PersonIDs {
		if _, err := tx.ExecContext(r.Context(), `UPDATE work_contributors SET position = $4 WHERE work_id = $1 AND person_id = $2 AND role = $3`, workID, id, req.Role, i); err != nil {
			h.fail(w, "order", err)
			return
		}
	}
	if req.Role == "author" {
		afterID, afterName, err := firstAuthor(r.Context(), tx, workID)
		if err == nil {
			err = confirmFirstAuthor(r.Context(), tx, workID, actor, beforeID, afterID, afterName, retired)
		}
		if err != nil {
			h.fail(w, "order", err)
			return
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "work.contributor_order", "work", strconv.Itoa(workID), map[string]any{"role": req.Role, "people": req.PersonIDs}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, "order", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
