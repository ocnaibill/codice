package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func (s *catalogStack) credit(a actor, work int, name, role string) (Contributor, *httptest.ResponseRecorder) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/works/%d/contributors", work), fmt.Sprintf(`{"name":%q,"role":%q}`, name, role))
	var c Contributor
	json.Unmarshal(rec.Body.Bytes(), &c)
	return c, rec
}

func (s *catalogStack) credits(work int) []Contributor {
	s.t.Helper()
	w, code := s.detail(admin, work)
	if code != 200 || w.Metadata == nil {
		s.t.Fatalf("detail of %d: %d", work, code)
	}
	return w.Metadata.Contributors
}

func creditList(cs []Contributor) string {
	var out []string
	for _, c := range cs {
		out = append(out, fmt.Sprintf("%s|%s|%d", c.Name, c.Role, c.Position))
	}
	return strings.Join(out, ";")
}

func TestWorkContributors_AddingCreditsAPersonAfterTheOthersOfTheSameRole(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")

	c, rec := s.credit(admin, duna, "  Brian   Herbert ", "author")
	if rec.Code != 201 || c.Name != "Brian Herbert" || c.Role != "author" || c.Position != 1 || c.PersonID == 0 {
		t.Fatalf("coauthor: %d %+v", rec.Code, c)
	}
	// Other roles have their own row of places, and the order the roles are listed in does not follow the order they were added.
	for _, add := range [][2]string{{"Narrador Um", "narrator"}, {"Tradutora Um", "translator"}, {"Tradutora Dois", "translator"}, {"Ilustrador Um", "illustrator"}, {"Editora Um", "editor"}} {
		if _, rec := s.credit(admin, duna, add[0], add[1]); rec.Code != 201 {
			t.Fatalf("%v: %d %s", add, rec.Code, rec.Body.String())
		}
	}
	want := "Frank Herbert|author|0;Brian Herbert|author|1;Tradutora Um|translator|0;Tradutora Dois|translator|1;Narrador Um|narrator|0;Editora Um|editor|0;Ilustrador Um|illustrator|0"
	if got := creditList(s.credits(duna)); got != want {
		t.Errorf("credits =\n %s\nwant\n %s", got, want)
	}
	// The card says all the authors, and nobody else.
	if w, _ := s.detail(ana, duna); w.Author != "Frank Herbert, Brian Herbert" {
		t.Errorf("author of the card = %q", w.Author)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.contributor_add'`); n != "6" {
		t.Errorf("audit entries = %s, want 6", n)
	}
}

func TestWorkContributors_ThePersonIsTheOneTheNameStandsForAndHasTheRoleOnce(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	other := s.addWork("Outro", "Brian Herbert", "o.epub", "epub")
	brian := s.personID("Brian Herbert")

	// A name that is a person of the library is that person: the library does not make a second one.
	c, rec := s.credit(admin, duna, "  Brian   Herbert ", "author")
	if rec.Code != 201 || c.PersonID != brian || c.Name != "Brian Herbert" {
		t.Errorf("a person that exists: %d %+v (person %d)", rec.Code, c, brian)
	}
	// The same person with the same role twice (and the author the work started with).
	for _, name := range []string{"Brian Herbert", "Frank   Herbert"} {
		if _, rec := s.credit(admin, duna, name, "author"); rec.Code != 409 {
			t.Errorf("%q again as an author: %d, want 409", name, rec.Code)
		}
	}
	// The same person may have another role, and be credited on another work.
	if _, rec := s.credit(admin, duna, "Brian Herbert", "editor"); rec.Code != 201 {
		t.Errorf("another role: %d", rec.Code)
	}
	if _, rec := s.credit(admin, other, "Frank Herbert", "author"); rec.Code != 201 {
		t.Errorf("another work: %d", rec.Code)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM person WHERE name IN ('Brian Herbert', 'Frank Herbert')`); n != "2" {
		t.Errorf("people = %s: a person was made twice", n)
	}
	// A name written another way is a person of its own for now: the queue of people that look alike is where an admin joins them.
	c, rec = s.credit(admin, duna, "Herbert, Kevin", "author")
	if rec.Code != 201 || c.Name != "Herbert, Kevin" {
		t.Errorf("a name the library does not know: %d %+v", rec.Code, c)
	}
}

func TestWorkContributors_WhatIsNotAPersonOrARoleIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	for name, body := range map[string]string{
		"no name":      `{"name":"","role":"author"}`,
		"spaces":       `{"name":"   ","role":"author"}`,
		"too long":     fmt.Sprintf(`{"name":%q,"role":"author"}`, strings.Repeat("x", 201)),
		"no role":      `{"name":"Alguém"}`,
		"unknown role": `{"name":"Alguém","role":"composer"}`,
		"not json":     `nope`,
	} {
		if rec := s.do(admin, "POST", fmt.Sprintf("/works/%d/contributors", duna), body); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if _, rec := s.credit(admin, 99999, "Alguém", "author"); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if rec := s.do(admin, "POST", "/works/abc/contributors", `{"name":"x","role":"author"}`); rec.Code != 404 {
		t.Errorf("bad id: %d, want 404", rec.Code)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM work_contributors WHERE work_id = $1`, duna); n != "1" {
		t.Errorf("people credited = %s, want only the author it had", n)
	}
}

