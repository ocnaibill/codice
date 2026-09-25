// Command equivalence-benchmark evaluates the production equivalence engine against a JSON corpus.
// Corpus preparation lives in benchmarks/equivalence so this command has no network or model
// dependencies and can be used by other evaluation sets later.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
)

type corpus struct {
	Files map[string]equivalence.File `json:"files"`
	Cases []testCase                  `json:"cases"`
}

type testCase struct {
	Suite       string `json:"suite"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	SourceSeq   int    `json:"sourceSequence"`
	ExpectedSeq *int   `json:"expectedSequence"`
}

type result struct {
	Suite         string `json:"suite"`
	Status        string `json:"status"`
	Method        string `json:"method,omitempty"`
	Confidence    string `json:"confidence,omitempty"`
	Distance      *int   `json:"distance,omitempty"`
	LatencyMicros int64  `json:"latencyMicros"`
}

func main() {
	var input corpus
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fatal("read corpus", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	for _, tc := range input.Cases {
		src, ok := input.Files[tc.Source]
		if !ok {
			fatal("unknown source", fmt.Errorf("%q", tc.Source))
		}
		dst, ok := input.Files[tc.Destination]
		if !ok {
			fatal("unknown destination", fmt.Errorf("%q", tc.Destination))
		}
		var source *equivalence.Segment
		for i := range src.Segments {
			if src.Segments[i].Sequence == tc.SourceSeq {
				source = &src.Segments[i]
				break
			}
		}
		if source == nil {
			fatal("unknown source sequence", fmt.Errorf("%s:%d", tc.Source, tc.SourceSeq))
		}

		started := time.Now()
		answer := equivalence.Find(src, dst, *source)
		row := result{Suite: tc.Suite, Status: answer.Status, LatencyMicros: time.Since(started).Microseconds()}
		if len(answer.Candidates) > 0 {
			candidate := answer.Candidates[0]
			row.Method, row.Confidence = candidate.Method, candidate.Confidence
			if tc.ExpectedSeq != nil {
				var locator struct {
					Offset int `json:"offset"`
				}
				if err := json.Unmarshal(candidate.Locator, &locator); err != nil {
					fatal("read candidate locator", err)
				}
				distance := locator.Offset - *tc.ExpectedSeq
				if distance < 0 {
					distance = -distance
				}
				row.Distance = &distance
			}
		}
		if err := encoder.Encode(row); err != nil {
			fatal("write result", err)
		}
	}
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "equivalence benchmark: %s: %v\n", action, err)
	os.Exit(1)
}
