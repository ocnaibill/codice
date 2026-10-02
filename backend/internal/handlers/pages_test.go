package handlers

import (
	"archive/zip"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestCoverPlaceholder_IsAPictureThatCannotRunAnything(t *testing.T) {
	rec := httptest.NewRecorder()
	CoverPlaceholder(rec, httptest.NewRequest("GET", "/covers/placeholder.svg", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(rec.Body.String(), "<svg") {
		t.Fatalf("%d %q %.30q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "sandbox") {
		t.Errorf("the picture must not be able to run anything: %q", csp)
	}
	if strings.Contains(rec.Body.String(), "<script") {
		t.Error("no script in the picture")
	}
}

// storedRAR builds a RAR 4 archive in which every file is stored, not compressed: enough for bsdtar to read, and the
// only way to have a CBR to test with, since nothing here can write the format.
func storedRAR(t *testing.T, files [][2]string) []byte {
	t.Helper()
	crc16 := func(b []byte) uint16 { return uint16(crc32.ChecksumIEEE(b)) }
	le16 := func(v uint16) []byte { return []byte{byte(v), byte(v >> 8)} }
	le32 := func(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }
	out := []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x00} // the marker
	main := append([]byte{0x73}, le16(0)...)                // MAIN_HEAD, no flags
	main = append(main, le16(13)...)
	main = append(main, 0, 0, 0, 0, 0, 0) // the two reserved fields
	out = append(append(out, le16(crc16(main))...), main...)
	for _, f := range files {
		name, data := []byte(f[0]), []byte(f[1])
		h := append([]byte{0x74}, le16(0x8000)...) // FILE_HEAD, with the size of the data after it
		h = append(h, le16(uint16(32+len(name)))...)
		h = append(h, le32(uint32(len(data)))...) // packed size
		h = append(h, le32(uint32(len(data)))...) // unpacked size
		h = append(h, 3)                          // made on Unix
		h = append(h, le32(crc32.ChecksumIEEE(data))...)
		h = append(h, le32(0x5a21a000)...) // the time
		h = append(h, 20, 0x30)            // version 2.0, method "store"
		h = append(h, le16(uint16(len(name)))...)
		h = append(h, le32(0o644)...)
		h = append(h, name...)
		out = append(append(out, le16(crc16(h))...), h...)
		out = append(out, data...)
	}
	return append(out, 0xC4, 0x3D, 0x7B, 0x00, 0x40, 0x07, 0x00) // the end
}

func needBsdtar(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bsdtar"); err != nil {
		t.Skip("bsdtar is not installed here (it is in the backend image)")
	}
}

func writeCBR(t *testing.T, files [][2]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "comic.cbr")
	if err := os.WriteFile(path, storedRAR(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractCBR_KeepsTheImagesInReadingOrderAndNothingElse(t *testing.T) {
	needBsdtar(t)
	cbr := writeCBR(t, [][2]string{
		{"b/002.jpg", "page-b2"},
		{"a/001.jpg", "page-a1"},
		{"a/002.png", "page-a2"},
		{"b/001.webp", "page-b1"},
		{"ComicInfo.xml", "<ComicInfo/>"},
		{"a/Thumbs.db", "junk"},
	})
	cache := filepath.Join(t.TempDir(), "cache", "pages", "7")
	if err := extractCBR(cbr, cache); err != nil {
		t.Fatal(err)
	}
	pages, err := listCachedPages(cbr, "7", filepath.Dir(filepath.Dir(filepath.Dir(cache))))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range pages {
		data, _ := os.ReadFile(filepath.Join(cache, p.FileName))
		got = append(got, string(data))
	}
	// Folder a before folder b, the pages of each in order, and no ComicInfo or thumbnails among them.
	if strings.Join(got, ",") != "page-a1,page-a2,page-b1,page-b2" {
		t.Errorf("pages: %v", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(cache))
	if len(entries) != 1 {
		t.Errorf("work directories were left behind: %v", entries)
	}
}

func TestExtractCBR_PagesAreOrderedByTheirPathInTheArchive(t *testing.T) {
	needBsdtar(t)
	// "a.jpg" sorts before "a/1.jpg" as text; the order is the one of the paths, not of the walk of the directories.
	cbr := writeCBR(t, [][2]string{{"a/1.jpg", "second"}, {"a.jpg", "first"}, {"b.jpg", "third"}})
	cache := filepath.Join(t.TempDir(), "cache", "pages", "2")
	if err := extractCBR(cbr, cache); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(cache)
	var got []string
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(cache, e.Name()))
		got = append(got, string(data))
	}
	if strings.Join(got, ",") != "first,second,third" {
		t.Errorf("order: %v", got)
	}
}

func TestExtractCBR_TwoFoldersWithTheSameFileNamesKeepAllPages(t *testing.T) {
	needBsdtar(t)
	cbr := writeCBR(t, [][2]string{{"ch1/001.jpg", "one"}, {"ch2/001.jpg", "two"}, {"ch3/001.jpg", "three"}})
	cache := filepath.Join(t.TempDir(), "cache", "pages", "1")
	if err := extractCBR(cbr, cache); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) != 3 {
		t.Fatalf("a page was lost to a name that repeats: %d files", len(entries))
	}
}

func TestExtractCBR_AnArchiveWithNoImagesOrNoArchiveFailsAndLeavesNoCache(t *testing.T) {
	needBsdtar(t)
	for name, path := range map[string]string{
		"no images": writeCBR(t, [][2]string{{"ComicInfo.xml", "<ComicInfo/>"}}),
		"not an archive": func() string {
			p := filepath.Join(t.TempDir(), "x.cbr")
			os.WriteFile(p, []byte("this is not a RAR"), 0o644)
			return p
		}(),
		"missing": filepath.Join(t.TempDir(), "nope.cbr"),
	} {
		cache := filepath.Join(t.TempDir(), "cache", "pages", "9")
		err := extractCBR(path, cache)
		if err == nil {
			t.Errorf("%s: no error", name)
		}
		// What failed is said: the reader of the log sees the extractor's own words, or that there were no images.
		if err != nil && name != "no images" && !strings.Contains(err.Error(), "bsdtar failed") {
			t.Errorf("%s: %v", name, err)
		}
		if err != nil && name == "no images" && !strings.Contains(err.Error(), "no images") {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := os.Stat(cache); err == nil {
			t.Errorf("%s: a cache was left", name)
		}
		if entries, _ := os.ReadDir(filepath.Dir(cache)); len(entries) != 0 {
			t.Errorf("%s: work directories were left: %v", name, entries)
		}
	}
}

func TestExtractCBR_ANameThatClimbsOutOfTheDirectoryWritesNothingOutside(t *testing.T) {
	needBsdtar(t)
	base := t.TempDir()
	cbr := writeCBR(t, [][2]string{{"../../evil.jpg", "evil"}, {"001.jpg", "ok"}})
	cache := filepath.Join(base, "cache", "pages", "3")
	_ = extractCBR(cbr, cache) // it may refuse the whole archive or only the entry
	filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.Contains(path, "evil") && !strings.HasPrefix(path, cache) {
			t.Errorf("a file was written outside the cache: %s", path)
		}
		return nil
	})
	if _, err := os.Stat(filepath.Join(base, "evil.jpg")); err == nil {
		t.Error("evil.jpg escaped")
	}
}

func TestExtractCBR_TwoExtractionsAtOnceLeaveOneCompleteCache(t *testing.T) {
	needBsdtar(t)
	cbr := writeCBR(t, [][2]string{{"001.jpg", "a"}, {"002.jpg", "b"}, {"003.jpg", "c"}})
	cache := filepath.Join(t.TempDir(), "cache", "pages", "5")
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { errs <- extractCBR(cbr, cache) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) != 3 {
		t.Errorf("the cache has %d pages", len(entries))
	}
	if siblings, _ := os.ReadDir(filepath.Dir(cache)); len(siblings) != 1 {
		t.Errorf("work directories were left: %v", siblings)
	}
}
