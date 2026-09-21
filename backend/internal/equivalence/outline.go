package equivalence

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
)

// The shape of a book (DEC-087): the nodes of a file's outline, the part of the book each is in, and
// the divisions of the story that two versions of a book can be matched by. The worker writes the
// nodes (worker/textindex/structure.py); a segment says which node it is in.

// Parts of a book.
const (
	PartFront = "front"
	PartBody  = "body"
	PartBack  = "back"
)

// NoNode is the node of a segment the outline does not reach (a cover before the first entry).
const NoNode = -1

const (
	minUnitChars       = 600 // a division with less text than this is a title, not a chapter
	minNumberedShare   = 0.6 // of the divisions that must carry a number for the numbers to be checked
	minNumberAgreement = 0.8 // of those, the share that must be the same in both versions
	coarseUnitShare    = 0.1 // a division larger than this share of the story is too big to land near a person in it
)

// Node is one entry of a file's outline.
type Node struct {
	Title string `json:"title"`
	Depth int    `json:"depth"` // 0 at the top
	Chars int    `json:"chars"` // the text it holds by itself
	Part  string `json:"part"`
}

// Unit is one division of a book's story at some depth of its outline: a chapter, a part.
type Unit struct {
	Key           string // what its segments carry as their Chapter
	Title         string
	Chars         int
	FirstSequence int
	Locator       json.RawMessage // the address of its first segment
	Excerpt       string          // the start of its text, to show
	Number        int             // "Chapter 3", "Book One": 0 if the title does not say
	HasNumber     bool
	Run           int // the numbering run it is in: the numbers start over where a second book begins
}

// Division is the story of a file cut at one depth of its outline.
type Division struct {
	Depth int
	Units []Unit
	of    map[int]int // segment sequence -> index into Units
}

// UnitOf is the unit a segment is in, if it is in one.
func (d *Division) UnitOf(sequence int) (int, bool) {
	i, ok := d.of[sequence]
	return i, ok
}

func parents(nodes []Node) []int {
	parent := make([]int, len(nodes))
	var stack []int
	for i, n := range nodes {
		for len(stack) > 0 && nodes[stack[len(stack)-1]].Depth >= n.Depth {
			stack = stack[:len(stack)-1]
		}
		parent[i] = -1
		if len(stack) > 0 {
			parent[i] = stack[len(stack)-1]
		}
		stack = append(stack, i)
	}
	return parent
}

func childCounts(parent []int) []int {
	kids := make([]int, len(parent))
	for _, p := range parent {
		if p >= 0 {
			kids[p]++
		}
	}
	return kids
}

func hasChildren(nodes []Node, i int) bool {
	return i+1 < len(nodes) && nodes[i+1].Depth > nodes[i].Depth
}

