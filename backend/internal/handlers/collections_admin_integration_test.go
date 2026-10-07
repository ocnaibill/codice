package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func (s *catalogStack) createCollection(a actor, name string) (int64, *httptest.ResponseRecorder) {
	s.t.Helper()
	rec := s.do(a, "POST", "/collections", fmt.Sprintf(`{"name":%q}`, name))
	var out struct{ ID int64 }
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.ID, rec
}

func (s *catalogStack) addToCollection(a actor, collection int64, work int, body string) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.do(a, "PUT", fmt.Sprintf("/collections/%d/works/%d", collection, work), body)
}

func (s *catalogStack) memberIDs(collection int64) []int {
	s.t.Helper()
	d, code := s.collection(admin, collection)
	if code != 200 {
		s.t.Fatalf("collection %d: %d", collection, code)
	}
	var ids []int
	for _, w := range d.Works {
		ids = append(ids, w.ID)
	}
	return ids
}

func sameOrder(got []int, want ...int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func (s *catalogStack) auditCount(action string) string {
	return s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = $1`, action)
}

func TestCollectionsAdmin_CreateMakesAnEmptyOneAndRefusesANameThatLeadsToAnother(t *testing.T) {
	s := newCatalogStack(t)
	id, rec := s.createCollection(admin, "  Marvel   Cinemático ")
	if rec.Code != 201 || id == 0 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT name || '|' || origin || '|' || kind FROM collections WHERE id = $1`, id); n != "Marvel Cinemático|manual|official" {
		t.Errorf("collection = %s", n)
	}
	// A collection made by hand is shown even with no work in it.
	if l := s.collections(ana, ""); l.Total != 1 || l.Data[0].Name != "Marvel Cinemático" || l.Data[0].WorkCount != 0 {
		t.Errorf("list = %+v", l)
	}
	// The same name written another way, or the name of one the metadata made, leads to the one that exists.
	if other, rec := s.createCollection(admin, "marvel cinematico"); rec.Code != 409 || !strings.Contains(rec.Body.String(), fmt.Sprintf(`"collectionId":%d`, id)) {
		t.Errorf("same name: %d %s (%d)", rec.Code, rec.Body.String(), other)
	}
	w := s.addWork("Vol", "X", "v.epub", "epub")
	s.exec(`UPDATE works SET series = 'Da Metadado' WHERE id = $1`, w)
	if _, rec := s.createCollection(admin, "da metadado"); rec.Code != 409 {
		t.Errorf("a name the metadata already made: %d", rec.Code)
	}
	for _, bad := range []string{`{"name":""}`, `{"name":"   "}`, `{}`, `nope`, fmt.Sprintf(`{"name":%q}`, strings.Repeat("x", 513))} {
		if rec := s.do(admin, "POST", "/collections", bad); rec.Code != 400 {
			t.Errorf("POST %.30s: %d, want 400", bad, rec.Code)
		}
	}
	// The longest name is 512 characters.
	if _, rec := s.createCollection(admin, strings.Repeat("y", 512)); rec.Code != 201 {
		t.Errorf("a name of 512 characters: %d, want 201", rec.Code)
	}
	if s.auditCount("collection.create") != "2" {
		t.Errorf("audit entries = %s, want 2", s.auditCount("collection.create"))
	}
}

