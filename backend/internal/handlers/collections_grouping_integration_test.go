package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) group(col int64, body string) (int, string) {
	s.t.Helper()
	rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/grouping", col), body)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func (s *catalogStack) sitsAt(work int) string {
	s.t.Helper()
	return s.scalar(`SELECT COALESCE(volume_number::text, '-') || '|' || COALESCE(story_arc, '-') FROM works WHERE id = $1`, work)
}

func TestGrouping_SaysTheArcAndTheVolumeOfARangeOfChaptersInOneStep(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter", "chapter", "chapter", "chapter", "volume")
	// the numbers in the series are 1..6; the last one is a volume file, number 6
	if code, body := s.group(col, `{"unit":"chapter","from":2,"to":4,"storyArc":"  A Era   de Ouro "}`); code != 200 || !strings.Contains(body, `"works":3`) {
		t.Fatalf("arc: %d %s", code, body)
	}
	if code, _ := s.group(col, `{"unit":"chapter","from":1,"to":3,"volumeNumber":"12,5"}`); code != 200 {
		t.Fatalf("volume: %d", code)
	}
	want := []string{"12.5|-", "12.5|A Era de Ouro", "12.5|A Era de Ouro", "-|A Era de Ouro", "-|-", "-|-"}
	for i, id := range ids {
		if got := s.sitsAt(id); got != want[i] {
			t.Errorf("work %d: %s, want %s", i+1, got, want[i])
		}
	}
	// What was said is shown on the page of the collection and in the metadata of the work.
	d, _ := s.collection(ana, col)
	by := map[int]CollectionWork{}
	for _, w := range d.Works {
		by[w.ID] = w
	}
	if w := by[ids[1]]; w.VolumeNumber == nil || *w.VolumeNumber != 12.5 || w.StoryArc != "A Era de Ouro" {
		t.Errorf("the page: %+v", w)
	}
	if w := by[ids[4]]; w.VolumeNumber != nil || w.StoryArc != "" {
		t.Errorf("a work nobody grouped says nothing: %+v", w)
	}
	if detail, _ := s.detail(ana, ids[1]); detail.Metadata == nil || detail.Metadata.StoryArc != "A Era de Ouro" || detail.Metadata.VolumeNumber == nil || *detail.Metadata.VolumeNumber != 12.5 {
		t.Errorf("the metadata of the work: %+v", detail.Metadata)
	}
	// Confirmed by hand, and audited.
	if s.scalar(`SELECT string_agg(field || ':' || source, ',' ORDER BY field) FROM work_field_sources WHERE work_id = $1 AND field IN ('story_arc', 'volume_number')`, ids[1]) != "story_arc:manual,volume_number:manual" {
		t.Errorf("provenance: %s", s.scalar(`SELECT string_agg(field, ',') FROM work_field_sources WHERE work_id = $1`, ids[1]))
	}
	if s.auditCount("collection.group") != "2" {
		t.Errorf("audit: %s", s.auditCount("collection.group"))
	}
}

func TestGrouping_LeavesWhatItIsNotAskedAndDoesNotChangeTwice(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter", "volume")
	s.group(col, `{"unit":"chapter","storyArc":"Arco 1"}`) // no range: every chapter
	if s.sitsAt(ids[0]) != "-|Arco 1" || s.sitsAt(ids[1]) != "-|Arco 1" || s.sitsAt(ids[2]) != "-|-" {
		t.Errorf("every work of the unit, and only them: %s %s %s", s.sitsAt(ids[0]), s.sitsAt(ids[1]), s.sitsAt(ids[2]))
	}
	// The same words again change nothing.
	if _, body := s.group(col, `{"unit":"chapter","storyArc":"Arco 1"}`); !strings.Contains(body, `"story_arc":0`) {
		t.Errorf("again: %s", body)
	}
	// The volume is said apart, and the arc is left.
	s.group(col, `{"from":1,"to":1,"volumeNumber":"3"}`)
	if s.sitsAt(ids[0]) != "3|Arco 1" {
		t.Errorf("the arc was kept: %s", s.sitsAt(ids[0]))
	}
	// Empty takes it away.
	s.group(col, `{"unit":"chapter","storyArc":""}`)
	if s.sitsAt(ids[0]) != "3|-" || s.sitsAt(ids[1]) != "-|-" {
		t.Errorf("cleared: %s %s", s.sitsAt(ids[0]), s.sitsAt(ids[1]))
	}
	// A work in the trash is not touched.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[1])
	s.group(col, `{"unit":"chapter","storyArc":"Outro"}`)
	if s.sitsAt(ids[1]) != "-|-" || s.sitsAt(ids[0]) != "3|Outro" {
		t.Errorf("the trash: %s %s", s.sitsAt(ids[1]), s.sitsAt(ids[0]))
	}
}

func TestGrouping_RefusesWhatMakesNoSense(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter")
	for name, body := range map[string]string{
		"nothing to say": `{"unit":"chapter"}`,
		"bad unit":       `{"unit":"arc","storyArc":"x"}`,
		"backwards":      `{"from":5,"to":2,"storyArc":"x"}`,
		"arc too long":   `{"storyArc":"` + strings.Repeat("a", 256) + `"}`,
		"volume zero":    `{"volumeNumber":"0"}`,
		"volume text":    `{"volumeNumber":"doze"}`,
		"volume big":     `{"volumeNumber":"10000"}`,
		"not json":       `nope`,
	} {
		if code, _ := s.group(col, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if code, _ := s.group(99999, `{"storyArc":"x"}`); code != http.StatusNotFound {
		t.Errorf("unknown: %d", code)
	}
	var personal int64
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Minha', 'manual') RETURNING id`, idAna).Scan(&personal)
	if code, _ := s.group(personal, `{"storyArc":"x"}`); code != http.StatusNotFound {
		t.Errorf("a list: %d", code)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", col), "")
	if code, _ := s.group(col, `{"storyArc":"x"}`); code != http.StatusConflict {
		t.Errorf("retired: %d", code)
	}
	if s.sitsAt(ids[0]) != "-|-" {
		t.Errorf("a refused request changed a work: %s", s.sitsAt(ids[0]))
	}
}

func TestGrouping_CanBeSaidInTheEditOfOneWork(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Cap 1", "Miura", "c1.cbz", "cbz")
	if code := s.editWork(w, `,"volume_number":"3,5","story_arc":" Arco   Um "`); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	if s.sitsAt(w) != "3.5|Arco Um" {
		t.Errorf("after the edit: %s", s.sitsAt(w))
	}
	if code := s.editWork(w, `,"volume_number":"","story_arc":""`); code != 200 || s.sitsAt(w) != "-|-" {
		t.Errorf("cleared: %d %s", code, s.sitsAt(w))
	}
	for _, extra := range []string{`,"volume_number":"-1"`, `,"volume_number":"x"`, `,"story_arc":"` + strings.Repeat("a", 256) + `"`} {
		if code := s.editWork(w, extra); code != http.StatusBadRequest {
			t.Errorf("%.40s: %d, want 400", extra, code)
		}
	}
	if s.scalar(`SELECT count(*) FROM work_field_sources WHERE work_id = $1 AND field IN ('volume_number', 'story_arc') AND source = 'manual'`, w) != "2" {
		t.Errorf("provenance of the edit")
	}
}
