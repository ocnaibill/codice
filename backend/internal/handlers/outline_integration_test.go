package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

type outlineBody struct {
	FileID   int64 `json:"fileId"`
	Chapters []struct {
		Title       string          `json:"title"`
		Depth       int             `json:"depth"`
		Part        string          `json:"part"`
		HasChildren bool            `json:"hasChildren"`
		Locator     json.RawMessage `json:"locator"`
		Percent     float64         `json:"percent"`
	} `json:"chapters"`
	Current *int `json:"current"`
}

func (s *catalogStack) outline(a actor, file int64) (int, outlineBody) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/progress/files/%d/outline", file), "")
	var out outlineBody
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// A book in three parts of the outline: a cover, two parts with two chapters and one, and an appendix.
func (s *catalogStack) outlinedEPUB(file int64) {
	s.t.Helper()
	node := func(title string, depth int, part string) map[string]any {
		return map[string]any{"title": title, "depth": depth, "chars": 300, "part": part}
	}
	text := strings.Repeat("x", 100)
	s.indexWithOutline(file,
		[]map[string]any{
			node("Capa", 0, "front"),
			node("Parte I", 0, "body"), node("Capítulo 1", 1, "body"), node("Capítulo 2", 1, "body"),
			node("Parte II", 0, "body"), node("Capítulo 3", 1, "body"),
			node("Apêndice", 0, "back"),
		},
		outlined{text: text, node: 0, locator: `{"type":"epub","href":"cover.xhtml"}`},
		outlined{text: text, node: 2, locator: `{"type":"epub","href":"c1.xhtml","progression":0}`},
		outlined{text: text, node: 2, locator: `{"type":"epub","href":"c1.xhtml","progression":0.5}`},
		outlined{text: text, node: 3, locator: `{"type":"epub","href":"c2.xhtml","progression":0}`},
		outlined{text: text, node: 5, locator: `{"type":"epub","href":"c3.xhtml","progression":0}`},
		outlined{text: text, node: 6, locator: `{"type":"epub","href":"app.xhtml","progression":0}`},
	)
}

func locatorOf(raw json.RawMessage) map[string]any {
	var m map[string]any
	json.Unmarshal(raw, &m)
	return m
}

func TestOutline_IsTheTableOfContentsWithWhereEachOneOpensAndHowFarThroughItStarts(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.outlinedEPUB(epub)

	code, out := s.outline(ana, epub)
	if code != 200 || out.FileID != epub || len(out.Chapters) != 7 {
		t.Fatalf("%d %+v", code, out)
	}
	titles := make([]string, len(out.Chapters))
	for i, c := range out.Chapters {
		titles[i] = c.Title
	}
	if strings.Join(titles, "|") != "Capa|Parte I|Capítulo 1|Capítulo 2|Parte II|Capítulo 3|Apêndice" {
		t.Errorf("in the order of the outline: %v", titles)
	}
	if !out.Chapters[1].HasChildren || out.Chapters[2].HasChildren || !out.Chapters[4].HasChildren || out.Chapters[6].HasChildren {
		t.Errorf("a part has chapters under it, a chapter does not: %+v", out.Chapters)
	}
	if out.Chapters[0].Part != "front" || out.Chapters[2].Part != "body" || out.Chapters[6].Part != "back" || out.Chapters[2].Depth != 1 {
		t.Errorf("the part of the book and the depth are the outline's: %+v", out.Chapters)
	}
	// A part opens where its first chapter does; a chapter at its own first text.
	if h := locatorOf(out.Chapters[1].Locator)["href"]; h != "c1.xhtml" {
		t.Errorf("Parte I opens at its first chapter: %v", h)
	}
	if h := locatorOf(out.Chapters[4].Locator)["href"]; h != "c3.xhtml" {
		t.Errorf("Parte II opens at its first chapter: %v", h)
	}
	if h := locatorOf(out.Chapters[3].Locator)["href"]; h != "c2.xhtml" {
		t.Errorf("Capítulo 2: %v", h)
	}
	// Six segments of 100 characters: the cover is at 0, chapter 1 at 1/6, chapter 2 at 3/6, chapter 3 at 4/6, the appendix at 5/6.
	want := map[int]float64{0: 0, 1: 16.7, 2: 16.7, 3: 50, 4: 66.7, 5: 66.7, 6: 83.3}
	for i, p := range want {
		if out.Chapters[i].Percent != p {
			t.Errorf("%s starts at %v%%, want %v", out.Chapters[i].Title, out.Chapters[i].Percent, p)
		}
	}
	if out.Current != nil {
		t.Errorf("nobody has read it: %v", *out.Current)
	}
}

