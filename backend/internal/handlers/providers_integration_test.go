package handlers

import (
	"encoding/json"
	"github.com/ocnaibill/codice/backend/internal/metaproviders"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type providerRow struct {
	ID            string
	Name          string
	Sends         []string
	Key           string
	Enabled       bool
	KeyConfigured *bool
	Health        *struct {
		State     string
		Status    int
		Problem   string
		CheckedAt string
		LastOkAt  *string
		Empty     bool
	}
	Test *struct {
		OK       bool
		State    string
		Status   int
		Results  int
		Ms       int
		TestedAt string
	}
	Testing bool
}

func (s *catalogStack) providers() []providerRow {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/metadata-providers", "")
	if rec.Code != 200 {
		s.t.Fatalf("providers: %d", rec.Code)
	}
	var out struct{ Data []providerRow }
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Data
}

func (s *catalogStack) setProvider(id, body string) int {
	return s.do(admin, "PUT", "/admin/metadata-providers/"+id, body).Code
}

func summary(rows []providerRow) string {
	var parts []string
	for _, r := range rows {
		on := "off"
		if r.Enabled {
			on = "on"
		}
		parts = append(parts, r.ID+"="+on)
	}
	return strings.Join(parts, " ")
}

func TestProviders_EveryOneIsOffUntilTheOwnerTurnsItOnAndSaysWhatItSends(t *testing.T) {
	s := newCatalogStack(t)
	rows := s.providers()
	if got := summary(rows); got != "google_books=off openlibrary=off comicvine=off anilist=off mangadex=off wikidata=off wikipedia=off" {
		t.Fatalf("by default: %s", got)
	}
	for _, r := range rows {
		first := "title"
		if r.ID == "wikipedia" {
			first = "page_title" // it is only asked for the page Wikidata says the work has, never for the title of the file
		}
		if r.Name == "" || len(r.Sends) == 0 || r.Sends[0] != first {
			t.Errorf("%s must say what it receives, %s first: %+v", r.ID, first, r)
		}
	}
	// Google Books and Open Library find a book by the ISBN of the file (DEC-142), and Open Library is also asked about an author whose key
	// an administrator accepted; each says so.
	if got := strings.Join(rows[0].Sends, ","); got != "title,isbn" {
		t.Errorf("Google Books receives %q", got)
	}
	if got := strings.Join(rows[1].Sends, ","); got != "title,isbn,author_key" {
		t.Errorf("Open Library receives %q", got)
	}
	for _, i := range []int{2, 3, 4, 5} {
		if len(rows[i].Sends) != 1 || rows[i].Sends[0] != "title" {
			t.Errorf("%s receives only the title: %v", rows[i].ID, rows[i].Sends)
		}
	}
	if got := strings.Join(rows[6].Sends, ","); got != "page_title" {
		t.Errorf("Wikipedia receives only the title of a page: %q", got)
	}
	if rows[0].Key != "required" || rows[1].Key != "" || rows[2].Key != "required" || rows[3].Key != "" || rows[4].Key != "" || rows[5].Key != "" || rows[6].Key != "" {
		t.Errorf("Google Books and ComicVine take a key they cannot do without, the others none: %+v", rows)
	}
	if got := rows[3].ID + "," + rows[4].ID + "," + rows[3].Name + "," + rows[4].Name; got != "anilist,mangadex,AniList,MangaDex" {
		t.Errorf("the manga providers are asked after ComicVine: %s", got)
	}
	if got := rows[5].ID + "," + rows[6].ID + "," + rows[5].Name + "," + rows[6].Name; got != "wikidata,wikipedia,Wikidata,Wikipedia" {
		t.Errorf("Wikidata and Wikipedia are last: %s", got)
	}
	for _, r := range rows {
		if r.KeyConfigured != nil {
			t.Errorf("%s: the worker has not said anything about keys yet, so nothing is known: %v", r.ID, *r.KeyConfigured)
		}
	}
}

func TestProviders_TurningOneOnOrOffLeavesTheOthersAndIsAudited(t *testing.T) {
	s := newCatalogStack(t)
	if code := s.setProvider("openlibrary", `{"enabled":true}`); code != 204 {
		t.Fatalf("on: %d", code)
	}
	if got := summary(s.providers()); got != "google_books=off openlibrary=on comicvine=off anilist=off mangadex=off wikidata=off wikipedia=off" {
		t.Errorf("after turning Open Library on: %s", got)
	}
	s.setProvider("google_books", `{"enabled":true}`)
	s.setProvider("openlibrary", `{"enabled":false}`)
	if got := summary(s.providers()); got != "google_books=on openlibrary=off comicvine=off anilist=off mangadex=off wikidata=off wikipedia=off" {
		t.Errorf("after the others: %s", got)
	}
	// What the worker reads is exactly this, with real booleans.
	if got := s.scalar(`SELECT value::text FROM settings WHERE key = 'metadata.providers'`); got != `{"openlibrary": false, "google_books": true}` {
		t.Errorf("setting = %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'providers.set' AND target_type = 'metadata_provider'`); got != "3" {
		t.Errorf("audit entries = %s, want 3", got)
	}
	if got := s.scalar(`SELECT string_agg(details->>'enabled', ',' ORDER BY id) FROM audit_log WHERE action = 'providers.set' AND target_id = 'openlibrary'`); got != "true,false" {
		t.Errorf("audit says %q for Open Library: it was turned on, then off", got)
	}
}

func TestProviders_RefusesWhatIsNotAYesOrANoOrAProviderThatDoesNotExistAndChangesNothing(t *testing.T) {
	s := newCatalogStack(t)
	for _, body := range []string{``, `{}`, `{"enabled":"yes"}`, `{"enabled":1}`, `{"enabled":null}`, `not json`} {
		if code := s.setProvider("openlibrary", body); code != 400 {
			t.Errorf("body %q: %d, want 400", body, code)
		}
	}
	if code := s.setProvider("bing", `{"enabled":true}`); code != 404 {
		t.Errorf("unknown provider: %d, want 404", code)
	}
	if got := s.scalar(`SELECT count(*) FROM settings WHERE key = 'metadata.providers'`); got != "0" {
		t.Errorf("a refused request saved something")
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'providers.set'`); got != "0" {
		t.Errorf("a refused request was audited as a change")
	}
}

func TestProviders_TwoChangedAtTheSameTimeDoNotUndoEachOther(t *testing.T) {
	s := newCatalogStack(t)
	var wg sync.WaitGroup
	for _, id := range []string{"google_books", "openlibrary", "comicvine", "anilist", "mangadex", "wikidata", "wikipedia"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				s.do(admin, "PUT", "/admin/metadata-providers/"+id, `{"enabled":true}`)
			}
		}()
	}
	wg.Wait()
	if got := summary(s.providers()); got != "google_books=on openlibrary=on comicvine=on anilist=on mangadex=on wikidata=on wikipedia=on" {
		t.Errorf("after turning all on at once: %s", got)
	}
}

func TestProviders_OnlyAPlainYesTurnsOneOnAndASettingThatIsNotAChoiceIsReplaced(t *testing.T) {
	s := newCatalogStack(t)
	for _, raw := range []string{`{"openlibrary": "true", "google_books": 1, "comicvine": null}`, `"yes"`, `[true]`, `true`} {
		s.exec(`INSERT INTO settings (key, value) VALUES ('metadata.providers', $1::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, raw)
		if got := summary(s.providers()); got != "google_books=off openlibrary=off comicvine=off anilist=off mangadex=off wikidata=off wikipedia=off" {
			t.Errorf("setting %s: %s, want all off", raw, got)
		}
	}
	// Choosing over a setting that is not an object makes it one.
	s.setProvider("openlibrary", `{"enabled":true}`)
	if got := summary(s.providers()); got != "google_books=off openlibrary=on comicvine=off anilist=off mangadex=off wikidata=off wikipedia=off" {
		t.Errorf("after choosing: %s", got)
	}
}

func TestSearchMetadata_NeverReachesTheWorkerWhileNoProviderIsOn(t *testing.T) {
	var hits atomic.Int32
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"results":[{"source":"Open Library","title":"Dune"}],"query":"Dune"}`))
	}))
	defer worker.Close()
	t.Setenv("WORKER_SEARCH_URL", worker.URL)
	s := newCatalogStack(t)

	rec := s.do(admin, "GET", "/metadata/search?q=Dune", "")
	var off struct {
		Results      []any
		ProvidersOff bool
	}
	json.Unmarshal(rec.Body.Bytes(), &off)
	if rec.Code != 200 || !off.ProvidersOff || off.Results == nil || len(off.Results) != 0 || hits.Load() != 0 {
		t.Fatalf("off: %d %s (worker hit %d times)", rec.Code, rec.Body, hits.Load())
	}

	s.setProvider("openlibrary", `{"enabled":true}`)
	rec = s.do(admin, "GET", "/metadata/search?q=Dune", "")
	if rec.Code != 200 || hits.Load() != 1 || !strings.Contains(rec.Body.String(), "Open Library") || strings.Contains(rec.Body.String(), "providersOff") {
		t.Fatalf("on: %d %s (hits %d)", rec.Code, rec.Body, hits.Load())
	}
	s.setProvider("openlibrary", `{"enabled":false}`)
	s.do(admin, "GET", "/metadata/search?q=Dune", "")
	if hits.Load() != 1 {
		t.Errorf("the worker was asked after the provider was turned off")
	}
}

func keyState(r providerRow) string {
	if r.KeyConfigured == nil {
		return "unknown"
	}
	if *r.KeyConfigured {
		return "set"
	}
	return "missing"
}

func (s *catalogStack) workerSays(raw string) {
	s.t.Helper()
	s.exec(`INSERT INTO settings (key, value) VALUES ('metadata.providers.keys', $1::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, raw)
}

func TestProviders_SayWhetherTheWorkerHasTheKeyOfEachThatTakesOneAndNeverTheKey(t *testing.T) {
	s := newCatalogStack(t)
	s.workerSays(`{"google_books": false, "comicvine": true, "openlibrary": true}`)
	rows := s.providers()
	got := keyState(rows[0]) + " " + keyState(rows[1]) + " " + keyState(rows[2])
	if got != "missing unknown set" {
		t.Errorf("keys = %q: a provider with no key has none to say anything about, even if told", got)
	}
	s.workerSays(`{"google_books": "yes", "comicvine": null}`)
	rows = s.providers()
	if got := keyState(rows[0]) + " " + keyState(rows[2]); got != "unknown unknown" {
		t.Errorf("what is not a plain true or false is not said: %q", got)
	}
	s.workerSays(`"nonsense"`)
	if got := keyState(s.providers()[2]); got != "unknown" {
		t.Errorf("a report that is not a report: %q", got)
	}
}

func TestProviders_CannotTurnOnWhatNeedsAKeyThatTheWorkerSaysItDoesNotHave(t *testing.T) {
	s := newCatalogStack(t)
	s.workerSays(`{"google_books": false, "comicvine": false}`)
	if code := s.setProvider("comicvine", `{"enabled":true}`); code != 409 {
		t.Fatalf("without the key: %d, want 409", code)
	}
	if got := s.scalar(`SELECT count(*) FROM settings WHERE key = 'metadata.providers'`); got != "0" {
		t.Errorf("the refused choice was saved")
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'providers.set'`); got != "0" {
		t.Errorf("the refused choice was audited as a change")
	}
	// Google Books shares a quota with every anonymous client without a key of its own: it is turned on with one.
	if code := s.setProvider("google_books", `{"enabled":true}`); code != 409 {
		t.Errorf("Google Books without a key: %d, want 409", code)
	}
	// A provider that works without a key, within a lower limit, is not stopped by it.
	known := metaproviders.Known
	metaproviders.Known = append(append([]metaproviders.Info{}, known...), metaproviders.Info{ID: "withoutkey", Name: "Without key", Sends: []string{"title"}, Key: "optional"})
	defer func() { metaproviders.Known = known }()
	s.workerSays(`{"google_books": false, "comicvine": false, "withoutkey": false}`)
	if code := s.setProvider("withoutkey", `{"enabled":true}`); code != 204 {
		t.Errorf("a key it can do without: %d", code)
	}
	metaproviders.Known = known
	// Turning it off is never in the way.
	s.exec(`INSERT INTO settings (key, value) VALUES ('metadata.providers', '{"comicvine": true}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
	if code := s.setProvider("comicvine", `{"enabled":false}`); code != 204 {
		t.Errorf("turning off without the key: %d", code)
	}
	// With the key it can be turned on.
	s.workerSays(`{"comicvine": true, "google_books": true}`)
	for _, id := range []string{"comicvine", "google_books"} {
		if code := s.setProvider(id, `{"enabled":true}`); code != 204 {
			t.Errorf("%s with the key: %d", id, code)
		}
	}
	if got := summary(s.providers()); got != "google_books=on openlibrary=off comicvine=on anilist=off mangadex=off wikidata=off wikipedia=off" { // the setting was replaced above
		t.Errorf("after: %s", got)
	}
}

func TestProviders_BeforeTheWorkerHasSaidAnythingTurningOnWhatNeedsAKeyIsAllowed(t *testing.T) {
	s := newCatalogStack(t)
	for _, id := range []string{"comicvine", "google_books"} {
		if code := s.setProvider(id, `{"enabled":true}`); code != 204 {
			t.Errorf("%s, not known yet: %d", id, code)
		}
	}
}

func TestProviders_SayHowEachOneAnsweredTheLastTimeAndNothingElse(t *testing.T) {
	s := newCatalogStack(t)
	for _, r := range s.providers() {
		if r.Health != nil {
			t.Errorf("%s: nothing was asked yet, so nothing is known: %+v", r.ID, r.Health)
		}
	}
	s.exec(`INSERT INTO provider_health (provider, state, status, problem, checked_at, last_ok_at, empty_streak) VALUES
		('google_books', 'key', 403, 'HTTP 403: the key is not allowed to ask this', now() - interval '5 minutes', now() - interval '2 days', 0),
		('openlibrary', 'ok', 200, '', now(), now(), 9),
		('comicvine', 'ok', 200, '', now(), now(), 10),
		('anilist', 'quota', 429, 'HTTP 429', now(), NULL, 3),
		('mangadex', 'down', 0, 'the request failed', now(), NULL, 0),
		('wikidata', 'error', 400, 'HTTP 400', now(), NULL, 0),
		('nobody', 'ok', 200, '', now(), now(), 0)`)
	by := map[string]providerRow{}
	for _, r := range s.providers() {
		by[r.ID] = r
	}
	if len(by) != 7 {
		t.Errorf("a provider the library does not have is not listed: %d", len(by))
	}
	google := by["google_books"].Health
	if google == nil || google.State != "key" || google.Status != 403 || !strings.Contains(google.Problem, "not allowed") || google.LastOkAt == nil || google.Empty || google.CheckedAt == "" {
		t.Errorf("Google Books: %+v", google)
	}
	for id, want := range map[string]string{"openlibrary": "ok", "comicvine": "ok", "anilist": "quota", "mangadex": "down", "wikidata": "error"} {
		if h := by[id].Health; h == nil || h.State != want {
			t.Errorf("%s: %+v, want %s", id, h, want)
		}
	}
	// Nine searches in a row with nothing is an unknown title; ten is a provider that may not be working.
	if by["openlibrary"].Health.Empty || !by["comicvine"].Health.Empty {
		t.Errorf("the warning for many empty searches: open library %v, comicvine %v", by["openlibrary"].Health.Empty, by["comicvine"].Health.Empty)
	}
	if by["anilist"].Health.LastOkAt != nil || by["wikipedia"].Health != nil {
		t.Errorf("never answered well, or never asked: %+v / %+v", by["anilist"].Health, by["wikipedia"].Health)
	}
	// What the page is told is the kind of answer and when, never a key or a title.
	raw := s.do(admin, "GET", "/admin/metadata-providers", "").Body.String()
	for _, secret := range []string{"api_key", "apikey", "key=", "query"} {
		if strings.Contains(strings.ToLower(raw), secret) {
			t.Errorf("the list says %q: %s", secret, raw)
		}
	}
}

func (s *catalogStack) testProvider(id string) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.do(admin, "POST", "/admin/metadata-providers/"+id+"/test", "")
}

func TestProviders_TheOwnerCanAskForOneToBeTestedAndTheLastTestIsInTheList(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`DELETE FROM jobs`)
	for _, r := range s.providers() {
		if r.Test != nil || r.Testing {
			t.Errorf("%s: never tested: %+v %v", r.ID, r.Test, r.Testing)
		}
	}
	rec := s.testProvider("google_books")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("queued: %d %s", rec.Code, rec.Body)
	}
	// A provider that is off is tested too: the question is fixed and public, and the owner asked.
	if got := s.scalar(`SELECT type || ':' || (payload->>'provider') || ':' || priority || ':' || (work_id IS NULL) || ':' || (created_by IS NOT NULL) FROM jobs WHERE state = 'pending'`); got != "provider_test:google_books:5:true:true" {
		t.Errorf("the job: %q", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'providers.test' AND target_id = 'google_books'`); got != "1" {
		t.Errorf("audited %s times", got)
	}
	by := map[string]providerRow{}
	for _, r := range s.providers() {
		by[r.ID] = r
	}
	if !by["google_books"].Testing || by["openlibrary"].Testing || by["google_books"].Enabled {
		t.Errorf("only the one that was asked is being tested, and testing does not turn it on: %+v", by)
	}
	// Asking again while it waits is the same request.
	if rec := s.testProvider("google_books"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "alreadyQueued") {
		t.Errorf("twice: %d %s", rec.Code, rec.Body)
	}
	// Another provider has a test of its own.
	if rec := s.testProvider("openlibrary"); rec.Code != http.StatusAccepted {
		t.Errorf("another provider: %d", rec.Code)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'provider_test'`); got != "2" {
		t.Errorf("jobs: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'providers.test'`); got != "2" {
		t.Errorf("a request that was the same as another was audited: %s", got)
	}
	// A running test counts as one, and a finished one is not "testing" and does not stop another.
	s.exec(`UPDATE jobs SET state = 'running' WHERE payload->>'provider' = 'google_books'`)
	if rec := s.testProvider("google_books"); rec.Code != http.StatusOK {
		t.Errorf("while it runs: %d", rec.Code)
	}
	s.exec(`UPDATE jobs SET state = 'succeeded', finished_at = now() WHERE payload->>'provider' = 'google_books'`)
	if s.providers()[0].Testing {
		t.Errorf("a finished test is not a test under way")
	}
	if rec := s.testProvider("google_books"); rec.Code != http.StatusAccepted {
		t.Errorf("again, after it ended: %d", rec.Code)
	}
}

func TestProviders_ThereIsNoSuchProviderToTest(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`DELETE FROM jobs`)
	for _, id := range []string{"nobody", "GOOGLE_BOOKS", "google_books;"} {
		if rec := s.testProvider(id); rec.Code != http.StatusNotFound {
			t.Errorf("%q: %d", id, rec.Code)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM jobs`); got != "0" {
		t.Errorf("jobs for nothing: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'providers.test'`); got != "0" {
		t.Errorf("audited: %s", got)
	}
}

func TestProviders_ThePageIsToldHowTheLastTestCameOutAndNothingElse(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO settings (key, value) VALUES ('metadata.providers.tests', $1::jsonb)`, `{
		"google_books": {"ok": false, "state": "key", "status": 400, "results": 0, "ms": 310, "at": "2026-10-10T01:00:00Z"},
		"openlibrary": {"ok": true, "state": "ok", "status": 200, "results": 12, "ms": 840, "at": "2026-10-10T01:01:00Z"},
		"comicvine": {"ok": false, "state": "nokey", "status": 0, "results": 0, "ms": 0, "at": "2026-10-10T01:02:00Z"},
		"anilist": {"ok": true, "state": "ok", "at": "not a time"},
		"mangadex": {"ok": true, "at": "2026-10-10T01:03:00Z"},
		"nobody": {"ok": true, "state": "ok", "at": "2026-10-10T01:04:00Z"}}`)
	by := map[string]providerRow{}
	for _, r := range s.providers() {
		by[r.ID] = r
	}
	g, o, c := by["google_books"].Test, by["openlibrary"].Test, by["comicvine"].Test
	if g == nil || g.OK || g.State != "key" || g.Status != 400 || g.Ms != 310 || g.TestedAt != "2026-10-10T01:00:00Z" {
		t.Errorf("Google Books: %+v", g)
	}
	if o == nil || !o.OK || o.State != "ok" || o.Results != 12 || o.Ms != 840 {
		t.Errorf("Open Library: %+v", o)
	}
	if c == nil || c.OK || c.State != "nokey" {
		t.Errorf("ComicVine: %+v", c)
	}
	// What the worker could not have written is left out, and so is a provider the library does not have.
	if by["anilist"].Test != nil || by["mangadex"].Test != nil || by["wikidata"].Test != nil || len(by) != 7 {
		t.Errorf("a test with no time or no state, or for nobody: %+v %+v", by["anilist"].Test, by["mangadex"].Test)
	}
	for _, setting := range []string{`"text"`, `[1]`, `{"google_books": "x"}`, `5`} {
		s.exec(`UPDATE settings SET value = $1::jsonb WHERE key = 'metadata.providers.tests'`, setting)
		for _, r := range s.providers() {
			if r.Test != nil {
				t.Errorf("%s: a setting that is not a report says %+v", setting, r.Test)
			}
		}
	}
}
