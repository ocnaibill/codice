// Package clippings reads what a reading device keeps of what a person marked: the file My Clippings.txt of a Kindle and the export of
// highlights of KOReader (JSON). Each gives a list of entries (the book, the passage, the note, when), and nothing else: where they go in the
// library is the importer's business.
package clippings

import (
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Origin is where an entry comes from.
const (
	Kindle   = "kindle"
	KOReader = "koreader"
)

// Entry is one thing the person marked in a book.
type Entry struct {
	Origin string
	Title  string
	Author string
	// Kind is "highlight" (a passage), "note" (a passage with something written about it, or a note on its own).
	Kind string
	Text string // the passage
	Note string // what the person wrote
	At   *time.Time
	// Color is what the device calls the color of the highlight, if it says.
	Color string
	// Chapter is the title of the chapter, when the device says.
	Chapter string
	// Place is where the device says it is: an xpointer for a reflowable book (KOReader), a page number for a paged one.
	Place string
	// Location is the Kindle's own count (a "location" of 128 characters), when it says; there is no way to turn it into a place of the file.
	Location string
}

// Result is what a file holds.
type Result struct {
	Format  string // "kindle" or "koreader"
	Entries []Entry
	// Skipped says what was left out and why, by reason: bookmarks (a place with nothing in it), clippings the Kindle did not give (a book with
	// a limit of clippings), and repeated ones.
	Skipped map[string]int
}

// Key is what two writings of a title or a name have in common: no accents, no case, no punctuation, single spaces.
func Key(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// GroupKey identifies a book as the device names it.
func GroupKey(title, author string) string { return Key(title) + "|" + Key(author) }
