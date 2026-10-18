package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

type peopleAnswer struct {
	Data []struct {
		ID    int
		Name  string
		Works int
	}
	Total   int
	Page    int
	Pages   int
	Letters []struct {
		Letter string
		People int
	}
}

func (s *catalogStack) people(a actor, query string) (int, peopleAnswer) {
	s.t.Helper()
	rec := s.do(a, "GET", "/device/people"+query, "")
	var out peopleAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestDevicePeople_ListsTheAuthorsByNameWithTheLettersThereAre(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Um", "Álvaro Campos", "a.epub", "epub")
	s.addWork("Dois", "Ana Souza", "b.epub", "epub")
	s.addWork("Três", "Ana Souza", "c.epub", "epub")
	s.addWork("Quatro", "Bruno Lima", "d.epub", "epub")
	s.addWork("Cinco", "123 Tester", "e.epub", "epub")

	code, got := s.people(ana, "")
	if code != 200 || got.Total != 4 {
		t.Fatalf("people: %d %+v", code, got)
	}
	// by name with no accent, as in the OPDS feed: what starts with a digit first, then Álvaro before Ana
	if got.Data[0].Name != "123 Tester" || got.Data[1].Name != "Álvaro Campos" || got.Data[2].Name != "Ana Souza" || got.Data[2].Works != 2 || got.Data[3].Name != "Bruno Lima" {
		t.Errorf("order and counts: %+v", got.Data)
	}
	if len(got.Letters) != 3 || got.Letters[0].Letter != "A" || got.Letters[0].People != 2 || got.Letters[1].Letter != "B" || got.Letters[2].Letter != "#" {
		t.Errorf("letters, with # at the end (a list of letters to jump to, not the order of the pages): %+v", got.Letters)
	}

	_, a := s.people(ana, "?letter=a")
	if a.Total != 2 || len(a.Data) != 2 || a.Data[0].Name != "Álvaro Campos" {
		t.Errorf("letter A: %+v", a)
	}
	_, hash := s.people(ana, "?letter=%23")
	if hash.Total != 1 || hash.Data[0].Name != "123 Tester" {
		t.Errorf("letter #: %+v", hash)
	}
	if _, none := s.people(ana, "?letter=Z"); none.Total != 0 || len(none.Data) != 0 || none.Pages != 1 {
		t.Errorf("a letter nobody has: %+v", none)
	}
	for _, bad := range []string{"?letter=AB", "?letter=1", "?letter=%C3%A1"} {
		if code, _ := s.people(ana, bad); code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}
	// a page past the end is the last one
	if _, far := s.people(ana, "?page=99"); far.Page != 1 || len(far.Data) != 4 {
		t.Errorf("page past the end: %+v", far)
	}
}

func TestDevicePeople_AreInPagesAndRetiredWorksDoNotCount(t *testing.T) {
	s := newCatalogStack(t)
	for i := 0; i < devicePeoplePage+5; i++ {
		s.addWork(fmt.Sprintf("Obra %d", i), fmt.Sprintf("Autor %03d", i), fmt.Sprintf("o%d.epub", i), "epub")
	}
	gone := s.addWork("Aposentada", "Zé da Lixeira", "z.epub", "epub")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)

	_, first := s.people(ana, "")
	if first.Total != devicePeoplePage+5 || first.Pages != 2 || len(first.Data) != devicePeoplePage {
		t.Fatalf("first page: total %d pages %d len %d", first.Total, first.Pages, len(first.Data))
	}
	_, second := s.people(ana, "?page=2")
	if second.Page != 2 || len(second.Data) != 5 {
		t.Errorf("second page: %+v", second)
	}
	for _, p := range append(first.Data, second.Data...) {
		if p.Name == "Zé da Lixeira" {
			t.Errorf("an author with only retired works is not listed")
		}
	}
}
