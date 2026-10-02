package dupes_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/dupes"
)

// The same book in another language (#38): proposed because read against the other, end to end, its passages are
// found one after the other, though no run of words is the same.

// person is a character of a story, the same in every language.
func person(story string, id int) string {
	letters := []byte("abcdefghijklmnopqrstuvwxyz")
	out := []byte{}
	for n := id + 1; n > 0; n /= 26 {
		out = append(out, letters[n%26])
	}
	return "Pers" + story + string(out)
}

// scenes is a book of n segments: each names four people, two of them from the scene before, in some language's frame
// of words (which no other language shares).
func scenes(story, frame string, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		names := []string{person(story, 3*i), person(story, 3*i+1), person(story, 3*i+2)}
		if i > 0 {
			names = append(names, person(story, 3*(i-1)))
		}
		out = append(out, fmt.Sprintf("%s %d %s %s %s.", frame, i, strings.Join(names[:2], " "), frame+"x", strings.Join(names[2:], " ")))
	}
	return out
}

// publishScenes gives the file published text of exactly these segments, in this language.
func (e *env) publishScenes(file int64, language string, texts []string) {
	e.t.Helper()
	var begin int
	if err := e.db.QueryRow(`SELECT text_extraction_begin($1)`, file).Scan(&begin); err != nil {
		e.t.Fatal(err)
	}
	for i, s := range texts {
		e.exec(`INSERT INTO document_segments (file_id, generation, sequence, text, locator, locator_version) VALUES ($1, $2, $3, $4, $5, 1)`,
			file, begin, i, s, fmt.Sprintf(`{"type":"text","offset":%d}`, i))
	}
	if err := e.db.QueryRow(`SELECT text_extraction_publish($1, $2, 6, $3, 'ready', 'native', $4, NULL)`, file, begin, fmt.Sprintf("%064d", begin), language).Scan(new(int)); err != nil {
		e.t.Fatal(err)
	}
}

type translationEvidence struct {
	Translation struct {
		FileA, FileB               int64
		Hits, Samples, SharedNames int
		Order                      float64
		NamesA, NamesB             int
		LanguageA, LanguageB       string
	}
}

