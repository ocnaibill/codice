package handlers

import (
	"fmt"
	"strings"
	"testing"
)

func (s *catalogStack) editWork(work int, extra string) int {
	s.t.Helper()
	body := fmt.Sprintf(`{"title":"Obra %d"%s}`, work, extra)
	return s.do(admin, "PUT", fmt.Sprintf("/works/%d", work), body).Code
}

func (s *catalogStack) kinds(work int) string {
	s.t.Helper()
	return s.scalar(`SELECT COALESCE(unit, '-') || '|' || COALESCE(comic_kind, '-') FROM works WHERE id = $1`, work)
}

func (s *catalogStack) classify(col int64, body string) (int, string) {
	s.t.Helper()
	rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/classification", col), body)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestComicUnits_AdminSaysWhatAWorkIsInTheEditAndItIsKeptWithItsOrigin(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Vol 1", "Eiichiro Oda", "v1.cbz", "cbz")
	if s.kinds(w) != "-|-" {
		t.Fatalf("a work starts with neither: %s", s.kinds(w))
	}
	if code := s.editWork(w, `,"unit":"volume","comic_kind":"manga"`); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	if s.kinds(w) != "volume|manga" {
		t.Errorf("after the edit: %s", s.kinds(w))
	}
	// They are shown with the metadata of the work, and their origin is the person.
	d, _ := s.detail(ana, w)
	if d.Metadata.Unit != "volume" || d.Metadata.ComicKind != "manga" {
		t.Errorf("metadata = %+v", d.Metadata)
	}
	if got := s.scalar(`SELECT string_agg(field || ':' || source, ',' ORDER BY field) FROM work_field_sources WHERE work_id = $1 AND field IN ('unit', 'comic_kind')`, w); got != "comic_kind:manual,unit:manual" {
		t.Errorf("origin = %q", got)
	}
	// An edit that does not say them leaves them; an empty one clears; a value that does not change writes nothing.
	s.editWork(w, `,"series":"One Piece"`)
	if s.kinds(w) != "volume|manga" {
		t.Errorf("an edit of something else changed them: %s", s.kinds(w))
	}
	s.editWork(w, `,"unit":"chapter"`)
	if s.kinds(w) != "chapter|manga" {
		t.Errorf("only the unit changes: %s", s.kinds(w))
	}
	s.editWork(w, `,"unit":"","comic_kind":""`)
	if s.kinds(w) != "-|-" {
		t.Errorf("empty clears: %s", s.kinds(w))
	}
	// Every unit and both kinds are valid.
	for _, body := range []string{`,"unit":"oneshot"`, `,"comic_kind":"comic"`, `,"unit":" volume "`} {
		if code := s.editWork(w, body); code != 200 {
			t.Errorf("%s: %d", body, code)
		}
	}
}

