package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func (s *catalogStack) makeList(a actor, name string) int64 {
	s.t.Helper()
	rec := s.do(a, "POST", "/my/collections", fmt.Sprintf(`{"name":%q}`, name))
	if rec.Code != 201 {
		s.t.Fatalf("POST /my/collections: %d %s", rec.Code, rec.Body.String())
	}
	var out struct{ ID int64 }
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.ID
}

func (s *catalogStack) putInList(a actor, list int64, work int, body string) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.do(a, "PUT", fmt.Sprintf("/my/collections/%d/works/%d", list, work), body)
}

// entries are the places of a list as the person reads it, in order.
func (s *catalogStack) entries(a actor, list int64) []CollectionWork {
	s.t.Helper()
	d, code := s.collection(a, list)
	if code != 200 {
		s.t.Fatalf("list %d: %d", list, code)
	}
	return d.Works
}

func titlesOf(entries []CollectionWork) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Title)
	}
	return out
}

func TestPersonalCollections_CreateRenameAndTheListOfTheCaller(t *testing.T) {
	s := newCatalogStack(t)
	id := s.makeList(ana, "  Para   ler  ")
	if n := s.scalar(`SELECT name || '|' || kind || '|' || owner_id::text FROM collections WHERE id = $1`, id); n != "Para ler|personal|"+idAna {
		t.Errorf("list = %s", n)
	}
	// An empty list is shown: it is the person's own.
	if l := s.collections(ana, "?kind=personal"); l.Total != 1 || l.Data[0].Name != "Para ler" || l.Data[0].WorkCount != 0 || l.Data[0].Kind != "personal" {
		t.Errorf("ana's lists = %+v", l)
	}
	if l := s.collections(bob, "?kind=personal"); l.Total != 0 {
		t.Errorf("bob sees ana's list: %+v", l)
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", id), `{"name":" Depois "}`); rec.Code != 200 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT name FROM collections WHERE id = $1`, id); n != "Depois" {
		t.Errorf("name = %q", n)
	}
	// The same name twice is the person's business.
	s.makeList(ana, "Depois")
	for _, bad := range []string{`{"name":""}`, `{"name":"   "}`, `nope`, fmt.Sprintf(`{"name":%q}`, strings.Repeat("x", 513))} {
		if rec := s.do(ana, "POST", "/my/collections", bad); rec.Code != 400 {
			t.Errorf("POST %.30s: %d, want 400", bad, rec.Code)
		}
		if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", id), bad); rec.Code != 400 {
			t.Errorf("PATCH %.30s: %d, want 400", bad, rec.Code)
		}
	}
}

func TestPersonalCollections_AnotherPersonsListIsNotFoundNeverForbidden(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("A", "X", "a.epub", "epub")
	list := s.makeList(ana, "Minha")
	s.putInList(ana, list, w, "")
	entry := s.entries(ana, list)[0].EntryID

	for _, who := range []actor{bob, admin} {
		calls := map[string]*httptest.ResponseRecorder{
			"detail":  s.do(who, "GET", fmt.Sprintf("/collections/%d", list), ""),
			"rename":  s.do(who, "PATCH", fmt.Sprintf("/my/collections/%d", list), `{"name":"x"}`),
			"add":     s.putInList(who, list, w, ""),
			"remove":  s.do(who, "DELETE", fmt.Sprintf("/my/collections/%d/entries/%d", list, entry), ""),
			"order":   s.do(who, "PUT", fmt.Sprintf("/my/collections/%d/order", list), fmt.Sprintf(`{"entryIds":[%d]}`, entry)),
			"retire":  s.do(who, "DELETE", fmt.Sprintf("/my/collections/%d", list), ""),
			"restore": s.do(who, "POST", fmt.Sprintf("/my/collections/%d/restore", list), ""),
		}
		for name, rec := range calls {
			if rec.Code != 404 {
				t.Errorf("%s: %s on ana's list: %d, want 404", who.role, name, rec.Code)
			}
		}
	}
	// And none of it did anything.
	if got := s.entries(ana, list); len(got) != 1 || s.scalar(`SELECT name FROM collections WHERE id = $1`, list) != "Minha" || s.scalar(`SELECT COALESCE(retired_at::text, 'live') FROM collections WHERE id = $1`, list) != "live" {
		t.Errorf("the list changed: %+v", got)
	}
	// The official collections are not managed from here, and the lists are not managed from there.
	col, _ := s.createCollection(admin, "Oficial")
	if rec := s.do(admin, "PATCH", fmt.Sprintf("/my/collections/%d", col), `{"name":"x"}`); rec.Code != 404 {
		t.Errorf("an official one through the personal route: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", list), `{"name":"x"}`); rec.Code != 404 {
		t.Errorf("a personal one through the official route: %d, want 404", rec.Code)
	}
	for _, target := range []string{"/my/collections/abc", "/my/collections/0", "/my/collections/99999"} {
		if rec := s.do(ana, "PATCH", target, `{"name":"x"}`); rec.Code != 404 {
			t.Errorf("PATCH %s: %d, want 404", target, rec.Code)
		}
	}
}