func (e *env) evidenceOf(t *testing.T) translationEvidence {
	var ev translationEvidence
	if err := json.Unmarshal([]byte(e.scalar(`SELECT evidence FROM duplicate_candidates`)), &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestDetect_ABookInAnotherLanguageIsProposedByReadingItAgainstTheOther(t *testing.T) {
	e := newEnv(t)
	pt, ptFile := e.workWithFile("A viagem", "Autor", "epub")
	en, enFile := e.workWithFile("The journey", "Writer", "epub") // another title, another author
	e.publishScenes(ptFile, "pt", scenes("a", "a historia seguiu e depois chegou", 150))
	e.publishScenes(enFile, "en", scenes("a", "the story went on and then came", 150))

	dupes.Detect(ctx, e.db, pt) // the first is looked at when its text is done: nothing to read it against yet
	if e.scalar(`SELECT count(*) FROM duplicate_candidates`) != "0" {
		t.Fatal("a pair before the second book had its fingerprint")
	}
	n, err := dupes.Detect(ctx, e.db, en)
	if err != nil || n != 1 {
		t.Fatalf("found %d (%v)", n, err)
	}
	if got, want := e.pairs(), fmt.Sprintf("%d-%d:translation:pending", pt, en); got != want {
		t.Fatalf("pairs %q, want %q", got, want)
	}
	c := e.evidenceOf(t).Translation
	if c.FileA != ptFile || c.FileB != enFile || c.Hits < 40 || c.Samples < 50 || c.Order < 0.99 || c.SharedNames < 100 ||
		c.NamesA == 0 || c.NamesB == 0 || c.LanguageA != "pt" || c.LanguageB != "en" {
		t.Errorf("evidence %+v", c)
	}
	// No run of words is shared, so it is not the same text, and asking again finds nothing new.
	if strings.Contains(e.scalar(`SELECT evidence::text FROM duplicate_candidates`), `"content"`) {
		t.Error("the same text")
	}
	for _, w := range []int{pt, en} {
		if n, _ := dupes.Detect(ctx, e.db, w); n != 0 || e.scalar(`SELECT count(*) FROM duplicate_candidates`) != "1" {
			t.Errorf("again, from %d: %d", w, n)
		}
	}
}

func TestDetect_TheEvidenceFollowsThePairWhicheverBookIsLookedAt(t *testing.T) {
	e := newEnv(t)
	low, lowFile := e.workWithFile("A", "x", "epub")
	high, highFile := e.workWithFile("B", "y", "epub")
	e.publishScenes(lowFile, "fr", scenes("b", "l histoire continua et puis", 150))
	e.publishScenes(highFile, "en", scenes("b", "the story went on and then", 150))
	dupes.Detect(ctx, e.db, low)
	if n, err := dupes.Detect(ctx, e.db, high); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	c := e.evidenceOf(t).Translation
	if c.FileA != lowFile || c.FileB != highFile || c.LanguageA != "fr" || c.LanguageB != "en" {
		t.Errorf("the sides follow the works, not who was looked at: %+v", c)
	}
}

func TestDetect_ASequelThatSharesTheCastIsNotProposed(t *testing.T) {
	e := newEnv(t)
	first, f1 := e.workWithFile("Primeiro", "Ana", "epub")
	second, f2 := e.workWithFile("Segundo", "Ana", "epub")
	sequel := scenes("d", "um outro idioma aqui", 150)
	// The cast of the first book comes into the sequel where the story takes it, in two scenes each time (so that
	// they are rare names of it too): not scene by scene along the first book. They share more than the least names
	// to be read against each other, and the reading says they are not the same book.
	for i := 0; i < 150; i += 2 {
		old := (i*7 + 3) % 150
		extra := " e depois " + person("c", 3*old) + " " + person("c", 3*old+1) + " " + person("c", 3*old+2)
		sequel[i] += extra
		sequel[i+1] += extra
	}
	e.publishScenes(f1, "en", scenes("c", "some language here", 150))
	e.publishScenes(f2, "en", sequel)
	dupes.Detect(ctx, e.db, first)
	if n, err := dupes.Detect(ctx, e.db, second); err != nil || n != 0 {
		t.Fatalf("found %d (%v): %s", n, err, e.pairs())
	}
	shared := e.scalar(`SELECT count(*) FROM (SELECT unnest(names) FROM text_fingerprints WHERE file_id = ` + fmt.Sprint(f1) + `
		INTERSECT SELECT unnest(names) FROM text_fingerprints WHERE file_id = ` + fmt.Sprint(f2) + `) x`)
	// A quarter of the 149 rare names of the first book, and more than the least: they are read against each other.
	if n, _ := strconv.Atoi(shared); n < 38 {
		t.Errorf("the books share %s rare names: too few for the test to be reading them against each other", shared)
	}
}

func TestDetect_BooksThatHaveNothingInCommonAreNotRead(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Um", "x", "epub")
	_, bf := e.workWithFile("Dois", "y", "epub")
	e.publishScenes(af, "pt", scenes("e", "um idioma qualquer", 150))
	e.publishScenes(bf, "en", scenes("f", "another language", 150))
	dupes.Detect(ctx, e.db, a)
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("found %d (%v)", n, err)
	}
	if e.scalar(`SELECT count(*) FROM text_fingerprints WHERE cardinality(names) > 100`) != "2" {
		t.Error("the fingerprints keep the rare names of the books")
	}
}

func TestDetect_AFewNamesInCommonAreNotEnoughToReadAgainstTheOther(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Um", "x", "epub")
	_, bf := e.workWithFile("Dois", "y", "epub")
	one := scenes("g", "um idioma", 150)
	other := scenes("g", "other language", 150)
	// The second book keeps the names of the first in only its first 6 scenes: fewer than the least to be read.
	for i := 6; i < 150; i++ {
		other[i] = strings.NewReplacer(person("g", 3*i), person("h", 3*i), person("g", 3*i+1), person("h", 3*i+1), person("g", 3*i+2), person("h", 3*i+2), person("g", 3*(i-1)), person("h", 3*(i-1))).Replace(other[i])
	}
	e.publishScenes(af, "pt", one)
	e.publishScenes(bf, "en", other)
	dupes.Detect(ctx, e.db, a)
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("found %d (%v)", n, err)
	}
}

func TestDetect_APairWaitingForAnotherReasonGainsTheEvidenceAndADecidedPairIsLeftAlone(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Duna", "Frank Herbert", "epub")
	b, bf := e.workWithFile("duna!", "frank herbert", "epub") // title and author: already a pair by metadata
	e.publishScenes(af, "pt", scenes("i", "um idioma aqui", 150))
	e.publishScenes(bf, "en", scenes("i", "other language here", 150))
	dupes.DetectAll(ctx, e.db) // finds the pair by title and author, then reads it
	if got := e.pairs(); got != fmt.Sprintf("%d-%d:title_author:pending", a, b) {
		t.Fatalf("pairs %s", got)
	}
	if e.scalar(`SELECT (evidence ? 'translation')::text FROM duplicate_candidates`) != "true" {
		t.Error("the pair by metadata gained the evidence of the reading")
	}
	// A pair somebody said was not the same work stays that way, and is not read again.
	e.exec(`UPDATE duplicate_candidates SET state = 'dismissed', evidence = NULL`)
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if e.scalar(`SELECT state || ':' || (evidence IS NULL)::text FROM duplicate_candidates`) != "dismissed:true" {
		t.Error("a decided pair was touched")
	}
}

