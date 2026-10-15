package wiki

// Cursor is how far a reader may see (DEC-179): a book of the main line and a chapter of it, and whether the side books (short
// stories beside the main line) are shown. Book -1 is a reader who has not started: nothing of the main line is seen.
type Cursor struct {
	Book    int  `json:"book"`
	Chapter int  `json:"chapter"`
	Side    bool `json:"side"`
}

// Nowhere is the cursor of someone who has not started reading.
var Nowhere = Cursor{Book: -1}

// Sees says whether something at (book, chapter) may be shown. A side book is seen whole when the reader asks for side books and
// not at all otherwise; the main line is seen up to the cursor, every earlier book whole.
func (c Cursor) Sees(side bool, book, chapter int) bool {
	if side {
		return c.Side
	}
	if book != c.Book {
		return book < c.Book
	}
	return chapter <= c.Chapter
}

// ChapterAt is the chapter a segment of a book's file is in: the last chapter starting at or before it. Chapters whose start is
// unknown are skipped; a segment before the first known start is in chapter 0 (before the story).
func ChapterAt(chapters []ChapterRow, sequence int) int {
	at, best := 0, -1
	for _, c := range chapters {
		if c.FirstSequence == nil || *c.FirstSequence > sequence {
			continue
		}
		if *c.FirstSequence > best || (*c.FirstSequence == best && c.Chapter > at) {
			at, best = c.Chapter, *c.FirstSequence
		}
	}
	return at
}

// ChapterRow is a stored chapter, as the read side needs it.
type ChapterRow struct {
	Chapter       int    `json:"chapter"`
	Title         string `json:"title"`
	Part          string `json:"part"`
	FirstSequence *int   `json:"-"`
	Editorial     bool   `json:"editorial"`
}

// LastStoryChapter is the last chapter that is not editorial: a reader who finished the book has read up to it.
func LastStoryChapter(chapters []ChapterRow) int {
	last := 0
	for _, c := range chapters {
		if !c.Editorial && c.Chapter > last {
			last = c.Chapter
		}
	}
	return last
}
