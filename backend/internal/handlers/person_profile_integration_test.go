package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) giveProfile(person int) {
	s.t.Helper()
	s.exec(`INSERT INTO person_profile (person_id, wikidata_id, description, born, died, bio, bio_language, bio_title, bio_url, bio_state,
	                                    image_path, image_credit, image_license, image_license_url, image_page_url)
	        VALUES ($1, 'Q7934', 'escritor de ficção científica americano (1920-1986)', '1920-10-08', '1986-02-11',
	                'Frank Herbert foi um escritor.', 'pt', 'Frank Herbert', 'https://pt.wikipedia.org/wiki/Frank_Herbert', 'done',
	                '/covers/person_Q7934.jpg', 'Unknown photographer', 'Public domain', '', 'https://commons.wikimedia.org/wiki/File:A.jpg')`, person)
}

func (s *catalogStack) setProfile(a actor, person int, body string) int {
	s.t.Helper()
	return s.do(a, "PUT", fmt.Sprintf("/admin/people/%d/profile", person), body).Code
}

func TestPersonProfile_IsOnThePageOfThePersonWithWhereItCameFrom(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	if p, code := s.personPage(ana, frank); code != 200 || p.Profile != nil {
		t.Fatalf("nothing was read: %d %+v", code, p.Profile)
	}
	s.giveProfile(frank)
	p, code := s.personPage(ana, frank)
	if code != 200 || p.Profile == nil {
		t.Fatalf("page: %d %+v", code, p.Profile)
	}
	got := p.Profile
	if got.WikidataID != "Q7934" || got.Description != "escritor de ficção científica americano (1920-1986)" || got.Born != "1920-10-08" || got.Died != "1986-02-11" {
		t.Errorf("profile: %+v", got)
	}
	if got.Bio != "Frank Herbert foi um escritor." || got.BioSource == nil || got.BioSource.Language != "pt" || got.BioSource.Title != "Frank Herbert" ||
		got.BioSource.URL != "https://pt.wikipedia.org/wiki/Frank_Herbert" || got.BioSource.License != "CC BY-SA 4.0" {
		t.Errorf("a biography says whose it is and where it is: %+v %+v", got.Bio, got.BioSource)
	}
	if got.Image == nil || got.Image.URL != "/covers/person_Q7934.jpg" || got.Image.Credit != "Unknown photographer" || got.Image.License != "Public domain" ||
		got.Image.PageURL != "https://commons.wikimedia.org/wiki/File:A.jpg" {
		t.Errorf("the photo says who made it and under what license: %+v", got.Image)
	}
	if got.Hidden || got.ImageHidden {
		t.Errorf("nobody hid it: %+v", got)
	}
}

func TestPersonProfile_WhatIsMissingIsNotSaid(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.exec(`INSERT INTO person_profile (person_id, wikidata_id, description) VALUES ($1, 'Q7934', 'escritor')`, frank)
	p, _ := s.personPage(ana, frank)
	if p.Profile == nil || p.Profile.Description != "escritor" || p.Profile.Born != "" || p.Profile.Died != "" || p.Profile.Bio != "" || p.Profile.BioSource != nil || p.Profile.Image != nil {
		t.Errorf("only the description: %+v", p.Profile)
	}
	// A biography with no page it came from is not a biography Wikipedia wrote.
	s.exec(`UPDATE person_profile SET bio = 'texto sem fonte', bio_language = NULL WHERE person_id = $1`, frank)
	if p, _ := s.personPage(ana, frank); p.Profile.Bio != "" || p.Profile.BioSource != nil {
		t.Errorf("a text with no source was shown: %+v", p.Profile)
	}
}

