package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

type favoritesResponse struct {
	Data  []FavoriteItem
	Total int
}

func (s *catalogStack) favorites(a actor) favoritesResponse {
	s.t.Helper()
	var out favoritesResponse
	rec := s.do(a, "GET", "/favorites", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /favorites: %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (s *catalogStack) favoriteCollection(a actor, id int64) int {
	s.t.Helper()
	return s.do(a, "POST", fmt.Sprintf("/collections/%d/favorite", id), "").Code
}

func (s *catalogStack) isFavoriteCollection(a actor, id int64) bool {
	s.t.Helper()
	d, code := s.collection(a, id)
	if code != 200 {
		s.t.Fatalf("collection %d: %d", id, code)
	}
	return d.Collection.IsFavorite
}

func TestFavoriteCollections_FavoritingAndUnfavoritingAreThePersonsOwn(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Pedra Filosofal", "J. K. Rowling", "a.epub", "epub")
	s.exec(`UPDATE works SET series = 'Harry Potter', series_index = 1 WHERE id = $1`, a)
	hp := s.collectionID("Harry Potter")

	if s.isFavoriteCollection(ana, hp) {
		t.Error("a collection was a favorite before anybody chose it")
	}
	if code := s.favoriteCollection(ana, hp); code != 200 {
		t.Fatalf("favorite: %d", code)
	}
	if code := s.favoriteCollection(ana, hp); code != 200 {
		t.Errorf("favoriting twice: %d, want 200", code)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM favorite_collections`); n != "1" {
		t.Errorf("rows = %s, want 1", n)
	}
	// It is Ana's favorite and nobody else's, in the detail and in the list.
	if !s.isFavoriteCollection(ana, hp) || s.isFavoriteCollection(bob, hp) || s.isFavoriteCollection(admin, hp) {
		t.Error("the favorite is not the person's alone")
	}
	if l := s.collections(ana, ""); !l.Data[0].IsFavorite {
		t.Errorf("ana's list: %+v", l.Data[0])
	}
	if l := s.collections(bob, ""); l.Data[0].IsFavorite {
		t.Errorf("bob's list: %+v", l.Data[0])
	}
	if f := s.favorites(bob); f.Total != 0 {
		t.Errorf("bob sees ana's favorites: %+v", f)
	}

	// Taking it away is the person's, and doing it twice is not an error; bob's does not touch ana's.
	if rec := s.do(bob, "DELETE", fmt.Sprintf("/collections/%d/favorite", hp), ""); rec.Code != 200 {
		t.Errorf("bob un-favoriting what he never favorited: %d, want 200", rec.Code)
	}
	if !s.isFavoriteCollection(ana, hp) {
		t.Error("bob took ana's favorite away")
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/collections/%d/favorite", hp), ""); rec.Code != 200 {
		t.Fatalf("unfavorite: %d", rec.Code)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/collections/%d/favorite", hp), ""); rec.Code != 200 {
		t.Errorf("unfavoriting twice: %d, want 200", rec.Code)
	}
	if s.isFavoriteCollection(ana, hp) {
		t.Error("still a favorite")
	}
}

func TestFavoriteCollections_OnlyWhatThePersonCanSeeCanBeFavorited(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("A", "X", "a.epub", "epub")
	s.exec(`UPDATE works SET series = 'Saga', series_index = 1 WHERE id = $1`, w)
	saga := s.collectionID("Saga")
	list := s.makeList(ana, "Minha")

	for _, id := range []string{"0", "-1", "abc", "99999"} {
		if rec := s.do(ana, "POST", "/collections/"+id+"/favorite", ""); rec.Code != 404 {
			t.Errorf("POST /collections/%s/favorite: %d, want 404", id, rec.Code)
		}
	}
	// A list is its owner's: nobody else can favorite it, not even the staff, and for them it is not found.
	if code := s.favoriteCollection(ana, list); code != 200 {
		t.Errorf("ana favoriting her own list: %d, want 200", code)
	}
	for _, who := range []actor{bob, admin} {
		if code := s.favoriteCollection(who, list); code != 404 {
			t.Errorf("%s favoriting ana's list: %d, want 404", who.role, code)
		}
	}
	// A collection that is retired is not found.
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", saga), "")
	if code := s.favoriteCollection(bob, saga); code != 404 {
		t.Errorf("favoriting a retired collection: %d, want 404", code)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM favorite_collections`); n != "1" {
		t.Errorf("rows = %s, want only ana's", n)
	}
}

func TestFavoriteCollections_TheHomeShowsOneCardForTheCollectionAndNeverHidesAFavorite(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Pedra Filosofal", "J. K. Rowling", "a.epub", "epub")
	b := s.addWork("Câmara Secreta", "J. K. Rowling", "b.epub", "epub")
	c := s.addWork("Mistborn", "Brandon Sanderson", "c.epub", "epub")
	s.exec(`UPDATE works SET series = 'Harry Potter', series_index = 1 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = 'Harry Potter', series_index = 2 WHERE id = $1`, b)
	s.exec(`UPDATE works SET series = 'Mistborn', series_index = 1 WHERE id = $1`, c)
	hp, mist := s.collectionID("Harry Potter"), s.collectionID("Mistborn")
	for _, w := range []int{a, b, c} {
		s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", w), "")
	}
	s.exec(`DELETE FROM favorite_collections`) // the migration made some for what was favorited before; start from none

	if f := s.favorites(ana); f.Total != 3 {
		t.Fatalf("three favorite works: %+v", f)
	}
	// Favoriting Harry Potter folds its two works into one card; Mistborn's work stays a work.
	s.favoriteCollection(ana, hp)
	f := s.favorites(ana)
	if f.Total != 2 || f.Data[0].Kind != "collection" || f.Data[0].CollectionID != hp || f.Data[1].Kind != "work" || f.Data[1].WorkID != c {
		t.Fatalf("after favoriting the collection: %+v", f)
	}
	// Favoriting the other as well.
	s.favoriteCollection(ana, mist)
	if f := s.favorites(ana); f.Total != 2 || f.Data[0].CollectionID != mist || f.Data[1].CollectionID != hp {
		t.Errorf("two collections, the newest first: %+v", f)
	}
	// Un-favoriting the collection brings its favorite works back, as works.
	s.do(ana, "DELETE", fmt.Sprintf("/collections/%d/favorite", hp), "")
	f = s.favorites(ana)
	got := map[int]bool{}
	for _, it := range f.Data {
		if it.Kind == "work" {
			got[it.WorkID] = true
		}
	}
	if f.Total != 3 || !got[a] || !got[b] || f.Data[0].CollectionID != mist {
		t.Errorf("after taking the collection away: %+v", f)
	}
	// What is somebody else's stays out.
	if o := s.favorites(bob); o.Total != 0 {
		t.Errorf("bob: %+v", o)
	}
}

func TestFavoriteCollections_AListIsShownWhileItIsThereAndGoesWhenPutAway(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.exec(`UPDATE editions SET cover_url = '/covers/d.jpg' WHERE work_id = $1`, w)
	list := s.makeList(ana, "Para ler")
	s.putInList(ana, list, w, "")
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idAna, s.primaryFile(w))
	s.favoriteCollection(ana, list)

	f := s.favorites(ana)
	if f.Total != 1 || f.Data[0].Kind != "collection" || f.Data[0].CollectionKind != "personal" || f.Data[0].Title != "Para ler" ||
		f.Data[0].WorkCount != 1 || f.Data[0].CompletedCount != 1 || f.Data[0].CoverURL != "/covers/d.jpg" {
		t.Errorf("favorite list: %+v", f.Data)
	}
	// Putting the list away takes it out of the favorites; bringing it back puts it in again.
	s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d", list), "")
	if f := s.favorites(ana); f.Total != 0 {
		t.Errorf("a list that is put away is a favorite: %+v", f)
	}
	s.do(ana, "POST", fmt.Sprintf("/my/collections/%d/restore", list), "")
	if f := s.favorites(ana); f.Total != 1 {
		t.Errorf("after restoring: %+v", f)
	}
	// A work of the list that is also a favorite work is not folded: a list is not a series.
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", w), "")
	if f := s.favorites(ana); f.Total != 2 {
		t.Errorf("the favorite work must still show next to the favorite list: %+v", f)
	}
}

func TestFavoriteCollections_ARetiredOfficialCollectionIsNotShownAndItsWorksComeBack(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Vol 1", "X", "a.epub", "epub")
	s.exec(`UPDATE works SET series = 'Saga', series_index = 1 WHERE id = $1`, a)
	saga := s.collectionID("Saga")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", a), "")
	s.favoriteCollection(ana, saga)
	if f := s.favorites(ana); f.Total != 1 || f.Data[0].Kind != "collection" {
		t.Fatalf("before: %+v", f)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", saga), "")
	if f := s.favorites(ana); f.Total != 1 || f.Data[0].Kind != "work" || f.Data[0].WorkID != a {
		t.Errorf("a retired collection must not show, and the favorite work must: %+v", f)
	}
	s.do(admin, "POST", fmt.Sprintf("/collections/%d/restore", saga), "")
	if f := s.favorites(ana); f.Total != 1 || f.Data[0].Kind != "collection" {
		t.Errorf("after restoring: %+v", f)
	}
}

func TestFavoriteCollections_AFavoriteWorkSaysWhetherThePersonFinishedIt(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Lido", "X", "a.epub", "epub")
	b := s.addWork("Por ler", "X", "b.epub", "epub")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", a), "")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", b), "")
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idAna, s.primaryFile(a))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idBob, s.primaryFile(b))
	done := map[string]bool{}
	for _, it := range s.favorites(ana).Data {
		done[it.Title] = it.Completed
	}
	if !done["Lido"] || done["Por ler"] {
		t.Errorf("completed = %v (what bob read is not ana's)", done)
	}
}

