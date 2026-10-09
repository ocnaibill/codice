package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type catList struct {
	Data []Category `json:"data"`
}

func (s *catalogStack) categories() []Category {
	s.t.Helper()
	rec := s.do(ana, "GET", "/categories", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /categories: %d %s", rec.Code, rec.Body.String())
	}
	var out catList
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatal(err)
	}
	return out.Data
}

func (s *catalogStack) category(id int64) Category {
	s.t.Helper()
	for _, c := range s.categories() {
		if c.ID == id {
			return c
		}
	}
	s.t.Fatalf("no category %d", id)
	return Category{}
}

func parentJSON(parent *int64) string {
	if parent == nil {
		return "null"
	}
	return fmt.Sprint(*parent)
}

// makeCategory creates one and fails the test if the server refuses.
func (s *catalogStack) makeCategory(name string, parent *int64) int64 {
	s.t.Helper()
	rec := s.do(admin, "POST", "/admin/categories", fmt.Sprintf(`{"name":%q,"parentId":%s}`, name, parentJSON(parent)))
	if rec.Code != 201 {
		s.t.Fatalf("creating %q: %d %s", name, rec.Code, rec.Body.String())
	}
	var c Category
	json.Unmarshal(rec.Body.Bytes(), &c)
	return c.ID
}

func (s *catalogStack) putCategory(id int64, name string, parent *int64) int {
	return s.do(admin, "PUT", fmt.Sprintf("/admin/categories/%d", id), fmt.Sprintf(`{"name":%q,"parentId":%s}`, name, parentJSON(parent))).Code
}

func (s *catalogStack) setWorkCategories(work int, ids string) int {
	return s.do(admin, "PUT", fmt.Sprintf("/works/%d/categories", work), fmt.Sprintf(`{"ids":%s}`, ids)).Code
}

func ptr(n int64) *int64 { return &n }

func names(cs []Category) string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return strings.Join(out, "|")
}

func TestCategories_StartEmptyAndAreAListNotNull(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "GET", "/categories", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"data":[]}` {
		t.Errorf("%d %q", rec.Code, rec.Body.String())
	}
}

func TestCategories_AreMadeAtTheTopOrUnderAnotherAndListedByNameWithoutRegardToCaseOrAccents(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	s.makeCategory("Zebra", nil)
	s.makeCategory("Ação", nil)
	s.makeCategory("banda", nil)
	s.makeCategory("Édito", nil)
	s.makeCategory("abacaxi", nil)
	seinen := s.makeCategory("Seinen", &manga)
	s.makeCategory("Shounen", &manga)

	got := s.categories()
	if names(got) != "abacaxi|Ação|banda|Édito|Mangá|Seinen|Shounen|Zebra" {
		t.Errorf("order = %s", names(got))
	}
	byID := map[int64]Category{}
	for _, c := range got {
		byID[c.ID] = c
	}
	if c := byID[seinen]; c.ParentID == nil || *c.ParentID != manga {
		t.Errorf("Seinen = %+v", c)
	}
	if c := byID[manga]; c.ParentID != nil {
		t.Errorf("Mangá = %+v", c)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'category.create'`); got != "8" {
		t.Errorf("audit entries = %s", got)
	}
}

func TestCategories_ANameIsOneLineOfUpTo80CharactersWithSingleSpaces(t *testing.T) {
	s := newCatalogStack(t)
	id := s.makeCategory("  Ficção    científica \t ", nil)
	if got := s.category(id).Name; got != "Ficção científica" {
		t.Errorf("kept as %q", got)
	}
	s.makeCategory(strings.Repeat("é", 80), nil) // 80 characters, 160 bytes
	for name, want := range map[string]int{
		"":                      400,
		"   ":                   400,
		strings.Repeat("é", 81): 400,
		"Com\x00nulo":           400,
		"​":                     201, // not a control character: a name, if an odd one
	} {
		if code := s.do(admin, "POST", "/admin/categories", fmt.Sprintf(`{"name":%q}`, name)).Code; code != want {
			t.Errorf("%q: %d, want %d", name, code, want)
		}
	}
	// A line break is a space, like any other: the name is kept on one line.
	if got := s.category(s.makeCategory("Duas\nlinhas", nil)).Name; got != "Duas linhas" {
		t.Errorf("a line break kept as %q", got)
	}
	if code := s.do(admin, "POST", "/admin/categories", `not json`).Code; code != 400 {
		t.Errorf("not json: %d", code)
	}
}

