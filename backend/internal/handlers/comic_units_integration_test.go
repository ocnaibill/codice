package handlers

import (
	"encoding/json"
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

// The shelves (#187): a comic or a manga is a matter of the kind of the work, not of its format.
func TestComicShelves_TheKindDecidesTheShelfAndTheFormatOnlyWhenThereIsNone(t *testing.T) {
	s := newCatalogStack(t)
	book := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	plainComic := s.addWork("Watchmen", "Alan Moore", "watchmen.cbz", "cbz")
	mangaCbz := s.addWork("One Piece 1", "Eiichiro Oda", "op1.cbz", "cbz")
	mangaPdf := s.addWork("Akira 1", "Katsuhiro Otomo", "akira.pdf", "pdf")
	comicPdf := s.addWork("Maus", "Art Spiegelman", "maus.pdf", "pdf")
	audio := s.addWork("Hobbit", "J.R.R. Tolkien", "hobbit.m4b", "m4b")
	retiredManga := s.addWork("Velho", "Alguém", "velho.cbz", "cbz")
	// The kind a person gives wins over the format, whatever it is: an audio drama marked as a comic is on the comic
	// shelf and not on the audio one.
	audioComic := s.addWork("Drama", "Alguém", "drama.m4b", "m4b")
	s.exec(`UPDATE works SET comic_kind = 'comic' WHERE id = $1`, audioComic)
	s.exec(`UPDATE works SET comic_kind = 'manga' WHERE id IN ($1, $2, $3)`, mangaCbz, mangaPdf, retiredManga)
	s.exec(`UPDATE works SET comic_kind = 'comic' WHERE id = $1`, comicPdf)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retiredManga)

	stats := func() DashboardStats {
		var st DashboardStats
		json.Unmarshal(s.do(ana, "GET", "/stats", "").Body.Bytes(), &st)
		return st
	}
	// A manga in PDF is a manga and not a book; a comic in PDF is a comic; a CBZ with no kind is still a comic; a work
	// that was retired is on no shelf.
	if got := stats().LibraryBreakdown; got != (FormatBreakdown{Livros: 1, Quadrinhos: 3, Mangas: 2, Audio: 1}) {
		t.Errorf("library = %+v", got)
	}

	for group, want := range map[string][]int{
		"ebooks": {book},
		"comics": {plainComic, comicPdf, audioComic},
		"mangas": {mangaCbz, mangaPdf},
		"audio":  {audio},
	} {
		if got := ids(s.list(ana, "?formatGroup="+group).Data); !sameIDs(got, want...) {
			t.Errorf("formatGroup=%s = %v, want %v", group, got, want)
		}
	}
	// The pages and the total follow the filter.
	if l := s.list(ana, "?formatGroup=mangas&limit=1"); l.Total != 2 || len(l.Data) != 1 {
		t.Errorf("a page of mangas: total %d, %d on the page", l.Total, len(l.Data))
	}
	// Read last, and finished: they are counted on the shelf of the work, under the format that was read.
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 20)`, idAna, s.primaryFile(mangaPdf))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 20)`, idAna, s.primaryFile(plainComic))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 20)`, idAna, s.primaryFile(book))
	if got := stats(); got.InProgressCount != 3 || got.InProgressBreakdown != (FormatBreakdown{Livros: 1, Quadrinhos: 1, Mangas: 1}) {
		t.Errorf("in progress = %+v", got.InProgressBreakdown)
	}
	for _, w := range []int{mangaCbz, mangaPdf, comicPdf} {
		f := s.primaryFile(w)
		s.exec(`INSERT INTO reading_completions (user_id, work_id, file_id, format, completed_at) SELECT $1, $2, id, format, now() FROM files WHERE id = $3`, idAna, w, f)
	}
	if got := stats(); got.CompletedThisMonth != 3 || got.CompletedBreakdown != (FormatBreakdown{Quadrinhos: 1, Mangas: 2}) {
		t.Errorf("completed = %+v (%d)", got.CompletedBreakdown, got.CompletedThisMonth)
	}

	// Changing the kind moves the work from a shelf to another, with no other change.
	s.editWork(plainComic, `,"comic_kind":"manga"`)
	if got := stats().LibraryBreakdown; got != (FormatBreakdown{Livros: 1, Quadrinhos: 2, Mangas: 3, Audio: 1}) {
		t.Errorf("after marking the Watchmen as a manga: %+v", got)
	}
	s.editWork(mangaPdf, `,"comic_kind":""`)
	if got := stats().LibraryBreakdown; got != (FormatBreakdown{Livros: 2, Quadrinhos: 2, Mangas: 2, Audio: 1}) {
		t.Errorf("after clearing the kind of a manga in PDF (it is a book again): %+v", got)
	}
}

type seriesAnswer struct {
	Collection *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"collection"`
	Next *SeriesStep `json:"next"`
}

