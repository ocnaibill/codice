package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/versions"
)

// The manual graph (#83, DEC-109): a person's concepts and the relations they draw. All personal.

type conceptView struct {
	ID            int64
	Name          string
	Description   string
	Aliases       []string
	RelationCount int
}

type relationView struct {
	ID        int64
	Type      string
	Label     string
	Inverse   string
	Symmetric bool
	Source    nodeView
	Target    nodeView
	Direction string
	Reads     string
	Comment   string
	Origin    string
}

type nodeView struct {
	Kind   string
	ID     int64
	Label  string
	Detail string
	State  string
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return v
}

func (s *catalogStack) newConcept(a actor, name string, aliases ...string) conceptView {
	s.t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "aliases": aliases})
	rec := s.do(a, "POST", "/concepts", string(body))
	if rec.Code != 201 {
		s.t.Fatalf("concept %q: %d %s", name, rec.Code, rec.Body.String())
	}
	return decode[conceptView](s.t, rec)
}

func (s *catalogStack) relate(a actor, sk string, sid int64, typ, tk string, tid int64) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.do(a, "POST", "/relations", fmt.Sprintf(`{"sourceKind":%q,"sourceId":%d,"type":%q,"targetKind":%q,"targetId":%d}`, sk, sid, typ, tk, tid))
}

func (s *catalogStack) mustRelate(a actor, sk string, sid int64, typ, tk string, tid int64) relationView {
	s.t.Helper()
	rec := s.relate(a, sk, sid, typ, tk, tid)
	if rec.Code != 201 {
		s.t.Fatalf("%s %s:%d -> %s:%d: %d %s", typ, sk, sid, tk, tid, rec.Code, rec.Body.String())
	}
	return decode[relationView](s.t, rec)
}

func (s *catalogStack) relationsOf(a actor, kind string, id int64) []relationView {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/relations?kind=%s&id=%d", kind, id), "")
	if rec.Code != 200 {
		s.t.Fatalf("relations of %s:%d: %d %s", kind, id, rec.Code, rec.Body.String())
	}
	return decode[struct{ Data []relationView }](s.t, rec).Data
}

func (s *catalogStack) newNote(a actor, work int, body string) int64 {
	s.t.Helper()
	b, _ := json.Marshal(map[string]any{"quote": "uma passagem", "body": body})
	rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), string(b))
	if rec.Code != 201 && rec.Code != 200 {
		s.t.Fatalf("note: %d %s", rec.Code, rec.Body.String())
	}
	return decode[struct{ ID int64 }](s.t, rec).ID
}

func TestGraph_TheTypesAreTheNineOfTheDesign(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "GET", "/graph/types", "")
	if rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	types := decode[struct {
		Types []struct {
			Key, Label, Inverse string
			Symmetric           bool
			Pairs               []struct{ Source, Target string }
		}
	}](t, rec).Types
	if len(types) != 9 || types[0].Key != "related" || !types[0].Symmetric || len(types[0].Pairs) != 9 {
		t.Fatalf("%+v", types)
	}
	var about int
	for i, ty := range types {
		if ty.Key == "about" {
			about = i
		}
	}
	got := types[about]
	if got.Label != "trata de" || got.Inverse != "é tratado em" || got.Symmetric || len(got.Pairs) != 1 || got.Pairs[0].Source != "work" || got.Pairs[0].Target != "concept" {
		t.Errorf("%+v", got)
	}
}

func TestConcepts_AreKeptReadChangedAndDeleted(t *testing.T) {
	s := newCatalogStack(t)
	c := s.newConcept(ana, "  Inteligência   Artificial ", "IA", " ia ", "AI", "")
	if c.Name != "Inteligência Artificial" || strings.Join(c.Aliases, ",") != "IA,AI" || c.Description != "" || c.RelationCount != 0 || c.ID == 0 {
		t.Fatalf("%+v", c)
	}
	rec := s.do(ana, "GET", fmt.Sprintf("/concepts/%d", c.ID), "")
	got := decode[struct {
		Concept   conceptView
		Relations []relationView
	}](t, rec)
	if rec.Code != 200 || got.Concept.Name != c.Name || len(got.Relations) != 0 || got.Relations == nil {
		t.Errorf("%d %+v", rec.Code, got)
	}
	rec = s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), `{"description":"Máquinas que aprendem.","aliases":["machine learning"]}`)
	c = decode[conceptView](t, rec)
	if rec.Code != 200 || c.Description != "Máquinas que aprendem." || strings.Join(c.Aliases, ",") != "machine learning" || c.Name != "Inteligência Artificial" {
		t.Fatalf("%d %+v", rec.Code, c)
	}
	// What was not sent is left as it was; what is renamed is found under the new name only.
	rec = s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), `{"name":"IA"}`)
	c = decode[conceptView](t, rec)
	if c.Name != "IA" || c.Description != "Máquinas que aprendem." || len(c.Aliases) != 1 {
		t.Fatalf("%+v", c)
	}
	if s.do(ana, "GET", "/concepts/resolve?name=Inteligência+Artificial", "").Code != 404 || s.do(ana, "GET", "/concepts/resolve?name=ia", "").Code != 200 {
		t.Error("the keys follow the name")
	}
	list := decode[struct{ Data []conceptView }](t, s.do(ana, "GET", "/concepts", "")).Data
	if len(list) != 1 || list[0].ID != c.ID {
		t.Errorf("%+v", list)
	}
	rec = s.do(ana, "DELETE", fmt.Sprintf("/concepts/%d", c.ID), "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"removedRelations":0`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if s.do(ana, "GET", fmt.Sprintf("/concepts/%d", c.ID), "").Code != 404 || s.scalar(`SELECT count(*) FROM concept_keys`) != "0" {
		t.Error("the concept and its names are gone")
	}
	if s.do(ana, "DELETE", fmt.Sprintf("/concepts/%d", c.ID), "").Code != 404 {
		t.Error("deleting twice")
	}
}

