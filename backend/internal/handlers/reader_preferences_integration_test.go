package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const goodReader = `{"theme":"sepia","font":"dislexia","size":130,"spacing":"media","margins":"larga","justify":true}`

type readerPreferences struct {
	Choice    string          `json:"choice"`
	Effective string          `json:"effective"`
	Reader    json.RawMessage `json:"reader"`
}

func readerPrefs(s *catalogStack, a actor) readerPreferences {
	s.t.Helper()
	rec := s.do(a, "GET", "/auth/preferences", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /auth/preferences: %d %s", rec.Code, rec.Body)
	}
	var p readerPreferences
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		s.t.Fatal(err)
	}
	return p
}

func TestReaderPreferences_NobodyHasChosenUntilTheyDo(t *testing.T) {
	s := newCatalogStack(t)
	if got := readerPrefs(s, ana).Reader; string(got) != "null" {
		t.Fatalf("no choice: %s", got)
	}
}

func TestReaderPreferences_AChoiceIsKeptAndReadBack(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "PUT", "/auth/preferences", `{"reader":`+goodReader+`}`)
	if rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	var put readerPreferences
	json.Unmarshal(rec.Body.Bytes(), &put)
	for _, got := range []string{string(put.Reader), string(readerPrefs(s, ana).Reader)} {
		var back map[string]any
		json.Unmarshal([]byte(got), &back)
		if back["theme"] != "sepia" || back["font"] != "dislexia" || back["size"] != float64(130) || back["margins"] != "larga" || back["justify"] != true {
			t.Fatalf("read back: %s", got)
		}
	}
}

