package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (s *catalogStack) sendImage(a actor, col int64, what string, content []byte) *httptest.ResponseRecorder {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if content != nil {
		fw, _ := mw.CreateFormFile("image", "x.jpg")
		fw.Write(content)
	}
	mw.Close()
	req := httptest.NewRequest("PUT", fmt.Sprintf("/collections/%d/%s", col, what), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func (s *catalogStack) collectionJSON(col int64) Collection {
	s.t.Helper()
	d, code := s.collection(ana, col)
	if code != 200 {
		s.t.Fatalf("collection %d: %d", col, code)
	}
	return d.Collection
}

func TestCollectionImages_AChosenCoverIsTheOneTheSeriesShowsEverywhere(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Naruto", "volume", "volume")
	s.theFileGaveCover(ids[0], "naruto1.jpg")

	if got := s.collectionJSON(col); got.CoverURL != "/covers/naruto1.jpg" || got.CoverIsManual {
		t.Fatalf("before: %q manual=%v, want the cover of the first work", got.CoverURL, got.CoverIsManual)
	}
	rec := s.sendImage(admin, col, "cover", coverJPEG(t, 600, 900))
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	var set struct{ URL string }
	json.Unmarshal(rec.Body.Bytes(), &set)
	if !strings.HasPrefix(set.URL, fmt.Sprintf("/covers/cover_manual_c%d_", col)) {
		t.Fatalf("url = %q", set.URL)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", filepath.Base(set.URL))); err != nil {
		t.Errorf("the file is not kept: %v", err)
	}

	// The page of the series, the list of collections, the card of the series in the library and the feed of the series all show it.
	if got := s.collectionJSON(col); got.CoverURL != set.URL || !got.CoverIsManual {
		t.Errorf("page: %q manual=%v", got.CoverURL, got.CoverIsManual)
	}
	var list struct{ Data []Collection }
	json.Unmarshal(s.do(ana, "GET", "/collections", "").Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].CoverURL != set.URL {
		t.Errorf("list: %+v", list.Data)
	}
	var found string
	for _, w := range s.collapsed(ana, "").Data {
		if w.Collapsed != nil {
			found = w.Collapsed.CoverURL
		}
	}
	if found != set.URL {
		t.Errorf("card of the series = %q", found)
	}
	if feed := s.do(ana, "GET", "/opds/series", "").Body.String(); !strings.Contains(feed, set.URL) {
		t.Errorf("the feed of the series does not carry the picture:\n%s", feed)
	}
	// What the works show is theirs and did not change.
	if got := s.coverOf(ids[0]); got != "/covers/naruto1.jpg" {
		t.Errorf("the cover of the work = %q", got)
	}

	// Another one replaces it, and the first file goes; back to automatic removes the file and shows the work's cover again.
	var second struct{ URL string }
	json.Unmarshal(s.sendImage(admin, col, "cover", coverJPEG(t, 400, 600)).Body.Bytes(), &second)
	if second.URL == "" || second.URL == set.URL {
		t.Fatalf("second = %q", second.URL)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", filepath.Base(set.URL))); !os.IsNotExist(err) {
		t.Errorf("the replaced picture is still kept: %v", err)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/cover", col), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: %d", rec.Code)
	}
	if got := s.collectionJSON(col); got.CoverURL != "/covers/naruto1.jpg" || got.CoverIsManual {
		t.Errorf("after: %q manual=%v", got.CoverURL, got.CoverIsManual)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", filepath.Base(second.URL))); !os.IsNotExist(err) {
		t.Errorf("the picture put away is still kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", "naruto1.jpg")); err != nil {
		t.Errorf("the cover of the work was touched: %v", err)
	}
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE target_type = 'collection'`); got != "collection.cover.set,collection.cover.set,collection.cover.reset" {
		t.Errorf("audit = %q", got)
	}
}

func TestCollectionImages_TheBannerIsWideAndOptional(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.seriesOf("Naruto", "volume")
	if got := s.collectionJSON(col); got.BannerURL != "" {
		t.Fatalf("a series has no banner until somebody chooses: %q", got.BannerURL)
	}
	if rec := s.sendImage(admin, col, "banner", coverJPEG(t, 600, 900)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "3:1") {
		t.Errorf("a cover as a banner: %d %s", rec.Code, rec.Body.String())
	}
	rec := s.sendImage(admin, col, "banner", coverJPEG(t, 1500, 500))
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	got := s.collectionJSON(col)
	if !strings.HasPrefix(got.BannerURL, fmt.Sprintf("/covers/cover_manual_banner_c%d_", col)) || got.CoverIsManual {
		t.Errorf("banner = %q, cover manual = %v", got.BannerURL, got.CoverIsManual)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/banner", col), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: %d", rec.Code)
	}
	if got := s.collectionJSON(col); got.BannerURL != "" {
		t.Errorf("banner after = %q", got.BannerURL)
	}
	if left := s.coverFiles(); len(left) != 0 {
		t.Errorf("files left: %v", left)
	}
}

func TestCollectionImages_RefusesWhatIsNotAGoodPictureOrASeriesOfTheStaff(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.seriesOf("Naruto", "volume")
	for name, c := range map[string]struct {
		body []byte
		code int
	}{
		"no picture":   {nil, http.StatusBadRequest},
		"not a jpeg":   {[]byte("GIF89a not a jpeg"), http.StatusBadRequest},
		"wrong ratio":  {coverJPEG(t, 600, 600), http.StatusBadRequest},
		"too narrow":   {coverJPEG(t, 100, 150), http.StatusBadRequest},
		"a good cover": {coverJPEG(t, 600, 900), http.StatusOK},
	} {
		if rec := s.sendImage(admin, col, "cover", c.body); rec.Code != c.code {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body.String(), c.code)
		}
	}
	if rec := s.sendImage(admin, 99999, "cover", coverJPEG(t, 600, 900)); rec.Code != http.StatusNotFound {
		t.Errorf("a series that does not exist: %d", rec.Code)
	}
	// A personal list is not a series of the library: it takes no picture here.
	s.exec(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Minha', 'manual')`, idAna)
	list := s.scalar(`SELECT id FROM collections WHERE kind = 'personal'`)
	var id int64
	fmt.Sscan(list, &id)
	if rec := s.sendImage(admin, id, "cover", coverJPEG(t, 600, 900)); rec.Code != http.StatusNotFound {
		t.Errorf("a personal list: %d", rec.Code)
	}
	// A retired series is restored before it is changed.
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	if rec := s.sendImage(admin, col, "cover", coverJPEG(t, 600, 900)); rec.Code != http.StatusConflict {
		t.Errorf("a retired series: %d", rec.Code)
	}
}
