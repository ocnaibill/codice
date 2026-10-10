package handlers

import "testing"

func TestEstimateRemaining_IsThePaceOfThePersonInTheFileAndNothingBeforeItIsWorthSaying(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
		percent float64
		want    int
	}{
		{"a tenth of the file in an hour: nine hours left", 3600, 10, 32400},
		{"half of it in two hours: two hours left", 7200, 50, 7200},
		{"ninety percent in nine hours: one left", 32400, 90, 3600},
		{"the least reading that counts", 600, 50, 600},
		{"a minute short of it", 599, 50, 0},
		{"the least of the file that counts", 3600, 5, 68400},
		{"a hair short of it", 3600, 4.9, 0},
		{"finished", 3600, 100, 0},
		{"nothing read", 0, 0, 0},
		{"a thousand hours is not a book", 4_000_000, 5.0, 0},
	} {
		if got := estimateRemaining(tc.seconds, tc.percent); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}
