package main

import "testing"

func TestNeedsDatabase(t *testing.T) {
	cases := map[string]bool{
		"":                                false,
		"help":                            false,
		"recover-owner":                   true,
		"transfer-owner --to ana":         true,
		"backup --dir /b":                 true,
		"verify-backup pacote.tar":        false,
		"verify-backup --deep pacote.tar": true,
		"prune-backups --dir /b":          false,
		// The restore replaces the database, which must have no other connection: this
		// program must not hold one open while it runs.
		"restore --in pacote.tar": false,
	}
	for line, want := range cases {
		var args []string
		for _, f := range splitFields(line) {
			args = append(args, f)
		}
		if got := needsDatabase(args); got != want {
			t.Errorf("needsDatabase(%q) = %v, want %v", line, got, want)
		}
	}
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
