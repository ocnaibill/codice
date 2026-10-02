package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// PageInfo represents a single page in a comic archive
type PageInfo struct {
	Number   int    `json:"number"`
	FileName string `json:"fileName"`
	URL      string `json:"url"`
}

// PageHandler stores database and storage dependencies
type PageHandler struct {
	DB *sql.DB
}

// validImageExts is the set of image extensions accepted for pages.
var validImageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

// GetPages returns the list of pages in a work's file
func (h *PageHandler) GetPages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	filePath, err := workFilePath(h.DB, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Work not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error fetching work", http.StatusInternalServerError)
		return
	}

	if !filePath.Valid || filePath.String == "" {
		http.Error(w, "Work has no file", http.StatusNotFound)
		return
	}

	fullPath := filePath.String         // absolute, resolved from the database
	storagePath := resolveStoragePath() // where the CBR page cache lives

	// Only CBZ/CBR support page listing for now
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext != ".cbz" && ext != ".cbr" {
		http.Error(w, "Format does not support page listing", http.StatusBadRequest)
		return
	}

	var pages []PageInfo
	switch ext {
	case ".cbz":
		pages, err = listCBZPages(fullPath)
	case ".cbr":
		pages, err = listCachedPages(fullPath, id, storagePath)
	}
	if err != nil {
		log.Printf("Error listing pages in %s: %v", fullPath, err)
		http.Error(w, "Error reading archive", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pages)
}

// ServePage streams a single page image from the archive
func (h *PageHandler) ServePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pageParam := chi.URLParam(r, "page")

	pageNum, err := strconv.Atoi(pageParam)
	if err != nil || pageNum < 0 {
		http.Error(w, "Invalid page number", http.StatusBadRequest)
		return
	}

	filePath, err := workFilePath(h.DB, id)
	if err != nil {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}

	if !filePath.Valid || filePath.String == "" {
		http.Error(w, "Work has no file", http.StatusNotFound)
		return
	}

	fullPath := filePath.String         // absolute, resolved from the database
	storagePath := resolveStoragePath() // where the CBR page cache lives
	ext := strings.ToLower(filepath.Ext(fullPath))

	switch ext {
	case ".cbz":
		servePageFromZip(w, r, fullPath, pageNum)
	case ".cbr":
		servePageFromCache(w, r, id, storagePath, pageNum)
	default:
		http.Error(w, "Format does not support page serving", http.StatusBadRequest)
	}
}

// ServePageThumbnail streams a reduced thumbnail for a page
func (h *PageHandler) ServePageThumbnail(w http.ResponseWriter, r *http.Request) {
	// TODO: generate actual thumbnails server-side and cache them
	id := chi.URLParam(r, "id")
	pageParam := chi.URLParam(r, "page")

	pageNum, err := strconv.Atoi(pageParam)
	if err != nil || pageNum < 0 {
		http.Error(w, "Invalid page number", http.StatusBadRequest)
		return
	}

	filePath, err := workFilePath(h.DB, id)
	if err != nil {
		http.Error(w, "Work not found", http.StatusNotFound)
		return
	}

	if !filePath.Valid || filePath.String == "" {
		http.Error(w, "Work has no file", http.StatusNotFound)
		return
	}

	fullPath := filePath.String         // absolute, resolved from the database
	storagePath := resolveStoragePath() // where the CBR page cache lives
	ext := strings.ToLower(filepath.Ext(fullPath))

	switch ext {
	case ".cbz":
		servePageFromZip(w, r, fullPath, pageNum)
	case ".cbr":
		servePageFromCache(w, r, id, storagePath, pageNum)
	default:
		http.Error(w, "Format does not support page serving", http.StatusBadRequest)
	}
}

// --- CBZ: direct ZIP access ---

func listCBZPages(zipPath string) ([]PageInfo, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer reader.Close()

	var images []string

	for _, f := range reader.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if validImageExts[ext] && !f.FileInfo().IsDir() {
			images = append(images, f.Name)
		}
	}

	sort.Slice(images, func(i, j int) bool {
		return strings.Compare(images[i], images[j]) < 0
	})

	pages := make([]PageInfo, 0, len(images))
	for i, name := range images {
		pages = append(pages, PageInfo{
			Number:   i,
			FileName: name,
			URL:      fmt.Sprintf("/pages/%d", i),
		})
	}

	return pages, nil
}