func TestWorkContributors_AWorkHasAtMostFiftyPeopleCredited(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`INSERT INTO person (name) SELECT 'Pessoa ' || g FROM generate_series(1, 49) g`)
	s.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'narrator', (id % 30)::smallint FROM person WHERE name LIKE 'Pessoa %'`, duna)
	if _, rec := s.credit(admin, duna, "Mais Uma", "author"); rec.Code != 409 {
		t.Errorf("the 51st person: %d, want 409", rec.Code)
	}
	other := s.addWork("Outra", "X", "o.epub", "epub")
	if _, rec := s.credit(admin, other, "Mais Uma", "author"); rec.Code != 201 {
		t.Errorf("the bound is of each work: %d, want 201", rec.Code)
	}
}

func TestWorkContributors_RemovingTakesOnlyThatRoleAndKeepsTheRowOfPlaces(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.credit(admin, duna, "Brian Herbert", "author")
	s.credit(admin, duna, "Kevin Anderson", "author")
	s.credit(admin, duna, "Brian Herbert", "editor")
	brian := s.personID("Brian Herbert")

	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/contributors/%d/author", duna, brian), ""); rec.Code != 204 {
		t.Fatalf("remove: %d", rec.Code)
	}
	// The others close ranks, and the same person stays with the other role and in the library.
	if got := creditList(s.credits(duna)); got != "Frank Herbert|author|0;Kevin Anderson|author|1;Brian Herbert|editor|0" {
		t.Errorf("credits = %s", got)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM person WHERE id = $1`, brian); n != "1" {
		t.Error("the person went from the library")
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/contributors/%d/author", duna, brian), ""); rec.Code != 404 {
		t.Errorf("removing twice: %d, want 404", rec.Code)
	}
	for _, target := range []string{
		fmt.Sprintf("/works/%d/contributors/%d/composer", duna, brian), fmt.Sprintf("/works/%d/contributors/abc/author", duna),
		fmt.Sprintf("/works/%d/contributors/0/author", duna), "/works/abc/contributors/1/author",
	} {
		if rec := s.do(admin, "DELETE", target, ""); rec.Code != 404 {
			t.Errorf("DELETE %s: %d, want 404", target, rec.Code)
		}
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/99999/contributors/%d/author", brian), ""); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.contributor_remove'`); n != "1" {
		t.Errorf("audit entries = %s, want 1", n)
	}
}

func TestWorkContributors_ClosingRanksOfOneRoleLeavesTheOtherRolesOfTheSamePersonAlone(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.credit(admin, duna, "Kevin Anderson", "author")
	s.credit(admin, duna, "Brian Herbert", "author")
	s.credit(admin, duna, "Editor Um", "editor")
	s.credit(admin, duna, "Editor Dois", "editor")
	s.credit(admin, duna, "Brian Herbert", "editor")
	kevin := s.personID("Kevin Anderson")
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d/contributors/%d/author", duna, kevin), ""); rec.Code != 204 {
		t.Fatalf("remove: %d", rec.Code)
	}
	if got := creditList(s.credits(duna)); got != "Frank Herbert|author|0;Brian Herbert|author|1;Editor Um|editor|0;Editor Dois|editor|1;Brian Herbert|editor|2" {
		t.Errorf("credits = %s: the other role of Brian was renumbered with his authorship", got)
	}
}

