package handlers

import (
	"fmt"
	"testing"
)

func (s *catalogStack) seriesDirection(a actor, work int) string {
	s.t.Helper()
	w, code := s.detail(a, work)
	if code != 200 || w.Metadata == nil {
		s.t.Fatalf("detail of %d: %d", work, code)
	}
	return w.Metadata.SeriesDirection
}

func TestSeriesDirection_IsChosenForTheCollectionAndComesWithEveryWorkOfIt(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter", "chapter")
	alone := s.addWork("Solto", "Autor", "solto.cbz", "cbz")
	path := fmt.Sprintf("/collections/%d", col)

	if got := s.seriesDirection(ana, ids[0]); got != "" {
		t.Errorf("nobody said: %q", got)
	}
	if rec := s.do(admin, "PATCH", path, `{"readingDirection":"rtl"}`); rec.Code != 200 {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	if d, _ := s.collection(ana, col); d.Collection.ReadingDirection != "rtl" || d.Collection.Name != "Berserk" {
		t.Errorf("the page says: %q %q", d.Collection.ReadingDirection, d.Collection.Name)
	}
	if s.seriesDirection(ana, ids[0]) != "rtl" || s.seriesDirection(bob, ids[1]) != "rtl" {
		t.Errorf("every work of the series carries it, for everybody")
	}
	if got := s.seriesDirection(ana, alone); got != "" {
		t.Errorf("a work in no series has none: %q", got)
	}
	// Alone it is an edit like the description: the series of the works is not touched, and it is audited.
	if s.auditCount("collection.describe") != "1" || s.auditCount("collection.rename") != "0" {
		t.Errorf("audit: %s %s", s.auditCount("collection.describe"), s.auditCount("collection.rename"))
	}
	if got := s.scalar(`SELECT details->>'readingDirection' FROM audit_log WHERE action = 'collection.describe' ORDER BY id DESC LIMIT 1`); got != "rtl" {
		t.Errorf("the audit says what was set: %q", got)
	}
	// Together with the name and the description.
	if rec := s.do(admin, "PATCH", path, `{"name":"Berserk (Miura)","description":"Dark fantasy.","readingDirection":"ltr"}`); rec.Code != 200 {
		t.Fatalf("all: %d %s", rec.Code, rec.Body.String())
	}
	d, _ := s.collection(ana, col)
	if d.Collection.ReadingDirection != "ltr" || d.Collection.Description != "Dark fantasy." || d.Collection.Name != "Berserk (Miura)" {
		t.Errorf("all three: %+v", d.Collection)
	}
	// The same words leave it as it is, and an empty one takes it away.
	if rec := s.do(admin, "PATCH", path, `{"description":"Outra."}`); rec.Code != 200 {
		t.Fatalf("description only: %d", rec.Code)
	}
	if d, _ := s.collection(ana, col); d.Collection.ReadingDirection != "ltr" {
		t.Errorf("a description does not take the direction: %q", d.Collection.ReadingDirection)
	}
	if rec := s.do(admin, "PATCH", path, `{"readingDirection":""}`); rec.Code != 200 {
		t.Fatalf("clear: %d", rec.Code)
	}
	if s.scalar(`SELECT reading_direction IS NULL FROM collections WHERE id = $1`, col) != "true" || s.seriesDirection(ana, ids[0]) != "" {
		t.Errorf("empty is NULL and the works say nothing")
	}
}

func TestSeriesDirection_RefusesWhatIsNotADirectionAndIsNotForListsNorForRetiredOnes(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "chapter")
	path := fmt.Sprintf("/collections/%d", col)
	for _, bad := range []string{`{"readingDirection":"double"}`, `{"readingDirection":"diagonal"}`, `{"readingDirection":"RTL"}`} {
		if rec := s.do(admin, "PATCH", path, bad); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}
	list := s.makeList(ana, "Minha")
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", list), `{"readingDirection":"rtl"}`); rec.Code != 400 {
		t.Errorf("a list has no direction: %d, want 400", rec.Code)
	}
	if s.do(admin, "PATCH", path, `{"readingDirection":"rtl"}`).Code != 200 {
		t.Fatal("set")
	}
	// A series put away does not say how its works are read.
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	if got := s.seriesDirection(ana, ids[0]); got != "" {
		t.Errorf("retired: %q", got)
	}
	if rec := s.do(admin, "PATCH", path, `{"readingDirection":"ltr"}`); rec.Code != 409 {
		t.Errorf("a retired one: %d, want 409", rec.Code)
	}
}
