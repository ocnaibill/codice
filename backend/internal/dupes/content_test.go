package dupes_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/dupes"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

// The same text in another file (#38): proposed by its words, whatever the title, the author or the format.

// book is n words in an order that no other seed repeats by chance.
func book(seed int64, n int) []string {
	r := rand.New(rand.NewSource(seed))
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("palavra%d", r.Intn(5000))
	}
	return out
}

// edition changes one word in every `every`, as another edition of the text.
func edition(words []string, every int) []string {
	out := append([]string{}, words...)
	for i := every; i < len(out); i += every {
		out[i] = "alterada"
	}
	return out
}

// segments cuts words into pieces of about 200 words, as the extraction does with a chapter.
func segments(words []string) []string {
	var out []string
	for i := 0; i < len(words); i += 200 {
		end := i + 200
		if end > len(words) {
			end = len(words)
		}
		out = append(out, strings.Join(words[i:end], " "))
	}
	return out
}

// workWithFile makes a work with one file.
func (e *env) workWithFile(title, author, format string) (int, int64) {
	e.t.Helper()
	id, _, file := testdb.AddWork(e.t, e.db, testdb.Work{Title: title, Path: title + "." + format, Format: format, Author: author})
	return id, file
}

type outline struct {
	front, body, back []string // the words of each part; with no part given the file has no outline
	loose             []string // words the outline does not reach (a cover before the first entry): no node
	bodyFirst         bool     // the body is the first node of the outline (it has no front)
}

// publish gives the file published text: the body words alone (no outline), or the three parts with an outline.
func (e *env) publish(file int64, words []string, o *outline) {
	e.t.Helper()
	var begin int
	if err := e.db.QueryRow(`SELECT text_extraction_begin($1)`, file).Scan(&begin); err != nil {
		e.t.Fatal(err)
	}
	insert := func(sequence int, text string, node any) {
		e.exec(`INSERT INTO document_segments (file_id, generation, sequence, text, locator, locator_version, node) VALUES ($1, $2, $3, $4, '{}', 1, $5)`,
			file, begin, sequence, text, node)
	}
	var structure any
	if o == nil {
		for i, s := range segments(words) {
			insert(i, s, nil)
		}
	} else {
		type node struct {
			Title string `json:"title"`
			Depth int    `json:"depth"`
			Chars int    `json:"chars"`
			Part  string `json:"part"`
		}
		var nodes []node
		sequence := 0
		for _, s := range segments(o.loose) {
			insert(sequence, s, nil)
			sequence++
		}
		parts := []struct {
			name  string
			words []string
		}{{"front", o.front}, {"body", o.body}, {"back", o.back}}
		if o.bodyFirst {
			parts = parts[1:]
		}
		for i, part := range parts {
			chars := 0
			for _, s := range segments(part.words) {
				insert(sequence, s, i)
				sequence++
				chars += len(s)
			}
			nodes = append(nodes, node{Title: part.name, Chars: chars, Part: part.name})
		}
		raw, _ := json.Marshal(nodes)
		structure = string(raw)
	}
	var published int
	if err := e.db.QueryRow(`SELECT text_extraction_publish($1, $2, 6, $3, 'ready', 'native', 'pt', $4::jsonb)`, file, begin,
		fmt.Sprintf("%064d", begin), structure).Scan(&published); err != nil {
		e.t.Fatal(err)
	}
}

