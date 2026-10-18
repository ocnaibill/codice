package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
)

type shelfAnswer struct {
	Works []struct {
		ID         int
		Title      string
		Author     string
		Series     string
		Favorite   bool
		Percent    float64
		InProgress bool
		Authors    []shelfAuthor
		Categories []shelfCategory
		Files      []struct {
			ID     int64
			Format string
		}
	}
	Missing []int
}

func (s *catalogStack) shelf(a actor, ids string) (int, shelfAnswer) {
	s.t.Helper()
	rec := s.do(a, "GET", "/device/shelf?works="+ids, "")
	var out shelfAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestDeviceShelf_GivesWhatAReaderKeepsAboutItsBooksInOneAnswer(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	rosa := s.addWork("O Nome da Rosa", "Umberto Eco", "rosa.epub", "epub")
	fiction := s.makeCategory("Ficção", nil)
	scifi := s.makeCategory("Ficção científica", &fiction)
	s.exec(`INSERT INTO work_categories (work_id, category_id, source) VALUES ($1, $2, 'manual')`, dune, scifi)
	s.exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, idAna, dune)
	s.progress(ana, "PUT", s.primaryFile(dune), `{"locator":{"type":"epub","href":"a"},"percent":42}`)

	code, got := s.shelf(ana, fmt.Sprintf("%d,%d,%d", dune, rosa, 999999))
	if code != 200 {
		t.Fatalf("shelf: %d", code)
	}
	if len(got.Works) != 2 || len(got.Missing) != 1 || got.Missing[0] != 999999 {
		t.Fatalf("two works and one missing: %+v", got)
	}
	d := got.Works[0]
	if d.Title != "Duna" || d.Author != "Frank Herbert" || !d.Favorite || d.Percent != 42 || !d.InProgress {
		t.Errorf("the state of the person: %+v", d)
	}
	if len(d.Authors) != 1 || d.Authors[0].Name != "Frank Herbert" {
		t.Errorf("authors: %+v", d.Authors)
	}
	// the category it is in, and the one above it
	names := map[string]bool{}
	for _, c := range d.Categories {
		names[c.Name] = true
	}
	if len(d.Categories) != 2 || !names["Ficção"] || !names["Ficção científica"] {
		t.Errorf("categories with the ones above: %+v", d.Categories)
	}
	if len(d.Files) != 1 || d.Files[0].Format != "epub" || d.Files[0].ID != s.primaryFile(dune) {
		t.Errorf("files: %+v", d.Files)
	}
	// a work with nothing marked and no category comes with empty lists, never null
	r := got.Works[1]
	if r.Title != "O Nome da Rosa" || r.Favorite || r.Percent != 0 || r.InProgress {
		t.Errorf("a work nothing was done with: %+v", r)
	}
	rec := s.do(ana, "GET", fmt.Sprintf("/device/shelf?works=%d", rosa), "")
	var raw struct {
		Works []map[string]any
	}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if raw.Works[0]["categories"] == nil || raw.Works[0]["authors"] == nil || raw.Works[0]["files"] == nil {
		t.Errorf("lists must not be null: %s", rec.Body.String())
	}
}

func TestDeviceShelf_TheStateIsTheCallersOwn(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, idAna, dune)
	s.progress(ana, "PUT", s.primaryFile(dune), `{"locator":{"type":"epub","href":"a"},"percent":42}`)
	_, got := s.shelf(bob, fmt.Sprint(dune))
	if len(got.Works) != 1 || got.Works[0].Favorite || got.Works[0].Percent != 0 {
		t.Errorf("bob sees his own state, not ana's: %+v", got.Works)
	}
}

func TestDeviceShelf_AskingForNothingOrTooMuchIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	for _, q := range []string{"", "abc", "0,-1"} {
		if code, _ := s.shelf(ana, q); code != 400 {
			t.Errorf("works=%q: %d", q, code)
		}
	}
	ids := ""
	for i := 1; i <= maxShelfWorks+1; i++ {
		ids += fmt.Sprintf("%d,", i)
	}
	if code, _ := s.shelf(ana, ids); code != 400 {
		t.Errorf("too many: %d", code)
	}
	// repeated ids count once
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	if _, got := s.shelf(ana, fmt.Sprintf("%d,%d,%d", dune, dune, dune)); len(got.Works) != 1 {
		t.Errorf("a repeated id is one work: %+v", got.Works)
	}
}

func TestDeviceShelf_AskingByFileGivesTheWorksTheFilesBelongTo(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	rosa := s.addWork("O Nome da Rosa", "Umberto Eco", "rosa.epub", "epub")
	duneFile, rosaFile := s.primaryFile(dune), s.primaryFile(rosa)
	rec := s.do(ana, "GET", fmt.Sprintf("/device/shelf?files=%d,%d,999999", duneFile, rosaFile), "")
	if rec.Code != 200 {
		t.Fatalf("by file: %d %s", rec.Code, rec.Body.String())
	}
	var got shelfAnswer
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Works) != 2 || got.Works[0].Files[0].ID != duneFile || got.Works[1].Files[0].ID != rosaFile {
		t.Errorf("each work comes with the file that was asked: %+v", got.Works)
	}
	// a file and its work asked together are one work
	rec = s.do(ana, "GET", fmt.Sprintf("/device/shelf?works=%d&files=%d", dune, duneFile), "")
	got = shelfAnswer{}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Works) != 1 {
		t.Errorf("one work, not two: %+v", got.Works)
	}
	// only files that are not in the library: nothing to answer, and it is said
	if rec := s.do(ana, "GET", "/device/shelf?files=999999", ""); rec.Code != 400 {
		t.Errorf("unknown file: %d", rec.Code)
	}
}
