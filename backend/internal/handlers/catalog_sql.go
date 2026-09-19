package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

// The catalog shows a work through its primary edition and that edition's first
// file (view work_primary), with all its authors joined. Every query that lists
// works starts from this fragment, so a work with several editions and files is
// still one row and no handler joins `editions` directly.
const catalogFrom = `
	FROM works w
	LEFT JOIN work_primary wp ON wp.work_id = w.id
	LEFT JOIN LATERAL (
		SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name) AS names
		FROM work_contributors c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = w.id AND c.role = 'author'
	) au ON TRUE`

// authorLabel is the author text shown for a work; absence is stated, never invented.
const authorLabel = `COALESCE(au.names, 'Unknown Author')`

// authorMatches reports, for the placeholder given, whether any author of the
// work matches a LIKE pattern.
func authorMatches(placeholder string) string {
	return `EXISTS (SELECT 1 FROM work_contributors c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = w.id AND c.role = 'author' AND LOWER(p.name) LIKE LOWER(` + placeholder + `))`
}

// isStaffRequest reports whether the caller is an owner or admin. The role comes
// from the session (read from the database on every request).
func isStaffRequest(r *http.Request) bool {
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	return authz.IsStaff(role)
}

// workFilePath returns the path of the primary file of an available (not
// retired) work.
func workFilePath(db *sql.DB, id string) (sql.NullString, error) {
	var p sql.NullString
	if _, err := strconv.Atoi(id); err != nil {
		return p, sql.ErrNoRows
	}
	err := db.QueryRow(`
		SELECT wp.file_path FROM works w JOIN work_primary wp ON wp.work_id = w.id
		WHERE w.id = $1 AND w.retired_at IS NULL`, id).Scan(&p)
	return p, err
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