func servePageFromZip(w http.ResponseWriter, r *http.Request, zipPath string, pageNum int) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		http.Error(w, "Error reading archive", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	var images []string

	for _, f := range reader.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if validImageExts[ext] && !f.FileInfo().IsDir() {
			images = append(images, f.Name)
		}
	}

	sort.Slice(images, func(i, j int) bool {
		return strings.Compare(images[i], images[j]) < 0
	})

	if pageNum < 0 || pageNum >= len(images) {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}

	entryName := images[pageNum]
	for _, f := range reader.File {
		if f.Name == entryName {
			rc, err := f.Open()
			if err != nil {
				http.Error(w, "Error reading page", http.StatusInternalServerError)
				return
			}
			defer rc.Close()

			// Read decompressed content into buffer for Range support
			data, err := io.ReadAll(rc)
			if err != nil {
				http.Error(w, "Error reading page data", http.StatusInternalServerError)
				return
			}

			// Detect content type from extension
			ext := strings.ToLower(filepath.Ext(entryName))
			contentType := "image/jpeg"
			switch ext {
			case ".png":
				contentType = "image/png"
			case ".webp":
				contentType = "image/webp"
			}

			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "public, max-age=604800, must-revalidate")
			http.ServeContent(w, r, entryName, f.Modified, bytes.NewReader(data))
			return
		}
	}

	http.Error(w, "Page not found", http.StatusNotFound)
}

// --- CBR: extract to cache on first access ---

func listCachedPages(rarPath string, workID string, storagePath string) ([]PageInfo, error) {
	cacheDir := filepath.Join(storagePath, "cache", "pages", workID)

	// If cache doesn't exist, extract it with bsdtar
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := extractCBR(rarPath, cacheDir); err != nil {
			return nil, fmt.Errorf("failed to extract CBR: %w", err)
		}
	}

	// List images from cache directory
	var images []string

	filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if validImageExts[ext] {
			images = append(images, info.Name())
		}
		return nil
	})

	sort.Strings(images)

	pages := make([]PageInfo, 0, len(images))
	for i, name := range images {
		pages = append(pages, PageInfo{
			Number:   i,
			FileName: name,
			URL:      fmt.Sprintf("/pages/%d", i),
		})
	}

	return pages, nil
}

// cbrExtractTimeout bounds one extraction: a comic is a few hundred images, not a day of work.
const cbrExtractTimeout = 2 * time.Minute

// extractCBR unpacks the pages of a CBR into cacheDir, flat and in reading order, with bsdtar (libarchive: it reads
// RAR, is free, and is what the worker uses for the same files). The archive goes first into a directory of its own,
// where only the images are kept, named by their place in the order (00000_page.jpg) so that two folders with a
// "001.jpg" each do not overwrite one another; then that directory takes the place of cacheDir in one step, so a
// second request never sees a half-extracted cache. bsdtar itself refuses a name that climbs out of the directory
// ("..") or goes through a link.
func extractCBR(rarPath string, cacheDir string) error {
	parent := filepath.Dir(cacheDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}
	raw, err := os.MkdirTemp(parent, ".unpack-*")
	if err != nil {
		return fmt.Errorf("failed to create a work dir: %w", err)
	}
	defer os.RemoveAll(raw)

	ctx, cancel := context.WithTimeout(context.Background(), cbrExtractTimeout)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "bsdtar", "-xf", rarPath, "-C", raw).CombinedOutput(); err != nil {
		return fmt.Errorf("bsdtar failed: %s - %w", strings.TrimSpace(string(output)), err)
	}

	// The images, by their path in the archive (a folder per chapter sorts before the next one).
	var images []string
	filepath.Walk(raw, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() || !validImageExts[strings.ToLower(filepath.Ext(info.Name()))] {
			return nil
		}
		rel, err := filepath.Rel(raw, path)
		if err == nil {
			images = append(images, rel)
		}
		return nil
	})
	if len(images) == 0 {
		return errors.New("the archive has no images")
	}
	sort.Strings(images)

	flat, err := os.MkdirTemp(parent, ".pages-*")
	if err != nil {
		return fmt.Errorf("failed to create a work dir: %w", err)
	}
	for i, rel := range images {
		name := fmt.Sprintf("%05d_%s", i, filepath.Base(rel))
		if err := os.Rename(filepath.Join(raw, rel), filepath.Join(flat, name)); err != nil {
			os.RemoveAll(flat)
			return fmt.Errorf("failed to keep a page: %w", err)
		}
	}
	// Another request may have extracted it meanwhile: then its cache stands and ours is dropped.
	if err := os.Rename(flat, cacheDir); err != nil {
		os.RemoveAll(flat)
		if _, statErr := os.Stat(cacheDir); statErr != nil {
			return fmt.Errorf("failed to publish the cache: %w", err)
		}
	}
	return nil
}

func servePageFromCache(w http.ResponseWriter, r *http.Request, workID string, storagePath string, pageNum int) {
	cacheDir := filepath.Join(storagePath, "cache", "pages", workID)

	// Ensure extracted
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		http.Error(w, "Pages not yet extracted. Call GetPages first.", http.StatusNotFound)
		return
	}

	// List and sort images
	var images []string
	filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if validImageExts[ext] {
			images = append(images, info.Name())
		}
		return nil
	})
	sort.Strings(images)

	if pageNum < 0 || pageNum >= len(images) {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}

	pagePath := filepath.Join(cacheDir, images[pageNum])

	// Detect content type from extension
	ext := strings.ToLower(filepath.Ext(images[pageNum]))
	contentType := "image/jpeg"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".webp":
		contentType = "image/webp"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=604800, must-revalidate")
	http.ServeFile(w, r, pagePath)
}
