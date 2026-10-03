package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/dictionary"
)

type dictPackage struct {
	ID          string  `json:"id"`
	State       string  `json:"state"`
	Stage       string  `json:"stage"`
	Progress    float64 `json:"progress"`
	Installable bool    `json:"installable"`
	Entries     int     `json:"entries"`
	Error       string  `json:"error"`
	URL         string  `json:"url"`
	InstalledAt *string `json:"installedAt"`
}

func dictList(t *testing.T, s *catalogStack) map[string]dictPackage {
	t.Helper()
	rec := s.do(admin, "GET", "/admin/dictionaries", "")
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Packages []dictPackage `json:"packages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]dictPackage{}
	for _, p := range body.Packages {
		out[p.ID] = p
	}
	return out
}

func TestDictionaryAdmin_ListsTheCatalogWithNothingInstalled(t *testing.T) {
	s := newCatalogStack(t)
	list := dictList(t, s)
	if len(list) != 20 {
		t.Fatalf("packages: %d", len(list))
	}
	for id, p := range list {
		if p.State != "available" || p.Progress != 0 || p.Entries != 0 {
			t.Errorf("%s: %+v", id, p)
		}
	}
	if !list["wikt-pt"].Installable || !list["wikt-fr"].Installable || !list["wikt-vi"].Installable {
		t.Fatalf("installable: pt %v, fr %v, vi %v", list["wikt-pt"].Installable, list["wikt-fr"].Installable, list["wikt-vi"].Installable)
	}
	// The address the worker downloads from is the server's business: it is not in what a browser is told.
	if strings.Contains(s.do(admin, "GET", "/admin/dictionaries", "").Body.String(), "kaikki.org/dictionary/downloads") {
		t.Fatal("the download address is shown to the browser")
	}
}

func TestDictionaryAdmin_InstallingQueuesAJobAndSaysWhereItIs(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", "")
	if rec.Code != 202 {
		t.Fatalf("install: %d %s", rec.Code, rec.Body.String())
	}
	var item dictPackage
	json.Unmarshal(rec.Body.Bytes(), &item)
	if item.State != "installing" || item.Stage != "queued" {
		t.Fatalf("item: %+v", item)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'dictionary' AND state = 'pending'`); got != "1" {
		t.Fatalf("jobs: %s", got)
	}
	if got := s.scalar(`SELECT payload->>'url' FROM jobs WHERE type = 'dictionary'`); got != "https://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz" {
		t.Fatalf("the job's address: %s", got)
	}
	if got := s.scalar(`SELECT (payload->>'package') || ' ' || (payload->>'edition') FROM jobs WHERE type = 'dictionary'`); got != "wikt-pt pt" {
		t.Fatalf("the job's package: %s", got)
	}
	// The worker is told which words to keep: the package's, not what a request says.
	if got := s.scalar(`SELECT array_to_string(ARRAY(SELECT jsonb_array_elements_text(payload->'headwords')), ',') FROM jobs WHERE type = 'dictionary'`); got != "pt,en,es,fr,de,it,ja,zh" {
		t.Fatalf("the job's headwords: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM dictionary_packages p JOIN jobs j ON j.id = p.job_id WHERE p.id = 'wikt-pt'`); got != "1" {
		t.Fatal("the package does not know its job")
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'dictionary.install'`); got != "1" {
		t.Fatalf("audit: %s", got)
	}
	if got := dictList(t, s)["wikt-pt"].State; got != "installing" {
		t.Fatalf("list: %s", got)
	}
}

func TestDictionaryAdmin_InstallingTwiceIsOneInstallation(t *testing.T) {
	s := newCatalogStack(t)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", ""); rec.Code != 202 {
		t.Fatalf("first: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", ""); rec.Code != 409 {
		t.Fatalf("second: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'dictionary'`); got != "1" {
		t.Fatalf("jobs: %s", got)
	}
}

func TestDictionaryAdmin_RefusesWhatIsNotInTheCatalogOrCannotBeInstalledYet(t *testing.T) {
	s := newCatalogStack(t)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-xx/install", ""); rec.Code != 404 {
		t.Fatalf("unknown: %d", rec.Code)
	}
	// A package this server cannot import yet is listed and cannot be installed.
	dictionary.Catalog = append(dictionary.Catalog, dictionary.Package{ID: "wikt-em-breve", Name: "Em breve", URL: "https://kaikki.org/x.gz"})
	t.Cleanup(func() { dictionary.Catalog = dictionary.Catalog[:len(dictionary.Catalog)-1] })
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-em-breve/install", ""); rec.Code != 409 {
		t.Fatalf("not installable: %d %s", rec.Code, rec.Body.String())
	}
	// An address is never taken from the request: whatever is sent is not read.
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", `{"url":"https://evil.example/x.gz"}`); rec.Code != 202 {
		t.Fatalf("with a body: %d", rec.Code)
	}
	if got := s.scalar(`SELECT payload->>'url' FROM jobs WHERE type = 'dictionary'`); strings.Contains(got, "evil") {
		t.Fatalf("a request chose the address: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'dictionary'`); got != "1" {
		t.Fatalf("jobs: %s", got)
	}
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-xx/cancel", ""); rec.Code != 404 {
		t.Fatalf("cancel unknown: %d", rec.Code)
	}
	if rec := s.do(admin, "DELETE", "/admin/dictionaries/wikt-xx", ""); rec.Code != 404 {
		t.Fatalf("remove unknown: %d", rec.Code)
	}
}

