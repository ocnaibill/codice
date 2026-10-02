package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// [[Concept]] links in notes (#21, DEC-110): the relation comes from the text, the text is never changed.

type noteView struct {
	ID    int64
	Body  string
	Links map[string]*struct {
		ConceptID   int64
		Name        string
		Description string
	}
}

func (s *catalogStack) notesOf(a actor) map[int64]noteView {
	s.t.Helper()
	rec := s.do(a, "GET", "/notes?limit=100", "")
	if rec.Code != 200 {
		s.t.Fatalf("notes: %d %s", rec.Code, rec.Body.String())
	}
	out := map[int64]noteView{}
	for _, n := range decode[struct{ Data []noteView }](s.t, rec).Data {
		out[n.ID] = n
	}
	return out
}

func (s *catalogStack) editNote(a actor, id int64, jsonBody string) {
	s.t.Helper()
	if rec := s.do(a, "PATCH", fmt.Sprintf("/notes/%d", id), jsonBody); rec.Code != 200 {
		s.t.Fatalf("edit note: %d %s", rec.Code, rec.Body.String())
	}
}

// links says what the note's relations to concepts are, as "type:concept id:origin".
func (s *catalogStack) mentionsOf(a actor, note int64) []string {
	s.t.Helper()
	var out []string
	for _, r := range s.relationsOf(a, "note", note) {
		out = append(out, fmt.Sprintf("%s:%d:%s", r.Type, r.Target.ID, r.Origin))
	}
	return out
}

func TestWikilinks_AConceptOfThePersonIsLinkedAndOneThatDoesNotExistIsPending(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	poder := s.newConcept(ana, "Poder", "dominação")
	body := "Sobre [[Poder]] e [[dominacao|o domínio]], mas também [[Destino]]."
	note := s.newNote(ana, w, body)

	if got := s.mentionsOf(ana, note); len(got) != 1 || got[0] != fmt.Sprintf("mentions:%d:wikilink", poder.ID) {
		t.Fatalf("a name and an alias of one concept are one relation: %v", got)
	}
	got := s.notesOf(ana)[note]
	if got.Body != body {
		t.Errorf("the text is as it was written: %q", got.Body)
	}
	if len(got.Links) != 3 || got.Links["Destino"] != nil || got.Links["Poder"] == nil || got.Links["dominacao"] == nil {
		t.Fatalf("links: %+v", got.Links)
	}
	if got.Links["Poder"].ConceptID != poder.ID || got.Links["dominacao"].ConceptID != poder.ID || got.Links["dominacao"].Name != "Poder" {
		t.Errorf("both point to the concept: %+v %+v", got.Links["Poder"], got.Links["dominacao"])
	}
	if s.scalar(`SELECT count(*) FROM concepts WHERE user_id = '`+idAna+`'`) != "1" {
		t.Error("a pending link must not make a concept")
	}
	// The relation reads like any other, from the concept.
	rels := s.relationsOf(ana, "concept", poder.ID)
	if len(rels) != 1 || rels[0].Source.Kind != "note" || rels[0].Source.ID != note || rels[0].Origin != "wikilink" || rels[0].Inverse != "é mencionado em" {
		t.Errorf("%+v", rels)
	}
}

func TestWikilinks_ACodeSpanOrBlockHoldsNoLink(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.newConcept(ana, "Poder")
	note := s.newNote(ana, w, "`[[Poder]]` e\n```\n[[Poder]]\n```\n\\[[Poder]]")
	if got := s.mentionsOf(ana, note); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if n := s.notesOf(ana)[note]; len(n.Links) != 0 {
		t.Errorf("%+v", n.Links)
	}
}

