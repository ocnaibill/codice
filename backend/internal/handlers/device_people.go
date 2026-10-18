package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/people"
)

const (
	devicePeoplePage = 30  // authors in a page of the list of a reader when it does not say how many fit its screen
	devicePeopleMax  = 100 // the most it may ask for in a page
)

// devicePerson is an author in the list of a reader: the name as the person reads it (surname first when they chose that) and how many works.
type devicePerson struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Works int    `json:"works"`
}

// People lists the people who wrote something in the library, by the name as the caller prefers to read it, a page at a time
// (`?page=`), or only those whose name starts with a letter (`?letter=A`; `#` is any other start). The answer also carries the letters there
// are, with how many people each, for a reader to jump to one instead of turning pages: a library has hundreds of authors.
func (h *DeviceHandler) People(w http.ResponseWriter, r *http.Request) {
	label := `p.name`
	if people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective == people.FamilyFirst {
		label = `COALESCE(p.family_name || COALESCE(', ' || p.given_name, ''), p.name)`
	}
	// the first letter with no accent, in capitals; anything that is not a letter is "#"
	initial := `CASE WHEN left(unaccent(lower(` + label + `)), 1) ~ '[a-z]' THEN upper(left(unaccent(lower(` + label + `)), 1)) ELSE '#' END`
	from := ` FROM person p
		JOIN work_contributors c ON c.person_id = p.id AND c.role = 'author'
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL`

	// the letters there are
	letters := []map[string]any{}
	rows, err := h.DB.QueryContext(r.Context(), `SELECT letter, count(*) FROM (SELECT p.id, `+initial+` AS letter`+from+` GROUP BY p.id) x GROUP BY letter ORDER BY (letter = '#'), letter`)
	if err != nil {
		http.Error(w, "Error fetching authors", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var letter string
		var n int
		if rows.Scan(&letter, &n) == nil {
			letters = append(letters, map[string]any{"letter": letter, "people": n})
		}
	}
	rows.Close()

	where, args := "", []any{}
	if l := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("letter"))); l != "" {
		if l != "#" && !(len(l) == 1 && l[0] >= 'A' && l[0] <= 'Z') {
			http.Error(w, "letter must be A-Z or #", http.StatusBadRequest)
			return
		}
		where = ` HAVING ` + initial + ` = $1`
		args = append(args, l)
	}
	page := 1
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 1 {
		page = n
	}
	// how many fit the screen of the reader: it asks (a page that does not fit cannot be read, there is no scrolling)
	per := devicePeoplePage
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		per = n
		if per > devicePeopleMax {
			per = devicePeopleMax
		}
	}

	// how many there are (with the letter, if one was asked), then the page
	var total int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM (SELECT p.id`+from+` GROUP BY p.id`+where+`) x`, args...).Scan(&total); err != nil {
		http.Error(w, "Error fetching authors", http.StatusInternalServerError)
		return
	}
	pages := (total + per - 1) / per
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	pageArgs := append(append([]any{}, args...), per, (page-1)*per)
	n := len(args)
	rows, err = h.DB.QueryContext(r.Context(), `SELECT p.id, `+label+`, count(DISTINCT w.id)`+from+` GROUP BY p.id`+where+
		` ORDER BY unaccent(lower(`+label+`)), p.id LIMIT $`+strconv.Itoa(n+1)+` OFFSET $`+strconv.Itoa(n+2), pageArgs...)
	if err != nil {
		http.Error(w, "Error fetching authors", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []devicePerson{}
	for rows.Next() {
		var p devicePerson
		if rows.Scan(&p.ID, &p.Name, &p.Works) == nil {
			out = append(out, p)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "page": page, "pages": pages, "limit": per, "letters": letters})
}
