package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// MediaHandler serves text files and audio streams with Range support
type MediaHandler struct {
	DB *sql.DB
}

// ServeText serves TXT/MD files as plain text
func (h *MediaHandler) ServeText(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filePath sql.NullString
	err := h.DB.QueryRow("SELECT file_path FROM works WHERE id = $1", id).Scan(&filePath)
	if err != nil || !filePath.Valid || filePath.String == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	fullPath := filepath.Join(storagePath, filePath.String)

	// Prevent directory traversal
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(storagePath)) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	f, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Error reading file info", http.StatusInternalServerError)
		return
	}

	// Detect content type
	ext := strings.ToLower(filepath.Ext(fullPath))
	contentType := "text/plain"
	if ext == ".md" {
		contentType = "text/markdown"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=604800, must-revalidate")
	http.ServeContent(w, r, filepath.Base(fullPath), stat.ModTime(), f)
}

// ServeAudio streams audio files with HTTP Range support
func (h *MediaHandler) ServeAudio(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filePath sql.NullString
	err := h.DB.QueryRow("SELECT file_path FROM works WHERE id = $1", id).Scan(&filePath)
	if err != nil || !filePath.Valid || filePath.String == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	fullPath := filepath.Join(storagePath, filePath.String)

	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(storagePath)) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	f, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Error reading file info", http.StatusInternalServerError)
		return
	}

	// Detect audio content type
	ext := strings.ToLower(filepath.Ext(fullPath))
	contentType := "audio/mpeg"
	switch ext {
	case ".mp3":
		contentType = "audio/mpeg"
	case ".m4a", ".m4b":
		contentType = "audio/mp4"
	case ".ogg":
		contentType = "audio/ogg"
	case ".wav":
		contentType = "audio/wav"
	case ".flac":
		contentType = "audio/flac"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=604800, must-revalidate")
	http.ServeContent(w, r, filepath.Base(fullPath), stat.ModTime(), f)
}

// readFileContent reads small text files into memory
func readFileContent(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}