package wiki

import (
	"strings"
	"testing"
	"time"
)

func TestCursor_SeesTheMainLineUpToItsChapterAndTheSideBooksOnlyWhenAsked(t *testing.T) {
	c := Cursor{Book: 1, Chapter: 5}
	cases := []struct {
		side          bool
		book, chapter int
		want          bool
	}{
		{false, 0, 99, true}, // an earlier book, whole
		{false, 1, 5, true},  // the reader's chapter
		{false, 1, 6, false}, // the next one
		{false, 2, 1, false}, // a later book
		{true, 3, 1, false},  // a side book, not asked for
	}
	for _, tc := range cases {
		if got := c.Sees(tc.side, tc.book, tc.chapter); got != tc.want {
			t.Errorf("Sees(%v, %d, %d) = %v", tc.side, tc.book, tc.chapter, got)
		}
	}
	c.Side = true
	if !c.Sees(true, 3, 40) {
		t.Error("a side book asked for is not seen")
	}
	if Nowhere.Sees(false, 0, 1) {
		t.Error("someone who has not started sees the first chapter")
	}
}

func seq(n int) *int { return &n }

func TestChapterAt_IsTheLastChapterStartingAtOrBeforeTheSegment(t *testing.T) {
	chapters := []ChapterRow{{Chapter: 1, FirstSequence: seq(3)}, {Chapter: 2, FirstSequence: seq(10)}, {Chapter: 3}, {Chapter: 4, FirstSequence: seq(20)}}
	for seqN, want := range map[int]int{0: 0, 3: 1, 9: 1, 10: 2, 19: 2, 20: 4, 99: 4} {
		if got := ChapterAt(chapters, seqN); got != want {
			t.Errorf("ChapterAt(%d) = %d, want %d", seqN, got, want)
		}
	}
	if LastStoryChapter(append(chapters, ChapterRow{Chapter: 5, Editorial: true})) != 4 {
		t.Error("an editorial chapter counted as story")
	}
}

func TestSameResource_MatchesThePathInsideTheArchiveAndTheOneOfThePackage(t *testing.T) {
	if !sameResource("OEBPS/Text/c1.xhtml", "Text/c1.xhtml") || !sameResource("Text/c%201.xhtml#x", "OEBPS/Text/c 1.xhtml") {
		t.Error("the same resource was not matched")
	}
	if sameResource("OEBPS/Text/c1.xhtml", "Text/c11.xhtml") || sameResource("a/c1.xhtml", "b/c1.xhtml") {
		t.Error("different resources were matched")
	}
}

func TestQuoteSequence_FindsTheWordsWhateverTheAccentsAndPunctuation(t *testing.T) {
	rb := resolvedBook{segments: []segment{
		{sequence: 0, text: fold("Prólogo.")},
		{sequence: 1, text: fold("— Não — disse ela, e saiu correndo pela praça.")},
		{sequence: 2, text: fold("Outro capítulo, a mesma praça.")},
	}}
	if got := quoteSequence("Nao, disse ela e saiu correndo", rb, 1, 2); got == nil || *got != 1 {
		t.Errorf("quote = %v", got)
	}
	if got := quoteSequence("uma frase que o livro não tem", rb, 0, -1); got != nil {
		t.Errorf("an absent quote was placed at %d", *got)
	}
}

func validBundle() *Bundle {
	book := 0
	return &Bundle{
		Format: 1, GeneratedAt: time.Now(), Provider: "p", Model: "m",
		Books: []Book{{Position: 0, Title: "L", SHA256: strings.Repeat("a", 64), Chapters: []Chapter{{Chapter: 1, Title: "1"}, {Chapter: 2, Title: "2"}}}},
		Entities: []Entity{{Slug: "a", Name: "A", Kind: "person", Page: true, Appearances: []Point{{0, 1}},
			Sections: []Section{{Kind: "bio", Book: &book, ChapterFrom: 1, ChapterTo: 2,
				Sentences: []Sentence{{Text: "x", Refs: []Reference{{Book: 0, Chapter: 1}}}}}}}},
	}
}

func TestValidate_RefusesWhatCouldNotBeShownSafely(t *testing.T) {
	if err := validBundle().Validate(); err != nil {
		t.Fatalf("a valid bundle: %v", err)
	}
	cases := map[string]func(b *Bundle){
		"a point in a chapter that does not exist": func(b *Bundle) { b.Entities[0].Appearances = []Point{{0, 9}} },
		"a sentence with no reference":             func(b *Bundle) { b.Entities[0].Sections[0].Sentences[0].Refs = nil },
		"an unknown kind":                          func(b *Bundle) { b.Entities[0].Kind = "god" },
		"a relation to an entity not in the bundle": func(b *Bundle) {
			b.Entities[0].Relations = []Relation{{Other: "zz", Name: "Z", Group: "allies", Book: 0, Chapter: 1}}
		},
		"books out of order":           func(b *Bundle) { b.Books[0].Position = 1 },
		"no hash of the file":          func(b *Bundle) { b.Books[0].SHA256 = "" },
		"no provider":                  func(b *Bundle) { b.Provider = "" },
		"a repeated slug":              func(b *Bundle) { b.Entities = append(b.Entities, b.Entities[0]) },
		"an entity that never appears": func(b *Bundle) { b.Entities[0].Appearances = nil },
		"an unknown format":            func(b *Bundle) { b.Format = 2 },
	}
	for name, mutate := range cases {
		b := validBundle()
		mutate(b)
		if err := b.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