func TestFavoriteCollections_AWorkInTheTrashIsNotShown(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Vol 1", "X", "a.epub", "epub")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", a), "")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, a)
	if f := s.favorites(ana); f.Total != 0 {
		t.Errorf("a work in the trash is a favorite: %+v", f)
	}
}

func TestFavoriteCollections_WhatIsNotThePersonsDoesNotHideOrShowAnything(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Pedra Filosofal", "J. K. Rowling", "a.epub", "epub")
	s.exec(`UPDATE works SET series = 'Harry Potter', series_index = 1 WHERE id = $1`, a)
	hp := s.collectionID("Harry Potter")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", a), "")
	s.exec(`DELETE FROM favorite_collections`)

	// Somebody else favoriting the collection does not fold ana's favorite work into it.
	s.favoriteCollection(bob, hp)
	if f := s.favorites(ana); f.Total != 1 || f.Data[0].Kind != "work" || f.Data[0].WorkID != a {
		t.Errorf("ana's favorite work after bob favorited the collection: %+v", f)
	}
	// A list of another person cannot be among one's favorites, whatever the table says.
	list := s.makeList(bob, "De bob")
	s.exec(`INSERT INTO favorite_collections (user_id, collection_id) VALUES ($1, $2)`, idAna, list)
	if f := s.favorites(ana); f.Total != 1 {
		t.Errorf("the list of another person among ana's favorites: %+v", f)
	}
	// A retired collection folds nothing, even if a place of it was left behind.
	s.favoriteCollection(ana, hp)
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, hp)
	if f := s.favorites(ana); f.Total != 1 || f.Data[0].Kind != "work" {
		t.Errorf("the favorite work must show when its collection is retired: %+v", f)
	}
}