func TestDetect_TheSameTextInAnotherFileIsProposedWhateverTheTitleAndTheAuthor(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	epub, epubFile := e.workWithFile("Duna", "Frank Herbert", "epub")
	pdf, pdfFile := e.workWithFile("Dune: edição de bolso", "Herbert, F.", "pdf")
	other, otherFile := e.workWithFile("Outro livro", "Alguém", "epub")
	e.publish(epubFile, text, nil)
	e.publish(pdfFile, edition(text, 60), nil) // another edition: one word in sixty changed
	e.publish(otherFile, book(2, 40000), nil)

	// Each work is looked at when its own text is done: the first finds nothing to compare with yet, the second
	// finds the first.
	dupes.Detect(ctx, e.db, other)
	if n, err := dupes.Detect(ctx, e.db, epub); err != nil || n != 0 {
		t.Fatalf("the first: %d (%v)", n, err)
	}
	n, err := dupes.Detect(ctx, e.db, pdf)
	if err != nil || n != 1 {
		t.Fatalf("found %d (%v), want 1", n, err)
	}
	if got, want := e.pairs(), fmt.Sprintf("%d-%d:content:pending", epub, pdf); got != want {
		t.Fatalf("pairs %q, want %q (not %d)", got, want, other)
	}
	var evidence struct {
		Content struct {
			FileA, FileB   int64
			OfA, OfB       float64
			Shared         int
			WordsA, WordsB int
		}
	}
	if err := json.Unmarshal([]byte(e.scalar(`SELECT evidence FROM duplicate_candidates`)), &evidence); err != nil {
		t.Fatal(err)
	}
	c := evidence.Content
	if c.FileA != epubFile || c.FileB != pdfFile || c.OfA < 0.6 || c.OfA > 1 || c.OfB < 0.6 || c.Shared < 500 || c.WordsA != 40000 || c.WordsB != 40000 {
		t.Errorf("evidence %+v", c)
	}
	// Asking again finds nothing new, from either side.
	for _, w := range []int{epub, pdf} {
		if n, _ := dupes.Detect(ctx, e.db, w); n != 0 || e.scalar(`SELECT count(*) FROM duplicate_candidates`) != "1" {
			t.Errorf("again, from %d: %d", w, n)
		}
	}
}

func TestDetect_TheEvidenceIsOfTheWorksInTheOrderOfThePair(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	// The work with the higher id is looked at, and the one with the lower id is the larger text.
	low, lowFile := e.workWithFile("A", "x", "epub")
	high, highFile := e.workWithFile("B", "y", "pdf")
	e.publish(lowFile, text, nil)
	e.publish(highFile, text, nil)
	dupes.Detect(ctx, e.db, low)
	if n, err := dupes.Detect(ctx, e.db, high); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if e.scalar(`SELECT work_a || '-' || work_b FROM duplicate_candidates`) != fmt.Sprintf("%d-%d", low, high) {
		t.Fatal("pair order")
	}
	if e.scalar(`SELECT (evidence->'content'->>'fileA') || ':' || (evidence->'content'->>'fileB') FROM duplicate_candidates`) != fmt.Sprintf("%d:%d", lowFile, highFile) {
		t.Errorf("the files follow the works: %s", e.scalar(`SELECT evidence::text FROM duplicate_candidates`))
	}
}

func TestDetect_WhatOnlyResemblesIsNotProposed(t *testing.T) {
	e := newEnv(t)
	first, f1 := e.workWithFile("Primeiro", "Ana", "epub")
	_, f2 := e.workWithFile("Segundo", "Ana", "epub")      // the same author, the same characters: another book
	_, f3 := e.workWithFile("Coletânea", "Ana", "epub")    // holds the first one and others
	_, f4 := e.workWithFile("Muito curto", "Ana", "epub")  // too short to be told by
	_, f5 := e.workWithFile("Outro idioma", "Ana", "epub") // a translation shares no run of words
	text := book(1, 40000)
	e.publish(f1, text, nil)
	e.publish(f2, book(2, 40000), nil)
	e.publish(f3, append(append(book(3, 60000), text...), book(4, 60000)...), nil)
	e.publish(f4, text[:2000], nil)
	e.publish(f5, book(5, 40000), nil)

	// The collection holds all of the first book, and the first is a third of it: not the same text.
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("found %d (%v): %s", n, err, e.pairs())
	}
	_ = first
	if e.scalar(`SELECT count(*) FROM text_fingerprints`) != "5" || e.scalar(`SELECT cardinality(sample) FROM text_fingerprints WHERE file_id = `+fmt.Sprint(f4)) != "0" {
		t.Error("a text too short is recorded with no sample, so it is not made again")
	}
}

