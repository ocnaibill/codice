package equivalence

// Whether one file is another's translation (or another translation of the same book), found by reading it
// against the other end to end (#38): a passage taken from along the smaller file is looked for in the larger,
// and in a translation most of the passages that are sampled are found, one after the other, in the order of the
// book. Two books that merely share characters (a sequel, another book of the saga) share names, but the
// passages that are found in them are few, and they do not follow the order of the book.
//
// Nothing here looks at a title, an author or a language: that two files are translations of one book is what
// the text says, with the names and numbers that survive translation (ByAnchors) and, where the wording is the
// same, the words (ByPassage). The outline is not used either: it would bring the chapters of one book to the
// chapters of the other whether or not the text agrees.

const (
	// ParallelSamples is how many passages are taken from along the smaller file.
	ParallelSamples = 60
	// MinParallelHits is the fewest passages that must be found, and MinParallelShare the share of the sample.
	MinParallelHits  = 8
	MinParallelShare = 0.12
	// MinParallelOrder is the share of the pairs of passages found that are in the same order in both files.
	MinParallelOrder = 0.80
	// minOrderedHits is the fewest hits from which an order is told.
	minOrderedHits = 6
	// MinParallelSegments is the fewest segments the smaller file may have (about 36,000 characters): a shorter text
	// has too few distinct passages to be read against another.
	MinParallelSegments = 30
)

// Parallel is what reading one file against another found.
type Parallel struct {
	Samples int     // passages taken from the smaller file (each segment is taken once)
	Hits    int     // found in the other
	Share   float64 // Hits / Samples
	Order   float64 // of the pairs of passages found, the share in the same order in both; -1 when too few were found
	Smaller int     // how many segments the smaller file has, and
	Larger  int     // the larger one
}

// Translation says whether the two files read as the same book in two versions: enough of the sampled passages
// are found and they come in the order of the book.
func (p Parallel) Translation() bool {
	return p.Hits >= MinParallelHits && p.Share >= MinParallelShare && p.Order >= MinParallelOrder
}

// body is the file as the comparison reads it: the segments of the body of the book, with no outline.
func body(f File) File {
	out := File{}
	for _, s := range f.Segments {
		if f.hasOutline() && s.Part != PartBody {
			continue
		}
		s.Part, s.Node, s.Chapter = "", NoNode, ""
		out.Segments = append(out.Segments, s)
	}
	return out
}

// ReadParallel reads the two files against each other, from the smaller (by segments) to the larger: the larger
// may hold more than the smaller (a book with its sequel in one file), and a passage of the smaller is in it.
func ReadParallel(a, b File, samples int) Parallel {
	a, b = body(a), body(b)
	if len(a.Segments) > len(b.Segments) {
		a, b = b, a
	}
	out := Parallel{Order: -1, Smaller: len(a.Segments), Larger: len(b.Segments)}
	if len(a.Segments) < MinParallelSegments || len(b.Segments) == 0 || samples <= 0 {
		return out
	}
	finder := newParallelFinder(a, b)
	place := make(map[int]int, len(b.Segments)) // sequence -> position in the larger file
	for i, s := range b.Segments {
		place[s.Sequence] = i
	}
	type hit struct{ from, to int }
	var hits []hit
	n := len(a.Segments)
	taken := map[int]bool{} // a segment is sampled once: in a short file the samples would fall on the same ones
	for j := 0; j < samples; j++ {
		i := int(float64(n) * (0.03 + 0.94*(float64(j)+0.5)/float64(samples)))
		if i >= n {
			i = n - 1
		}
		if taken[i] {
			continue
		}
		taken[i] = true
		out.Samples++
		if sequence, ok := finder.find(a.Segments[i]); ok {
			if to, in := place[sequence]; in {
				hits = append(hits, hit{i, to})
			}
		}
	}
	out.Hits = len(hits)
	out.Share = float64(out.Hits) / float64(out.Samples)
	if len(hits) >= minOrderedHits {
		to := make([]int, len(hits))
		for i, h := range hits {
			to[i] = h.to
		}
		out.Order = inOrder(to)
	}
	return out
}

// inOrder is the share of the pairs of places, given in the order of the smaller file, that are in the same order in the
// larger one: 1 when they follow the book, about a half when they are scattered. Two passages that fall in one segment
// are in order (a longer file cut in fewer pieces puts neighbours together).
func inOrder(to []int) float64 {
	pairs, ordered := 0, 0
	for x := 0; x < len(to); x++ {
		for y := x + 1; y < len(to); y++ {
			pairs++
			if to[x] <= to[y] {
				ordered++
			}
		}
	}
	if pairs == 0 {
		return -1
	}
	return float64(ordered) / float64(pairs)
}

// parallelFinder is Find for two files with no outline, made to be asked many times: what Find makes from the two
// files on every question (the word sequences of every segment, the names, the windows) is made once here.
type parallelFinder struct {
	srcBySeq, dstBySeq map[int]Segment
	dstWindowBySeq     map[int]Segment
	toDestination      *shingleIndex // the wording of the larger file, for the passage
	toNames            *anchorIndex  // the names of the larger file, read with the neighbours
	backWindowText     *shingleIndex // and the way back from them, in the smaller file read with the neighbours
	backWindowNames    *anchorIndex
}

func newParallelFinder(src, dst File) *parallelFinder {
	srcWindows, dstWindows := windows(src.Segments), windows(dst.Segments)
	return &parallelFinder{
		srcBySeq: sequences(src.Segments), dstBySeq: sequences(dst.Segments),
		dstWindowBySeq: sequences(dstWindows),
		toDestination:  newShingleIndex(dst.Segments),
		toNames:        newAnchorIndex(dstWindows),
		backWindowText: newShingleIndex(srcWindows), backWindowNames: newAnchorIndex(srcWindows),
	}
}

// best is the candidate Find would put first: the most trusted, then the best scored.
func best(list []Candidate) (Candidate, bool) {
	if len(list) == 0 {
		return Candidate{}, false
	}
	first := list[0]
	for _, c := range list[1:] {
		if rank(c.Confidence) > rank(first.Confidence) || (rank(c.Confidence) == rank(first.Confidence) && c.Score > first.Score) {
			first = c
		}
	}
	return first, true
}

// find is where in the larger file the passage is, by its words, or by its names and numbers; the sequence of the
// segment, and whether it was found at all. As Find does, a place that does not lead back to the passage is dropped
// (when it rests on names) or trusted less (when it rests on words).
func (f *parallelFinder) find(source Segment) (int, bool) {
	// By its words. Where the way back does not lead here, a place found by its words is trusted less, not dropped
	// (Find lowers its confidence), and a hit does not depend on the confidence, so it is kept as it is.
	if c, ok := best(f.toDestination.match(source)); ok {
		return c.sequence, true
	}
	window := windowAround(source, f.srcBySeq)
	var names []Candidate
	for _, c := range f.toNames.match(window) {
		if reverseWith(window, f.dstWindowBySeq[c.sequence], f.backWindowText.match, f.backWindowNames.match) == Disagrees {
			continue // names alone, and the way back does not lead here: not it
		}
		names = append(names, c)
	}
	if c, ok := best(names); ok {
		return c.sequence, true
	}
	return 0, false
}
