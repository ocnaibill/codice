package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

type collectionList struct {
	Data       []Collection
	Total      int
	Limit      int
	TotalPages int
}

type collectionDetail struct {
	Collection Collection
	Works      []CollectionWork
	Continue   *SeriesStep
	Summary    CollectionSummary
}

func (s *catalogStack) collections(a actor, query string) collectionList {
	s.t.Helper()
	var out collectionList
	rec := s.do(a, "GET", "/collections"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /collections%s: %d %s", query, rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (s *catalogStack) collection(a actor, id int64) (collectionDetail, int) {
	s.t.Helper()
	var out collectionDetail
	rec := s.do(a, "GET", fmt.Sprintf("/collections/%d", id), "")
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func (s *catalogStack) collectionID(name string) int64 {
	s.t.Helper()
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM collections WHERE name = $1`, name).Scan(&id); err != nil {
		s.t.Fatalf("collection %q: %v", name, err)
	}
	return id
}

func TestCollections_ListAndDetail(t *testing.T) {
	s := newCatalogStack(t)
	v1 := s.addWork("Pedra Filosofal", "J. K. Rowling", "hp1.epub", "epub")
	v2 := s.addWork("Câmara Secreta", "J. K. Rowling", "hp2.epub", "epub")
	extra := s.addWork("Contos", "J. K. Rowling", "hp0.epub", "epub") // no number: comes last
	gone := s.addWork("Retirado", "J. K. Rowling", "hp9.epub", "epub")
	solo := s.addWork("Solto", "Alguém", "solo.epub", "epub")
	_ = solo
	s.exec(`UPDATE works SET series = 'Harry Potter', series_index = 1 WHERE id = $1`, v1)
	s.exec(`UPDATE works SET series = 'harry potter', series_index = 2 WHERE id = $1`, v2)
	s.exec(`UPDATE works SET series = 'Harry Potter' WHERE id = $1`, extra)
	s.exec(`UPDATE works SET series = 'Harry Potter', series_index = 7 WHERE id = $1`, gone)
	s.exec(`UPDATE editions SET cover_url = '/covers/v2.jpg' WHERE work_id = $1`, v2)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idAna, s.primaryFile(v1))

	l := s.collections(ana, "")
	if l.Total != 1 || len(l.Data) != 1 {
		t.Fatalf("collections = %+v, want only Harry Potter (a work with no series makes none)", l)
	}
	c := l.Data[0]
	if c.Kind != "official" || c.Name != "Harry Potter" || c.WorkCount != 3 || c.CompletedCount != 1 || c.CoverURL != "/covers/v2.jpg" {
		t.Errorf("collection = %+v (a retired work does not count; the cover is the first one a work has)", c)
	}
	// What was read is each person's own.
	if b := s.collections(bob, "").Data[0]; b.WorkCount != 3 || b.CompletedCount != 0 {
		t.Errorf("bob's view = %+v", b)
	}

	d, code := s.collection(ana, c.ID)
	if code != 200 || d.Collection.Name != "Harry Potter" || d.Collection.WorkCount != 3 || d.Collection.CompletedCount != 1 {
		t.Fatalf("detail = %d %+v", code, d.Collection)
	}
	var order []int
	for _, w := range d.Works {
		order = append(order, w.ID)
	}
	if len(order) != 3 || order[0] != v1 || order[1] != v2 || order[2] != extra {
		t.Errorf("order = %v, want %v (by number, the one with none last, the retired one out)", order, []int{v1, v2, extra})
	}
	if w := d.Works[0]; w.Author != "J. K. Rowling" || !w.Completed || w.Position == nil || *w.Position != 1 {
		t.Errorf("first work = %+v", w)
	}
	if d.Collection.CoverURL != "/covers/v2.jpg" {
		t.Errorf("detail cover = %q, want the first one a work has", d.Collection.CoverURL)
	}
	if d.Works[2].Position != nil || d.Works[1].Completed {
		t.Errorf("work with no number / not read = %+v %+v", d.Works[2], d.Works[1])
	}
}

func TestCollections_WhatIsNotWorthACardIsNotListed(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	s.exec(`UPDATE works SET series = 'Antiga' WHERE id = $1`, a)
	if n := s.collections(ana, "").Total; n != 1 {
		t.Fatalf("total = %d, want 1", n)
	}
	// The only work leaves: the collection that was born from it has nothing to show.
	s.exec(`UPDATE works SET series = NULL WHERE id = $1`, a)
	if l := s.collections(ana, ""); l.Total != 0 || len(l.Data) != 0 {
		t.Errorf("an empty collection of the metadata was listed: %+v", l)
	}
	// A collection whose only work was retired has nothing to show either.
	s.exec(`UPDATE works SET series = 'Antiga' WHERE id = $1`, a)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, a)
	if l := s.collections(ana, ""); l.Total != 0 {
		t.Errorf("a collection of retired works was listed: %+v", l)
	}
	s.exec(`UPDATE works SET retired_at = NULL, series = NULL WHERE id = $1`, a)
	// One a person made theirs stays, empty as it is.
	s.exec(`UPDATE collections SET edited_at = now() WHERE name = 'Antiga'`)
	if l := s.collections(ana, ""); l.Total != 1 || l.Data[0].WorkCount != 0 || l.Data[0].CoverURL != "/covers/placeholder.svg" {
		t.Errorf("an edited empty collection: %+v", l)
	}
	s.exec(`UPDATE collections SET edited_at = NULL, origin = 'manual' WHERE name = 'Antiga'`)
	if s.collections(ana, "").Total != 1 {
		t.Error("a collection made by hand disappeared when empty")
	}
	// A retired one is not listed and not found for a reader.
	id := s.collectionID("Antiga")
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, id)
	if s.collections(ana, "").Total != 0 {
		t.Error("a retired collection was listed")
	}
	if _, code := s.collection(ana, id); code != 404 {
		t.Errorf("a retired collection: %d, want 404", code)
	}
}

func TestCollections_APersonalOneIsItsOwnersAlone(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	var id int64
	if err := s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Para ler', 'manual') RETURNING id`, idAna).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 1)`, id, a)

	if l := s.collections(ana, "?kind=personal"); l.Total != 1 || l.Data[0].Kind != "personal" || l.Data[0].WorkCount != 1 {
		t.Errorf("ana's list = %+v", l)
	}
	if l := s.collections(ana, ""); l.Total != 0 {
		t.Errorf("a personal collection in the list of the official ones: %+v", l)
	}
	if d, code := s.collection(ana, id); code != 200 || len(d.Works) != 1 {
		t.Errorf("ana's detail = %d %+v", code, d)
	}
	for _, who := range []actor{bob, admin} {
		for _, q := range []string{"", "?kind=personal", "?kind=personal&retired=true", "?retired=true"} {
			if l := s.collections(who, q); l.Total != 0 {
				t.Errorf("%s sees ana's collection in the list%s: %+v", who.role, q, l)
			}
		}
		if _, code := s.collection(who, id); code != 404 {
			t.Errorf("%s got ana's collection: %d, want 404", who.role, code)
		}
	}
}

func TestCollections_NotFoundAndPaging(t *testing.T) {
	s := newCatalogStack(t)
	for i := 0; i < 5; i++ {
		w := s.addWork(fmt.Sprintf("Obra %d", i), "X", fmt.Sprintf("o%d.epub", i), "epub")
		s.exec(`UPDATE works SET series = $2 WHERE id = $1`, w, fmt.Sprintf("Série %c", 'E'-i))
	}
	for _, target := range []string{"/collections/0", "/collections/-3", "/collections/abc", "/collections/99999"} {
		if rec := s.do(ana, "GET", target, ""); rec.Code != 404 {
			t.Errorf("GET %s: %d, want 404", target, rec.Code)
		}
	}
	p1 := s.collections(ana, "?limit=2&page=1")
	p3 := s.collections(ana, "?limit=2&page=3")
	if p1.Total != 5 || p1.TotalPages != 3 || len(p1.Data) != 2 || len(p3.Data) != 1 {
		t.Fatalf("paging: %+v %+v", p1, p3)
	}
	if l := s.collections(ana, "?limit=101"); l.Limit != 50 || len(l.Data) != 5 {
		t.Errorf("a limit over 100 must fall back to 50, got %d", l.Limit)
	}
	// By name, whatever the order they were made in.
	if p1.Data[0].Name != "Série A" || p1.Data[1].Name != "Série B" || p3.Data[0].Name != "Série E" {
		t.Errorf("order: %q %q ... %q", p1.Data[0].Name, p1.Data[1].Name, p3.Data[0].Name)
	}
}

func TestCollections_TheCoverIsTheOneOfTheFirstWorkInTheOrderOfTheCollection(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A segundo", "X", "a.epub", "epub")
	b := s.addWork("B primeiro", "X", "b.epub", "epub")
	s.exec(`UPDATE works SET series = 'S', series_index = 2 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = 'S', series_index = 1 WHERE id = $1`, b)
	s.exec(`UPDATE editions SET cover_url = '/covers/a.jpg' WHERE work_id = $1`, a)
	s.exec(`UPDATE editions SET cover_url = '/covers/b.jpg' WHERE work_id = $1`, b)
	if c := s.collections(ana, "").Data[0]; c.CoverURL != "/covers/b.jpg" {
		t.Errorf("list cover = %q, want the one of the work numbered 1", c.CoverURL)
	}
	d, _ := s.collection(ana, s.collectionID("S"))
	if d.Collection.CoverURL != "/covers/b.jpg" || d.Works[0].ID != b {
		t.Errorf("detail = %+v", d)
	}
}

// The edit form of the admin writes the series like any other writer, and the collection follows.
func TestCollections_EditingTheSeriesInTheFormMovesTheWork(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	put := func(series string, index float64) {
		body := fmt.Sprintf(`{"title":"Duna","author":"Frank Herbert","tags":[],"series":%q,"series_index":%v}`, series, index)
		if rec := s.do(admin, "PUT", fmt.Sprintf("/works/%d", a), body); rec.Code != 200 {
			t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
		}
	}
	put("Crônicas de Duna", 1)
	d, code := s.collection(ana, s.collectionID("Crônicas de Duna"))
	if code != 200 || len(d.Works) != 1 || d.Works[0].ID != a {
		t.Fatalf("after writing the series: %d %+v", code, d)
	}
	put("", 0)
	if d, _ := s.collection(ana, s.collectionID("Crônicas de Duna")); len(d.Works) != 0 {
		t.Errorf("clearing the series in the form left the work in the collection: %+v", d.Works)
	}
}
