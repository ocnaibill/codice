package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/storage"
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

// FileByIDHandler serves a file by its id. Referenced files need it: they live in
// a directory the owner authorised, not under the storage directory, so they have
// no path a /files/ URL could name. Authorization is the same as for /files/:
// the catalog must own the file, and a retired work's files are for staff only.
type FileByIDHandler struct {
	DB          *sql.DB
	StorageRoot string
}

func (h *FileByIDHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	var mode, state string
	var root, rel sql.NullString
	var retired bool
	err = h.DB.QueryRowContext(r.Context(), `
		SELECT l.mode, l.root, l.path, l.state, w.retired_at IS NOT NULL
		FROM files f
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		JOIN storage_locations l ON l.file_id = f.id
		WHERE f.id = $1 ORDER BY l.id LIMIT 1`, id).Scan(&mode, &root, &rel, &state, &retired)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (state != "ok" || (retired && !isStaffRequest(r)))) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Error looking up file", http.StatusInternalServerError)
		return
	}
	full, ok := storage.AbsPath(h.StorageRoot, mode, root.String, rel.String)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if mode == "referenced" {
		// The directory may have changed since it was catalogued (a symlink put where
		// a file was). Serve only what still resolves inside its authorised root.
		realFile, err1 := filepath.EvalSymlinks(full)
		realRoot, err2 := filepath.EvalSymlinks(root.String)
		if err1 != nil || err2 != nil || !isWithin(realFile, realRoot) {
			http.NotFound(w, r)
			return
		}
		full = realFile
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	http.ServeContent(w, r, path.Base(rel.String), info.ModTime().Truncate(time.Second), f)
}