// Depths are the depths at which the story of a file can be cut: those of the nodes that hold text
// and of the ones they are under.
func Depths(nodes []Node, segments []Segment) []int {
	parent := parents(nodes)
	seen := map[int]bool{}
	for _, s := range segments {
		for n := s.Node; n >= 0 && n < len(nodes); n = parent[n] {
			if nodes[n].Part == PartBody {
				seen[nodes[n].Depth] = true
			}
		}
	}
	out := make([]int, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Ints(out)
	return out
}

// BuildDivision cuts the story of a file at one depth: each unit is a node of that depth with
// everything under it (or a shallower node that has nothing under it). Text a node holds between its
// own children belongs to no unit, and neither does a division too small to be a chapter. Only the
// body of the book is divided: a preface or an appendix is not a chapter of anything.
func BuildDivision(nodes []Node, segments []Segment, depth int) *Division {
	parent := parents(nodes)
	kids := childCounts(parent)
	div := &Division{Depth: depth, of: map[int]int{}}
	byNode := map[int]int{}
	for _, seg := range segments {
		n := seg.Node
		if n < 0 || n >= len(nodes) || nodes[n].Part != PartBody {
			continue
		}
		u := n
		for nodes[u].Depth > depth && parent[u] >= 0 {
			u = parent[u]
		}
		if (nodes[u].Depth < depth && hasChildren(nodes, u)) || nodes[u].Part != PartBody {
			continue
		}
		i, ok := byNode[u]
		if !ok {
			i = len(div.Units)
			byNode[u] = i
			unit := Unit{Key: unitKey(u), Title: nodes[u].Title, FirstSequence: seg.Sequence, Locator: seg.Locator, Excerpt: Excerpt(seg.Text, 200)}
			unit.Number, unit.HasNumber = numberOfNode(nodes, parent, kids, u)
			div.Units = append(div.Units, unit)
		}
		div.Units[i].Chars += len(seg.Text)
		div.of[seg.Sequence] = i
	}

	// Too small to be a chapter: dropped, and the units after it move up.
	keep := make([]int, len(div.Units))
	var units []Unit
	for i, u := range div.Units {
		if u.Chars < minUnitChars {
			keep[i] = -1
			continue
		}
		keep[i] = len(units)
		units = append(units, u)
	}
	for seq, i := range div.of {
		if keep[i] < 0 {
			delete(div.of, seq)
		} else {
			div.of[seq] = keep[i]
		}
	}
	div.Units = units

	// Runs: the numbers go up through a book and start over at the next one (a sequel bound in the
	// same file), so where a number does not go up a new run begins. A division with no number just
	// before that point (the preface of the second book) opens the new run, it does not end the old one.
	run, previous, tail := 0, 0, -1 // tail: where the divisions with no number since the last one that had begin
	for i := range div.Units {
		u := &div.Units[i]
		if !u.HasNumber {
			if tail < 0 {
				tail = i
			}
			u.Run = run
			continue
		}
		if previous > 0 && u.Number <= previous {
			run++
			if tail >= 0 {
				for j := tail; j < i; j++ {
					div.Units[j].Run = run
				}
			}
		}
		previous = u.Number
		tail = -1
		u.Run = run
	}
	return div
}

func unitKey(node int) string { return "u" + strconv.Itoa(node) }

// numberOfNode is the number a node's title gives, or, when the title has only a name ("THE GUN
// CLUB.") and the chapter's number is in the entry it is the only child of ("CHAPTER I."), that
// one's. A part with several chapters does not lend its number to each of them.
func numberOfNode(nodes []Node, parent, kids []int, i int) (int, bool) {
	for n := i; n >= 0; n = parent[n] {
		if v, ok := ChapterNumber(nodes[n].Title); ok {
			return v, true
		}
		if parent[n] < 0 || kids[parent[n]] != 1 {
			break
		}
	}
	return 0, false
}

// runs lists the units of each run, in order.
func (d *Division) runs() [][]int {
	var out [][]int
	for i, u := range d.Units {
		for len(out) <= u.Run {
			out = append(out, nil)
		}
		out[u.Run] = append(out[u.Run], i)
	}
	return out
}

// Alignment says which unit of the destination is the one the source position is in.
type Alignment struct {
	Source, Dest *Division
	SourceUnit   int
	DestUnit     int
	Verified     bool    // the numbers in the titles agree, not only how many divisions there are
	Basis        string  // "count" or "number"
	Agreement    float64 // of the numbered divisions, the share whose numbers are the same
	Units        int     // how many divisions were matched: the finer, the better
}

// Alignments' bases.
const (
	BasisCount  = "count"
	BasisNumber = "number"
)

// Align finds where the story of the source and the story of the destination coincide, from the
// outlines of both: the two are cut at the depths where they have as many divisions as each other
// and the division the source position is in is matched, by its place, with the one at the same
// place in the other. Where the titles carry numbers they must say the same; where a book holds a
// second one (the numbers start over) only a run of the same length is matched. Nothing here looks
// at words: it says "the third chapter of a book of twenty-eight" and checks that against the
// numbers when there are any. It returns nil when the outlines do not say enough.
func Align(srcNodes []Node, srcSegments []Segment, dstNodes []Node, dstSegments []Segment, sourceSequence int) *Alignment {
	if len(srcNodes) == 0 || len(dstNodes) == 0 {
		return nil
	}
	var best *Alignment
	better := func(a *Alignment) bool {
		if best == nil {
			return true
		}
		if a.Verified != best.Verified {
			return a.Verified
		}
		return a.Units > best.Units
	}
	var dstDivisions []*Division // from the top of the outline down: a tie goes to the coarser cut
	for _, dd := range Depths(dstNodes, dstSegments) {
		dstDivisions = append(dstDivisions, BuildDivision(dstNodes, dstSegments, dd))
	}
	for _, ds := range Depths(srcNodes, srcSegments) {
		src := BuildDivision(srcNodes, srcSegments, ds)
		u, ok := src.UnitOf(sourceSequence)
		if !ok {
			continue
		}
		run := src.runs()[src.Units[u].Run]
		position := indexOf(run, u)
		if len(run) < minChaptersToCount {
			continue
		}
		for _, dst := range dstDivisions {
			var same [][]int
			for _, r := range dst.runs() {
				if len(r) == len(run) {
					same = append(same, r)
				}
			}
			if len(same) != 1 {
				continue // none the same length, or two that could be the one: no guess
			}
			verified, agreement, ok := checkNumbers(src, run, dst, same[0])
			if !ok {
				continue // the numbers say these are not the same divisions
			}
			a := &Alignment{Source: src, Dest: dst, SourceUnit: u, DestUnit: same[0][position], Verified: verified, Basis: BasisCount, Agreement: agreement, Units: len(run)}
			if better(a) {
				best = a
			}
		}
	}
	if best != nil {
		return best
	}
	return alignByNumber(srcNodes, srcSegments, dstNodes, dstSegments, sourceSequence, dstDivisions)
}

func indexOf(list []int, v int) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

// checkNumbers compares the numbers of two runs place by place. ok is false when they contradict
// each other; verified when enough of them are there to say so and they agree.
func checkNumbers(src *Division, srcRun []int, dst *Division, dstRun []int) (verified bool, agreement float64, ok bool) {
	covered, agree := 0, 0
	for i := range srcRun {
		a, b := src.Units[srcRun[i]], dst.Units[dstRun[i]]
		if a.HasNumber && b.HasNumber {
			covered++
			if a.Number == b.Number {
				agree++
			}
		}
	}
	if covered == 0 || float64(covered)/float64(len(srcRun)) < minNumberedShare {
		return false, 0, true // nothing to check the count against
	}
	agreement = float64(agree) / float64(covered)
	if agreement < minNumberAgreement {
		return false, agreement, false
	}
	return true, agreement, true
}

func countNumber(d *Division, number int) int {
	n := 0
	for _, u := range d.Units {
		if u.HasNumber && u.Number == number {
			n++
		}
	}
	return n
}

// alignByNumber is for versions with a different number of divisions (an abridgement): the division
// the source is in is matched with the one that carries the same number, if only one does.
func alignByNumber(srcNodes []Node, srcSegments []Segment, dstNodes []Node, dstSegments []Segment, sourceSequence int, dstDivisions []*Division) *Alignment {
	for _, ds := range Depths(srcNodes, srcSegments) {
		src := BuildDivision(srcNodes, srcSegments, ds)
		u, ok := src.UnitOf(sourceSequence)
		if !ok || !src.Units[u].HasNumber || countNumber(src, src.Units[u].Number) != 1 {
			continue // a number that a second book of the same file repeats says nothing
		}
		for _, dst := range dstDivisions {
			found := -1
			for i, d := range dst.Units {
				if d.HasNumber && d.Number == src.Units[u].Number {
					found = i
				}
			}
			if found >= 0 && countNumber(dst, src.Units[u].Number) == 1 {
				return &Alignment{Source: src, Dest: dst, SourceUnit: u, DestUnit: found, Basis: BasisNumber, Units: 1}
			}
		}
	}
	return nil
}

// Candidate is the chapter of the destination that the alignment points to, for when no passage is
// found inside it.
func (a *Alignment) Candidate() Candidate {
	unit := a.Dest.Units[a.DestUnit]
	total := 0
	for _, u := range a.Dest.Units {
		total += u.Chars
	}
	share := 0.0
	if total > 0 {
		share = float64(unit.Chars) / float64(total)
	}
	confidence := Medium
	switch {
	case a.Basis == BasisNumber:
		confidence = Low
	case a.Verified && share <= coarseUnitShare:
		confidence = High
	}
	// The opening of a part that is a third of the book is where the person is, at best, in the sense
	// of the part they are in: the outline is right and the place is not near. It is offered, and it
	// does not claim more than that.
	return Candidate{
		Precision: ChapterOnly, Confidence: confidence, Method: MethodStructure, Score: 1,
		Locator: unit.Locator, Section: unit.Title, Excerpt: unit.Excerpt,
		Evidence: map[string]any{
			"reason": "same place in the outline", "basis": a.Basis, "divisions": a.Units,
			"numbersAgree": a.Verified, "from": a.Source.Units[a.SourceUnit].Title, "unitShare": math.Round(share*1000) / 1000,
		},
		sequence: unit.FirstSequence, chapter: unit.Key,
	}
}

// Sequence is where the candidate is, in the reading order of the destination.
func (c Candidate) Sequence() int { return c.sequence }

// Verdict is what the way back says about a candidate.
type Verdict int

const (
	Unknown   Verdict = iota // the way back found nothing to compare: nothing was learned
	Agrees                   // from the candidate, the source passage comes out
	Disagrees                // from the candidate, some other passage comes out
)

// Reverse walks a candidate back: starting from the passage it points to, it looks in the source for
// where that passage is. A candidate that only looks right from the source (a page that names every
// character) does not lead back to where the person was.
func Reverse(original Segment, candidate Segment, sourceSegments []Segment) Verdict {
	back := ByPassage(candidate, sourceSegments)
	if len(back) == 0 {
		back = ByAnchors(candidate, sourceSegments)
	}
	if len(back) == 0 {
		return Unknown
	}
	for _, b := range back {
		if abs(b.sequence-original.Sequence) <= adjacentSegments+1 {
			return Agrees
		}
	}
	return Disagrees
}