func TestCategories_TwoWithTheSameNameCannotBeSideBySideButCanBeInDifferentPlaces(t *testing.T) {
	s := newCatalogStack(t)
	comics := s.makeCategory("Quadrinhos", nil)
	novels := s.makeCategory("Romances", nil)
	s.makeCategory("Terror", &comics)
	s.makeCategory("Terror", &novels)
	s.makeCategory("Terror", nil)
	for name, parent := range map[string]*int64{"terror": &comics, "TERROR": &novels, "Terror": nil, "terror ": nil} {
		if code := s.do(admin, "POST", "/admin/categories", fmt.Sprintf(`{"name":%q,"parentId":%s}`, name, parentJSON(parent))).Code; code != 409 {
			t.Errorf("%q under %v: %d, want 409", name, parentJSON(parent), code)
		}
	}
}

func TestCategories_GoAtMostThreeLevelsDeepAndAParentMustExist(t *testing.T) {
	s := newCatalogStack(t)
	a := s.makeCategory("A", nil)
	b := s.makeCategory("B", &a)
	c := s.makeCategory("C", &b)
	if code := s.do(admin, "POST", "/admin/categories", fmt.Sprintf(`{"name":"D","parentId":%d}`, c)).Code; code != 400 {
		t.Errorf("a fourth level: %d, want 400", code)
	}
	if code := s.do(admin, "POST", "/admin/categories", `{"name":"D","parentId":99999}`).Code; code != 404 {
		t.Errorf("an unknown parent: %d, want 404", code)
	}
	if n := len(s.categories()); n != 3 {
		t.Errorf("%d categories, want 3", n)
	}
}

func TestCategories_AreRenamedAndMovedButNeverIntoThemselvesNorPastTheLimit(t *testing.T) {
	s := newCatalogStack(t)
	a := s.makeCategory("A", nil)
	b := s.makeCategory("B", &a)
	c := s.makeCategory("C", &b)
	x := s.makeCategory("X", nil)
	y := s.makeCategory("Y", &x)

	if code := s.putCategory(a, "A renomeada", nil); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	if got := s.category(a).Name; got != "A renomeada" {
		t.Errorf("name = %q", got)
	}
	if code := s.putCategory(a, "A renomeada", nil); code != 200 {
		t.Errorf("the same name it already has: %d, want 200", code)
	}
	// Into itself or into what is under it.
	for name, parent := range map[string]int64{"itself": a, "its child": b, "its grandchild": c} {
		if code := s.putCategory(a, "A renomeada", &parent); code != 400 {
			t.Errorf("moving into %s: %d, want 400", name, code)
		}
	}
	// A subtree of two levels under a category of two levels is five: too deep. Under one of one level it is three: fine.
	if code := s.putCategory(b, "B", &y); code != 400 {
		t.Errorf("past the limit: %d, want 400", code)
	}
	if code := s.putCategory(b, "B", &x); code != 200 {
		t.Errorf("within the limit: %d, want 200", code)
	}
	if got := s.category(c); got.ParentID == nil || *got.ParentID != b {
		t.Errorf("C is still under B: %+v", got)
	}
	// Back to the top.
	if code := s.putCategory(b, "B", nil); code != 200 {
		t.Errorf("to the top: %d", code)
	}
	if got := s.category(b); got.ParentID != nil {
		t.Errorf("B = %+v", got)
	}
	// Names that are taken where it goes, or already there.
	if code := s.putCategory(x, "b", nil); code != 409 {
		t.Errorf("a name taken at the top: %d, want 409", code)
	}
	if code := s.putCategory(y, "Y", nil); code != 200 {
		t.Errorf("Y to the top: %d", code)
	}
	if code := s.putCategory(c, "Y", nil); code != 409 {
		t.Errorf("C to the top as Y: %d, want 409", code)
	}
	// Unknown ones.
	for _, id := range []string{"99999", "0", "x"} {
		if code := s.do(admin, "PUT", "/admin/categories/"+id, `{"name":"Z"}`).Code; code != 404 {
			t.Errorf("PUT %s: %d, want 404", id, code)
		}
	}
	if code := s.putCategory(a, "Z", ptr(99999)); code != 404 {
		t.Errorf("an unknown parent: %d, want 404", code)
	}
	if code := s.putCategory(a, "", nil); code != 400 {
		t.Errorf("no name: %d, want 400", code)
	}
	if got := s.scalar(`SELECT details->>'was' FROM audit_log WHERE action = 'category.update' ORDER BY id LIMIT 1`); got != "A" {
		t.Errorf("audit remembers the old name as %q", got)
	}
}

