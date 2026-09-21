package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
)

func epubSegment(seq int, href string, progression float64) equivalence.Segment {
	loc := fmt.Sprintf(`{"type":"epub","href":%q,"progression":%v}`, href, progression)
	return equivalence.Segment{Sequence: seq, Text: fmt.Sprintf("segment %d", seq), Locator: json.RawMessage(loc)}
}

// A chapter file can hold a great deal of the book (a Project Gutenberg EPUB puts ten chapters in
// one file), so the place inside it is the reader's "progression", not just the file.
func TestNearestSegment_EpubUsesTheProgressionInsideTheChapter(t *testing.T) {
	segments := []equivalence.Segment{
		epubSegment(0, "a.xhtml", 0),
		epubSegment(1, "b.xhtml", 0),
		epubSegment(2, "b.xhtml", 0.25),
		epubSegment(3, "b.xhtml", 0.5),
		epubSegment(4, "b.xhtml", 0.75),
		epubSegment(5, "c.xhtml", 0),
	}
	at := func(p string) *equivalence.Segment {
		return nearestSegment(segments, json.RawMessage(`{"type":"epub","href":"b.xhtml","progression":`+p+`}`))
	}

	for _, c := range []struct {
		progression string
		want        int
	}{
		{"0", 1},
		{"0.1", 1},  // still inside the first segment, which starts at 0
		{"0.25", 2}, // exactly where the second starts
		{"0.6", 3},  // between two starts: the one it is inside (the last starting before it)
		{"0.99", 4}, // the end of the chapter belongs to the last segment
	} {
		got := at(c.progression)
		if got == nil || got.Sequence != c.want {
			t.Errorf("progression %s: got %+v, want sequence %d", c.progression, got, c.want)
		}
	}
}

func TestNearestSegment_EpubWithoutProgressionIsTheStartOfTheChapter(t *testing.T) {
	segments := []equivalence.Segment{epubSegment(0, "a.xhtml", 0), epubSegment(1, "b.xhtml", 0), epubSegment(2, "b.xhtml", 0.5)}
	got := nearestSegment(segments, json.RawMessage(`{"type":"epub","href":"b.xhtml"}`))
	if got == nil || got.Sequence != 1 {
		t.Fatalf("got %+v, want the first segment of the chapter", got)
	}
}

func TestNearestSegment_EpubChapterNotIndexedIsNothing(t *testing.T) {
	segments := []equivalence.Segment{epubSegment(0, "a.xhtml", 0)}
	if got := nearestSegment(segments, json.RawMessage(`{"type":"epub","href":"missing.xhtml","progression":0.5}`)); got != nil {
		t.Fatalf("got %+v, want nothing: no invented place", got)
	}
}
