package handlers

import (
	"math"
	"sort"
	"time"
)

// The story of a series (DEC-202): the chapters, in the order they are told, and what each file of the series contains of them. A chapter
// contains itself; a volume, an omnibus or a "Cap 12-13" contains the range its owner wrote; a work with no range and no chapter number has no
// place in the story. Everything here derives what to show from that; nothing is written to a work, and reading a volume never marks the
// chapters it contains.

// storyItem is a work of the series as the story sees it.
type storyItem struct {
	ID       int
	Unit     string
	Position *float64 // the number in the series
	From, To *float64 // the range it says it contains
	Done     bool
	Started  bool
	LastAt   *time.Time
}

// span is the part of the story the item contains, and whether it has one: the range it says, or itself when it is a chapter with a number.
func (it storyItem) span() (from, to float64, ok bool) {
	if it.From != nil && it.To != nil {
		return *it.From, *it.To, true
	}
	if it.Unit == "chapter" && it.Position != nil && *it.Position > 0 {
		return *it.Position, *it.Position, true
	}
	return 0, 0, false
}

// holds says whether an item gathers other works: it has a range that is more than one chapter.
func (it storyItem) holds() bool {
	from, to, ok := it.span()
	return ok && to > from
}

// NumberRange is a stretch of whole chapter numbers, both ends included.
type NumberRange struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// Story is what the page of a series says of its story (DEC-202).
type Story struct {
	// Ranged says some work of the series has a range of its own: only then does the story order the page and decide where to go on.
	Ranged bool `json:"ranged"`
	// Order is the works in the order of the story: each begins at the chapter it begins with, a work that contains many before the ones
	// it contains, which are in Nested, under it. Unplaced are the works with no place in it (a volume that does not say what it contains).
	Order    []int         `json:"order"`
	Nested   map[int][]int `json:"nested"`
	Unplaced []int         `json:"unplaced"`
	// Chapters is how many chapters the files of the series contain, Read how many of them the caller has read (a chapter is read when it or a
	// work that contains it is finished), and Gaps the whole numbers between the first and the last that no file contains.
	Chapters  int           `json:"chapters"`
	Read      int           `json:"read"`
	ReadSpans []NumberRange `json:"readSpans"`
	Gaps      []NumberRange `json:"gaps"`
	// Continue is the work to go on with, by the story; 0 when everything is read or the series has no story.
	Continue int `json:"continue,omitempty"`
}

// buildStory reads the story of the works of a series, given in the order of the collection. The complementary works are not part of it.
func buildStory(all []storyItem) Story {
	st := Story{Order: []int{}, Nested: map[int][]int{}, Unplaced: []int{}, ReadSpans: []NumberRange{}, Gaps: []NumberRange{}}
	var items []storyItem
	for _, it := range all {
		if it.Unit == unitExtra {
			continue
		}
		items = append(items, it)
		if it.From != nil && it.To != nil {
			st.Ranged = true
		}
	}
	var placed []storyItem
	for _, it := range items {
		if _, _, ok := it.span(); ok {
			placed = append(placed, it)
		} else {
			st.Unplaced = append(st.Unplaced, it.ID)
		}
	}
	// Each work begins where its range begins; the one that contains more comes first, and then the volumes before the chapters.
	sort.SliceStable(placed, func(i, j int) bool {
		fi, ti, _ := placed[i].span()
		fj, tj, _ := placed[j].span()
		if fi != fj {
			return fi < fj
		}
		if ti != tj {
			return ti > tj
		}
		return placed[i].ID < placed[j].ID
	})

	// A chapter goes under the first work before it that contains it and more (a volume, an omnibus, a "Cap 12-13"): it stays in the list, folded.
	var holders []storyItem
	for _, it := range placed {
		from, to, _ := it.span()
		var holder *storyItem
		if it.Unit == "chapter" && to == from {
			for i := range holders {
				if hf, ht, _ := holders[i].span(); hf <= from && to <= ht {
					holder = &holders[i]
					break
				}
			}
		}
		if holder != nil {
			st.Nested[holder.ID] = append(st.Nested[holder.ID], it.ID)
			continue
		}
		st.Order = append(st.Order, it.ID)
		if it.holds() {
			holders = append(holders, it)
		}
	}

	// The chapters the files contain: the whole numbers of every range, and the number of every chapter (10.5 is one).
	universe := map[float64]bool{}
	read := map[float64]bool{}
	for _, it := range placed {
		from, to, _ := it.span()
		mark := func(n float64) {
			universe[n] = true
			if it.Done {
				read[n] = true
			}
		}
		mark(from)
		mark(to)
		for n := math.Ceil(from); n <= to; n++ {
			mark(n)
		}
	}
	numbers := make([]float64, 0, len(universe))
	for n := range universe {
		numbers = append(numbers, n)
	}
	sort.Float64s(numbers)
	st.Chapters = len(numbers)
	for _, n := range numbers {
		if read[n] {
			st.Read++
			st.ReadSpans = extend(st.ReadSpans, n)
		}
	}
	if len(numbers) > 0 {
		for n := math.Ceil(numbers[0]); n <= numbers[len(numbers)-1]; n++ {
			if !universe[n] {
				st.Gaps = extend(st.Gaps, n)
			}
		}
	}
	st.Continue = continueStory(placed, numbers, read)
	return st
}

// extend adds a whole number to the last stretch when it follows it, and otherwise begins another.
func extend(spans []NumberRange, n float64) []NumberRange {
	if k := len(spans); k > 0 && n == spans[k-1].To+1 {
		spans[k-1].To = n
		return spans
	}
	return append(spans, NumberRange{From: n, To: n})
}

// continueStory is the work to go on with: the one the caller has begun and read last, else the one that holds the first chapter they have not
// read, taking among the works that contain it the one they have begun, else of the same unit as what they read last, else the one that
// contains the most.
func continueStory(placed []storyItem, numbers []float64, read map[float64]bool) int {
	var last *storyItem
	for i := range placed {
		it := &placed[i]
		if it.Started && it.LastAt != nil && (last == nil || it.LastAt.After(*last.LastAt)) {
			last = it
		}
	}
	if last != nil {
		return last.ID
	}
	var lastRead *storyItem
	for i := range placed {
		it := &placed[i]
		if it.Done && it.LastAt != nil && (lastRead == nil || it.LastAt.After(*lastRead.LastAt)) {
			lastRead = it
		}
	}
	for _, n := range numbers {
		if read[n] {
			continue
		}
		var best *storyItem
		for i := range placed {
			it := &placed[i]
			if it.Done {
				continue
			}
			from, to, _ := it.span()
			if n < from || n > to {
				continue
			}
			if best == nil || prefer(it, best, lastRead) {
				best = it
			}
		}
		if best != nil {
			return best.ID
		}
	}
	return 0
}

// prefer says whether a is a better work to go on with than b: the one of the unit the caller read last, then the one that contains more.
func prefer(a, b, lastRead *storyItem) bool {
	if lastRead != nil && (a.Unit == lastRead.Unit) != (b.Unit == lastRead.Unit) {
		return a.Unit == lastRead.Unit
	}
	fa, ta, _ := a.span()
	fb, tb, _ := b.span()
	if ta-fa != tb-fb {
		return ta-fa > tb-fb
	}
	return a.ID < b.ID
}
