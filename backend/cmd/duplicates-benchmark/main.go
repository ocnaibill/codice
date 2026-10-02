// Command duplicates-benchmark evaluates the fingerprint that finds the same text in two files (#38) against a JSON
// corpus. The corpus is prepared by benchmarks/duplicates, so this command has no network dependency: it only runs
// the production functions (internal/fingerprint) on the words it is given.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ocnaibill/codice/backend/internal/equivalence"
	"github.com/ocnaibill/codice/backend/internal/fingerprint"
)

type corpus struct {
	Files map[string][]string `json:"files"`
	Pairs []pair              `json:"pairs"`
	// Books are files with their segments, for the pairs that are read against each other (translations).
	Books    map[string]equivalence.File `json:"books"`
	Parallel []pair                      `json:"parallel"`
}

type pair struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Kind   string `json:"kind"`
	Expect string `json:"expect"` // same, contains or none
}

type result struct {
	Mode                        string `json:"mode"` // text (the fingerprint) or parallel (read against each other)
	A, B, Kind, Expect, Verdict string
	OfA                         float64 `json:"ofA"`
	OfB                         float64 `json:"ofB"`
	Shared                      int     `json:"shared"`
	HashesA                     int     `json:"hashesA"`
	HashesB                     int     `json:"hashesB"`
	Samples                     int     `json:"samples"`
	Hits                        int     `json:"hits"`
	Share                       float64 `json:"share"`
	Order                       float64 `json:"order"`
	LatencyMillis               int64   `json:"latencyMillis"`
}

func main() {
	var input corpus
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintln(os.Stderr, "duplicates benchmark: read corpus:", err)
		os.Exit(1)
	}
	prints := map[string][]int64{}
	for name, words := range input.Files {
		prints[name] = fingerprint.Of(words)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, p := range input.Pairs {
		a, b := prints[p.A], prints[p.B]
		o := fingerprint.Compare(a, b)
		verdict := "none"
		switch {
		case a == nil || b == nil:
			verdict = "too short"
		case o.Same():
			verdict = "same"
		case o.Contains():
			verdict = "contains"
		}
		if err := enc.Encode(result{Mode: "text", A: p.A, B: p.B, Kind: p.Kind, Expect: p.Expect, Verdict: verdict,
			OfA: o.OfA, OfB: o.OfB, Shared: o.Shared, HashesA: len(a), HashesB: len(b)}); err != nil {
			fmt.Fprintln(os.Stderr, "duplicates benchmark: write:", err)
			os.Exit(1)
		}
	}
	for _, p := range input.Parallel {
		started := time.Now()
		read := equivalence.ReadParallel(input.Books[p.A], input.Books[p.B], equivalence.ParallelSamples)
		verdict := "none"
		if read.Translation() {
			verdict = "translation"
		}
		if err := enc.Encode(result{Mode: "parallel", A: p.A, B: p.B, Kind: p.Kind, Expect: p.Expect, Verdict: verdict,
			Samples: read.Samples, Hits: read.Hits, Share: read.Share, Order: read.Order, LatencyMillis: time.Since(started).Milliseconds()}); err != nil {
			fmt.Fprintln(os.Stderr, "duplicates benchmark: write:", err)
			os.Exit(1)
		}
	}
}
