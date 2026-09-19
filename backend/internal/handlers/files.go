package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// FileAccess describes who owns a stored file, for authorization.
type FileAccess struct {
	WorkRetired bool
}

// ErrFileNotFound means the path is not a file the catalog knows about.
var ErrFileNotFound = errors.New("file not found")

// FileLookup resolves a stored path to its owner. It returns ErrFileNotFound
// for a path that belongs to no work.
type FileLookup func(ctx context.Context, rel string) (FileAccess, error)

// NewFileLookup finds the work that owns a managed file.
func NewFileLookup(db *sql.DB) FileLookup {
	return func(ctx context.Context, rel string) (FileAccess, error) {
		var a FileAccess
		err := db.QueryRowContext(ctx, `
			SELECT w.retired_at IS NOT NULL
			FROM storage_locations l
			JOIN files f ON f.id = l.file_id
			JOIN editions e ON e.id = f.edition_id
			JOIN works w ON w.id = e.work_id
			WHERE l.mode = 'managed' AND l.state = 'ok' AND l.path = $1
			LIMIT 1`, rel).Scan(&a.WorkRetired)
		if errors.Is(err, sql.ErrNoRows) {
			return a, ErrFileNotFound
		}
		return a, err
	}
}

// NewCoverLookup finds the work that owns a cover image. A cover no edition
// claims (the placeholder, for instance) is public to any signed-in user.
func NewCoverLookup(db *sql.DB) FileLookup {
	return func(ctx context.Context, name string) (FileAccess, error) {
		var retired sql.NullBool
		err := db.QueryRowContext(ctx, `
			SELECT bool_and(w.retired_at IS NOT NULL)
			FROM editions e JOIN works w ON w.id = e.work_id
			WHERE e.cover_url = '/covers/' || $1`, name).Scan(&retired)
		if err != nil {
			return FileAccess{}, err
		}
		return FileAccess{WorkRetired: retired.Valid && retired.Bool}, nil
	}
}

// FilesHandler serves stored files behind authorization. Unlike a plain file
// server it serves only paths the catalog owns, never lists directories, and
// hides the files of a retired work from everyone but owner and admin.
type FilesHandler struct {
	Root        string // directory the paths are relative to
	Lookup      FileLookup
	CacheHeader string
}

// cleanRelPath validates the wildcard part of the URL: no traversal, no
// absolute paths, no directory requests.
func cleanRelPath(raw string) (string, bool) {
	if raw == "" || strings.HasSuffix(raw, "/") || strings.ContainsRune(raw, 0) || strings.Contains(raw, `\`) {
		return "", false
	}
	clean := path.Clean(raw)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || path.IsAbs(clean) {
		return "", false
	}
	return clean, true
}

func (h *FilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rel, ok := cleanRelPath(chi.URLParam(r, "*"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	access, err := h.Lookup(r.Context(), rel)
	switch {
	case errors.Is(err, ErrFileNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, "Error looking up file", http.StatusInternalServerError)
		return
	}
	if access.WorkRetired && !isStaffRequest(r) {
		http.NotFound(w, r)
		return
	}

	full, inside := insideStorage(h.Root, filepath.FromSlash(rel))
	if !inside {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	if h.CacheHeader != "" {
		w.Header().Set("Cache-Control", h.CacheHeader)
	}
	if strings.HasSuffix(strings.ToLower(rel), ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	// ServeContent gives Range requests, which readers and audio players use.
	http.ServeContent(w, r, path.Base(rel), info.ModTime().Truncate(time.Second), f)
}