func TestDictionaryAdmin_AnInstalledOneCanBeInstalledAgainToUpdateIt(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, stage, progress, source_url, entries) VALUES ('wikt-pt', 'ready', 'done', 1, 'x', 5)`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-pt', 'pt', 'casa', 'casa', '{}')`)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", ""); rec.Code != 202 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT state || ' ' || progress::text FROM dictionary_packages WHERE id = 'wikt-pt'`); got != "installing 0" {
		t.Fatalf("package: %s", got)
	}
	// What is installed keeps working while the new one comes: nothing is thrown away before it is all in.
	if got := s.scalar(`SELECT count(*) FROM dictionary_entries WHERE package_id = 'wikt-pt'`); got != "1" {
		t.Fatalf("entries: %s", got)
	}
}

func TestDictionaryAdmin_AFailedOneCanBeTriedAgain_AndTheErrorGoes(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, stage, source_url, error) VALUES ('wikt-pt', 'failed', 'downloading', 'x', 'NetworkError')`)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", ""); rec.Code != 202 {
		t.Fatalf("again: %d %s", rec.Code, rec.Body.String())
	}
	if got := dictList(t, s)["wikt-pt"]; got.State != "installing" || got.Error != "" {
		t.Fatalf("package: %+v", got)
	}
}

func TestDictionaryAdmin_CancellingAWaitingInstallationFailsItAtOnce(t *testing.T) {
	s := newCatalogStack(t)
	s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", "")
	rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/cancel", "")
	if rec.Code != 200 {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT state FROM jobs WHERE type = 'dictionary'`); got != "cancelled" {
		t.Fatalf("job: %s", got)
	}
	got := dictList(t, s)["wikt-pt"]
	if got.State != "failed" || got.Stage != "cancelled" || got.Error == "" {
		t.Fatalf("package: %+v", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'dictionary.cancel'`); got != "1" {
		t.Fatalf("audit: %s", got)
	}
}

func TestDictionaryAdmin_CancellingARunningInstallationAsksTheWorkerToStop(t *testing.T) {
	s := newCatalogStack(t)
	s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", "")
	s.exec(`UPDATE jobs SET state = 'running', lease_owner = 'w', lease_expires_at = now() + interval '1 minute' WHERE type = 'dictionary'`)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/cancel", ""); rec.Code != 200 {
		t.Fatalf("cancel: %d", rec.Code)
	}
	if got := s.scalar(`SELECT cancel_requested::text || ' ' || state FROM jobs WHERE type = 'dictionary'`); got != "true running" {
		t.Fatalf("job: %s", got)
	}
	// The package says it stopped when the worker does, not before.
	if got := dictList(t, s)["wikt-pt"].State; got != "installing" {
		t.Fatalf("package: %s", got)
	}
}

func TestDictionaryAdmin_CancellingWhatIsNotBeingInstalledIsRefused(t *testing.T) {
	s := newCatalogStack(t)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/cancel", ""); rec.Code != 409 {
		t.Fatalf("nothing: %d", rec.Code)
	}
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-pt', 'ready', 'x')`)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/cancel", ""); rec.Code != 409 {
		t.Fatalf("ready: %d", rec.Code)
	}
}

