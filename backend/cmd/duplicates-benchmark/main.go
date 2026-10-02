// Command duplicates-benchmark evaluates the fingerprint that finds the same text in two files (#38) against a JSON
// corpus. The corpus is prepared by benchmarks/duplicates, so this command has no network dependency: it only runs
// the production functions (internal/fingerprint) on the words it is given.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ocnaibill/codice/backend/internal/fingerprint"
)

type corpus struct {
	Files map[string][]string `json:"files"`
	Pairs []pair              `json:"pairs"`
}

type pair struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Kind   string `json:"kind"`
	Expect string `json:"expect"` // same, contains or none
}

type result struct {
	A, B, Kind, Expect, Verdict string
	OfA                         float64 `json:"ofA"`
	OfB                         float64 `json:"ofB"`
	Shared                      int     `json:"shared"`
	HashesA                     int     `json:"hashesA"`
	HashesB                     int     `json:"hashesB"`
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
		if err := enc.Encode(result{A: p.A, B: p.B, Kind: p.Kind, Expect: p.Expect, Verdict: verdict,
			OfA: o.OfA, OfB: o.OfB, Shared: o.Shared, HashesA: len(a), HashesB: len(b)}); err != nil {
			fmt.Fprintln(os.Stderr, "duplicates benchmark: write:", err)
			os.Exit(1)
		}
	}
}
