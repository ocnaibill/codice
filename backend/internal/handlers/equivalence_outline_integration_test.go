package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The outline of a file (DEC-087) as the worker publishes it, and the segments that know their node.
type outlined struct {
	text, locator string
	node          int
}

func (s *catalogStack) indexWithOutline(file int64, nodes []map[string]any, segs ...outlined) {
	s.t.Helper()
	var gen int
	if err := s.db.QueryRow(`SELECT text_extraction_begin($1)`, file).Scan(&gen); err != nil {
		s.t.Fatal(err)
	}
	for i, sg := range segs {
		s.exec(`INSERT INTO document_segments (file_id, generation, sequence, text, locator, locator_version, node)
		        VALUES ($1, $2, $3, $4, $5::jsonb, 1, $6)`, file, gen, i, sg.text, sg.locator, sg.node)
	}
	structure, _ := json.Marshal(nodes)
	if _, err := s.db.Exec(`SELECT text_extraction_publish($1, $2, 2, 'abc', 'ready', 'native', 'pt', $3::jsonb)`, file, gen, string(structure)); err != nil {
		s.t.Fatal(err)
	}
}

// filler is text that shares nothing with any other filler (each seed has words of its own).
func filler(seed int) string {
	var w []string
	for i := 0; i < 90; i++ {
		w = append(w, fmt.Sprintf("palavra%dx%d", seed, i))
	}
	return strings.Join(w, " ")
}

func chapterNodes(title string, n int, extra ...map[string]any) []map[string]any {
	var nodes []map[string]any
	for i := 1; i <= n; i++ {
		nodes = append(nodes, map[string]any{"title": fmt.Sprintf("%s %d", title, i), "depth": 0, "chars": 800, "part": "body"})
	}
	return append(nodes, extra...)
}

const (
	outlineNamesPT = "a reunião foi feita por Barbicane com Nicholl e Ardan perto de Baltimore depois que Maston falou em 1866 e de novo em 1867 diante de todo o clube"
	outlineNamesEN = "the meeting was held by Barbicane with Nicholl and Ardan near Baltimore after Maston spoke in 1866 and again in 1867 before the whole club"
)

// An EPUB that holds all its chapters in one file (the reader's place in it is the progression) and a
// PDF with bookmarks, with an appendix that names everyone.
func (s *catalogStack) outlinedBook(sourceName, destName string) (src, dst int64) {
	work, epub, pdf := s.bookWithTwoFiles()
	_ = work
	var source, dest []outlined
	for i := 0; i < 5; i++ {
		text := filler(i)
		destText := filler(100 + i)
		if i == 2 {
			text += " " + outlineNamesPT
			destText += " " + outlineNamesEN
		}
		source = append(source, outlined{text: text, node: i, locator: fmt.Sprintf(`{"type":"epub","href":"all.xhtml","progression":%v}`, float64(i)/5)})
		dest = append(dest, outlined{text: destText, node: i, locator: fmt.Sprintf(`{"type":"pdf","page":%d}`, i*10)})
	}
	dest = append(dest, outlined{text: "Appendix: who is who. Barbicane, Nicholl, Ardan, Baltimore, Maston, 1866, 1867 and many others " + filler(999), node: 5, locator: `{"type":"pdf","page":90}`})
	s.indexWithOutline(epub, chapterNodes(sourceName, 5), source...)
	s.indexWithOutline(pdf, chapterNodes(destName, 5, map[string]any{"title": "Appendix", "depth": 0, "chars": 900, "part": "back"}), dest...)
	return epub, pdf
}

func TestEquivalenceOutline_TheChapterInsideAFileOfManyIsFoundByTheProgressionAndAnsweredInsideTheAlignedChapter(t *testing.T) {
	s := newCatalogStack(t)
	epub, pdf := s.outlinedBook("Capítulo", "Chapter")
	// The reader says "all.xhtml, half way": that is the third chapter, not the start of the file.
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"all.xhtml","progression":0.5}}`)
	code, r := s.equivalent(ana, pdf, epub)
	if code != 200 || r.Status != "found" || len(r.Candidates) != 1 {
		t.Fatalf("%d %+v", code, r)
	}
	c := r.Candidates[0]
	var loc map[string]any
	json.Unmarshal(c.Locator, &loc)
	if c.Method != "anchors" || loc["page"] != float64(20) || c.Confidence != "high" || c.Evidence["reverse"] != "agrees" {
		t.Errorf("the names are in the third chapter (page 20), never in the appendix, and the outline confirms them: %+v", c)
	}
}

func TestEquivalenceOutline_WhenOnlyALargeStructuralUnitMatchesNoPlaceIsOffered(t *testing.T) {
	s := newCatalogStack(t)
	epub, pdf := s.outlinedBook("Capítulo", "Chapter")
	s.exec(`UPDATE document_segments SET text = $1 WHERE file_id = $2`, filler(2), epub) // no names anywhere
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"all.xhtml","progression":0.5}}`)
	_, r := s.equivalent(ana, pdf, epub)
	if r.Status != "not_found" || len(r.Candidates) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestEquivalenceOutline_APositionInTheBackOfTheBookIsNotMatchedWithAChapter(t *testing.T) {
	s := newCatalogStack(t)
	epub, pdf := s.outlinedBook("Capítulo", "Chapter")
	// Put the source's own place in a back-matter node too.
	s.exec(`UPDATE text_extractions SET structure = jsonb_set(structure, '{4,part}', '"back"') WHERE file_id = $1`, epub)
	s.progress(ana, "PUT", epub, `{"locator":{"type":"epub","href":"all.xhtml","progression":0.9}}`)
	_, r := s.equivalent(ana, pdf, epub)
	for _, c := range r.Candidates {
		if c.Method == "structure" {
			t.Errorf("a place outside the story has no chapter: %+v", c)
		}
	}
}