func TestDetect_ALicenceEveryFileOfASourceCarriesIsNotTheText(t *testing.T) {
	e := newEnv(t)
	licence := book(99, 3000)
	_, f1 := e.workWithFile("Um", "x", "epub")
	_, f2 := e.workWithFile("Dois", "y", "epub")
	e.publish(f1, nil, &outline{front: book(10, 500), body: book(1, 30000), back: licence})
	e.publish(f2, nil, &outline{front: book(10, 500), body: book(2, 30000), back: licence})
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("found %d (%v): the front and the back are not read", n, err)
	}
	// And the same body under another front and back is the same text.
	_, f3 := e.workWithFile("Tres", "z", "pdf")
	e.publish(f3, nil, &outline{front: book(11, 700), body: book(1, 30000), back: book(12, 100)})
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 1 {
		t.Fatalf("found %d (%v)", n, err)
	}
	if e.scalar(`SELECT (evidence->'content'->>'wordsA')::int FROM duplicate_candidates`) != "30000" {
		t.Errorf("only the body is counted: %s", e.scalar(`SELECT evidence::text FROM duplicate_candidates`))
	}
}

func TestDetect_WhatTheOutlineDoesNotReachIsNotTheStory(t *testing.T) {
	e := newEnv(t)
	licence := book(99, 3000)
	_, f1 := e.workWithFile("Um", "x", "epub")
	_, f2 := e.workWithFile("Dois", "y", "epub")
	// A licence before the first entry of the outline, in a segment that belongs to no node.
	e.publish(f1, nil, &outline{loose: licence, body: book(1, 30000), bodyFirst: true})
	e.publish(f2, nil, &outline{loose: licence, body: book(2, 30000), bodyFirst: true})
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("found %d (%v): a segment with no node was read", n, err)
	}
	if e.scalar(`SELECT words FROM text_fingerprints WHERE file_id = `+fmt.Sprint(f1)) != "30000" {
		t.Error("only the body is counted")
	}
}

func TestDetect_AFileWithNoOutlineIsReadWhole(t *testing.T) {
	e := newEnv(t)
	text := book(1, 30000)
	_, f1 := e.workWithFile("Um", "x", "pdf")
	_, f2 := e.workWithFile("Dois", "y", "epub")
	e.publish(f1, text, nil)
	e.publish(f2, nil, &outline{front: book(10, 500), body: text, back: book(11, 500)})
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 1 {
		t.Fatalf("found %d (%v)", n, err)
	}
}

func TestDetect_APairWaitingForAnotherReasonGainsTheEvidenceAndAPairThatWasDecidedIsLeftAlone(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	a, af := e.workWithFile("Duna", "Frank Herbert", "epub")
	b, bf := e.workWithFile("duna", "frank herbert", "pdf") // title and author: already a pair by metadata
	c, cf := e.workWithFile("Dune", "F. H.", "pdf")
	for _, f := range []int64{af, bf, cf} {
		e.publish(f, text, nil)
	}
	dupes.Detect(ctx, e.db, c)
	if n, err := dupes.Detect(ctx, e.db, a); err != nil || n != 2 { // a-b by metadata; a-c by content
		t.Fatalf("%d %v", n, err)
	}
	got := e.pairs()
	if !strings.Contains(got, fmt.Sprintf("%d-%d:title_author:pending", a, b)) || !strings.Contains(got, fmt.Sprintf("%d-%d:content:pending", a, c)) {
		t.Fatalf("pairs %s", got)
	}
	// The pair a-b came by title and author, and it is the same text too: the reason stays, the evidence is added.
	if n, err := dupes.Detect(ctx, e.db, b); err != nil || n != 1 { // b-c is new
		t.Fatalf("%d %v", n, err)
	}
	if e.scalar(`SELECT count(*) FROM duplicate_candidates`) != "3" {
		t.Fatalf("pairs %s", e.pairs())
	}
	if e.scalar(fmt.Sprintf(`SELECT reason || ':' || (evidence IS NOT NULL) FROM duplicate_candidates WHERE work_a = %d AND work_b = %d`, a, b)) != "title_author:true" {
		t.Errorf("the reason stays and the evidence is added")
	}
	// A pair somebody said was not the same work stays that way, and is not given evidence.
	e.exec(`UPDATE duplicate_candidates SET state = 'dismissed' WHERE work_a = $1 AND work_b = $2`, a, c)
	e.exec(`UPDATE duplicate_candidates SET evidence = NULL WHERE work_a = $1 AND work_b = $2`, a, c)
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if e.scalar(fmt.Sprintf(`SELECT state || ':' || (evidence IS NULL) FROM duplicate_candidates WHERE work_a = %d AND work_b = %d`, a, c)) != "dismissed:true" {
		t.Error("a decided pair was touched")
	}
}

