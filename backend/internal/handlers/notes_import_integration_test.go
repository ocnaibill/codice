package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (s *catalogStack) importRequest(a actor, path, file string, mapping map[string]int) *httptest.ResponseRecorder {
	s.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "My Clippings.txt")
	part.Write([]byte(file))
	if mapping != nil {
		raw, _ := json.Marshal(mapping)
		mw.WriteField("mapping", string(raw))
	}
	mw.Close()
	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

const kindleFile = "\ufeffO Nome da Rosa (Eco, Umberto)\n" +
	"- Seu destaque na posição 345-346 | Adicionado: domingo, 4 de outubro de 2026 21:15:30\n\n" +
	"O nome da rosa é um romance de Umberto Eco que passa em uma abadia.\n==========\n" +
	"O Nome da Rosa (Eco, Umberto)\n" +
	"- Sua anotação na posição 346 | Adicionado: domingo, 4 de outubro de 2026 21:16:00\n\n" +
	"Voltar a este trecho.\n==========\n" +
	"O Nome da Rosa (Eco, Umberto)\n" +
	"- Seu destaque na posição 900 | Adicionado: domingo, 4 de outubro de 2026 21:30:00\n\n" +
	"Uma frase que o livro da biblioteca não tem nenhuma palavra dela.\n==========\n" +
	"Livro Que Não Existe (Fulano)\n" +
	"- Seu destaque na posição 10 | Adicionado: domingo, 4 de outubro de 2026 22:00:00\n\n" +
	"texto de outro livro\n==========\n"

