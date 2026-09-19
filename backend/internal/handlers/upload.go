package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// HandleUpload processes form-data, saves the PDF/EPUB and enqueues the task
func (h *UploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	// 1. Limit upload size (e.g., 50MB)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	// 2. Extract file with key 'document'
	file, header, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "Error reading uploaded file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 3. Validate file extension against supported formats
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !SupportedFormats[ext] {
		http.Error(w, "Unsupported file format", http.StatusBadRequest)
		return
	}

	// 4. Prepare target directory using resolved CODICE_STORAGE_PATH
	storagePath := resolveStoragePath()

	if err := os.MkdirAll(storagePath, 0755); err != nil {
		http.Error(w, "Error preparing uploads directory", http.StatusInternalServerError)
		return
	}

	// Isolate base filename to prevent directory traversal attacks
	safeFilename := filepath.Base(header.Filename)
	fileName := fmt.Sprintf("%d_%s", time.Now().Unix(), safeFilename)
	filePath := filepath.Join(storagePath, fileName)

	// 5. Save file to disk
	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Error saving file to disk", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(filePath)
		http.Error(w, "Error writing file content", http.StatusInternalServerError)
		return
	}
	dst.Close()

	var workID int
	query := `INSERT INTO works (original_title, file_path) VALUES ($1, $2) RETURNING id`

	err = h.DB.QueryRow(query, safeFilename, fileName).Scan(&workID)
	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Error creating database record", http.StatusInternalServerError)
		return
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Error resolving file path", http.StatusInternalServerError)
		return
	}

	// Enqueue task in Redis
	ctx := context.Background()
	err = h.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "ingestion_tasks",
		Values: map[string]interface{}{
			"file_path": absPath,
			"work_id":   workID,
		},
	}).Err()

	if err != nil {
		os.Remove(filePath)
		http.Error(w, "File saved, but error enqueuing task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Upload completed and enqueued",
		"work_id": workID,
	})
}

type BulkImportRequest struct {
	Directory string `json:"directory"`
}

type BulkImportResponse struct {
	Message  string `json:"message"`
	Scanned  int    `json:"scanned"`
	Enqueued int    `json:"enqueued"`
	Errors   int    `json:"errors"`
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

	var scannedCount, enqueuedCount, errorCount int
	ctx := context.Background()

	err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// A symlink named like a book could expose any file the server can
		// read; only regular files are imported.
		if !info.Mode().IsRegular() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !SupportedFormats[ext] {
			return nil
		}

		scannedCount++

		safeFilename := filepath.Base(info.Name())
		fileName := fmt.Sprintf("%d_%d_%s", time.Now().Unix(), scannedCount, safeFilename)
		dstPath := filepath.Join(storagePath, fileName)

		srcFile, err := os.Open(path)
		if err != nil {
			errorCount++
			return nil
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			errorCount++
			return nil
		}

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			dstFile.Close()
			os.Remove(dstPath)
			errorCount++
			return nil
		}
		dstFile.Close()

		var workID int
		query := `INSERT INTO works (original_title, file_path) VALUES ($1, $2) RETURNING id`
		if err := h.DB.QueryRow(query, safeFilename, fileName).Scan(&workID); err != nil {
			os.Remove(dstPath)
			errorCount++
			return nil
		}

		absDstPath, err := filepath.Abs(dstPath)
		if err != nil {
			os.Remove(dstPath)
			errorCount++
			return nil
		}

		if err := h.RedisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: "ingestion_tasks",
			Values: map[string]interface{}{
				"file_path": absDstPath,
				"work_id":   workID,
			},
		}).Err(); err != nil {
			os.Remove(dstPath)
			errorCount++
			return nil
		}

		enqueuedCount++
		return nil
	})

	if err != nil {
		http.Error(w, "Error during bulk directory traversal", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(BulkImportResponse{
		Message:  "Bulk import completed",
		Scanned:  scannedCount,
		Enqueued: enqueuedCount,
		Errors:   errorCount,
	})
}