package handlers

import (
	"fmt"
	"testing"
)

func TestSeriesRemaining_IsThePaceOfTheCallerPerUnitAndSaysNothingWhenItCannot(t *testing.T) {
	w := func(unit string, completed, started bool, percent float64, seconds int) CollectionWork {
		return CollectionWork{ID: 1, Available: true, Unit: unit, Completed: completed, Started: started, Percent: percent, seconds: seconds}
	}
	cases := []struct {
		name  string
		works []CollectionWork
		want  int
	}{
		{"nothing read yet", []CollectionWork{w("chapter", false, false, 0, 0), w("chapter", false, false, 0, 0)}, 0},
		{"one finished is not a pace", []CollectionWork{w("chapter", true, false, 100, 900), w("chapter", false, false, 0, 0)}, 0},
		{"two finished, less than ten minutes", []CollectionWork{w("chapter", true, false, 100, 200), w("chapter", true, false, 100, 300), w("chapter", false, false, 0, 0)}, 0},
		{"two finished, three to go", []CollectionWork{
			w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600),
			w("chapter", false, true, 50, 300), w("chapter", false, false, 0, 0), w("chapter", false, false, 0, 0),
		}, 1500},
		{"the one in progress has a pace of its own when it is worth saying", []CollectionWork{
			w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600),
			w("chapter", false, true, 50, 1200), // 1200 s for half: 1200 left
		}, 1200},
		{"everything is read", []CollectionWork{w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600)}, 0},
		{"a unit with works left and no pace stops the whole", []CollectionWork{
			w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600), w("chapter", false, false, 0, 0),
			w("volume", false, false, 0, 0),
		}, 0},
		{"a unit with nothing left needs no pace", []CollectionWork{
			w("volume", true, false, 100, 50), w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600), w("chapter", false, false, 0, 0),
		}, 600},
		{"the complementary works are not the sequence", []CollectionWork{
			w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600), w("extra", false, false, 0, 0),
		}, 0},
		{"a finished work with no time measured is not in the pace", []CollectionWork{
			w("chapter", true, false, 100, 0), w("chapter", true, false, 100, 600), w("chapter", false, false, 0, 0),
		}, 0},
	}
	for _, c := range cases {
		if got := seriesRemaining(c.works); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	gone := w("chapter", false, false, 0, 0)
	gone.Available = false
	if got := seriesRemaining([]CollectionWork{w("chapter", true, false, 100, 600), w("chapter", true, false, 100, 600), gone}); got != 0 {
		t.Errorf("a work that left the library is not to be read: %d", got)
	}
}

func TestCollectionPage_SaysTheMarkedPlacesAndHowLongIsLeftAtThePaceOfTheCaller(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter", "chapter", "chapter", "chapter", "extra")
	s.read(idAna, ids[0], true)
	s.read(idAna, ids[1], true)
	s.read(idAna, ids[2], false)
	s.exec(`UPDATE reading_progress SET percent_complete = 50 WHERE user_id = $1 AND file_id = $2`, idAna, s.primaryFile(ids[2]))
	s.exec(`UPDATE reading_progress SET reading_seconds = 600 WHERE user_id = $1 AND file_id IN ($2, $3)`, idAna, s.primaryFile(ids[0]), s.primaryFile(ids[1]))
	s.exec(`UPDATE reading_progress SET reading_seconds = 300 WHERE user_id = $1 AND file_id = $2`, idAna, s.primaryFile(ids[2]))

	mark := func(a actor, work int) {
		t.Helper()
		body := fmt.Sprintf(`{"kind":"bookmark","fileId":%d,"locator":{"type":"image","index":3}}`, s.primaryFile(work))
		if rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), body); rec.Code != 201 && rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	mark(ana, ids[0])
	mark(ana, ids[0])
	mark(ana, ids[2])
	mark(bob, ids[0])
	// A highlight is not a marked place: it counts in the notes, not in the marks.
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", ids[0]), `{"kind":"highlight","quote":"um trecho"}`); rec.Code != 201 && rec.Code != 200 {
		t.Fatalf("highlight: %d %s", rec.Code, rec.Body.String())
	}

	d, code := s.collection(ana, col)
	if code != 200 {
		t.Fatalf("%d", code)
	}
	got := map[int]int{}
	for _, w := range d.Works {
		got[w.ID] = w.Bookmarks
	}
	if got[ids[0]] != 2 || got[ids[2]] != 1 || got[ids[1]] != 0 || d.Summary.Bookmarks != 3 || d.Summary.Notes != 1 {
		t.Errorf("bookmarks by work: %v, whole: %d", got, d.Summary.Bookmarks)
	}
	// Two chapters of 600 s, the third at half (300 s: no pace of its own yet, so half the average), and two not begun: 300 + 2 * 600.
	if d.Summary.RemainingSeconds != 1500 {
		t.Errorf("remaining = %d", d.Summary.RemainingSeconds)
	}
	// The hours are the sum of the time in the works, the complementary one included.
	if d.Summary.ReadingSeconds != 1500 {
		t.Errorf("reading = %d", d.Summary.ReadingSeconds)
	}
	// Bob read nothing: no pace, and his own marks.
	e, _ := s.collection(bob, col)
	if e.Summary.RemainingSeconds != 0 || e.Summary.Bookmarks != 1 {
		t.Errorf("bob: %+v", e.Summary)
	}
}