func TestNotesImport_AKindleFileComesInAsNotesWithPlacesWhereTheTextIsFound(t *testing.T) {
	s := newCatalogStack(t)
	rosa := s.addWork("O Nome da Rosa", "Umberto Eco", "rosa.epub", "epub")
	s.addWork("O Nome do Vento", "Patrick Rothfuss", "vento.epub", "epub")
	s.index(s.primaryFile(rosa),
		segment{text: "Prólogo sem importância nenhuma.", locator: `{"type":"epub","href":"Text/p.xhtml","progression":0.0}`},
		segment{text: "O nome da rosa é um romance de Umberto Eco, que passa em uma abadia italiana.", locator: `{"type":"epub","href":"Text/c1.xhtml","progression":0.4}`})

	// the preview says which work each book is, and what could not be matched
	rec := s.importRequest(ana, "/notes/import/preview", kindleFile, nil)
	if rec.Code != 200 {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	var preview struct {
		Format  string
		Entries int
		Groups  []ImportGroup
	}
	json.Unmarshal(rec.Body.Bytes(), &preview)
	if preview.Format != "kindle" || len(preview.Groups) != 2 || preview.Entries != 3 {
		t.Fatalf("preview = %+v", preview)
	}
	g := preview.Groups[0]
	if g.Title != "O Nome da Rosa" || g.Highlights != 1 || g.Notes != 1 || len(g.Candidates) == 0 || g.Candidates[0].WorkID != rosa {
		t.Errorf("first group = %+v", g)
	}
	if g.Suggested != rosa {
		t.Errorf("title and author (\"Eco, Umberto\" and \"Umberto Eco\") match: the work is chosen for the person: %d", g.Suggested)
	}
	if other := preview.Groups[1]; other.Suggested != 0 || len(other.Candidates) != 0 {
		t.Errorf("a book the library does not have: %+v", other)
	}
	if s.scalar(`SELECT count(*)::text FROM notes`) != "0" {
		t.Error("the preview wrote something")
	}

	// the import
	mapping := map[string]int{g.Key: rosa}
	rec = s.importRequest(ana, "/notes/import", kindleFile, mapping)
	if rec.Code != 200 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["created"] != float64(2) || out["located"] != float64(1) || out["unlocated"] != float64(1) || out["ignored"] != float64(1) {
		t.Errorf("result = %v", out)
	}
	var kind, quote, body, tags, locator, color, at string
	s.db.QueryRow(`SELECT kind, quote, body, tags::text, COALESCE(locator::text, ''), color, created_at::date::text FROM notes WHERE user_id = $1 AND quote LIKE 'O nome da rosa%'`, idAna).
		Scan(&kind, &quote, &body, &tags, &locator, &color, &at)
	if kind != "note" || body != "Voltar a este trecho." || tags != "{kindle}" || at != "2026-10-04" {
		t.Errorf("the passage with its note: %s %q %q %s %s", kind, quote, body, tags, at)
	}
	if !strings.Contains(locator, `"href": "Text/c1.xhtml"`) || !strings.Contains(locator, `"progression": 0.4`) {
		t.Errorf("the place found in the text of the book: %s", locator)
	}
	if s.scalar(`SELECT count(*)::text FROM notes WHERE locator IS NULL AND file_id IS NOT NULL`) != "1" {
		t.Error("a passage the book does not have is kept with its file and no place")
	}
	// doing it again brings nothing again, and the notes belong to the person who imported
	rec = s.importRequest(ana, "/notes/import", kindleFile, mapping)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["created"] != float64(0) || out["duplicates"] != float64(2) {
		t.Errorf("the second time: %v", out)
	}
	if s.scalar(`SELECT count(*)::text FROM notes WHERE user_id = $1`, idBob) != "0" {
		t.Error("another person has notes")
	}
}

func TestNotesImport_AKOReaderExportPutsEachHighlightAtItsParagraph(t *testing.T) {
	s := newCatalogStack(t)
	work, file, _ := (&kosyncStack{catalogStack: s}).book("As Foices", "foices.epub", "epub")
	export := `{"title":"As Foices","author":"Neal Shusterman","entries":[
		{"sort":"highlight","time":1791900000,"text":"Parágrafo 7 do item 2.","color":"yellow","pn_xp":"/body/DocFragment[2]/body/div/p[7]/text().0"},
		{"sort":"highlight","time":1791900100,"text":"Outro","note":"minha nota","color":"blue","pn_xp":"/body/DocFragment[1]/body/div/p[2]"},
		{"sort":"highlight","time":1791900200,"text":"Sem lugar que se entenda","pn_xp":"/body/p[3]"}]}`
	rec := s.importRequest(ana, "/notes/import/preview", export, nil)
	var preview struct{ Groups []ImportGroup }
	json.Unmarshal(rec.Body.Bytes(), &preview)
	if rec.Code != 200 || len(preview.Groups) != 1 || preview.Groups[0].Suggested != work {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	rec = s.importRequest(ana, "/notes/import", export, map[string]int{preview.Groups[0].Key: work})
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["created"] != float64(3) || out["located"] != float64(2) {
		t.Fatalf("import: %d %v", rec.Code, out)
	}
	var loc, color string
	s.db.QueryRow(`SELECT locator::text, color FROM notes WHERE quote = 'Parágrafo 7 do item 2.'`).Scan(&loc, &color)
	// body is the 2nd child of html, the div its 1st, and the 7th p is the 8th child of the div (the h1 comes first): /16, in the second item
	if !strings.Contains(loc, `epubcfi(/6/4!/4/2/16)`) || color != "sepia" {
		t.Errorf("the paragraph of the xpointer: %s, color %s", loc, color)
	}
	if s.scalar(`SELECT color FROM notes WHERE quote = 'Outro'`) != "indigo" || s.scalar(`SELECT kind FROM notes WHERE quote = 'Outro'`) != "note" {
		t.Error("a highlight with something written on it is a note, and blue is indigo")
	}
	if s.scalar(`SELECT tags::text FROM notes WHERE quote = 'Outro'`) != "{koreader}" || s.scalar(`SELECT file_id::text FROM notes WHERE quote = 'Outro'`) != fmt.Sprint(file) {
		t.Error("origin and file")
	}
}

func TestNotesImport_WhatIsNotAFileOfNotesIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Duna", "Frank Herbert", "dune.epub", "epub")
	for name, content := range map[string]string{"empty": "", "nothing": "hello there", "bad json": "{\"a\":", "json that is not an export": `{"hello":1}`} {
		if rec := s.importRequest(ana, "/notes/import/preview", content, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if rec := s.importRequest(ana, "/notes/import", kindleFile, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("no mapping: %d", rec.Code)
	}
	if rec := s.importRequest(ana, "/notes/import", kindleFile, map[string]int{"x": 99999}); rec.Code != http.StatusBadRequest {
		t.Errorf("a work that is not there: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if rec := s.importRequest(ana, "/notes/import", kindleFile, map[string]int{"x": work}); rec.Code != http.StatusBadRequest {
		t.Errorf("a retired work: %d", rec.Code)
	}
}
