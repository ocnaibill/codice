package handlers

import "testing"

// The worker proposes the language it found in a file's text (#35) as a suggestion with the source
// "detected": accepting it is what writes it, and rejecting it leaves the edition as it was.
func TestCandidates_AcceptingTheDetectedLanguageWritesItToTheEditionAndSaysWhereItCameFrom(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("dune.pdf", "", "d.pdf", "pdf")
	cand := s.addCandidate(id, "language", "pt", "detected")

	if got := s.scalar(`SELECT COALESCE(language, '') FROM editions WHERE work_id = $1 AND is_primary`, id); got != "" {
		t.Fatalf("the edition had a language before: %q", got)
	}
	if code := s.decide(id, cand, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if got := s.scalar(`SELECT language FROM editions WHERE work_id = $1 AND is_primary`, id); got != "pt" {
		t.Errorf("language = %q", got)
	}
	m := s.meta(id)
	if m.Sources["language"] != "detected" || !m.Locks["language"] {
		t.Errorf("source %q, locked %v: a language a person accepted is confirmed and locked", m.Sources["language"], m.Locks["language"])
	}
}

func TestCandidates_RejectingTheDetectedLanguageChangesNothing(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("dune.pdf", "", "d.pdf", "pdf")
	cand := s.addCandidate(id, "language", "es", "detected")
	if code := s.decide(id, cand, "reject"); code != 200 {
		t.Fatalf("reject: %d", code)
	}
	if got := s.scalar(`SELECT COALESCE(language, '') FROM editions WHERE work_id = $1 AND is_primary`, id); got != "" {
		t.Errorf("language = %q", got)
	}
	if got := s.scalar(`SELECT state FROM metadata_candidates WHERE id = $1`, cand); got != "rejected" {
		t.Errorf("state = %q", got)
	}
}