func TestCategories_OneWithSubcategoriesIsNotDeletedAndOneWithWorksOnlyLetsThemGo(t *testing.T) {
	s := newCatalogStack(t)
	parent := s.makeCategory("Mangá", nil)
	child := s.makeCategory("Seinen", &parent)
	work := s.addWork("Berserk", "Kentaro Miura", "a.cbz", "cbz")
	s.setWorkCategories(work, fmt.Sprintf("[%d]", child))

	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/categories/%d", parent), "").Code; code != 409 {
		t.Errorf("with a subcategory: %d, want 409", code)
	}
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/categories/%d", child), "").Code; code != 204 {
		t.Fatalf("a leaf with a work: %d, want 204", code)
	}
	if got := s.scalar(`SELECT count(*) FROM works WHERE id = $1`, work); got != "1" {
		t.Errorf("the work was deleted with the category")
	}
	if got := s.scalar(`SELECT count(*) FROM work_categories`); got != "0" {
		t.Errorf("%s links left", got)
	}
	if got := s.scalar(`SELECT (details->>'works') || '|' || (details->>'name') FROM audit_log WHERE action = 'category.delete'`); got != "1|Seinen" {
		t.Errorf("audit = %q", got)
	}
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/categories/%d", parent), "").Code; code != 204 {
		t.Errorf("now a leaf: %d, want 204", code)
	}
	for _, id := range []string{"99999", "0", "x"} {
		if code := s.do(admin, "DELETE", "/admin/categories/"+id, "").Code; code != 404 {
			t.Errorf("DELETE %s: %d, want 404", id, code)
		}
	}
}

func TestCategories_AWorkIsInTheOnesItWasPutInAndAlsoCountsForTheOnesAbove(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	seinen := s.makeCategory("Seinen", &manga)
	shounen := s.makeCategory("Shounen", &manga)
	scifi := s.makeCategory("Ficção científica", nil)
	berserk := s.addWork("Berserk", "Kentaro Miura", "a.cbz", "cbz")
	naruto := s.addWork("Naruto", "Masashi Kishimoto", "b.cbz", "cbz")
	duna := s.addWork("Duna", "Frank Herbert", "c.epub", "epub")

	s.setWorkCategories(berserk, fmt.Sprintf("[%d,%d]", seinen, manga)) // in the parent too: counted once there
	s.setWorkCategories(naruto, fmt.Sprintf("[%d]", shounen))
	s.setWorkCategories(duna, fmt.Sprintf("[%d,%d,%d]", scifi, scifi, scifi)) // the same one three times is one

	counts := map[string][2]int{}
	for _, c := range s.categories() {
		counts[c.Name] = [2]int{c.Works, c.Own}
	}
	for name, want := range map[string][2]int{"Mangá": {2, 1}, "Seinen": {1, 1}, "Shounen": {1, 1}, "Ficção científica": {1, 1}} {
		if counts[name] != want {
			t.Errorf("%s = %v (works, own), want %v", name, counts[name], want)
		}
	}
	// A retired work is in none of the counts.
	s.retire(naruto)
	counts = map[string][2]int{}
	for _, c := range s.categories() {
		counts[c.Name] = [2]int{c.Works, c.Own}
	}
	if counts["Shounen"] != [2]int{0, 0} || counts["Mangá"] != [2]int{1, 1} {
		t.Errorf("after retiring: Shounen %v, Mangá %v", counts["Shounen"], counts["Mangá"])
	}
}