func TestDetect_TheFilesOfOneWorkAreNotComparedWithEachOtherAndARetiredWorkIsNotProposed(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	a, af := e.workWithFile("Duna", "x", "epub")
	// A second file under the same work: another format of the same book already put together.
	var edition int
	e.db.QueryRow(`SELECT id FROM editions WHERE work_id = $1`, a).Scan(&edition)
	var second int64
	e.db.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, 'pdf') RETURNING id`, edition).Scan(&second)
	e.publish(af, text, nil)
	e.publish(second, text, nil)
	if n, _ := dupes.DetectAll(ctx, e.db); n != 0 {
		t.Fatalf("found %d inside one work", n)
	}
	retired, rf := e.workWithFile("Velho", "y", "epub")
	e.publish(rf, text, nil)
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retired)
	if n, _ := dupes.DetectAll(ctx, e.db); n != 0 {
		t.Fatalf("found %d with a retired work", n)
	}
}

func TestDetect_ARetiredWorkIsNotProposedEvenWithAFingerprint(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	a, af := e.workWithFile("Um", "x", "epub")
	old, of := e.workWithFile("Velho", "y", "pdf")
	e.publish(af, text, nil)
	e.publish(of, text, nil)
	if n, _ := dupes.DetectAll(ctx, e.db); n != 1 { // both are active: the pair is found, and both have a fingerprint
		t.Fatalf("found %d", n)
	}
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, old)
	if n, _ := dupes.Detect(ctx, e.db, a); n != 0 {
		t.Errorf("found %d with a retired work that has a fingerprint", n)
	}
}

func TestDetect_AFingerprintOfTextThatWasReadAgainIsNotUsed(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	a, af := e.workWithFile("Um", "x", "epub")
	b, bf := e.workWithFile("Dois", "y", "pdf")
	e.publish(af, text, nil)
	e.publish(bf, text, nil)
	dupes.Detect(ctx, e.db, b)         // the fingerprint of the second file is made
	e.publish(bf, book(7, 40000), nil) // and then its text is another (read again), its fingerprint not yet
	if n, err := dupes.Detect(ctx, e.db, a); err != nil || n != 0 {
		t.Fatalf("found %d (%v): the old text of the other file was used", n, err)
	}
	_ = b
}

func TestDetect_OfTheFilesOfAnotherWorkTheBestMatchIsTheEvidence(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	a, af := e.workWithFile("Um", "x", "epub")
	b, bf := e.workWithFile("Dois", "y", "pdf")
	var ed int
	e.db.QueryRow(`SELECT id FROM editions WHERE work_id = $1`, b).Scan(&ed)
	var second int64
	e.db.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, 'epub') RETURNING id`, ed).Scan(&second)
	e.publish(af, text, nil)
	e.publish(bf, edition(text, 40), nil) // a rougher edition: one word in forty
	e.publish(second, text, nil)          // and an exact one
	dupes.Detect(ctx, e.db, b)
	if n, err := dupes.Detect(ctx, e.db, a); err != nil || n != 1 {
		t.Fatalf("found %d (%v)", n, err)
	}
	if got := e.scalar(`SELECT (evidence->'content'->>'fileB')::bigint FROM duplicate_candidates`); got != fmt.Sprint(second) {
		t.Errorf("the evidence is of file %s, the exact match is %d", got, second)
	}
}