func TestPersonProfile_HidingItIsForStaffAndNobodyElseSeesWhatIsHidden(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)

	// (Who may call it is the router's: cmd/api/router_test.go has the route among the staff's.)
	// Hiding only the photo: the text stays for everybody, the photo is gone for readers and told to staff as hidden.
	if code := s.setProfile(admin, frank, `{"imageHidden":true}`); code != http.StatusNoContent {
		t.Fatalf("hide the photo: %d", code)
	}
	reader, _ := s.personPage(ana, frank)
	staff, _ := s.personPage(admin, frank)
	if reader.Profile.Image != nil || reader.Profile.Bio == "" || reader.Profile.ImageHidden {
		t.Errorf("a reader: the photo is hidden and nothing says so: %+v", reader.Profile)
	}
	if staff.Profile.Image == nil || !staff.Profile.ImageHidden || staff.Profile.Hidden {
		t.Errorf("staff see the photo to show it again: %+v", staff.Profile)
	}

	// Hiding the profile: a reader sees none, staff see it marked.
	if code := s.setProfile(admin, frank, `{"hidden":true}`); code != http.StatusNoContent {
		t.Fatalf("hide: %d", code)
	}
	if p, _ := s.personPage(ana, frank); p.Profile != nil {
		t.Errorf("a reader sees a hidden profile: %+v", p.Profile)
	}
	if p, _ := s.personPage(admin, frank); p.Profile == nil || !p.Profile.Hidden || !p.Profile.ImageHidden || p.Profile.Description == "" {
		t.Errorf("staff see it marked hidden: %+v", p.Profile)
	}
	// What is not in the request is left as it was, and showing again is the same call.
	if code := s.setProfile(admin, frank, `{"hidden":false}`); code != http.StatusNoContent {
		t.Fatalf("show: %d", code)
	}
	p, _ := s.personPage(ana, frank)
	if p.Profile == nil || p.Profile.Image != nil || p.Profile.Bio == "" {
		t.Errorf("shown again, with the photo still hidden: %+v", p.Profile)
	}
	if code := s.setProfile(admin, frank, `{"imageHidden":false}`); code != http.StatusNoContent {
		t.Fatalf("show the photo: %d", code)
	}
	if p, _ := s.personPage(ana, frank); p.Profile.Image == nil {
		t.Errorf("the photo did not come back")
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.profile' AND target_id = $1`, fmt.Sprint(frank)); got != "4" {
		t.Errorf("audited %s times", got)
	}
	if got := s.scalar(`SELECT updated_by IS NOT NULL FROM person_profile WHERE person_id = $1`, frank); got != "true" {
		t.Errorf("who changed it is not kept: %s", got)
	}
}

func TestPersonProfile_RefusesWhatIsNotAChoiceOrAPersonWithNoProfile(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)
	for _, body := range []string{``, `{}`, `[]`, `{"hidden":"yes"}`, `{"hidden":null}`, `not json`, `{"hidden":` + strings.Repeat("1", 2000) + `}`} {
		if code := s.setProfile(admin, frank, body); code != http.StatusBadRequest {
			t.Errorf("%q: %d, want 400", body[:min(len(body), 20)], code)
		}
	}
	if code := s.setProfile(admin, 999999, `{"hidden":true}`); code != http.StatusNotFound {
		t.Errorf("no such person: %d", code)
	}
	s.exec(`INSERT INTO person (name) VALUES ('Sem Perfil')`)
	if code := s.setProfile(admin, s.personID("Sem Perfil"), `{"hidden":true}`); code != http.StatusNotFound {
		t.Errorf("a person with no profile: %d", code)
	}
	if code := s.do(admin, "PUT", "/admin/people/abc/profile", `{"hidden":true}`).Code; code != http.StatusBadRequest && code != http.StatusNotFound {
		t.Errorf("not a number: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.profile'`); got != "0" {
		t.Errorf("a refused request was audited: %s", got)
	}
	if p, _ := s.personPage(ana, frank); p.Profile == nil || p.Profile.Hidden {
		t.Errorf("a refused request changed it")
	}
}

func TestPersonProfile_GoesAwayWithThePersonAndTheProfileKeepsNoKey(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)
	raw := s.do(ana, "GET", fmt.Sprintf("/people/%d", frank), "").Body.String()
	var anyJSON map[string]any
	if json.Unmarshal([]byte(raw), &anyJSON) != nil {
		t.Fatalf("not JSON: %s", raw)
	}
	for _, leak := range []string{"updated_by", "updatedBy", "fetched", "bio_state", "bioState", "person_id"} {
		if strings.Contains(raw, leak) {
			t.Errorf("the page says %q: %s", leak, raw)
		}
	}
	s.exec(`DELETE FROM person WHERE id = $1`, frank)
	if got := s.scalar(`SELECT count(*) FROM person_profile`); got != "0" {
		t.Errorf("the profile of a person who is gone stayed: %s", got)
	}
}
