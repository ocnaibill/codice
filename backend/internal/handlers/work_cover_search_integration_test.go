package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (s *catalogStack) askCovers(a actor, work int, body string) (CoverSearch, int) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/works/%d/cover/search", work), body)
	var out CoverSearch
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func (s *catalogStack) coverSearchOf(a actor, work int) (CoverSearch, int) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/works/%d/cover/search", work), "")
	var out CoverSearch
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

// theWorkerFound does what the worker does when it answers: keeps the pictures in the folder and the list in the row.
func (s *catalogStack) theWorkerFound(work int, files map[string][]byte) {
	s.t.Helper()
	dir := filepath.Join(s.storage, "cache", "cover-search")
	os.MkdirAll(dir, 0o755)
	var results []CoverCandidate
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			s.t.Fatal(err)
		}
		results = append(results, CoverCandidate{File: name, Provider: "openlibrary", Source: "Open Library", Title: "Duna", Author: "Frank Herbert"})
	}
	raw, _ := json.Marshal(results)
	s.exec(`UPDATE work_cover_searches SET state = 'done', results = $2::jsonb, finished_at = now() WHERE work_id = $1`, work, string(raw))
}

func TestCoverSearch_AskingQueuesOneJobForTheWorkAndNothingIsPutOnIt(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`UPDATE editions SET cover_url = '/covers/duna.jpg' WHERE work_id = $1 AND is_primary`, duna)

	got, code := s.askCovers(admin, duna, `{"query":"  Dune   Frank "}`)
	if code != http.StatusAccepted || got.State != "pending" || got.Query != "Dune Frank" || len(got.Results) != 0 {
		t.Fatalf("ask: %d %+v", code, got)
	}
	if n := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'cover_search' AND state = 'pending'`); n != "1" {
		t.Errorf("jobs = %s", n)
	}
	if p := s.scalar(`SELECT payload::text FROM jobs WHERE type = 'cover_search'`); !strings.Contains(p, fmt.Sprintf(`"work": %d`, duna)) || !strings.Contains(p, `"query": "Dune Frank"`) {
		t.Errorf("payload = %s", p)
	}
	// Asking again while it waits is the same request.
	if _, code := s.askCovers(admin, duna, ``); code != http.StatusAccepted {
		t.Fatalf("again: %d", code)
	}
	if n := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'cover_search'`); n != "1" {
		t.Errorf("jobs after asking twice = %s", n)
	}
	// What was asked for is what the next answer is read against; and the cover of the work is the cover it was.
	if q := s.scalar(`SELECT query FROM work_cover_searches WHERE work_id = $1`, duna); q != "" {
		t.Errorf("query = %q, want the one of the last request (none)", q)
	}
	if c := s.coverOf(duna); c != "/covers/duna.jpg" {
		t.Errorf("cover = %s", c)
	}
	if s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'work.cover.search' AND target_id = $1`, fmt.Sprint(duna)) != "1" {
		t.Errorf("audit: one entry for the job that was queued")
	}
}

func TestCoverSearch_RefusesWhatCannotBeSearched(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	if _, code := s.askCovers(admin, 999999, ``); code != http.StatusNotFound {
		t.Errorf("a work that does not exist: %d", code)
	}
	if _, code := s.askCovers(admin, duna, `{"query":"`+strings.Repeat("a", 201)+`"}`); code != http.StatusBadRequest {
		t.Errorf("a query too long: %d", code)
	}
	if _, code := s.askCovers(admin, duna, `{not json`); code != http.StatusBadRequest {
		t.Errorf("not JSON: %d", code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, duna)
	if _, code := s.askCovers(admin, duna, ``); code != http.StatusNotFound {
		t.Errorf("a retired work: %d", code)
	}
	if n := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'cover_search'`); n != "0" {
		t.Errorf("jobs = %s", n)
	}
}