func TestCollectionsAdmin_AddingAWorkWritesAndLocksItsSeries(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	b := s.addWork("B", "X", "b.epub", "epub")
	c := s.addWork("C", "X", "c.epub", "epub")
	col, _ := s.createCollection(admin, "Franquia")

	// With no position, a work goes to the end.
	if rec := s.addToCollection(admin, col, a, ""); rec.Code != 204 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	s.addToCollection(admin, col, b, "")
	s.addToCollection(admin, col, c, `{"position": 10}`)
	if got := s.memberIDs(col); !sameOrder(got, a, b, c) {
		t.Errorf("order = %v, want %v", got, []int{a, b, c})
	}
	if v := s.scalar(`SELECT series || '|' || series_index || '|' || series_lock FROM works WHERE id = $1`, b); v != "Franquia|2|true" {
		t.Errorf("b = %s, want the series written, numbered 2 and locked", v)
	}
	if v := s.scalar(`SELECT source FROM work_field_sources WHERE work_id = $1 AND field = 'series'`, b); v != "manual" {
		t.Errorf("source = %q, want manual", v)
	}

	// Adding again, with a number, reorders; with none, nothing moves.
	s.addToCollection(admin, col, a, `{"position": 20}`)
	if got := s.memberIDs(col); !sameOrder(got, b, c, a) {
		t.Errorf("order after renumbering = %v", got)
	}
	s.addToCollection(admin, col, a, "")
	if v := s.scalar(`SELECT series_index FROM works WHERE id = $1`, a); v != "20" {
		t.Errorf("a with no position went to %s, want it to stay at 20", v)
	}

	// Saying where a work belongs locks its series, even when the series was right already.
	d := s.addWork("D", "X", "d.epub", "epub")
	s.exec(`UPDATE works SET series = 'Franquia', series_index = 30 WHERE id = $1`, d)
	if v := s.scalar(`SELECT series_lock FROM works WHERE id = $1`, d); v != "false" {
		t.Fatalf("setup: the series of d is locked (%s)", v)
	}
	s.addToCollection(admin, col, d, `{"position": 30}`)
	if v := s.scalar(`SELECT series_lock FROM works WHERE id = $1`, d); v != "true" {
		t.Errorf("series_lock of d = %s, want true", v)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/works/%d", col, d), "")

	// A work from another official collection moves: it is in one at most.
	other, _ := s.createCollection(admin, "Outra")
	s.addToCollection(admin, other, a, "")
	if got := s.memberIDs(col); !sameOrder(got, b, c) {
		t.Errorf("the work stayed in the first collection: %v", got)
	}
	if got := s.memberIDs(other); !sameOrder(got, a) {
		t.Errorf("the second collection = %v", got)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE work_id = $1 AND official`, a); n != "1" {
		t.Errorf("a is in %s official collections", n)
	}
	if v := s.scalar(`SELECT details->>'work' || '|' || (details->>'was') || '|' || (details->>'position') FROM audit_log WHERE action = 'collection.add_work' AND (details->>'work')::int = $1 ORDER BY id DESC LIMIT 1`, a); v != fmt.Sprintf("%d|Franquia|1", a) {
		t.Errorf("audit details of the move of a = %q", v)
	}
	if s.auditCount("collection.add_work") != "7" {
		t.Errorf("audit entries = %s, want 7", s.auditCount("collection.add_work"))
	}
}

func TestCollectionsAdmin_AddingIsRefusedWhereItMakesNoSense(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	gone := s.addWork("Gone", "X", "g.epub", "epub")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)
	col, _ := s.createCollection(admin, "Franquia")

	if rec := s.addToCollection(admin, col, gone, ""); rec.Code != 409 {
		t.Errorf("a work in the trash: %d, want 409", rec.Code)
	}
	if rec := s.addToCollection(admin, col, 99999, ""); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if rec := s.addToCollection(admin, 99999, a, ""); rec.Code != 404 {
		t.Errorf("unknown collection: %d, want 404", rec.Code)
	}
	if rec := s.addToCollection(admin, col, a, `{"position": -1}`); rec.Code != 400 {
		t.Errorf("negative position: %d, want 400", rec.Code)
	}
	if rec := s.addToCollection(admin, col, a, `{"position":`); rec.Code != 400 {
		t.Errorf("broken body: %d, want 400", rec.Code)
	}
	for _, target := range []string{"/collections/abc/works/1", "/collections/1/works/abc", "/collections/0/works/1", "/collections/1/works/99999999999"} {
		if rec := s.do(admin, "PUT", target, ""); rec.Code != 404 {
			t.Errorf("PUT %s: %d, want 404", target, rec.Code)
		}
	}
	// A personal collection is not managed here.
	var personal int64
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Minha', 'manual') RETURNING id`, idAna).Scan(&personal)
	if rec := s.addToCollection(admin, personal, a, ""); rec.Code != 404 {
		t.Errorf("a personal collection: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", personal), `{"name":"x"}`); rec.Code != 404 {
		t.Errorf("renaming a personal collection: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", personal), ""); rec.Code != 404 {
		t.Errorf("retiring a personal collection: %d, want 404", rec.Code)
	}
}

func TestCollectionsAdmin_RemovingClearsAndLocksTheSeriesSoTheAnalysisDoesNotPutItBack(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	b := s.addWork("B", "X", "b.epub", "epub")
	s.exec(`UPDATE works SET series = 'Duna', series_index = 1 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = 'Duna', series_index = 2 WHERE id = $1`, b)
	col := s.collectionID("Duna")

	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/works/%d", col, a), ""); rec.Code != 204 {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.memberIDs(col); !sameOrder(got, b) {
		t.Errorf("members = %v, want only b", got)
	}
	if v := s.scalar(`SELECT COALESCE(series, '<null>') || '|' || series_index || '|' || series_lock FROM works WHERE id = $1`, a); v != "|0|true" {
		t.Errorf("a = %q, want an empty, numberless, locked series", v)
	}
	// Not a member any more: a second removal and a work that never was.
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/works/%d", col, a), ""); rec.Code != 404 {
		t.Errorf("removing twice: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/99999/works/%d", b), ""); rec.Code != 404 {
		t.Errorf("unknown collection: %d, want 404", rec.Code)
	}
	if v := s.scalar(`SELECT (details->>'work') || '|' || (details->>'series') FROM audit_log WHERE action = 'collection.remove_work'`); v != fmt.Sprintf("%d|Duna", a) {
		t.Errorf("audit details of the removal = %q", v)
	}
	if s.auditCount("collection.remove_work") != "1" {
		t.Errorf("audit entries = %s, want 1", s.auditCount("collection.remove_work"))
	}
}