func TestPersonalCollections_AddingGoesToTheEndAndAWorkCanBeInManyLists(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	b := s.addWork("B", "X", "b.epub", "epub")
	c := s.addWork("C", "X", "c.epub", "epub")
	one := s.makeList(ana, "Um")
	two := s.makeList(ana, "Dois")

	for _, w := range []int{a, b} {
		if rec := s.putInList(ana, one, w, ""); rec.Code != 204 {
			t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
		}
	}
	s.putInList(ana, one, c, `{"position": 10}`)
	if got := titlesOf(s.entries(ana, one)); strings.Join(got, ",") != "A,B,C" {
		t.Errorf("order = %v", got)
	}
	// The same work in another list, and a list holds it once.
	s.putInList(ana, two, a, "")
	s.putInList(ana, one, a, "") // already there, no place given: stays
	if n := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE work_id = $1 AND NOT official`, a); n != "2" {
		t.Errorf("a is in %s lists, want 2", n)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE collection_id = $1`, one); n != "3" {
		t.Errorf("list one has %s places, want 3", n)
	}
	if p := s.entries(ana, one)[0].Position; p == nil || *p != 1 {
		t.Errorf("a stayed at %v, want 1", p)
	}
	// Putting it again with a place moves it.
	s.putInList(ana, one, a, `{"position": 20}`)
	if got := titlesOf(s.entries(ana, one)); strings.Join(got, ",") != "B,C,A" {
		t.Errorf("order after moving a = %v", got)
	}
	// A work in a list is not in an official collection because of it, and the series does not take it out of the list.
	s.exec(`UPDATE works SET series = 'Saga', series_index = 1 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = NULL WHERE id = $1`, a)
	if n := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE work_id = $1 AND NOT official`, a); n != "2" {
		t.Errorf("the series took the work out of a list: %s", n)
	}
	// A list of the person is not the official collection of the work.
	if got := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE work_id = $1 AND official`, b); got != "0" {
		t.Errorf("an official membership appeared: %s", got)
	}
}

func TestPersonalCollections_AddingIsRefusedWhereItMakesNoSense(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	gone := s.addWork("Gone", "X", "g.epub", "epub")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)
	list := s.makeList(ana, "Minha")

	if rec := s.putInList(ana, list, gone, ""); rec.Code != 409 {
		t.Errorf("a work in the trash: %d, want 409", rec.Code)
	}
	if rec := s.putInList(ana, list, 99999, ""); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if rec := s.putInList(ana, list, a, `{"position": -1}`); rec.Code != 400 {
		t.Errorf("negative place: %d, want 400", rec.Code)
	}
	if rec := s.putInList(ana, list, a, `{"position":`); rec.Code != 400 {
		t.Errorf("broken body: %d, want 400", rec.Code)
	}
	for _, target := range []string{"/my/collections/1/works/abc", "/my/collections/1/works/99999999999", "/my/collections/abc/works/1"} {
		if rec := s.do(ana, "PUT", target, ""); rec.Code != 404 {
			t.Errorf("PUT %s: %d, want 404", target, rec.Code)
		}
	}
	// A list put away takes nothing and gives nothing until it is back.
	s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d", list), "")
	if rec := s.putInList(ana, list, a, ""); rec.Code != 409 {
		t.Errorf("adding to a retired list: %d, want 409", rec.Code)
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", list), `{"name":"x"}`); rec.Code != 409 {
		t.Errorf("renaming a retired list: %d, want 409", rec.Code)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d/entries/1", list), ""); rec.Code != 409 {
		t.Errorf("removing from a retired list: %d, want 409", rec.Code)
	}
	if rec := s.do(ana, "PUT", fmt.Sprintf("/my/collections/%d/order", list), `{"entryIds":[]}`); rec.Code != 409 {
		t.Errorf("ordering a retired list: %d, want 409", rec.Code)
	}
}

