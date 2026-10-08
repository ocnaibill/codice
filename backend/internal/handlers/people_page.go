package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// PersonPage is what the page of a person says (#186): who they are, the names they go by, in which roles the library has works of
// theirs, and the collections those works are in. The works themselves are the list of works with `person=` and `role=`.
type PersonPage struct {
	ID int `json:"id"`
	// Name is the name as it is stored; DisplayName is how the account that asks is shown it.
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Aliases     []string `json:"aliases"`
	// Roles are the roles the person has on works of the library, with how many works, in the order the roles are listed.
	Roles       []PersonRole       `json:"roles"`
	Collections []PersonCollection `json:"collections"`
}

// PersonRole is a role of a person and how many works the library has of theirs with it.
type PersonRole struct {
	Role  string `json:"role"`
	Works int    `json:"works"`
}

// PersonCollection is an official collection with works of the person, and how many of those.
type PersonCollection struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Works int    `json:"works"`
}

// Page answers GET /people/{id}. The works that are retired do not count, for anybody.
func (h *PeopleHandler) Page(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	order := people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective
	page := PersonPage{Aliases: []string{}, Roles: []PersonRole{}, Collections: []PersonCollection{}}
	err = h.DB.QueryRowContext(r.Context(), `SELECT p.id, p.name, `+displayNameSQL("p", "$2")+` FROM person p WHERE p.id = $1`, id, order).
		Scan(&page.ID, &page.Name, &page.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}

	// The other names they are written with, except the one they go by.
	rows, err := h.DB.QueryContext(r.Context(), `SELECT alias FROM person_alias WHERE person_id = $1 AND alias <> $2 ORDER BY lower(alias), alias`, id, page.Name)
	if err != nil {
		log.Println("Error reading the aliases of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Aliases = append(page.Aliases, a)
	}
	rows.Close()

	rows, err = h.DB.QueryContext(r.Context(), `
		SELECT c.role, count(DISTINCT c.work_id)
		FROM work_contributors c JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		WHERE c.person_id = $1
		GROUP BY c.role
		ORDER BY array_position($2::text[], c.role::text)`, id, "{"+strings.Join(roleOrder, ",")+"}")
	if err != nil {
		log.Println("Error reading the roles of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var pr PersonRole
		if err := rows.Scan(&pr.Role, &pr.Works); err != nil {
			rows.Close()
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Roles = append(page.Roles, pr)
	}
	rows.Close()

	rows, err = h.DB.QueryContext(r.Context(), `
		SELECT col.id, col.name, count(DISTINCT cw.work_id)
		FROM work_contributors c
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		JOIN collection_works cw ON cw.work_id = w.id AND cw.official
		JOIN collections col ON col.id = cw.collection_id AND col.retired_at IS NULL AND col.kind = 'official'
		WHERE c.person_id = $1
		GROUP BY col.id, col.name
		ORDER BY lower(col.name), col.id`, id)
	if err != nil {
		log.Println("Error reading the collections of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pc PersonCollection
		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Works); err != nil {
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Collections = append(page.Collections, pc)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
