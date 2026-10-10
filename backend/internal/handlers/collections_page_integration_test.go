package handlers

import (
	"fmt"
	"strings"
	"testing"
)

func TestCollectionPage_SaysWhatTheCallerDidOfEachWorkAndOfTheWhole(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Harry Potter", "volume", "volume", "volume", "volume")
	v1, v2, v3 := ids[0], ids[1], ids[2]
	s.exec(`UPDATE works SET original_year = 1997, description = $2 WHERE id = $1`, v1, strings.Repeat("Sinopse longa. ", 40))
	s.exec(`UPDATE works SET original_year = 1998 WHERE id = $1`, v2)
	s.exec(`UPDATE works SET original_year = 2007 WHERE id = $1`, ids[3])

	// Ana finished the first (a file read to the end), is in the middle of the second, with a chapter, and has not begun the third.
	s.progress(ana, "PUT", s.primaryFile(v1), `{"locator":{"type":"image","index":30},"percent":100,"completed":true,"chapter":"O fim","unitIndex":31,"unitTotal":31}`)
	s.progress(ana, "PUT", s.primaryFile(v2), `{"locator":{"type":"image","index":20},"percent":64,"chapter":"Capítulo 14","unitIndex":204,"unitTotal":318}`)
	s.exec(`UPDATE reading_progress SET reading_seconds = 7200 WHERE user_id = $1 AND file_id = $2`, idAna, s.primaryFile(v1))
	s.exec(`UPDATE reading_progress SET reading_seconds = 1800 WHERE user_id = $1 AND file_id = $2`, idAna, s.primaryFile(v2))
	s.do(ana, "PUT", fmt.Sprintf("/works/%d/rating", v1), `{"stars":5}`)
	// The fourth was marked finished as a whole, with no file read to the end.
	s.exec(`INSERT INTO work_reading_state (user_id, work_id, finished_at) VALUES ($1, $2, now())`, idAna, ids[3])
	// Bob read the third.
	s.progress(bob, "PUT", s.primaryFile(v3), `{"locator":{"type":"image","index":9},"percent":90}`)
	s.exec(`UPDATE reading_progress SET reading_seconds = 99 WHERE user_id = $1`, idBob)

	d, code := s.collection(ana, col)
	if code != 200 {
		t.Fatalf("%d", code)
	}
	by := map[int]CollectionWork{}
	for _, w := range d.Works {
		by[w.ID] = w
	}
	a, b, c := by[v1], by[v2], by[v3]
	if a.Percent != 100 || a.Started || !a.Completed || a.CompletedAt == nil || a.Rating != 5 || a.OriginalYear == nil || *a.OriginalYear != 1997 {
		t.Errorf("the finished one: %+v", a)
	}
	if len([]rune(a.Synopsis)) != 300 {
		t.Errorf("a few lines of the synopsis, not all: %d", len([]rune(a.Synopsis)))
	}
	if a.Chapter != "" || a.UnitTotal != 0 {
		t.Errorf("a finished work says no chapter: it is not where the person is: %+v", a)
	}
	if d4 := by[ids[3]]; !d4.Completed || d4.Percent != 100 || d4.Started {
		t.Errorf("a work marked as finished is finished: %+v", d4)
	}
	if b.Percent != 64 || !b.Started || b.Completed || b.Chapter != "Capítulo 14" || b.UnitIndex != 204 || b.UnitTotal != 318 || b.ReadFormat != "cbz" {
		t.Errorf("the one in progress: %+v", b)
	}
	if c.Percent != 0 || c.Started || c.Completed || c.Rating != 0 || c.Chapter != "" {
		t.Errorf("the one not begun is the caller's, not bob's: %+v", c)
	}
	if len(a.Formats) != 1 || a.Formats[0] != "cbz" {
		t.Errorf("formats: %v", a.Formats)
	}
	sum := d.Summary
	if sum.Works != 4 || sum.Finished != 2 || sum.InProgress != 1 || sum.ReadingSeconds != 9000 {
		t.Errorf("summary: %+v", sum)
	}
	// (100 + 64 + 0 + 100) / 4
	if sum.Percent != 66 {
		t.Errorf("percent = %v", sum.Percent)
	}
	if sum.YearFrom == nil || *sum.YearFrom != 1997 || sum.YearTo == nil || *sum.YearTo != 2007 {
		t.Errorf("years: %v %v", sum.YearFrom, sum.YearTo)
	}
	// Bob's numbers are his.
	if e, _ := s.collection(bob, col); e.Summary.Finished != 0 || e.Summary.InProgress != 1 || e.Summary.ReadingSeconds != 99 {
		t.Errorf("bob: %+v", e.Summary)
	}
}

