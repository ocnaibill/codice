package equivalence

import (
	"encoding/json"
	"sort"
)

// The first guesses (QA-028: to be measured, on a corpus with reorganized chapters, translations,
// prefaces, abridged versions and OCR errors, before anyone trusts them).
const (
	shingleSize        = 4    // words in a sequence
	minSourceShingles  = 8    // less than this is too little text to compare
	minSharedShingles  = 6    // and at least this many must be found
	highOverlap        = 0.60 // of the source's sequences found in the candidate
	mediumOverlap      = 0.30
	minAnchors         = 4 // names and numbers in the source passage
	minSharedAnchors   = 3
	mediumAnchorShare  = 0.60 // below it, low confidence
	minAnchorShare     = 0.40
	minChaptersToCount = 3
	maxTextCandidates  = 3
	adjacentSegments   = 1 // a candidate this close to a better one is the same passage
	excerptChars       = 240
)

// Confidence levels, from the least to the most sure.
const (
	Low    = "low"
	Medium = "medium"
	High   = "high"
)

// Methods, and the precision of what they point to.
const (
	MethodText      = "text"      // the passage around the position was found in the destination
	MethodStructure = "structure" // the chapter was matched by its title, its number or its place
	MethodAnchors   = "anchors"   // names and numbers of the passage were found together
	Passage         = "passage"
	ChapterOnly     = "chapter"
)

// Segment is a piece of a file's text with its address.
type Segment struct {
	ID       int64
	Sequence int
	Section  string
	Chapter  string // the key of the chapter it is in (see Chapter.Key)
	Text     string
	Locator  json.RawMessage
	Node     int    // the node of the file's outline it is in (see Node); only meaningful with an outline
	Part     string // front, body or back, from the outline; empty when the file has none
}

// Candidate is a place in the destination that may be the one.
type Candidate struct {
	Precision  string          `json:"precision"`
	Confidence string          `json:"confidence"`
	Method     string          `json:"method"`
	Score      float64         `json:"score"`
	Locator    json.RawMessage `json:"locator"`
	Section    string          `json:"section,omitempty"`
	Excerpt    string          `json:"excerpt"`
	Evidence   map[string]any  `json:"evidence"`

	sequence int
	chapter  string
}

func rank(confidence string) int {
	switch confidence {
	case High:
		return 3
	case Medium:
		return 2
	}
	return 1
}

// ByPassage compares the source passage with the segments of the destination that the database
// found likely, and keeps those that share enough of its word sequences.
func ByPassage(source Segment, candidates []Segment) []Candidate {
	sourceShingles := Shingles(Words(source.Text), shingleSize)
	if len(sourceShingles) < minSourceShingles {
		return nil
	}
	var out []Candidate
	for _, c := range candidates {
		n, share := Shared(sourceShingles, Shingles(Words(c.Text), shingleSize))
		if n < minSharedShingles || share < mediumOverlap {
			continue
		}
		confidence := Medium
		if share >= highOverlap {
			confidence = High
		}
		out = append(out, Candidate{
			Precision: Passage, Confidence: confidence, Method: MethodText, Score: share,
			Locator: c.Locator, Section: c.Section, Excerpt: Excerpt(c.Text, excerptChars),
			Evidence: map[string]any{"sharedSequences": n, "sourceSequences": len(sourceShingles)},
			sequence: c.Sequence, chapter: c.Chapter,
		})
	}
	return keepBest(out)
}

// ByAnchors is for versions that share no wording, a translation: it looks for the names and numbers
// of the source passage together in one segment of the destination.
func ByAnchors(source Segment, candidates []Segment) []Candidate {
	sourceAnchors := Anchors(source.Text)
	if len(sourceAnchors) < minAnchors {
		return nil
	}
	var out []Candidate
	for _, c := range candidates {
		found := Anchors(c.Text)
		n := 0
		for a := range sourceAnchors {
			if _, ok := found[a]; ok {
				n++
			}
		}
		share := float64(n) / float64(len(sourceAnchors))
		if n < minSharedAnchors || share < minAnchorShare {
			continue
		}
		confidence := Low
		if share >= mediumAnchorShare {
			confidence = Medium
		}
		out = append(out, Candidate{
			Precision: Passage, Confidence: confidence, Method: MethodAnchors, Score: share,
			Locator: c.Locator, Section: c.Section, Excerpt: Excerpt(c.Text, excerptChars),
			Evidence: map[string]any{"sharedNames": n, "sourceNames": len(sourceAnchors)},
			sequence: c.Sequence, chapter: c.Chapter,
		})
	}
	return keepBest(out)
}

