package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