func TestConcepts_ANameBelongsToOneConceptOfThePersonByWhatItReadsAs(t *testing.T) {
	s := newCatalogStack(t)
	ia := s.newConcept(ana, "Inteligência Artificial", "IA")
	other := s.newConcept(ana, "Estoicismo")
	for _, name := range []string{"inteligencia artificial!", "  IA ", "Ia"} {
		body, _ := json.Marshal(map[string]any{"name": name})
		rec := s.do(ana, "POST", "/concepts", string(body))
		if rec.Code != 409 || !strings.Contains(rec.Body.String(), fmt.Sprintf(`"conceptId":%d`, ia.ID)) {
			t.Errorf("%q: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	// An alias that is another concept's name, and a rename onto a taken name: refused, and nothing changed.
	if rec := s.do(ana, "POST", "/concepts", `{"name":"Lógica","aliases":["estoicismo"]}`); rec.Code != 409 {
		t.Errorf("alias against a name: %d", rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM concepts`) != "2" || s.scalar(`SELECT count(*) FROM concept_keys`) != "3" {
		t.Errorf("a refused concept left something: %s %s", s.scalar(`SELECT count(*) FROM concepts`), s.scalar(`SELECT count(*) FROM concept_keys`))
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", other.ID), `{"name":"IA","description":"mudou?"}`); rec.Code != 409 {
		t.Fatalf("rename onto a taken name: %d", rec.Code)
	}
	if got := decode[struct{ Concept conceptView }](t, s.do(ana, "GET", fmt.Sprintf("/concepts/%d", other.ID), "")).Concept; got.Description != "" || got.Name != "Estoicismo" {
		t.Errorf("a refused change was kept: %+v", got)
	}
	// A concept may take back its own name, as its alias or the other way round; another person may use the same names.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", ia.ID), `{"name":"IA","aliases":["Inteligência Artificial"]}`); rec.Code != 200 {
		t.Errorf("its own names: %d %s", rec.Code, rec.Body.String())
	}
	if c := s.newConcept(bob, "IA", "Estoicismo"); c.ID == 0 {
		t.Error("another person")
	}
}

func TestConcepts_AreFoundByTheirNameOrAnAliasAsAnoteWouldPointToThem(t *testing.T) {
	s := newCatalogStack(t)
	c := s.newConcept(ana, "Inteligência Artificial", "IA")
	for _, name := range []string{"Inteligência Artificial", "inteligencia   artificial", "IA", "ia."} {
		rec := s.do(ana, "GET", "/concepts/resolve?name="+strings.ReplaceAll(name, " ", "%20"), "")
		if rec.Code != 200 || decode[conceptView](t, rec).ID != c.ID {
			t.Errorf("%q: %d", name, rec.Code)
		}
	}
	for _, name := range []string{"", "%20%20", "Outro", "inteligencia"} {
		if rec := s.do(ana, "GET", "/concepts/resolve?name="+name, ""); rec.Code != 404 {
			t.Errorf("%q: %d", name, rec.Code)
		}
	}
	if s.do(ana, "GET", "/concepts/resolve", "").Code != 404 {
		t.Error("no name")
	}
}

func TestConcepts_TheListIsFilteredByTheBeginningOfANameOrAnAlias(t *testing.T) {
	s := newCatalogStack(t)
	s.newConcept(ana, "Zeta")
	ia := s.newConcept(ana, "Inteligência Artificial", "IA")
	s.newConcept(ana, "100% humano")
	s.newConcept(ana, "a_b")
	names := func(q string) string {
		var out []string
		for _, c := range decode[struct{ Data []conceptView }](t, s.do(ana, "GET", "/concepts?q="+q, "")).Data {
			out = append(out, c.Name)
		}
		return strings.Join(out, "|")
	}
	if got := names(""); got != "100% humano|a_b|Inteligência Artificial|Zeta" {
		t.Errorf("by name: %s", got)
	}
	if names("intel") != "Inteligência Artificial" || names("ia") != "Inteligência Artificial" || names("IA") != "Inteligência Artificial" {
		t.Errorf("by name or alias: %q %q", names("intel"), names("ia"))
	}
	if names("zz") != "" || names("rtificial") != "" {
		t.Error("only the beginning")
	}
	// What is only punctuation finds nothing (it is no name), and the wildcards of the database are only punctuation here.
	if names("%25") != "" || names("_") != "" || names("...") != "" || names("a_") != "a_b" {
		t.Errorf("punctuation: %q %q %q %q", names("%25"), names("_"), names("..."), names("a_"))
	}
	_ = ia
}

func TestConcepts_WhatIsNotAConceptIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	long := strings.Repeat("a", 121)
	aliases := make([]string, 21)
	for i := range aliases {
		aliases[i] = fmt.Sprintf("apelido %d", i)
	}
	tooMany, _ := json.Marshal(map[string]any{"name": "Muitos", "aliases": aliases})
	for name, body := range map[string]string{
		"no name":            `{}`,
		"empty name":         `{"name":""}`,
		"blank name":         `{"name":"   "}`,
		"only punctuation":   `{"name":"!!!"}`,
		"too long a name":    `{"name":"` + long + `"}`,
		"too long a text":    `{"name":"x","description":"` + strings.Repeat("a", 4001) + `"}`,
		"too long an alias":  `{"name":"x","aliases":["` + long + `"]}`,
		"too many aliases":   string(tooMany),
		"not json":           `nope`,
		"a name that is not": `{"name":5}`,
	} {
		if rec := s.do(ana, "POST", "/concepts", body); rec.Code != 400 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if s.scalar(`SELECT count(*) FROM concepts`) != "0" {
		t.Error("something was kept")
	}
	// The limits themselves are fine, and control characters are taken out.
	body, _ := json.Marshal(map[string]any{"name": strings.Repeat("a", 120), "description": strings.Repeat("b", 4000), "aliases": aliases[:20]})
	if rec := s.do(ana, "POST", "/concepts", string(body)); rec.Code != 201 {
		t.Errorf("at the limits: %d %s", rec.Code, rec.Body.String())
	}
	if c := decode[conceptView](t, s.do(ana, "POST", "/concepts", "{\"name\":\"Com\\u0000controle\",\"description\":\"linha\\r\\ndois\"}")); c.Name != "Comcontrole" || c.Description != "linha\ndois" {
		t.Errorf("%+v", c)
	}
	if rec := s.do(ana, "PATCH", "/concepts/abc", `{}`); rec.Code != 404 {
		t.Errorf("a bad id: %d", rec.Code)
	}
	c := s.newConcept(ana, "Existe")
	for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{"description":"` + strings.Repeat("a", 4001) + `"}`, `nope`} {
		if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), body); rec.Code != 400 {
			t.Errorf("patch %.30s: %d", body, rec.Code)
		}
	}
}

func TestConcepts_ThereIsALimitOfThemForAPerson(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO concepts (user_id, name) SELECT $1, 'c' || g FROM generate_series(1, $2) g`, idAna, maxConcepts)
	if rec := s.do(ana, "POST", "/concepts", `{"name":"mais um"}`); rec.Code != 409 {
		t.Errorf("%d", rec.Code)
	}
	if rec := s.do(bob, "POST", "/concepts", `{"name":"mais um"}`); rec.Code != 201 {
		t.Errorf("another person: %d", rec.Code)
	}
}

func TestGraph_EverythingInItIsPersonal(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	mine := s.newConcept(ana, "Poder", "Domínio")
	note := s.newNote(ana, work, "A especiaria controla o poder")
	rel := s.mustRelate(ana, "note", note, "mentions", "concept", mine.ID)
	own := s.newConcept(bob, "Outro")

	// Bob cannot reach any of it: not found, never forbidden.
	for _, c := range []struct{ method, path, body string }{
		{"GET", fmt.Sprintf("/concepts/%d", mine.ID), ""},
		{"PATCH", fmt.Sprintf("/concepts/%d", mine.ID), `{"name":"Meu"}`},
		{"DELETE", fmt.Sprintf("/concepts/%d", mine.ID), ""},
		{"GET", "/concepts/resolve?name=Poder", ""},
		{"GET", "/concepts/resolve?name=Dom%C3%ADnio", ""},
		{"PATCH", fmt.Sprintf("/relations/%d", rel.ID), `{"comment":"meu"}`},
		{"DELETE", fmt.Sprintf("/relations/%d", rel.ID), ""},
	} {
		if rec := s.do(bob, c.method, c.path, c.body); rec.Code != 404 {
			t.Errorf("%s %s: %d", c.method, c.path, rec.Code)
		}
	}
	if got := s.relationsOf(bob, "concept", mine.ID); len(got) != 0 {
		t.Errorf("her relations: %+v", got)
	}
	if got := s.relationsOf(bob, "note", note); len(got) != 0 {
		t.Errorf("her relations of her note: %+v", got)
	}
	all := decode[struct{ Data []relationView }](t, s.do(bob, "GET", "/relations", "")).Data
	if len(all) != 0 || len(decode[struct{ Data []conceptView }](t, s.do(bob, "GET", "/concepts", "")).Data) != 1 {
		t.Errorf("his graph holds hers: %+v", all)
	}
	// He cannot draw a relation to what is hers, either, whether it is her concept or her note.
	if rec := s.relate(bob, "concept", own.ID, "related", "concept", mine.ID); rec.Code != 404 {
		t.Errorf("to her concept: %d", rec.Code)
	}
	if rec := s.relate(bob, "note", note, "mentions", "concept", own.ID); rec.Code != 404 {
		t.Errorf("from her note: %d", rec.Code)
	}
	// And hers are untouched.
	if s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idAna+`'`) != "1" || s.scalar(`SELECT name FROM concepts WHERE id = `+fmt.Sprint(mine.ID)) != "Poder" {
		t.Error("something of hers changed")
	}
}

func TestRelations_EachTypeJoinsTheKindsOfItsDesign(t *testing.T) {
	s := newCatalogStack(t)
	w1, w2 := s.addWork("Um", "A", "1.epub", "epub"), s.addWork("Dois", "B", "2.epub", "epub")
	a, b := s.newConcept(ana, "A"), s.newConcept(ana, "B")
	n1, n2 := s.newNote(ana, w1, "primeira"), s.newNote(ana, w1, "segunda")
	W1, W2 := int64(w1), int64(w2)
	for _, c := range []struct {
		typ        string
		sk         string
		sid        int64
		tk         string
		tid        int64
		label, inv string
	}{
		{"related", "work", W1, "concept", a.ID, "relacionado a", "relacionado a"},
		{"related", "note", n1, "note", n2, "relacionado a", "relacionado a"},
		{"part_of", "concept", a.ID, "concept", b.ID, "é parte de", "tem como parte"},
		{"kind_of", "concept", b.ID, "concept", a.ID, "é um tipo de", "tem como tipo"},
		{"about", "work", W1, "concept", b.ID, "trata de", "é tratado em"},
		{"exemplifies", "note", n1, "concept", a.ID, "exemplifica", "é exemplificado por"},
		{"exemplifies", "work", W2, "concept", a.ID, "exemplifica", "é exemplificado por"},
		{"defines", "note", n2, "concept", b.ID, "define", "é definido em"},
		{"in_dialogue_with", "work", W1, "work", W2, "dialoga com", "dialoga com"},
		{"in_dialogue_with", "note", n1, "note", n2, "dialoga com", "dialoga com"},
		{"opposes", "concept", a.ID, "concept", b.ID, "se opõe a", "se opõe a"},
		{"mentions", "note", n2, "concept", a.ID, "menciona", "é mencionado em"},
	} {
		r := s.mustRelate(ana, c.sk, c.sid, c.typ, c.tk, c.tid)
		if r.Type != c.typ || r.Label != c.label || r.Inverse != c.inv || r.Source.Kind != c.sk || r.Source.ID != c.sid || r.Target.Kind != c.tk ||
			r.Target.ID != c.tid || r.Origin != "manual" || r.Comment != "" || r.Source.State != "available" || r.Target.State != "available" {
			t.Errorf("%+v", r)
		}
	}
}

func TestRelations_WhatADesignDoesNotAllowIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Um", "A", "1.epub", "epub")
	c, d := s.newConcept(ana, "A"), s.newConcept(ana, "B")
	n := s.newNote(ana, w, "nota")
	W := int64(w)
	other := s.addWork("Dois", "B", "2.epub", "epub")
	cases := map[string]*httptest.ResponseRecorder{
		"a type that is not on the list":  s.relate(ana, "concept", c.ID, "made_up", "concept", d.ID),
		"no type":                         s.relate(ana, "concept", c.ID, "", "concept", d.ID),
		"a kind that is not one":          s.relate(ana, "author", 1, "related", "concept", d.ID),
		"a kind that is not one (target)": s.relate(ana, "concept", c.ID, "related", "tag", 1),
		"a node that is not a number":     s.relate(ana, "concept", 0, "related", "concept", d.ID),
		"a negative one":                  s.relate(ana, "concept", c.ID, "related", "concept", -3),
		"itself":                          s.relate(ana, "concept", c.ID, "related", "concept", c.ID),
		"about from a note":               s.relate(ana, "note", n, "about", "concept", c.ID),
		"about to a work":                 s.relate(ana, "concept", c.ID, "about", "work", W),
		"part of between works":           s.relate(ana, "work", W, "part_of", "work", int64(other)),
		"defines from a work":             s.relate(ana, "work", W, "defines", "concept", c.ID),
		"mentions from a concept":         s.relate(ana, "concept", c.ID, "mentions", "concept", d.ID),
		"dialogue between kinds":          s.relate(ana, "work", W, "in_dialogue_with", "note", n),
		"dialogue between concepts":       s.relate(ana, "concept", c.ID, "in_dialogue_with", "concept", d.ID),
	}
	for name, rec := range cases {
		if rec.Code != 400 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := s.relate(ana, "concept", c.ID, "part_of", "concept", 999999); rec.Code != 404 {
		t.Errorf("a concept that is not there: %d", rec.Code)
	}
	if rec := s.relate(ana, "note", 999999, "mentions", "concept", c.ID); rec.Code != 404 {
		t.Errorf("a note that is not there: %d", rec.Code)
	}
	if rec := s.relate(ana, "work", 999999, "about", "concept", c.ID); rec.Code != 404 {
		t.Errorf("a work that is not there: %d", rec.Code)
	}
	if rec := s.do(ana, "POST", "/relations", `{"sourceKind":"concept","sourceId":1,"type":"related","targetKind":"concept","targetId":2,"comment":"`+strings.Repeat("a", 1001)+`"}`); rec.Code != 400 {
		t.Errorf("a long comment: %d", rec.Code)
	}
	if rec := s.do(ana, "POST", "/relations", `nope`); rec.Code != 400 {
		t.Errorf("not json: %d", rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM relations`) != "0" {
		t.Error("a refused relation was kept")
	}
}

func TestRelations_TheSameRelationIsOneAndAPairWithNoDirectionCountsOnce(t *testing.T) {
	s := newCatalogStack(t)
	a, b := s.newConcept(ana, "A"), s.newConcept(ana, "B")
	s.mustRelate(ana, "concept", a.ID, "part_of", "concept", b.ID)
	if rec := s.relate(ana, "concept", a.ID, "part_of", "concept", b.ID); rec.Code != 409 {
		t.Errorf("twice: %d", rec.Code)
	}
	// A direction has two ways, and the other one is another relation; another type is another relation too.
	s.mustRelate(ana, "concept", b.ID, "part_of", "concept", a.ID)
	s.mustRelate(ana, "concept", a.ID, "kind_of", "concept", b.ID)
	// With no direction, the pair is one whichever end was the source.
	s.mustRelate(ana, "concept", a.ID, "related", "concept", b.ID)
	if rec := s.relate(ana, "concept", b.ID, "related", "concept", a.ID); rec.Code != 409 {
		t.Errorf("reversed, with no direction: %d", rec.Code)
	}
	s.mustRelate(ana, "concept", a.ID, "opposes", "concept", b.ID)
	if rec := s.relate(ana, "concept", b.ID, "opposes", "concept", a.ID); rec.Code != 409 {
		t.Errorf("reversed: %d", rec.Code)
	}
	// And another person may draw the same.
	x, y := s.newConcept(bob, "A"), s.newConcept(bob, "B")
	s.mustRelate(bob, "concept", x.ID, "related", "concept", y.ID)
	if s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idAna+`'`) != "5" {
		t.Errorf("%s", s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idAna+`'`))
	}
}

func TestRelations_AreReadFromANodeWithTheDirectionAndHowTheyReadFromThere(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	power, politics := s.newConcept(ana, "Poder"), s.newConcept(ana, "Política")
	note := s.newNote(ana, w, "A especiaria controla o poder")
	s.mustRelate(ana, "work", int64(w), "about", "concept", power.ID)
	s.mustRelate(ana, "concept", power.ID, "part_of", "concept", politics.ID)
	s.mustRelate(ana, "concept", politics.ID, "related", "concept", power.ID)
	rel := s.mustRelate(ana, "note", note, "mentions", "concept", power.ID)

	got := s.relationsOf(ana, "concept", power.ID)
	if len(got) != 4 {
		t.Fatalf("%+v", got)
	}
	by := map[string]relationView{}
	for _, r := range got {
		by[r.Type] = r
	}
	about := by["about"]
	if about.Direction != "in" || about.Reads != "é tratado em" || about.Source.Label != "Duna" || about.Source.Detail != "Frank Herbert" || about.Target.Label != "Poder" {
		t.Errorf("about: %+v", about)
	}
	part := by["part_of"]
	if part.Direction != "out" || part.Reads != "é parte de" || part.Target.Label != "Política" {
		t.Errorf("part_of: %+v", part)
	}
	related := by["related"]
	if related.Direction != "in" || related.Reads != "relacionado a" || !related.Symmetric {
		t.Errorf("related: %+v", related)
	}
	mention := by["mentions"]
	if mention.ID != rel.ID || mention.Direction != "in" || mention.Reads != "é mencionado em" || mention.Source.Kind != "note" || mention.Source.Detail != "Duna" {
		t.Errorf("mentions: %+v", mention)
	}
	// From the other end of one of them.
	if got := s.relationsOf(ana, "concept", politics.ID); len(got) != 2 {
		t.Errorf("politics: %+v", got)
	}
	// The newest first.
	if got[0].ID < got[len(got)-1].ID {
		t.Errorf("not newest first: %v %v", got[0].ID, got[len(got)-1].ID)
	}
	// The whole graph of the person, and what a node with none reads as.
	if all := decode[struct{ Data []relationView }](t, s.do(ana, "GET", "/relations", "")).Data; len(all) != 4 || all[0].Direction != "" {
		t.Errorf("all: %d", len(all))
	}
	if got := s.relationsOf(ana, "work", int64(w)); len(got) != 1 || got[0].Direction != "out" || got[0].Reads != "trata de" {
		t.Errorf("the work: %+v", got)
	}
	free := s.newConcept(ana, "Solto")
	if got := s.relationsOf(ana, "concept", free.ID); got == nil || len(got) != 0 {
		t.Errorf("a node with none: %+v", got)
	}
	c := decode[struct{ Concept conceptView }](t, s.do(ana, "GET", fmt.Sprintf("/concepts/%d", power.ID), "")).Concept
	if c.RelationCount != 4 {
		t.Errorf("the count: %d", c.RelationCount)
	}
	for _, q := range []string{"", "?kind=concept", "?id=3", "?kind=author&id=1", "?kind=concept&id=x", "?kind=concept&id=0"} {
		want := 400
		if q == "" {
			want = 200
		}
		if rec := s.do(ana, "GET", "/relations"+q, ""); rec.Code != want {
			t.Errorf("%q: %d", q, rec.Code)
		}
	}
}

func TestRelations_TheCommentAndTheTypeCanBeChanged(t *testing.T) {
	s := newCatalogStack(t)
	a, b := s.newConcept(ana, "A"), s.newConcept(ana, "B")
	rel := s.mustRelate(ana, "concept", a.ID, "related", "concept", b.ID)
	path := fmt.Sprintf("/relations/%d", rel.ID)
	got := decode[relationView](t, s.do(ana, "PATCH", path, `{"comment":"  porque sim\r\n"}`))
	if got.Comment != "porque sim" || got.Type != "related" {
		t.Fatalf("%+v", got)
	}
	got = decode[relationView](t, s.do(ana, "PATCH", path, `{"type":"opposes"}`))
	if got.Type != "opposes" || got.Label != "se opõe a" || got.Comment != "porque sim" {
		t.Fatalf("a type changed, the comment kept: %+v", got)
	}
	got = decode[relationView](t, s.do(ana, "PATCH", path, `{"type":"part_of","comment":""}`))
	if got.Type != "part_of" || got.Comment != "" {
		t.Fatalf("both: %+v", got)
	}
	// Another that cannot join these two kinds, an unknown one, and one that would repeat another relation.
	if rec := s.do(ana, "PATCH", path, `{"type":"about"}`); rec.Code != 400 {
		t.Errorf("a type for other kinds: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", path, `{"type":"made_up"}`); rec.Code != 400 {
		t.Errorf("unknown: %d", rec.Code)
	}
	s.mustRelate(ana, "concept", a.ID, "kind_of", "concept", b.ID)
	if rec := s.do(ana, "PATCH", path, `{"type":"kind_of"}`); rec.Code != 409 {
		t.Errorf("a repeat: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", path, `{"comment":"`+strings.Repeat("a", 1001)+`"}`); rec.Code != 400 {
		t.Errorf("a long comment: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", path, `{}`); rec.Code != 200 {
		t.Errorf("nothing to change: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", "/relations/999999", `{"comment":"x"}`); rec.Code != 404 {
		t.Errorf("not there: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", "/relations/abc", `{}`); rec.Code != 404 {
		t.Errorf("not an id: %d", rec.Code)
	}
	if rec := s.do(ana, "DELETE", path, ""); rec.Code != 200 || s.scalar(`SELECT count(*) FROM concepts`) != "2" {
		t.Errorf("deleting a relation leaves what it joined: %d", rec.Code)
	}
	if rec := s.do(ana, "DELETE", path, ""); rec.Code != 404 {
		t.Errorf("twice: %d", rec.Code)
	}
}

func TestRelations_AWorkThatLeavesTheLibraryLeavesTheRelationWithWhatItWasCalled(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	concept := s.newConcept(ana, "Poder")
	s.mustRelate(ana, "work", int64(w), "about", "concept", concept.ID)
	state := func() nodeView {
		t.Helper()
		got := s.relationsOf(ana, "concept", concept.ID)
		if len(got) != 1 {
			t.Fatalf("%+v", got)
		}
		return got[0].Source
	}
	if n := state(); n.State != "available" || n.Label != "Duna" {
		t.Fatalf("%+v", n)
	}
	// In the trash: the relation stays, says so, and comes back with the work.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, w)
	if n := state(); n.State != "retired" || n.Label != "Duna" || n.Detail != "Frank Herbert" {
		t.Errorf("retired: %+v", n)
	}
	if rec := s.relate(ana, "work", int64(w), "about", "concept", s.newConcept(ana, "Outro").ID); rec.Code != 404 {
		t.Errorf("a new relation to a work in the trash: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = NULL WHERE id = $1`, w)
	if n := state(); n.State != "available" {
		t.Errorf("restored: %+v", n)
	}
	// Deleted for good: what it was called when the relation was drawn, and the person can still take it away.
	s.exec(`UPDATE works SET original_title = 'Duna (nova capa)' WHERE id = $1`, w)
	if n := state(); n.Label != "Duna (nova capa)" {
		t.Errorf("renamed while it is there: %+v", n)
	}
	s.exec(`DELETE FROM works WHERE id = $1`, w)
	n := state()
	if n.State != "deleted" || n.Label != "Duna" || n.Detail != "Frank Herbert" || n.ID != int64(w) {
		t.Errorf("deleted: %+v", n)
	}
	rels := s.relationsOf(ana, "concept", concept.ID)
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/relations/%d", rels[0].ID), ""); rec.Code != 200 {
		t.Errorf("removing it: %d", rec.Code)
	}
}

func TestRelations_WhatThePersonDeletesTakesItsRelationsWithIt(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	a, b := s.newConcept(ana, "A"), s.newConcept(ana, "B")
	note := s.newNote(ana, w, "uma nota")
	s.mustRelate(ana, "note", note, "mentions", "concept", a.ID)
	s.mustRelate(ana, "note", note, "defines", "concept", b.ID)
	s.mustRelate(ana, "concept", a.ID, "part_of", "concept", b.ID)
	s.mustRelate(ana, "work", int64(w), "about", "concept", a.ID)
	// Another person's relation to the same work stays whatever she does.
	c := s.newConcept(bob, "C")
	s.mustRelate(bob, "work", int64(w), "about", "concept", c.ID)

	if rec := s.do(ana, "DELETE", fmt.Sprintf("/notes/%d", note), ""); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if got := s.scalar(`SELECT count(*) FROM relations WHERE user_id = '` + idAna + `'`); got != "2" {
		t.Errorf("the note's two relations go, the rest stay: %s", got)
	}
	rec := s.do(ana, "DELETE", fmt.Sprintf("/concepts/%d", a.ID), "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"removedRelations":2`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT count(*) FROM relations WHERE user_id = '` + idAna + `'`); got != "0" {
		t.Errorf("left: %s", got)
	}
	if s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idBob+`'`) != "1" || s.scalar(`SELECT count(*) FROM concepts WHERE id = `+fmt.Sprint(b.ID)) != "1" {
		t.Error("what was not hers, or not deleted, was touched")
	}
	// A person who goes takes their concepts and relations; the others' stay.
	s.mustRelate(ana, "work", int64(w), "about", "concept", b.ID)
	s.exec(`DELETE FROM users WHERE id = $1`, idAna)
	if s.scalar(`SELECT count(*) FROM concepts WHERE user_id = '`+idAna+`'`) != "0" || s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idAna+`'`) != "0" ||
		s.scalar(`SELECT count(*) FROM concept_keys WHERE user_id = '`+idAna+`'`) != "0" || s.scalar(`SELECT count(*) FROM relations`) != "1" {
		t.Error("a person deleted left graph behind, or took another's")
	}
}

func TestRelations_ANoteIsReadByItsTextAndSaysWhichWorkItIsFrom(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	c := s.newConcept(ana, "Poder")
	long := s.newNote(ana, w, strings.Repeat("palavra ", 15))
	short := s.newNote(ana, w, "")
	s.mustRelate(ana, "note", long, "mentions", "concept", c.ID)
	s.mustRelate(ana, "note", short, "mentions", "concept", c.ID)
	var labels = map[int64]string{}
	for _, r := range s.relationsOf(ana, "concept", c.ID) {
		labels[r.Source.ID] = r.Source.Label
		if r.Source.Detail != "Duna" {
			t.Errorf("the work: %+v", r.Source)
		}
	}
	if !strings.HasSuffix(labels[long], "…") || len([]rune(labels[long])) != 81 || !strings.HasPrefix(labels[long], "palavra palavra") {
		t.Errorf("a long note: %q", labels[long])
	}
	if labels[short] != "uma passagem" {
		t.Errorf("a note with only the quotation: %q", labels[short])
	}
}

func TestJoinVersions_TheRelationsOfTheWorkThatGoesFollowToTheOneThatStays(t *testing.T) {
	s := newCatalogStack(t)
	keep, gone := s.addWork("Duna", "Frank Herbert", "k.epub", "epub"), s.addWork("Dune", "Frank Herbert", "g.pdf", "pdf")
	K, G := int64(keep), int64(gone)
	power, politics, free := s.newConcept(ana, "Poder"), s.newConcept(ana, "Política"), s.newConcept(ana, "Livre")
	// What the work that goes had, from either end, and what the one that stays already had too.
	s.mustRelate(ana, "work", G, "about", "concept", power.ID)
	s.mustRelate(ana, "work", K, "about", "concept", power.ID)
	s.mustRelate(ana, "work", G, "about", "concept", politics.ID)
	s.mustRelate(ana, "concept", free.ID, "related", "work", G)
	s.mustRelate(ana, "work", K, "related", "concept", free.ID) // the same pair as the one just above, once moved
	s.mustRelate(ana, "work", G, "in_dialogue_with", "work", K) // between the two: it would join a work to itself
	x := s.addWork("Outro", "X", "x.epub", "epub")
	s.mustRelate(ana, "work", int64(x), "in_dialogue_with", "work", G)
	s.mustRelate(ana, "work", G, "opposes", "concept", free.ID)
	cb := s.newConcept(bob, "Dele")
	s.mustRelate(bob, "work", G, "about", "concept", cb.ID)

	if _, err := versions.Join(context.Background(), s.db, keep, gone, idAdmin); err != nil {
		t.Fatal(err)
	}
	if s.scalar(`SELECT count(*) FROM relations WHERE (source_kind = 'work' AND source_id = `+fmt.Sprint(gone)+`) OR (target_kind = 'work' AND target_id = `+fmt.Sprint(gone)+`)`) != "0" {
		t.Error("a relation is left with the work that went")
	}
	got := map[string]int{}
	for _, r := range s.relationsOf(ana, "work", K) {
		other := r.Target
		if r.Direction == "in" {
			other = r.Source
		}
		got[r.Type+" "+other.Label]++
	}
	want := map[string]int{
		"about Poder":            1, // the one it had, and the one that came, once
		"about Política":         1,
		"related Livre":          1, // from either end, once
		"in_dialogue_with Outro": 1,
		"opposes Livre":          1,
		"in_dialogue_with Duna":  0, // joined to itself: gone
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: %d, want %d (all: %v)", k, got[k], n, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("%v", got)
	}
	// What was another person's follows for them, too; and what the person had stays theirs.
	if rels := s.relationsOf(bob, "work", K); len(rels) != 1 || rels[0].Target.Label != "Dele" {
		t.Errorf("his: %+v", rels)
	}
	if s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idAna+`' AND comment <> ''`) != "0" {
		t.Error("a comment appeared")
	}
}

func TestRelations_TheDatabaseHoldsWhatTheServerPromises(t *testing.T) {
	s := newCatalogStack(t)
	a, b := s.newConcept(ana, "A"), s.newConcept(ana, "B")
	insert := func(sk string, sid int64, typ, tk string, tid int64) error {
		_, err := s.db.Exec(`INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id) VALUES ($1, $2, $3, $4, $5, $6)`, idAna, sk, sid, typ, tk, tid)
		return err
	}
	if err := insert("concept", a.ID, "related", "concept", a.ID); err == nil {
		t.Error("a relation of a node with itself")
	}
	if err := insert("author", a.ID, "related", "concept", b.ID); err == nil {
		t.Error("a kind that is not one")
	}
	if err := insert("concept", a.ID, "related", "concept", b.ID); err != nil {
		t.Fatal(err)
	}
	if err := insert("concept", b.ID, "related", "concept", a.ID); err == nil {
		t.Error("the reverse of a pair with no direction")
	}
	if err := insert("concept", a.ID, "related", "concept", b.ID); err == nil {
		t.Error("the same twice")
	}
	if _, err := s.db.Exec(`INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id, origin) VALUES ($1, 'concept', $2, 'mentions', 'concept', $3, 'guess')`, idAna, a.ID, b.ID); err == nil {
		t.Error("an origin that is not one")
	}
}

func TestRelations_ThereIsALimitOfThemForAPerson(t *testing.T) {
	s := newCatalogStack(t)
	c := s.newConcept(ana, "A")
	s.exec(`INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id)
	        SELECT $1, 'note', g, 'mentions', 'concept', $2 FROM generate_series(1, $3) g`, idAna, c.ID, maxRelations)
	d := s.newConcept(ana, "B")
	if rec := s.relate(ana, "concept", c.ID, "related", "concept", d.ID); rec.Code != 409 {
		t.Errorf("%d", rec.Code)
	}
}
