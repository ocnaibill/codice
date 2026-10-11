package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// coverJPEG is a picture of the given size, as the page's cutting tool would send it.
func coverJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (s *catalogStack) sendCover(a actor, work int, content []byte) *httptest.ResponseRecorder {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if content != nil {
		fw, _ := mw.CreateFormFile("image", "capa.jpg")
		fw.Write(content)
	}
	mw.Close()
	req := httptest.NewRequest("PUT", fmt.Sprintf("/works/%d/cover", work), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func (s *catalogStack) coverFiles() []string {
	s.t.Helper()
	entries, _ := os.ReadDir(filepath.Join(s.storage, "covers"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (s *catalogStack) coverOf(work int) string {
	return s.scalar(`SELECT cover_url FROM editions WHERE work_id = $1 AND is_primary`, work)
}

// theFileGaveCover pretends the worker read a cover out of the file: it is shown and also remembered as the cover of the file.
func (s *catalogStack) theFileGaveCover(work int, name string) {
	s.t.Helper()
	os.MkdirAll(filepath.Join(s.storage, "covers"), 0o755)
	if err := os.WriteFile(filepath.Join(s.storage, "covers", name), []byte("x"), 0o644); err != nil {
		s.t.Fatal(err)
	}
	s.exec(`UPDATE editions SET cover_url = $2::text, auto_cover_url = $2::text WHERE work_id = $1 AND is_primary`, work, "/covers/"+name)
}

func TestWorkCover_AChosenCoverIsShownLockedAndAttributedToWhoChoseIt(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.theFileGaveCover(duna, "duna.jpg")

	rec := s.sendCover(admin, duna, coverJPEG(t, 600, 900))
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	url := s.coverOf(duna)
	if !strings.HasPrefix(url, fmt.Sprintf("/covers/cover_manual_%d_", duna)) || !strings.Contains(rec.Body.String(), url) {
		t.Fatalf("cover_url = %q, answer %s", url, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", filepath.Base(url))); err != nil {
		t.Errorf("the file of the cover: %v", err)
	}
	if got := s.scalar(`SELECT cover_lock::text FROM works WHERE id = $1`, duna); got != "true" {
		t.Errorf("cover_lock = %s: the worker would replace it", got)
	}
	if got := s.scalar(`SELECT auto_cover_url FROM editions WHERE work_id = $1 AND is_primary`, duna); got != "/covers/duna.jpg" {
		t.Errorf("the cover of the file was not kept: %q", got)
	}
	w, _ := s.detail(admin, duna)
	if w.CoverURL != url || w.Metadata == nil || w.Metadata.Sources["cover"] != "manual" || !w.Metadata.Locks["cover"] {
		t.Errorf("detail: cover %q, metadata %+v", w.CoverURL, w.Metadata)
	}
	if s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'work.cover.set' AND target_id = $1`, fmt.Sprint(duna)) != "1" {
		t.Errorf("the audit")
	}
	// The file of the library is not touched: the cover of the file stays on disk for going back to it.
	if _, err := os.Stat(filepath.Join(s.storage, "covers", "duna.jpg")); err != nil {
		t.Errorf("the cover of the file was removed: %v", err)
	}
}

func TestWorkCover_ChoosingAgainKeepsOnlyTheLastOneOfThisWork(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	other := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	s.sendCover(admin, other, coverJPEG(t, 600, 900))
	otherURL := s.coverOf(other)

	s.sendCover(admin, duna, coverJPEG(t, 600, 900))
	first := s.coverOf(duna)
	s.sendCover(admin, duna, coverJPEG(t, 300, 450))
	second := s.coverOf(duna)
	if first == second {
		t.Fatalf("the same name twice: %s", first)
	}
	files := strings.Join(s.coverFiles(), ",")
	if strings.Contains(files, filepath.Base(first)) || !strings.Contains(files, filepath.Base(second)) {
		t.Errorf("covers on disk: %s (first %s, second %s)", files, first, second)
	}
	if !strings.Contains(files, filepath.Base(otherURL)) {
		t.Errorf("the cover of another work was removed: %s", files)
	}
}

func TestWorkCover_WhatIsNotAProperCoverIsRefusedAndNothingIsKept(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.theFileGaveCover(duna, "duna.jpg")
	var asPNG bytes.Buffer
	png.Encode(&asPNG, image.NewRGBA(image.Rect(0, 0, 600, 900)))
	broken := coverJPEG(t, 600, 900)
	broken = append(broken[:len(broken)/2], bytes.Repeat([]byte{0}, 200)...)

	for name, c := range map[string]struct {
		body []byte
		code int
	}{
		"a PNG":                     {asPNG.Bytes(), http.StatusBadRequest},
		"text":                      {[]byte("not an image"), http.StatusBadRequest},
		"not 2:3":                   {coverJPEG(t, 600, 600), http.StatusBadRequest},
		"almost, but off by 5%":     {coverJPEG(t, 600, 945), http.StatusBadRequest},
		"too small":                 {coverJPEG(t, 100, 150), http.StatusBadRequest},
		"too large in pixels":       {coverJPEG(t, 2100, 3150), http.StatusBadRequest},
		"a good header, a bad body": {broken, http.StatusBadRequest},
		"too heavy":                 {append(coverJPEG(t, 600, 900), bytes.Repeat([]byte{0}, 3<<20)...), http.StatusRequestEntityTooLarge},
		"nothing sent":              {nil, http.StatusBadRequest},
	} {
		if rec := s.sendCover(admin, duna, c.body); rec.Code != c.code {
			t.Errorf("%s: %d, want %d", name, rec.Code, c.code)
		}
	}
	if rec := s.sendCover(admin, 999999, coverJPEG(t, 600, 900)); rec.Code != http.StatusNotFound {
		t.Errorf("a work that does not exist: %d", rec.Code)
	}
	if got := s.coverOf(duna); got != "/covers/duna.jpg" {
		t.Errorf("the cover changed: %s", got)
	}
	if got := s.scalar(`SELECT cover_lock::text FROM works WHERE id = $1`, duna); got != "false" {
		t.Errorf("cover_lock = %s", got)
	}
	if files := s.coverFiles(); len(files) != 1 {
		t.Errorf("something was kept: %v", files)
	}
}

func TestWorkCover_GoingBackReturnsTheCoverOfTheFileAndFreesTheWorker(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.theFileGaveCover(duna, "duna.jpg")
	s.sendCover(admin, duna, coverJPEG(t, 600, 900))
	manual := s.coverOf(duna)

	rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/cover", duna), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/covers/duna.jpg") {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.coverOf(duna); got != "/covers/duna.jpg" {
		t.Errorf("cover_url = %s", got)
	}
	if got := s.scalar(`SELECT cover_lock::text FROM works WHERE id = $1`, duna); got != "false" {
		t.Errorf("cover_lock = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_field_sources WHERE work_id = $1 AND field = 'cover'`, duna); got != "0" {
		t.Errorf("the cover still says it is by hand")
	}
	files := strings.Join(s.coverFiles(), ",")
	if strings.Contains(files, filepath.Base(manual)) || !strings.Contains(files, "duna.jpg") {
		t.Errorf("covers on disk: %s", files)
	}
	if s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'work.cover.reset'`) != "1" {
		t.Errorf("the audit")
	}
}

func TestWorkCover_ABookWithNoCoverOfItsOwnGoesBackToNone(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`UPDATE editions SET cover_url = NULL, auto_cover_url = NULL WHERE work_id = $1 AND is_primary`, duna)
	s.sendCover(admin, duna, coverJPEG(t, 600, 900))
	rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/cover", duna), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "placeholder.svg") {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT COALESCE(cover_url, 'none') FROM editions WHERE work_id = $1 AND is_primary`, duna); got != "none" {
		t.Errorf("cover_url = %s", got)
	}
	if rec := s.do(admin, "DELETE", "/works/999999/cover", ""); rec.Code != http.StatusNotFound {
		t.Errorf("a work that does not exist: %d", rec.Code)
	}
}

func TestWorkCover_GoingBackNeverRemovesWhatTheWorkerOwns(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	// The lock was set by hand, with the cover of the file in use; there is no cover chosen by hand to remove.
	s.theFileGaveCover(duna, "duna.jpg")
	s.exec(`UPDATE works SET cover_lock = TRUE WHERE id = $1`, duna)
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/cover", duna), ""); rec.Code != http.StatusOK {
		t.Fatalf("reset: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", "duna.jpg")); err != nil {
		t.Errorf("the cover of the file was removed: %v", err)
	}
	if got := s.scalar(`SELECT cover_lock::text FROM works WHERE id = $1`, duna); got != "false" {
		t.Errorf("cover_lock = %s", got)
	}
}

func TestWorkCover_ACutAPixelOffIsNotRefused(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	// A cut in a browser is a pixel off now and then (the frame is not always a whole number of pixels at the scale of the screen).
	for _, h := range []int{899, 901} {
		if rec := s.sendCover(admin, duna, coverJPEG(t, 600, h)); rec.Code != http.StatusOK {
			t.Errorf("600×%d: %d", h, rec.Code)
		}
	}
}

func TestWorkCover_TheCoverOfAnotherWorkIsNeverRemovedByThisOne(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	other := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	s.sendCover(admin, other, coverJPEG(t, 600, 900))
	theirs := s.coverOf(other)
	// By some mistake (two works put under one) the work shows a cover that was chosen for another.
	s.exec(`UPDATE editions SET cover_url = $2 WHERE work_id = $1 AND is_primary`, duna, theirs)
	if rec := s.sendCover(admin, duna, coverJPEG(t, 600, 900)); rec.Code != http.StatusOK {
		t.Fatalf("set: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", filepath.Base(theirs))); err != nil {
		t.Errorf("the cover of the other work was removed: %v", err)
	}
}
