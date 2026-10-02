package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The counts that the screen of notes offers to narrow the list by (UI-05): by kind and by tag.

type facetsView struct {
	Kinds map[string]int
	Tags  []struct {
		Tag   string
		Count int
	}
}

func (s *catalogStack) facets(a actor, query string) facetsView {
	s.t.Helper()
	rec := s.do(a, "GET", "/notes/facets"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /notes/facets%s: %d %s", query, rec.Code, rec.Body)
	}
	var out facetsView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatal(err)
	}
	return out
}

// tagCounts is "tag:count" in order, lower-cased: a tag is one whatever its case.
func (f facetsView) tagCounts() string {
	var out []string
	for _, t := range f.Tags {
		out = append(out, fmt.Sprintf("%s:%d", strings.ToLower(t.Tag), t.Count))
	}
	return strings.Join(out, " ")
}

func (s *catalogStack) seedFacets() (work1, work2 int) {
	s.t.Helper()
	work1, _, pdf := s.bookWithTwoFiles()
	work2 = s.addWork("Fundação", "Isaac Asimov", "f.epub", "epub")
	for _, n := range []struct {
		a    actor
		work int
		body string
	}{
		{ana, work1, `{"kind":"note","body":"a","tags":["Filosofia","poder"]}`},
		{ana, work1, `{"kind":"note","body":"b","tags":["filosofia"]}`},
		{ana, work1, `{"kind":"highlight","quote":"c","tags":["FILOSOFIA","ecologia"]}`},
		{ana, work1, `{"kind":"highlight","quote":"d","tags":["ecologia"]}`},
		{ana, work1, fmt.Sprintf(`{"kind":"bookmark","fileId":%d,"locator":{"type":"pdf","page":11}}`, pdf)},
		{ana, work2, `{"kind":"note","body":"e","tags":["poder"]}`},
		{ana, work2, `{"kind":"note","body":"sem tag"}`},
		{bob, work1, `{"kind":"note","body":"do bob","tags":["segredo","filosofia"]}`},
	} {
		if code, _ := s.addNote(n.a, n.work, n.body); code != 201 {
			s.t.Fatalf("seed %s: %d", n.body, code)
		}
	}
	return work1, work2
}

func TestNoteFacets_CountByKindAndByTagWhateverTheCase(t *testing.T) {
	s := newCatalogStack(t)
	s.seedFacets()
	f := s.facets(ana, "")
	if f.Kinds["note"] != 4 || f.Kinds["highlight"] != 2 || f.Kinds["bookmark"] != 1 {
		t.Errorf("kinds: %+v", f.Kinds)
	}
	// Filosofia, filosofia and FILOSOFIA are one tag with three notes; the commonest first, then by name.
	if got := f.tagCounts(); got != "filosofia:3 ecologia:2 poder:2" {
		t.Errorf("tags: %q", got)
	}
}

