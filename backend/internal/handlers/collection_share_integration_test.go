package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

type shareAnswer struct {
	Preview       bool           `json:"preview"`
	Works         int            `json:"works"`
	WorksChanged  int            `json:"worksChanged"`
	Changed       map[string]int `json:"changed"`
	Kept          map[string]int `json:"kept"`
	SourceHasNone []string       `json:"sourceHasNone"`
}

func (s *catalogStack) share(col int64, body string) (shareAnswer, int) {
	s.t.Helper()
	rec := s.do(admin, "POST", fmt.Sprintf("/collections/%d/share", col), body)
	var out shareAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func (s *catalogStack) publisherOf(work int) string {
	return s.scalar(`SELECT COALESCE(publisher, '') FROM editions WHERE work_id = $1 AND is_primary`, work)
}

// aSeries makes a series of three works with each its own author: a volume (the source), a chapter and another volume.
func (s *catalogStack) aSeries() (col int64, vol, chap, vol2 int) {
	s.t.Helper()
	mk := func(title, author, unit string, n int) int {
		id := s.addWork(title, author, title+".cbz", "cbz")
		s.exec(`UPDATE works SET series = 'Berserk', series_index = $2, unit = $3 WHERE id = $1`, id, n, unit)
		return id
	}
	vol = mk("Berserk Vol 1", "Kentaro Miura", "volume", 1)
	chap = mk("Berserk Cap 5", "Autor B", "chapter", 5)
	vol2 = mk("Berserk Vol 2", "Autor C", "volume", 2)
	return s.collectionID("Berserk"), vol, chap, vol2
}

func TestShare_FillsOnlyWhatIsBlankAndTheOthersKeepTheirOwn(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, chap, vol2 := s.aSeries()
	s.exec(`UPDATE editions SET publisher = 'Panini' WHERE work_id = $1 AND is_primary`, vol)
	s.exec(`UPDATE editions SET publisher = 'JBC' WHERE work_id = $1 AND is_primary`, vol2)
	body := fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill","preview":true}`, vol)

	got, code := s.share(col, body)
	if code != 200 || !got.Preview || got.Works != 2 || got.WorksChanged != 1 || got.Changed["publisher"] != 1 || got.Kept["publisher"] != 1 {
		t.Fatalf("preview: %d %+v", code, got)
	}
	if s.publisherOf(chap) != "" {
		t.Fatalf("a preview wrote something")
	}
	if _, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill"}`, vol)); code != 200 {
		t.Fatalf("apply: %d", code)
	}
	if s.publisherOf(chap) != "Panini" || s.publisherOf(vol2) != "JBC" {
		t.Errorf("chapter %q, other volume %q: want Panini and JBC kept", s.publisherOf(chap), s.publisherOf(vol2))
	}
	// Written as a person writes it: locked, with the manual provenance, and in the audit.
	if got := s.scalar(`SELECT publisher_lock::text FROM works WHERE id = $1`, chap); got != "true" {
		t.Errorf("publisher_lock = %s", got)
	}
	if got := s.scalar(`SELECT source FROM work_field_sources WHERE work_id = $1 AND field = 'publisher'`, chap); got != "manual" {
		t.Errorf("source = %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'collection.share'`); got != "1" {
		t.Errorf("audit entries = %s, want 1 (the preview is not one)", got)
	}
}

func TestShare_ReplaceGivesItToEveryWorkOfTheUnitsAsked(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, chap, vol2 := s.aSeries()
	s.exec(`UPDATE editions SET publisher = 'Panini' WHERE work_id = $1 AND is_primary`, vol)
	s.exec(`UPDATE editions SET publisher = 'JBC' WHERE work_id IN ($1, $2) AND is_primary`, chap, vol2)
	got, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"units":["volume"],"mode":"replace"}`, vol))
	if code != 200 || got.Works != 1 || got.Changed["publisher"] != 1 {
		t.Fatalf("%d %+v", code, got)
	}
	if s.publisherOf(vol2) != "Panini" || s.publisherOf(chap) != "JBC" {
		t.Errorf("other volume %q, chapter %q: want Panini and the chapter untouched", s.publisherOf(vol2), s.publisherOf(chap))
	}
}

func TestShare_CreditsCategoriesAndTagsAreCopiedAsLists(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, chap, vol2 := s.aSeries()
	names := func(work int) string {
		return s.scalar(`SELECT COALESCE(string_agg(p.name, '|' ORDER BY c.position), '') FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = $1 AND c.role = 'author'`, work)
	}
	if got, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["authors"],"mode":"fill"}`, vol)); code != 200 || got.WorksChanged != 0 || got.Kept["authors"] != 2 {
		t.Fatalf("fill keeps the authors they have: %d %+v", code, got)
	}
	if _, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["authors"],"mode":"replace"}`, vol)); code != 200 {
		t.Fatal(code)
	}
	if names(chap) != "Kentaro Miura" || names(vol2) != "Kentaro Miura" {
		t.Errorf("authors: %q %q", names(chap), names(vol2))
	}
	s.exec(`INSERT INTO tags (name) VALUES ('Dark') ON CONFLICT DO NOTHING`)
	s.exec(`INSERT INTO work_tags (work_id, tag_id) SELECT $1, id FROM tags WHERE name = 'Dark'`, vol)
	if got, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["tags"],"mode":"fill"}`, vol)); code != 200 || got.Changed["tags"] != 2 {
		t.Fatalf("tags: %d %+v", code, got)
	}
	if n := s.scalar(`SELECT count(*) FROM work_tags WHERE work_id IN ($1, $2)`, chap, vol2); n != "2" {
		t.Errorf("tags copied to %s works, want 2", n)
	}
}

