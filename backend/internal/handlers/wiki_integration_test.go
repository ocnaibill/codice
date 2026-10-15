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

// The wiki of one book of three chapters in two parts: Part I is chapters 1 and 2, Part II is chapter 3. Everything that only
// chapter 3 may show carries the word SEGREDO, so a response can be checked for leaks by looking for it (RF-051).
const wikiSHA = "abababababababababababababababababababababababababababababababab"

func wikiBundle(sha string) string {
	return `{
	"format": 1, "generated_at": "2026-10-15T12:00:00Z", "provider": "deepseek", "model": "deepseek-v4-flash", "remote": true,
	"books": [{"position": 0, "title": "O Livro", "side": false, "sha256": "` + sha + `", "chapters": [
		{"chapter": 1, "title": "Um", "part": "Parte I", "href": "OEBPS/Text/c1.xhtml"},
		{"chapter": 2, "title": "Dois", "part": "Parte I", "href": "OEBPS/Text/c2.xhtml"},
		{"chapter": 3, "title": "SEGREDO do título", "part": "Parte II", "href": "OEBPS/Text/c3.xhtml"},
		{"chapter": 4, "title": "Outros livros", "part": "Final", "href": "", "editorial": true}]}],
	"entities": [
		{"slug": "ana-maria", "name": "Ana Maria", "kind": "person", "page": true,
		 "appearances": [{"book": 0, "chapter": 1}, {"book": 0, "chapter": 2}, {"book": 0, "chapter": 3}],
		 "aliases": [{"alias": "Aninha", "book": 0, "chapter": 1}, {"alias": "SEGREDO a Rainha", "book": 0, "chapter": 3}],
		 "states": [{"book": 0, "chapter": 1, "text": "chega à cidade"}, {"book": 0, "chapter": 2, "text": "aprende o ofício"},
		            {"book": 0, "chapter": 3, "text": "SEGREDO é coroada"}],
		 "sections": [
			{"kind": "who", "book": 0, "chapter_from": 1, "chapter_to": 2, "part": "Parte I",
			 "sentences": [{"paragraph": 1, "text": "Ana Maria é uma aprendiz.", "refs": [{"book": 0, "chapter": 1, "fact": "chega", "quote": "Ana Maria chegou à cidade numa manhã fria de inverno"}]},
			               {"paragraph": 1, "text": "SEGREDO: a identidade que só o capítulo 3 revela.", "refs": [{"book": 0, "chapter": 3, "fact": "SEGREDO"}]}]},
			{"kind": "who", "book": 0, "chapter_from": 3, "chapter_to": 3, "part": "Parte II",
			 "sentences": [{"paragraph": 1, "text": "Ana Maria é a SEGREDO rainha.", "refs": [{"book": 0, "chapter": 3, "fact": "coroada"}]}]},
			{"kind": "bio", "book": 0, "chapter_from": 1, "chapter_to": 2, "part": "Parte I",
			 "sentences": [{"paragraph": 1, "text": "Chegou à cidade e aprendeu o ofício.", "refs": [{"book": 0, "chapter": 1, "fact": "chega"}, {"book": 0, "chapter": 2, "fact": "aprende"}]}]},
			{"kind": "bio", "book": 0, "chapter_from": 3, "chapter_to": 3, "part": "Parte II",
			 "sentences": [{"paragraph": 1, "text": "Foi coroada, SEGREDO.", "refs": [{"book": 0, "chapter": 3, "fact": "SEGREDO coroação"}]}]},
			{"kind": "personality",
			 "sentences": [{"paragraph": 1, "text": "É paciente.", "refs": [{"book": 0, "chapter": 2, "fact": "paciência"}]},
			               {"paragraph": 1, "text": "É SEGREDO implacável.", "refs": [{"book": 0, "chapter": 3, "fact": "SEGREDO"}]}]}],
		 "relations": [{"other": "mestre", "name": "Mestre", "group": "mentors", "role": "mestre", "book": 0, "chapter": 1},
		               {"other": "vilao", "name": "Vilão", "group": "enemies", "role": "SEGREDO inimigo", "book": 0, "chapter": 3}]},
		{"slug": "mestre", "name": "Mestre", "kind": "person", "page": false, "appearances": [{"book": 0, "chapter": 1}]},
		{"slug": "vilao", "name": "Vilão SEGREDO", "kind": "person", "page": true, "appearances": [{"book": 0, "chapter": 3}],
		 "sections": [{"kind": "bio", "book": 0, "chapter_from": 3, "chapter_to": 3, "part": "Parte II",
		   "sentences": [{"paragraph": 1, "text": "SEGREDO trama.", "refs": [{"book": 0, "chapter": 3, "fact": "trama"}]}]}]}],
	"edges": [{"a": "ana-maria", "b": "mestre", "label": "aprendiz de", "book": 0, "chapter": 1},
	          {"a": "ana-maria", "b": "vilao", "label": "SEGREDO inimiga", "book": 0, "chapter": 3}],
	"suggestions": [{"kind": "merge", "entity": "ana-maria", "other": "vilao", "evidence": "SEGREDO", "book": 0, "chapter": 3}]
}`
}

