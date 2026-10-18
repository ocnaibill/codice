package handlers

import (
	"reflect"
	"testing"
	"time"
)

func f(n float64) *float64 { return &n }

func chap(id int, n float64) storyItem { return storyItem{ID: id, Unit: "chapter", Position: f(n)} }
func vol(id int, n, from, to float64) storyItem {
	return storyItem{ID: id, Unit: "volume", Position: f(n), From: f(from), To: f(to)}
}

func TestStory_TheListFollowsTheStoryAndAVolumeEntersWhereItsRangeBegins(t *testing.T) {
	// The mixed series of the maintainer: caps 1, 2, then a volume with 3 to 7, then 8; the volume lists before the chapters it contains.
	st := buildStory([]storyItem{chap(1, 1), chap(2, 2), vol(30, 1, 3, 7), chap(8, 8), chap(4, 4), chap(5, 5)})
	if want := []int{1, 2, 30, 8}; !reflect.DeepEqual(st.Order, want) {
		t.Errorf("order = %v, want %v", st.Order, want)
	}
	if want := []int{4, 5}; !reflect.DeepEqual(st.Nested[30], want) {
		t.Errorf("nested under the volume = %v, want %v", st.Nested[30], want)
	}
	if !st.Ranged {
		t.Errorf("a series with a range is ranged")
	}
}

func TestStory_NeverVolumesFirstAndTheChaptersBetweenThemAfter(t *testing.T) {
	// Vol 1 (1-9), Vol 2 (13-15) and the chapters 10, 11, 12 in between: the story is Vol 1, 10, 11, 12, Vol 2.
	st := buildStory([]storyItem{vol(101, 1, 1, 9), vol(102, 2, 13, 15), chap(10, 10), chap(11, 11), chap(12, 12)})
	if want := []int{101, 10, 11, 12, 102}; !reflect.DeepEqual(st.Order, want) {
		t.Errorf("order = %v, want %v", st.Order, want)
	}
	if len(st.Gaps) != 0 {
		t.Errorf("gaps = %v, nothing is missing", st.Gaps)
	}
}

func TestStory_AVolumeThatSaysNothingHasNoPlaceAndTheComplementaryOnesAreNotInIt(t *testing.T) {
	loose := storyItem{ID: 50, Unit: "volume", Position: f(3)}
	extra := storyItem{ID: 60, Unit: unitExtra, Position: f(1)}
	st := buildStory([]storyItem{chap(1, 1), loose, extra, chap(2, 2)})
	if !reflect.DeepEqual(st.Order, []int{1, 2}) || !reflect.DeepEqual(st.Unplaced, []int{50}) {
		t.Errorf("order %v unplaced %v", st.Order, st.Unplaced)
	}
	if st.Ranged {
		t.Errorf("no work has a range: the page keeps its old order and its old way of going on")
	}
}

func TestStory_TheGapsAreWhatNoFileContainsAndAVolumeFillsThem(t *testing.T) {
	st := buildStory([]storyItem{vol(1, 1, 1, 9), vol(2, 3, 18, 26), chap(31, 31), chap(32, 32)})
	want := []NumberRange{{10, 17}, {27, 30}}
	if !reflect.DeepEqual(st.Gaps, want) {
		t.Errorf("gaps = %v, want %v", st.Gaps, want)
	}
	if st.Chapters != 9+9+2 {
		t.Errorf("chapters = %d, want 20", st.Chapters)
	}
}

func TestStory_AChapterIsReadWhenItOrAVolumeThatContainsItIsFinished(t *testing.T) {
	v := vol(10, 1, 1, 5)
	v.Done = true
	c := chap(8, 8)
	c.Done = true
	st := buildStory([]storyItem{v, chap(3, 3), c, chap(9, 9)})
	if st.Chapters != 7 || st.Read != 6 { // 1..5, 8, 9
		t.Errorf("chapters %d read %d, want 7 and 6", st.Chapters, st.Read)
	}
	if want := []NumberRange{{1, 5}, {8, 8}}; !reflect.DeepEqual(st.ReadSpans, want) {
		t.Errorf("read spans = %v, want %v", st.ReadSpans, want)
	}
	// reading the chapter 3 itself changes nothing in the volume: it was already counted
	if st.Continue != 9 {
		t.Errorf("continue = %d, want the chapter 9, the first not read that exists (6 and 7 do not)", st.Continue)
	}
}

func TestStory_ReadingAVolumeDoesNotSendYouBackToTheChaptersItContains(t *testing.T) {
	v := vol(10, 3, 18, 26)
	v.Done = true
	st := buildStory([]storyItem{chap(18, 18), chap(19, 19), v, chap(31, 31)})
	if st.Continue != 31 {
		t.Errorf("continue = %d, want the chapter 31", st.Continue)
	}
	if st.Read != 9 || st.Chapters != 10 {
		t.Errorf("read %d of %d, want 9 of 10", st.Read, st.Chapters)
	}
}

func TestStory_GoingOnPrefersWhatWasBeguneThenTheUnitLastRead(t *testing.T) {
	now := time.Now()
	then := now.Add(-time.Hour)
	begun := chap(5, 5)
	begun.Started, begun.LastAt = true, &now
	if got := buildStory([]storyItem{chap(1, 1), begun, vol(9, 1, 6, 9)}).Continue; got != 5 {
		t.Errorf("the one begun: %d, want 5", got)
	}
	// Nothing begun: the first chapter not read is 4, held by the volume 4-7 and by the loose chapter 4. The caller read a volume last.
	readVol := vol(8, 1, 1, 3)
	readVol.Done, readVol.LastAt = true, &then
	if got := buildStory([]storyItem{readVol, vol(9, 2, 4, 7), chap(4, 4)}).Continue; got != 9 {
		t.Errorf("same unit as the last read: %d, want the volume 9", got)
	}
	readChap := chap(1, 1)
	readChap.Done, readChap.LastAt = true, &then
	if got := buildStory([]storyItem{readChap, vol(9, 2, 4, 7), chap(4, 4), chap(2, 2), chap(3, 3)}).Continue; got != 2 {
		t.Errorf("first not read: %d, want 2", got)
	}
	if got := buildStory([]storyItem{readChap}).Continue; got != 0 {
		t.Errorf("everything read: %d, want 0", got)
	}
}

func TestStory_ADecimalChapterIsAChapterAndARangeMayBeginAtOne(t *testing.T) {
	st := buildStory([]storyItem{chap(1, 10), chap(2, 10.5), chap(3, 11), {ID: 4, Unit: "chapter", Position: f(12), From: f(12), To: f(13)}})
	if st.Chapters != 5 || len(st.Gaps) != 0 {
		t.Errorf("chapters %d gaps %v", st.Chapters, st.Gaps)
	}
	if want := []int{1, 2, 3, 4}; !reflect.DeepEqual(st.Order, want) {
		t.Errorf("order = %v", st.Order)
	}
}

func TestStory_TheCompleteEditionHoldsTheLooseChaptersAndStaysBesideTheVolumes(t *testing.T) {
	st := buildStory([]storyItem{vol(100, 1, 1, 12), vol(101, 1, 1, 6), chap(1, 1), chap(2, 2), chap(8, 8)})
	if want := []int{100, 101}; !reflect.DeepEqual(st.Order, want) {
		t.Errorf("order = %v, want the complete edition, then the volume that begins in the same place", st.Order)
	}
	if want := []int{1, 2, 8}; !reflect.DeepEqual(st.Nested[100], want) {
		t.Errorf("nested = %v, want the loose chapters under the complete edition", st.Nested[100])
	}
}