func TestComicUnits_WhatIsNotAUnitOrAKindIsRefusedAndNothingIsSaved(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Vol 1", "X", "v1.cbz", "cbz")
	s.editWork(w, `,"unit":"volume"`)
	for name, body := range map[string]string{
		"unit":      `,"unit":"arc"`,
		"unit pt":   `,"unit":"volume ","comic_kind":"graphic"`,
		"kind":      `,"comic_kind":"webtoon"`,
		"kind case": `,"comic_kind":"Manga"`,
	} {
		if code := s.editWork(w, body); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if s.kinds(w) != "volume|-" {
		t.Errorf("a refused edit changed the work: %s", s.kinds(w))
	}
	// The database refuses them too.
	if _, err := s.db.Exec(`UPDATE works SET unit = 'arc' WHERE id = $1`, w); err == nil {
		t.Error("the database took a unit it does not know")
	}
	if _, err := s.db.Exec(`UPDATE works SET comic_kind = 'webtoon' WHERE id = $1`, w); err == nil {
		t.Error("the database took a kind it does not know")
	}
}

func (s *catalogStack) seriesOf(name string, units ...string) (col int64, ids []int) {
	s.t.Helper()
	for i, u := range units {
		w := s.addWork(fmt.Sprintf("%s %d", name, i+1), "Autor", fmt.Sprintf("%s%d.cbz", name, i), "cbz")
		s.exec(`UPDATE works SET series = $2, series_index = $3, unit = NULLIF($4, '') WHERE id = $1`, w, name, i+1, u)
		ids = append(ids, w)
	}
	return s.collectionID(name), ids
}

func TestComicUnits_TheCollectionPageSaysTheUnitOfEachWork(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("One Piece", "volume", "chapter", "", "oneshot")
	s.exec(`UPDATE works SET comic_kind = 'manga' WHERE id = $1`, ids[0])
	d, _ := s.collection(ana, col)
	got := map[int]string{}
	for _, w := range d.Works {
		got[w.ID] = w.Unit + "|" + w.ComicKind
	}
	if got[ids[0]] != "volume|manga" || got[ids[1]] != "chapter|" || got[ids[2]] != "|" || got[ids[3]] != "oneshot|" {
		t.Errorf("rows = %v", got)
	}
}

func TestComicUnits_OrderingANumberedGroupLeavesTheOtherGroupsAlone(t *testing.T) {
	s := newCatalogStack(t)
	// Two volumes and three chapters, each numbered on its own.
	col, ids := s.seriesOf("Naruto", "volume", "volume", "chapter", "chapter", "chapter")
	s.exec(`UPDATE works SET series_index = 1 WHERE id = $1`, ids[2])
	s.exec(`UPDATE works SET series_index = 2 WHERE id = $1`, ids[3])
	s.exec(`UPDATE works SET series_index = 3 WHERE id = $1`, ids[4])
	order := func(unit string, list ...int) int {
		var parts []string
		for _, id := range list {
			parts = append(parts, fmt.Sprint(id))
		}
		body := fmt.Sprintf(`{"unit":%q,"workIds":[%s]}`, unit, strings.Join(parts, ","))
		return s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), body).Code
	}
	if code := order("chapter", ids[4], ids[2], ids[3]); code != 204 {
		t.Fatalf("order chapters: %d", code)
	}
	num := func(id int) string { return s.scalar(`SELECT series_index FROM works WHERE id = $1`, id) }
	if num(ids[4]) != "1" || num(ids[2]) != "2" || num(ids[3]) != "3" {
		t.Errorf("chapters = %s %s %s, want 1 2 3 in the order given", num(ids[4]), num(ids[2]), num(ids[3]))
	}
	if num(ids[0]) != "1" || num(ids[1]) != "2" {
		t.Errorf("volumes were touched: %s %s", num(ids[0]), num(ids[1]))
	}
	// The list must be exactly the works of that group.
	for name, c := range map[string]int{
		"a volume in the chapters":   order("chapter", ids[4], ids[2], ids[3], ids[0]),
		"missing a chapter":          order("chapter", ids[4], ids[2]),
		"the volumes as chapters":    order("chapter", ids[0], ids[1]),
		"a group that does not have": order("oneshot", ids[0]),
		"repeated":                   order("volume", ids[0], ids[0]),
	} {
		if c != 400 {
			t.Errorf("%s: %d, want 400", name, c)
		}
	}
	if code := order("volume", ids[1], ids[0]); code != 204 {
		t.Errorf("order volumes: %d", code)
	}
	if num(ids[1]) != "1" || num(ids[0]) != "2" {
		t.Errorf("volumes = %s %s", num(ids[1]), num(ids[0]))
	}
	// A unit that does not exist is refused; without a unit the order is of the whole collection, as before.
	if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), `{"unit":"arc","workIds":[]}`); rec.Code != 400 {
		t.Errorf("unknown unit: %d, want 400", rec.Code)
	}
	if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), fmt.Sprintf(`{"workIds":[%d]}`, ids[0])); rec.Code != 400 {
		t.Errorf("a list of one work for the whole collection: %d, want 400", rec.Code)
	}
}

func TestComicUnits_ThoseWithNoUnitAreAGroupToo(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Série", "volume", "", "")
	body := fmt.Sprintf(`{"unit":"","workIds":[%d,%d]}`, ids[2], ids[1])
	if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), body); rec.Code != 204 {
		t.Fatalf("order of the works with no unit: %d", rec.Code)
	}
	if got := s.scalar(`SELECT series_index FROM works WHERE id = $1`, ids[2]); got != "1" {
		t.Errorf("first = %s, want 1", got)
	}
}

func TestComicUnits_AWorkAddedGoesToTheEndOfItsOwnGroup(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.seriesOf("Bleach", "volume", "volume", "chapter")
	s.exec(`UPDATE works SET series_index = 40 WHERE series = 'Bleach' AND unit = 'chapter'`)
	vol := s.addWork("Outro volume", "X", "o.cbz", "cbz")
	chap := s.addWork("Outro capítulo", "X", "c.cbz", "cbz")
	s.exec(`UPDATE works SET unit = 'volume' WHERE id = $1`, vol)
	s.exec(`UPDATE works SET unit = 'chapter' WHERE id = $1`, chap)
	if rec := s.addToCollection(admin, col, vol, ""); rec.Code != 204 {
		t.Fatalf("add volume: %d", rec.Code)
	}
	if rec := s.addToCollection(admin, col, chap, ""); rec.Code != 204 {
		t.Fatalf("add chapter: %d", rec.Code)
	}
	if got := s.scalar(`SELECT series_index FROM works WHERE id = $1`, vol); got != "3" {
		t.Errorf("a volume goes after volume 2, got %s", got)
	}
	if got := s.scalar(`SELECT series_index FROM works WHERE id = $1`, chap); got != "41" {
		t.Errorf("a chapter goes after chapter 40, got %s", got)
	}
	// One with no unit goes after those with none.
	none := s.addWork("Sem unidade", "X", "n.cbz", "cbz")
	s.addToCollection(admin, col, none, "")
	if got := s.scalar(`SELECT series_index FROM works WHERE id = $1`, none); got != "1" {
		t.Errorf("the first with no unit is 1, got %s", got)
	}
}

