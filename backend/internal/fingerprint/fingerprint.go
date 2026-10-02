// Package fingerprint tells whether two files hold the same text (#38), however they are made: an EPUB and a
// PDF of one book, two editions of one translation. It looks at the words only, never at a title or an author.
//
// A file is reduced to a sample of its text: every run of ShingleWords consecutive words (folded: case, accents
// and punctuation do not count) is hashed, and one hash in SampleModulus is kept. Two files of the same text
// keep nearly the same hashes, whatever their pages, chapters or hyphens; files that merely resemble each other
// (a sequel, a book of the same author) keep almost none, because a run of eight words does not repeat by
// chance. Only the body of the book is read: a licence or a note that every file of a source carries would
// otherwise make all of them look alike.
//
// This is a sample, so it is small (a book of a hundred thousand words keeps about three thousand numbers),
// and it can be compared, and found by a database index, without the text.
package fingerprint

import (
	"hash/fnv"
	"sort"
)

const (
	// Version is the version of the method. A fingerprint made by another version is made again.
	Version = 1
	// ShingleWords is how many consecutive words make one run that is hashed.
	ShingleWords = 8
	// SampleModulus: a run is kept when its hash is a multiple of this.
	SampleModulus = 32
	// MinHashes is the smallest sample that is trusted: with fewer, a few passages in common (a quotation, a
	// shared preface) would be most of the text. About 3,000 words.
	MinHashes = 100
	// MaxHashes bounds the sample of a huge file.
	MaxHashes = 40000
	// SameContainment is the share of each file's sample that the other must hold for them to be the same text.
	// Unrelated books share under one in a hundred, two editions of one text or an EPUB and a PDF of it share
	// four in five or more, and a book inside a collection of two books of its size holds half of it: the limit
	// sits between the last two, so a collection is never taken for its book.
	SameContainment = 0.7
)

// Of is the fingerprint of a text given as its words (see equivalence.Words): the sampled hashes, sorted and
// without repetition. It returns nil when the text is too short to be trusted.
func Of(words []string) []int64 {
	seen := map[uint64]struct{}{}
	for i := 0; i+ShingleWords <= len(words); i++ {
		if sum := hashRun(words[i : i+ShingleWords]); sum%SampleModulus == 0 {
			seen[sum] = struct{}{}
		}
	}
	if len(seen) < MinHashes {
		return nil
	}
	out := make([]int64, 0, len(seen))
	for sum := range seen {
		out = append(out, int64(sum))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	if len(out) > MaxHashes {
		out = out[:MaxHashes]
	}
	return out
}

// hashRun is the hash of a run of words. The words are told apart (a zero byte ends each), so that "ab cd" and
// "a bcd" are not the same run.
func hashRun(words []string) uint64 {
	h := fnv.New64a()
	for _, w := range words {
		h.Write([]byte(w))
		h.Write([]byte{0})
	}
	// The low bits of FNV-1a depend only on the low bits of each byte, so a sample taken by them (a multiple of
	// SampleModulus) would be thin or empty for some texts. This mixes every bit into every other (the finalizer of
	// MurmurHash3).
	x := h.Sum64()
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

// Overlap is how two fingerprints compare.
type Overlap struct {
	Shared int     // hashes in both
	OfA    float64 // the share of A's hashes that B holds
	OfB    float64 // the share of B's hashes that A holds
}

// Compare counts the hashes two sorted fingerprints share.
func Compare(a, b []int64) Overlap {
	shared := 0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			shared++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	o := Overlap{Shared: shared}
	if len(a) > 0 {
		o.OfA = float64(shared) / float64(len(a))
	}
	if len(b) > 0 {
		o.OfB = float64(shared) / float64(len(b))
	}
	return o
}

// Same says whether the two files are the same text: each holds at least SameContainment of what the other has. A file
// that holds most of another but is a small part of it (a volume inside a collection) is not the same text.
func (o Overlap) Same() bool {
	return o.OfA >= SameContainment && o.OfB >= SameContainment
}

// Contains says that one file holds most of the other and not the other way round: the smaller is part of the
// larger (a volume of a collection, an excerpt). It is not the same text and is not proposed as such.
func (o Overlap) Contains() bool {
	return !o.Same() && (o.OfA >= SameContainment || o.OfB >= SameContainment)
}