func TestPersonalCollections_RemovingAndOrderingNameThePlaces(t *testing.T) {
	s := newCatalogStack(t)
	var works []int
	list := s.makeList(ana, "Minha")
	other := s.makeList(ana, "Outra")
	for i := 0; i < 3; i++ {
		w := s.addWork(fmt.Sprintf("Obra %d", i), "X", fmt.Sprintf("o%d.epub", i), "epub")
		works = append(works, w)
		s.putInList(ana, list, w, "")
	}
	s.putInList(ana, other, works[0], "")
	es := s.entries(ana, list)
	otherEntry := s.entries(ana, other)[0].EntryID

	order := fmt.Sprintf(`{"entryIds":[%d,%d,%d]}`, es[2].EntryID, es[0].EntryID, es[1].EntryID)
	if rec := s.do(ana, "PUT", fmt.Sprintf("/my/collections/%d/order", list), order); rec.Code != 204 {
		t.Fatalf("order: %d %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(titlesOf(s.entries(ana, list)), ","); got != "Obra 2,Obra 0,Obra 1" {
		t.Errorf("order = %s", got)
	}
	for i, e := range s.entries(ana, list) {
		if e.Position == nil || *e.Position != float64(i+1) {
			t.Errorf("place %d is numbered %v, want %d", i, e.Position, i+1)
		}
	}
	// The list must be all the places of this list, each once.
	for name, body := range map[string]string{
		"missing":  fmt.Sprintf(`{"entryIds":[%d,%d]}`, es[0].EntryID, es[1].EntryID),
		"repeated": fmt.Sprintf(`{"entryIds":[%d,%d,%d,%d]}`, es[0].EntryID, es[0].EntryID, es[1].EntryID, es[2].EntryID),
		"foreign":  fmt.Sprintf(`{"entryIds":[%d,%d,%d]}`, es[0].EntryID, es[1].EntryID, otherEntry),
		"unknown":  fmt.Sprintf(`{"entryIds":[%d,%d,99999]}`, es[0].EntryID, es[1].EntryID),
		"empty":    `{"entryIds":[]}`,
		"not json": `nope`,
	} {
		if rec := s.do(ana, "PUT", fmt.Sprintf("/my/collections/%d/order", list), body); rec.Code != 400 {
			t.Errorf("order %s: %d, want 400", name, rec.Code)
		}
	}
	if got := strings.Join(titlesOf(s.entries(ana, list)), ","); got != "Obra 2,Obra 0,Obra 1" {
		t.Errorf("a refused order changed the list: %s", got)
	}

	// Taking a place out: only the place of this list.
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d/entries/%d", list, otherEntry), ""); rec.Code != 404 {
		t.Errorf("the place of another list: %d, want 404", rec.Code)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d/entries/%d", list, es[0].EntryID), ""); rec.Code != 204 {
		t.Fatalf("remove: %d", rec.Code)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d/entries/%d", list, es[0].EntryID), ""); rec.Code != 404 {
		t.Errorf("removing twice: %d, want 404", rec.Code)
	}
	if got := strings.Join(titlesOf(s.entries(ana, list)), ","); got != "Obra 2,Obra 1" {
		t.Errorf("after removing = %s", got)
	}
	if s.scalar(`SELECT COUNT(*) FROM works WHERE id = $1`, works[2]) != "1" {
		t.Error("removing a place deleted the work")
	}
	if rec := s.do(ana, "DELETE", "/my/collections/abc/entries/1", ""); rec.Code != 404 {
		t.Errorf("bad list id: %d", rec.Code)
	}
}

