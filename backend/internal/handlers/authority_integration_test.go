package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const herbertKey = `{"query":"Dune","credits":[{"name":"Frank Herbert","ids":{"openlibrary":"OL79034A"}},{"name":"John Schoenherr","role":"illustrator","ids":{"openlibrary":"OL999A"}}]}`

func (s *catalogStack) addCandidateWithEvidence(workID int, field, value, source, evidence string) int {
	s.t.Helper()
	var id int
	if err := s.db.QueryRow(`INSERT INTO metadata_candidates (work_id, field, value, source, evidence) VALUES ($1, $2, $3, $4, $5::jsonb) RETURNING id`,
		workID, field, value, source, evidence).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *catalogStack) authorities(name string) string {
	s.t.Helper()
	return s.scalar(`SELECT COALESCE(string_agg(a.scheme || ':' || a.value, ',' ORDER BY a.scheme, a.value), '')
		FROM person p JOIN person_authority a ON a.person_id = p.id WHERE p.name = $1`, name)
}

type authorityPair struct {
	ID       int64
	Reason   string
	Evidence map[string]string
	A, B     struct {
		Name        string
		Authorities []string
	}
}

func (s *catalogStack) authorityPairs() []authorityPair {
	s.t.Helper()
	var out struct{ Data []authorityPair }
	json.Unmarshal(s.do(admin, "GET", "/admin/people/merges", "").Body.Bytes(), &out)
	return out.Data
}

