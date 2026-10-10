package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func (s *catalogStack) rate(a actor, work int, body string) int {
	s.t.Helper()
	return s.do(a, "PUT", fmt.Sprintf("/works/%d/rating", work), body).Code
}

func (s *catalogStack) rating(a actor, work int) int {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/works/%d", work), "")
	var out struct {
		Rating int `json:"rating"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Rating
}

func TestRatings_AreThePersonsOwnAndCanBeChangedAndTakenAway(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	if got := s.rating(ana, work); got != 0 {
		t.Errorf("no stars yet: %d", got)
	}
	if code := s.rate(ana, work, `{"stars":4}`); code != http.StatusNoContent {
		t.Fatalf("rate: %d", code)
	}
	if got := s.rating(ana, work); got != 4 {
		t.Errorf("four stars: %d", got)
	}
	// Changed, not added.
	s.rate(ana, work, `{"stars":2}`)
	if got := s.rating(ana, work); got != 2 {
		t.Errorf("changed: %d", got)
	}
	if n := s.scalar(`SELECT count(*) FROM work_ratings WHERE user_id = $1`, idAna); n != "1" {
		t.Errorf("one rating per person and work: %s", n)
	}
	// Only theirs.
	if got := s.rating(bob, work); got != 0 {
		t.Errorf("bob sees ana's stars: %d", got)
	}
	s.rate(bob, work, `{"stars":5}`)
	if s.rating(ana, work) != 2 || s.rating(bob, work) != 5 {
		t.Errorf("each has their own")
	}
	// Taken away: theirs only, and twice is not an error.
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/works/%d/rating", work), ""); rec.Code != http.StatusNoContent {
		t.Errorf("take away: %d", rec.Code)
	}
	s.do(ana, "DELETE", fmt.Sprintf("/works/%d/rating", work), "")
	if s.rating(ana, work) != 0 || s.rating(bob, work) != 5 {
		t.Errorf("only ana's went")
	}
}

func TestRatings_RefuseWhatIsNotFromOneToFiveAndWhatIsNotThere(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	for _, body := range []string{`{"stars":0}`, `{"stars":6}`, `{"stars":-1}`, `{"stars":"cinco"}`, `{}`, `nope`, `{"stars":null}`} {
		if code := s.rate(ana, work, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, code)
		}
	}
	if s.rating(ana, work) != 0 {
		t.Errorf("a refused rating stores nothing")
	}
	if code := s.rate(ana, 999999, `{"stars":3}`); code != http.StatusNotFound {
		t.Errorf("no such work: %d", code)
	}
	if rec := s.do(ana, "PUT", "/works/abc/rating", `{"stars":3}`); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code := s.rate(ana, work, `{"stars":3}`); code != http.StatusConflict {
		t.Errorf("a work in the trash: %d", code)
	}
}

func TestRatings_GoWithTheWorkAndTheAccount(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.rate(ana, work, `{"stars":3}`)
	s.exec(`DELETE FROM works WHERE id = $1`, work)
	if n := s.scalar(`SELECT count(*) FROM work_ratings`); n != "0" {
		t.Errorf("the ratings of a work that is gone: %s", n)
	}
}
