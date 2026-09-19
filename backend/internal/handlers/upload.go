package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/redis/go-redis/v9"
)

// SupportedFormats is the set of file extensions accepted by upload and bulk import.
var SupportedFormats = map[string]bool{
	".pdf": true, ".epub": true,
	".cbz": true, ".cbr": true,
	".txt": true, ".md": true,
	".mobi": true, ".azw": true, ".azw3": true,
	".mp3": true, ".m4a": true, ".m4b": true,
	".flac": true, ".ogg": true, ".wav": true,
}

// resolveStoragePath resolves a relative CODICE_STORAGE_PATH from the project root.
func resolveStoragePath() string {
	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}
	if !filepath.IsAbs(storagePath) {
		cwd, _ := os.Getwd()
		candidate := cwd
		for i := 0; i < 5; i++ {
			if filepath.Base(candidate) == "backend" {
				candidate = filepath.Dir(candidate)
				break
			}
			if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break
			}
			candidate = parent
		}
		storagePath = filepath.Join(candidate, storagePath)
	}
	return storagePath
}

var errOutsideImportRoots = errors.New("directory is outside the allowed import roots")

// importRoots lists the directories bulk import may read: the default
// <storage>/import first, then absolute paths from CODICE_IMPORT_ROOTS
// (separated like PATH). The owner will manage these from the interface later
// (DEC-035); until then they come from configuration.
func importRoots(storagePath string) []string {
	roots := []string{filepath.Join(storagePath, "import")}
	for _, r := range filepath.SplitList(os.Getenv("CODICE_IMPORT_ROOTS")) {
		if r = strings.TrimSpace(r); r != "" && filepath.IsAbs(r) {
			roots = append(roots, filepath.Clean(r))
		}
	}
	return roots
}

// resolveImportDir returns the real path of the requested directory if it is
// inside one of the roots. An empty request means the default root and a
// relative one is taken relative to it. Symlinks are resolved on both sides, so
// a link inside a root cannot lead outside it.
func resolveImportDir(requested string, roots []string) (string, error) {
	if requested == "" {
		requested = roots[0]
	} else if !filepath.IsAbs(requested) {
		requested = filepath.Join(roots[0], requested)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(requested))
	if err != nil {
		if os.IsNotExist(err) {
			// A path that does not exist yet can still be refused for where it points.
			if !lexicallyInside(filepath.Clean(requested), roots) {
				return "", errOutsideImportRoots
			}
			return "", os.ErrNotExist
		}
		return "", err
	}
	for _, root := range roots {
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if isWithin(real, realRoot) {
			return real, nil
		}
	}
	return "", errOutsideImportRoots
}

func lexicallyInside(path string, roots []string) bool {
	for _, root := range roots {
		if isWithin(path, root) {
			return true
		}
	}
	return false
}

// isWithin reports whether path is root or below it (not a sibling that merely
// shares a name prefix).
func isWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// UploadHandler holds Redis and PostgreSQL connections
type UploadHandler struct {
	DB          *sql.DB
	RedisClient *redis.Client
}

// maxUploadBytes is the largest file the server accepts. It is a real limit on
// the request body, not just on memory (RF-008). CODICE_MAX_UPLOAD_MB overrides
// the default of 1 GiB.
func maxUploadBytes() int64 {
	if mb, err := strconv.ParseInt(os.Getenv("CODICE_MAX_UPLOAD_MB"), 10, 64); err == nil && mb > 0 {
		return mb << 20
	}
	return 1 << 30
}

// ingestStatus maps an ingestion error to an HTTP answer.
func ingestStatus(w http.ResponseWriter, err error) {
	var dup *errDuplicate
	var bad *errBadContent
	switch {
	case errors.As(err, &dup):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "duplicate", "message": dup.Error(),
			"work_id": dup.WorkID, "file_id": dup.FileID, "title": dup.Title, "retired": dup.Retired,
		})
	case errors.Is(err, errUnsupportedFormat):
		http.Error(w, "Unsupported file format", http.StatusBadRequest)
	case errors.Is(err, errTooLarge):
		http.Error(w, "File is too large", http.StatusRequestEntityTooLarge)
	case errors.As(err, &bad):
		http.Error(w, bad.Error(), http.StatusUnsupportedMediaType)
	default:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "File is too large", http.StatusRequestEntityTooLarge)
			return
		}
		log.Println("Ingestion failed:", err)
		http.Error(w, "Error saving file", http.StatusInternalServerError)
	}
}

