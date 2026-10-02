package equivalence

import "strings"

// File is what is known of one file's text: its segments, in reading order, and the nodes of its
// outline when the worker found one (DEC-087).
type File struct {
	Segments []Segment
	Nodes    []Node
}

func (f File) hasOutline() bool { return len(f.Nodes) > 0 }

// Chapters groups the segments by their Chapter key, in the order they first appear, and says which
// chapter each segment (by sequence) is in. A segment with no key is in none. This is the division of
// a file with no outline, which is the file's own (an EPUB's files, a Markdown file's headings).
func Chapters(segments []Segment) ([]Chapter, map[int]int) {
	var list []Chapter
	index := map[string]int{}
	segmentChapter := map[int]int{}
	for _, s := range segments {
		if s.Chapter == "" {
			continue
		}
		i, ok := index[s.Chapter]
		if !ok {
			list = append(list, Chapter{Key: s.Chapter, Title: s.Section, FirstSequence: s.Sequence, Locator: s.Locator, Excerpt: Excerpt(s.Text, 200)})
			i = len(list) - 1
			index[s.Chapter] = i
		}
		segmentChapter[s.Sequence] = i
	}
	return list, segmentChapter
}

func raise(confidence string) string {
	if confidence == Low {
		return Medium
	}
	return High
}

func lower(confidence string) string {
	switch confidence {
	case High:
		return Medium
	case Medium:
		return Low
	}
	return ""
}

// Find says where in dst the passage that source (a segment of src) is in can be found: the whole of
// the way from the outline down to the words, and back.
//
//  1. The outlines, if both files have one, say which division of the story of dst matches the one
//     source is in (Align). Only the body of a book is compared with the body, and a preface with a
//     preface: an appendix that names every character never competes with a chapter.
//  2. Inside that division, when the numbers in the titles confirm it, the passage is looked for by its
//     words (ByPassage) and, in a translation, by its names and numbers (ByAnchors).
//  3. What was found is walked back (Reverse): a place that does not lead back to where the person was
//     is dropped or trusted less.
//  4. What the outline alone says (the chapter) is what is offered when no passage was found in it.
func Find(src, dst File, source Segment) Answer {
	want := PartBody // where the person is, when the file does not say
	if src.hasOutline() && source.Part != "" {
		want = source.Part
	}
	var aligned *Alignment
	if src.hasOutline() && dst.hasOutline() && want == PartBody {
		aligned = Align(src.Nodes, src.Segments, dst.Nodes, dst.Segments, source.Sequence)
	}

	pool := destinationPool(dst, want, aligned)
	back := sourcePool(src, want, aligned)
	bySequence := make(map[int]Segment, len(dst.Segments))
	for _, s := range dst.Segments {
		bySequence[s.Sequence] = s
	}

	// A candidate is walked back from the passage it points to. For names, the passage and the way back are
	// read with their neighbours (windows), for the same reason the search is.
	windowedPool, windowedBack := windows(pool), windows(back)
	windowedBySequence := make(map[int]Segment, len(windowedPool))
	for _, s := range windowedPool {
		windowedBySequence[s.Sequence] = s
	}
	sourceWindow := windowAround(source, sequences(src.Segments))
	verify := func(list []Candidate, names bool) []Candidate {
		var out []Candidate
		for _, c := range list {
			var verdict Verdict
			if names {
				verdict = Reverse(sourceWindow, windowedBySequence[c.sequence], windowedBack)
			} else {
				verdict = Reverse(source, bySequence[c.sequence], back)
			}
			switch verdict {
			case Agrees:
				c.Evidence["reverse"] = "agrees"
				if aligned != nil && aligned.Verified {
					c.Confidence = raise(c.Confidence)
				}
			case Disagrees:
				if c.Method == MethodAnchors {
					continue // names alone, and the way back does not lead here: not it
				}
				c.Evidence["reverse"] = "disagrees"
				if c.Confidence = lower(c.Confidence); c.Confidence == "" {
					continue
				}
			}
			out = append(out, c)
		}
		return out
	}

	passages := verify(ByPassage(source, pool), false)
	var anchors []Candidate
	if len(passages) == 0 {
		anchors = verify(ByAnchors(sourceWindow, windowedPool), true)
		if len(anchors) == 0 {
			anchors = BySemantic(source, pool, back)
		}
	}

	var chapter *Candidate
	switch {
	case aligned != nil:
		// Structure alone is useful as a fallback only when it can place the reader near their
		// position inside a small, numbered chapter. The beginning of a large part is not a useful
		// answer merely because the outline identifies that part.
		chapter = aligned.Approximate(source.Sequence)
	case !src.hasOutline() && !dst.hasOutline():
		// No outline on either side: the divisions are the files' own, and a file is not always a
		// chapter (one may hold ten), so what they say is not trusted as far as an outline is.
		srcChapters, srcIndex := Chapters(src.Segments)
		dstChapters, _ := Chapters(dst.Segments)
		if i, ok := srcIndex[source.Sequence]; ok {
			if c := ByStructure(i, srcChapters, dstChapters); c != nil {
				if c.Confidence == High {
					c.Confidence = Medium
				}
				chapter = c
			}
		}
	}
	return Combine(passages, anchors, chapter)
}