func TestDictionaryAdmin_RemovingTakesEverythingTheDictionaryBrought(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, entries) VALUES ('wikt-pt', 'ready', 'x', 1)`)
	s.exec(`INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-pt', 'pt', 'casa', 'casa', '{}')`)
	s.exec(`INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma) VALUES ('wikt-pt', 'pt', 'corro', 'corro', 'correr')`)
	s.exec(`INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word) VALUES ('wikt-pt', 'ja', 'x', 'x', 'pt', 'correr')`)
	if rec := s.do(admin, "DELETE", "/admin/dictionaries/wikt-pt", ""); rec.Code != 204 {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	for _, table := range []string{"dictionary_packages", "dictionary_entries", "dictionary_forms", "dictionary_links"} {
		if got := s.scalar(`SELECT count(*) FROM ` + table); got != "0" {
			t.Errorf("%s: %s left", table, got)
		}
	}
	if got := dictList(t, s)["wikt-pt"].State; got != "available" {
		t.Fatalf("state: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'dictionary.remove'`); got != "1" {
		t.Fatalf("audit: %s", got)
	}
	if rec := s.do(admin, "DELETE", "/admin/dictionaries/wikt-pt", ""); rec.Code != 404 {
		t.Fatalf("remove again: %d", rec.Code)
	}
}

func TestDictionaryAdmin_AnInstallationInProgressHasToBeCancelledBeforeItIsRemoved(t *testing.T) {
	s := newCatalogStack(t)
	s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", "")
	if rec := s.do(admin, "DELETE", "/admin/dictionaries/wikt-pt", ""); rec.Code != 409 {
		t.Fatalf("remove while installing: %d", rec.Code)
	}
	if got := s.scalar(`SELECT count(*) FROM dictionary_packages`); got != "1" {
		t.Fatalf("the package was removed: %s", got)
	}
}

func TestDictionaryAdmin_ShowsHowFarAnInstallationHasGot(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO dictionary_packages (id, state, stage, progress, bytes_done, bytes_total, source_url, entries, source_date)
		VALUES ('wikt-pt', 'installing', 'importing', 0.5, 100, 200, 'x', 7, 'Mon, 28 Sep 2026 15:20:37 GMT')`)
	var body struct {
		Packages []map[string]any `json:"packages"`
	}
	json.Unmarshal(s.do(admin, "GET", "/admin/dictionaries", "").Body.Bytes(), &body)
	pt := body.Packages[0]
	if pt["id"] != "wikt-pt" || pt["stage"] != "importing" || pt["progress"] != 0.5 || pt["bytesDone"] != float64(100) || pt["bytesTotal"] != float64(200) || pt["entries"] != float64(7) || pt["sourceDate"] != "Mon, 28 Sep 2026 15:20:37 GMT" {
		t.Fatalf("package: %+v", pt)
	}
}

func TestDictionaryAdmin_SaysWhoInstalledItAndWhen(t *testing.T) {
	s := newCatalogStack(t)
	s.do(ana, "POST", "/admin/dictionaries/wikt-pt/install", "")
	if got := s.scalar(`SELECT installed_by::text FROM dictionary_packages WHERE id = 'wikt-pt'`); got != idAna {
		t.Fatalf("installed by: %s", got)
	}
	if got := dictList(t, s)["wikt-pt"]; got.InstalledAt != nil {
		t.Fatalf("installed at, while it is still being installed: %v", *got.InstalledAt)
	}
	s.exec(`UPDATE dictionary_packages SET state = 'ready', installed_at = '2026-10-03T10:00:00Z'`)
	if got := dictList(t, s)["wikt-pt"]; got.InstalledAt == nil || !strings.HasPrefix(*got.InstalledAt, "2026-10-03T10:00:00") {
		t.Fatalf("installed at: %v", got.InstalledAt)
	}
	// An update is asked for by whoever asks for it.
	s.do(admin, "POST", "/admin/dictionaries/wikt-pt/install", "")
	if got := s.scalar(`SELECT installed_by::text FROM dictionary_packages WHERE id = 'wikt-pt'`); got != idAdmin {
		t.Fatalf("installed by, on update: %s", got)
	}
}

func TestDictionaryAdmin_OnlyAnInstallationThatIsRunningCanBeCancelled(t *testing.T) {
	s := newCatalogStack(t)
	// A package that is ready, with the job that installed it long finished: cancelling is refused and touches nothing.
	s.exec(`INSERT INTO jobs (type, payload, state, finished_at) VALUES ('dictionary', '{}', 'succeeded', now())`)
	s.exec(`INSERT INTO dictionary_packages (id, state, source_url, job_id, entries) VALUES ('wikt-pt', 'ready', 'x', (SELECT max(id) FROM jobs), 9)`)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/cancel", ""); rec.Code != 409 {
		t.Fatalf("ready with a job: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT p.state || ' ' || j.cancel_requested::text FROM dictionary_packages p, jobs j WHERE j.id = p.job_id`); got != "ready false" {
		t.Fatalf("touched: %s", got)
	}
	// One that says it is being installed but has no job to stop.
	s.exec(`UPDATE dictionary_packages SET state = 'installing', job_id = NULL`)
	if rec := s.do(admin, "POST", "/admin/dictionaries/wikt-pt/cancel", ""); rec.Code != 409 {
		t.Fatalf("installing with no job: %d %s", rec.Code, rec.Body.String())
	}
}