func TestPersonalCollections_AWorkThatLeavesTheLibraryKeepsItsPlaceWithItsLabel(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	b := s.addWork("Neuromancer", "William Gibson", "b.epub", "epub")
	list := s.makeList(ana, "Clássicos")
	s.putInList(ana, list, a, "")
	s.putInList(ana, list, b, "")

	// In the trash: still there, marked, and it counts for nothing.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, a)
	es := s.entries(ana, list)
	if len(es) != 2 || es[0].Title != "Duna" || es[0].Available || es[0].ID != a || !es[1].Available {
		t.Fatalf("with a work in the trash: %+v", es)
	}
	if l := s.collections(ana, "?kind=personal"); l.Data[0].WorkCount != 1 {
		t.Errorf("count = %d, want only the one that is available", l.Data[0].WorkCount)
	}

	// Deleted for good: the place stays, with what the person saw.
	s.exec(`DELETE FROM works WHERE id = $1`, a)
	es = s.entries(ana, list)
	if len(es) != 2 {
		t.Fatalf("a deleted work took its place: %+v", es)
	}
	if e := es[0]; e.Title != "Duna" || e.Author != "Frank Herbert" || e.Available || e.ID != 0 || e.EntryID == 0 {
		t.Errorf("the place of the deleted work = %+v", e)
	}
	d, _ := s.collection(ana, list)
	if d.Collection.WorkCount != 1 {
		t.Errorf("count = %d, want 1", d.Collection.WorkCount)
	}
	// It can be ordered and taken out like any other.
	order := fmt.Sprintf(`{"entryIds":[%d,%d]}`, es[1].EntryID, es[0].EntryID)
	if rec := s.do(ana, "PUT", fmt.Sprintf("/my/collections/%d/order", list), order); rec.Code != 204 {
		t.Errorf("order with a gone work: %d", rec.Code)
	}
	if got := strings.Join(titlesOf(s.entries(ana, list)), ","); got != "Neuromancer,Duna" {
		t.Errorf("order = %s", got)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d/entries/%d", list, es[0].EntryID), ""); rec.Code != 204 {
		t.Errorf("removing the place of a gone work: %d", rec.Code)
	}
	if got := titlesOf(s.entries(ana, list)); len(got) != 1 {
		t.Errorf("after removing = %v", got)
	}
}

func TestPersonalCollections_RetiringAndRestoringKeepTheWorksInIt(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	list := s.makeList(ana, "Minha")
	s.putInList(ana, list, a, "")

	living := s.makeList(ana, "Viva")
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d", list), ""); rec.Code != 204 {
		t.Fatalf("retire: %d", rec.Code)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/my/collections/%d", list), ""); rec.Code != 204 {
		t.Errorf("retiring twice: %d, want 204", rec.Code)
	}
	if l := s.collections(ana, "?kind=personal"); l.Total != 1 || l.Data[0].ID != living {
		t.Errorf("the live lists = %+v, want only the live one", l)
	}
	if l := s.collections(ana, "?kind=personal&retired=true"); l.Total != 1 || l.Data[0].ID != list || !l.Data[0].Retired || l.Data[0].WorkCount != 1 {
		t.Errorf("retired lists = %+v (its works stay in it)", l)
	}
	if l := s.collections(bob, "?kind=personal&retired=true"); l.Total != 0 {
		t.Errorf("bob sees ana's retired list: %+v", l)
	}
	// The owner can still open it, and it says it is put away.
	if d, code := s.collection(ana, list); code != 200 || !d.Collection.Retired || len(d.Works) != 1 {
		t.Errorf("detail = %d %+v", code, d)
	}
	if rec := s.do(ana, "POST", fmt.Sprintf("/my/collections/%d/restore", list), ""); rec.Code != 204 {
		t.Fatalf("restore: %d", rec.Code)
	}
	if rec := s.do(ana, "POST", fmt.Sprintf("/my/collections/%d/restore", list), ""); rec.Code != 204 {
		t.Errorf("restoring one that is not retired: %d, want 204", rec.Code)
	}
	if l := s.collections(ana, "?kind=personal"); l.Total != 2 || l.Data[0].ID != list || l.Data[0].WorkCount != 1 || l.Data[0].Retired {
		t.Errorf("after restoring = %+v", l)
	}
}