func TestAuthority_AcceptingAnAuthorKeepsTheKeyTheSourceGaveThatNameAndNothingElse(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Dune", "F. Herbert", "a.epub", "epub")
	c := s.addCandidateWithEvidence(w, "author", "Frank Herbert", "Open Library", herbertKey)
	if got := s.authorities("Frank Herbert"); got != "" {
		t.Fatalf("a suggestion nobody accepted already left a key: %q", got)
	}
	if code := s.decide(w, c, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if got := s.authorities("Frank Herbert"); got != "openlibrary:OL79034A" {
		t.Errorf("keys = %q: the key of the credit with that name, and not the illustrator's", got)
	}
	if got := s.scalar(`SELECT source FROM person_authority`); got != "Open Library" {
		t.Errorf("source = %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM person_authority`); got != "1" {
		t.Errorf("rows = %s", got)
	}
	// Accepting it again through another work is the same key for the same person: nothing new, no pair with itself.
	w2 := s.addWork("Messias", "F. Herbert", "b.epub", "epub")
	c2 := s.addCandidateWithEvidence(w2, "author", "Frank Herbert", "Open Library", herbertKey)
	if code := s.decide(w2, c2, "accept"); code != 200 {
		t.Fatalf("accept 2: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM person_authority`); got != "1" {
		t.Errorf("rows = %s, want still 1", got)
	}
	if n := len(s.authorityPairs()); n != 0 {
		t.Errorf("a person was proposed to be merged with itself: %d pairs", n)
	}
}

func TestAuthority_NothingIsKeptWhenTheDecisionIsNotAnAcceptedAuthorOrTheRecordDoesNotCreditTheName(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Dune", "F. Herbert", "a.epub", "epub")
	rejected := s.addCandidateWithEvidence(w, "author", "Frank Herbert", "Open Library", herbertKey)
	if code := s.decide(w, rejected, "reject"); code != 200 {
		t.Fatal(code)
	}
	// Only an author carries a person: a title that happens to read like a credited name carries no key.
	title := s.addCandidateWithEvidence(w, "title", "Frank Herbert", "Open Library", herbertKey)
	if code := s.decide(w, title, "accept"); code != 200 {
		t.Fatal(code)
	}
	// The record credits other people: a name it does not credit does not borrow a neighbour's key.
	other := s.addCandidateWithEvidence(w, "author", "Brian Herbert", "Open Library", herbertKey)
	if code := s.decide(w, other, "accept"); code != 200 {
		t.Fatal(code)
	}
	plain := s.addWork("Solo", "Plato", "b.epub", "epub")
	noEvidence := s.addCandidate(plain, "author", "Platão", "Google Books")
	if code := s.decide(plain, noEvidence, "accept"); code != 200 {
		t.Fatal(code)
	}
	if got := s.scalar(`SELECT count(*) FROM person_authority`); got != "0" {
		t.Errorf("keys kept = %s, want 0", got)
	}
}

func TestAuthority_TwoPeopleHoldingTheSameKeyAreProposedForMergingWithTheKeyAsEvidence(t *testing.T) {
	s := newCatalogStack(t)
	// "Frank P. Herbert" shares no set of words with "Frank Herbert": only the key says they are one.
	w1 := s.addWork("Dune", "F. Herbert", "a.epub", "epub")
	w2 := s.addWork("Messias", "Herbert", "b.epub", "epub")
	c1 := s.addCandidateWithEvidence(w1, "author", "Frank Herbert", "Open Library", herbertKey)
	c2 := s.addCandidateWithEvidence(w2, "author", "Frank P. Herbert", "Open Library",
		`{"credits":[{"name":"Frank P. Herbert","ids":{"openlibrary":"OL79034A"}}]}`)
	s.decide(w1, c1, "accept")
	if n := len(s.authorityPairs()); n != 0 {
		t.Fatalf("one key is no pair: %d", n)
	}
	s.decide(w2, c2, "accept")
	pairs := s.authorityPairs()
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v", pairs)
	}
	p := pairs[0]
	if p.Reason != "authority" || p.Evidence["scheme"] != "openlibrary" || p.Evidence["value"] != "OL79034A" {
		t.Errorf("reason/evidence = %q %v", p.Reason, p.Evidence)
	}
	for _, side := range []struct {
		Name        string
		Authorities []string
	}{p.A, p.B} {
		if len(side.Authorities) != 1 || side.Authorities[0] != "openlibrary:OL79034A" {
			t.Errorf("%s authorities = %v: each side shows the key it holds", side.Name, side.Authorities)
		}
	}
	// Only a human decides: nothing was merged.
	if got := s.scalar(`SELECT count(*) FROM person WHERE name IN ('Frank Herbert', 'Frank P. Herbert')`); got != "2" {
		t.Errorf("people = %s", got)
	}
}

func TestAuthority_TheSameKeyOutranksTheSameWordsAndIsListedFirst(t *testing.T) {
	s := newCatalogStack(t)
	// A pair proposed for sharing words, the oldest ...
	s.addWork("X", "Ann Lee", "c.epub", "epub")
	s.addWork("Y", "Lee Ann", "d.epub", "epub")
	s.exec(`INSERT INTO person_merge_candidates (person_a, person_b) SELECT LEAST(a.id, b.id), GREATEST(a.id, b.id)
		FROM person a, person b WHERE a.name = 'Ann Lee' AND b.name = 'Lee Ann'`)
	// ... and another, an older suggestion for words that the key now confirms.
	s.addWork("Dune", "Frank Herbert", "a.epub", "epub")
	w2 := s.addWork("Messias", "Herbert, Frank", "b.epub", "epub")
	s.exec(`INSERT INTO person_merge_candidates (person_a, person_b) SELECT LEAST(a.id, b.id), GREATEST(a.id, b.id)
		FROM person a, person b WHERE a.name = 'Frank Herbert' AND b.name = 'Herbert, Frank'`)
	c := s.addCandidateWithEvidence(w2, "author", "Herbert, Frank", "Open Library",
		`{"credits":[{"name":"Herbert, Frank","ids":{"openlibrary":"OL79034A"}}]}`)
	s.exec(`INSERT INTO person_authority (person_id, scheme, value) SELECT id, 'openlibrary', 'OL79034A' FROM person WHERE name = 'Frank Herbert'`)
	s.decide(w2, c, "accept")

	pairs := s.authorityPairs()
	if len(pairs) != 2 {
		t.Fatalf("pairs = %+v: the same pair is promoted, not repeated", pairs)
	}
	if pairs[0].Reason != "authority" || pairs[1].Reason != "words" {
		t.Errorf("reasons = %q, %q: the key is listed first, although it was proposed later", pairs[0].Reason, pairs[1].Reason)
	}
}

func TestAuthority_APairSomeoneSaidAreNotTheSameStaysDecidedWhateverKeyArrives(t *testing.T) {
	s := newCatalogStack(t)
	w1 := s.addWork("Dune", "F. Herbert", "a.epub", "epub")
	w2 := s.addWork("Messias", "Herbert", "b.epub", "epub")
	s.decide(w1, s.addCandidateWithEvidence(w1, "author", "Frank Herbert", "Open Library", herbertKey), "accept")
	s.exec(`INSERT INTO person (name) VALUES ('Frank P. Herbert')`)
	s.exec(`INSERT INTO person_merge_candidates (person_a, person_b, state)
		SELECT LEAST(a.id, b.id), GREATEST(a.id, b.id), 'dismissed' FROM person a, person b WHERE a.name = 'Frank Herbert' AND b.name = 'Frank P. Herbert'`)
	s.decide(w2, s.addCandidateWithEvidence(w2, "author", "Frank P. Herbert", "Open Library",
		`{"credits":[{"name":"Frank P. Herbert","ids":{"openlibrary":"OL79034A"}}]}`), "accept")
	if n := len(s.authorityPairs()); n != 0 {
		t.Errorf("a pair that was dismissed came back: %d", n)
	}
	if got := s.scalar(`SELECT state || '/' || reason || '/' || COALESCE(evidence::text, '')` + ` FROM person_merge_candidates`); got != "dismissed/words/" {
		t.Errorf("the decided pair was touched: %s", got)
	}
}

func TestAuthority_MergingTwoPeopleKeepsEveryKeyOfBoth(t *testing.T) {
	s := newCatalogStack(t)
	w1 := s.addWork("Dune", "F. Herbert", "a.epub", "epub")
	w2 := s.addWork("Messias", "Herbert", "b.epub", "epub")
	s.decide(w1, s.addCandidateWithEvidence(w1, "author", "Frank Herbert", "Open Library", herbertKey), "accept")
	s.decide(w2, s.addCandidateWithEvidence(w2, "author", "Frank P. Herbert", "Open Library",
		`{"credits":[{"name":"Frank P. Herbert","ids":{"openlibrary":"OL79034A","comicvine":"4050-1"}}]}`), "accept")
	pair := s.authorityPairs()[0]
	keep := s.personID("Frank Herbert")
	if code := s.do(admin, "POST", fmt.Sprintf("/admin/people/merges/%d/merge", pair.ID), fmt.Sprintf(`{"keep":%d,"confirm":true}`, keep)).Code; code != 204 {
		t.Fatalf("merge: %d", code)
	}
	if got := s.authorities("Frank Herbert"); got != "comicvine:4050-1,openlibrary:OL79034A" {
		t.Errorf("keys after the merge = %q: what the absorbed person knew is not lost", got)
	}
	if got := s.scalar(`SELECT count(*) FROM person_authority`); got != "2" {
		t.Errorf("rows = %s", got)
	}
}

func (s *catalogStack) contributors(workID int) string {
	s.t.Helper()
	return s.scalar(`SELECT COALESCE(string_agg(p.name || '/' || c.role || '/' || c.position, '; ' ORDER BY c.role, c.position), '')
		FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = $1`, workID)
}

const goodOmens = `{"credits":[{"name":"Terry Pratchett","ids":{"openlibrary":"OL25712A"}},{"name":"Neil Gaiman","ids":{"openlibrary":"OL53305A"}},{"name":"Paul Kidby","role":"illustrator","ids":{"openlibrary":"OL5A"}}]}`

func TestContributors_AcceptingAddsEachPersonInTheirRoleAfterWhoeverIsThereAndKeepsTheirKeys(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Good Omens", "Terry Pratchett", "a.epub", "epub")
	c := s.addCandidateWithEvidence(w, "contributors",
		`[{"name":"Neil Gaiman","role":"author"},{"name":"Paul Kidby","role":"illustrator"}]`, "Open Library", goodOmens)
	if got := s.candidates(w); len(got) != 1 || got[0].Current != "Terry Pratchett (author)" {
		t.Fatalf("the admin sees who the work has next to the suggestion: %+v", got)
	}
	if code := s.decide(w, c, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if got := s.contributors(w); got != "Terry Pratchett/author/0; Neil Gaiman/author/1; Paul Kidby/illustrator/0" {
		t.Errorf("contributors = %q", got)
	}
	if got := s.authorities("Neil Gaiman") + " " + s.authorities("Paul Kidby"); got != "openlibrary:OL53305A openlibrary:OL5A" {
		t.Errorf("keys = %q: each person keeps the key of their own credit", got)
	}
	if got := s.authorities("Terry Pratchett"); got != "" {
		t.Errorf("someone who was not added got a key: %q", got)
	}
	if got := s.scalar(`SELECT details->'added'->>0 || ' | ' || (details->'added'->>1) FROM audit_log WHERE action = 'metadata.accept'`); got != "Neil Gaiman (author) | Paul Kidby (illustrator)" {
		t.Errorf("audit = %q", got)
	}
}

func TestContributors_AcceptingOnlyAddsAndLeavesOutWhatIsNotAValidPerson(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Good Omens", "Terry Pratchett", "a.epub", "epub")
	s.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'illustrator', 3 FROM person WHERE name = 'Terry Pratchett'`, w)
	value := `[{"name":"Terry Pratchett","role":"author"},{"name":"Terry Pratchett","role":"illustrator"},
		{"name":"Colorist Person","role":"colorist"},{"name":"  ","role":"author"},{"name":"` + strings.Repeat("x", 201) + `","role":"author"},
		{"name":"Neil Gaiman","role":"author"}]`
	c := s.addCandidateWithEvidence(w, "contributors", value, "Open Library", goodOmens)
	if code := s.decide(w, c, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	if got := s.contributors(w); got != "Terry Pratchett/author/0; Neil Gaiman/author/1; Terry Pratchett/illustrator/3" {
		t.Errorf("contributors = %q: nobody is removed or moved, and what is not valid is left out", got)
	}
	if got := s.scalar(`SELECT count(*) FROM person`); got != "2" {
		t.Errorf("people = %s: an invalid entry created a person", got)
	}
	if got := s.scalar(`SELECT jsonb_array_length(details->'added')::text || ' ' || (details->'added'->>0) FROM audit_log WHERE action = 'metadata.accept'`); got != "1 Neil Gaiman (author)" {
		t.Errorf("audit = %q: only who was really added is reported", got)
	}
}

func TestContributors_ASuggestionIsCappedSoItCannotFloodAWork(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Anthology", "Editor One", "a.epub", "epub")
	var items []string
	for i := 0; i < 60; i++ {
		items = append(items, fmt.Sprintf(`{"name":"Writer %c%c","role":"author"}`, 'a'+i/26, 'a'+i%26))
	}
	c := s.addCandidateWithEvidence(w, "contributors", "["+strings.Join(items, ",")+"]", "Open Library", `{}`)
	if code := s.decide(w, c, "accept"); code != 200 {
		t.Fatal(code)
	}
	if got := s.scalar(`SELECT count(*) FROM work_contributors WHERE work_id = $1 AND role = 'author'`, w); got != "51" {
		t.Errorf("authors = %s, want the one it had and 50 more", got)
	}
}

func TestContributors_ARejectedOrMalformedSuggestionChangesNothingAndSuggestionsFromSeveralSourcesAllStay(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Good Omens", "Terry Pratchett", "a.epub", "epub")
	reject := s.addCandidateWithEvidence(w, "contributors", `[{"name":"Neil Gaiman","role":"author"}]`, "Open Library", goodOmens)
	if code := s.decide(w, reject, "reject"); code != 200 {
		t.Fatal(code)
	}
	if got := s.contributors(w); got != "Terry Pratchett/author/0" {
		t.Errorf("a rejected suggestion changed the work: %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM person_authority`); got != "0" {
		t.Errorf("a rejected suggestion left keys: %s", got)
	}
	bad := s.addCandidateWithEvidence(w, "contributors", `{"not":"a list"}`, "Open Library", goodOmens)
	if code := s.decide(w, bad, "accept"); code != 422 {
		t.Errorf("malformed: %d, want 422", code)
	}
	one := s.addCandidateWithEvidence(w, "contributors", `[{"name":"Paul Kidby","role":"illustrator"}]`, "Open Library", goodOmens)
	two := s.addCandidateWithEvidence(w, "contributors", `[{"name":"Neil Gaiman","role":"author"}]`, "Google Books", `{}`)
	if code := s.decide(w, one, "accept"); code != 200 {
		t.Fatal(code)
	}
	if got := s.scalar(`SELECT state FROM metadata_candidates WHERE id = $1`, two); got != "pending" {
		t.Errorf("another source's suggestion was dismissed (%s): adding people does not settle a field", got)
	}
}

func keysOf(c Candidate) string {
	parts := []string{}
	for _, k := range c.Keys {
		parts = append(parts, k.Name+"="+k.Scheme+":"+k.Value)
	}
	return strings.Join(parts, "|")
}

func TestCandidates_TheListSaysWhichKeysAcceptingWouldKeepAndAcceptingKeepsExactlyThose(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Good Omens", "T. Pratchett", "a.epub", "epub")
	author := s.addCandidateWithEvidence(w, "author", "Neil Gaiman", "Open Library", goodOmens)
	people := s.addCandidateWithEvidence(w, "contributors",
		`[{"name":"Paul  Kidby ","role":"illustrator"},{"name":"Terry Pratchett","role":"author"},{"name":"Neil Gaiman","role":"colorist"}]`, "Open Library", goodOmens)
	both := s.addCandidateWithEvidence(w, "author", "Anne Rice", "Google Books",
		`{"credits":[{"name":"Anne Rice","ids":{"openlibrary":"OL1A","comicvine":"4050-9","goodreads":"7"}}]}`)
	title := s.addCandidateWithEvidence(w, "title", "Neil Gaiman", "Open Library", goodOmens)
	bare := s.addCandidate(w, "isbn", "9788576573135", "Google Books")

	byID := map[int]Candidate{}
	for _, c := range s.candidates(w) {
		byID[int(c.ID)] = c
	}
	if got := keysOf(byID[author]); got != "Neil Gaiman=openlibrary:OL53305A" {
		t.Errorf("author keys = %q", got)
	}
	if got := keysOf(byID[people]); got != "Paul Kidby=openlibrary:OL5A|Terry Pratchett=openlibrary:OL25712A" {
		t.Errorf("contributors keys = %q: each listed person, in the listed order, and not an invalid entry", got)
	}
	if got := keysOf(byID[both]); got != "Anne Rice=comicvine:4050-9|Anne Rice=goodreads:7|Anne Rice=openlibrary:OL1A" {
		t.Errorf("several schemes = %q: in a stable order", got)
	}
	for _, id := range []int{title, bare} {
		if byID[id].Keys == nil || len(byID[id].Keys) != 0 {
			t.Errorf("a %s suggestion keeps no key and says so with an empty list, got %#v", byID[id].Field, byID[id].Keys)
		}
	}
	// What it said it would keep is what accepting keeps.
	s.decide(w, author, "accept")
	s.decide(w, people, "accept")
	if got := s.authorities("Neil Gaiman") + " " + s.authorities("Paul Kidby") + " " + s.authorities("Terry Pratchett"); got != "openlibrary:OL53305A openlibrary:OL5A openlibrary:OL25712A" {
		t.Errorf("kept = %q", got)
	}
}
