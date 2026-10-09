package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func (s *catalogStack) tag(work int, names ...string) {
	s.t.Helper()
	for _, name := range names {
		s.exec(`INSERT INTO tags (name) VALUES ($1) ON CONFLICT DO NOTHING`, name)
		s.exec(`INSERT INTO work_tags (work_id, tag_id) SELECT $1, id FROM tags WHERE name = $2 ON CONFLICT DO NOTHING`, work, name)
	}
}

func (s *catalogStack) addRule(category int64, term string) (int, CategoryRule) {
	s.t.Helper()
	rec := s.do(admin, "POST", fmt.Sprintf("/admin/categories/%d/rules", category), fmt.Sprintf(`{"term":%q}`, term))
	var rule CategoryRule
	json.Unmarshal(rec.Body.Bytes(), &rule)
	return rec.Code, rule
}

func (s *catalogStack) mustRule(category int64, terms ...string) {
	s.t.Helper()
	for _, term := range terms {
		if code, _ := s.addRule(category, term); code != 201 {
			s.t.Fatalf("rule %q: %d", term, code)
		}
	}
}

func (s *catalogStack) rulesPreview() RulesPreview {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/categories/rules/preview", "")
	if rec.Code != 200 {
		s.t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	var out RulesPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatal(err)
	}
	return out
}

