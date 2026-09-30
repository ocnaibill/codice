package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (s *catalogStack) join(a actor, source, into int) (int, map[string]any) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/admin/works/%d/join", source), fmt.Sprintf(`{"into":%d}`, into))
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (s *catalogStack) split(a actor, edition int) (int, map[string]any) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/admin/editions/%d/split", edition), "")
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (s *catalogStack) editionOf(file int64) int {
	s.t.Helper()
	return mustAtoi(s.scalar(`SELECT edition_id FROM files WHERE id = $1`, file))
}

func mustAtoi(v string) int {
	var n int
	fmt.Sscan(v, &n)
	return n
}

// The four Dune files of the real test: each uploaded as a work of its own.
func (s *catalogStack) fourDunes() (en, pt, ptbr, pdf int) {
	en = s.addWork("Dune", "Frank Herbert", "en.epub", "epub")
	pt = s.addWork("Duna", "Frank Herbert", "pt.epub", "epub")
	ptbr = s.addWork("Duna 01 - Duna", "Frank Herbert", "ptbr.pdf", "pdf")
	pdf = s.addWork("Dune", "Herbert, Frank, author", "en.pdf", "pdf")
	return
}

func TestJoin_FilesKeepTheirIdsAndTheirReadingAndTheWorkKeepsWhatPeopleDidWithIt(t *testing.T) {
	s := newCatalogStack(t)
	keep, _, _, absorbed := s.fourDunes()
	keepFile, absFile := s.primaryFile(keep), s.primaryFile(absorbed)

	// Ana read the absorbed work's file to the end, took a note in it, favorited the work and finished it;
	// Bob has the kept work among his favorites.
	s.progress(ana, "PUT", absFile, `{"locator":{"type":"pdf","page":7},"percent":60}`) // page 8 to a person
	s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", absFile), `{"completed":true}`)
	s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", absorbed), `{"finished":true}`)
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", absorbed), "")
	s.do(bob, "POST", fmt.Sprintf("/works/%d/favorite", keep), "")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", absorbed), fmt.Sprintf(`{"quote":"da Ana","fileId":%d}`, absFile))
	s.exec(`INSERT INTO tags (name) VALUES ('Sci-Fi') ON CONFLICT DO NOTHING`)
	s.exec(`INSERT INTO work_tags SELECT $1, id FROM tags WHERE name = 'Sci-Fi'`, absorbed)
	keepEdition := s.editionOf(keepFile)
	s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'isbn')`, keep, absorbed)

	code, out := s.join(admin, absorbed, keep)
	if code != 200 || out["workId"] != float64(keep) || out["editions"] != float64(1) {
		t.Fatalf("join: %d %v", code, out)
	}

	// Both files are under the work that stayed, with their own ids, positions and the primary edition unchanged.
	w, _ := s.detail(ana, keep)
	if len(w.Editions) != 2 {
		t.Fatalf("editions = %d", len(w.Editions))
	}
	if got := s.scalar(`SELECT id FROM editions WHERE work_id = $1 AND is_primary`, keep); got != fmt.Sprint(keepEdition) {
		t.Errorf("the primary edition changed: %s", got)
	}
	if code, st := s.progress(ana, "GET", absFile, ""); code != 200 || st.Position != "8" || !st.Completed {
		t.Errorf("the reading position of the moved file: %d %+v", code, st)
	}
	// What people had done with the absorbed work is under the one that stayed.
	if got := s.scalar(`SELECT count(*) FROM reading_completions WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("completions = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_reading_state WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("the finished mark = %s", got)
	}
	if got := s.scalar(`SELECT string_agg(u.username, ',' ORDER BY u.username) FROM favorites f JOIN users u ON u.id = f.user_id WHERE f.work_id = $1`, keep); got != "ana,bob" {
		t.Errorf("favorites = %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM notes WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("notes = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_tags WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("tags = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_contributors WHERE work_id = $1`, keep); got != "2" {
		t.Errorf("authors = %s", got)
	}
	// The work that is left is retired, not deleted, and the pair is decided and audited.
	if got := s.scalar(`SELECT retired_at IS NOT NULL FROM works WHERE id = $1`, absorbed); got != "true" {
		t.Errorf("the absorbed work is not retired")
	}
	if _, code := s.detail(ana, absorbed); code != 404 {
		t.Errorf("a retired work is listed: %d", code)
	}
	if got := s.scalar(`SELECT state FROM duplicate_candidates WHERE work_a = $1 AND work_b = $2`, keep, absorbed); got != "linked" {
		t.Errorf("the pair = %s", got)
	}
	if got := s.scalar(`SELECT string_agg(action, ',') FROM audit_log WHERE action = 'work.join'`); got != "work.join" {
		t.Errorf("audit = %q", got)
	}
}

func TestJoin_AllFourFilesEndUpInOneWorkWhereTheEquivalentPositionIsOffered(t *testing.T) {
	s := newCatalogStack(t)
	en, pt, ptbr, pdf := s.fourDunes()
	for _, src := range []int{pt, ptbr, pdf} {
		if code, _ := s.join(admin, src, en); code != 200 {
			t.Fatalf("join %d: %d", src, code)
		}
	}
	w, _ := s.detail(ana, en)
	files := 0
	for _, e := range w.Editions {
		files += len(e.Files)
	}
	if len(w.Editions) != 4 || files != 4 {
		t.Fatalf("editions %d, files %d", len(w.Editions), files)
	}
	// Two files of different works could not be compared; now they can.
	a, b := s.primaryFile(en), s.scalar(`SELECT f.id FROM files f JOIN editions e ON e.id = f.edition_id WHERE e.work_id = $1 AND f.id <> $2 ORDER BY f.id LIMIT 1`, en, s.primaryFile(en))
	s.progress(ana, "PUT", a, `{"locator":{"type":"epub","href":"c.xhtml"}}`)
	if rec := s.do(ana, "GET", fmt.Sprintf("/progress/files/%s/equivalent?from=%d", b, a), ""); rec.Code != http.StatusOK {
		t.Errorf("equivalent position between joined versions: %d %s", rec.Code, rec.Body)
	}
}

func TestJoin_RefusesWhatItCannotDoAndChangesNothing(t *testing.T) {
	s := newCatalogStack(t)
	a, b, _, _ := s.fourDunes()
	if code, _ := s.join(admin, a, a); code != 400 {
		t.Errorf("a work into itself: %d", code)
	}
	if code, _ := s.join(admin, a, 99999); code != 404 {
		t.Errorf("unknown target: %d", code)
	}
	if code, _ := s.join(admin, 99999, a); code != 404 {
		t.Errorf("unknown source: %d", code)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/join", a), `{}`); rec.Code != 400 {
		t.Errorf("no target: %d", rec.Code)
	}
	if got := s.scalar(`SELECT count(*) FROM works WHERE retired_at IS NOT NULL`); got != "0" {
		t.Fatalf("a refused join retired something: %s", got)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, b)
	if code, _ := s.join(admin, b, a); code != 409 {
		t.Errorf("a retired source: %d", code)
	}
	if code, _ := s.join(admin, a, b); code != 409 {
		t.Errorf("a retired target: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM editions WHERE work_id = $1`, a); got != "1" {
		t.Errorf("editions moved by a refused join: %s", got)
	}
}

func TestSplit_PutsAnEditionBackWhereItCameFromAndRestoresThatWork(t *testing.T) {
	s := newCatalogStack(t)
	keep, _, _, absorbed := s.fourDunes()
	absFile := s.primaryFile(absorbed)
	edition := s.editionOf(absFile)
	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", absorbed), fmt.Sprintf(`{"quote":"no pdf","fileId":%d}`, absFile))
	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", keep), fmt.Sprintf(`{"quote":"no epub","fileId":%d}`, s.primaryFile(keep)))
	s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d/completion", absFile), `{"completed":true}`)
	s.join(admin, absorbed, keep)

	code, out := s.split(admin, edition)
	if code != 200 || out["workId"] != float64(absorbed) || out["restored"] != true {
		t.Fatalf("split: %d %v", code, out)
	}
	if got := s.scalar(`SELECT retired_at IS NULL FROM works WHERE id = $1`, absorbed); got != "true" {
		t.Errorf("the work it came from is not back")
	}
	if got := s.scalar(`SELECT work_id FROM editions WHERE id = $1`, edition); got != fmt.Sprint(absorbed) {
		t.Errorf("the edition is in work %s", got)
	}
	// What was about its file went with it; what was about the other one stayed.
	if got := s.scalar(`SELECT count(*) FROM notes WHERE work_id = $1`, absorbed); got != "1" {
		t.Errorf("notes of the restored work = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM notes WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("notes of the work that stayed = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM reading_completions WHERE work_id = $1`, absorbed); got != "1" {
		t.Errorf("completions of the restored work = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM reading_completions WHERE work_id = $1`, keep); got != "0" {
		t.Errorf("completions left behind = %s", got)
	}
	// Each work has exactly one primary edition, and the restored work has its own title and author.
	for _, w := range []int{keep, absorbed} {
		if got := s.scalar(`SELECT count(*) FROM editions WHERE work_id = $1 AND is_primary`, w); got != "1" {
			t.Errorf("work %d primary editions = %s", w, got)
		}
	}
	if got := s.scalar(`SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = $1`, absorbed); got != "Herbert, Frank, author" {
		t.Errorf("the restored work's author = %q (joining must not have changed it)", got)
	}
	if _, code := s.detail(ana, absorbed); code != 200 {
		t.Errorf("the restored work is not listed: %d", code)
	}
}

func TestSplit_AnEditionWithNowhereToGoGetsAWorkOfItsOwn(t *testing.T) {
	s := newCatalogStack(t)
	work, _, pdf := s.bookWithTwoFiles()
	other := s.addEdition(work, "en")
	s.addFile(other, "epub", "en.epub", "managed")
	edition := s.editionOf(pdf)
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", work), "")

	code, out := s.split(admin, edition)
	if code != 200 || out["restored"] != false {
		t.Fatalf("split: %d %v", code, out)
	}
	newWork := int(out["workId"].(float64))
	if newWork == work {
		t.Fatal("no new work")
	}
	if got := s.scalar(`SELECT original_title FROM works WHERE id = $1`, newWork); got != "Duna" {
		t.Errorf("title = %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM work_contributors WHERE work_id = $1`, newWork); got != "1" {
		t.Errorf("authors = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM favorites WHERE work_id = $1`, newWork); got != "1" {
		t.Errorf("the favorite did not follow: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM editions WHERE work_id = $1 AND is_primary`, newWork); got != "1" {
		t.Errorf("the new work has no primary edition")
	}
	if got := s.scalar(`SELECT count(*) FROM editions WHERE work_id = $1 AND is_primary`, work); got != "1" {
		t.Errorf("the old work lost its primary edition")
	}
}

func TestSplit_TheOnlyEditionOfAWorkStaysAndAnUnknownOneIsNotFound(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Solo", "X", "a.epub", "epub")
	edition := s.editionOf(s.primaryFile(work))
	if code, _ := s.split(admin, edition); code != 400 {
		t.Errorf("the only edition: %d", code)
	}
	if code, _ := s.split(admin, 99999); code != 404 {
		t.Errorf("unknown: %d", code)
	}
	if got := s.scalar(`SELECT count(*) FROM works`); got != "1" {
		t.Errorf("a refused split made a work: %s", got)
	}
}

func TestNotTheSame_IsRememberedAndTheScanNeverProposesThePairAgain(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	b := s.addWork("Duna", "Frank Herbert", "b.pdf", "pdf")
	c := s.addWork("Outro", "Z", "c.epub", "epub")

	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/not-same-as", b), fmt.Sprintf(`{"workId":%d}`, a)); rec.Code != 204 {
		t.Fatalf("not-same-as: %d %s", rec.Code, rec.Body)
	}
	for i := 0; i < 2; i++ { // a scan of everything, twice
		s.do(admin, "POST", "/admin/duplicates/scan", "")
	}
	s.exec(`DELETE FROM jobs`)
	if got := s.scalar(`SELECT state || ':' || reason FROM duplicate_candidates WHERE work_a = $1 AND work_b = $2`, a, b); got != "dismissed:manual" {
		t.Errorf("the pair = %q", got)
	}
	// Even after the pair was proposed by the system and left pending, the decision wins.
	s.exec(`DELETE FROM duplicate_candidates`)
	s.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`, a, c)
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/not-same-as", c), fmt.Sprintf(`{"workId":%d}`, a)); rec.Code != 204 {
		t.Fatalf("second: %d", rec.Code)
	}
	if got := s.scalar(`SELECT state FROM duplicate_candidates WHERE work_a = $1 AND work_b = $2`, a, c); got != "dismissed" {
		t.Errorf("a pending pair that a person said is not the same = %q", got)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/not-same-as", a), fmt.Sprintf(`{"workId":%d}`, a)); rec.Code != 400 {
		t.Errorf("with itself: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/works/%d/not-same-as", a), `{"workId":99999}`); rec.Code != 404 {
		t.Errorf("unknown: %d", rec.Code)
	}
	if !strings.Contains(s.scalar(`SELECT string_agg(action, ',') FROM audit_log`), "work.not_same") {
		t.Errorf("not audited")
	}
}

func TestJoin_TheEarliestDateOnWhichSomeoneFinishedTheWorkStaysAndAcceptedPositionsFollow(t *testing.T) {
	s := newCatalogStack(t)
	keep, _, _, absorbed := s.fourDunes()
	keepFile, absFile := s.primaryFile(keep), s.primaryFile(absorbed)
	// (Reading on in a file that is not finished clears the work's finished mark, DEC-080: save first.)
	s.progress(ana, "PUT", absFile, `{"locator":{"type":"pdf","page":1}}`)
	s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", keep), `{"finished":true}`)
	s.do(ana, "PUT", fmt.Sprintf("/progress/works/%d/finished", absorbed), `{"finished":true}`)
	s.exec(`UPDATE work_reading_state SET finished_at = '2020-01-01T00:00:00Z' WHERE work_id = $1`, absorbed)
	body := fmt.Sprintf(`{"sourceFileId":%d,"locator":{"type":"pdf","page":1},"method":"text","confidence":"high","precision":"passage"}`, absFile)
	if rec := s.do(ana, "POST", fmt.Sprintf("/progress/files/%d/equivalent/accept", keepFile), body); rec.Code != http.StatusCreated {
		t.Fatalf("accept: %d", rec.Code)
	}
	s.exec(`UPDATE equivalent_position_acceptances SET work_id = $1`, absorbed)

	if code, _ := s.join(admin, absorbed, keep); code != 200 {
		t.Fatalf("join: %d", code)
	}
	if got := s.scalar(`SELECT to_char(finished_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') FROM work_reading_state WHERE work_id = $1`, keep); got != "2020-01-01" {
		t.Errorf("finished on %s, want the earliest date", got)
	}
	if got := s.scalar(`SELECT count(*) FROM equivalent_position_acceptances WHERE work_id = $1`, keep); got != "1" {
		t.Errorf("accepted positions under the work that stayed = %s", got)
	}
}