func TestCategories_TheDetailOfAWorkListsItsCategoriesWithTheirPath(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	seinen := s.makeCategory("Seinen", &manga)
	scifi := s.makeCategory("Ficção científica", nil)
	work := s.addWork("Berserk", "Kentaro Miura", "a.cbz", "cbz")
	s.addWork("Outro", "Alguém", "b.cbz", "cbz")

	w, _ := s.detail(ana, work)
	if w.Metadata == nil || w.Metadata.Categories == nil || len(w.Metadata.Categories) != 0 {
		t.Fatalf("a work in none: %+v", w.Metadata)
	}
	if code := s.setWorkCategories(work, fmt.Sprintf("[%d,%d]", seinen, scifi)); code != 200 {
		t.Fatalf("%d", code)
	}
	w, _ = s.detail(ana, work)
	var got []string
	for _, c := range w.Metadata.Categories {
		got = append(got, fmt.Sprintf("%s=%s", c.Name, c.Path))
	}
	if want := "Ficção científica=Ficção científica|Seinen=Mangá › Seinen"; strings.Join(got, "|") != want {
		t.Errorf("categories = %v, want %s", got, want)
	}
}

func TestCategories_SettingThoseOfAWorkReplacesWhatItHadAndRefusesWhatDoesNotExist(t *testing.T) {
	s := newCatalogStack(t)
	a := s.makeCategory("A", nil)
	b := s.makeCategory("B", nil)
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	other := s.addWork("Outro", "Alguém", "b.epub", "epub")
	s.setWorkCategories(other, fmt.Sprintf("[%d]", a))

	set := func() string {
		return s.scalar(`SELECT coalesce(string_agg(category_id::text, ',' ORDER BY category_id), '') FROM work_categories WHERE work_id = $1`, work)
	}
	s.setWorkCategories(work, fmt.Sprintf("[%d]", a))
	s.setWorkCategories(work, fmt.Sprintf("[%d]", b))
	if got := set(); got != fmt.Sprint(b) {
		t.Errorf("replaced by B: %q", got)
	}
	// Keeping one it already had, and adding another, is what editing a work usually is.
	s.setWorkCategories(work, fmt.Sprintf("[%d]", b))
	if code := s.setWorkCategories(work, fmt.Sprintf("[%d,%d]", b, a)); code != 200 || set() != fmt.Sprintf("%d,%d", a, b) {
		t.Errorf("keeping B and adding A: %d, %q", code, set())
	}
	s.setWorkCategories(work, fmt.Sprintf("[%d]", b))
	// A request with a category that does not exist changes nothing.
	if code := s.setWorkCategories(work, fmt.Sprintf("[%d,99999]", a)); code != 404 {
		t.Errorf("unknown category: %d, want 404", code)
	}
	if got := set(); got != fmt.Sprint(b) {
		t.Errorf("a refused request changed it to %q", got)
	}
	// Not a category (0, negative) is left out, as is a repeat.
	if code := s.setWorkCategories(work, fmt.Sprintf("[%d,%d,0,-3]", a, a)); code != 200 {
		t.Errorf("%d", code)
	}
	if got := set(); got != fmt.Sprint(a) {
		t.Errorf("after A twice: %q", got)
	}
	if code := s.setWorkCategories(work, "[]"); code != 200 || set() != "" {
		t.Errorf("empty: %d, %q", code, set())
	}
	// What other works have is theirs.
	if got := s.scalar(`SELECT count(*) FROM work_categories WHERE work_id = $1`, other); got != "1" {
		t.Errorf("the other work has %s categories", got)
	}
	if code := s.do(admin, "PUT", fmt.Sprintf("/works/%d/categories", work), `{}`).Code; code != 400 {
		t.Errorf("no ids: %d, want 400", code)
	}
	if code := s.do(admin, "PUT", fmt.Sprintf("/works/%d/categories", work), `{"ids":null}`).Code; code != 400 {
		t.Errorf("null ids: %d, want 400", code)
	}
	if code := s.do(admin, "PUT", fmt.Sprintf("/works/%d/categories", work), `nope`).Code; code != 400 {
		t.Errorf("not json: %d, want 400", code)
	}
	for _, id := range []string{"99999", "0", "x"} {
		if code := s.do(admin, "PUT", "/works/"+id+"/categories", `{"ids":[]}`).Code; code != 404 {
			t.Errorf("work %s: %d, want 404", id, code)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'work.categories'`); got == "0" {
		t.Errorf("no audit entry")
	}
}

func (s *catalogStack) inCategory(work int, ids ...int64) {
	s.t.Helper()
	list := "["
	for i, id := range ids {
		if i > 0 {
			list += ","
		}
		list += fmt.Sprint(id)
	}
	if code := s.setWorkCategories(work, list+"]"); code != 200 {
		s.t.Fatalf("putting %d in %s]: %d", work, list, code)
	}
}

func listedTitles(l workList) string {
	out := []string{}
	for _, w := range l.Data {
		out = append(out, w.Title)
	}
	return strings.Join(out, "|")
}

func TestWorksOfACategory_AreTheOnesInItAndUnderItAndNoOthers(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	seinen := s.makeCategory("Seinen", &manga)
	dark := s.makeCategory("Dark", &seinen)
	scifi := s.makeCategory("Ficção científica", nil)
	berserk := s.addWork("Berserk", "Kentaro Miura", "a.cbz", "cbz")
	vagabond := s.addWork("Vagabond", "Takehiko Inoue", "b.cbz", "cbz")
	naruto := s.addWork("Naruto", "Masashi Kishimoto", "c.cbz", "cbz")
	duna := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.addWork("Sem categoria", "Alguém", "e.epub", "epub")
	s.inCategory(berserk, dark)
	s.inCategory(vagabond, seinen)
	s.inCategory(naruto, manga)
	s.inCategory(duna, scifi, seinen) // in two places, listed once

	for query, want := range map[string]string{
		fmt.Sprintf("?category=%d&sort=title", manga):  "Berserk|Duna|Naruto|Vagabond",
		fmt.Sprintf("?category=%d&sort=title", seinen): "Berserk|Duna|Vagabond",
		fmt.Sprintf("?category=%d&sort=title", dark):   "Berserk",
		fmt.Sprintf("?category=%d&sort=title", scifi):  "Duna",
		"?category=99999": "",
		"?category=0":     "",
		"?category=-1":    "",
		"?category=x":     "",
		fmt.Sprintf("?category=%d&search=duna", manga):    "Duna",
		fmt.Sprintf("?category=%d&search=duna", scifi):    "Duna",
		fmt.Sprintf("?category=%d&search=berserk", scifi): "",
	} {
		if got := listedTitles(s.list(ana, query)); got != want {
			t.Errorf("%s: %q, want %q", query, got, want)
		}
	}
	if l := s.list(ana, fmt.Sprintf("?category=%d", manga)); l.Total != 4 {
		t.Errorf("total = %d, want 4 (a work in two places under it counts once)", l.Total)
	}
	// Pages: the filter is in the count and in the page.
	first := s.list(ana, fmt.Sprintf("?category=%d&sort=title&limit=2", manga))
	second := s.list(ana, fmt.Sprintf("?category=%d&sort=title&limit=2&page=2", manga))
	if listedTitles(first) != "Berserk|Duna" || listedTitles(second) != "Naruto|Vagabond" || first.Total != 4 {
		t.Errorf("pages: %q, %q (total %d)", listedTitles(first), listedTitles(second), first.Total)
	}
	// A retired work is not in it, and without the filter nothing changes.
	s.retire(naruto)
	if got := listedTitles(s.list(ana, fmt.Sprintf("?category=%d&sort=title", manga))); got != "Berserk|Duna|Vagabond" {
		t.Errorf("after retiring: %q", got)
	}
	if l := s.list(ana, ""); l.Total != 4 {
		t.Errorf("the whole library = %d, want 4", l.Total)
	}
}

func TestWorksOfACategory_AreEachByThemselvesEvenWhenTheGridPutsASeriesTogether(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	for i := 1; i <= 3; i++ {
		id := s.addWork(fmt.Sprintf("Vagabond %d", i), "Takehiko Inoue", fmt.Sprintf("%d.cbz", i), "cbz")
		s.exec(`UPDATE works SET series = 'Vagabond', series_index = $2, unit = 'volume', comic_kind = 'manga' WHERE id = $1`, id, i)
		s.inCategory(id, manga)
	}
	if l := s.list(ana, fmt.Sprintf("?category=%d&series=collapse&sort=title", manga)); l.Total != 3 {
		t.Errorf("%d items, want the 3 works", l.Total)
	}
}

func TestCategories_CoversAreAskedForAndAreOfTheNewestWorksWithOne(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	seinen := s.makeCategory("Seinen", &manga)
	empty := s.makeCategory("Vazia", nil)
	cover := func(work int, url string) {
		s.exec(`UPDATE editions SET cover_url = $1 WHERE work_id = $2 AND is_primary`, url, work)
	}
	var works []int
	for i := 1; i <= 5; i++ {
		id := s.addWork(fmt.Sprintf("Livro %d", i), "Alguém", fmt.Sprintf("%d.cbz", i), "cbz")
		cover(id, fmt.Sprintf("/covers/%d.jpg", i))
		works = append(works, id)
	}
	noCover := s.addWork("Sem capa", "Alguém", "x.cbz", "cbz")
	s.exec(`UPDATE editions SET cover_url = '' WHERE work_id = $1 AND is_primary`, noCover)
	s.inCategory(works[0], manga)
	s.inCategory(works[1], seinen)
	s.inCategory(works[2], seinen)
	s.inCategory(works[3], manga)
	s.inCategory(works[4], seinen)
	s.inCategory(noCover, seinen)
	gone := s.addWork("Retirado", "Alguém", "g.cbz", "cbz")
	cover(gone, "/covers/gone.jpg")
	s.inCategory(gone, seinen)
	s.retire(gone)

	get := func(query string) map[string]Category {
		rec := s.do(ana, "GET", "/categories"+query, "")
		var out catList
		json.Unmarshal(rec.Body.Bytes(), &out)
		by := map[string]Category{}
		for _, c := range out.Data {
			by[c.Name] = c
		}
		return by
	}
	// Not asked for: not sent.
	if c := get("")["Mangá"]; len(c.Covers) != 0 {
		t.Errorf("covers without asking: %v", c.Covers)
	}
	by := get("?covers=1")
	// The newest first (the last work put in), three at most, the ones under it too, none without a cover, none retired.
	if got := strings.Join(by["Mangá"].Covers, "|"); got != "/covers/5.jpg|/covers/4.jpg|/covers/3.jpg" {
		t.Errorf("Mangá = %s", got)
	}
	if got := strings.Join(by["Seinen"].Covers, "|"); got != "/covers/5.jpg|/covers/3.jpg|/covers/2.jpg" {
		t.Errorf("Seinen = %s", got)
	}
	if got := by["Vazia"].Covers; len(got) != 0 {
		t.Errorf("an empty category has covers: %v", got)
	}
	_ = empty
	if rec := s.do(ana, "GET", "/categories?covers=2", ""); strings.Contains(rec.Body.String(), "covers") {
		t.Errorf("covers=2 is not asking for them")
	}
}
