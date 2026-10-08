package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func (s *catalogStack) collapsedRaw(query string) map[string]any {
	s.t.Helper()
	var out map[string]any
	json.Unmarshal(s.do(ana, "GET", "/works"+query, "").Body.Bytes(), &out)
	return out
}

func (s *catalogStack) collapsed(a actor, query string) workList {
	s.t.Helper()
	sep := "?"
	if strings.Contains(query, "?") {
		sep = "&"
	}
	return s.list(a, query+sep+"series=collapse")
}

func titles(works []Work) string {
	var out []string
	for _, w := range works {
		out = append(out, w.Title)
	}
	return strings.Join(out, "|")
}

func TestSeriesCards_AllTheWorksOfASeriesAreOneCard(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	_, naruto := s.seriesOf("Naruto", "volume", "chapter", "chapter", "volume", "chapter")
	akira := s.addWork("Akira", "Otomo", "akira.cbz", "cbz")
	s.exec(`UPDATE works SET series = 'Akira', series_index = 1, unit = 'volume' WHERE id = $1`, akira)
	// A work with no unit stays a work, even the newest of a series that has units, and the series stands on its newest work with one.
	extra := s.addWork("Naruto Extra", "Autor", "extra.cbz", "cbz")
	s.exec(`UPDATE works SET series = 'Naruto', series_index = 99 WHERE id = $1`, extra)
	// A series of books has no unit: it is not put together.
	h1 := s.addWork("Harry 1", "Rowling", "h1.epub", "epub")
	h2 := s.addWork("Harry 2", "Rowling", "h2.epub", "epub")
	s.exec(`UPDATE works SET series = 'Harry', series_index = 1 WHERE id = $1`, h1)
	s.exec(`UPDATE works SET series = 'Harry', series_index = 2 WHERE id = $1`, h2)

	// What the list always was, when nobody asks for the series.
	if got := ids(s.list(ana, "").Data); len(got) != 10 {
		t.Fatalf("plain list: %v", got)
	}
	got := s.collapsed(ana, "")
	// Newest first: Harry 2, Harry 1, the Extra with no unit, the single volume of Akira (a series of one is a work), the newest of
	// Naruto, Duna.
	if want := []int{h2, h1, extra, akira, naruto[4], duna}; !equalIDs(ids(got.Data), want) {
		t.Fatalf("collapsed list = %v, want %v", ids(got.Data), want)
	}
	if got.Total != 6 {
		t.Errorf("total = %d, want 6", got.Total)
	}
	card := got.Data[4].Collapsed
	if card == nil || card.Name != "Naruto" || card.Volumes != 2 || card.Chapters != 3 || card.OneShots != 0 {
		t.Fatalf("the card of Naruto: %+v", card)
	}
	for _, w := range []Work{got.Data[0], got.Data[1], got.Data[2], got.Data[3], got.Data[5]} {
		if w.Collapsed != nil {
			t.Errorf("%s is a work, not a series: %+v", w.Title, w.Collapsed)
		}
	}
	// The cards of the works are the same as always: title, author, format.
	if got.Data[4].Title != "Naruto 5" || got.Data[5].Author != "Frank Herbert" {
		t.Errorf("the cards: %q %q", got.Data[4].Title, got.Data[5].Author)
	}
}

func equalIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSeriesCards_PagesAndTotalCountCards(t *testing.T) {
	s := newCatalogStack(t)
	s.seriesOf("Bleach", "chapter", "chapter", "chapter", "chapter")
	for i := 0; i < 5; i++ {
		s.addWork(fmt.Sprintf("Livro %d", i), "A", fmt.Sprintf("l%d.epub", i), "epub")
	}
	seen := map[string]bool{}
	var order []string
	for page := 1; page <= 3; page++ {
		l := s.collapsed(ana, fmt.Sprintf("?limit=2&page=%d", page))
		if l.Total != 6 {
			t.Errorf("page %d: total = %d, want 6", page, l.Total)
		}
		for _, w := range l.Data {
			key := w.Title
			if w.Collapsed != nil {
				key = "serie:" + w.Collapsed.Name
			}
			if seen[key] {
				t.Errorf("%s came twice", key)
			}
			seen[key] = true
			order = append(order, key)
		}
	}
	if len(seen) != 6 || order[len(order)-1] != "serie:Bleach" {
		t.Errorf("the six cards, the series last (it is the oldest): %v", order)
	}
}

func TestSeriesCards_TheSeriesIsSortedByItsName(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Zebra", "Autor", "z.epub", "epub")
	s.addWork("Moby Dick", "Autor", "m.epub", "epub")
	_, ids := s.seriesOf("Naruto", "chapter", "chapter")
	// The works of the series are called by a name that would come first: the series is sorted by its own.
	s.exec(`UPDATE works SET original_title = 'Aardvark ' || id WHERE id = ANY($1)`, pq.Array(ids))
	names := func(l workList) string {
		var out []string
		for _, w := range l.Data {
			if w.Collapsed != nil {
				out = append(out, "serie:"+w.Collapsed.Name)
			} else {
				out = append(out, w.Title)
			}
		}
		return strings.Join(out, "|")
	}
	if got := names(s.collapsed(ana, "?sort=title")); got != "Moby Dick|serie:Naruto|Zebra" {
		t.Errorf("by title: %v", got)
	}
	// Same author: the name breaks the tie, and a series comes once.
	if got := names(s.collapsed(ana, "?sort=author")); got != "Moby Dick|serie:Naruto|Zebra" {
		t.Errorf("by author: %v", got)
	}
}

