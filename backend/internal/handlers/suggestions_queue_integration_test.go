package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

type queuedWork struct {
	WorkID  int
	Title   string
	Author  string
	Pending int
	Fields  []string
}

func (s *catalogStack) queue() (list []queuedWork, total int) {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/suggestions", "")
	if rec.Code != 200 {
		s.t.Fatalf("queue: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Data  []queuedWork
		Total int
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Data, out.Total
}

func TestSuggestionQueue_ListsTheWorksWithSomethingToDecideTheOldestFirst(t *testing.T) {
	s := newCatalogStack(t)
	newer := s.addWork("Newer", "Ann Lee", "a.epub", "epub")
	older := s.addWork("Older", "Bo Kim", "b.epub", "epub")
	decided := s.addWork("Decided", "Cy Roe", "c.epub", "epub")
	retired := s.addWork("Retired", "Di Fox", "d.epub", "epub")
	none := s.addWork("None", "Ed Poe", "e.epub", "epub")

	s.exec(`INSERT INTO metadata_candidates (work_id, field, value, source, created_at) VALUES
		($1, 'title', 'N1', 'x', now() - interval '1 hour'), ($1, 'isbn', '1', 'x', now()), ($1, 'isbn', '2', 'y', now()), ($1, 'tags', '["a"]', 'x', now())`, newer)
	s.exec(`INSERT INTO metadata_candidates (work_id, field, value, source, created_at) VALUES ($1, 'author', 'O1', 'x', now() - interval '2 days')`, older)
	s.exec(`INSERT INTO metadata_candidates (work_id, field, value, source, state) VALUES ($1, 'title', 'D', 'x', 'accepted'), ($1, 'isbn', '9', 'x', 'rejected')`, decided)
	s.exec(`INSERT INTO metadata_candidates (work_id, field, value, source) VALUES ($1, 'title', 'R', 'x')`, retired)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retired)
	_ = none

	list, total := s.queue()
	if total != 2 || len(list) != 2 {
		t.Fatalf("total = %d, listed = %+v: only works with a suggestion waiting, and not a retired one", total, list)
	}
	if list[0].WorkID != older || list[1].WorkID != newer {
		t.Errorf("order = %d, %d: the one waiting the longest comes first", list[0].WorkID, list[1].WorkID)
	}
	if got := fmt.Sprint(list[0].Title, "|", list[0].Author, "|", list[0].Pending, "|", list[0].Fields); got != "Older|Bo Kim|1|[author]" {
		t.Errorf("first = %s", got)
	}
	if got := fmt.Sprint(list[1].Title, "|", list[1].Author, "|", list[1].Pending, "|", list[1].Fields); got != "Newer|Ann Lee|4|[isbn tags title]" {
		t.Errorf("second = %s: every pending suggestion counted, each field named once, in order", got)
	}
}

func TestSuggestionQueue_AWorkLeavesItWhenItsLastSuggestionIsDecidedAndAnEmptyQueueIsAnEmptyList(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Dune", "F. Herbert", "a.epub", "epub")
	a := s.addCandidate(w, "isbn", "9788576573135", "Google Books")
	b := s.addCandidate(w, "publisher", "Aleph", "Google Books")
	s.decide(w, a, "reject")
	if list, total := s.queue(); total != 1 || len(list) != 1 || list[0].Pending != 1 || fmt.Sprint(list[0].Fields) != "[publisher]" {
		t.Fatalf("after one decision: %d %+v", total, list)
	}
	s.decide(w, b, "accept")
	list, total := s.queue()
	if total != 0 || list == nil || len(list) != 0 {
		t.Errorf("after the last: total = %d, list = %#v", total, list)
	}
}

func TestSuggestionQueue_ListsAtMostWhatItCapsButSaysHowManyWorksWait(t *testing.T) {
	old := maxQueuedWorks
	maxQueuedWorks = 2
	defer func() { maxQueuedWorks = old }()
	s := newCatalogStack(t)
	for i := 0; i < 4; i++ {
		w := s.addWork(fmt.Sprintf("W%d", i), "Ann Lee", fmt.Sprintf("%d.epub", i), "epub")
		s.addCandidate(w, "isbn", fmt.Sprint(i), "x")
	}
	list, total := s.queue()
	if len(list) != 2 || total != 4 {
		t.Errorf("listed %d of %d, want 2 of 4", len(list), total)
	}
}
