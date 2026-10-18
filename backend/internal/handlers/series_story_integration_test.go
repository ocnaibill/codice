package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

type storyDetail struct {
	Continue *SeriesStep `json:"continue"`
	Story    *Story      `json:"story"`
}

func (s *catalogStack) storyOf(col int64) storyDetail {
	s.t.Helper()
	var d storyDetail
	json.Unmarshal(s.do(ana, "GET", fmt.Sprintf("/collections/%d", col), "").Body.Bytes(), &d)
	return d
}

// aRangedSeries makes a volume that contains the chapters 1 to 3 and the loose chapters 2, 4, 5 and 6.
func (s *catalogStack) aRangedSeries() (col int64, vol int, chaps map[int]int) {
	s.t.Helper()
	mk := func(title, unit string, n int) int {
		id := s.addWork(title, "Autor", title+".cbz", "cbz")
		s.exec(`UPDATE works SET series = 'Absolute Batman', series_index = $2, unit = $3 WHERE id = $1`, id, n, unit)
		return id
	}
	vol = mk("Vol 1", "volume", 1)
	s.exec(`UPDATE works SET covers_from = 1, covers_to = 3 WHERE id = $1`, vol)
	chaps = map[int]int{}
	for _, n := range []int{2, 4, 5, 6} {
		chaps[n] = mk(fmt.Sprintf("Cap %d", n), "chapter", n)
	}
	return s.collectionID("Absolute Batman"), vol, chaps
}

func TestStory_ThePageOfASeriesSaysItsStoryAndWhereToGoOn(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, c := s.aRangedSeries()
	d := s.storyOf(col)
	if d.Story == nil || !d.Story.Ranged {
		t.Fatalf("story = %+v", d.Story)
	}
	if want := []int{vol, c[4], c[5], c[6]}; !reflect.DeepEqual(d.Story.Order, want) {
		t.Errorf("order = %v, want %v", d.Story.Order, want)
	}
	if want := []int{c[2]}; !reflect.DeepEqual(d.Story.Nested[vol], want) {
		t.Errorf("nested = %v, want the loose chapter 2 under the volume", d.Story.Nested[vol])
	}
	if d.Story.Chapters != 6 || d.Story.Read != 0 || len(d.Story.Gaps) != 0 {
		t.Errorf("chapters %d read %d gaps %v", d.Story.Chapters, d.Story.Read, d.Story.Gaps)
	}
	if d.Continue == nil || d.Continue.ID != vol {
		t.Fatalf("go on = %+v, want the volume (it holds the first chapter and the most)", d.Continue)
	}

	// Reading the volume reads its chapters and goes on after them; the chapter 2 itself is not marked.
	s.read(idAna, vol, true)
	d = s.storyOf(col)
	if d.Story.Read != 3 || d.Continue == nil || d.Continue.ID != c[4] {
		t.Errorf("read %d, go on = %+v, want 3 and the chapter 4", d.Story.Read, d.Continue)
	}
	if got := s.scalar(`SELECT count(*) FROM reading_progress WHERE file_id = $1`, s.primaryFile(c[2])); got != "0" {
		t.Errorf("reading the volume wrote progress on the chapter 2: %s", got)
	}
	series, code := s.seriesOfWork(ana, vol)
	if code != 200 || series.Next == nil || series.Next.ID != c[4] {
		t.Errorf("next after the volume = %+v (%d), want the chapter 4", series.Next, code)
	}
}

func TestStory_AWorkSaysWhatItContainsWithItsMetadataAndOnlyAsAWholeRange(t *testing.T) {
	s := newCatalogStack(t)
	_, vol, _ := s.aRangedSeries()
	put := func(body string) int {
		return s.do(admin, "PUT", fmt.Sprintf("/works/%d", vol), body).Code
	}
	for name, body := range map[string]string{
		"only the first":     `{"title":"Vol 1","covers_from":"1","covers_to":""}`,
		"the last before it": `{"title":"Vol 1","covers_from":"9","covers_to":"3"}`,
		"not a number":       `{"title":"Vol 1","covers_from":"a","covers_to":"3"}`,
		"zero":               `{"title":"Vol 1","covers_from":"0","covers_to":"3"}`,
	} {
		if code := put(body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if code := put(`{"title":"Vol 1","covers_from":"4","covers_to":"6,5"}`); code != 200 {
		t.Fatalf("a good range: %d", code)
	}
	if got := s.scalar(`SELECT covers_from || '-' || covers_to FROM works WHERE id = $1`, vol); got != "4-6.5" {
		t.Errorf("stored = %q", got)
	}
	if got := s.scalar(`SELECT source FROM work_field_sources WHERE work_id = $1 AND field = 'covers_from'`, vol); got != "manual" {
		t.Errorf("source = %q", got)
	}
	// Taking it away takes both.
	if code := put(`{"title":"Vol 1","covers_from":"","covers_to":""}`); code != 200 {
		t.Fatal(code)
	}
	if got := s.scalar(`SELECT count(*) FROM works WHERE id = $1 AND covers_from IS NULL AND covers_to IS NULL`, vol); got != "1" {
		t.Errorf("the range was not taken away")
	}
}
