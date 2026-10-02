package equivalence

import "sort"

const (
	// minNameSegments and maxNameShare bound what counts as a rare name of a book: it is in at least two of its
	// segments (a name said once may be a slip or a stranger) and in at most this share of them (the hero is in
	// all of them and says nothing about which book it is).
	minNameSegments = 2
	maxNameShare    = 0.08
	// MaxRareNames bounds what is kept of a book.
	MaxRareNames = 1500
)

// RareNames are the names and numbers (see Anchors) that a book keeps through translation and that tell it from
// others: those that are in a few of its segments, as folded words. A file in another language shares most of them
// with a translation of itself and few with a book that has nothing to do with it.
func RareNames(segments []Segment) []string {
	documents := map[string]int{}
	for _, s := range segments {
		for name := range Anchors(s.Text) {
			documents[name]++
		}
	}
	limit := int(maxNameShare * float64(len(segments)))
	if limit < minNameSegments {
		limit = minNameSegments
	}
	type counted struct {
		name  string
		count int
	}
	var names []counted
	for name, n := range documents {
		if n >= minNameSegments && n <= limit {
			names = append(names, counted{name, n})
		}
	}
	// The commonest first when there are too many, then by name so that the same book gives the same list.
	sort.Slice(names, func(i, j int) bool {
		if names[i].count != names[j].count {
			return names[i].count > names[j].count
		}
		return names[i].name < names[j].name
	})
	if len(names) > MaxRareNames {
		names = names[:MaxRareNames]
	}
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n.name
	}
	sort.Strings(out)
	return out
}