func TestReaderPreferences_AreTheirOwnerAndNobodyElses(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"reader":`+goodReader+`}`)
	if got := readerPrefs(s, bob).Reader; string(got) != "null" {
		t.Fatalf("bob sees ana's choice: %s", got)
	}
	s.do(bob, "PUT", "/auth/preferences", `{"reader":{"theme":"preto","font":"livro","size":90,"spacing":"livro","margins":"livro","justify":false}}`)
	if !strings.Contains(string(readerPrefs(s, ana).Reader), `"sepia"`) || !strings.Contains(string(readerPrefs(s, bob).Reader), `"preto"`) {
		t.Fatalf("each has their own: ana %s, bob %s", readerPrefs(s, ana).Reader, readerPrefs(s, bob).Reader)
	}
}

func TestReaderPreferences_WhatIsNotInTheListsIsRefusedAndKeepsWhatWas(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"reader":`+goodReader+`}`)
	for name, body := range map[string]string{
		"a color typed by hand":    `{"reader":{"theme":"#ff00ff","font":"livro","size":100,"spacing":"livro","margins":"livro","justify":false}}`,
		"a font of the web":        `{"reader":{"theme":"papel","font":"Comic Sans","size":100,"spacing":"livro","margins":"livro","justify":false}}`,
		"a size off the steps":     `{"reader":{"theme":"papel","font":"livro","size":133,"spacing":"livro","margins":"livro","justify":false}}`,
		"a size too big":           `{"reader":{"theme":"papel","font":"livro","size":400,"spacing":"livro","margins":"livro","justify":false}}`,
		"a field it does not know": `{"reader":{"theme":"papel","font":"livro","size":100,"spacing":"livro","margins":"livro","justify":false,"css":"x"}}`,
		"a missing field":          `{"reader":{"theme":"papel"}}`,
		"text for a number":        `{"reader":{"theme":"papel","font":"livro","size":"100","spacing":"livro","margins":"livro","justify":false}}`,
		"a list":                   `{"reader":[]}`,
		"a string":                 `{"reader":"papel"}`,
	} {
		if rec := s.do(ana, "PUT", "/auth/preferences", body); rec.Code != 400 {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if got := string(readerPrefs(s, ana).Reader); !strings.Contains(got, `"sepia"`) {
		t.Fatalf("a refused choice changed what was kept: %s", got)
	}
}

func TestReaderPreferences_NullTakesTheChoiceAway(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"reader":`+goodReader+`}`)
	if rec := s.do(ana, "PUT", "/auth/preferences", `{"reader":null}`); rec.Code != 200 {
		t.Fatalf("null: %d", rec.Code)
	}
	if got := readerPrefs(s, ana).Reader; string(got) != "null" {
		t.Fatalf("taken away: %s", got)
	}
}

func TestReaderPreferences_TheTwoChoicesDoNotTouchEachOther(t *testing.T) {
	s := newCatalogStack(t)
	// the reading choice leaves the name order alone ...
	s.do(ana, "PUT", "/auth/preferences", `{"nameOrder":"family_first"}`)
	s.do(ana, "PUT", "/auth/preferences", `{"reader":`+goodReader+`}`)
	if p := readerPrefs(s, ana); p.Choice != "family_first" {
		t.Fatalf("the name order was touched by a reading choice: %+v", p)
	}
	// ... and the name order leaves the reading choice alone, as it did not mention it
	s.do(ana, "PUT", "/auth/preferences", `{"nameOrder":"given_first"}`)
	p := readerPrefs(s, ana)
	if p.Choice != "given_first" || !strings.Contains(string(p.Reader), `"sepia"`) {
		t.Fatalf("the reading choice was touched by a name order: %+v %s", p, p.Reader)
	}
	// an empty name order goes back to the library's, and still keeps the reading choice
	s.do(ana, "PUT", "/auth/preferences", `{"nameOrder":""}`)
	if p := readerPrefs(s, ana); p.Choice != "" || !strings.Contains(string(p.Reader), `"sepia"`) {
		t.Fatalf("%+v %s", p, p.Reader)
	}
}

func TestReaderPreferences_BothInOneRequestAreBothKept(t *testing.T) {
	s := newCatalogStack(t)
	if rec := s.do(ana, "PUT", "/auth/preferences", fmt.Sprintf(`{"nameOrder":"family_first","reader":%s}`, goodReader)); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if p := readerPrefs(s, ana); p.Choice != "family_first" || !strings.Contains(string(p.Reader), `"dislexia"`) {
		t.Fatalf("%+v %s", p, p.Reader)
	}
}

func TestReaderPreferences_ARefusedNameOrderKeepsEverythingAsItWas(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"reader":`+goodReader+`}`)
	if rec := s.do(ana, "PUT", "/auth/preferences", fmt.Sprintf(`{"nameOrder":"sideways","reader":%s}`, strings.Replace(goodReader, "sepia", "preto", 1))); rec.Code != 400 {
		t.Fatalf("a name order that is not one: %d", rec.Code)
	}
	if got := string(readerPrefs(s, ana).Reader); !strings.Contains(got, `"sepia"`) {
		t.Fatalf("nothing of a refused request is kept: %s", got)
	}
}

func TestReaderPreferences_WhatIsKeptAndNoLongerValidIsAsIfThereWereNone(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`UPDATE users SET reader_prefs = '{"theme":"vermelho","font":"livro","size":100,"spacing":"livro","margins":"livro","justify":false}'::jsonb WHERE id = $1`, ana.id)
	if got := readerPrefs(s, ana).Reader; string(got) != "null" {
		t.Fatalf("an old value that is not in the lists any more: %s", got)
	}
}

type namePrefs struct {
	DisplayName      string `json:"displayName"`
	DisplayNameAsked bool   `json:"displayNameAsked"`
	Choice           string `json:"choice"`
}

func namePrefsOf(s *catalogStack, a actor) namePrefs {
	s.t.Helper()
	var p namePrefs
	json.Unmarshal(s.do(a, "GET", "/auth/preferences", "").Body.Bytes(), &p)
	return p
}