func (s *catalogStack) seriesOfWork(a actor, work int) (seriesAnswer, int) {
	s.t.Helper()
	var out seriesAnswer
	rec := s.do(a, "GET", fmt.Sprintf("/works/%d/series", work), "")
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func (s *catalogStack) read(user string, work int, completed bool) {
	s.t.Helper()
	if completed {
		s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, user, s.primaryFile(work))
		return
	}
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 30)`, user, s.primaryFile(work))
}

func TestSeries_TheReaderIsToldTheNextWorkOfTheSameUnit(t *testing.T) {
	s := newCatalogStack(t)
	_, ids := s.seriesOf("Naruto", "volume", "chapter", "volume", "chapter", "chapter")
	// Each group is numbered on its own: the order of the collection is the order inside the group.
	for i, n := range map[int]int{0: 1, 2: 2, 1: 1, 3: 2, 4: 3} {
		s.exec(`UPDATE collection_works SET position = $2 WHERE work_id = $1`, ids[i], n)
	}
	v1, c1, v2, c2, c3 := ids[0], ids[1], ids[2], ids[3], ids[4]
	next := func(work int) int {
		got, code := s.seriesOfWork(ana, work)
		if code != 200 || got.Collection == nil || got.Collection.Name != "Naruto" {
			t.Fatalf("series of %d: %d %+v", work, code, got)
		}
		if got.Next == nil {
			return 0
		}
		return got.Next.ID
	}
	if next(v1) != v2 || next(c1) != c2 || next(c2) != c3 {
		t.Errorf("next: v1 %d c1 %d c2 %d", next(v1), next(c1), next(c2))
	}
	if next(v2) != 0 || next(c3) != 0 {
		t.Errorf("the last of a group has no next: v2 %d c3 %d", next(v2), next(c3))
	}
	// What the step says.
	got, _ := s.seriesOfWork(ana, c1)
	if got.Next.Title != "Naruto 4" || got.Next.Unit != "chapter" || got.Next.Position == nil || *got.Next.Position != 2 || got.Next.Started {
		t.Errorf("step = %+v", got.Next)
	}
	// A next that is begun says so; one in the trash is skipped; one with no file is not a step.
	s.read(idAna, c3, false)
	if got, _ := s.seriesOfWork(ana, c2); got.Next == nil || got.Next.ID != c3 || !got.Next.Started {
		t.Errorf("a begun next: %+v", got.Next)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, c2)
	if next(c1) != c3 {
		t.Errorf("the next skips what is in the trash: %d", next(c1))
	}
	// A work with no file to open is not a step: the last one that can be read has no next.
	s.exec(`INSERT INTO works (original_title, series, series_index, unit) VALUES ('Sem arquivo', 'Naruto', 9, 'chapter')`)
	if s.scalar(`SELECT COUNT(*) FROM collection_works cw JOIN works w ON w.id = cw.work_id WHERE w.original_title = 'Sem arquivo'`) != "1" {
		t.Fatal("the work with no file is in the collection")
	}
	if next(c3) != 0 {
		t.Errorf("a work with no file was offered: %d", next(c3))
	}
	// Nor is one whose file went missing from the disk.
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, s.primaryFile(c3))
	if next(c1) != 0 {
		t.Errorf("a missing file was offered: %d", next(c1))
	}
	s.exec(`UPDATE files SET availability = 'available' WHERE id = $1`, s.primaryFile(c3))
	// Another person's reading does not change the next.
	if got, code := s.seriesOfWork(bob, c1); code != 200 || got.Next == nil || got.Next.ID != c3 || got.Next.Started {
		t.Errorf("bob: %d %+v", code, got.Next)
	}
}

func TestSeries_AWorkWithNoSeriesHasNothingToGoOnWith(t *testing.T) {
	s := newCatalogStack(t)
	alone := s.addWork("Solto", "Autor", "solto.epub", "epub")
	got, code := s.seriesOfWork(ana, alone)
	if code != 200 || got.Collection != nil || got.Next != nil {
		t.Errorf("a work in no collection: %d %+v", code, got)
	}
	if _, code := s.seriesOfWork(ana, 99999); code != 200 {
		t.Errorf("a work that does not exist is also nothing, not an error: %d", code)
	}
	if rec := s.do(ana, "GET", "/works/abc/series", ""); rec.Code != 404 {
		t.Errorf("a bad id: %d", rec.Code)
	}
	// A retired collection is not read on.
	col, ids := s.seriesOf("Velha", "chapter", "chapter")
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	if got, _ := s.seriesOfWork(ana, ids[0]); got.Collection != nil || got.Next != nil {
		t.Errorf("retired collection: %+v", got)
	}
	// Nor is a personal list a series: the work is in it too, and the series is still the official one.
	col2, ids2 := s.seriesOf("Nova", "chapter", "chapter")
	list := s.makeList(ana, "Para depois")
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 1)`, list, ids2[1])
	if got, _ := s.seriesOfWork(ana, ids2[0]); got.Collection == nil || got.Collection.ID != col2 || got.Next == nil || got.Next.ID != ids2[1] {
		t.Errorf("personal list: %+v", got)
	}
}

