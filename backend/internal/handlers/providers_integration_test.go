package handlers

import (
	"encoding/json"
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
	if got := summary(rows); got != "google_books=off openlibrary=off comicvine=off" {
		t.Fatalf("by default: %s", got)
	}
	for _, r := range rows {
		if r.Name == "" || len(r.Sends) != 1 || r.Sends[0] != "title" {
			t.Errorf("%s must say what it receives: %+v", r.ID, r)
		}
	}
	if rows[0].Key != "optional" || rows[1].Key != "" || rows[2].Key != "required" {
		t.Errorf("Google Books takes a key it can do without, Open Library none, ComicVine one it cannot: %+v", rows)
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
	if got := summary(s.providers()); got != "google_books=off openlibrary=on comicvine=off" {
		t.Errorf("after turning Open Library on: %s", got)
	}
	s.setProvider("google_books", `{"enabled":true}`)
	s.setProvider("openlibrary", `{"enabled":false}`)
	if got := summary(s.providers()); got != "google_books=on openlibrary=off comicvine=off" {
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
	for _, id := range []string{"google_books", "openlibrary", "comicvine"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				s.do(admin, "PUT", "/admin/metadata-providers/"+id, `{"enabled":true}`)
			}
		}()
	}
	wg.Wait()
	if got := summary(s.providers()); got != "google_books=on openlibrary=on comicvine=on" {
		t.Errorf("after turning all on at once: %s", got)
	}
}

func TestProviders_OnlyAPlainYesTurnsOneOnAndASettingThatIsNotAChoiceIsReplaced(t *testing.T) {
	s := newCatalogStack(t)
	for _, raw := range []string{`{"openlibrary": "true", "google_books": 1, "comicvine": null}`, `"yes"`, `[true]`, `true`} {
		s.exec(`INSERT INTO settings (key, value) VALUES ('metadata.providers', $1::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, raw)
		if got := summary(s.providers()); got != "google_books=off openlibrary=off comicvine=off" {
			t.Errorf("setting %s: %s, want all off", raw, got)
		}
	}
	// Choosing over a setting that is not an object makes it one.
	s.setProvider("openlibrary", `{"enabled":true}`)
	if got := summary(s.providers()); got != "google_books=off openlibrary=on comicvine=off" {
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
	// A key it can do without does not stop Google Books.
	if code := s.setProvider("google_books", `{"enabled":true}`); code != 204 {
		t.Errorf("Google Books without a key: %d", code)
	}
	// Turning it off is never in the way.
	s.exec(`INSERT INTO settings (key, value) VALUES ('metadata.providers', '{"comicvine": true}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
	if code := s.setProvider("comicvine", `{"enabled":false}`); code != 204 {
		t.Errorf("turning off without the key: %d", code)
	}
	// With the key it can be turned on.
	s.workerSays(`{"comicvine": true}`)
	if code := s.setProvider("comicvine", `{"enabled":true}`); code != 204 {
		t.Errorf("with the key: %d", code)
	}
	if got := summary(s.providers()); got != "google_books=off openlibrary=off comicvine=on" { // the setting was replaced above
		t.Errorf("after: %s", got)
	}
}

func TestProviders_BeforeTheWorkerHasSaidAnythingTurningOnWhatNeedsAKeyIsAllowed(t *testing.T) {
	s := newCatalogStack(t)
	if code := s.setProvider("comicvine", `{"enabled":true}`); code != 204 {
		t.Errorf("not known yet: %d", code)
	}
}