func (s *catalogStack) applyRules(links int) (int, string) {
	rec := s.do(admin, "POST", "/admin/categories/rules/apply", fmt.Sprintf(`{"links":%d}`, links))
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func (s *catalogStack) linked(work int) string {
	return s.scalar(`SELECT coalesce(string_agg(category_id::text || ':' || source, ',' ORDER BY category_id), '') FROM work_categories WHERE work_id = $1`, work)
}

func TestCategoryRules_AreTermsOfACategoryKeptWithoutRegardToCaseAccentsOrSpaces(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	code, rule := s.addRule(scifi, "  Science   Fiction ")
	if code != 201 || rule.Term != "Science Fiction" || rule.CategoryID != scifi || rule.ID == 0 {
		t.Fatalf("added: %d %+v", code, rule)
	}
	for _, term := range []string{"science fiction", "SCIENCE FICTION", "science  fiction"} {
		if code, _ := s.addRule(scifi, term); code != 409 {
			t.Errorf("%q again: %d, want 409", term, code)
		}
	}
	s.mustRule(scifi, "ficção científica")
	if code, _ := s.addRule(scifi, "ficcao cientifica"); code != 409 {
		t.Errorf("without the accents: %d, want 409", code)
	}
	// The same term can be in another category.
	fantasy := s.makeCategory("Fantasia", nil)
	if code, _ := s.addRule(fantasy, "science fiction"); code != 201 {
		t.Errorf("the same term in another category: %d", code)
	}
}

func TestCategoryRules_ATermIsOneLineOfUpToHundredCharacters(t *testing.T) {
	s := newCatalogStack(t)
	cat := s.makeCategory("A", nil)
	for term, want := range map[string]int{
		"":                       400,
		"   ":                    400,
		strings.Repeat("é", 101): 400,
		strings.Repeat("é", 100): 201,
		"com\x00nulo":            400,
		"duas\nlinhas":           201,
	} {
		if code, _ := s.addRule(cat, term); code != want {
			t.Errorf("%q (%d characters): %d, want %d", term[:min(len(term), 12)], utf8.RuneCountInString(term), code, want)
		}
	}
	if code := s.do(admin, "POST", fmt.Sprintf("/admin/categories/%d/rules", cat), `nope`).Code; code != 400 {
		t.Errorf("not json: %d, want 400", code)
	}
	if code, _ := s.addRule(99999, "x"); code != 404 {
		t.Errorf("unknown category: %d, want 404", code)
	}
	if code := s.do(admin, "POST", "/admin/categories/x/rules", `{"term":"x"}`).Code; code != 404 {
		t.Errorf("bad id: %d, want 404", code)
	}
}

func TestCategoryRules_ACategoryHasAtMostThreeHundredTerms(t *testing.T) {
	s := newCatalogStack(t)
	cat := s.makeCategory("A", nil)
	s.exec(`INSERT INTO category_rules (category_id, term, term_key) SELECT $1, 't' || g, 't' || g FROM generate_series(1, 299) g`, cat)
	if code, _ := s.addRule(cat, "the 300th"); code != 201 {
		t.Fatalf("the 300th: %d", code)
	}
	if code, _ := s.addRule(cat, "the 301st"); code != 400 {
		t.Errorf("the 301st: %d, want 400", code)
	}
}

func TestCategoryRules_AreListedByCategoryAndTermAndRemoved(t *testing.T) {
	s := newCatalogStack(t)
	zeta := s.makeCategory("Zeta", nil)
	alfa := s.makeCategory("Ação", nil)
	s.mustRule(zeta, "b", "a")
	s.mustRule(alfa, "z")
	rec := s.do(admin, "GET", "/admin/categories/rules", "")
	var list struct {
		Data []CategoryRule `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	got := []string{}
	for _, r := range list.Data {
		got = append(got, fmt.Sprintf("%d:%s", r.CategoryID, r.Term))
	}
	if want := fmt.Sprintf("%d:z|%d:a|%d:b", alfa, zeta, zeta); strings.Join(got, "|") != want {
		t.Errorf("rules = %v, want %s", got, want)
	}
	id := list.Data[0].ID
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/categories/rules/%d", id), "").Code; code != 204 {
		t.Errorf("removing: %d", code)
	}
	for _, bad := range []string{fmt.Sprint(id), "99999", "x"} {
		if code := s.do(admin, "DELETE", "/admin/categories/rules/"+bad, "").Code; code != 404 {
			t.Errorf("removing %s: %d, want 404", bad, code)
		}
	}
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'category.rule_%'`); got != "category.rule_add,category.rule_add,category.rule_add,category.rule_remove" {
		t.Errorf("audit = %q", got)
	}
	// Deleting the category takes its rules.
	if code := s.do(admin, "DELETE", fmt.Sprintf("/admin/categories/%d", zeta), "").Code; code != 204 {
		t.Fatalf("deleting the category: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM category_rules`); got != "0" {
		t.Errorf("%s rules left", got)
	}
}

func TestCategoryRulesPreview_SaysWhatApplyingWouldDoAndDoesNothing(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	manga := s.makeCategory("Mangá", nil)
	s.makeCategory("Sem regras", nil)
	s.mustRule(scifi, "science fiction", "ficção científica", "sci-fi")
	s.mustRule(manga, "manga")

	exact := s.addWork("Neuromancer", "William Gibson", "a.epub", "epub")
	accent := s.addWork("Solaris", "Stanisław Lem", "b.epub", "epub")
	parts := s.addWork("Duna", "Frank Herbert", "c.epub", "epub")
	upper := s.addWork("Foundation", "Isaac Asimov", "d.epub", "epub")
	none := s.addWork("Poemas", "Alguém", "e.epub", "epub")
	substring := s.addWork("Física", "Alguém", "f.epub", "epub")
	retired := s.addWork("Retirada", "Alguém", "g.epub", "epub")
	both := s.addWork("Akira", "Katsuhiro Otomo", "h.cbz", "cbz")
	already := s.addWork("Já está", "Alguém", "i.epub", "epub")
	s.tag(exact, "Science Fiction")
	s.tag(accent, "FICCAO CIENTIFICA")
	s.tag(parts, "Fiction / Science Fiction / General")
	s.tag(upper, "SCI-FI", "Classics")
	s.tag(none, "Poetry")
	s.tag(substring, "Science")
	s.tag(retired, "Science Fiction")
	s.tag(both, "Manga", "Sci-Fi")
	s.tag(already, "science fiction")
	s.retire(retired)
	s.inCategory(already, scifi)
	before := s.scalar(`SELECT count(*) FROM work_categories`)

	got := s.rulesPreview()
	want := RulesPreview{
		Categories: []RulesPreviewCategory{
			{ID: scifi, Name: "Ficção científica", Matched: 6, Fresh: 5},
			{ID: manga, Name: "Mangá", Matched: 1, Fresh: 1},
		},
		Links: 6, Works: 5,
		WithoutNow:   8, // 9 works, retired not counted, one in a category
		WithoutAfter: 2, // Poemas and Física
	}
	// 9 works are active (the retired one is not): all but "Já está" have no category.
	want.WithoutNow = 7
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("preview = %+v\nwant     %+v", got, want)
	}
	if after := s.scalar(`SELECT count(*) FROM work_categories`); after != before {
		t.Errorf("the preview changed the links: %s -> %s", before, after)
	}
}

func TestCategoryRulesPreview_IsEmptyWithNoRules(t *testing.T) {
	s := newCatalogStack(t)
	s.makeCategory("A", nil)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	got := s.rulesPreview()
	if len(got.Categories) != 0 || got.Links != 0 || got.Works != 0 || got.WithoutNow != 1 || got.WithoutAfter != 1 {
		t.Errorf("%+v", got)
	}
	rec := s.do(admin, "GET", "/admin/categories/rules/preview", "")
	if !strings.Contains(rec.Body.String(), `"categories":[]`) {
		t.Errorf("categories is not a list: %s", rec.Body.String())
	}
}

func TestCategoryRulesApply_PutsTheWorksInTheCategoriesOnlyIfThePreviewIsStillTrue(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	s.mustRule(scifi, "science fiction")
	a := s.addWork("Neuromancer", "William Gibson", "a.epub", "epub")
	b := s.addWork("Duna", "Frank Herbert", "b.epub", "epub")
	manual := s.addWork("À mão", "Alguém", "c.epub", "epub")
	s.tag(a, "Science Fiction")
	s.tag(b, "Fiction / Science Fiction / General")
	s.inCategory(manual, scifi)
	s.tag(manual, "Science Fiction")
	preview := s.rulesPreview()
	if preview.Links != 2 {
		t.Fatalf("links = %d", preview.Links)
	}

	// Not what the person saw: nothing is done.
	for _, links := range []int{0, 1, 3} {
		if code, _ := s.applyRules(links); code != 409 {
			t.Errorf("links %d: %d, want 409", links, code)
		}
	}
	if s.linked(a) != "" || s.linked(b) != "" {
		t.Fatalf("a refused request put works in categories: %q %q", s.linked(a), s.linked(b))
	}
	code, body := s.applyRules(2)
	if code != 200 || body != `{"links":2,"works":2}` {
		t.Fatalf("apply: %d %s", code, body)
	}
	if got := s.linked(a); got != fmt.Sprintf("%d:rule", scifi) {
		t.Errorf("a = %q", got)
	}
	if got := s.linked(b); got != fmt.Sprintf("%d:rule", scifi) {
		t.Errorf("b = %q", got)
	}
	if got := s.linked(manual); got != fmt.Sprintf("%d:manual", scifi) {
		t.Errorf("what a person put stays theirs: %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_categories WHERE source = 'rule' AND assigned_by = $1`, idAdmin); got != "2" {
		t.Errorf("assigned by the one who applied: %s", got)
	}
	if got := s.scalar(`SELECT details->>'links' || '|' || (details->>'works') FROM audit_log WHERE action = 'category.rules_applied'`); got != "2|2" {
		t.Errorf("audit = %q", got)
	}
	// Applied, there is nothing more to do, and asking again does no harm.
	if again := s.rulesPreview(); again.Links != 0 || again.Works != 0 || again.WithoutAfter != 0 {
		t.Errorf("after: %+v", again)
	}
	if code, body := s.applyRules(0); code != 200 || body != `{"links":0,"works":0}` {
		t.Errorf("again: %d %s", code, body)
	}
	if n := s.scalar(`SELECT count(*) FROM work_categories`); n != "3" {
		t.Errorf("%s links", n)
	}
}

func TestCategoryRulesApply_NeedsTheNumberTheyAreLookingAt(t *testing.T) {
	s := newCatalogStack(t)
	for name, body := range map[string]string{"nothing": `{}`, "null": `{"links":null}`, "negative": `{"links":-1}`, "text": `{"links":"2"}`, "not json": `nope`} {
		if code := s.do(admin, "POST", "/admin/categories/rules/apply", body).Code; code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
}

func TestCategoryRulesApply_NeverTakesAWorkOutOfACategory(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	fantasy := s.makeCategory("Fantasia", nil)
	s.mustRule(fantasy, "fantasy")
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.tag(work, "Fantasy") // the tags say fantasy, the person said sci-fi
	s.inCategory(work, scifi)
	if code, _ := s.applyRules(1); code != 200 {
		t.Fatalf("apply: %d", code)
	}
	if got := s.linked(work); got != fmt.Sprintf("%d:manual,%d:rule", scifi, fantasy) {
		t.Errorf("links = %q: the one the person put stays, and the rule adds", got)
	}
}

func TestCategoryRulesApply_DoNotPutBackWhatAPersonTookOut(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	s.mustRule(scifi, "science fiction")
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.tag(work, "Science Fiction")
	if code, _ := s.applyRules(1); code != 200 {
		t.Fatal("first apply")
	}
	excluded := func() string {
		return s.scalar(`SELECT count(*) FROM work_category_exclusions WHERE work_id = $1`, work)
	}

	// A person takes it out: the rules leave it out from then on.
	s.setWorkCategories(work, "[]")
	if s.linked(work) != "" || excluded() != "1" {
		t.Fatalf("taken out: %q, exclusions %s", s.linked(work), excluded())
	}
	if p := s.rulesPreview(); p.Links != 0 || p.Categories[0].Matched != 1 || p.Categories[0].Fresh != 0 {
		t.Errorf("preview = %+v: it matches, and it is not new", p)
	}
	if code, body := s.applyRules(0); code != 200 || body != `{"links":0,"works":0}` {
		t.Errorf("apply: %d %s", code, body)
	}
	if s.linked(work) != "" {
		t.Errorf("put back: %q", s.linked(work))
	}
	// A person puts it back: it is theirs, and the exclusion is gone.
	s.setWorkCategories(work, fmt.Sprintf("[%d]", scifi))
	if excluded() != "0" || s.linked(work) != fmt.Sprintf("%d:manual", scifi) {
		t.Errorf("put back by a person: %q, exclusions %s", s.linked(work), excluded())
	}
	// Taking it out again is excluding it again.
	s.setWorkCategories(work, "[]")
	if excluded() != "1" {
		t.Errorf("out again: exclusions %s", excluded())
	}
}

func TestCategoryRulesApply_DoNotPutBackWhatAPersonPutAndTookOutEither(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	s.mustRule(scifi, "science fiction")
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.inCategory(work, scifi)
	s.setWorkCategories(work, "[]")
	// It is tagged afterwards: the person took it out, so the rules leave it out.
	s.tag(work, "Science Fiction")
	if p := s.rulesPreview(); p.Links != 0 || p.Categories[0].Matched != 1 {
		t.Errorf("preview = %+v", p)
	}
	// Other works are not affected by it.
	other := s.addWork("Outra", "Alguém", "b.epub", "epub")
	s.tag(other, "Science Fiction")
	if p := s.rulesPreview(); p.Links != 1 {
		t.Errorf("links = %d, want the other work's", p.Links)
	}
	// A work that was never in the category has nothing taken out of it, so nothing is excluded.
	s.setWorkCategories(other, "[]")
	if got := s.scalar(`SELECT count(*) FROM work_category_exclusions`); got != "1" {
		t.Errorf("%s exclusions, want only the first work's", got)
	}
}

func TestCategoryRules_AreAppliedAcrossTheTreeAndToTheParentOnlyWhereTheyAre(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.makeCategory("Mangá", nil)
	seinen := s.makeCategory("Seinen", &manga)
	s.mustRule(manga, "manga")
	s.mustRule(seinen, "seinen")
	berserk := s.addWork("Berserk", "Kentaro Miura", "a.cbz", "cbz")
	other := s.addWork("Outro", "Alguém", "b.cbz", "cbz")
	s.tag(berserk, "Manga", "Seinen")
	s.tag(other, "Seinen")
	if code, _ := s.applyRules(3); code != 200 {
		t.Fatal("apply")
	}
	if got := s.linked(berserk); got != fmt.Sprintf("%d:rule,%d:rule", manga, seinen) {
		t.Errorf("berserk = %q", got)
	}
	if got := s.linked(other); got != fmt.Sprintf("%d:rule", seinen) {
		t.Errorf("other = %q", got)
	}
	by := map[string]Category{}
	for _, c := range s.categories() {
		by[c.Name] = c
	}
	if by["Mangá"].Works != 2 || by["Mangá"].Own != 1 || by["Seinen"].Works != 2 {
		t.Errorf("counts: %+v %+v", by["Mangá"], by["Seinen"])
	}
}

func starterTerms(g starterGroup) int {
	n := len(g.Terms)
	for _, c := range g.Children {
		n += starterTerms(c)
	}
	return n
}

func starterNames(g starterGroup) int {
	n := 1
	for _, c := range g.Children {
		n += starterNames(c)
	}
	return n
}

func TestCategoryStarter_TheListIsWellMade(t *testing.T) {
	s := newCatalogStack(t)
	if len(starterList) < 20 {
		t.Errorf("%d groups, want at least 20", len(starterList))
	}
	top := map[string]bool{}
	var check func(g starterGroup, depth int, siblings map[string]bool)
	check = func(g starterGroup, depth int, siblings map[string]bool) {
		if depth > 2 {
			t.Errorf("%q is %d levels deep", g.Name, depth)
		}
		if name, ok := tidyCategoryName(g.Name); !ok || name != g.Name {
			t.Errorf("%q is not a good name", g.Name)
		}
		if siblings[strings.ToLower(g.Name)] {
			t.Errorf("%q twice side by side", g.Name)
		}
		siblings[strings.ToLower(g.Name)] = true
		if len(g.Terms) == 0 {
			t.Errorf("%q has no terms", g.Name)
		}
		keys := map[string]bool{}
		for _, term := range g.Terms {
			if tidy, ok := tidyTerm(term); !ok || tidy != term {
				t.Errorf("%q: %q is not a good term", g.Name, term)
			}
			// Compared the way the rules are: without accents either.
			key := s.scalar(`SELECT `+termKeySQL("$1::text"), term)
			if keys[key] {
				t.Errorf("%q: %q twice (or only with another accent)", g.Name, term)
			}
			keys[key] = true
		}
		if len(g.Terms) > maxRulesPerCategory {
			t.Errorf("%q has %d terms", g.Name, len(g.Terms))
		}
		kids := map[string]bool{}
		for _, c := range g.Children {
			check(c, depth+1, kids)
		}
	}
	for _, g := range starterList {
		check(g, 1, top)
	}
	for _, want := range []string{"Quadrinhos", "Mangá"} {
		if !top[strings.ToLower(want)] {
			t.Errorf("%s is not in the list", want)
		}
	}
}

func TestCategoryStarter_IsOfferedAndMakesOnlyWhatWasChosenAndOnlyCategoriesAndTerms(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(admin, "GET", "/admin/categories/starter", "")
	var offered struct {
		Data []starterGroup `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &offered)
	if rec.Code != 200 || len(offered.Data) != len(starterList) {
		t.Fatalf("offered: %d, %d groups", rec.Code, len(offered.Data))
	}
	if n := len(s.categories()); n != 0 {
		t.Fatalf("asking for the list made %d categories", n)
	}
	work := s.addWork("Berserk", "Kentaro Miura", "a.cbz", "cbz")
	s.tag(work, "Seinen")

	rec = s.do(admin, "POST", "/admin/categories/starter", `{"groups":["Mangá","Quadrinhos","Mangá"]}`)
	var made struct{ Categories, Rules int }
	json.Unmarshal(rec.Body.Bytes(), &made)
	wantCats, wantRules := 0, 0
	for _, g := range starterList {
		if g.Name == "Mangá" || g.Name == "Quadrinhos" {
			wantCats += starterNames(g)
			wantRules += starterTerms(g)
		}
	}
	if rec.Code != 200 || made.Categories != wantCats || made.Rules != wantRules {
		t.Fatalf("made: %d %+v, want %d categories and %d rules", rec.Code, made, wantCats, wantRules)
	}
	if got := len(s.categories()); got != wantCats {
		t.Errorf("%d categories", got)
	}
	by := map[string]Category{}
	for _, c := range s.categories() {
		by[c.Name] = c
	}
	if c := by["Seinen"]; c.ParentID == nil || *c.ParentID != by["Mangá"].ID {
		t.Errorf("Seinen = %+v", c)
	}
	if got := s.scalar(`SELECT count(*) FROM work_categories`); got != "0" {
		t.Errorf("%s works were put in categories", got)
	}
	// Asked for again, nothing is made twice.
	rec = s.do(admin, "POST", "/admin/categories/starter", `{"groups":["Mangá"]}`)
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Categories != 0 || made.Rules != 0 {
		t.Errorf("again: %d %+v", rec.Code, made)
	}
	// The rules do work once they are applied.
	p := s.rulesPreview()
	if p.Links != 1 || p.Works != 1 {
		t.Fatalf("preview: %+v", p)
	}
	if code, _ := s.applyRules(1); code != 200 {
		t.Fatal("apply")
	}
	if got := s.linked(work); got != fmt.Sprintf("%d:rule", by["Seinen"].ID) {
		t.Errorf("Berserk = %q", got)
	}
	if got := s.scalar(`SELECT details->>'categories' || '|' || (details->>'rules') FROM audit_log WHERE action = 'category.starter' ORDER BY id LIMIT 1`); got != fmt.Sprintf("%d|%d", wantCats, wantRules) {
		t.Errorf("audit = %q", got)
	}
}

func TestCategoryStarter_UsesTheCategoryThatIsAlreadyThereAndAddsItsTerms(t *testing.T) {
	s := newCatalogStack(t)
	mine := s.makeCategory("mangá", nil) // another way to write it: it is the same category
	s.mustRule(mine, "manga")
	rec := s.do(admin, "POST", "/admin/categories/starter", `{"groups":["Mangá"]}`)
	var made struct{ Categories, Rules int }
	json.Unmarshal(rec.Body.Bytes(), &made)
	var group starterGroup
	for _, g := range starterList {
		if g.Name == "Mangá" {
			group = g
		}
	}
	if made.Categories != starterNames(group)-1 {
		t.Errorf("made %d categories, want %d (the top one existed)", made.Categories, starterNames(group)-1)
	}
	if made.Rules != starterTerms(group)-1 { // "manga" was a rule already
		t.Errorf("made %d rules, want %d", made.Rules, starterTerms(group)-1)
	}
	if got := s.scalar(`SELECT count(*) FROM categories WHERE parent_id IS NULL`); got != "1" {
		t.Errorf("%s categories at the top", got)
	}
	if got := s.category(mine).Name; got != "mangá" {
		t.Errorf("the name the person chose was changed to %q", got)
	}
}

func TestCategoryStarter_RefusesWhatIsNotInTheList(t *testing.T) {
	s := newCatalogStack(t)
	for name, body := range map[string]string{
		"nothing": `{}`, "empty": `{"groups":[]}`, "unknown": `{"groups":["Inventada"]}`,
		"a child": `{"groups":["Seinen"]}`, "one unknown among good ones": `{"groups":["Mangá","Inventada"]}`, "not json": `nope`,
	} {
		if code := s.do(admin, "POST", "/admin/categories/starter", body).Code; code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if n := len(s.categories()); n != 0 {
		t.Errorf("a refused request made %d categories", n)
	}
}

func TestCategoryRulesPreview_ATagWithASlashIsComparedWholeAndByItsParts(t *testing.T) {
	s := newCatalogStack(t)
	cat := s.makeCategory("Ficção especulativa", nil)
	s.mustRule(cat, "sci-fi/fantasy") // the whole tag, which has a slash
	whole := s.addWork("Inteira", "Alguém", "a.epub", "epub")
	part := s.addWork("Parte", "Alguém", "b.epub", "epub")
	other := s.addWork("Outra", "Alguém", "c.epub", "epub")
	s.tag(whole, "Sci-Fi/Fantasy")
	s.tag(part, "Sci-Fi")
	s.tag(other, "Fantasy")
	if p := s.rulesPreview(); p.Links != 1 || p.Categories[0].Matched != 1 {
		t.Errorf("preview = %+v: only the whole tag matches a term that has the slash", p)
	}
	s.mustRule(cat, "sci-fi")
	if p := s.rulesPreview(); p.Links != 2 {
		t.Errorf("links = %d: a part of a tag is a term of its own too", p.Links)
	}
}

func TestCategoryRulesPreview_SpacesInATagCountAsOne(t *testing.T) {
	s := newCatalogStack(t)
	cat := s.makeCategory("Ficção científica", nil)
	s.mustRule(cat, "science fiction")
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.tag(work, "  Science \t  Fiction ")
	if p := s.rulesPreview(); p.Links != 1 {
		t.Errorf("links = %d: the spaces of the tag are not the same as the term's", p.Links)
	}
}

func TestCategoryRulesApply_CountsAWorkOnceWhateverTheNumberOfPlacesItGets(t *testing.T) {
	s := newCatalogStack(t)
	scifi := s.makeCategory("Ficção científica", nil)
	classics := s.makeCategory("Clássicos", nil)
	s.mustRule(scifi, "science fiction")
	s.mustRule(classics, "classics")
	work := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	s.tag(work, "Science Fiction", "Classics")
	if p := s.rulesPreview(); p.Links != 2 || p.Works != 1 {
		t.Fatalf("preview = %+v", p)
	}
	if code, body := s.applyRules(2); code != 200 || body != `{"links":2,"works":1}` {
		t.Errorf("apply: %d %s", code, body)
	}
}
