package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) candidateState(id int) string {
	s.t.Helper()
	return s.scalar(`SELECT state FROM metadata_candidates WHERE id = $1`, id)
}

func TestSeriesSuggestion_AcceptingWritesOnTheSeriesAndSettlesTheOtherWorksOfIt(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter", "chapter")
	other, _ := s.seriesOf("Outra", "chapter")
	pending := s.addCandidate(ids[0], "series_status", "hiatus", "AniList")
	sameField := s.addCandidate(ids[1], "series_status", "ongoing", "MangaDex") // another volume of the same series, another answer
	title := s.addCandidate(ids[2], "series_original_title", "ベルセルク", "AniList")
	elsewhere := s.addCandidate(s.addWork("Solto", "X", "x.cbz", "cbz"), "series_status", "finished", "AniList")

	// The page of the suggestions says what the series says today, and the state as a word the server keeps.
	rec := s.do(admin, "GET", fmt.Sprintf("/works/%d/candidates", ids[0]), "")
	if !strings.Contains(rec.Body.String(), `"field":"series_status"`) || !strings.Contains(rec.Body.String(), `"current":""`) {
		t.Errorf("list: %s", rec.Body.String())
	}
	if code := s.decide(ids[0], pending, "accept"); code != http.StatusOK {
		t.Fatalf("accept: %d", code)
	}
	d, _ := s.collection(ana, col)
	if d.Collection.PublicationStatus != "hiatus" || d.Collection.OriginalTitle != "" {
		t.Errorf("written on the series: %+v", d.Collection)
	}
	if s.candidateState(pending) != "accepted" || s.candidateState(sameField) != "rejected" {
		t.Errorf("the other works of the series: %s %s", s.candidateState(pending), s.candidateState(sameField))
	}
	if s.candidateState(title) != "pending" || s.candidateState(elsewhere) != "pending" {
		t.Errorf("another field and another series stay: %s %s", s.candidateState(title), s.candidateState(elsewhere))
	}
	// Nothing of the work itself changed: no field of it is locked or sourced by this.
	if s.scalar(`SELECT count(*) FROM work_field_sources WHERE work_id = $1`, ids[0]) != "0" {
		t.Errorf("the work was touched")
	}
	if s.scalar(`SELECT details->>'collectionId' || '|' || (details->>'value') FROM audit_log WHERE action = 'metadata.accept' ORDER BY id DESC LIMIT 1`) != fmt.Sprintf("%d|hiatus", col) {
		t.Errorf("audit: %s", s.scalar(`SELECT details::text FROM audit_log WHERE action = 'metadata.accept' ORDER BY id DESC LIMIT 1`))
	}
	if code := s.decide(ids[2], title, "accept"); code != http.StatusOK {
		t.Fatal(code)
	}
	if d, _ := s.collection(ana, col); d.Collection.OriginalTitle != "ベルセルク" || d.Collection.PublicationStatus != "hiatus" {
		t.Errorf("both: %+v", d.Collection)
	}
	// The list now says what the series has today.
	again := s.addCandidate(ids[0], "series_status", "finished", "MangaDex")
	rec = s.do(admin, "GET", fmt.Sprintf("/works/%d/candidates", ids[0]), "")
	if !strings.Contains(rec.Body.String(), `"current":"hiatus"`) {
		t.Errorf("today: %s", rec.Body.String())
	}
	_ = other
	_ = again
}

func TestSeriesSuggestion_RejectingChangesNothingOfTheSeries(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter")
	c := s.addCandidate(ids[0], "series_status", "cancelled", "AniList")
	if code := s.decide(ids[0], c, "reject"); code != http.StatusOK {
		t.Fatal(code)
	}
	if d, _ := s.collection(ana, col); d.Collection.PublicationStatus != "" {
		t.Errorf("rejected and written: %+v", d.Collection)
	}
	if s.candidateState(c) != "rejected" {
		t.Errorf("state: %s", s.candidateState(c))
	}
}

func TestSeriesSuggestion_RefusesWhatCannotBeAcceptedAndLeavesItPending(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter")
	alone := s.addWork("Solto", "X", "x.cbz", "cbz")
	cases := map[string]struct {
		work         int
		field, value string
	}{
		"a state that is not one": {ids[0], "series_status", "paused"},
		"an empty title":          {ids[0], "series_original_title", "   "},
		"a title too long":        {ids[0], "series_original_title", strings.Repeat("あ", 256)},
		"a work in no series":     {alone, "series_status", "hiatus"},
	}
	for name, c := range cases {
		id := s.addCandidate(c.work, c.field, c.value, "AniList")
		if code := s.decide(c.work, id, "accept"); code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", name, code)
		}
		if s.candidateState(id) != "pending" {
			t.Errorf("%s: it was decided", name)
		}
	}
	if d, _ := s.collection(ana, col); d.Collection.PublicationStatus != "" || d.Collection.OriginalTitle != "" {
		t.Errorf("a refused suggestion wrote: %+v", d.Collection)
	}
	// A series that was put away is not written on.
	retiredCand := s.addCandidate(ids[0], "series_status", "finished", "AniList")
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	if code := s.decide(ids[0], retiredCand, "accept"); code != http.StatusUnprocessableEntity {
		t.Errorf("retired: %d, want 422", code)
	}
}