func TestWikilinks_ThePersonCreatingTheConceptResolvesWhatWasPending(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	one := s.newNote(ana, w, "Ideia de [[Estoicismo]]")
	two := s.newNote(ana, w, "Outra de [[estoicismo!]] e [[Virtude]]")
	other := s.newNote(ana, w, "Sem relação")
	if len(s.mentionsOf(ana, one)) != 0 {
		t.Fatal("pending links make no relation")
	}

	c := s.newConcept(ana, "Estoicismo")
	for _, n := range []int64{one, two} {
		if got := s.mentionsOf(ana, n); len(got) != 1 || got[0] != fmt.Sprintf("mentions:%d:wikilink", c.ID) {
			t.Errorf("note %d: %v", n, got)
		}
	}
	if len(s.mentionsOf(ana, other)) != 0 {
		t.Error("a note that cites nothing")
	}
	if l := s.notesOf(ana)[two].Links; l["Virtude"] != nil || l["estoicismo!"] == nil {
		t.Errorf("%+v", l)
	}

	// A new alias does the same.
	s.newConcept(ana, "Ética", "Virtude")
	ethics := s.scalar(`SELECT id FROM concepts WHERE name = 'Ética'`)
	if got := s.mentionsOf(ana, two); len(got) != 2 {
		t.Errorf("an alias given on creation: %v (concept %s)", got, ethics)
	}
}

func TestWikilinks_AnAliasAddedLaterAndARenameFollowTheText(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	c := s.newConcept(ana, "Inteligência Artificial")
	note := s.newNote(ana, w, "Fala de [[IA]] e de [[Inteligência Artificial]]")
	want := fmt.Sprintf("mentions:%d:wikilink", c.ID)
	if got := s.mentionsOf(ana, note); len(got) != 1 || got[0] != want {
		t.Fatal(got)
	}
	if s.notesOf(ana)[note].Links["IA"] != nil {
		t.Fatal("IA is not yet a name of it")
	}

	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), `{"aliases":["IA"]}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if n := s.notesOf(ana)[note]; n.Links["IA"] == nil || n.Links["IA"].ConceptID != c.ID {
		t.Errorf("the alias resolves the link: %+v", n.Links)
	}

	// A new name that drops the old one: the note still cites the old one, which is pending now, and the relation goes
	// with the link that no longer points anywhere. The text is never touched.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), `{"name":"Aprendizado de máquina","aliases":[]}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if got := s.mentionsOf(ana, note); len(got) != 0 {
		t.Errorf("the relation goes with the name: %v", got)
	}
	n := s.notesOf(ana)[note]
	if n.Body != "Fala de [[IA]] e de [[Inteligência Artificial]]" || n.Links["IA"] != nil || n.Links["Inteligência Artificial"] != nil {
		t.Errorf("%+v", n)
	}

	// Naming it the old name again brings the relation back.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), `{"aliases":["ia"]}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if got := s.mentionsOf(ana, note); len(got) != 1 || got[0] != want {
		t.Errorf("%v", got)
	}

	// A change that leaves the names as they are (the description, the case) changes no relation.
	before := s.scalar(`SELECT id FROM relations WHERE origin = 'wikilink'`)
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", c.ID), `{"description":"x","name":"APRENDIZADO DE MÁQUINA"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if s.scalar(`SELECT id FROM relations WHERE origin = 'wikilink'`) != before {
		t.Error("the relation was made again")
	}
}

func TestWikilinks_TheRelationFollowsTheTextWhenItIsEdited(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.newConcept(ana, "A")
	b := s.newConcept(ana, "B")
	note := s.newNote(ana, w, "[[A]]")
	bystander := s.newNote(ana, w, "[[A]] e [[B]]")
	if got := s.mentionsOf(ana, note); len(got) != 1 {
		t.Fatal(got)
	}

	s.editNote(ana, note, `{"body":"agora [[B]]"}`)
	if got := s.mentionsOf(ana, note); len(got) != 1 || got[0] != fmt.Sprintf("mentions:%d:wikilink", b.ID) {
		t.Errorf("A leaves and B comes: %v", got)
	}
	// Editing what is not the text leaves them as they are.
	ofNote := fmt.Sprintf(`SELECT id FROM relations WHERE origin = 'wikilink' AND source_id = %d`, note)
	before := s.scalar(ofNote)
	s.editNote(ana, note, `{"tags":["x"],"quote":"outra passagem"}`)
	if before == "" || s.scalar(ofNote) != before {
		t.Error("an edit of the tags redid the relation")
	}
	s.editNote(ana, note, `{"body":"sem link nenhum"}`)
	if got := s.mentionsOf(ana, note); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if got := s.mentionsOf(ana, bystander); len(got) != 2 {
		t.Errorf("another note's relations are not touched by the edit: %v", got)
	}
	if s.scalar(`SELECT count(*) FROM concepts WHERE user_id = '`+idAna+`'`) != "2" {
		t.Error("the concepts stay")
	}
}