func TestDisplayName_NobodyWasAskedUntilTheyAnswer(t *testing.T) {
	s := newCatalogStack(t)
	if p := namePrefsOf(s, ana); p.DisplayName != "" || p.DisplayNameAsked {
		t.Fatalf("a new account: %+v", p)
	}
}

func TestDisplayName_IsCleanedKeptReadBackAndTheirOwn(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "PUT", "/auth/preferences", `{"displayName":"  Ana   Maria‮ "}`)
	var put namePrefs
	json.Unmarshal(rec.Body.Bytes(), &put)
	if rec.Code != 200 || put.DisplayName != "Ana Maria" || !put.DisplayNameAsked {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	if p := namePrefsOf(s, ana); p.DisplayName != "Ana Maria" || !p.DisplayNameAsked {
		t.Errorf("read back: %+v", p)
	}
	if p := namePrefsOf(s, bob); p.DisplayName != "" || p.DisplayNameAsked {
		t.Errorf("bob sees ana's name or was marked as asked: %+v", p)
	}
}

func TestDisplayName_AnswerOfMyUserNameIsAnAnswerAndIsNotAskedAgain(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"displayName":"Aninha"}`)
	if rec := s.do(ana, "PUT", "/auth/preferences", `{"displayName":""}`); rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	if p := namePrefsOf(s, ana); p.DisplayName != "" || !p.DisplayNameAsked {
		t.Errorf("after going back to the user name: %+v", p)
	}
	if s.scalar(`SELECT (display_name IS NULL)::text FROM users WHERE id = $1`, ana.id) != "true" {
		t.Errorf("the user name is kept as nothing, not as an empty text")
	}
	if rec := s.do(bob, "PUT", "/auth/preferences", `{"displayName":"   "}`); rec.Code != 200 || !namePrefsOf(s, bob).DisplayNameAsked {
		t.Errorf("an answer of only spaces is the user name, and counts as an answer")
	}
}

func TestDisplayName_ALongOneIsRefusedAndNothingChanges(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"displayName":"Ana"}`)
	if rec := s.do(ana, "PUT", "/auth/preferences", fmt.Sprintf(`{"displayName":%q}`, strings.Repeat("ç", 61))); rec.Code != 400 {
		t.Errorf("61 characters: %d, want 400", rec.Code)
	}
	if p := namePrefsOf(s, ana); p.DisplayName != "Ana" {
		t.Errorf("a refused name changed something: %+v", p)
	}
	if rec := s.do(ana, "PUT", "/auth/preferences", fmt.Sprintf(`{"displayName":%q}`, strings.Repeat("ç", 60))); rec.Code != 200 {
		t.Errorf("60 characters: %d, want 200", rec.Code)
	}
}

func TestDisplayName_OtherPreferencesLeaveItAloneAndItLeavesThem(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"displayName":"Ana","nameOrder":"family_first"}`)
	s.do(ana, "PUT", "/auth/preferences", `{"nameOrder":"given_first"}`)
	if p := namePrefsOf(s, ana); p.DisplayName != "Ana" || p.Choice != "given_first" {
		t.Errorf("a request that did not mention the name changed it: %+v", p)
	}
	s.do(ana, "PUT", "/auth/preferences", `{"displayName":"Aninha"}`)
	if p := namePrefsOf(s, ana); p.Choice != "given_first" {
		t.Errorf("a request that only had the name changed the order: %+v", p)
	}
}

func TestDisplayName_TheTimeItWasAskedDoesNotMoveWhenItIsChanged(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "PUT", "/auth/preferences", `{"displayName":"Ana"}`)
	first := s.scalar(`SELECT display_name_asked_at::text FROM users WHERE id = $1`, ana.id)
	s.do(ana, "PUT", "/auth/preferences", `{"displayName":"Aninha"}`)
	if got := s.scalar(`SELECT display_name_asked_at::text FROM users WHERE id = $1`, ana.id); got != first {
		t.Errorf("asked_at moved from %s to %s", first, got)
	}
}