func TestShare_AFieldTheSourceHasNoneOfIsNotSharedAndSaysSo(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, chap, _ := s.aSeries()
	s.exec(`UPDATE editions SET publisher = 'JBC' WHERE work_id = $1 AND is_primary`, chap)
	got, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"replace"}`, vol))
	if code != 200 || len(got.SourceHasNone) != 1 || got.SourceHasNone[0] != "publisher" || got.WorksChanged != 0 {
		t.Fatalf("%d %+v", code, got)
	}
	if s.publisherOf(chap) != "JBC" {
		t.Errorf("the source had none and still took the value away: %q", s.publisherOf(chap))
	}
}

func TestShare_RefusesWhatIsNotAShareableThingOrASeriesOfTheWork(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, _, _ := s.aSeries()
	other := s.addWork("Solto", "X", "solto.epub", "epub")
	for name, body := range map[string]string{
		"title is the file's":   fmt.Sprintf(`{"sourceWorkId":%d,"fields":["title"],"mode":"fill"}`, vol),
		"no fields":             fmt.Sprintf(`{"sourceWorkId":%d,"fields":[],"mode":"fill"}`, vol),
		"no mode":               fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"]}`, vol),
		"a work of another one": fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill"}`, other),
		"a bad unit":            fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill","units":["x"]}`, vol),
		"a backwards range":     fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill","from":9,"to":2}`, vol),
	} {
		if _, code := s.share(col, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if _, code := s.share(99999, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill"}`, vol)); code != http.StatusNotFound {
		t.Errorf("a series that does not exist: %d", code)
	}
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	if _, code := s.share(col, fmt.Sprintf(`{"sourceWorkId":%d,"fields":["publisher"],"mode":"fill"}`, vol)); code != http.StatusConflict {
		t.Errorf("a retired series: %d", code)
	}
}

func TestPublisher_TheCardOfASeriesNamesThePublishersOfItsWorksMostCommonFirst(t *testing.T) {
	s := newCatalogStack(t)
	col, vol, chap, vol2 := s.aSeries()
	if got := s.collectionJSON(col).Publisher; got != "" {
		t.Fatalf("no work says one: %q", got)
	}
	s.exec(`UPDATE editions SET publisher = 'Panini' WHERE work_id IN ($1, $2) AND is_primary`, vol, vol2)
	s.exec(`UPDATE editions SET publisher = 'JBC' WHERE work_id = $1 AND is_primary`, chap)
	if got := s.collectionJSON(col).Publisher; got != "Panini, JBC" {
		t.Errorf("page = %q, want the most common first", got)
	}
	var list struct{ Data []Collection }
	json.Unmarshal(s.do(ana, "GET", "/collections", "").Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].Publisher != "Panini, JBC" {
		t.Errorf("list = %+v", list.Data)
	}
	var card string
	for _, w := range s.collapsed(ana, "").Data {
		if w.Collapsed != nil {
			card = w.Collapsed.Publisher
		}
	}
	if card != "Panini, JBC" {
		t.Errorf("card of the series = %q", card)
	}
	// A work in the trash does not count.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, chap)
	if got := s.collectionJSON(col).Publisher; got != "Panini" {
		t.Errorf("after the chapter went to the trash: %q", got)
	}
}