func TestWikilinks_ARelationOfThePersonsOwnStaysWhenTheLinkGoes(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	c := s.newConcept(ana, "A")
	note := s.newNote(ana, w, "nada")
	manual := s.mustRelate(ana, "note", note, "mentions", "concept", c.ID)

	s.editNote(ana, note, `{"body":"cita [[A]]"}`)
	got := s.relationsOf(ana, "note", note)
	if len(got) != 1 || got[0].ID != manual.ID || got[0].Origin != "manual" {
		t.Fatalf("the one she drew is the one: %+v", got)
	}
	s.editNote(ana, note, `{"body":"já não cita"}`)
	if got := s.relationsOf(ana, "note", note); len(got) != 1 || got[0].ID != manual.ID {
		t.Errorf("it stays: %+v", got)
	}

	// And one that comes from a link cannot be drawn again by hand.
	other := s.newNote(ana, w, "cita [[A]]")
	if rec := s.relate(ana, "note", other, "mentions", "concept", c.ID); rec.Code != 409 {
		t.Errorf("%d", rec.Code)
	}
	// Another type is hers to draw.
	if rec := s.relate(ana, "note", other, "defines", "concept", c.ID); rec.Code != 201 {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestWikilinks_TheRelationFromTheTextIsChangedInTheText(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	c := s.newConcept(ana, "A")
	note := s.newNote(ana, w, "cita [[A]]")
	rel := s.relationsOf(ana, "note", note)[0]

	for _, body := range []string{`{"comment":"x"}`, `{"type":"related"}`, `{}`} {
		if rec := s.do(ana, "PATCH", fmt.Sprintf("/relations/%d", rel.ID), body); rec.Code != 409 || !strings.Contains(rec.Body.String(), "texto da nota") {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/relations/%d", rel.ID), ""); rec.Code != 409 || !strings.Contains(rec.Body.String(), "texto da nota") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if len(s.relationsOf(ana, "note", note)) != 1 {
		t.Error("it was changed")
	}
	// Not hers: not found, never forbidden (RN-006).
	if rec := s.do(bob, "DELETE", fmt.Sprintf("/relations/%d", rel.ID), ""); rec.Code != 404 {
		t.Errorf("%d", rec.Code)
	}
	if rec := s.do(bob, "PATCH", fmt.Sprintf("/relations/%d", rel.ID), `{"comment":"x"}`); rec.Code != 404 {
		t.Errorf("%d", rec.Code)
	}
	// One she drew is hers to take away, as before.
	m := s.mustRelate(ana, "concept", c.ID, "related", "note", note)
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/relations/%d", m.ID), ""); rec.Code != 200 {
		t.Errorf("%d", rec.Code)
	}
}

func TestWikilinks_DeletingTheConceptOrTheNoteLeavesTheTextAndPendsTheLink(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	c := s.newConcept(ana, "A")
	note := s.newNote(ana, w, "cita [[A]]")
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/concepts/%d", c.ID), ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"removedRelations":1`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	n := s.notesOf(ana)[note]
	if n.Body != "cita [[A]]" || n.Links["A"] != nil || len(n.Links) != 1 {
		t.Errorf("%+v", n)
	}
	// A concept made again gets it back.
	again := s.newConcept(ana, "A")
	if got := s.mentionsOf(ana, note); len(got) != 1 || got[0] != fmt.Sprintf("mentions:%d:wikilink", again.ID) {
		t.Errorf("%v", got)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/notes/%d", note), ""); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM relations`) != "0" {
		t.Error("the note took its relation")
	}
}

func TestWikilinks_AConceptIsOfThePersonAndSoIsWhatLinksToIt(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	bobs := s.newConcept(bob, "Poder")
	note := s.newNote(ana, w, "cita [[Poder]]")
	if got := s.mentionsOf(ana, note); len(got) != 0 {
		t.Fatalf("another person's concept is not hers to link: %v", got)
	}
	if n := s.notesOf(ana)[note]; n.Links["Poder"] != nil {
		t.Errorf("it leaks the other's concept: %+v", n.Links)
	}

	// Each makes theirs: the same name, two concepts, each linked to their own notes.
	bobNote := s.newNote(bob, w, "também [[Poder]]")
	hers := s.newConcept(ana, "Poder")
	if s.scalar(`SELECT count(*) FROM relations WHERE user_id = '`+idAna+`' AND source_id = `+fmt.Sprint(bobNote)) != "0" {
		t.Error("a concept of hers linked a note of his")
	}
	if got := s.mentionsOf(ana, note); len(got) != 1 || got[0] != fmt.Sprintf("mentions:%d:wikilink", hers.ID) {
		t.Errorf("%v", got)
	}
	if got := s.mentionsOf(bob, bobNote); len(got) != 1 || got[0] != fmt.Sprintf("mentions:%d:wikilink", bobs.ID) {
		t.Errorf("%v", got)
	}
	// What one does to a concept does not reach the other's notes.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/concepts/%d", hers.ID), `{"name":"Outro"}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if got := s.mentionsOf(bob, bobNote); len(got) != 1 {
		t.Errorf("a rename of hers took his: %v", got)
	}
	if rec := s.do(ana, "DELETE", fmt.Sprintf("/concepts/%d", hers.ID), ""); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if got := s.mentionsOf(bob, bobNote); len(got) != 1 {
		t.Errorf("%v", got)
	}
	if n := s.notesOf(bob)[bobNote]; n.Links["Poder"] == nil || n.Links["Poder"].ConceptID != bobs.ID {
		t.Errorf("%+v", n.Links)
	}
}

func TestWikilinks_AtTheLimitOfRelationsTheNoteIsStillSaved(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	c := s.newConcept(ana, "A")
	d := s.newConcept(ana, "B")
	s.exec(`INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id)
	        SELECT $1, 'note', g, 'mentions', 'concept', $2 FROM generate_series(100000, 100000 + $3 - 1) g`, idAna, c.ID, maxRelations)
	note := s.newNote(ana, w, "cita [[B]]")
	if s.scalar(`SELECT count(*) FROM relations WHERE origin = 'wikilink'`) != "0" {
		t.Error("a relation past the limit")
	}
	if n := s.notesOf(ana)[note]; n.Links["B"] == nil || n.Links["B"].ConceptID != d.ID {
		t.Errorf("the link still resolves: %+v", n.Links)
	}
}

func TestWikilinks_TheExportKeepsWhatWasWrittenAndAddsNothing(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.newConcept(ana, "Poder")
	s.newNote(ana, w, "Sobre [[Poder|o poder]] e [[Destino]]")
	md := s.do(ana, "GET", "/notes/export?format=md", "")
	if md.Code != 200 || !strings.Contains(md.Body.String(), "Sobre [[Poder|o poder]] e [[Destino]]") {
		t.Errorf("%d %s", md.Code, md.Body.String())
	}
	js := s.do(ana, "GET", "/notes/export?format=json", "")
	if js.Code != 200 || !strings.Contains(js.Body.String(), `Sobre [[Poder|o poder]] e [[Destino]]`) || strings.Contains(js.Body.String(), `"links"`) {
		t.Errorf("%d %s", js.Code, js.Body.String())
	}
}

func TestWikilinks_ANoteCarriesAHintOfTheConceptNotAllOfIt(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	long := strings.Repeat("a", 400)
	body, _ := json.Marshal(map[string]any{"name": "Poder", "description": long})
	if rec := s.do(ana, "POST", "/concepts", string(body)); rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	note := s.newNote(ana, w, "[[Poder]]")
	if d := s.notesOf(ana)[note].Links["Poder"].Description; len(d) != 280 {
		t.Errorf("%d", len(d))
	}
}