// HandleUpload receives one file (form field "document"), streams it to a
// staging file with a hash, validates it, and hands it to the ingestion path.
func (h *UploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	limit := maxUploadBytes()
	// The whole body is capped: multipart framing adds a little to the file.
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Expected a multipart form", http.StatusBadRequest)
		return
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			http.Error(w, "Error reading uploaded file", http.StatusBadRequest)
			return
		}
		if err != nil {
			ingestStatus(w, err)
			return
		}
		if part.FormName() != "document" || part.FileName() == "" {
			continue
		}

		res, err := h.ingest(r.Context(), part, part.FileName(), ingestOptions{
			Actor: currentUserID(r), Priority: jobs.PriorityManual, MaxBytes: limit,
		})
		if err != nil {
			ingestStatus(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Upload completed and enqueued",
			"work_id": res.WorkID,
			"job_id":  res.JobID,
		})
		return
	}
}

type BulkImportRequest struct {
	Directory string `json:"directory"`
}

type BulkImportResponse struct {
	Message    string `json:"message"`
	Scanned    int    `json:"scanned"`
	Enqueued   int    `json:"enqueued"`
	Duplicates int    `json:"duplicates"`
	Errors     int    `json:"errors"`
}

// HandleBulkImport scans a directory recursively and enqueues all discovered PDF/EPUB/CBZ documents
func (h *UploadHandler) HandleBulkImport(w http.ResponseWriter, r *http.Request) {
	var req BulkImportRequest
	json.NewDecoder(r.Body).Decode(&req)

	storagePath := resolveStoragePath()
	roots := importRoots(storagePath)

	// The default import directory is created on first use; directories named in
	// a request never are.
	if req.Directory == "" {
		if err := os.MkdirAll(roots[0], 0755); err != nil {
			http.Error(w, "Error preparing import directory", http.StatusInternalServerError)
			return
		}
	}

	targetDir, err := resolveImportDir(req.Directory, roots)
	switch {
	case errors.Is(err, errOutsideImportRoots):
		http.Error(w, "Forbidden: directory is outside the allowed import roots", http.StatusForbidden)
		return
	case errors.Is(err, os.ErrNotExist):
		http.Error(w, "Directory not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "Error resolving import directory", http.StatusInternalServerError)
		return
	}

	var scannedCount, enqueuedCount, duplicateCount, errorCount int
	ctx := r.Context()
	actor := currentUserID(r)
	limit := maxUploadBytes()

	err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// A symlink named like a book could expose any file the server can
		// read; only regular files are imported.
		if !info.Mode().IsRegular() {
			return nil
		}
		if !SupportedFormats[strings.ToLower(filepath.Ext(info.Name()))] {
			return nil
		}
		scannedCount++

		src, err := os.Open(path)
		if err != nil {
			errorCount++
			return nil
		}
		defer src.Close()

		// Same path as an upload (hash, validation, one transaction), at batch
		// priority so manual imports are served first (DEC-069).
		_, err = h.ingest(ctx, src, info.Name(), ingestOptions{Actor: actor, Priority: jobs.PriorityBatch, MaxBytes: limit})
		var dup *errDuplicate
		switch {
		case err == nil:
			enqueuedCount++
		case errors.As(err, &dup):
			duplicateCount++
		default:
			log.Printf("bulk import: %s: %v", info.Name(), err)
			errorCount++
		}
		return nil
	})

	if err != nil {
		http.Error(w, "Error during bulk directory traversal", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(BulkImportResponse{
		Message:    "Bulk import completed",
		Scanned:    scannedCount,
		Enqueued:   enqueuedCount,
		Duplicates: duplicateCount,
		Errors:     errorCount,
	})
}