func TestOutline_SaysTheChapterTheCallersSavedPlaceIsInAndOnlyTheirs(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.outlinedEPUB(epub)

	// Half way through the first chapter's file: the second segment of Capítulo 1.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c1.xhtml","progression":0.6}}`)
	_, out := s.outline(ana, epub)
	if out.Current == nil || *out.Current != 2 {
		t.Fatalf("in Capítulo 1 (the third entry): %v", out.Current)
	}
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c3.xhtml","progression":0.1}}`)
	if _, out := s.outline(ana, epub); out.Current == nil || *out.Current != 5 {
		t.Errorf("in Capítulo 3: %v", out.Current)
	}
	// The first entry counts too: the cover is the entry 0.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"cover.xhtml"}}`)
	if _, out := s.outline(ana, epub); out.Current == nil || *out.Current != 0 {
		t.Errorf("on the cover: %v", out.Current)
	}
	// Reading is personal.
	if _, out := s.outline(bob, epub); out.Current != nil {
		t.Errorf("bob has not read it: %v", *out.Current)
	}
}

func TestOutline_AFileWithNoIndexOrNoOutlineHasAnEmptyListAndNothingIsMadeUp(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	code, out := s.outline(ana, pdf) // never read by the worker
	if code != 200 || out.Chapters == nil || len(out.Chapters) != 0 || out.Current != nil {
		t.Errorf("no index: %d %+v", code, out)
	}
	rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%d/outline", pdf), "")
	if !strings.Contains(rec.Body.String(), `"chapters":[]`) {
		t.Errorf("an empty list, not null: %s", rec.Body.String())
	}
	// Indexed, but the file has no table of contents.
	s.indexWithOutline(epub, nil, outlined{text: strings.Repeat("x", 100), node: 0, locator: `{"type":"epub","href":"a.xhtml"}`})
	if _, out := s.outline(ana, epub); len(out.Chapters) != 0 || out.Current != nil {
		t.Errorf("no outline: %+v", out)
	}
}

func TestOutline_AFileThatIsNotThereIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, _ := s.bookWithTwoFiles()
	if code, _ := s.outline(ana, 999999); code != http.StatusNotFound {
		t.Errorf("unknown file: %d", code)
	}
	if rec := s.do(ana, "GET", "/progress/files/abc/outline", ""); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code, _ := s.outline(ana, epub); code != http.StatusNotFound {
		t.Errorf("a retired work: %d", code)
	}
}

func TestOutline_AnEntryWithNoTextOpensNowhereAndIsNotLentTheNextOnes(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	node := func(title string, depth int) map[string]any {
		return map[string]any{"title": title, "depth": depth, "chars": 100, "part": "body"}
	}
	s.indexWithOutline(epub,
		[]map[string]any{node("Parte vazia", 0), node("Capítulo sem texto", 1), node("Parte com texto", 0), node("Capítulo com texto", 1)},
		outlined{text: strings.Repeat("x", 100), node: 3, locator: `{"type":"epub","href":"c.xhtml","progression":0}`},
	)
	_, out := s.outline(ana, epub)
	if len(out.Chapters) != 4 {
		t.Fatalf("%+v", out)
	}
	if string(out.Chapters[0].Locator) != "null" || string(out.Chapters[1].Locator) != "null" {
		t.Errorf("what holds no text opens nowhere, and does not take the text of the part that follows: %s %s", out.Chapters[0].Locator, out.Chapters[1].Locator)
	}
	if locatorOf(out.Chapters[2].Locator)["href"] != "c.xhtml" || locatorOf(out.Chapters[3].Locator)["href"] != "c.xhtml" {
		t.Errorf("the part with text opens where its chapter does: %+v", out.Chapters)
	}
}