func TestCoverSearch_TheAnswerIsReadBackAndAskingAgainThrowsItAway(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	if _, code := s.coverSearchOf(admin, duna); code != http.StatusNotFound {
		t.Fatalf("before any search: %d", code)
	}
	s.askCovers(admin, duna, ``)
	if got, _ := s.coverSearchOf(admin, duna); got.State != "pending" || len(got.Results) != 0 {
		t.Errorf("while it waits: %+v", got)
	}
	s.theWorkerFound(duna, map[string][]byte{fmt.Sprintf("%d_0_aaaaaaaa.jpg", duna): []byte("jpeg")})
	got, code := s.coverSearchOf(admin, duna)
	if code != 200 || got.State != "done" || len(got.Results) != 1 || got.Results[0].Source != "Open Library" || got.Results[0].Provider != "openlibrary" {
		t.Fatalf("answered: %d %+v", code, got)
	}
	s.exec(`UPDATE jobs SET state = 'succeeded' WHERE type = 'cover_search'`)
	s.askCovers(admin, duna, `{"query":"Dune"}`)
	if got, _ := s.coverSearchOf(admin, duna); got.State != "pending" || len(got.Results) != 0 || got.Query != "Dune" {
		t.Errorf("asked again: %+v", got)
	}
}

func TestCoverSearch_AFoundPictureIsServedOnlyToTheWorkThatListsIt(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	other := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	name := fmt.Sprintf("%d_0_aaaaaaaa.jpg", duna)
	png := fmt.Sprintf("%d_1_bbbbbbbb.png", duna)
	s.askCovers(admin, duna, ``)
	s.theWorkerFound(duna, map[string][]byte{name: []byte("jpeg bytes"), png: []byte("png bytes")})
	// A picture left in the folder that the answer does not list is not reachable.
	os.WriteFile(filepath.Join(s.storage, "cache", "cover-search", fmt.Sprintf("%d_2_cccccccc.jpg", duna)), []byte("unlisted"), 0o644)

	get := func(work int, file string) (int, string, string) {
		rec := s.do(admin, "GET", fmt.Sprintf("/works/%d/cover/candidates/%s", work, file), "")
		return rec.Code, rec.Header().Get("Content-Type"), rec.Body.String()
	}
	if code, ct, body := get(duna, name); code != 200 || ct != "image/jpeg" || body != "jpeg bytes" {
		t.Errorf("listed jpeg: %d %s %q", code, ct, body)
	}
	if code, ct, _ := get(duna, png); code != 200 || ct != "image/png" {
		t.Errorf("listed png: %d %s", code, ct)
	}
	if code, _, _ := get(duna, fmt.Sprintf("%d_2_cccccccc.jpg", duna)); code != 404 {
		t.Errorf("unlisted: %d", code)
	}
	// Not by the work it was not found for, nor by a name that is not one the worker gives.
	if code, _, _ := get(other, name); code != 404 {
		t.Errorf("another work's address: %d", code)
	}
	for _, bad := range []string{"..%2F..%2Fetc%2Fpasswd", "x.jpg", name + ".svg", "%2e%2e%2f" + name} {
		if code, _, _ := get(duna, bad); code != 404 {
			t.Errorf("%q: %d", bad, code)
		}
	}
	// Not even when the other work's own answer, by some mistake, lists it: a name is only served to the work it was made for.
	s.askCovers(admin, other, ``)
	raw, _ := json.Marshal([]CoverCandidate{{File: name}})
	s.exec(`UPDATE work_cover_searches SET state = 'done', results = $2::jsonb WHERE work_id = $1`, other, string(raw))
	if code, _, _ := get(other, name); code != 404 {
		t.Errorf("a name listed by another work: %d", code)
	}
	// The folder expires: the list still says it, the picture is gone.
	os.Remove(filepath.Join(s.storage, "cache", "cover-search", name))
	if code, _, _ := get(duna, name); code != 404 {
		t.Errorf("expired: %d", code)
	}
}

func TestCoverSearch_NothingIsServedWhileTheSearchWaits(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	name := fmt.Sprintf("%d_0_aaaaaaaa.jpg", duna)
	s.askCovers(admin, duna, ``)
	s.theWorkerFound(duna, map[string][]byte{name: []byte("jpeg")})
	s.askCovers(admin, duna, ``) // a new search: the old answer is thrown away, the old picture may still be on disk
	if rec := s.do(admin, "GET", fmt.Sprintf("/works/%d/cover/candidates/%s", duna, name), ""); rec.Code != http.StatusNotFound {
		t.Errorf("picture of a search that was thrown away: %d", rec.Code)
	}
}