func TestSeriesCards_TheShelfDecidesWhichWorksCount(t *testing.T) {
	s := newCatalogStack(t)
	_, ids := s.seriesOf("Mista", "chapter", "chapter", "chapter")
	// The newest of the series is a comic, the other two are manga.
	s.exec(`UPDATE works SET comic_kind = 'manga' WHERE id IN ($1, $2)`, ids[0], ids[1])
	s.exec(`UPDATE works SET comic_kind = 'comic' WHERE id = $1`, ids[2])
	mangas := s.collapsed(ana, "?formatGroup=mangas")
	if len(mangas.Data) != 1 || mangas.Data[0].ID != ids[1] || mangas.Data[0].Collapsed == nil || mangas.Total != 1 {
		t.Errorf("the manga shelf: the series stands on its newest manga: %v (total %d)", ids2(mangas.Data), mangas.Total)
	}
	comics := s.collapsed(ana, "?formatGroup=comics")
	if len(comics.Data) != 1 || comics.Data[0].ID != ids[2] || comics.Data[0].Collapsed == nil {
		t.Errorf("the comic shelf: %v", ids2(comics.Data))
	}
	// The card says what the series has, whichever shelf asks.
	if c := mangas.Data[0].Collapsed; c.Chapters != 3 {
		t.Errorf("the card counts the whole series: %+v", c)
	}
	if got := s.collapsed(ana, "?formatGroup=ebooks"); got.Total != 0 {
		t.Errorf("the book shelf: %d", got.Total)
	}
}

func ids2(works []Work) []int { return ids(works) }

