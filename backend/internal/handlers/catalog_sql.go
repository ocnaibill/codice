package handlers

import (
	"database/sql"
	"github.com/ocnaibill/codice/backend/internal/people"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// The catalog shows a work through its primary edition and that edition's first
// file (view work_primary), with all its authors joined. Every query that lists
// works starts from this fragment, so a work with several editions and files is
// still one row and no handler joins `editions` directly.
const catalogFrom = `
	FROM works w
	LEFT JOIN work_primary wp ON wp.work_id = w.id` + authorLateral

// authorLateral is the join (alias `au`) that gives a work its authors: their names joined, given names first or surname first, and
// each author apart (the id and the name as it is shown), in their order, for the card to link each to the page of the person (#186).
const authorLateral = `
	LEFT JOIN LATERAL (
		SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name) AS names,
		       string_agg(CASE WHEN p.family_name IS NULL THEN p.name ELSE p.family_name || COALESCE(', ' || p.given_name, '') END,
		                  '; ' ORDER BY c.position, p.name) AS names_family,
		       array_agg(p.id ORDER BY c.position, p.name) AS ids,
		       array_agg(p.name ORDER BY c.position, p.name) AS person_names,
		       array_agg(CASE WHEN p.family_name IS NULL THEN p.name ELSE p.family_name || COALESCE(', ' || p.given_name, '') END
		                 ORDER BY c.position, p.name) AS person_names_family
		FROM work_contributors c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = w.id AND c.role = 'author'
	) au ON TRUE`

// WorkAuthor is an author of a work as a card links it: the person, and the name as the account is shown it.
type WorkAuthor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// personNamesFor is the column of the names of the authors, each apart, for the order an account prefers.
func personNamesFor(order string) string {
	if order == people.FamilyFirst {
		return "au.person_names_family"
	}
	return "au.person_names"
}

// workAuthorsOf pairs the ids and the names read for a work (never null: a work with no author has none).
func workAuthorsOf(ids pq.Int64Array, names pq.StringArray) []WorkAuthor {
	out := make([]WorkAuthor, 0, len(ids))
	for i, id := range ids {
		if i < len(names) {
			out = append(out, WorkAuthor{ID: int(id), Name: names[i]})
		}
	}
	return out
}

// authorLabel is the author text shown for a work, given names first; absence is stated, never invented.
const authorLabel = `COALESCE(au.names, 'Unknown Author')`

// authorLabelFor is the same for the order a person prefers (#64): "Herbert, Frank" puts the surname first
// where it is known, and leaves a name whose parts are not known as it is. Only presentation.
func authorLabelFor(order string) string {
	if order == people.FamilyFirst {
		return `COALESCE(au.names_family, 'Unknown Author')`
	}
	return authorLabel
}

// titleMatches reports, for the placeholder given, whether the work goes by a name that matches a LIKE pattern: its main title, an
// alternative title it keeps, or the title of one of its editions (#185).
func titleMatches(placeholder string) string {
	return `(LOWER(w.original_title) LIKE LOWER(` + placeholder + `)
		OR EXISTS (SELECT 1 FROM work_titles wt WHERE wt.work_id = w.id AND LOWER(wt.title) LIKE LOWER(` + placeholder + `))
		OR EXISTS (SELECT 1 FROM editions we WHERE we.work_id = w.id AND LOWER(we.title) LIKE LOWER(` + placeholder + `)))`
}

// authorMatches reports, for the placeholder given, whether any author of the
// work matches a LIKE pattern.
func authorMatches(placeholder string) string {
	return `EXISTS (SELECT 1 FROM work_contributors c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = w.id AND c.role = 'author' AND (LOWER(p.name) LIKE LOWER(` + placeholder + `)
		   -- the name as it is shown with the surname first, whichever order the person who searches prefers
		   OR (p.family_name IS NOT NULL AND LOWER(p.family_name || COALESCE(', ' || p.given_name, '')) LIKE LOWER(` + placeholder + `))
		   OR EXISTS (SELECT 1 FROM person_alias a WHERE a.person_id = p.id AND LOWER(a.alias) LIKE LOWER(` + placeholder + `))))`
}

// A search term is literal text. Escape LIKE's wildcards before surrounding it
// with the two wildcards that implement a substring search.
func catalogSearchPattern(query string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
}

// isStaffRequest reports whether the caller is an owner or admin. The role comes
// from the session (read from the database on every request).
func isStaffRequest(r *http.Request) bool {
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	return authz.IsStaff(role)
}

// workFilePath returns the absolute path of the primary file of an available
// (not retired) work, whether it is managed or referenced.
func workFilePath(db *sql.DB, id string) (sql.NullString, error) {
	var out sql.NullString
	if _, err := strconv.Atoi(id); err != nil {
		return out, sql.ErrNoRows
	}
	var mode, root, rel sql.NullString
	err := db.QueryRow(`
		SELECT wp.file_mode, wp.file_root, wp.file_path
		FROM works w JOIN work_primary wp ON wp.work_id = w.id
		WHERE w.id = $1 AND w.retired_at IS NULL AND COALESCE(wp.file_state, 'ok') = 'ok'`, id).Scan(&mode, &root, &rel)
	if err != nil || !rel.Valid {
		return out, err
	}
	if full, ok := storage.AbsPath(resolveStoragePath(), mode.String, root.String, rel.String); ok {
		out = sql.NullString{String: full, Valid: true}
	}
	return out, nil
}

// filesURL is the URL of a managed file. Paths now have folders, spaces and
// accents, so each segment is escaped on its own; escaping the whole path would
// turn the folder separators into %2F.
func filesURL(rel string) string {
	segments := strings.Split(rel, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return "/files/" + strings.Join(segments, "/")
}

// fileHref is the URL of a file: managed files by their path, referenced files
// (which have no path inside the storage directory) by id.
func fileHref(fileID sql.NullInt64, mode, rel string) string {
	if mode == "referenced" && fileID.Valid {
		return "/file/" + strconv.FormatInt(fileID.Int64, 10)
	}
	if rel == "" {
		return ""
	}
	return filesURL(rel)
}

// catalogNeedsJoins says whether choosing the page of the catalog needs more than the table of works: a filter on the
// primary file's format, on the caller's progress or favorites, or an order by author reads the joins of the card. Without
// any of them (the library's default view, the search by title, the ordering by title) the page is chosen on `works` alone.
// Every join of the card yields one row per work, so leaving them out changes neither the count nor the order.
func catalogNeedsJoins(sort string, inProgress, favorite bool, formatGroup string) bool {
	return inProgress || favorite || formatGroup != "" || sort == "author"
}

// catalogOrderBy is how the catalog is sorted: the newest first (what it always was), by title, or by author in
// the order the caller prefers (#64), so that a person who reads "Herbert, Frank" finds it among the Hs. Names
// are compared without accents or case, works with no author come last, and every sort ends in the same way so
// that a page never repeats or skips a work.
func catalogOrderBy(sort, order string) string {
	return catalogOrderByTitled(sort, order, "w.original_title")
}

// catalogOrderByTitled is catalogOrderBy for a list in which some rows stand for a whole series (#187) and are sorted by the
// name of the series: title is the expression that names a row.
func catalogOrderByTitled(sort, order, title string) string {
	key := func(expr string) string { return "unaccent(lower(" + expr + "))" }
	switch sort {
	case "title":
		return key(title) + ", w.id DESC"
	case "author":
		return "(au.names IS NULL), " + key(authorLabelFor(order)) + ", " + key(title) + ", w.id DESC"
	}
	return "w.id DESC"
}
