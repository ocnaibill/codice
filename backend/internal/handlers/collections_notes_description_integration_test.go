package handlers

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestCollectionPage_CountsTheNotesOfTheCallerOnEachWorkAndOnTheWhole(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Duna", "volume", "volume", "volume")
	note := func(a actor, work int, kind string) {
		t.Helper()
		body := fmt.Sprintf(`{"kind":%q,"quote":"trecho"}`, kind)
		if kind == "bookmark" {
			body = fmt.Sprintf(`{"kind":"bookmark","fileId":%d,"locator":{"type":"image","index":3}}`, s.primaryFile(work))
		}
		if rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), body); rec.Code != 201 && rec.Code != 200 {
			t.Fatalf("%s: %d %s", kind, rec.Code, rec.Body.String())
		}
	}
	note(ana, ids[0], "highlight")
	note(ana, ids[0], "highlight")
	note(ana, ids[0], "note")
	note(ana, ids[0], "bookmark") // a bookmark is a place, not a note
	// Not counted either: a note of her own with no passage of the book, and a bookmark that carries a text.
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", ids[0]), `{"kind":"note","body":"só uma ideia minha"}`); rec.Code != 201 && rec.Code != 200 {
		t.Fatalf("free note: %d %s", rec.Code, rec.Body.String())
	}
	s.exec(`UPDATE notes SET quote = 'um marcador com texto' WHERE user_id = $1 AND kind = 'bookmark'`, idAna)
	note(ana, ids[1], "note")
	note(bob, ids[2], "highlight")

	d, code := s.collection(ana, col)
	if code != 200 {
		t.Fatalf("%d", code)
	}
	got := map[int]int{}
	for _, w := range d.Works {
		got[w.ID] = w.Notes
	}
	if got[ids[0]] != 3 || got[ids[1]] != 1 || got[ids[2]] != 0 {
		t.Errorf("notes by work: %v", got)
	}
	if d.Summary.Notes != 4 {
		t.Errorf("the whole: %d", d.Summary.Notes)
	}
	if e, _ := s.collection(bob, col); e.Summary.Notes != 1 {
		t.Errorf("bob counts his own: %d", e.Summary.Notes)
	}
}

func TestNotes_CanBeAskedForByTheCollectionTheirWorksAreIn(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Duna", "volume", "volume")
	outside := s.addWork("Solaris", "Stanisław Lem", "c.epub", "epub")
	note := func(a actor, work int, quote string) {
		t.Helper()
		if rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), `{"kind":"highlight","quote":"`+quote+`"}`); rec.Code != 201 && rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	note(ana, ids[0], "do um")
	note(ana, ids[1], "do dois")
	note(ana, outside, "de fora")
	note(bob, ids[0], "do bob")
	quotes := func(a actor, query string) string {
		var out []string
		for _, n := range s.notesWithChapters(a, query).Data {
			out = append(out, n.Quote)
		}
		sort.Strings(out)
		return strings.Join(out, "|")
	}
	q := fmt.Sprintf("collectionId=%d&limit=50", col)
	if got := quotes(ana, q); got != "do dois|do um" {
		t.Errorf("ana: %s", got)
	}
	if got := quotes(bob, q); got != "do bob" {
		t.Errorf("bob: %s", got)
	}

	// A personal list of bob is his: the notes of ana on a work of it do not come from it.
	list := s.makeList(bob, "Minha")
	if rec := s.putInList(bob, list, ids[0], `{}`); rec.Code >= 300 {
		t.Fatalf("%d", rec.Code)
	}
	if got := quotes(ana, fmt.Sprintf("collectionId=%d&limit=50", list)); got != "" {
		t.Errorf("a list of another is not a filter for ana: %s", got)
	}
	if got := quotes(bob, fmt.Sprintf("collectionId=%d&limit=50", list)); got != "do bob" {
		t.Errorf("his own list: %s", got)
	}
	for _, bad := range []string{"collectionId=abc", "collectionId=0", "collectionId=-3"} {
		if rec := s.do(ana, "GET", "/notes?"+bad, ""); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}
}

func TestCollectionDescription_IsWrittenByTheStaffForAnOfficialOneAndByTheOwnerForAList(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Duna", "volume", "volume")
	patch := func(a actor, path, body string) int {
		t.Helper()
		return s.do(a, "PATCH", path, body).Code
	}
	path := fmt.Sprintf("/collections/%d", col)
	if code := patch(admin, path, `{"description":"  A saga\r\nde Arrakis.  "}`); code != 200 {
		t.Fatalf("describe: %d", code)
	}
	if d, _ := s.collection(ana, col); d.Collection.Description != "A saga\nde Arrakis." || d.Collection.Name != "Duna" {
		t.Errorf("description: %q name: %q", d.Collection.Description, d.Collection.Name)
	}
	if s.auditCount("collection.describe") != "1" || s.auditCount("collection.rename") != "0" {
		t.Errorf("audit: describe %s rename %s", s.auditCount("collection.describe"), s.auditCount("collection.rename"))
	}
	// Name and description together: both change.
	if code := patch(admin, path, `{"name":"Duna Saga","description":"Seis livros."}`); code != 200 {
		t.Fatalf("both: %d", code)
	}
	d, _ := s.collection(ana, col)
	if d.Collection.Name != "Duna Saga" || d.Collection.Description != "Seis livros." {
		t.Errorf("both: %q %q", d.Collection.Name, d.Collection.Description)
	}
	// Empty clears it, and the name stays.
	if code := patch(admin, path, `{"description":"   "}`); code != 200 {
		t.Fatalf("clear: %d", code)
	}
	if d, _ := s.collection(ana, col); d.Collection.Description != "" || d.Collection.Name != "Duna Saga" {
		t.Errorf("cleared: %q %q", d.Collection.Description, d.Collection.Name)
	}
	if n := s.scalar(`SELECT description IS NULL FROM collections WHERE id = $1`, col); n != "true" {
		t.Errorf("empty is NULL, not an empty text: %s", n)
	}
	// What is refused.
	for _, bad := range []string{`{}`, `{"description":"` + strings.Repeat("a", 2001) + `"}`, `{"name":"  "}`} {
		if code := patch(admin, path, bad); code != 400 {
			t.Errorf("%.30s: %d, want 400", bad, code)
		}
	}
	// Retired: restore first.
	s.exec(`UPDATE collections SET retired_at = now() WHERE id = $1`, col)
	if code := patch(admin, path, `{"description":"x"}`); code != 409 {
		t.Errorf("a retired one: %d, want 409", code)
	}

	// A list of the caller.
	list := s.makeList(ana, "Para o fim de semana")
	lp := fmt.Sprintf("/my/collections/%d", list)
	if code := patch(ana, lp, `{"description":"Leituras curtas."}`); code != 200 {
		t.Fatalf("list: %d", code)
	}
	if d, _ := s.collection(ana, list); d.Collection.Description != "Leituras curtas." {
		t.Errorf("list description: %q", d.Collection.Description)
	}
	if code := patch(bob, lp, `{"description":"do bob"}`); code != 404 {
		t.Errorf("another's list: %d, want 404", code)
	}
	// The list the Códice keeps is not described.
	s.readLater(ana, "PUT", ids[0])
	system := s.scalar(`SELECT id FROM collections WHERE system_key IS NOT NULL AND owner_id = $1`, idAna)
	if rec := s.do(ana, "PATCH", "/my/collections/"+system, `{"description":"minha"}`); rec.Code != 409 {
		t.Errorf("the list of the Códice: %d, want 409", rec.Code)
	}
}
