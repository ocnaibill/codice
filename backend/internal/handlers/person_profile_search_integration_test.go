package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) searchFor(a actor, person int, body string) (int, ProfileSearch) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/admin/people/%d/profile/search", person), body)
	var out ProfileSearch
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (s *catalogStack) searchResult(a actor, person int) (int, ProfileSearch) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/admin/people/%d/profile/search", person), "")
	var out ProfileSearch
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (s *catalogStack) answerSearch(person int, state, results string) {
	s.t.Helper()
	s.exec(`UPDATE person_profile_searches SET state = $2, results = $3::jsonb, finished_at = now() WHERE person_id = $1`, person, state, results)
}

func (s *catalogStack) link(a actor, person int, qid string) (int, string) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/admin/people/%d/profile/link", person), fmt.Sprintf(`{"wikidataId":%q}`, qid))
	var out struct {
		Profile string `json:"profile"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out.Profile
}

const neal = `[{"id":"Q6984190","label":"Neal Shusterman","description":"Escritor norte-americano","born":"1962-11-12","photo":true,"wikipedia":true},
                {"id":"Q111","label":"Neal S.","description":"outro","photo":false,"wikipedia":false}]`

func TestProfileSearch_IsAskedOfTheWorkerByTheNameOfThePersonOrByWhatStaffTyped(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Scythe", "Neal Shusterman", "a.epub", "epub")
	person := s.personID("Neal Shusterman")
	if code, _ := s.searchResult(admin, person); code != http.StatusNotFound {
		t.Errorf("none asked yet: %d", code)
	}
	code, got := s.searchFor(admin, person, ``)
	if code != http.StatusAccepted || got.State != "pending" || got.Query != "Neal Shusterman" || got.Results == nil {
		t.Fatalf("ask: %d %+v", code, got)
	}
	if s.scalar(`SELECT count(*) FROM jobs WHERE type = 'profile_search' AND payload->>'person' = $1 AND payload->>'query' = 'Neal Shusterman'`, fmt.Sprint(person)) != "1" {
		t.Errorf("the job: %s", s.scalar(`SELECT count(*) FROM jobs WHERE type = 'profile_search'`))
	}
	// Asked again while it waits: the same request.
	if code, _ := s.searchFor(admin, person, `{"query":"Shusterman"}`); code != http.StatusAccepted {
		t.Errorf("again: %d", code)
	}
	if s.scalar(`SELECT count(*) FROM jobs WHERE type = 'profile_search'`) != "1" {
		t.Errorf("two jobs for the same person")
	}
	if s.scalar(`SELECT query FROM person_profile_searches WHERE person_id = $1`, person) != "Shusterman" {
		t.Errorf("what was typed is what is kept")
	}
	if s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.profile.search'`) != "1" {
		t.Errorf("the audit: only the request that made a job")
	}
	// Answered, it is read back.
	s.answerSearch(person, "done", neal)
	code, res := s.searchResult(admin, person)
	if code != 200 || res.State != "done" || len(res.Results) != 2 || res.Results[0].ID != "Q6984190" || res.Results[0].Born != "1962-11-12" || !res.Results[0].Photo {
		t.Errorf("result: %d %+v", code, res)
	}
	// A new search forgets the old answer.
	s.exec(`UPDATE jobs SET state = 'succeeded' WHERE type = 'profile_search'`)
	if code, _ := s.searchFor(admin, person, ``); code != http.StatusAccepted {
		t.Fatal(code)
	}
	if _, res := s.searchResult(admin, person); res.State != "pending" || len(res.Results) != 0 {
		t.Errorf("an old answer stayed: %+v", res)
	}
	if s.scalar(`SELECT count(*) FROM jobs WHERE type = 'profile_search'`) != "2" {
		t.Errorf("a finished search is not a waiting one")
	}
}

func TestProfileSearch_RefusesWhatIsNotAPersonOrATextThatFits(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Scythe", "Neal Shusterman", "a.epub", "epub")
	person := s.personID("Neal Shusterman")
	if code, _ := s.searchFor(admin, 999999, ``); code != http.StatusNotFound {
		t.Errorf("no such person: %d", code)
	}
	if code, _ := s.searchFor(admin, person, `{"query":"`+strings.Repeat("a", 201)+`"}`); code != http.StatusBadRequest {
		t.Errorf("too long: %d", code)
	}
	if code, _ := s.searchFor(admin, person, `not json`); code != http.StatusBadRequest {
		t.Errorf("not json: %d", code)
	}
	if s.scalar(`SELECT count(*) FROM jobs WHERE type = 'profile_search'`) != "0" || s.scalar(`SELECT count(*) FROM person_profile_searches`) != "0" {
		t.Errorf("a refused request left something")
	}
	if rec := s.do(admin, "POST", "/admin/people/abc/profile/search", ``); rec.Code != http.StatusNotFound {
		t.Errorf("not a number: %d", rec.Code)
	}
}

