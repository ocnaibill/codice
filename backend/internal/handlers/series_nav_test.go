package handlers

import (
	"testing"
	"time"
)

func entry(id int, unit string, state string, at int) seriesEntry {
	e := seriesEntry{SeriesStep: SeriesStep{ID: id, Title: "w", Unit: unit}}
	switch state {
	case "done":
		e.Done = true
	case "started":
		e.Started = true
	}
	if at > 0 {
		t := time.Date(2026, 10, 1, 0, 0, at, 0, time.UTC)
		e.LastAt = &t
	}
	return e
}

func stepID(s *SeriesStep) int {
	if s == nil {
		return -1
	}
	return s.ID
}

func TestSeriesSteps_NextIsTheNextOfTheSameUnit(t *testing.T) {
	list := []seriesEntry{
		entry(1, "volume", "", 0), entry(2, "volume", "", 0), entry(3, "chapter", "", 0), entry(4, "volume", "", 0),
		entry(5, "chapter", "", 0), entry(6, "", "", 0), entry(7, "", "", 0), entry(8, "oneshot", "", 0),
	}
	for id, want := range map[int]int{
		1: 2,                       // volume to volume
		2: 4,                       // a chapter between them is not a volume
		3: 5,                       // chapter to chapter
		6: 7,                       // the works with no unit follow one another
		4: -1, 5: -1, 7: -1, 8: -1, // the last of a group has none
		99: -1, // a work that is not in the list
	} {
		if got := stepID(nextInSeries(list, id)); got != want {
			t.Errorf("next of %d = %d, want %d", id, got, want)
		}
	}
	if nextInSeries(nil, 1) != nil {
		t.Error("an empty series has no next")
	}
}

func TestSeriesSteps_NextCarriesWhatTheScreenNeeds(t *testing.T) {
	pos := 3.0
	a := entry(1, "chapter", "", 0)
	b := entry(2, "chapter", "started", 5)
	b.Position = &pos
	b.Title = "Cap. três"
	got := nextInSeries([]seriesEntry{a, b}, 1)
	if got == nil || got.ID != 2 || got.Title != "Cap. três" || got.Unit != "chapter" || got.Position == nil || *got.Position != 3 || !got.Started {
		t.Errorf("step = %+v", got)
	}
}

func TestSeriesSteps_GoOn(t *testing.T) {
	cases := []struct {
		name  string
		list  []seriesEntry
		want  int
		begun bool
	}{
		{"nothing read starts the first group", []seriesEntry{entry(1, "chapter", "", 0), entry(2, "volume", "", 0), entry(3, "volume", "", 0)}, 2, false},
		{"the work with no unit comes last", []seriesEntry{entry(1, "", "", 0), entry(2, "oneshot", "", 0)}, 2, false},
		{"goes on after the last finished", []seriesEntry{entry(1, "chapter", "done", 1), entry(2, "chapter", "done", 2), entry(3, "chapter", "", 0), entry(4, "chapter", "", 0)}, 3, true},
		{"the one begun comes before an earlier one never read", []seriesEntry{entry(1, "chapter", "", 0), entry(2, "chapter", "done", 1), entry(3, "chapter", "started", 2), entry(4, "chapter", "", 0)}, 3, true},
		{"the first not finished, even if a later one was read", []seriesEntry{entry(1, "chapter", "", 0), entry(2, "chapter", "done", 1), entry(3, "chapter", "done", 2)}, 1, true},
		{"only begun, nothing finished, is still a series begun", []seriesEntry{entry(1, "chapter", "", 0), entry(2, "chapter", "started", 3)}, 2, true},
		{"follows the group read last", []seriesEntry{entry(1, "volume", "done", 1), entry(2, "volume", "", 0), entry(3, "chapter", "done", 5), entry(4, "chapter", "", 0)}, 4, true},
		{"the group read last wins whichever comes first in the list", []seriesEntry{entry(1, "chapter", "done", 5), entry(2, "chapter", "", 0), entry(3, "volume", "done", 1), entry(4, "volume", "", 0)}, 2, true},
		{"a group with nothing left hands over to the next", []seriesEntry{entry(1, "chapter", "done", 5), entry(2, "volume", "", 0)}, 2, true},
		{"finished: nothing to go on with", []seriesEntry{entry(1, "chapter", "done", 5), entry(2, "volume", "done", 2)}, -1, false},
		{"empty", nil, -1, false},
	}
	for _, c := range cases {
		got := continueSeries(c.list)
		if stepID(got) != c.want {
			t.Errorf("%s: step %d, want %d", c.name, stepID(got), c.want)
			continue
		}
		if got != nil && got.Begun != c.begun {
			t.Errorf("%s: begun = %v, want %v", c.name, got.Begun, c.begun)
		}
	}
}
