package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

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

	// 3. Validate file extension (PDF, EPUB, or CBZ)
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" && ext != ".epub" && ext != ".cbz" {
		http.Error(w, "Unsupported file format. Please upload PDF, EPUB, or CBZ only.", http.StatusBadRequest)
		return
	}

	// 4. Prepare target directory using CODICE_STORAGE_PATH (fallback to ./uploads)
	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

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

	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	targetDir := req.Directory
	if targetDir == "" {
		targetDir = filepath.Join(storagePath, "import")
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		os.MkdirAll(targetDir, 0755)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BulkImportResponse{
			Message:  fmt.Sprintf("Import directory created at '%s'. Place files there and run bulk import again.", targetDir),
			Scanned:  0,
			Enqueued: 0,
			Errors:   0,
		})
		return
	}

	var scannedCount, enqueuedCount, errorCount int
	ctx := context.Background()

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext != ".pdf" && ext != ".epub" && ext != ".cbz" {
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