func TestProfileLink_TiesTheIdentifierOfACandidateToThePersonAndTheWorkerReadsTheProfile(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Scythe", "Neal Shusterman", "a.epub", "epub")
	person := s.personID("Neal Shusterman")
	// No search, a search that waits, and one that was not answered: nothing to choose from.
	if code, _ := s.link(admin, person, "Q6984190"); code != http.StatusBadRequest {
		t.Errorf("no search: %d", code)
	}
	s.searchFor(admin, person, ``)
	if code, _ := s.link(admin, person, "Q6984190"); code != http.StatusBadRequest {
		t.Errorf("pending: %d", code)
	}
	s.answerSearch(person, "failed", `[]`)
	if code, _ := s.link(admin, person, "Q6984190"); code != http.StatusBadRequest {
		t.Errorf("failed: %d", code)
	}
	// Candidates are only to be chosen from an answer: what is kept of a search that did not finish is not.
	for _, state := range []string{"pending", "off", "failed"} {
		s.answerSearch(person, state, neal)
		if code, _ := s.link(admin, person, "Q6984190"); code != http.StatusBadRequest {
			t.Errorf("%s with candidates: %d, want 400", state, code)
		}
	}
	s.answerSearch(person, "done", neal)
	for _, bad := range []string{"Q999", "q6984190", "Q6984190; DROP", "", "6984190"} {
		if code, _ := s.link(admin, person, bad); code != http.StatusBadRequest {
			t.Errorf("%q: %d, want 400", bad, code)
		}
	}
	if s.scalar(`SELECT count(*) FROM person_authority WHERE scheme = 'wikidata'`) != "0" {
		t.Errorf("a refused choice kept an identifier")
	}
	// A lookup remembered as missing is asked again once a human says it is that one.
	s.exec(`INSERT INTO authority_lookups (source, key, state) VALUES ('wikidata', 'Q6984190', 'missing')`)
	code, outcome := s.link(admin, person, "Q6984190")
	if code != http.StatusAccepted || outcome != "queued" {
		t.Fatalf("link: %d %q", code, outcome)
	}
	if s.scalar(`SELECT value || '|' || source FROM person_authority WHERE person_id = $1 AND scheme = 'wikidata'`, person) != "Q6984190|Escolhido na página da pessoa" {
		t.Errorf("the identifier")
	}
	if s.scalar(`SELECT count(*) FROM authority_lookups WHERE key = 'Q6984190'`) != "0" {
		t.Errorf("the lookup is remembered, and the worker would not ask")
	}
	if s.scalar(`SELECT details->>'outcome' FROM audit_log WHERE action = 'person.profile.link'`) != "queued" {
		t.Errorf("audit")
	}
}

func TestProfileLink_ReplacesAProfileReadForAnotherIdentifierAndKeepsOneWrittenByHand(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank) // Q7934, from the worker
	s.exec(`INSERT INTO person_authority (person_id, scheme, value, source) VALUES ($1, 'wikidata', 'Q7934', 'Open Library')`, frank)
	s.searchFor(admin, frank, ``)
	s.answerSearch(frank, "done", `[{"id":"Q7934","label":"Frank Herbert","description":"x","photo":false,"wikipedia":false},{"id":"Q555","label":"Frank H.","description":"y","photo":false,"wikipedia":false}]`)

	// The same one: nothing to do.
	if code, outcome := s.link(admin, frank, "Q7934"); code != http.StatusAccepted || outcome != "present" {
		t.Errorf("same: %d %q", code, outcome)
	}
	if s.scalar(`SELECT count(*) FROM person_profile WHERE person_id = $1`, frank) != "1" {
		t.Errorf("the profile went for the same identifier")
	}
	// Another one: the profile that was read is replaced, and so is the identifier.
	if code, outcome := s.link(admin, frank, "Q555"); code != http.StatusAccepted || outcome != "queued" {
		t.Fatalf("other: %d %q", code, outcome)
	}
	if s.scalar(`SELECT count(*) FROM person_profile WHERE person_id = $1`, frank) != "0" {
		t.Errorf("the profile of the wrong person stayed")
	}
	if s.scalar(`SELECT string_agg(value, ',') FROM person_authority WHERE person_id = $1 AND scheme = 'wikidata'`, frank) != "Q555" {
		t.Errorf("identifiers: %s", s.scalar(`SELECT string_agg(value, ',') FROM person_authority WHERE person_id = $1`, frank))
	}
	// A profile written by hand is the staff's: choosing an author does not throw it away.
	s.exec(`INSERT INTO person_profile (person_id, wikidata_id, description, place_read, manual) VALUES ($1, NULL, 'meu texto', TRUE, TRUE)`, frank)
	if code, outcome := s.link(admin, frank, "Q7934"); code != http.StatusAccepted || outcome != "kept" {
		t.Fatalf("manual: %d %q", code, outcome)
	}
	if s.scalar(`SELECT description FROM person_profile WHERE person_id = $1`, frank) != "meu texto" {
		t.Errorf("the profile that was written was replaced")
	}
	if s.scalar(`SELECT string_agg(value, ',') FROM person_authority WHERE person_id = $1 AND scheme = 'wikidata'`, frank) != "Q7934" {
		t.Errorf("the identifier is still recorded, for when the profile is discarded")
	}
}

func TestProfileLink_AnotherPersonThatHoldsTheSameIdentifierIsProposedForMerging(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Scythe", "Neal Shusterman", "a.epub", "epub")
	s.addWork("Thunderhead", "N. Shusterman", "b.epub", "epub")
	a, b := s.personID("Neal Shusterman"), s.personID("N. Shusterman")
	for _, p := range []int{a, b} {
		s.searchFor(admin, p, ``)
		s.answerSearch(p, "done", neal)
		if code, _ := s.link(admin, p, "Q6984190"); code != http.StatusAccepted {
			t.Fatalf("link %d: %d", p, code)
		}
	}
	if s.scalar(`SELECT count(*) FROM person_merge_candidates WHERE reason = 'authority'`) != "1" {
		t.Errorf("two people with one identifier are one person to be decided: %s", s.scalar(`SELECT count(*) FROM person_merge_candidates`))
	}
}