func TestOutline_ASegmentOfAnOutlineThatIsNotTheOneInTheFileIsNotAnEntry(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.outlinedEPUB(epub)
	s.exec(`UPDATE document_segments SET node = 40 WHERE file_id = $1 AND locator->>'href' = 'c3.xhtml'`, epub)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"c3.xhtml","progression":0.1}}`)
	if _, out := s.outline(ana, epub); out.Current != nil {
		t.Errorf("the place is in an entry that the outline does not have: %v", *out.Current)
	}
}

type chaptered struct {
	Data []struct {
		ID      int    `json:"id"`
		Kind    string `json:"kind"`
		Quote   string `json:"quote"`
		Chapter string `json:"chapter"`
	} `json:"data"`
}

func (s *catalogStack) notesWithChapters(a actor, query string) chaptered {
	s.t.Helper()
	rec := s.do(a, "GET", "/notes?"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out chaptered
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestNotes_SayTheChapterTheirPlaceIsInWhenAskedAndTheFileHasAnOutline(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.outlinedEPUB(epub)
	add := func(a actor, file int64, body string) {
		t.Helper()
		if rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), body); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	f := fmt.Sprint(epub)
	add(ana, epub, `{"kind":"highlight","quote":"na capa","fileId":`+f+`,"locator":{"type":"epub","href":"cover.xhtml"}}`)
	add(ana, epub, `{"kind":"highlight","quote":"no meio do um","fileId":`+f+`,"locator":{"type":"epub","href":"c1.xhtml","progression":0.6}}`)
	add(ana, epub, `{"kind":"note","quote":"no três","body":"minha nota","fileId":`+f+`,"locator":{"type":"epub","href":"c3.xhtml","progression":0.2}}`)
	add(ana, pdf, `{"kind":"highlight","quote":"num pdf sem índice","fileId":`+fmt.Sprint(pdf)+`,"locator":{"type":"pdf","page":3}}`)
	add(ana, epub, `{"kind":"highlight","quote":"sem lugar"}`)

	got := map[string]string{}
	for _, n := range s.notesWithChapters(ana, fmt.Sprintf("workId=%d&limit=50&chapters=true", work)).Data {
		got[n.Quote] = n.Chapter
	}
	want := map[string]string{
		"na capa":            "Capa",
		"no meio do um":      "Capítulo 1",
		"no três":            "Capítulo 3",
		"num pdf sem índice": "",
		"sem lugar":          "",
	}
	for q, c := range want {
		if got[q] != c {
			t.Errorf("%q is in %q, want %q", q, got[q], c)
		}
	}
	// A note whose file was taken away keeps its place and has no file to say a chapter of.
	s.exec(`UPDATE notes SET file_id = NULL WHERE quote = 'no três'`)
	for _, n := range s.notesWithChapters(ana, fmt.Sprintf("workId=%d&limit=50&chapters=true", work)).Data {
		if n.Quote == "no três" && n.Chapter != "" {
			t.Errorf("a note with no file has no chapter: %+v", n)
		}
	}
	// Not asked for: not said (the list of every note does not read the outline of every file).
	for _, n := range s.notesWithChapters(ana, fmt.Sprintf("workId=%d&limit=50", work)).Data {
		if n.Chapter != "" {
			t.Errorf("the chapter was not asked for: %+v", n)
		}
	}
	// The notes are the caller's.
	if got := s.notesWithChapters(bob, fmt.Sprintf("workId=%d&limit=50&chapters=true", work)); len(got.Data) != 0 {
		t.Errorf("bob sees ana's notes: %+v", got.Data)
	}
}

func TestNotes_CanBeAskedForByThePersonWhoseWorksTheyAreOn(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	messias := s.addWork("Messias", "Frank Herbert", "b.epub", "epub")
	solaris := s.addWork("Solaris", "Stanisław Lem", "c.epub", "epub")
	s.credit(admin, solaris, "Frank Herbert", "translator") // another role of the same person counts
	note := func(a actor, work int, quote string) {
		t.Helper()
		if rec := s.do(a, "POST", fmt.Sprintf("/works/%d/notes", work), `{"kind":"highlight","quote":"`+quote+`"}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	note(ana, duna, "do Duna")
	note(ana, messias, "do Messias")
	note(ana, solaris, "do Solaris")
	note(bob, duna, "do Duna do Bob")
	other := s.personID("Stanisław Lem")
	frank := s.personID("Frank Herbert")

	quotes := func(a actor, person int) string {
		var out []string
		for _, n := range s.notesWithChapters(a, fmt.Sprintf("personId=%d&limit=50", person)).Data {
			out = append(out, n.Quote)
		}
		sort.Strings(out)
		return strings.Join(out, "|")
	}
	if got := quotes(ana, frank); got != "do Duna|do Messias|do Solaris" {
		t.Errorf("ana, Frank: %s", got)
	}
	if got := quotes(ana, other); got != "do Solaris" {
		t.Errorf("ana, Lem: %s", got)
	}
	// Only the caller's own.
	if got := quotes(bob, frank); got != "do Duna do Bob" {
		t.Errorf("bob: %s", got)
	}
	// What is not a person.
	for _, bad := range []string{"personId=abc", "personId=0", "personId=-3"} {
		if rec := s.do(ana, "GET", "/notes?"+bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
}