// keepBest orders candidates by score, drops one that is the neighbour of a better one (the same
// passage, cut by a page or a chunk) and keeps a few.
func keepBest(in []Candidate) []Candidate {
	sort.SliceStable(in, func(i, j int) bool { return in[i].Score > in[j].Score })
	var out []Candidate
	for _, c := range in {
		near := false
		for _, kept := range out {
			if abs(kept.sequence-c.sequence) <= adjacentSegments {
				near = true
				break
			}
		}
		if !near {
			out = append(out, c)
		}
		if len(out) == maxTextCandidates {
			break
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ByStructure matches the chapter the source position is in with a chapter of the destination: by
// its title when the two are called the same and only one is, by the number in "Capítulo 3", and,
// failing both, by its place when both versions have the same number of chapters.
func ByStructure(index int, source, destination []Chapter) *Candidate {
	if index < 0 || index >= len(source) || len(destination) == 0 {
		return nil
	}
	for i := range source {
		source[i].prepare()
	}
	for i := range destination {
		destination[i].prepare()
	}
	src := source[index]
	build := func(c Chapter, confidence, why string, evidence map[string]any) *Candidate {
		evidence["reason"] = why
		return &Candidate{
			Precision: ChapterOnly, Confidence: confidence, Method: MethodStructure, Score: 1,
			Locator: c.Locator, Section: c.Title, Excerpt: c.Excerpt, Evidence: evidence,
			sequence: c.FirstSequence, chapter: c.Key,
		}
	}

	if only := unique(destination, func(c Chapter) bool { return SameTitle(src.Title, c.Title) }); only != nil {
		return build(*only, High, "same title", map[string]any{"title": src.Title})
	}
	if src.hasNumber {
		if only := unique(destination, func(c Chapter) bool { return c.hasNumber && c.number == src.number }); only != nil {
			return build(*only, High, "same chapter number", map[string]any{"number": src.number})
		}
	}
	if EqualCount(source, destination) {
		return build(destination[index], Low, "same number of chapters", map[string]any{"chapters": len(destination), "position": index + 1})
	}
	return nil
}

func unique(chapters []Chapter, match func(Chapter) bool) *Chapter {
	var found *Chapter
	for i := range chapters {
		if match(chapters[i]) {
			if found != nil {
				return nil // two chapters answer: it is not a match
			}
			found = &chapters[i]
		}
	}
	return found
}

// Answer is what is offered to the person: found (the first candidate is the place), ambiguous (they
// choose) or not found (nothing was located, and nothing is made up).
type Answer struct {
	Status     string
	Candidates []Candidate
}

const (
	Found     = "found"
	Ambiguous = "ambiguous"
	NotFound  = "not_found"
)

// Combine puts the evidence of the three methods together. A passage found in the very chapter that
// the structure points to is one answer with two reasons; a chapter the passages do not mention is
// a candidate of its own; names and numbers are only used when the wording gave nothing.
func Combine(passages, anchors []Candidate, chapter *Candidate) Answer {
	all := append([]Candidate{}, passages...)
	if len(passages) == 0 {
		all = append(all, anchors...)
	}
	if chapter != nil {
		agreed := false
		for i := range all {
			if all[i].chapter != "" && all[i].chapter == chapter.chapter {
				all[i].Evidence["chapterAgrees"] = true
				agreed = true
			}
		}
		if !agreed {
			all = append(all, *chapter)
		}
	}
	if len(all) == 0 {
		return Answer{Status: NotFound}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if rank(all[i].Confidence) != rank(all[j].Confidence) {
			return rank(all[i].Confidence) > rank(all[j].Confidence)
		}
		return all[i].Score > all[j].Score
	})
	if len(all) == 1 {
		return Answer{Status: Found, Candidates: all}
	}
	first, second := all[0], all[1]
	switch {
	case rank(first.Confidence) > rank(second.Confidence):
		return Answer{Status: Found, Candidates: all}
	case first.chapter != "" && first.chapter == second.chapter:
		return Answer{Status: Found, Candidates: all} // two views of the same chapter
	}
	return Answer{Status: Ambiguous, Candidates: all}
}