func TestComicUnits_ClassifyingACollectionAtOnce(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Berserk", "", "chapter", "", "")
	s.exec(`UPDATE works SET comic_kind = 'comic' WHERE id = $1`, ids[2])
	outside := s.addWork("De fora", "X", "x.cbz", "cbz")
	gone := ids[3]
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)

	// Only those that have no value yet.
	code, body := s.classify(col, `{"unit":"volume","comicKind":"manga","onlyUnset":true}`)
	if code != 200 || !strings.Contains(body, `"unit":2`) || !strings.Contains(body, `"comic_kind":2`) {
		t.Fatalf("classify: %d %s (unit: the two with none that are not in the trash; kind: all but the one that had one)", code, body)
	}
	for i, want := range []string{"volume|manga", "chapter|manga", "volume|comic"} {
		if got := s.kinds(ids[i]); got != want {
			t.Errorf("work %d = %s, want %s", i, got, want)
		}
	}
	if s.kinds(outside) != "-|-" || s.kinds(gone) != "-|-" {
		t.Errorf("a work outside, or in the trash, was classified: %s / %s", s.kinds(outside), s.kinds(gone))
	}
	if got := s.scalar(`SELECT count(*) FROM work_field_sources WHERE field IN ('unit', 'comic_kind') AND source = 'manual'`); got != "4" {
		t.Errorf("origins = %s, want 4 (the ones that changed)", got)
	}

	// Without onlyUnset it overrides; a field that is not there is left alone; the same value again changes nothing.
	code, body = s.classify(col, `{"comicKind":"comic"}`)
	if code != 200 || !strings.Contains(body, `"comic_kind":2`) || strings.Contains(body, `"unit"`) {
		t.Errorf("override the kind: %d %s", code, body)
	}
	if s.kinds(ids[0]) != "volume|comic" || s.kinds(ids[1]) != "chapter|comic" {
		t.Errorf("after the override: %s %s", s.kinds(ids[0]), s.kinds(ids[1]))
	}
	if _, body := s.classify(col, `{"comicKind":"comic"}`); !strings.Contains(body, `"comic_kind":0`) {
		t.Errorf("the same value again: %s", body)
	}
	// An empty value clears.
	if _, body := s.classify(col, `{"unit":""}`); !strings.Contains(body, `"unit":3`) || s.kinds(ids[0]) != "-|comic" {
		t.Errorf("clearing: %s / %s", body, s.kinds(ids[0]))
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'collection.classify'`); n != "4" {
		t.Errorf("audit entries = %s, want 4", n)
	}
}

func TestComicUnits_ClassifyingRefusesWhatMakesNoSense(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Série", "", "")
	for name, body := range map[string]string{
		"nothing to say": `{}`,
		"only the flag":  `{"onlyUnset":true}`,
		"bad unit":       `{"unit":"arc"}`,
		"bad kind":       `{"comicKind":"webtoon"}`,
		"not json":       `nope`,
	} {
		if code, _ := s.classify(col, body); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if code, _ := s.classify(99999, `{"unit":"volume"}`); code != 404 {
		t.Errorf("unknown collection: %d, want 404", code)
	}
	if rec := s.do(admin, "PUT", "/collections/abc/classification", `{"unit":"volume"}`); rec.Code != 404 {
		t.Errorf("bad id: %d, want 404", rec.Code)
	}
	var personal int64
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Minha', 'manual') RETURNING id`, idAna).Scan(&personal)
	if code, _ := s.classify(personal, `{"unit":"volume"}`); code != 404 {
		t.Errorf("a personal list: %d, want 404", code)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", col), "")
	if code, _ := s.classify(col, `{"unit":"volume"}`); code != 409 {
		t.Errorf("a retired collection: %d, want 409", code)
	}
	if s.kinds(ids[0]) != "-|-" {
		t.Errorf("a refused classification changed a work: %s", s.kinds(ids[0]))
	}
}