func TestWorkContributors_OrderingIsAuditedAndOfTheRoleGiven(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.credit(admin, duna, "Tradutora A", "translator")
	s.credit(admin, duna, "Tradutora B", "translator")
	a, b := s.personID("Tradutora A"), s.personID("Tradutora B")
	if rec := s.do(admin, "PUT", fmt.Sprintf("/works/%d/contributors/order", duna), fmt.Sprintf(`{"role":"translator","personIds":[%d,%d]}`, b, a)); rec.Code != 204 {
		t.Fatalf("order: %d", rec.Code)
	}
	if got := creditList(s.credits(duna)); got != "Frank Herbert|author|0;Tradutora B|translator|0;Tradutora A|translator|1" {
		t.Errorf("credits = %s", got)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.contributor_order'`); n != "1" {
		t.Errorf("audit entries = %s, want 1", n)
	}
}

func TestWorkContributors_OrderingNumbersThePeopleOfTheRoleInTheListGiven(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.credit(admin, duna, "Brian Herbert", "author")
	s.credit(admin, duna, "Kevin Anderson", "author")
	s.credit(admin, duna, "Tradutora", "translator")
	frank, brian, kevin, tr := s.personID("Frank Herbert"), s.personID("Brian Herbert"), s.personID("Kevin Anderson"), s.personID("Tradutora")
	put := func(body string) int {
		return s.do(admin, "PUT", fmt.Sprintf("/works/%d/contributors/order", duna), body).Code
	}

	if code := put(fmt.Sprintf(`{"role":"author","personIds":[%d,%d,%d]}`, kevin, frank, brian)); code != 204 {
		t.Fatalf("order: %d", code)
	}
	if got := creditList(s.credits(duna)); got != "Kevin Anderson|author|0;Frank Herbert|author|1;Brian Herbert|author|2;Tradutora|translator|0" {
		t.Errorf("credits = %s", got)
	}
	if w, _ := s.detail(ana, duna); w.Author != "Kevin Anderson, Frank Herbert, Brian Herbert" {
		t.Errorf("author of the card = %q", w.Author)
	}
	for name, body := range map[string]string{
		"missing":      fmt.Sprintf(`{"role":"author","personIds":[%d,%d]}`, kevin, frank),
		"repeated":     fmt.Sprintf(`{"role":"author","personIds":[%d,%d,%d,%d]}`, kevin, kevin, frank, brian),
		"foreign":      fmt.Sprintf(`{"role":"author","personIds":[%d,%d,%d]}`, kevin, frank, tr),
		"unknown":      fmt.Sprintf(`{"role":"author","personIds":[%d,%d,99999]}`, kevin, frank),
		"empty":        `{"role":"author","personIds":[]}`,
		"unknown role": fmt.Sprintf(`{"role":"composer","personIds":[%d]}`, tr),
		"not json":     `nope`,
	} {
		if code := put(body); code != 400 {
			t.Errorf("order %s: %d, want 400", name, code)
		}
	}
	if code := put(fmt.Sprintf(`{"role":"translator","personIds":[%d]}`, tr)); code != 204 {
		t.Errorf("a role with one person: %d, want 204", code)
	}
	if rec := s.do(admin, "PUT", "/works/99999/contributors/order", `{"role":"author","personIds":[]}`); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if got := creditList(s.credits(duna)); !strings.HasPrefix(got, "Kevin Anderson|author|0;Frank Herbert|author|1;") {
		t.Errorf("a refused order changed the credits: %s", got)
	}
}

