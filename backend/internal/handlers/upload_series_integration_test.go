package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// uploadIn sends a file with the fields that say where its work belongs, in the order the form of the bulk upload writes them.
func (s *catalogStack) uploadIn(a actor, filename string, content []byte, fields [][2]string) *httptest.ResponseRecorder {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range fields {
		mw.WriteField(f[0], f[1])
	}
	fw, _ := mw.CreateFormFile("document", filename)
	fw.Write(content)
	mw.Close()
	req := httptest.NewRequest("POST", "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func chapter(n, title string) [][2]string {
	return [][2]string{{"series", "Jujutsu Kaisen Modelo"}, {"series_index", n}, {"unit", "chapter"}, {"comic_kind", "manga"}, {"title", title}}
}

func workOf(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	var resp struct {
		WorkID int `json:"work_id"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || resp.WorkID == 0 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	return resp.WorkID
}

func TestUpload_AChapterOfASeriesIsPlacedAndLockedAtOnce(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.uploadIn(admin, "Jujutsu Kaisen Modelo - Cap 1.cbz", corpusFile(t, "cbz_ltr.cbz"), chapter("1", "Jujutsu Kaisen Modelo - Cap. 1"))
	id := workOf(t, rec)
	var answered struct {
		CollectionID int64 `json:"collection_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &answered)
	if got := s.scalar(`SELECT collection_id FROM collection_works WHERE work_id = $1 AND official`, id); got != fmt.Sprint(answered.CollectionID) || answered.CollectionID == 0 {
		t.Errorf("collection_id = %d, want the collection %s", answered.CollectionID, got)
	}

	got := s.scalar(`SELECT original_title || '|' || series || '|' || series_index || '|' || unit || '|' || comic_kind || '|' || title_lock || '|' || series_lock FROM works WHERE id = $1`, id)
	if want := "Jujutsu Kaisen Modelo - Cap. 1|Jujutsu Kaisen Modelo|1|chapter|manga|true|true"; got != want {
		t.Errorf("work = %q, want %q", got, want)
	}
	// Written as a person writes it: the provenance says so, and the work is in the official collection of the series, at its number.
	if got := s.scalar(`SELECT string_agg(field || ':' || source, ',' ORDER BY field) FROM work_field_sources WHERE work_id = $1 AND field IN ('title', 'series', 'series_index')`, id); got == "" {
		t.Errorf("no provenance for the fields written by hand")
	}
	if got := s.scalar(`SELECT c.name || '|' || cw.position || '|' || COALESCE(c.reading_direction, '') FROM collection_works cw JOIN collections c ON c.id = cw.collection_id WHERE cw.work_id = $1 AND cw.official`, id); got != "Jujutsu Kaisen Modelo|1|rtl" {
		t.Errorf("collection = %q, want the series at number 1 read right to left", got)
	}
}

func TestUpload_ANumberAlreadyInTheSeriesIsSkippedAndNamesTheHolder(t *testing.T) {
	s := newCatalogStack(t)
	s.uploadIn(admin, "c1.cbz", corpusFile(t, "cbz_ltr.cbz"), chapter("1", "Cap. 1"))
	two := workOf(t, s.uploadIn(admin, "c2.cbz", corpusFile(t, "cbz_rtl.cbz"), chapter("2", "Cap. 2")))
	before := s.counts()

	// Other bytes, same series, unit and number (written another way): not stored, and the answer says who holds it.
	again := chapter("2", "Cap. 2 outra vez")
	again[0][1] = "  jujutsu   KAISEN modelo "
	rec := s.uploadIn(admin, "c2b.cbz", corpusFile(t, "cbz_webtoon_imagem_longa.cbz"), again)
	if rec.Code != http.StatusConflict {
		t.Fatalf("repeated number: %d %s, want 409", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error  string `json:"error"`
		WorkID int    `json:"work_id"`
		Title  string `json:"title"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error != "in_series" || resp.WorkID != two || resp.Title != "Cap. 2" {
		t.Errorf("answer = %+v, want in_series held by work %d", resp, two)
	}
	if after := s.counts(); after != before {
		t.Errorf("a skipped chapter changed the library: %s -> %s", before, after)
	}
	if n := len(s.stored()); n != 2 {
		t.Errorf("%d files in storage, want the 2 chapters", n)
	}

	// Another unit with the same number is another work (volume 2 is not chapter 2), and so is a retired holder's number.
	vol := chapter("2", "Vol. 2")
	vol[2][1] = "volume"
	workOf(t, s.uploadIn(admin, "v2.cbz", corpusFile(t, "cbz_webtoon_imagem_longa.cbz"), vol))
}

func TestUpload_ARetiredChapterGivesItsNumberBack(t *testing.T) {
	s := newCatalogStack(t)
	one := workOf(t, s.uploadIn(admin, "c1.cbz", corpusFile(t, "cbz_ltr.cbz"), chapter("1", "Cap. 1")))
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, one)
	workOf(t, s.uploadIn(admin, "c1b.cbz", corpusFile(t, "cbz_rtl.cbz"), chapter("1", "Cap. 1 de novo")))
}

func TestUpload_ADirectionAPersonChoseIsNotChanged(t *testing.T) {
	s := newCatalogStack(t)
	one := workOf(t, s.uploadIn(admin, "c1.cbz", corpusFile(t, "cbz_ltr.cbz"), chapter("1", "Cap. 1")))
	s.exec(`UPDATE collections SET reading_direction = 'webtoon' WHERE id = (SELECT collection_id FROM collection_works WHERE work_id = $1 AND official)`, one)
	two := workOf(t, s.uploadIn(admin, "c2.cbz", corpusFile(t, "cbz_rtl.cbz"), chapter("2", "Cap. 2")))
	if got := s.scalar(`SELECT reading_direction FROM collections WHERE id = (SELECT collection_id FROM collection_works WHERE work_id = $1 AND official)`, two); got != "webtoon" {
		t.Errorf("direction = %q, want the one the person chose", got)
	}
}

func TestUpload_FieldsThatAreNotValidAreRefusedBeforeTheFileIsRead(t *testing.T) {
	s := newCatalogStack(t)
	for name, fields := range map[string][][2]string{
		"unit":                  {{"series", "X"}, {"unit", "bogus"}},
		"kind":                  {{"series", "X"}, {"comic_kind", "novel"}},
		"number without series": {{"series_index", "3"}},
		"number notation":       {{"series", "X"}, {"series_index", "1e3"}},
		"number negative":       {{"series", "X"}, {"series_index", "-1"}},
		"number text":           {{"series", "X"}, {"series_index", "dois"}},
	} {
		if rec := s.uploadIn(admin, "c.cbz", corpusFile(t, "cbz_ltr.cbz"), fields); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, rec.Code, rec.Body.String())
		}
	}
	if got := s.counts(); got != "0/0/0" {
		t.Errorf("a refused upload left %s behind", got)
	}
}

func TestUpload_ADecimalNumberAndACommaAreAccepted(t *testing.T) {
	s := newCatalogStack(t)
	f := chapter("10,5", "Cap. 10.5")
	id := workOf(t, s.uploadIn(admin, "c.cbz", corpusFile(t, "cbz_ltr.cbz"), f))
	if got := s.scalar(`SELECT series_index FROM works WHERE id = $1`, id); got != "10.5" {
		t.Errorf("series_index = %q", got)
	}
}

func TestUpload_WithoutPlaceFieldsNothingIsLockedOrPlaced(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.upload(admin, "Watchmen 01.cbz", corpusFile(t, "cbz_ltr.cbz"))
	id := workOf(t, rec)
	if got := s.scalar(`SELECT title_lock || '|' || series_lock || '|' || COALESCE(series, '') FROM works WHERE id = $1`, id); got != "false|false|" {
		t.Errorf("work = %q, want nothing locked or placed", got)
	}
}
