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

	universe, read := contentOf(placed)
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
	st.Continue = continueStory(placed, numbers, read, nil)
	return st
}

// contentOf is the chapters the files contain: the whole numbers of every range and the number of every chapter (10.5 is one), and which of
// them are read, that is, contained in a work that is finished.
func contentOf(placed []storyItem) (universe, read map[float64]bool) {
	universe, read = map[float64]bool{}, map[float64]bool{}
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
	return universe, read
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
func continueStory(placed []storyItem, numbers []float64, read map[float64]bool, after *float64) int {
	var last *storyItem
	for i := range placed {
		it := &placed[i]
		if it.Started && it.LastAt != nil && (last == nil || it.LastAt.After(*last.LastAt)) {
			last = it
		}
	}
	// With `after`, what is asked is what follows a point of the story, not where the caller left off.
	if last != nil && after == nil {
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
		if read[n] || (after != nil && n <= *after) {
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

// storyItemsOf are the works of a series as the story reads them.
func storyItemsOf(entries []seriesEntry) []storyItem {
	items := make([]storyItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, storyItem{ID: e.ID, Unit: e.Unit, Position: e.Position, From: e.From, To: e.To, Done: e.Done, Started: e.Started, LastAt: e.LastAt})
	}
	return items
}

// stepOf is the work of the series with that id as a button to it, and whether the caller has read anything of the series.
func stepOf(entries []seriesEntry, id int) *SeriesStep {
	begun := false
	for i := range entries {
		if entries[i].Unit != unitExtra && (entries[i].Done || entries[i].Started) {
			begun = true
		}
	}
	for i := range entries {
		if entries[i].ID == id {
			step := entries[i].SeriesStep
			step.Begun = begun
			return &step
		}
	}
	return nil
}

// continueFor is the work to go on with: by the story when some work of the series says what it contains (DEC-202), and as it always was
// otherwise.
func continueFor(entries []seriesEntry) *SeriesStep {
	st := buildStory(storyItemsOf(entries))
	if !st.Ranged {
		return continueSeries(entries)
	}
	if st.Continue == 0 {
		return nil
	}
	return stepOf(entries, st.Continue)
}

// nextFor is the work that follows workID once it is read: by the story when the series has one, the first chapter after the last one the work
// contains that is not read yet, and as it always was otherwise. A work with no place in the story has no next.
func nextFor(entries []seriesEntry, workID int) *SeriesStep {
	items := storyItemsOf(entries)
	if !buildStory(items).Ranged {
		return nextInSeries(entries, workID)
	}
	var end *float64
	for i := range items {
		if items[i].ID == workID {
			if _, to, ok := items[i].span(); ok {
				end = &to
			}
			items[i].Done = true
		}
	}
	if end == nil {
		return nil
	}
	var placed []storyItem
	for _, it := range items {
		if _, _, ok := it.span(); ok && it.Unit != unitExtra {
			placed = append(placed, it)
		}
	}
	universe, read := contentOf(placed)
	numbers := make([]float64, 0, len(universe))
	for n := range universe {
		numbers = append(numbers, n)
	}
	sort.Float64s(numbers)
	if id := continueStory(placed, numbers, read, end); id != 0 {
		return stepOf(entries, id)
	}
	return nil
}