func TestCollectionPage_SaysWhoWroteItTheTagsAndTheNumbersThatAreSkipped(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Série", "volume", "volume", "volume", "volume", "chapter", "chapter", "oneshot", "oneshot")
	// volumes 1, 2, 4 and 6 (a gap at 3 and 5), chapters 1 and 3 (a gap at 2), and one-shots, which are not a sequence
	for i, n := range []float64{1, 2, 4, 6, 1, 3, 1, 5} {
		s.exec(`UPDATE works SET series_index = $2 WHERE id = $1`, ids[i], n)
	}
	s.credit(admin, ids[0], "Autora Principal", "author")
	s.credit(admin, ids[1], "Autora Principal", "author")
	s.credit(admin, ids[0], "Aaron Aaronson", "author") // first by name, but of one work
	s.credit(admin, ids[0], "Lia Wyler", "translator")
	for _, tag := range []string{"magia", "escola"} {
		s.exec(`INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, tag)
	}
	for _, w := range ids {
		s.exec(`INSERT INTO work_tags (work_id, tag_id) SELECT $1, id FROM tags WHERE name = 'magia'`, w)
	}
	s.exec(`INSERT INTO work_tags (work_id, tag_id) SELECT $1, id FROM tags WHERE name = 'escola'`, ids[0])

	d, _ := s.collection(ana, col)
	sum := d.Summary
	var missing []string
	for _, m := range sum.Missing {
		missing = append(missing, fmt.Sprintf("%s:%v", m.Unit, m.Number))
	}
	if strings.Join(missing, ",") != "chapter:2,volume:3,volume:5" {
		t.Errorf("the numbers the series skips, by unit: %v", missing)
	}
	// The one every work has first, the one two have next; each with the person to open.
	if len(sum.Authors) != 3 || sum.Authors[0].Name != "Autor" || sum.Authors[0].Works != 8 || sum.Authors[1].Name != "Autora Principal" || sum.Authors[1].Works != 2 || sum.Authors[2].Name != "Aaron Aaronson" || sum.Authors[0].ID == 0 {
		t.Errorf("authors: %+v", sum.Authors)
	}
	if len(sum.Translators) != 1 || sum.Translators[0].Name != "Lia Wyler" {
		t.Errorf("translators: %+v", sum.Translators)
	}
	if len(sum.Tags) != 2 || sum.Tags[0].Name != "magia" || sum.Tags[0].Works != 8 || sum.Tags[1].Name != "escola" {
		t.Errorf("tags: %+v", sum.Tags)
	}
}

func TestSkippedNumbers_AreTheWholeOnesBetweenTheFirstAndTheLastThatNoWorkHas(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
		want []float64
	}{
		{"none", nil, nil},
		{"one number says nothing", []float64{3}, nil},
		{"a run has none", []float64{1, 2, 3}, nil},
		{"a gap", []float64{1, 2, 4}, []float64{3}},
		{"two gaps, out of order", []float64{5, 1, 3}, []float64{2, 4}},
		{"a number counted once", []float64{1, 1, 3}, []float64{2}},
		{"half numbers are not asked for", []float64{1, 2.5, 4}, []float64{2, 3}},
		{"a series that starts at 3 skips nothing before it", []float64{3, 4, 6}, []float64{5}},
		{"zero and negatives are not numbers of a series", []float64{0, -1, 2, 4}, []float64{3}},
	}
	for _, tc := range cases {
		got := skippedNumbers(tc.in)
		if fmt.Sprint(got) != fmt.Sprint(append([]float64{}, tc.want...)) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
