package handlers

import (
	"fmt"
	"testing"
	"time"
)

func (s *catalogStack) originalYear(id int) string {
	s.t.Helper()
	m := s.meta(id)
	if m.OriginalYear == nil {
		return "none"
	}
	return fmt.Sprint(*m.OriginalYear)
}

func TestOriginalYear_IsWrittenByHandConfirmedAndLockedLikeTheOtherFields(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.exec(`UPDATE editions SET publication_date = '2021', publisher = 'Aleph' WHERE work_id = $1 AND is_primary`, id)
	if got := s.originalYear(id); got != "none" {
		t.Fatalf("nobody knows it yet: %s", got)
	}
	if code := s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":"1965"}`); code != 200 {
		t.Fatalf("update: %d", code)
	}
	m := s.meta(id)
	if s.originalYear(id) != "1965" || !m.Locks["original_year"] || m.Sources["original_year"] != "manual" {
		t.Errorf("written, locked and manual: %v %v %v", s.originalYear(id), m.Locks, m.Sources)
	}
	// The year of the edition is another thing, and is left as it was.
	if m.PublicationDate != "2021" || m.Locks["publication_date"] {
		t.Errorf("the edition's date moved: %+v", m)
	}
	// A change of something else leaves the year, and an absent field is not a clear.
	if code := s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"publisher":"Outra"}`); code != 200 {
		t.Fatalf("update: %d", code)
	}
	if got := s.originalYear(id); got != "1965" {
		t.Errorf("an absent field leaves it: %s", got)
	}
	// Cleared with an empty text; before the common era with a minus.
	s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":""}`)
	if got := s.originalYear(id); got != "none" {
		t.Errorf("cleared: %s", got)
	}
	s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":"-384"}`)
	if got := s.originalYear(id); got != "-384" {
		t.Errorf("before the common era: %s", got)
	}
	// The lock can be set and let go of by itself.
	s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"original_year_lock":false}`)
	if s.meta(id).Locks["original_year"] {
		t.Errorf("the lock was let go of")
	}
}

func TestOriginalYear_RefusesWhatIsNotAYear(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	for _, bad := range []string{"abc", "0", "19 65", "1.5", "+1965", "01965", "-3001", "99999", "3000"} {
		if code := s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":"`+bad+`"}`); code != 400 {
			t.Errorf("%q: %d", bad, code)
		}
	}
	if got := s.originalYear(id); got != "none" {
		t.Errorf("a refused year stores nothing: %s", got)
	}
	// The edges that are years.
	for _, ok := range []string{"-3000", "1", "2026"} {
		if code := s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":"`+ok+`"}`); code != 200 {
			t.Errorf("%q: %d", ok, code)
		}
	}
}

func TestOriginalYear_ACandidateOfAProviderIsAcceptedAsConfirmedAndTheOthersForItAreDismissed(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.exec(`UPDATE works SET original_year = 1970 WHERE id = $1`, id)
	first := s.addCandidate(id, "original_year", "1965", "openlibrary")
	other := s.addCandidate(id, "original_year", "1966", "wikidata")
	list := s.candidates(id)
	if len(list) != 2 {
		t.Fatalf("two suggestions: %+v", list)
	}
	for _, c := range list {
		if c.Field != "original_year" || c.Current != "1970" {
			t.Errorf("the current year is listed beside it: %+v", c)
		}
	}
	if code := s.decide(id, first, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	m := s.meta(id)
	if s.originalYear(id) != "1965" || !m.Locks["original_year"] || m.Sources["original_year"] != "openlibrary" {
		t.Errorf("accepted: %v %v %v", s.originalYear(id), m.Locks, m.Sources)
	}
	if left := s.candidates(id); len(left) != 0 {
		t.Errorf("the field is settled, the other suggestion goes: %+v", left)
	}
	_ = other
}

func TestOriginalYear_ACandidateThatIsNotAYearIsNotApplied(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	for _, value := range []string{"sem ano", "0", "1965-05-01"} {
		cand := s.addCandidate(id, "original_year", value, "openlibrary")
		if code := s.decide(id, cand, "accept"); code != 422 {
			t.Errorf("%q: %d", value, code)
		}
	}
	if got := s.originalYear(id); got != "none" {
		t.Errorf("nothing applied: %s", got)
	}
}

func TestOriginalYear_IsAtMostNextYear(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	next := time.Now().Year() + 1
	if code := s.put(id, fmt.Sprintf(`{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":"%d"}`, next)); code != 200 {
		t.Errorf("next year, for a book that is announced: %d", code)
	}
	if code := s.put(id, fmt.Sprintf(`{"title":"Duna","author":"Frank Herbert","tags":[],"original_year":"%d"}`, next+1)); code != 400 {
		t.Errorf("the year after that: %d", code)
	}
}
