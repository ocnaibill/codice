package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type noteList struct {
	Data  []Note `json:"data"`
	Total int    `json:"total"`
}

func (s *catalogStack) addNote(a actor, work int, body string) (int, int) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), body)
	var out struct{ ID int }
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out.ID
}

func (s *catalogStack) notes(a actor, query string) noteList {
	s.t.Helper()
	var out noteList
	rec := s.do(a, "GET", "/notes"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /notes%s: %d %s", query, rec.Code, rec.Body)
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestNotes_EachKindHasWhatItNeeds(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	_ = epub

	ok := map[string]string{
		"a saved quotation":       `{"quote":"Fear is the mind-killer"}`,
		"a note in own words":     `{"body":"Lembra o **Leviatã**."}`,
		"a note with a quotation": `{"kind":"note","quote":"trecho","body":"minha leitura","tags":["tema"]}`,
		"a bookmark":              fmt.Sprintf(`{"kind":"bookmark","fileId":%d,"locator":{"type":"pdf","page":11}}`, pdf),
	}
	for name, body := range ok {
		if code, _ := s.addNote(ana, work, body); code != http.StatusCreated {
			t.Errorf("%s: %d", name, code)
		}
	}
	bad := map[string]string{
		"nothing at all":           `{}`,
		"only spaces":              `{"quote":"   ","body":"\n"}`,
		"an unknown kind":          `{"kind":"comment","body":"x"}`,
		"a bookmark with no place": `{"kind":"bookmark"}`,
		"a place with no file":     `{"body":"x","locator":{"type":"pdf","page":1}}`,
		"not JSON":                 `nope`,
	}
	for name, body := range bad {
		if code, _ := s.addNote(ana, work, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	kinds := s.scalar(`SELECT string_agg(kind, ',' ORDER BY id) FROM notes`)
	if n := strings.Count(kinds, ",") + 1; n != 4 || !strings.Contains(kinds, "bookmark") {
		t.Errorf("kinds stored: %s", kinds)
	}
}

func TestNotes_APlaceMustBeInAFileOfTheWorkAndFitItsFormat(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	foreign := s.primaryFile(other)

	cases := map[string]string{
		"a locator of another kind":       fmt.Sprintf(`{"body":"x","fileId":%d,"locator":{"type":"epub","href":"a"}}`, pdf),
		"a file of another work":          fmt.Sprintf(`{"body":"x","fileId":%d,"locator":{"type":"epub","href":"a"}}`, foreign),
		"a file that does not exist":      `{"body":"x","fileId":999999,"locator":{"type":"pdf","page":1}}`,
		"a locator with an unknown field": fmt.Sprintf(`{"body":"x","fileId":%d,"locator":{"type":"pdf","page":1,"x":1}}`, pdf),
	}
	for name, body := range cases {
		if code, _ := s.addNote(ana, work, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if n := s.scalar(`SELECT count(*) FROM notes`); n != "0" {
		t.Errorf("refused notes left %s rows", n)
	}

	code, id := s.addNote(ana, work, fmt.Sprintf(`{"body":"vale","fileId":%d,"locator":{"type":"epub","cfi":"epubcfi(/6/2)"}}`, epub))
	if code != http.StatusCreated {
		t.Fatal(code)
	}
	n := s.notes(ana, "").Data[0]
	if n.ID != id || n.FileID == nil || *n.FileID != epub || n.FileFormat != "epub" || !n.FileAvailable ||
		n.LocatorVersion == nil || *n.LocatorVersion != 1 || !strings.Contains(string(n.Locator), "epubcfi(/6/2)") {
		t.Errorf("stored note: %+v", n)
	}
}

func TestNotes_TextIsKeptAsTextAndCleaned(t *testing.T) {
	s := newCatalogStack(t)
	work, _, _ := s.bookWithTwoFiles()

	body := "linha 1\r\nlinha 2\x00\x07 <script>alert(1)</script> [x](javascript:alert(1))\u2028fim"
	raw, _ := json.Marshal(map[string]any{
		"body": body, "quote": strings.Repeat("é", 2500),
		"tags": []string{"  Filosofia  ", "filosofia", "", "dois   espaços", "a\nb"},
	})
	if code, _ := s.addNote(ana, work, string(raw)); code != http.StatusCreated {
		t.Fatal(code)
	}
	n := s.notes(ana, "").Data[0]
	if n.Body != "linha 1\nlinha 2 <script>alert(1)</script> [x](javascript:alert(1))fim" {
		t.Errorf("body = %q: control characters go, CRLF becomes LF, markup stays as typed text", n.Body)
	}
	if got := len([]rune(n.Quote)); got != maxQuote {
		t.Errorf("a long quotation is cut by characters, not bytes: %d", got)
	}
	if strings.Join(n.Tags, "|") != "Filosofia|dois espaços|a b" {
		t.Errorf("tags = %q", n.Tags)
	}

	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("t%d", i)
	}
	limits := map[string]any{
		"more than 20 tags": map[string]any{"body": "x", "tags": tooMany},
		"a tag of 41":       map[string]any{"body": "x", "tags": []string{strings.Repeat("a", 41)}},
		"a body too long":   map[string]any{"body": strings.Repeat("a", maxBody+1)},
	}
	for name, v := range limits {
		raw, _ := json.Marshal(v)
		if code, _ := s.addNote(ana, work, string(raw)); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
}

func TestNotes_NobodyElseSeesEditsDeletesOrExportsThem(t *testing.T) {
	s := newCatalogStack(t)
	work, _, _ := s.bookWithTwoFiles()
	_, anaNote := s.addNote(ana, work, `{"body":"segredo da Ana","tags":["intimo"]}`)
	s.addNote(bob, work, `{"body":"nota do Bob","tags":["intimo"]}`)

	// Lists, filters and searches only ever reach one's own.
	for _, q := range []string{"", "?tag=intimo", "?q=segredo", "?workId=" + fmt.Sprint(work), "?kind=note"} {
		for _, n := range s.notes(bob, q).Data {
			if strings.Contains(n.Body, "Ana") {
				t.Errorf("bob %s saw ana's note: %+v", q, n)
			}
		}
	}
	if got := s.notes(bob, "?q=segredo"); got.Total != 0 || len(got.Data) != 0 {
		t.Errorf("searching for ana's words finds nothing for bob: %+v", got)
	}

	// Another person's note is not found, whether edited or deleted, and stays as it was.
	if rec := s.do(bob, "PATCH", fmt.Sprintf("/notes/%d", anaNote), `{"body":"roubado"}`); rec.Code != 404 {
		t.Errorf("edit: %d", rec.Code)
	}
	if rec := s.do(bob, "DELETE", fmt.Sprintf("/notes/%d", anaNote), ""); rec.Code != 404 {
		t.Errorf("delete: %d", rec.Code)
	}
	if got := s.scalar(`SELECT body FROM notes WHERE id = $1`, anaNote); got != "segredo da Ana" {
		t.Errorf("ana's note is now %q", got)
	}

	for _, format := range []string{"md", "json"} {
		rec := s.do(bob, "GET", "/notes/export?format="+format, "")
		if rec.Code != 200 || strings.Contains(rec.Body.String(), "Ana") || !strings.Contains(rec.Body.String(), "Bob") {
			t.Errorf("bob's %s export: %d %s", format, rec.Code, rec.Body)
		}
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/notes/%d", 999999), `{"body":"x"}`); rec.Code != 404 {
		t.Errorf("no such note: %d", rec.Code)
	}
}

func TestNotes_AreFoundByWorkKindTagAndText(t *testing.T) {
	s := newCatalogStack(t)
	duna, _, _ := s.bookWithTwoFiles()
	outro := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	s.addNote(ana, duna, `{"quote":"Fear is the mind-killer","tags":["Medo","dune"]}`)
	s.addNote(ana, duna, `{"body":"100% de aproveitamento_ok","tags":["medo"]}`)
	s.addNote(ana, outro, `{"body":"o céu era da cor de uma tela","tags":["ciberpunk"]}`)

	count := func(q string) int { return s.notes(ana, q).Total }
	cases := map[string]int{
		"":                             3,
		"?workId=" + fmt.Sprint(duna):  2,
		"?workId=" + fmt.Sprint(outro): 1,
		"?kind=highlight":              1,
		"?kind=note":                   2,
		"?tag=MEDO":                    2, // tags match ignoring case
		"?tag=dune":                    1,
		"?q=mind-killer":               1,
		"?q=neuromancer":               1, // the title kept on the note is searched too
		"?q=CIBERPUNK":                 1, // and the tags, ignoring case
		"?q=%25":                       1, // a % is a character, not "anything"
		"?q=_":                         1, // so is _
		"?q=nada+disso":                0,
		"?workId=" + fmt.Sprint(duna) + "&tag=medo&kind=note": 1,
	}
	for q, want := range cases {
		if got := count(q); got != want {
			t.Errorf("%q: %d notes, want %d", q, got, want)
		}
	}

	page := s.notes(ana, "?limit=2&offset=2")
	if page.Total != 3 || len(page.Data) != 1 {
		t.Errorf("paging keeps the total: %+v", page)
	}
	for _, bad := range []string{"?kind=x", "?workId=abc", "?fileId=-1", "?q=" + strings.Repeat("a", 201)} {
		if rec := s.do(ana, "GET", "/notes"+bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
}

func TestNotes_CanBeEditedButNotLeftEmpty(t *testing.T) {
	s := newCatalogStack(t)
	work, _, pdf := s.bookWithTwoFiles()
	_, id := s.addNote(ana, work, fmt.Sprintf(`{"kind":"note","quote":"trecho","body":"antes","tags":["a"],"fileId":%d,"locator":{"type":"pdf","page":4}}`, pdf))
	before := s.notes(ana, "").Data[0]
	url := fmt.Sprintf("/notes/%d", id)

	if rec := s.do(ana, "PATCH", url, `{"body":"depois","tags":["b","c"]}`); rec.Code != 200 {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	after := s.notes(ana, "").Data[0]
	if after.Body != "depois" || strings.Join(after.Tags, ",") != "b,c" || after.Quote != "trecho" {
		t.Errorf("only what was sent changes: %+v", after)
	}
	if after.Kind != before.Kind || string(after.Locator) != string(before.Locator) || after.FileID == nil || *after.FileID != pdf {
		t.Errorf("kind and place do not move: %+v", after)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) || !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("updatedAt moves, createdAt does not: %v %v", before, after)
	}

	// Taking away everything a note has is refused; taking away one of two parts is fine.
	if rec := s.do(ana, "PATCH", url, `{"quote":"","body":""}`); rec.Code != 400 {
		t.Errorf("an empty note: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", url, `{"quote":""}`); rec.Code != 200 {
		t.Errorf("dropping only the quotation: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", url, `{"tags":[]}`); rec.Code != 200 || len(s.notes(ana, "").Data[0].Tags) != 0 {
		t.Errorf("clearing the tags: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", url, `{"body":"`+strings.Repeat("a", maxBody+1)+`"}`); rec.Code != 400 {
		t.Errorf("too long: %d", rec.Code)
	}
	if got := s.scalar(`SELECT body FROM notes WHERE id = $1`, id); got != "depois" {
		t.Errorf("refused edits changed the note: %q", got)
	}

	// A bookmark has no words, and may stay that way.
	_, mark := s.addNote(ana, work, fmt.Sprintf(`{"kind":"bookmark","fileId":%d,"locator":{"type":"pdf","page":1}}`, pdf))
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/notes/%d", mark), `{"body":""}`); rec.Code != 200 {
		t.Errorf("editing a bookmark: %d", rec.Code)
	}
}

func TestNotes_SurviveTheWorkAndExportWithTheirReference(t *testing.T) {
	s := newCatalogStack(t)
	work, _, pdf := s.bookWithTwoFiles()
	s.addNote(ana, work, fmt.Sprintf(`{"kind":"highlight","quote":"Fear is the mind-killer.","tags":["medo","dune"],"fileId":%d,"locator":{"type":"pdf","page":11,"label":"xii"}}`, pdf))
	s.addNote(ana, work, `{"body":"Lembra o Leviatã.\nMais uma linha."}`)

	// The work is gone, files and all. The person's notes are not.
	s.exec(`DELETE FROM works WHERE id = $1`, work)
	got := s.notes(ana, "").Data
	if len(got) != 2 || got[0].SourceAvailable || got[0].FileAvailable || got[0].WorkID != nil || got[0].FileID != nil ||
		got[0].WorkTitle != "Duna" || got[0].WorkAuthor != "Frank Herbert" {
		t.Fatalf("notes after the work is gone: %+v", got)
	}
	if !strings.Contains(string(got[1].Locator), `"page":11`) {
		t.Errorf("the address is kept even when the file is not: %s", got[1].Locator)
	}

	rec := s.do(ana, "GET", "/notes/export", "")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/markdown") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "attachment; filename=\"codice-anotacoes-") {
		t.Fatalf("export headers: %d %v", rec.Code, rec.Header())
	}
	md := rec.Body.String()
	for _, want := range []string{
		"# Anotações do Códice", "2 anotações",
		"## Duna", "*Frank Herbert*", "não está mais no acervo",
		"### Destaque · ", "### Nota · ",
		"> Fear is the mind-killer.",
		"Lembra o Leviatã.\nMais uma linha.",
		"Tags: #medo #dune",
		"<!-- codice:note id=",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the export lacks %q:\n%s", want, md)
		}
	}
	// The place is said in words and kept exactly.
	if !strings.Contains(md, "página xii (12 do arquivo)") || !strings.Contains(md, `"page":11`) {
		t.Errorf("the place is missing:\n%s", md)
	}
}

func TestNotes_ExportInJSONAndByFilter(t *testing.T) {
	s := newCatalogStack(t)
	duna, _, _ := s.bookWithTwoFiles()
	outro := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	s.addNote(ana, duna, `{"body":"sobre Duna","tags":["x"]}`)
	s.addNote(ana, outro, `{"body":"sobre Neuromancer"}`)

	rec := s.do(ana, "GET", "/notes/export?format=json&workId="+fmt.Sprint(outro), "")
	var out struct {
		Count int
		Notes []Note
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Count != 1 || out.Notes[0].Body != "sobre Neuromancer" {
		t.Errorf("filtered JSON export: %d %s", rec.Code, rec.Body)
	}
	if !strings.HasSuffix(rec.Header().Get("Content-Disposition"), `.json"`) {
		t.Errorf("filename: %q", rec.Header().Get("Content-Disposition"))
	}
	if rec := s.do(ana, "GET", "/notes/export?format=pdf", ""); rec.Code != 400 {
		t.Errorf("an unknown format: %d", rec.Code)
	}
	if md := s.do(ana, "GET", "/notes/export?workId="+fmt.Sprint(outro), "").Body.String(); !strings.Contains(md, "· 1 anotação\n") {
		t.Errorf("the singular: %s", md)
	}
	if rec := s.do(ana, "GET", "/notes/export?tag=nao-existe", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "0 anotações") {
		t.Errorf("nothing to export is an empty document, not an error: %d %s", rec.Code, rec.Body)
	}
}

func TestNotes_AMarkdownExportCannotBeBrokenOutOfItsComment(t *testing.T) {
	s := newCatalogStack(t)
	work, _, pdf := s.bookWithTwoFiles()
	// The locator is validated, so it cannot carry a "-->"; the title on the note can carry
	// markdown-looking text, which stays one line under its heading.
	s.exec(`UPDATE works SET original_title = E'Duna\n# Injetado' WHERE id = $1`, work)
	s.addNote(ana, work, fmt.Sprintf(`{"body":"x","fileId":%d,"locator":{"type":"pdf","page":0,"label":"--> <b>"}}`, pdf))

	md := s.do(ana, "GET", "/notes/export", "").Body.String()
	if strings.Contains(md, "\n# Injetado") {
		t.Errorf("a title must stay on its heading line:\n%s", md)
	}
	// What is said in words carries no markup, and nothing inside the comment can end it early.
	start := strings.Index(md, "<!-- codice:note")
	if text := md[:start]; strings.Contains(text, "<b>") || !strings.Contains(text, "b (1 do arquivo)") {
		t.Errorf("the label went into the text with its markup:\n%s", text)
	}
	inner := md[start+len("<!--") : strings.LastIndex(md, "-->")]
	if strings.Contains(inner, "--") {
		t.Errorf("the comment could be closed from inside: %q", inner)
	}
}
