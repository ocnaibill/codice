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

	// 4. Prepare target directory and sanitize filename against path traversal
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		http.Error(w, "Error preparing uploads directory", http.StatusInternalServerError)
		return
	}

	// Isolate base filename to prevent directory traversal attacks
	safeFilename := filepath.Base(header.Filename)
	fileName := fmt.Sprintf("%d_%s", time.Now().Unix(), safeFilename)
	filePath := filepath.Join(uploadDir, fileName)

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