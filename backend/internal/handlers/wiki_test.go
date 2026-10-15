package handlers

import (
	"testing"

	"github.com/ocnaibill/codice/backend/internal/wiki"
)

// A share of the book read in another file (another edition) never shows a chapter the reader may not have finished.
func TestShareOf_RoundsDownToAChapterReadWhole(t *testing.T) {
	chapters := []wiki.ChapterRow{{Chapter: 1}, {Chapter: 2}, {Chapter: 3}, {Chapter: 4}, {Chapter: 5, Editorial: true}}
	for percent, want := range map[float64]int{0: 0, 0.2: 0, 0.25: 1, 0.74: 2, 1: 4, 50: 2, 100: 4} {
		if got := shareOf(chapters, percent); got != want {
			t.Errorf("shareOf(%v) = %d, want %d", percent, got, want)
		}
	}
}
