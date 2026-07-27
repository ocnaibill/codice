package handlers

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Test listCBZPages with a synthetic CBZ file
func createTestCBZ(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test.cbz")
	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("failed to create test cbz: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry: %v", err)
		}
		entry.Write([]byte(content))
	}
	w.Close()
	return tmpFile
}

func TestListCBZPages_ListsImagesOnly(t *testing.T) {
	zipPath := createTestCBZ(t, map[string]string{
		"page001.jpg":  "fake-jpg-data",
		"page002.jpg":  "fake-jpg-data",
		"page003.png":  "fake-png-data",
		"metadata.xml": "<xml></xml>",
		"cover.webp":   "fake-webp-data",
	})
	pages, err := listCBZPages(zipPath)
	if err != nil {
		t.Fatalf("listCBZPages failed: %v", err)
	}
	if len(pages) != 4 {
		t.Errorf("expected 4 images, got %d", len(pages))
	}
}

func TestListCBZPages_ReturnsSorted(t *testing.T) {
	zipPath := createTestCBZ(t, map[string]string{
		"003.jpg": "data",
		"001.jpg": "data",
		"002.jpg": "data",
	})
	pages, err := listCBZPages(zipPath)
	if err != nil {
		t.Fatalf("listCBZPages failed: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
	if pages[0].FileName != "001.jpg" {
		t.Errorf("expected first page 001.jpg, got %s", pages[0].FileName)
	}
}

func TestListCBZPages_EmptyArchive(t *testing.T) {
	zipPath := createTestCBZ(t, map[string]string{})
	pages, err := listCBZPages(zipPath)
	if err != nil {
		t.Fatalf("listCBZPages failed: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages for empty archive, got %d", len(pages))
	}
}

func TestListCBZPages_InvalidZip(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "invalid.cbz")
	os.WriteFile(tmpFile, []byte("not a zip file"), 0644)
	_, err := listCBZPages(tmpFile)
	if err == nil {
		t.Error("expected error for invalid zip, got nil")
	}
}

func TestServePageFromZip_ServesCorrectPage(t *testing.T) {
	zipPath := createTestCBZ(t, map[string]string{
		"001.jpg": "page-one-data",
		"002.jpg": "page-two-data",
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	servePageFromZip(rec, req, zipPath, 0)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body != "page-one-data" {
		t.Errorf("expected 'page-one-data', got '%s'", body)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestServePageFromZip_PageNotFound(t *testing.T) {
	zipPath := createTestCBZ(t, map[string]string{
		"001.jpg": "data",
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	servePageFromZip(rec, req, zipPath, 999)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for out-of-range page, got %d", rec.Code)
	}
}

func TestServePageFromZip_InvalidZip(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "bad.cbz")
	os.WriteFile(tmpFile, []byte("garbage"), 0644)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	servePageFromZip(rec, req, tmpFile, 0)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for invalid zip, got %d", rec.Code)
	}
}