func TestDetect_TheFingerprintIsMadeOnceAndMadeAgainWhenTheTextChanges(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	a, af := e.workWithFile("Um", "x", "epub")
	_, bf := e.workWithFile("Dois", "y", "pdf")
	e.publish(af, text, nil)
	e.publish(bf, text, nil)
	dupes.Detect(ctx, e.db, a)
	made := e.scalar(`SELECT generation || ':' || method FROM text_fingerprints WHERE file_id = ` + fmt.Sprint(af))
	if made != "1:2" {
		t.Fatalf("fingerprint %s", made)
	}
	// Nothing changed: it is not made again (the row keeps its identity).
	e.exec(`UPDATE text_fingerprints SET words = 7 WHERE file_id = $1`, af)
	dupes.Detect(ctx, e.db, a)
	if e.scalar(`SELECT words FROM text_fingerprints WHERE file_id = `+fmt.Sprint(af)) != "7" {
		t.Error("it was made again though the text did not change")
	}
	// The text is read again (a new generation), and the fingerprint follows it.
	e.publish(af, book(3, 40000), nil)
	dupes.Detect(ctx, e.db, a)
	if e.scalar(`SELECT generation || ':' || words FROM text_fingerprints WHERE file_id = `+fmt.Sprint(af)) != "2:40000" {
		t.Errorf("fingerprint %s", e.scalar(`SELECT generation || ':' || words FROM text_fingerprints WHERE file_id = `+fmt.Sprint(af)))
	}
	// A fingerprint made by an older method is made again.
	e.exec(`UPDATE text_fingerprints SET method = 0, words = 7 WHERE file_id = $1`, af)
	dupes.Detect(ctx, e.db, a)
	if e.scalar(`SELECT method || ':' || words FROM text_fingerprints WHERE file_id = `+fmt.Sprint(af)) != "2:40000" {
		t.Error("an old method's fingerprint was kept")
	}
}

func TestDetect_AFileWithNoTextHasNoFingerprint(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Escaneado", "x", "pdf")
	e.exec(`INSERT INTO text_extractions (file_id, generation, extractor_version, status) VALUES ($1, 0, 6, 'empty')`, af)
	if n, err := dupes.Detect(ctx, e.db, a); err != nil || n != 0 || e.scalar(`SELECT count(*) FROM text_fingerprints`) != "0" {
		t.Errorf("%d %v", n, err)
	}
}

func TestListPending_SaysWhatTheFilesShare(t *testing.T) {
	e := newEnv(t)
	text := book(1, 40000)
	_, af := e.workWithFile("Um", "x", "epub")
	_, bf := e.workWithFile("Dois", "y", "pdf")
	e.publish(af, text, nil)
	e.publish(bf, text, nil)
	dupes.DetectAll(ctx, e.db)
	list, err := dupes.ListPending(ctx, e.db)
	if err != nil || len(list) != 1 || list[0].Reason != "content" {
		t.Fatalf("%v %+v", err, list)
	}
	var ev struct{ Content struct{ OfA, OfB float64 } }
	if json.Unmarshal(list[0].Evidence, &ev) != nil || ev.Content.OfA != 1 || ev.Content.OfB != 1 {
		t.Errorf("evidence %s", list[0].Evidence)
	}
	// A pair by metadata has none.
	c, _ := e.workWithFile("Um", "x", "cbz")
	_ = c
	dupes.DetectAll(ctx, e.db)
	for _, item := range mustList(t, e) {
		if item.Reason == "title_author" && len(item.Evidence) != 0 {
			t.Errorf("a pair by metadata carries evidence: %s", item.Evidence)
		}
	}
}

func mustList(t *testing.T, e *env) []dupes.Candidate {
	list, err := dupes.ListPending(ctx, e.db)
	if err != nil {
		t.Fatal(err)
	}
	return list
}