func TestCollectionsAdmin_RenamingKeepsTheOldTextLeadingToTheCollection(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	b := s.addWork("B", "X", "b.epub", "epub")
	s.exec(`UPDATE works SET series = 'Marvel Civil War', series_index = 1 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = 'Marvel Civil War', series_index = 2 WHERE id = $1`, b)
	col := s.collectionID("Marvel Civil War")

	if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", col), `{"name":" Guerra  Civil "}`); rec.Code != 200 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT name FROM collections WHERE id = $1`, col); n != "Guerra Civil" {
		t.Errorf("name = %q", n)
	}
	for _, w := range []int{a, b} {
		if v := s.scalar(`SELECT series || '|' || series_lock FROM works WHERE id = $1`, w); v != "Guerra Civil|true" {
			t.Errorf("work %d = %s, want the new name, locked", w, v)
		}
	}
	if got := s.memberIDs(col); !sameOrder(got, a, b) {
		t.Errorf("members after the rename = %v", got)
	}
	// The old text and the new one both lead there; one collection only.
	c := s.addWork("C", "X", "c.epub", "epub")
	s.exec(`UPDATE works SET series = 'marvel civil war', series_index = 3 WHERE id = $1`, c)
	if got := s.memberIDs(col); !sameOrder(got, a, b, c) {
		t.Errorf("a work with the old text did not join: %v", got)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM collections`); n != "1" {
		t.Errorf("collections = %s, want 1", n)
	}
	// An empty collection that was renamed is not hidden.
	for _, w := range []int{a, b, c} {
		s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/works/%d", col, w), "")
	}
	if l := s.collections(ana, ""); l.Total != 1 || l.Data[0].Name != "Guerra Civil" {
		t.Errorf("list = %+v", l)
	}
	if s.auditCount("collection.rename") != "1" {
		t.Errorf("audit entries = %s, want 1", s.auditCount("collection.rename"))
	}
}

func TestCollectionsAdmin_RenamingIsRefusedWhenTheNameLeadsToAnotherOrTheCollectionIsRetired(t *testing.T) {
	s := newCatalogStack(t)
	one, _ := s.createCollection(admin, "Um")
	two, _ := s.createCollection(admin, "Dois")
	if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", one), `{"name":"DOIS"}`); rec.Code != 409 || !strings.Contains(rec.Body.String(), fmt.Sprintf(`"collectionId":%d`, two)) {
		t.Errorf("a name of another: %d %s", rec.Code, rec.Body.String())
	}
	// Writing its own name another way is fine.
	if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", one), `{"name":"um"}`); rec.Code != 200 {
		t.Errorf("own name: %d", rec.Code)
	}
	for _, bad := range []string{`{"name":""}`, `nope`} {
		if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", one), bad); rec.Code != 400 {
			t.Errorf("PATCH %s: %d, want 400", bad, rec.Code)
		}
	}
	if rec := s.do(admin, "PATCH", "/collections/99999", `{"name":"x"}`); rec.Code != 404 {
		t.Errorf("unknown: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "PATCH", "/collections/abc", `{"name":"x"}`); rec.Code != 404 {
		t.Errorf("bad id: %d, want 404", rec.Code)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", one), "")
	if rec := s.do(admin, "PATCH", fmt.Sprintf("/collections/%d", one), `{"name":"Novo"}`); rec.Code != 409 {
		t.Errorf("a retired one: %d, want 409", rec.Code)
	}
}

func TestCollectionsAdmin_OrderNumbersTheWorksInTheListGiven(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.createCollection(admin, "Saga")
	var ids []int
	for i := 0; i < 3; i++ {
		w := s.addWork(fmt.Sprintf("Vol %d", i), "X", fmt.Sprintf("v%d.epub", i), "epub")
		s.addToCollection(admin, col, w, "")
		ids = append(ids, w)
	}
	gone := s.addWork("Fora", "X", "f.epub", "epub")
	s.addToCollection(admin, col, gone, "")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)

	order := fmt.Sprintf(`{"workIds":[%d,%d,%d]}`, ids[2], ids[0], ids[1])
	if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), order); rec.Code != 204 {
		t.Fatalf("order: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.memberIDs(col); !sameOrder(got, ids[2], ids[0], ids[1]) {
		t.Errorf("order = %v", got)
	}
	if v := s.scalar(`SELECT series_index FROM works WHERE id = $1`, ids[1]); v != "3" {
		t.Errorf("last number = %s, want 3", v)
	}
	// The list must be exactly the works of the collection that are not in the trash.
	for name, body := range map[string]string{
		"missing":  fmt.Sprintf(`{"workIds":[%d,%d]}`, ids[0], ids[1]),
		"repeated": fmt.Sprintf(`{"workIds":[%d,%d,%d,%d]}`, ids[0], ids[0], ids[1], ids[2]),
		"foreign":  fmt.Sprintf(`{"workIds":[%d,%d,%d,99999]}`, ids[0], ids[1], ids[2]),
		"swapped":  fmt.Sprintf(`{"workIds":[%d,%d,99999]}`, ids[0], ids[1]),
		"in trash": fmt.Sprintf(`{"workIds":[%d,%d,%d,%d]}`, ids[0], ids[1], ids[2], gone),
		"empty":    `{"workIds":[]}`,
		"not json": `nope`,
	} {
		if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), body); rec.Code != 400 {
			t.Errorf("order %s: %d, want 400", name, rec.Code)
		}
	}
	if rec := s.do(admin, "PUT", "/collections/99999/order", order); rec.Code != 404 {
		t.Errorf("unknown collection: %d, want 404", rec.Code)
	}
	if got := s.memberIDs(col); !sameOrder(got, ids[2], ids[0], ids[1]) {
		t.Errorf("a refused order changed the collection: %v", got)
	}
	s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", col), "")
	if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), order); rec.Code != 409 {
		t.Errorf("a retired one: %d, want 409", rec.Code)
	}
}

func TestCollectionsAdmin_RetiringTouchesNoWorkAndRestoringBringsThemBack(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "X", "a.epub", "epub")
	b := s.addWork("B", "X", "b.epub", "epub")
	s.exec(`UPDATE works SET series = 'Saga', series_index = 1 WHERE id = $1`, a)
	s.exec(`UPDATE works SET series = 'Saga', series_index = 2 WHERE id = $1`, b)
	col := s.collectionID("Saga")

	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", col), ""); rec.Code != 204 {
		t.Fatalf("retire: %d", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d", col), ""); rec.Code != 204 {
		t.Errorf("retiring twice: %d, want 204", rec.Code)
	}
	if v := s.scalar(`SELECT series || '|' || series_index FROM works WHERE id = $1`, a); v != "Saga|1" {
		t.Errorf("a = %s: retiring touched the work", v)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE collection_id = $1`, col); n != "0" {
		t.Errorf("a retired collection keeps %s memberships", n)
	}
	if s.collections(ana, "").Total != 0 {
		t.Error("a retired collection is listed")
	}
	living, _ := s.createCollection(admin, "Viva")
	if l := s.collections(admin, "?retired=true"); l.Total != 1 || l.Data[0].ID != col {
		t.Errorf("the retired list has the live ones too: %+v (live %d)", l, living)
	}
	if _, code := s.collection(ana, col); code != 404 {
		t.Errorf("a reader opening a retired collection: %d, want 404", code)
	}
	// The staff lists the retired ones, and opens them; a reader asking for them gets the ordinary list.
	if l := s.collections(admin, "?retired=true"); l.Total != 1 || l.Data[0].ID != col || !l.Data[0].Retired {
		t.Errorf("retired list = %+v", l)
	}
	if l := s.collections(ana, "?retired=true"); l.Total != 1 || l.Data[0].ID != living || l.Data[0].Retired {
		t.Errorf("a reader asking for the retired must get the ordinary list: %+v", l)
	}
	if d, code := s.collection(admin, col); code != 200 || !d.Collection.Retired {
		t.Errorf("staff detail = %d %+v", code, d.Collection)
	}
	// While retired it takes no work, and is not made again.
	c := s.addWork("C", "X", "c.epub", "epub")
	s.exec(`UPDATE works SET series = 'saga', series_index = 3 WHERE id = $1`, c)
	if n := s.scalar(`SELECT COUNT(*) FROM collection_works WHERE work_id = $1`, c); n != "0" {
		t.Error("a work joined a retired collection")
	}
	if rec := s.addToCollection(admin, col, a, ""); rec.Code != 409 {
		t.Errorf("adding to a retired one: %d, want 409", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/collections/%d/works/%d", col, a), ""); rec.Code != 409 {
		t.Errorf("removing from a retired one: %d, want 409", rec.Code)
	}

	if rec := s.do(admin, "POST", fmt.Sprintf("/collections/%d/restore", col), ""); rec.Code != 204 {
		t.Fatalf("restore: %d", rec.Code)
	}
	if got := s.memberIDs(col); !sameOrder(got, a, b, c) {
		t.Errorf("after restoring = %v, want all three, in order", got)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/collections/%d/restore", col), ""); rec.Code != 204 {
		t.Errorf("restoring one that is not retired: %d, want 204", rec.Code)
	}
	if rec := s.do(admin, "POST", "/collections/99999/restore", ""); rec.Code != 404 {
		t.Errorf("unknown: %d, want 404", rec.Code)
	}
	if s.auditCount("collection.retire") != "1" || s.auditCount("collection.restore") != "1" {
		t.Errorf("audit entries: retire %s, restore %s, want 1 each", s.auditCount("collection.retire"), s.auditCount("collection.restore"))
	}
}