func TestSeries_TheCollectionPageSaysWhereToGoOn(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Bleach", "chapter", "chapter", "chapter")
	goOn := func(a actor) *SeriesStep {
		d, code := s.collection(a, col)
		if code != 200 {
			t.Fatalf("collection: %d", code)
		}
		return d.Continue
	}
	// Nothing read: the first, to begin.
	if got := goOn(ana); got == nil || got.ID != ids[0] || got.Begun || got.Started {
		t.Errorf("to begin: %+v", got)
	}
	// One finished: the next one, and it says the series was begun.
	s.read(idAna, ids[0], true)
	if got := goOn(ana); got == nil || got.ID != ids[1] || !got.Begun || got.Started {
		t.Errorf("after the first: %+v", got)
	}
	// Another person starts at the first.
	if got := goOn(bob); got == nil || got.ID != ids[0] || got.Begun {
		t.Errorf("bob: %+v", got)
	}
	// One begun is the one to go on with, even with an earlier one never read.
	s.read(idAna, ids[2], false)
	if got := goOn(ana); got == nil || got.ID != ids[2] || !got.Started {
		t.Errorf("begun: %+v", got)
	}
	// Marking a whole work as finished counts as finishing it.
	s.exec(`INSERT INTO work_reading_state (user_id, work_id) VALUES ($1, $2)`, idAna, ids[2])
	if got := goOn(ana); got == nil || got.ID != ids[1] || got.Started {
		t.Errorf("a work marked as finished: %+v", got)
	}
	// A work marked as finished is not "begun", though a version of it is still open.
	if got, _ := s.seriesOfWork(ana, ids[1]); got.Next == nil || got.Next.ID != ids[2] || got.Next.Started {
		t.Errorf("the next, marked as finished: %+v", got.Next)
	}
	// Reading on a file that went missing counts for nothing: it is not where to go on.
	s.read(idAna, ids[1], false)
	lost := s.primaryFile(ids[1])
	s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, ids[1])), "cbz", "outra.cbz", "managed")
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, lost)
	if got := goOn(ana); got == nil || got.ID != ids[1] || got.Started {
		t.Errorf("progress on a missing file: %+v", got)
	}
	s.exec(`DELETE FROM reading_progress WHERE file_id = $1`, lost)
	s.exec(`UPDATE files SET availability = 'available' WHERE id = $1`, lost)
	s.read(idAna, ids[1], true)
	if got := goOn(ana); got != nil {
		t.Errorf("everything finished: %+v", got)
	}
	// A work in the trash is not offered.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[0])
	if got := goOn(bob); got == nil || got.ID != ids[1] {
		t.Errorf("a work in the trash: %+v", got)
	}
}

func TestSeries_TheCollectionPageOfAListOrARetiredOneHasNothingToGoOnWith(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.seriesOf("Retirada", "chapter", "chapter")
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	d, code := s.collection(admin, col)
	if code != 200 || d.Continue != nil {
		t.Errorf("retired: %d %+v", code, d.Continue)
	}
	list := s.makeList(ana, "Lista")
	_, ids := s.seriesOf("Outra", "chapter")
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 1)`, list, ids[0])
	d, code = s.collection(ana, list)
	if code != 200 || d.Continue != nil || len(d.Works) != 1 {
		t.Errorf("personal list: %d %+v %d works", code, d.Continue, len(d.Works))
	}
}