func TestNoteFacets_AreEmptyNotNullForAPersonWithNoNotes(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "GET", "/notes/facets", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"tags":[]`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	f := s.facets(ana, "")
	if f.Kinds["note"] != 0 || f.Kinds["highlight"] != 0 || f.Kinds["bookmark"] != 0 || len(f.Kinds) != 3 {
		t.Errorf("%+v", f.Kinds)
	}
}

func TestNoteFacets_FollowTheOtherFiltersButNotTheirOwn(t *testing.T) {
	s := newCatalogStack(t)
	w1, w2 := s.seedFacets()

	// The tag narrows the kinds and not the tags: the person sees what every other tag would give.
	f := s.facets(ana, "?tag=poder")
	if f.Kinds["note"] != 2 || f.Kinds["highlight"] != 0 || f.Kinds["bookmark"] != 0 {
		t.Errorf("tag=poder, kinds: %+v", f.Kinds)
	}
	if got := f.tagCounts(); got != "filosofia:3 ecologia:2 poder:2" {
		t.Errorf("tag=poder, tags must ignore their own choice: %q", got)
	}
	if f := s.facets(ana, "?tag=PODER"); f.Kinds["note"] != 2 {
		t.Errorf("a tag is one whatever its case: %+v", f.Kinds)
	}

	// The kind narrows the tags and not the kinds.
	f = s.facets(ana, "?kind=highlight")
	if got := f.tagCounts(); got != "ecologia:2 filosofia:1" {
		t.Errorf("kind=highlight, tags: %q", got)
	}
	if f.Kinds["note"] != 4 || f.Kinds["highlight"] != 2 || f.Kinds["bookmark"] != 1 {
		t.Errorf("kind=highlight, kinds must ignore their own choice: %+v", f.Kinds)
	}

	// The work and the text narrow both.
	f = s.facets(ana, fmt.Sprintf("?workId=%d", w2))
	if f.Kinds["note"] != 2 || f.Kinds["highlight"] != 0 || f.tagCounts() != "poder:1" {
		t.Errorf("workId=%d: %+v %q", w2, f.Kinds, f.tagCounts())
	}
	f = s.facets(ana, fmt.Sprintf("?workId=%d&tag=ecologia&kind=note", w1))
	// The kinds count under the tag (ecologia: two highlights), the tags under the kind (the tags of the notes).
	if f.Kinds["highlight"] != 2 || f.Kinds["note"] != 0 || f.tagCounts() != "filosofia:2 poder:1" {
		t.Errorf("both choices at once: %+v %q", f.Kinds, f.tagCounts())
	}
	f = s.facets(ana, "?q=sem+tag")
	if f.Kinds["note"] != 1 || len(f.Tags) != 0 {
		t.Errorf("q: %+v %q", f.Kinds, f.tagCounts())
	}
	// What is typed in a search is text, not a pattern.
	if f := s.facets(ana, "?q=%25"); f.Kinds["note"] != 0 {
		t.Errorf("a percent sign matches nothing: %+v", f.Kinds)
	}
}

func TestNoteFacets_AreOnlyOfThePerson(t *testing.T) {
	s := newCatalogStack(t)
	s.seedFacets()
	f := s.facets(bob, "")
	if f.Kinds["note"] != 1 || f.Kinds["highlight"] != 0 || f.tagCounts() != "filosofia:1 segredo:1" {
		t.Errorf("bob sees only his: %+v %q", f.Kinds, f.tagCounts())
	}
	if got := s.facets(ana, "").tagCounts(); strings.Contains(got, "segredo") {
		t.Errorf("a tag of bob's is in ana's: %q", got)
	}
	// Asking for the tag of someone else counts nothing.
	if f := s.facets(ana, "?tag=segredo"); f.Kinds["note"] != 0 {
		t.Errorf("%+v", f.Kinds)
	}
}

func TestNoteFacets_RefuseWhatTheListRefuses(t *testing.T) {
	s := newCatalogStack(t)
	for _, q := range []string{"?kind=comment", "?workId=x", "?fileId=-1", "?q=" + strings.Repeat("a", 201)} {
		if rec := s.do(ana, "GET", "/notes/facets"+q, ""); rec.Code != 400 {
			t.Errorf("%s: %d", q, rec.Code)
		}
	}
}

func TestNoteFacets_OfferTheCommonestHundredTags(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	for i := 0; i < 103; i++ {
		body := fmt.Sprintf(`{"body":"n%d","tags":["t%03d"]}`, i, i)
		if code, _ := s.addNote(ana, work, body); code != 201 {
			t.Fatal(code)
		}
	}
	// One tag has two notes, so it comes first.
	s.addNote(ana, work, `{"body":"extra","tags":["T050"]}`)
	f := s.facets(ana, "")
	if len(f.Tags) != 100 || strings.ToLower(f.Tags[0].Tag) != "t050" || f.Tags[0].Count != 2 {
		t.Errorf("%d tags, first %+v", len(f.Tags), f.Tags[0])
	}
}