func TestDetect_APairThatIsAlreadyKnownToBeTheSameTextIsNotReadAgain(t *testing.T) {
	e := newEnv(t)
	// The same text, with the same names: it would be found by reading it against the other, too.
	text := scenes("p", "o mesmo texto aqui", 400)
	_, af := e.workWithFile("Um", "x", "epub")
	_, bf := e.workWithFile("Dois", "y", "pdf")
	e.publishScenes(af, "pt", text)
	e.publishScenes(bf, "pt", text)
	dupes.DetectAll(ctx, e.db)
	if e.scalar(`SELECT reason FROM duplicate_candidates`) != "content" {
		t.Fatalf("pairs %s", e.pairs())
	}
	if strings.Contains(e.scalar(`SELECT evidence::text FROM duplicate_candidates`), "translation") {
		t.Error("a pair of the same text was also read as a translation")
	}
}

func TestDetect_OfTheFilesOfAnotherWorkTheOneThatReadsBestIsTheEvidence(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Um", "x", "epub")
	b, weak := e.workWithFile("Dois", "y", "epub")
	var ed int
	e.db.QueryRow(`SELECT id FROM editions WHERE work_id = $1`, b).Scan(&ed)
	var strong int64
	e.db.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, 'pdf') RETURNING id`, ed).Scan(&strong)
	original := scenes("q", "um idioma", 150)
	rough := scenes("q", "other language", 150)
	for i := 70; i < 150; i++ { // the rough copy keeps the first part of the book and then goes elsewhere
		rough[i] = scenes("r", "other language", 150)[i]
	}
	e.publishScenes(af, "pt", original)
	e.publishScenes(weak, "en", rough)
	e.publishScenes(strong, "en", scenes("q", "another language", 150))
	dupes.Detect(ctx, e.db, b)
	if n, err := dupes.Detect(ctx, e.db, a); err != nil || n != 1 {
		t.Fatalf("found %d (%v)", n, err)
	}
	if got := e.evidenceOf(t).Translation.FileB; got != strong {
		t.Errorf("the evidence is of file %d, the one that reads best is %d (the rough copy is %d)", got, strong, weak)
	}
}

func TestDetect_ARetiredWorkAndAShortBookAreNotProposedAsTranslations(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Um", "x", "epub")
	old, of := e.workWithFile("Velho", "y", "epub")
	e.publishScenes(af, "pt", scenes("j", "um idioma", 150))
	e.publishScenes(of, "en", scenes("j", "another one", 150))
	dupes.DetectAll(ctx, e.db) // both active: found
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, old)
	if n, _ := dupes.Detect(ctx, e.db, a); n != 0 {
		t.Errorf("found %d with a retired work", n)
	}
	e.exec(`UPDATE works SET retired_at = NULL WHERE id = $1`, old)
	short, sf := e.workWithFile("Curto", "z", "epub")
	e.publishScenes(sf, "es", scenes("k", "otra lengua", 20))
	_, kf := e.workWithFile("Curto de novo", "w", "epub")
	e.publishScenes(kf, "en", scenes("k", "other one", 20))
	e.exec(`DELETE FROM duplicate_candidates`)
	dupes.Detect(ctx, e.db, short)
	if n, _ := dupes.DetectAll(ctx, e.db); n != 1 { // only the pair of the two long books
		t.Errorf("found %d: %s", n, e.pairs())
	}
}

func TestListPending_SaysWhatTheReadingFound(t *testing.T) {
	e := newEnv(t)
	a, af := e.workWithFile("Um", "x", "epub")
	_, bf := e.workWithFile("Dois", "y", "epub")
	e.publishScenes(af, "pt", scenes("l", "um idioma", 150))
	e.publishScenes(bf, "en", scenes("l", "another", 150))
	dupes.Detect(ctx, e.db, a)
	dupes.DetectAll(ctx, e.db)
	list, err := dupes.ListPending(ctx, e.db)
	if err != nil || len(list) != 1 || list[0].Reason != "translation" {
		t.Fatalf("%v %+v", err, list)
	}
	var ev struct{ Translation struct{ Hits, Samples int } }
	if json.Unmarshal(list[0].Evidence, &ev) != nil || ev.Translation.Hits < 40 {
		t.Errorf("evidence %s", list[0].Evidence)
	}
}