func TestSeriesCards_OnlyThePlainGridPutsASeriesTogether(t *testing.T) {
	s := newCatalogStack(t)
	_, ids := s.seriesOf("Bleach", "chapter", "chapter", "chapter")
	s.exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, idAna, ids[0])
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, '3', 30)`, idAna, s.primaryFile(ids[1]))
	// The favorite and the one in progress are not the newest of the series: they are shown as the works they are.
	for q, want := range map[string]int{"?favorite=true": ids[0], "?inProgress=true": ids[1]} {
		l := s.collapsed(ana, q)
		if l.Total != 1 || len(l.Data) != 1 || l.Data[0].ID != want || l.Data[0].Collapsed != nil {
			t.Errorf("%s: %v (total %d)", q, ids2(l.Data), l.Total)
		}
	}
	for _, w := range s.collapsed(ana, "?search=Bleach").Data {
		if w.Collapsed != nil {
			t.Error("search: a series card among the works")
		}
	}
	if l := s.collapsed(ana, "?search=Bleach"); l.Total != 3 {
		t.Errorf("the search finds each chapter: %d", l.Total)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[2])
	if l := s.collapsed(admin, "?retired=true"); l.Total != 1 || l.Data[0].Collapsed != nil {
		t.Errorf("the trash: %d", l.Total)
	}
}

func TestSeriesCards_WhatIsRetiredIsNotInTheSeries(t *testing.T) {
	s := newCatalogStack(t)
	_, ids := s.seriesOf("Bleach", "chapter", "chapter", "chapter")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[2])
	l := s.collapsed(ana, "")
	if len(l.Data) != 1 || l.Data[0].ID != ids[1] || l.Data[0].Collapsed == nil || l.Data[0].Collapsed.Chapters != 2 {
		t.Errorf("the newest that is left stands for it: %v %+v", ids2(l.Data), l.Data[0].Collapsed)
	}
	// With one left, it is a work again.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[1])
	l = s.collapsed(ana, "")
	if len(l.Data) != 1 || l.Data[0].ID != ids[0] || l.Data[0].Collapsed != nil {
		t.Errorf("a series of one: %v", ids2(l.Data))
	}
}

func TestSeriesCards_ARetiredCollectionIsNotASeries(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Velha", "chapter", "chapter", "chapter")
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	l := s.collapsed(ana, "")
	if l.Total != 3 || !sameIDs(ids2(l.Data), ids...) {
		t.Errorf("the works of a retired collection are works: %v", ids2(l.Data))
	}
	for _, w := range l.Data {
		if w.Collapsed != nil {
			t.Errorf("%d: %+v", w.ID, w.Collapsed)
		}
	}
}

func TestSeriesCards_TheCardSaysWhereToGoOnAndShowsTheFirstCover(t *testing.T) {
	s := newCatalogStack(t)
	_, ids := s.seriesOf("Bleach", "chapter", "chapter", "chapter")
	s.exec(`UPDATE editions SET cover_url = '/covers/first.jpg' WHERE work_id = $1`, ids[0])
	s.exec(`UPDATE editions SET cover_url = '/covers/last.jpg' WHERE work_id = $1`, ids[2])
	s.read(idAna, ids[0], true)
	card := func(a actor) Work { return s.collapsed(a, "").Data[0] }
	if w := card(ana); w.Collapsed.Continue == nil || w.Collapsed.Continue.ID != ids[1] || !w.Collapsed.Continue.Begun {
		t.Errorf("ana goes on at the second: %+v", w.Collapsed.Continue)
	}
	if w := card(bob); w.Collapsed.Continue == nil || w.Collapsed.Continue.ID != ids[0] || w.Collapsed.Continue.Begun {
		t.Errorf("bob begins: %+v", w.Collapsed.Continue)
	}
	// The cover is the one the collection shows (the first), not the cover of the newest work that carries the card.
	if w := card(ana); w.CoverURL != "/covers/first.jpg" || w.Collapsed.CoverURL != "/covers/first.jpg" {
		t.Errorf("cover: %q / %q", w.CoverURL, w.Collapsed.CoverURL)
	}
	s.read(idAna, ids[1], true)
	s.read(idAna, ids[2], true)
	if w := card(ana); w.Collapsed.Continue != nil {
		t.Errorf("everything read: %+v", w.Collapsed.Continue)
	}
}

func TestSeriesCards_TheListSaysWhetherItsTotalCountsCards(t *testing.T) {
	s := newCatalogStack(t)
	s.seriesOf("Bleach", "chapter", "chapter")
	if got := s.collapsedRaw("?series=collapse")["series"]; got != true {
		t.Errorf("collapsed: series = %v", got)
	}
	if got := s.collapsedRaw("")["series"]; got != false {
		t.Errorf("plain: series = %v", got)
	}
	if got := s.collapsedRaw("?series=collapse&search=Bleach")["series"]; got != false {
		t.Errorf("a search is not collapsed: series = %v", got)
	}
}

func TestSeriesCards_ALibraryWithNoUnitIsTheListItAlwaysWas(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.addWork("Hobbit", "Tolkien", "hobbit.m4b", "m4b")
	if got := s.collapsedRaw("?series=collapse")["series"]; got != false {
		t.Errorf("nothing to put together: series = %v", got)
	}
	l := s.collapsed(ana, "")
	if l.Total != 2 || len(l.Data) != 2 {
		t.Errorf("list: %d", l.Total)
	}
	for _, q := range []string{"?formatGroup=ebooks", "?sort=title", "?sort=author", "?formatGroup=audio&sort=author"} {
		if got := s.collapsed(ana, q); got.Total == 0 {
			t.Errorf("%s found nothing", q)
		}
	}
}

func TestSeriesCards_TheCardCountsTheWorksThatCameInTheLastSevenDaysAndWereNotFinished(t *testing.T) {
	s := newCatalogStack(t)
	_, ids := s.seriesOf("Bleach", "chapter", "chapter", "chapter", "chapter", "chapter")
	card := func(a actor) *SeriesCard { return s.collapsed(a, "").Data[0].Collapsed }
	// Everything came a month ago, but a few: a bit under seven days ago is new, a bit over is not.
	s.exec(`UPDATE works SET created_at = now() - interval '30 days' WHERE id = ANY($1)`, pq.Array(ids))
	if got := card(ana).NewCount; got != 0 {
		t.Fatalf("nothing is new: %d", got)
	}
	s.exec(`UPDATE works SET created_at = now() - interval '6 days 23 hours' WHERE id = $1`, ids[3])
	s.exec(`UPDATE works SET created_at = now() - interval '7 days 1 hour' WHERE id = $1`, ids[2])
	s.exec(`UPDATE works SET created_at = now() WHERE id = $1`, ids[4])
	if got := card(ana).NewCount; got != 2 {
		t.Errorf("just under seven days and today are new, just over is not: %d", got)
	}
	// It is for everybody, and what a person finished is not new to them.
	s.read(idAna, ids[4], true)
	if a, b := card(ana).NewCount, card(bob).NewCount; a != 1 || b != 2 {
		t.Errorf("ana finished one: ana %d, bob %d", a, b)
	}
	// A work begun and not finished is still new; one marked as finished is not.
	s.read(idAna, ids[3], false)
	if got := card(ana).NewCount; got != 1 {
		t.Errorf("begun is not finished: %d", got)
	}
	s.exec(`INSERT INTO work_reading_state (user_id, work_id) VALUES ($1, $2)`, idAna, ids[3])
	if got := card(ana).NewCount; got != 0 {
		t.Errorf("marked as finished: %d", got)
	}
	// Reading none of the series, or reading all the old ones, changes nothing for what is new.
	s.read(idBob, ids[0], true)
	if got := card(bob).NewCount; got != 2 {
		t.Errorf("bob read an old one: %d", got)
	}
	// What is in the trash, or has no file on the disk, is not counted.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, ids[4])
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, s.primaryFile(ids[3]))
	if got := card(bob).NewCount; got != 0 {
		t.Errorf("trash and missing: %d", got)
	}
}
