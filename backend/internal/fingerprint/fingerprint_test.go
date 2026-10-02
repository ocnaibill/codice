package fingerprint

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// book is n distinct words, in an order seeded by `seed`: a text no other seed repeats by chance.
func book(seed int64, n int) []string {
	r := rand.New(rand.NewSource(seed))
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("w%d", r.Intn(4000))
	}
	return out
}

func TestOf_TheSampleIsAboutOneInThirtyTwoWhateverTheWordsAre(t *testing.T) {
	// Words that are a name and a digit, in a stock of phrases: where the low bits of an unmixed FNV-1a hash are not
	// spread (it keeps half of what it should).
	phrases := make([][]string, 1000)
	for i := range phrases {
		for j := 0; j < 7; j++ {
			phrases[i] = append(phrases[i], fmt.Sprintf("p%dw%d", i, j))
		}
	}
	r := rand.New(rand.NewSource(1))
	var words []string
	for len(words) < 28000 {
		words = append(words, phrases[r.Intn(len(phrases))]...)
	}
	// About 27,970 distinct runs of eight words: about 874 kept.
	if got := len(Of(words)); got < 780 || got > 970 {
		t.Errorf("%d hashes, about 874 expected", got)
	}
}

func TestOf_IsSortedWithoutRepetitionAndAboutOneInThirtyTwo(t *testing.T) {
	got := Of(book(1, 100000))
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Fatal("not sorted")
	}
	for i := 1; i < len(got); i++ {
		if got[i] == got[i-1] {
			t.Fatal("repeated hash")
		}
	}
	// 100,000 runs, one in 32 kept: about 3,100.
	if len(got) < 2600 || len(got) > 3600 {
		t.Errorf("%d hashes", len(got))
	}
}

func TestOf_TheSameWordsGiveTheSameHashesAndAnotherTextOtherOnes(t *testing.T) {
	a, again, other := Of(book(1, 30000)), Of(book(1, 30000)), Of(book(2, 30000))
	if o := Compare(a, again); o.OfA != 1 || o.OfB != 1 || !o.Same() {
		t.Errorf("same text: %+v", o)
	}
	if o := Compare(a, other); o.Same() || o.Contains() || o.OfA > 0.02 {
		t.Errorf("another text: %+v", o)
	}
}

func TestOf_TooLittleTextIsNotFingerprinted(t *testing.T) {
	if got := Of(book(1, 2000)); got != nil {
		t.Errorf("%d hashes from 2,000 words", len(got))
	}
	if got := Of(nil); got != nil {
		t.Error("nothing")
	}
	if got := Of(book(1, 100)); got != nil {
		t.Error("fewer words than a run")
	}
	if got := Of(book(1, 4000)); got == nil {
		t.Error("4,000 words is enough")
	}
}

func TestOf_ABoundOnAHugeFile(t *testing.T) {
	words := book(3, 1500000)
	if got := Of(words); len(got) != MaxHashes {
		t.Errorf("%d hashes", len(got))
	}
}

func TestHashRun_TheBoundariesBetweenWordsCount(t *testing.T) {
	// "ab cd" and "a bcd" are not the same two words.
	if hashRun([]string{"ab", "cd"}) == hashRun([]string{"a", "bcd"}) {
		t.Error("the words ran together")
	}
	if hashRun([]string{"a", "b"}) == hashRun([]string{"b", "a"}) {
		t.Error("the order does not count")
	}
	if hashRun([]string{"a", "b"}) != hashRun([]string{"a", "b"}) {
		t.Error("not deterministic")
	}
}

