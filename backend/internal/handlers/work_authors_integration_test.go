package handlers

import (
	"fmt"
	"testing"
)

func authorsText(as []WorkAuthor) string {
	out := ""
	for i, a := range as {
		if i > 0 {
			out += ";"
		}
		out += fmt.Sprintf("%d:%s", a.ID, a.Name)
	}
	return out
}

func TestWorkAuthors_EveryCardCarriesItsAuthorsOneByOneSoEachCanBeLinked(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	solo := s.addWork("Sem autor", "", "b.epub", "epub")
	s.credit(admin, duna, "Brian Herbert", "author")
	s.credit(admin, duna, "Tradutora", "translator") // not an author: not here
	frank, brian := s.personID("Frank Herbert"), s.personID("Brian Herbert")

	l := s.list(ana, "?sort=title")
	byID := map[int]Work{}
	for _, w := range l.Data {
		byID[w.ID] = w
	}
	if got := authorsText(byID[duna].Authors); got != fmt.Sprintf("%d:Frank Herbert;%d:Brian Herbert", frank, brian) {
		t.Errorf("authors of the card = %s (in their order, translators out)", got)
	}
	if byID[duna].Author != "Frank Herbert, Brian Herbert" {
		t.Errorf("the text is still there: %q", byID[duna].Author)
	}
	// A work with no author has none, and says so with an empty list, not a null.
	if a := byID[solo].Authors; a == nil || len(a) != 0 {
		t.Errorf("a work with no author: %#v", a)
	}
	// The detail of a work carries them too.
	w, _ := s.detail(ana, duna)
	if got := authorsText(w.Authors); got != fmt.Sprintf("%d:Frank Herbert;%d:Brian Herbert", frank, brian) {
		t.Errorf("authors in the detail = %s", got)
	}
	// Putting another first changes the order of the card.
	s.do(admin, "PUT", fmt.Sprintf("/works/%d/contributors/order", duna), fmt.Sprintf(`{"role":"author","personIds":[%d,%d]}`, brian, frank))
	w, _ = s.detail(ana, duna)
	if got := authorsText(w.Authors); got != fmt.Sprintf("%d:Brian Herbert;%d:Frank Herbert", brian, frank) {
		t.Errorf("authors after the new order = %s", got)
	}
}

func TestWorkAuthors_TheNamesAreTheOnesTheAccountIsShown(t *testing.T) {
	s := newCatalogStack(t)
	herbert, _, both := s.authorsBook() // Frank Herbert has a known surname; Plato does not
	frank, plato := s.personID("Frank Herbert"), s.personID("Plato")
	s.setChoice(ana, "family_first")

	w, _ := s.detail(ana, both)
	if got := authorsText(w.Authors); got != fmt.Sprintf("%d:Herbert, Frank;%d:Plato", frank, plato) {
		t.Errorf("authors for the account that sees surnames first = %s", got)
	}
	w, _ = s.detail(bob, both)
	if got := authorsText(w.Authors); got != fmt.Sprintf("%d:Frank Herbert;%d:Plato", frank, plato) {
		t.Errorf("authors for the account that chose nothing = %s", got)
	}
	// The same in the list, which is built another way.
	for _, c := range s.list(ana, "").Data {
		if c.ID == herbert && authorsText(c.Authors) != fmt.Sprintf("%d:Herbert, Frank", frank) {
			t.Errorf("authors in the list = %s", authorsText(c.Authors))
		}
	}
}

func TestWorkAuthors_ThePassagesOfTheSearchCarryTheAuthorsToo(t *testing.T) {
	s := newCatalogStack(t)
	herbert, _, both := s.authorsBook()
	s.index(s.primaryFile(both), segment{text: "areia e especiaria no deserto de Arrakis", locator: `{"type":"epub","href":"c1.xhtml"}`})
	s.index(s.primaryFile(herbert), segment{text: "outra passagem sobre especiaria", locator: `{"type":"epub","href":"c2.xhtml"}`})
	frank, plato := s.personID("Frank Herbert"), s.personID("Plato")

	_, r := s.search(ana, q("especiaria"))
	if len(r.Data) != 2 {
		t.Fatalf("hits = %d, want 2", len(r.Data))
	}
	for _, h := range r.Data {
		switch h.WorkID {
		case both:
			if authorsText(h.WorkAuthors) != fmt.Sprintf("%d:Frank Herbert;%d:Plato", frank, plato) || h.WorkAuthor != "Frank Herbert, Plato" {
				t.Errorf("authors of the hit of two authors: %s / %q", authorsText(h.WorkAuthors), h.WorkAuthor)
			}
		case herbert:
			if authorsText(h.WorkAuthors) != fmt.Sprintf("%d:Frank Herbert", frank) {
				t.Errorf("authors of the hit of one author: %s", authorsText(h.WorkAuthors))
			}
		}
	}
	// The names follow the account here as well.
	s.setChoice(ana, "family_first")
	_, r = s.search(ana, q("especiaria"))
	for _, h := range r.Data {
		if h.WorkID == herbert && authorsText(h.WorkAuthors) != fmt.Sprintf("%d:Herbert, Frank", frank) {
			t.Errorf("authors of the hit for the surname-first account: %s", authorsText(h.WorkAuthors))
		}
	}
	// A hit of a work with no author has an empty list.
	bare := s.addWork("Anônima", "", "x.epub", "epub")
	s.index(s.primaryFile(bare), segment{text: "uma passagem anônima sobre especiaria", locator: `{"type":"epub","href":"c3.xhtml"}`})
	_, r = s.search(ana, q("anônima"))
	if len(r.Data) != 1 || r.Data[0].WorkAuthors == nil || len(r.Data[0].WorkAuthors) != 0 {
		t.Errorf("a hit with no author: %#v", r.Data)
	}
}
