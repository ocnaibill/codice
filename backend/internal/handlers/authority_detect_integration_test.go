package handlers

import (
	"fmt"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/people"
)

func (s *catalogStack) hold(person, scheme, value string) {
	s.t.Helper()
	s.exec(`INSERT INTO person_authority (person_id, scheme, value, source) SELECT id, $2, $3, 'Open Library' FROM person WHERE name = $1 ON CONFLICT DO NOTHING`, person, scheme, value)
}

func (s *catalogStack) detect() int {
	s.t.Helper()
	n, err := people.Detect(s.t.Context(), s.db)
	if err != nil {
		s.t.Fatal(err)
	}
	return n
}

func pairSummary(p authorityPair) string {
	return fmt.Sprintf("%s|%s|%s:%s", p.Reason, p.A.Name+"+"+p.B.Name, p.Evidence["scheme"], p.Evidence["value"])
}

func TestDetect_TwoPeopleWithDifferentKeysThatShareAnIdentifierAreProposedWithTheIdentifierAsEvidence(t *testing.T) {
	s := newCatalogStack(t)
	// Two Open Library authors for one person: no word in common, and two keys, but the same Wikidata.
	s.addWork("Dune", "Frank Herbert", "a.epub", "epub")
	s.addWork("Messias", "F. P. Herbert", "b.epub", "epub")
	s.addWork("Other", "Brian Herbert", "c.epub", "epub")
	s.hold("Frank Herbert", "openlibrary", "OL1A")
	s.hold("F. P. Herbert", "openlibrary", "OL2A")
	s.hold("Brian Herbert", "openlibrary", "OL3A")
	if n := s.detect(); n != 0 {
		t.Fatalf("nothing in common yet: %d pairs", n)
	}
	s.hold("Frank Herbert", "wikidata", "Q7934")
	s.hold("F. P. Herbert", "wikidata", "Q7934")
	s.hold("Brian Herbert", "wikidata", "Q711137")
	if n := s.detect(); n != 1 {
		t.Fatalf("new pairs = %d, want 1", n)
	}
	pairs := s.authorityPairs() // right after the first scan: a later scan would promote a pair proposed for another reason
	if n := s.detect(); n != 0 {
		t.Errorf("a second scan found %d new pairs", n)
	}
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v", pairs)
	}
	got := pairSummary(pairs[0])
	if got != "authority|Frank Herbert+F. P. Herbert|wikidata:Q7934" && got != "authority|F. P. Herbert+Frank Herbert|wikidata:Q7934" {
		t.Errorf("pair = %s", got)
	}
	for _, side := range []struct{ Authorities []string }{{pairs[0].A.Authorities}, {pairs[0].B.Authorities}} {
		if len(side.Authorities) != 2 {
			t.Errorf("each side shows what it holds: %v", side.Authorities)
		}
	}
	// A merge is only proposed.
	if got := s.scalar(`SELECT count(*) FROM person`); got != "3" {
		t.Errorf("people = %s: nothing is merged on its own", got)
	}
}

func TestDetect_TheIdentifierThatSaysTheMostIsTheEvidenceAndASinglePersonIsNeverPairedWithItself(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("A", "Ann Lee", "a.epub", "epub")
	s.addWork("B", "Annie Lee", "b.epub", "epub")
	for _, p := range []string{"Ann Lee", "Annie Lee"} {
		s.hold(p, "isni", "0000000121347853")
		s.hold(p, "viaf", "59083797")
		s.hold(p, "openlibrary", "OL9A")
		s.hold(p, "wikidata", "Q1")
	}
	// A person with two keys that share an identifier with itself is not a pair.
	s.addWork("C", "Solo Person", "c.epub", "epub")
	s.hold("Solo Person", "openlibrary", "OL5A")
	s.hold("Solo Person", "openlibrary", "OL6A")
	s.hold("Solo Person", "wikidata", "Q99")
	if n := s.detect(); n != 1 {
		t.Fatalf("pairs = %d, want 1: one pair however many identifiers the two share", n)
	}
	if got := s.scalar(`SELECT evidence->>'scheme' FROM person_merge_candidates`); got != "wikidata" {
		t.Errorf("evidence = %s: Wikidata says the most", got)
	}
	s.exec(`DELETE FROM person_merge_candidates`)
	s.exec(`DELETE FROM person_authority WHERE scheme = 'wikidata'`)
	s.detect()
	if got := s.scalar(`SELECT evidence->>'scheme' FROM person_merge_candidates`); got != "viaf" {
		t.Errorf("then VIAF, before ISNI and the Open Library key: %s", got)
	}
	s.exec(`DELETE FROM person_merge_candidates`)
	s.exec(`DELETE FROM person_authority WHERE scheme = 'viaf'`)
	s.detect()
	if got := s.scalar(`SELECT evidence->>'scheme' FROM person_merge_candidates`); got != "isni" {
		t.Errorf("then ISNI: %s", got)
	}
	s.exec(`DELETE FROM person_merge_candidates`)
	s.exec(`DELETE FROM person_authority WHERE scheme = 'isni'`)
	s.detect()
	if got := s.scalar(`SELECT evidence->>'scheme' FROM person_merge_candidates`); got != "openlibrary" {
		t.Errorf("and the key itself: %s", got)
	}
}

func TestDetect_APairProposedOnlyForItsWordsIsPromotedAndOneThatWasDismissedStaysDismissed(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("A", "Frank Herbert", "a.epub", "epub")
	s.addWork("B", "Herbert, Frank", "b.epub", "epub")
	s.addWork("C", "Ann Lee", "c.epub", "epub")
	s.addWork("D", "Lee Ann", "d.epub", "epub")
	if n := s.detect(); n != 2 {
		t.Fatalf("words pairs = %d", n)
	}
	s.exec(`UPDATE person_merge_candidates SET state = 'dismissed' WHERE person_a IN (SELECT id FROM person WHERE name IN ('Ann Lee', 'Lee Ann'))`)
	for _, p := range []string{"Frank Herbert", "Herbert, Frank", "Ann Lee", "Lee Ann"} {
		s.hold(p, "wikidata", "Q"+map[string]string{"Frank Herbert": "7934", "Herbert, Frank": "7934", "Ann Lee": "5", "Lee Ann": "5"}[p])
	}
	if n := s.detect(); n != 0 {
		t.Errorf("both pairs already existed: %d new", n)
	}
	if got := s.scalar(`SELECT count(*) FROM person_merge_candidates WHERE reason = 'authority' AND state = 'pending'`); got != "1" {
		t.Errorf("promoted pending pairs = %s, want 1", got)
	}
	if got := s.scalar(`SELECT reason || '/' || state || '/' || COALESCE(evidence::text, '') FROM person_merge_candidates WHERE state = 'dismissed'`); got != "words/dismissed/" {
		t.Errorf("the dismissed pair was touched: %s", got)
	}
}

func TestDetect_TheSameValueUnderAnotherKindOfIdentifierIsNotTheSameIdentifier(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("A", "Ann Lee", "a.epub", "epub")
	s.addWork("B", "Bo Kim", "b.epub", "epub")
	s.hold("Ann Lee", "viaf", "123456")
	s.hold("Bo Kim", "isni", "123456")
	if n := s.detect(); n != 0 {
		t.Errorf("a VIAF number and an ISNI that read the same are not one identifier: %d pairs", n)
	}
}
