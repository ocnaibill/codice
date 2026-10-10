package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) readLater(a actor, method string, work int) int {
	s.t.Helper()
	return s.do(a, method, fmt.Sprintf("/works/%d/read-later", work), "").Code
}

func (s *catalogStack) readLaterMark(a actor, work int) bool {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/works/%d", work), "")
	var out struct {
		ReadLater bool `json:"readLater"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.ReadLater
}

func TestReadLater_IsAListOfThePersonThatIsMadeTheFirstTimeAndHoldsTheWorkOnce(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Zeta", "Frank Herbert", "duna.epub", "epub")
	neuro := s.addWork("Alfa", "William Gibson", "neuro.epub", "epub")

	if n := s.scalar(`SELECT count(*) FROM collections WHERE system_key IS NOT NULL`); n != "0" {
		t.Fatalf("nobody asked for it yet: %s", n)
	}
	if s.readLaterMark(ana, duna) {
		t.Errorf("not put aside yet")
	}
	if code := s.readLater(ana, "PUT", duna); code != http.StatusNoContent {
		t.Fatalf("put aside: %d", code)
	}
	if code := s.readLater(ana, "PUT", duna); code != http.StatusNoContent { // twice is not an error and not twice in the list
		t.Errorf("again: %d", code)
	}
	s.readLater(ana, "PUT", neuro)
	if !s.readLaterMark(ana, duna) || !s.readLaterMark(ana, neuro) {
		t.Errorf("the page of the work knows it is put aside")
	}

	// One list, with the name the Códice gave it, that the person sees among theirs, with the works in the order they were put.
	if n := s.scalar(`SELECT count(*) FROM collections WHERE owner_id = $1 AND system_key = 'read_later'`, idAna); n != "1" {
		t.Fatalf("one list: %s", n)
	}
	lists := s.collections(ana, "?kind=personal")
	if lists.Total != 1 || lists.Data[0].Name != "Ler depois" || lists.Data[0].System != "read_later" || lists.Data[0].WorkCount != 2 {
		t.Fatalf("ana's lists: %+v", lists)
	}
	if got := titlesOf(s.entries(ana, lists.Data[0].ID)); strings.Join(got, "|") != "Zeta|Alfa" {
		t.Errorf("in the order they were put aside: %v", got)
	}

	// It is personal.
	if s.readLaterMark(bob, duna) {
		t.Errorf("bob did not put it aside")
	}
	if l := s.collections(bob, "?kind=personal"); l.Total != 0 {
		t.Errorf("bob sees ana's list: %+v", l)
	}

	// Taking it out, and doing it again, and from a person who has no list at all.
	if code := s.readLater(ana, "DELETE", duna); code != http.StatusNoContent {
		t.Errorf("take it out: %d", code)
	}
	s.readLater(ana, "DELETE", duna)
	if s.readLaterMark(ana, duna) || !s.readLaterMark(ana, neuro) {
		t.Errorf("only the one that was taken out")
	}
	if code := s.readLater(bob, "DELETE", neuro); code != http.StatusNoContent {
		t.Errorf("nothing to take out: %d", code)
	}
	if !s.readLaterMark(ana, neuro) {
		t.Errorf("bob took a work out of ana's list")
	}
	if n := s.scalar(`SELECT count(*) FROM collections WHERE owner_id = $1`, idBob); n != "0" {
		t.Errorf("taking out does not make the list: %s", n)
	}
}

func TestReadLater_ANameIsNotAnotherList_AndTheListIsNotRenamedOrPutAway(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	// A list the person made with the same name is theirs, another one.
	mine := s.makeList(ana, "Ler depois")
	s.readLater(ana, "PUT", work)
	var system int64
	for _, c := range s.collections(ana, "?kind=personal").Data {
		if c.System == "read_later" {
			system = c.ID
		}
	}
	if system == 0 || system == mine {
		t.Fatalf("the list of the Códice is not the one of the person: %d %d", system, mine)
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", system), `{"name":"Outro"}`); rec.Code != http.StatusConflict {
		t.Errorf("rename: %d", rec.Code)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d", system), ""); rec.Code != http.StatusConflict {
		t.Errorf("put away: %d", rec.Code)
	}
	if n := s.scalar(`SELECT name || '|' || (retired_at IS NULL)::text FROM collections WHERE id = $1`, system); n != "Ler depois|true" {
		t.Errorf("untouched: %s", n)
	}
	// Their own can be.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", mine), `{"name":"Outra"}`); rec.Code != 200 {
		t.Errorf("their own list: %d", rec.Code)
	}
	// The works in it can be taken out like in any list.
	entries := s.entries(ana, system)
	if len(entries) != 1 {
		t.Fatalf("%+v", entries)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d/entries/%d", system, entries[0].EntryID), ""); rec.Code != http.StatusNoContent {
		t.Errorf("take a work out: %d", rec.Code)
	}
}

func TestReadLater_DoesNotCountAmongTheListsAPersonMayHaveAndRefusesWhatIsNotThere(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.readLater(ana, "PUT", work)
	// 199 lists of their own, and the one of the Códice, leave room for one more: it is not one of the lists they make.
	for i := 0; i < maxPersonalCollections-1; i++ {
		s.exec(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, $2, 'manual')`, idAna, fmt.Sprintf("Lista %d", i))
	}
	if rec := s.do(ana, "POST", "/my/collections", `{"name":"A ducentésima"}`); rec.Code != http.StatusCreated {
		t.Errorf("the list of the Códice does not count: %d", rec.Code)
	}
	if rec := s.do(ana, "POST", "/my/collections", `{"name":"Mais uma"}`); rec.Code != http.StatusConflict {
		t.Errorf("the bound is of the lists they make: %d", rec.Code)
	}
	if code := s.readLater(ana, "PUT", 999999); code != http.StatusNotFound {
		t.Errorf("a work that is not there: %d", code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code := s.readLater(bob, "PUT", work); code != http.StatusConflict {
		t.Errorf("a work in the trash: %d", code)
	}
	if rec := s.do(ana, "PUT", "/works/abc/read-later", ""); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
}