func TestWorkContributors_ChangingWhoIsTheFirstAuthorConfirmsItByHandAndOnlyThat(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`INSERT INTO notes (user_id, work_id, quote, source_title, source_author) VALUES ($1, $2, 'trecho', 'Duna', 'Frank Herbert')`, idAna, duna)
	locked := func() string {
		return s.scalar(`SELECT w.author_lock::text || '|' || COALESCE((SELECT source FROM work_field_sources WHERE work_id = w.id AND field = 'author'), '-') FROM works w WHERE w.id = $1`, duna)
	}
	if locked() != "false|-" {
		t.Fatalf("start: %s", locked())
	}

	// A coauthor, a translator and putting the authors in the same order leave the first author as it is.
	s.credit(admin, duna, "Brian Herbert", "author")
	s.credit(admin, duna, "Tradutora", "translator")
	frank, brian := s.personID("Frank Herbert"), s.personID("Brian Herbert")
	s.do(admin, "PUT", fmt.Sprintf("/works/%d/contributors/order", duna), fmt.Sprintf(`{"role":"author","personIds":[%d,%d]}`, frank, brian))
	if locked() != "false|-" {
		t.Errorf("nothing changed who the first author is, yet: %s", locked())
	}

	// Putting another first changes it: the author is confirmed by hand, and the notes follow.
	s.do(admin, "PUT", fmt.Sprintf("/works/%d/contributors/order", duna), fmt.Sprintf(`{"role":"author","personIds":[%d,%d]}`, brian, frank))
	if locked() != "true|manual" {
		t.Errorf("after putting another first: %s", locked())
	}
	if n := s.scalar(`SELECT source_author FROM notes WHERE work_id = $1`, duna); n != "Brian Herbert" {
		t.Errorf("what the notes remember of the author = %q", n)
	}
	// Taking the first away makes the next one the first.
	s.exec(`UPDATE works SET author_lock = FALSE WHERE id = $1`, duna)
	s.exec(`DELETE FROM work_field_sources WHERE work_id = $1`, duna)
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d/contributors/%d/author", duna, brian), "")
	if locked() != "true|manual" || s.scalar(`SELECT source_author FROM notes WHERE work_id = $1`, duna) != "Frank Herbert" {
		t.Errorf("after removing the first: %s", locked())
	}
}

func TestWorkContributors_TheFirstAuthorOfAWorkThatHadNoneIsConfirmedAndTheNotesFollow(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Sem autor", "", "s.epub", "epub")
	s.exec(`INSERT INTO notes (user_id, work_id, quote, source_title) VALUES ($1, $2, 'trecho', 'Sem autor')`, idAna, w)
	if _, rec := s.credit(admin, w, "Alguém", "author"); rec.Code != 201 {
		t.Fatal(rec.Code)
	}
	if v := s.scalar(`SELECT author_lock::text FROM works WHERE id = $1`, w); v != "true" {
		t.Errorf("author_lock = %s, want true", v)
	}
	if n := s.scalar(`SELECT source_author FROM notes WHERE work_id = $1`, w); n != "Alguém" {
		t.Errorf("notes = %q", n)
	}
	// Taking the only author leaves none: the notes say none.
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d/contributors/%d/author", w, s.personID("Alguém")), "")
	if n := s.scalar(`SELECT COALESCE(source_author, '<none>') FROM notes WHERE work_id = $1`, w); n != "<none>" {
		t.Errorf("notes after taking the only author = %q", n)
	}
}

func TestWorkContributors_AWorkInTheTrashCanBeCorrectedAndItsNotesAreLeftAlone(t *testing.T) {
	s := newCatalogStack(t)
	w := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.exec(`INSERT INTO notes (user_id, work_id, quote, source_title, source_author) VALUES ($1, $2, 'trecho', 'Duna', 'Frank Herbert')`, idAna, w)
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, w)
	frank, _ := s.credit(admin, w, "Brian Herbert", "author")
	if frank.Position != 1 {
		t.Errorf("a coauthor of a work in the trash: %+v", frank)
	}
	s.do(admin, "PUT", fmt.Sprintf("/works/%d/contributors/order", w), fmt.Sprintf(`{"role":"author","personIds":[%d,%d]}`, s.personID("Brian Herbert"), s.personID("Frank Herbert")))
	if n := s.scalar(`SELECT source_author FROM notes WHERE work_id = $1`, w); n != "Frank Herbert" {
		t.Errorf("notes of a work in the trash = %q, want them left as they were", n)
	}
}

func TestWorkContributors_ACoauthorIsFoundByTheSearch(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	s.credit(admin, duna, "Kevin Anderson", "author")
	if got := ids(s.list(ana, "?search=anderson").Data); !sameIDs(got, duna) {
		t.Errorf("search for the coauthor = %v, want %v", got, duna)
	}
}

func TestWorkContributors_TwoChangesAtTheSameTimeWaitForEachOther(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Somebody else has the work in hand; the foreign key of a credit would not wait for this lock, adding one must.
	if _, err := tx.Exec(`SELECT 1 FROM works WHERE id = $1 FOR NO KEY UPDATE`, duna); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		_, rec := s.credit(admin, duna, "Brian Herbert", "author")
		done <- rec.Code
	}()
	select {
	case code := <-done:
		t.Fatalf("the second change did not wait (answered %d)", code)
	case <-time.After(400 * time.Millisecond):
	}
	tx.Commit()
	if code := <-done; code != 201 {
		t.Errorf("after the wait: %d, want 201", code)
	}
}