func TestOf_RunsOfEightWordsDoNotRepeatByChanceWhereRunsOfFourDo(t *testing.T) {
	// Two books made of the same stock of seven-word phrases in another order. Every run of four words inside a
	// phrase is in both books (more than half of all of them); a run of eight crosses from one phrase into the next,
	// which is what a text is made of, and is in both by chance a few times in a thousand.
	phrases := make([][]string, 1000)
	for i := range phrases {
		for j := 0; j < 7; j++ {
			phrases[i] = append(phrases[i], fmt.Sprintf("p%dw%d", i, j))
		}
	}
	made := func(seed int64) []string {
		r := rand.New(rand.NewSource(seed))
		var out []string
		for len(out) < 28000 {
			out = append(out, phrases[r.Intn(len(phrases))]...)
		}
		return out
	}
	if o := Compare(Of(made(1)), Of(made(2))); o.OfA > 0.05 || o.OfB > 0.05 || o.Same() {
		t.Errorf("two books of the same stock phrases: %+v", o)
	}
}

// edition is the words of a book with a share of them changed, as another edition of the same text.
func edition(words []string, every int) []string {
	out := append([]string{}, words...)
	for i := every; i < len(out); i += every {
		out[i] = "changed"
	}
	return out
}

func TestCompare_AnEditionWithSomeWordsChangedIsStillTheSameText(t *testing.T) {
	words := book(5, 80000)
	o := Compare(Of(words), Of(edition(words, 40))) // one word in forty: one run in five is touched
	if !o.Same() || o.OfA < 0.7 {
		t.Errorf("%+v", o)
	}
}

func TestCompare_ABookThatIsOnlyPartOfAnotherIsNotTheSameText(t *testing.T) {
	words := book(6, 100000)
	whole, part := Of(words), Of(words[:30000])
	o := Compare(whole, part)
	if o.Same() || !o.Contains() {
		t.Errorf("a part: %+v", o)
	}
	if o.OfB < 0.99 || o.OfA > 0.35 || o.OfA < 0.2 {
		t.Errorf("the whole holds the part, the part is a third of the whole: %+v", o)
	}
	reverse := Compare(part, whole)
	if reverse.Same() || !reverse.Contains() || reverse.OfA != o.OfB {
		t.Errorf("it does not depend on the order: %+v", reverse)
	}
}

func TestCompare_TheSharesAreEachOfItsOwnFile(t *testing.T) {
	a := []int64{1, 2, 3, 4}
	b := []int64{3, 4, 5, 6, 7, 8, 9, 10}
	o := Compare(a, b)
	if o.Shared != 2 || o.OfA != 0.5 || o.OfB != 0.25 {
		t.Errorf("%+v", o)
	}
	if got := Compare(nil, b); got.Shared != 0 || got.OfA != 0 || got.OfB != 0 {
		t.Errorf("%+v", got)
	}
}

func TestOverlap_SameNeedsMostOfEachAndContainsNeedsMostOfOneSide(t *testing.T) {
	for _, c := range []struct {
		o              Overlap
		same, contains bool
	}{
		{Overlap{OfA: 0.9, OfB: 0.88}, true, false},
		{Overlap{OfA: 0.7, OfB: 0.7}, true, false},
		{Overlap{OfA: 0.69, OfB: 0.9}, false, true},
		{Overlap{OfA: 0.9, OfB: 0.2}, false, true},
		{Overlap{OfA: 0.5, OfB: 1}, false, true}, // a book in a collection of two books of its size
		{Overlap{OfA: 0.69, OfB: 0.69}, false, false},
		{Overlap{OfA: 0.01, OfB: 0.01}, false, false},
	} {
		if c.o.Same() != c.same || c.o.Contains() != c.contains {
			t.Errorf("%+v: same %v contains %v", c.o, c.o.Same(), c.o.Contains())
		}
	}
}

func TestCompare_ABookInACollectionOfTwoBooksOfItsSizeIsNotTheSameText(t *testing.T) {
	one := book(8, 40000)
	collection := append(append([]string{}, one...), book(9, 40000)...)
	o := Compare(Of(one), Of(collection))
	if o.Same() || !o.Contains() || o.OfB < 0.4 || o.OfB > 0.6 {
		t.Errorf("%+v", o)
	}
}