// destinationPool is what of the destination is looked in.
func destinationPool(dst File, want string, aligned *Alignment) []Segment {
	out := make([]Segment, 0, len(dst.Segments))
	for _, s := range dst.Segments {
		if dst.hasOutline() {
			if s.Part != want {
				continue
			}
			if aligned != nil {
				u, in := aligned.Dest.UnitOf(s.Sequence)
				switch {
				case in:
					s.Chapter = aligned.Dest.Units[u].Key
					if aligned.Verified && u != aligned.DestUnit {
						continue
					}
				case aligned.Verified:
					continue
				default:
					s.Chapter = ""
				}
			}
		}
		out = append(out, s)
	}
	return out
}

// sourcePool is what of the source the way back looks in.
func sourcePool(src File, want string, aligned *Alignment) []Segment {
	out := make([]Segment, 0, len(src.Segments))
	for _, s := range src.Segments {
		if src.hasOutline() && s.Part != want {
			continue
		}
		if aligned != nil && aligned.Verified {
			if u, in := aligned.Source.UnitOf(s.Sequence); !in || u != aligned.SourceUnit {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// windowRadius is how many segments on each side are read with a segment when names are compared: a
// passage is cut by a page or a chunk in a different place in every edition, so one segment holds only
// part of the names of the passage, and three hold most of them.
const windowRadius = 1

// windowAround is a copy of the segment with the text of its neighbours (the segments of the file at
// sequence-1 and sequence+1) added, in order. The address, section and chapter are the segment's own.
func windowAround(s Segment, bySequence map[int]Segment) Segment {
	var text []string
	for n := s.Sequence - windowRadius; n <= s.Sequence+windowRadius; n++ {
		if n == s.Sequence {
			text = append(text, s.Text)
		} else if near, ok := bySequence[n]; ok {
			text = append(text, near.Text)
		}
	}
	s.own = s.Text
	s.Text = strings.Join(text, "\n")
	return s
}

// windows is the pool as segments with their neighbours' text, for comparing names. A neighbour is one
// of the pool: what the pool leaves out (another chapter, a preface) does not lend names.
func windows(pool []Segment) []Segment {
	bySequence := make(map[int]Segment, len(pool))
	for _, s := range pool {
		bySequence[s.Sequence] = s
	}
	out := make([]Segment, len(pool))
	for i, s := range pool {
		out[i] = windowAround(s, bySequence)
	}
	return out
}

func sequences(segments []Segment) map[int]Segment {
	out := make(map[int]Segment, len(segments))
	for _, s := range segments {
		out[s.Sequence] = s
	}
	return out
}