func TestPersonalCollections_TheBoundsOfWhatOneAccountKeeps(t *testing.T) {
	s := newCatalogStack(t)
	// The lists: 200 live ones is the most; one put away does not count, and bringing it back does.
	s.exec(`INSERT INTO collections (kind, owner_id, name, origin) SELECT 'personal', $1::uuid, 'Lista ' || g, 'manual' FROM generate_series(1, 200) g`, idAna)
	if rec := s.do(ana, "POST", "/my/collections", `{"name":"mais uma"}`); rec.Code != 409 {
		t.Errorf("the 201st list: %d, want 409", rec.Code)
	}
	if rec := s.do(bob, "POST", "/my/collections", `{"name":"a de bob"}`); rec.Code != 201 {
		t.Errorf("bob's first list: %d, want 201 (the bound is per person)", rec.Code)
	}
	id := s.scalar(`SELECT id FROM collections WHERE name = 'Lista 1' AND owner_id = $1`, idAna)
	s.do(ana, "DELETE", "/my/collections/"+id, "")
	if rec := s.do(ana, "POST", "/my/collections", `{"name":"mais uma"}`); rec.Code != 201 {
		t.Errorf("a list after putting one away: %d, want 201", rec.Code)
	}
	if rec := s.do(ana, "POST", "/my/collections/"+id+"/restore", ""); rec.Code != 409 {
		t.Errorf("bringing one back over the bound: %d, want 409", rec.Code)
	}

	// The works in a list: 5000 places is the most.
	w := s.addWork("A", "X", "a.epub", "epub")
	list := s.makeList(bob, "Cheia")
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, label) SELECT $1, NULL, FALSE, 'obra ' || g FROM generate_series(1, 5000) g`, list)
	if rec := s.putInList(bob, list, w, ""); rec.Code != 409 {
		t.Errorf("the 5001st place: %d, want 409", rec.Code)
	}
	// Moving one that is in already is not a new place.
	s.exec(`DELETE FROM collection_works WHERE id = (SELECT min(id) FROM collection_works WHERE collection_id = $1)`, list)
	if rec := s.putInList(bob, list, w, ""); rec.Code != 204 {
		t.Errorf("a place when there is room: %d, want 204", rec.Code)
	}
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, label) VALUES ($1, NULL, FALSE, 'mais uma')`, list)
	if rec := s.putInList(bob, list, w, `{"position": 3}`); rec.Code != 204 {
		t.Errorf("moving a work of a full list: %d, want 204", rec.Code)
	}
}

func TestPersonalCollections_TheCoverIsOfTheFirstWorkThatIsThere(t *testing.T) {
	s := newCatalogStack(t)
	out := s.addWork("Na lixeira", "X", "o.epub", "epub")
	in := s.addWork("No acervo", "X", "i.epub", "epub")
	s.exec(`UPDATE editions SET cover_url = '/covers/out.jpg' WHERE work_id = $1`, out)
	s.exec(`UPDATE editions SET cover_url = '/covers/in.jpg' WHERE work_id = $1`, in)
	list := s.makeList(ana, "Capas")
	s.putInList(ana, list, out, "")
	s.putInList(ana, list, in, "")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, out)
	d, _ := s.collection(ana, list)
	if d.Collection.CoverURL != "/covers/in.jpg" {
		t.Errorf("cover = %q, want the one of the work that is there", d.Collection.CoverURL)
	}
}

func TestPersonalCollections_PlacesWithNoNumberComeByTitleWhetherTheWorkIsThereOrNot(t *testing.T) {
	s := newCatalogStack(t)
	zebra := s.addWork("Zebra", "X", "z.epub", "epub")
	list := s.makeList(ana, "Sem número")
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, list, zebra)
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, label) VALUES ($1, NULL, FALSE, 'Abelha')`, list)
	if got := strings.Join(titlesOf(s.entries(ana, list)), ","); got != "Abelha,Zebra" {
		t.Errorf("order = %s, want by title (the label of a work that is gone included)", got)
	}
}