func (s *catalogStack) wikiBook() (int, int64) {
	s.t.Helper()
	work := s.addWork("O Livro", "Autora", "livro.epub", "epub")
	file := s.primaryFile(work)
	s.exec(`UPDATE files SET sha256 = $1 WHERE id = $2`, wikiSHA, file)
	s.index(file,
		segment{text: "Ana Maria chegou à cidade numa manhã fria de inverno, com uma mala só.", locator: `{"type":"epub","href":"Text/c1.xhtml","progression":0}`},
		segment{text: "Ela procurou o Mestre na oficina.", locator: `{"type":"epub","href":"Text/c1.xhtml","progression":0.5}`},
		segment{text: "Ana aprendeu o ofício.", locator: `{"type":"epub","href":"Text/c2.xhtml","progression":0}`},
		segment{text: "A coroação.", locator: `{"type":"epub","href":"Text/c3.xhtml","progression":0}`},
	)
	return work, file
}

func (s *catalogStack) importWiki(a actor, bundle string) *httptest.ResponseRecorder {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("bundle", "wiki.json")
	fw.Write([]byte(bundle))
	mw.Close()
	req := httptest.NewRequest("POST", "/admin/wiki/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

type wikiSummary struct {
	Wiki *struct {
		Project struct {
			ID       int64  `json:"id"`
			Provider string `json:"provider"`
			Remote   bool   `json:"remote"`
		} `json:"project"`
		Cursor struct {
			Book    int    `json:"book"`
			Chapter int    `json:"chapter"`
			Source  string `json:"source"`
		} `json:"cursor"`
		Index []struct {
			Slug string `json:"slug"`
		} `json:"index"`
		Books []struct {
			Chapters []struct {
				Chapter int    `json:"chapter"`
				Title   string `json:"title"`
			} `json:"chapters"`
		} `json:"books"`
	} `json:"wiki"`
}

func (s *catalogStack) wikiOf(a actor, work int, query string) (wikiSummary, string) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/works/%d/wiki%s", work, query), "")
	if rec.Code != 200 {
		s.t.Fatalf("GET wiki: %d %s", rec.Code, rec.Body.String())
	}
	var out wikiSummary
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Body.String()
}

func readTo(s *catalogStack, a actor, file int64, href string) {
	s.t.Helper()
	if code, _ := s.progress(a, "PUT", file, `{"locator":{"type":"epub","href":"`+href+`","progression":0.1}}`); code != 200 {
		s.t.Fatalf("progress: %d", code)
	}
}

func TestWiki_AReaderSeesNothingBeyondTheChapterTheyAreIn(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.wikiBook()
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	readTo(s, ana, file, "Text/c2.xhtml")

	sum, raw := s.wikiOf(ana, work, "")
	if sum.Wiki == nil || sum.Wiki.Cursor.Book != 0 || sum.Wiki.Cursor.Chapter != 2 || sum.Wiki.Cursor.Source != "progress" {
		t.Fatalf("cursor = %s", raw)
	}
	if strings.Contains(raw, "SEGREDO") || strings.Contains(raw, "vilao") {
		t.Errorf("the summary leaks what lies beyond chapter 2: %s", raw)
	}
	if !sum.Wiki.Project.Remote || sum.Wiki.Project.Provider != "deepseek" {
		t.Errorf("project = %+v", sum.Wiki.Project)
	}
	// The cursor control lists the chapters, titled only up to the reader's place; the editorial one is not a chapter to read.
	chs := sum.Wiki.Books[0].Chapters
	if len(chs) != 3 || chs[1].Title != "Dois" || chs[2].Title != "" {
		t.Errorf("chapters = %+v", chs)
	}

	rec := s.do(ana, "GET", fmt.Sprintf("/wiki/%d/entities/ana-maria", sum.Wiki.Project.ID), "")
	page := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("page: %d %s", rec.Code, page)
	}
	if strings.Contains(page, "SEGREDO") {
		t.Errorf("the page leaks what lies beyond chapter 2: %s", page)
	}
	for _, want := range []string{"Ana Maria é uma aprendiz.", "Chegou à cidade e aprendeu o ofício.", "É paciente.", "Aninha", "aprende o ofício", `"group":"mentors"`, `"slug":"mestre"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q: %s", want, page)
		}
	}
	// The quote of a reference is found in the file, for opening the reader at it.
	if !strings.Contains(page, `"sequence":0`) || !strings.Contains(page, `"locator":{`) {
		t.Errorf("the quote's segment or its locator is missing: %s", page)
	}
	// An entity that has not appeared yet is not found at all, and the graph does not have it.
	if rec := s.do(ana, "GET", fmt.Sprintf("/wiki/%d/entities/vilao", sum.Wiki.Project.ID), ""); rec.Code != 404 {
		t.Errorf("an entity of chapter 3 = %d", rec.Code)
	}
	rec = s.do(ana, "GET", fmt.Sprintf("/wiki/%d/graph", sum.Wiki.Project.ID), "")
	if strings.Contains(rec.Body.String(), "SEGREDO") || strings.Contains(rec.Body.String(), "vilao") {
		t.Errorf("the graph leaks: %s", rec.Body.String())
	}

	// Choosing to go further shows it, and changes nothing in the progress.
	rec = s.do(ana, "GET", fmt.Sprintf("/wiki/%d/entities/ana-maria?book=0&chapter=3", sum.Wiki.Project.ID), "")
	if !strings.Contains(rec.Body.String(), "Ana Maria é a SEGREDO rainha.") || !strings.Contains(rec.Body.String(), `"source":"manual"`) {
		t.Errorf("a manual cursor at chapter 3 = %s", rec.Body.String())
	}
	if again, _ := s.wikiOf(ana, work, ""); again.Wiki.Cursor.Chapter != 2 {
		t.Errorf("the manual cursor moved the progress: %+v", again.Wiki.Cursor)
	}
}

func TestWiki_InsideAPartTheChaptersShowOneByOneAndTheBiographyWaits(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.wikiBook()
	s.importWiki(admin, wikiBundle(wikiSHA))
	readTo(s, ana, file, "Text/c1.xhtml")
	sum, _ := s.wikiOf(ana, work, "")
	var out struct {
		Page struct {
			Who        *json.RawMessage `json:"who"`
			Biography  []any            `json:"biography"`
			InProgress *struct {
				Part   string `json:"part"`
				States []struct {
					Chapter int `json:"chapter"`
				} `json:"states"`
			} `json:"inProgress"`
			Hidden *struct {
				ChapterFrom int `json:"chapterFrom"`
				ChapterTo   int `json:"chapterTo"`
			} `json:"hidden"`
		} `json:"page"`
	}
	rec := s.do(ana, "GET", fmt.Sprintf("/wiki/%d/entities/ana-maria", sum.Wiki.Project.ID), "")
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Page.Who != nil || len(out.Page.Biography) != 0 {
		t.Errorf("Part I is not read whole yet: %s", rec.Body.String())
	}
	if out.Page.InProgress == nil || out.Page.InProgress.Part != "Parte I" || len(out.Page.InProgress.States) != 1 || out.Page.InProgress.States[0].Chapter != 1 {
		t.Errorf("in progress = %s", rec.Body.String())
	}
	if out.Page.Hidden == nil || out.Page.Hidden.ChapterFrom != 2 || out.Page.Hidden.ChapterTo != 3 {
		t.Errorf("hidden = %s", rec.Body.String())
	}
}

func TestWiki_SomeoneWhoHasNotStartedSeesTheBooksButNoEntities(t *testing.T) {
	s := newCatalogStack(t)
	work, _ := s.wikiBook()
	s.importWiki(admin, wikiBundle(wikiSHA))
	sum, raw := s.wikiOf(bob, work, "")
	if sum.Wiki == nil || sum.Wiki.Cursor.Source != "none" || len(sum.Wiki.Index) != 0 {
		t.Errorf("bob = %s", raw)
	}
	if strings.Contains(raw, "Ana Maria") {
		t.Errorf("leak: %s", raw)
	}
}

func TestWiki_SuggestionsAreForStaffOnly(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.wikiBook()
	s.importWiki(admin, wikiBundle(wikiSHA))
	readTo(s, ana, file, "Text/c3.xhtml")
	readTo(s, admin, file, "Text/c3.xhtml")
	sum, _ := s.wikiOf(ana, work, "")
	path := fmt.Sprintf("/wiki/%d/entities/ana-maria", sum.Wiki.Project.ID)
	if strings.Contains(s.do(ana, "GET", path, "").Body.String(), `"suggestions"`) {
		t.Error("a reader sees the identity suggestions")
	}
	if !strings.Contains(s.do(admin, "GET", path, "").Body.String(), `"kind":"merge"`) {
		t.Error("an admin does not see the identity suggestion")
	}
}

func TestWiki_ImportKeepsWhatAnAdminDecidedAndRejectsWhatIsNotInTheLibrary(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.wikiBook()
	s.importWiki(admin, wikiBundle(wikiSHA))
	s.exec(`UPDATE wiki_sentences SET hidden_at = now(), hidden_by = $1 WHERE text = 'É paciente.'`, idAdmin)
	s.exec(`UPDATE wiki_suggestions SET state = 'declined', decided_by = $1, decided_at = now()`, idAdmin)
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("re-import: %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT count(*) FROM wiki_projects`); n != "1" {
		t.Errorf("projects = %s, want the old one replaced", n)
	}
	if h := s.scalar(`SELECT count(*) FROM wiki_sentences WHERE text = 'É paciente.' AND hidden_at IS NOT NULL`); h != "1" {
		t.Errorf("the sentence marked wrong came back")
	}
	if st := s.scalar(`SELECT state FROM wiki_suggestions`); st != "declined" {
		t.Errorf("the decided suggestion is %s again", st)
	}
	readTo(s, ana, file, "Text/c2.xhtml")
	sum, _ := s.wikiOf(ana, work, "")
	if page := s.do(ana, "GET", fmt.Sprintf("/wiki/%d/entities/ana-maria", sum.Wiki.Project.ID), "").Body.String(); strings.Contains(page, "É paciente.") {
		t.Errorf("a hidden sentence is shown: %s", page)
	}

	rec := s.importWiki(admin, wikiBundle(strings.Repeat("c", 64)))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "não está no acervo") {
		t.Errorf("a book not in the library = %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.importWiki(admin, `{"format": 1}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an incomplete bundle = %d %s", rec.Code, rec.Body.String())
	}
	if n := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'wiki.import'`); n != "2" {
		t.Errorf("audit entries = %s", n)
	}
}

// withUpdate is the bundle of a later generation: the same wiki, a new entity with a page, and a section that Ana Maria did not have.
func withUpdate(t *testing.T, bundle string) string {
	t.Helper()
	var b map[string]any
	if err := json.Unmarshal([]byte(bundle), &b); err != nil {
		t.Fatal(err)
	}
	ents := b["entities"].([]any)
	for _, e := range ents {
		m := e.(map[string]any)
		if m["slug"] == "ana-maria" {
			m["sections"] = append(m["sections"].([]any), map[string]any{
				"kind": "bio", "book": 0, "chapter_from": 2, "chapter_to": 2, "part": "Parte Nova",
				"sentences": []any{map[string]any{"paragraph": 1, "text": "Ana Maria soube de uma frase nova da atualização.",
					"refs": []any{map[string]any{"book": 0, "chapter": 2, "fact": "nova"}}}},
			})
		}
	}
	b["entities"] = append(ents, map[string]any{
		"slug": "recem-chegado", "name": "Recém-chegado", "kind": "person", "page": true,
		"appearances": []any{map[string]any{"book": 0, "chapter": 2}},
		"sections": []any{map[string]any{"kind": "who", "book": 0, "chapter_from": 1, "chapter_to": 2, "part": "Parte I",
			"sentences": []any{map[string]any{"paragraph": 1, "text": "O Recém-chegado chegou com a atualização.",
				"refs": []any{map[string]any{"book": 0, "chapter": 2, "fact": "chegou"}}}}}},
	})
	out, _ := json.Marshal(b)
	return string(out)
}

func TestWikiReview_AnUpdateListsWhatItBroughtAndTheStaffApprovesOrUndoesIt(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.wikiBook()
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("first import: %d", rec.Code)
	}
	var id int64
	s.db.QueryRow(`SELECT id FROM wiki_projects WHERE work_id = $1`, work).Scan(&id)
	if rec := s.do(admin, "GET", fmt.Sprintf("/wiki/%d/review", id), ""); !strings.Contains(rec.Body.String(), `"changes":[]`) {
		t.Errorf("a first generation has nothing to review: %s", rec.Body.String())
	}

	if rec := s.importWiki(admin, withUpdate(t, wikiBundle(wikiSHA))); rec.Code != 201 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	s.db.QueryRow(`SELECT id FROM wiki_projects WHERE work_id = $1`, work).Scan(&id)
	s.exec(`UPDATE wiki_projects SET review = 'pending' WHERE id = $1`, id) // what the worker's import does for a new book
	rec := s.do(admin, "GET", fmt.Sprintf("/wiki/%d/review", id), "")
	var rev struct {
		Review  string `json:"review"`
		Changes []struct {
			Slug   string   `json:"slug"`
			New    bool     `json:"new"`
			Page   bool     `json:"page"`
			Labels []string `json:"labels"`
			Undone bool     `json:"undone"`
		} `json:"changes"`
	}
	json.Unmarshal(rec.Body.Bytes(), &rev)
	if rev.Review != "pending" || len(rev.Changes) != 2 || rev.Changes[0].Slug != "recem-chegado" || !rev.Changes[0].New ||
		rev.Changes[1].Slug != "ana-maria" || rev.Changes[1].New || len(rev.Changes[1].Labels) != 1 || !strings.Contains(rev.Changes[1].Labels[0], "Parte Nova") {
		t.Fatalf("review = %s", rec.Body.String())
	}

	// Ana, who is at chapter 2, sees the new sentence until the staff undoes what the entity gained.
	readTo(s, ana, file, "Text/c2.xhtml")
	readTo(s, admin, file, "Text/c2.xhtml") // the staff is held to their own point of reading too (DEC-179)
	page := func(a actor) string {
		return s.do(a, "GET", fmt.Sprintf("/wiki/%d/entities/ana-maria", id), "").Body.String()
	}
	if !strings.Contains(page(ana), "frase nova da atualização") {
		t.Fatalf("the update is not shown: %s", page(ana))
	}
	rec = s.do(admin, "POST", fmt.Sprintf("/wiki/%d/review/undo", id), `{"slug":"ana-maria"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"hidden":1`) {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(page(ana), "frase nova da atualização") {
		t.Error("a reader still sees what the staff undid")
	}
	if got := page(admin); !strings.Contains(got, "frase nova da atualização") || !strings.Contains(got, `"hidden":true`) {
		t.Errorf("the staff does not see it struck through: %s", got)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/wiki/%d/review/undo", id), `{"slug":"nao-existe"}`); rec.Code != 404 {
		t.Errorf("undo of an entity that did not change: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/wiki/%d/review/approve", id), ""); rec.Code != 204 {
		t.Fatalf("approve: %d", rec.Code)
	}
	var state string
	s.db.QueryRow(`SELECT review FROM wiki_projects WHERE id = $1`, id).Scan(&state)
	if state != "none" {
		t.Errorf("review after approving = %s", state)
	}
	// What was hidden stays hidden through a generation that writes the same text.
	if rec := s.importWiki(admin, withUpdate(t, wikiBundle(wikiSHA))); rec.Code != 201 {
		t.Fatalf("regenerate: %d", rec.Code)
	}
	s.db.QueryRow(`SELECT id FROM wiki_projects WHERE work_id = $1`, work).Scan(&id)
	if strings.Contains(page(ana), "frase nova da atualização") {
		t.Error("a generation brought back a sentence the staff hid")
	}
}

func TestWikiSentences_TheStaffMarksOneAsWrongAndItGoesForEveryReaderAndComesBack(t *testing.T) {
	s := newCatalogStack(t)
	work, file := s.wikiBook()
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("import: %d", rec.Code)
	}
	var id, sentence int64
	s.db.QueryRow(`SELECT id FROM wiki_projects WHERE work_id = $1`, work).Scan(&id)
	s.db.QueryRow(`SELECT s.id FROM wiki_sentences s WHERE s.text = 'Ana Maria é uma aprendiz.'`).Scan(&sentence)
	readTo(s, ana, file, "Text/c2.xhtml")
	readTo(s, admin, file, "Text/c2.xhtml")
	body := func(a actor) string {
		return s.do(a, "GET", fmt.Sprintf("/wiki/%d/entities/ana-maria", id), "").Body.String()
	}
	url := fmt.Sprintf("/wiki/%d/sentences/%d/hidden", id, sentence)
	if !strings.Contains(body(ana), "Ana Maria é uma aprendiz.") {
		t.Fatal("the sentence is not there to begin with")
	}
	if rec := s.do(admin, "PUT", url, `{"hidden":true}`); rec.Code != 204 {
		t.Fatalf("hide: %d", rec.Code)
	}
	if strings.Contains(body(ana), "Ana Maria é uma aprendiz.") {
		t.Error("a reader still gets a sentence marked wrong")
	}
	if got := body(admin); !strings.Contains(got, "Ana Maria é uma aprendiz.") || !strings.Contains(got, `"hidden":true`) {
		t.Errorf("the staff does not get it flagged: %s", got)
	}
	// A sentence of another wiki is not found through this one.
	if rec := s.do(admin, "PUT", fmt.Sprintf("/wiki/%d/sentences/999999/hidden", id), `{"hidden":true}`); rec.Code != 404 {
		t.Errorf("an unknown sentence: %d", rec.Code)
	}
	if rec := s.do(admin, "PUT", url, `{"hidden":false}`); rec.Code != 204 || !strings.Contains(body(ana), "Ana Maria é uma aprendiz.") {
		t.Errorf("showing it again: %d", rec.Code)
	}
}
