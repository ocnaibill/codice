package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/wiki"
)

var owner = actor{idAdmin, "owner"}

// readableWork is a work whose EPUB has its text extracted: one the generator can read.
func (s *catalogStack) readableWork(title string) int {
	s.t.Helper()
	work := s.addWork(title, "Autora", strings.ToLower(strings.ReplaceAll(title, " ", ""))+".epub", "epub")
	s.index(s.primaryFile(work), segment{text: "Texto de " + title + ".", locator: `{"type":"epub","href":"Text/c1.xhtml","progression":0}`})
	return work
}

// readableSeries is a series of readable works, by the series field of their metadata (DEC-130).
func (s *catalogStack) readableSeries(name string, n int) (int64, []int) {
	s.t.Helper()
	var ids []int
	for i := 1; i <= n; i++ {
		w := s.readableWork(fmt.Sprintf("%s %d", name, i))
		s.exec(`UPDATE works SET series = $2, series_index = $3 WHERE id = $1`, w, name, i)
		ids = append(ids, w)
	}
	return s.collectionID(name), ids
}

type libraryAnswer struct {
	Items []struct {
		Kind     string `json:"kind"`
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Books    int    `json:"books"`
		Readable int    `json:"readable"`
		Excluded bool   `json:"excluded"`
		SpinOffs int    `json:"spinOffs"`
		Wiki     *struct {
			ProjectID int64 `json:"projectId"`
		} `json:"wiki"`
		Run *struct {
			ID    int64  `json:"id"`
			State string `json:"state"`
		} `json:"run"`
	} `json:"items"`
	Total int `json:"total"`
}

func (s *catalogStack) wikiLibrary(query string) libraryAnswer {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/wiki-library"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("library: %d %s", rec.Code, rec.Body.String())
	}
	var out libraryAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

type runsAnswer struct {
	Runs []struct {
		ID    int64 `json:"id"`
		Scope struct {
			Kind string `json:"kind"`
			ID   int64  `json:"id"`
		} `json:"scope"`
		Title          string `json:"title"`
		Origin         string `json:"origin"`
		State          string `json:"state"`
		Position       int    `json:"position"`
		PauseRequested bool   `json:"pauseRequested"`
	} `json:"runs"`
}

func (s *catalogStack) wikiRuns() runsAnswer {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/wiki-runs", "")
	var out runsAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (s *catalogStack) configureWiki(baseURL string) {
	s.t.Helper()
	body := fmt.Sprintf(`{"kind":"openai","baseUrl":%q,"model":"modelo-x","key":"sk-segredo-0123456789"}`, baseURL)
	if rec := s.do(owner, "PUT", "/admin/wiki-provider", body); rec.Code != 200 {
		s.t.Fatalf("configure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestWikiProvider_TheKeyIsSealedNeverSentBackAndOnlyTheOwnerSeesTheDetails(t *testing.T) {
	s := newCatalogStack(t)
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer srv.Close()

	if rec := s.do(admin, "GET", "/admin/wiki-provider", ""); !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Errorf("before: %s", rec.Body.String())
	}
	if rec := s.do(owner, "PUT", "/admin/wiki-provider", `{"kind":"deepseek","model":"deepseek-chat"}`); rec.Code != 400 {
		t.Errorf("DeepSeek without a key: %d", rec.Code)
	}
	s.configureWiki(srv.URL + "/v1")
	ownerView := s.do(owner, "GET", "/admin/wiki-provider", "").Body.String()
	for _, want := range []string{`"keySet":true`, `"keyHint":"…6789"`, `"baseUrl":"` + srv.URL + `/v1"`, `"remote":false`, `"masterKey":true`} {
		if !strings.Contains(ownerView, want) {
			t.Errorf("the owner's view lacks %s: %s", want, ownerView)
		}
	}
	adminView := s.do(admin, "GET", "/admin/wiki-provider", "").Body.String()
	if !strings.Contains(adminView, `"model":"modelo-x"`) || strings.Contains(adminView, "baseUrl") || strings.Contains(adminView, "keyHint") {
		t.Errorf("the admin's view: %s", adminView)
	}
	// The key is nowhere in clear: not in the answers, the setting, nor the audit.
	var stored, audited string
	s.db.QueryRow(`SELECT value::text FROM settings WHERE key = 'wiki.provider'`).Scan(&stored)
	s.db.QueryRow(`SELECT COALESCE(string_agg(details::text, ' '), '') FROM audit_log WHERE action LIKE 'wiki.provider%'`).Scan(&audited)
	for name, text := range map[string]string{"owner view": ownerView, "admin view": adminView, "setting": stored, "audit": audited} {
		if strings.Contains(text, "segredo") {
			t.Errorf("the key is in the %s: %s", name, text)
		}
	}

	// Changing the model keeps the key; "Testar" uses it.
	if rec := s.do(owner, "PUT", "/admin/wiki-provider", fmt.Sprintf(`{"kind":"openai","baseUrl":%q,"model":"modelo-y"}`, srv.URL+"/v1")); !strings.Contains(rec.Body.String(), `"keySet":true`) {
		t.Errorf("the key was lost: %s", rec.Body.String())
	}
	rec := s.do(owner, "POST", "/admin/wiki-provider/test", "")
	if !strings.Contains(rec.Body.String(), `"ok":true`) || auth != "Bearer sk-segredo-0123456789" {
		t.Errorf("test: %s, auth %q", rec.Body.String(), auth)
	}

	// Without the master key the key cannot be kept, and the one kept cannot be read.
	s.wikiAdmin.SecretKey = ""
	if rec := s.do(owner, "PUT", "/admin/wiki-provider", fmt.Sprintf(`{"kind":"openai","baseUrl":%q,"model":"m","key":"sk-outra-chave-1234"}`, srv.URL)); rec.Code != 409 {
		t.Errorf("a key without a master key: %d", rec.Code)
	}
	if rec := s.do(owner, "GET", "/admin/wiki-provider", ""); !strings.Contains(rec.Body.String(), `"keyUnreadable":true`) || !strings.Contains(rec.Body.String(), `"masterKey":false`) {
		t.Errorf("without the master key: %s", rec.Body.String())
	}

	if rec := s.do(owner, "DELETE", "/admin/wiki-provider", ""); !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Errorf("delete: %s", rec.Body.String())
	}
}

func TestWikiRuns_OneLiveRunPerWorkOrSeriesAndNothingWithoutAProvider(t *testing.T) {
	s := newCatalogStack(t)
	series, books := s.readableSeries("Scythe", 2)
	alone := s.readableWork("Addie")
	comic := s.addWork("Gibi", "Autor", "gibi.cbz", "cbz")

	if rec := s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, alone)); rec.Code != 409 {
		t.Errorf("without a provider: %d %s", rec.Code, rec.Body.String())
	}
	s.configureWiki("https://modelo.exemplo.com/v1")

	// A book of a series is generated with its series; a comic has no text to read.
	if rec := s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, books[0])); rec.Code != 400 {
		t.Errorf("a book of a series alone: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, comic)); rec.Code != 400 {
		t.Errorf("a comic: %d", rec.Code)
	}
	rec := s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"series","id":%d}`, series))
	if rec.Code != 202 {
		t.Fatalf("series: %d %s", rec.Code, rec.Body.String())
	}
	if again := s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"series","id":%d}`, series)); again.Code != 200 || !strings.Contains(again.Body.String(), `"alreadyQueued":true`) {
		t.Errorf("twice: %d %s", again.Code, again.Body.String())
	}
	s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, alone))
	runs := s.wikiRuns().Runs
	if len(runs) != 2 || runs[0].Title != "Scythe" || runs[0].Position != 1 || runs[1].Title != "Addie" || runs[1].Position != 2 {
		t.Errorf("queue = %+v", runs)
	}
	// At most one running at all: the database refuses a second.
	s.exec(`UPDATE wiki_runs SET state = 'running' WHERE id = $1`, runs[0].ID)
	if _, err := s.db.Exec(`UPDATE wiki_runs SET state = 'running' WHERE id = $1`, runs[1].ID); err == nil {
		t.Error("two runs running at once")
	}
}

func TestWikiRuns_PauseResumeAndCancelAskTheRunningOneAndActAtOnceOnTheOthers(t *testing.T) {
	s := newCatalogStack(t)
	s.configureWiki("https://modelo.exemplo.com/v1")
	a, b := s.readableWork("A"), s.readableWork("B")
	s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, a))
	s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, b))
	runs := s.wikiRuns().Runs
	running, queued := runs[0].ID, runs[1].ID
	s.exec(`UPDATE wiki_runs SET state = 'running', started_at = now() WHERE id = $1`, running)

	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/wiki-runs/%d/pause", running), ""); rec.Code != 204 {
		t.Fatalf("pause running: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/wiki-runs/%d/pause", queued), ""); rec.Code != 204 {
		t.Fatalf("pause queued: %d", rec.Code)
	}
	var state1, state2 string
	var asked bool
	s.db.QueryRow(`SELECT state, pause_requested FROM wiki_runs WHERE id = $1`, running).Scan(&state1, &asked)
	s.db.QueryRow(`SELECT state FROM wiki_runs WHERE id = $1`, queued).Scan(&state2)
	if state1 != "running" || !asked || state2 != "paused" {
		t.Errorf("after pause: running %s (asked %v), queued %s", state1, asked, state2)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/wiki-runs/%d/resume", running), ""); rec.Code != 409 {
		t.Errorf("resume a running run: %d", rec.Code)
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/wiki-runs/%d/resume", queued), ""); rec.Code != 204 {
		t.Errorf("resume: %d", rec.Code)
	}
	var resumes int
	s.db.QueryRow(`SELECT state, resumes FROM wiki_runs WHERE id = $1`, queued).Scan(&state2, &resumes)
	if state2 != "queued" || resumes != 0 {
		t.Errorf("resumed a run that never started: %s, %d resumes", state2, resumes)
	}
	s.do(admin, "POST", fmt.Sprintf("/admin/wiki-runs/%d/cancel", running), "")
	s.do(admin, "POST", fmt.Sprintf("/admin/wiki-runs/%d/cancel", queued), "")
	var cancelAsked bool
	s.db.QueryRow(`SELECT state, cancel_requested FROM wiki_runs WHERE id = $1`, running).Scan(&state1, &cancelAsked)
	s.db.QueryRow(`SELECT state FROM wiki_runs WHERE id = $1`, queued).Scan(&state2)
	if state1 != "running" || !cancelAsked || state2 != "cancelled" {
		t.Errorf("after cancel: running %s (asked %v), queued %s", state1, cancelAsked, state2)
	}
	if rec := s.do(admin, "POST", "/admin/wiki-runs/999999/pause", ""); rec.Code != 404 {
		t.Errorf("an unknown run: %d", rec.Code)
	}
}

func TestWikiLibrary_GenerateForAllLeavesOutTheExceptionsAndWhatHasAWiki(t *testing.T) {
	s := newCatalogStack(t)
	s.configureWiki("https://modelo.exemplo.com/v1")
	series, _ := s.readableSeries("Scythe", 3)
	excluded := s.readableWork("Hilda")
	done, file := s.wikiBook() // has a wiki
	_ = file
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	waiting := s.readableWork("Addie")
	s.addWork("Gibi", "Autor", "gibi.cbz", "cbz")

	lib := s.wikiLibrary("")
	titles := []string{}
	for _, it := range lib.Items {
		titles = append(titles, fmt.Sprintf("%s:%s:%d/%d", it.Kind, it.Title, it.Readable, it.Books))
	}
	if strings.Join(titles, " ") != "work:Addie:1/1 work:Hilda:1/1 work:O Livro:1/1 series:Scythe:3/3" || lib.Total != 4 {
		t.Errorf("library = %v (%d)", titles, lib.Total)
	}

	if rec := s.do(admin, "PUT", "/admin/wiki-exclusions", fmt.Sprintf(`{"kind":"work","id":%d,"excluded":true}`, excluded)); rec.Code != 204 {
		t.Fatalf("exclude: %d %s", rec.Code, rec.Body.String())
	}
	rec := s.do(admin, "POST", "/admin/wiki-runs/all", "")
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), `"queued":2`) {
		t.Fatalf("all: %d %s", rec.Code, rec.Body.String())
	}
	got := map[string]string{}
	for _, it := range s.wikiLibrary("").Items {
		state := "-"
		if it.Run != nil {
			state = it.Run.State
		}
		got[it.Title] = fmt.Sprintf("%s excluded=%v wiki=%v", state, it.Excluded, it.Wiki != nil)
	}
	want := map[string]string{
		"Addie":   "queued excluded=false wiki=false",
		"Scythe":  "queued excluded=false wiki=false",
		"Hilda":   "- excluded=true wiki=false",
		"O Livro": "- excluded=false wiki=true",
	}
	for title, w := range want {
		if got[title] != w {
			t.Errorf("%s = %q, want %q", title, got[title], w)
		}
	}
	// Again: nothing new. And a direct request generates an exception all the same.
	if rec := s.do(admin, "POST", "/admin/wiki-runs/all", ""); !strings.Contains(rec.Body.String(), `"queued":0`) {
		t.Errorf("again: %s", rec.Body.String())
	}
	if rec := s.do(admin, "POST", "/admin/wiki-runs", fmt.Sprintf(`{"kind":"work","id":%d}`, excluded)); rec.Code != 202 {
		t.Errorf("an exception asked for directly: %d", rec.Code)
	}
	for _, r := range s.wikiRuns().Runs {
		if r.Title == "Addie" && r.Origin != "batch" || r.Title == "Hilda" && r.Origin != "manual" {
			t.Errorf("origin of %s = %s", r.Title, r.Origin)
		}
	}

	// Filters and search.
	if l := s.wikiLibrary("?filter=excluded"); len(l.Items) != 1 || l.Items[0].Title != "Hilda" {
		t.Errorf("excluded = %+v", l.Items)
	}
	if l := s.wikiLibrary("?filter=with"); len(l.Items) != 1 || l.Items[0].ID != int64(done) {
		t.Errorf("with = %+v", l.Items)
	}
	if l := s.wikiLibrary("?q=scy"); len(l.Items) != 1 || l.Items[0].ID != series {
		t.Errorf("search = %+v", l.Items)
	}
	if l := s.wikiLibrary("?limit=2&offset=2"); len(l.Items) != 2 || l.Total != 4 || l.Items[0].Title != "O Livro" {
		t.Errorf("page 2 = %+v", l)
	}
	_ = waiting

	// Unmarking takes it out of the exceptions.
	s.do(admin, "PUT", "/admin/wiki-exclusions", fmt.Sprintf(`{"kind":"work","id":%d,"excluded":false}`, excluded))
	if l := s.wikiLibrary("?filter=excluded"); len(l.Items) != 0 {
		t.Errorf("still excluded: %+v", l.Items)
	}
}

// handOver puts a run at the hand-over, as the worker leaves it when it is done: stage import, state running, with its bundle.
func (s *catalogStack) handOver(work int, origin, bundle string) int64 {
	s.t.Helper()
	var id int64
	if err := s.db.QueryRow(`INSERT INTO wiki_runs (work_id, origin, state, stage, requested_by, started_at)
		VALUES ($1, $2, 'running', 'import', $3, now()) RETURNING id`, work, origin, idAdmin).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	s.exec(`INSERT INTO wiki_run_bundles (run_id, bundle) VALUES ($1, $2::jsonb)`, id, bundle)
	return id
}

func TestWikiImporter_StoresWhatTheWorkerHandedOverClosesTheRunAndMarksANewBookToReview(t *testing.T) {
	s := newCatalogStack(t)
	work, _ := s.wikiBook()
	id := s.handOver(work, "manual", wikiBundle(wikiSHA))
	worked, err := wiki.ImportNext(context.Background(), s.db, "importer:test")
	if err != nil || !worked {
		t.Fatalf("ImportNext = %v, %v", worked, err)
	}
	var state, review string
	var bundles int
	s.db.QueryRow(`SELECT state FROM wiki_runs WHERE id = $1`, id).Scan(&state)
	s.db.QueryRow(`SELECT review FROM wiki_projects WHERE work_id = $1`, work).Scan(&review)
	s.db.QueryRow(`SELECT count(*) FROM wiki_run_bundles`).Scan(&bundles)
	if state != "done" || review != "none" || bundles != 0 {
		t.Errorf("after: run %s, review %s, bundles kept %d", state, review, bundles)
	}
	if again, _ := wiki.ImportNext(context.Background(), s.db, "importer:test"); again {
		t.Error("imported the same run twice")
	}

	// A book that came into a series: the wiki is updated and waits for the staff (DEC-180).
	s.handOver(work, "new_book", wikiBundle(wikiSHA))
	wiki.ImportNext(context.Background(), s.db, "importer:test")
	s.db.QueryRow(`SELECT review FROM wiki_projects WHERE work_id = $1`, work).Scan(&review)
	if review != "pending" {
		t.Errorf("review after a new book = %q", review)
	}
}

func TestWikiImporter_ABundleItCannotStoreFailsTheRunWithTheReason(t *testing.T) {
	s := newCatalogStack(t)
	work, _ := s.wikiBook()
	id := s.handOver(work, "manual", wikiBundle("cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"))
	wiki.ImportNext(context.Background(), s.db, "importer:test")
	var state, errs string
	s.db.QueryRow(`SELECT state, errors::text FROM wiki_runs WHERE id = $1`, id).Scan(&state, &errs)
	if state != "failed" || !strings.Contains(errs, "não está no acervo") {
		t.Errorf("run = %s, errors = %s", state, errs)
	}
	var projects int
	s.db.QueryRow(`SELECT count(*) FROM wiki_projects`).Scan(&projects)
	if projects != 0 {
		t.Errorf("a wiki was stored from a bundle that failed")
	}
}

func TestWikiSettings_TheCutIsTheOwnersAndThePreviewCountsWhatWouldHaveAPage(t *testing.T) {
	s := newCatalogStack(t)
	s.wikiBook()
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	get := func(query string) (min int, preview []wiki.PreviewItem) {
		rec := s.do(admin, "GET", "/admin/wiki-settings"+query, "")
		var out struct {
			MinChapters int                `json:"minChapters"`
			PreviewMin  int                `json:"previewMin"`
			Preview     []wiki.PreviewItem `json:"preview"`
		}
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out.PreviewMin, out.Preview
	}
	if min, _ := get(""); min != wiki.DefaultMinChapters {
		t.Errorf("default cut = %d", min)
	}
	// The bundle has ana-maria in chapters 1 to 3, mestre in 1 and vilao in 3.
	_, one := get("?min=1")
	_, two := get("?min=2")
	_, big := get("?min=4")
	if len(one) != 1 || one[0].Entities != 3 || one[0].Would != 3 || one[0].Pages != 2 || one[0].ByKind["person"] != 3 {
		t.Errorf("min=1: %+v", one)
	}
	if two[0].Would != 1 || big[0].Would != 0 {
		t.Errorf("min=2: %+v, min=4: %+v", two, big)
	}
	if rec := s.do(owner, "PUT", "/admin/wiki-settings", `{"minChapters":3}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"minChapters":3`) {
		t.Errorf("set: %d %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{`{"minChapters":0}`, `{"minChapters":500}`, `{"minChapters":"x"}`, `{}`} {
		if rec := s.do(owner, "PUT", "/admin/wiki-settings", body); rec.Code != 400 {
			t.Errorf("%s accepted: %d", body, rec.Code)
		}
	}
	if min, _ := get(""); min != 3 {
		t.Errorf("cut after saving = %d", min)
	}
}

func TestWikiDelete_TakesWhatTheModelAnsweredForThatWikiAlong(t *testing.T) {
	s := newCatalogStack(t)
	work, _ := s.wikiBook()
	if rec := s.importWiki(admin, wikiBundle(wikiSHA)); rec.Code != 201 {
		t.Fatalf("import: %d", rec.Code)
	}
	other := s.readableWork("Outra")
	s.exec(`INSERT INTO wiki_llm_cache (key, response, work_id) VALUES ('k1', '{}', $1), ('k2', '{}', $2)`, work, other)
	var id int64
	s.db.QueryRow(`SELECT id FROM wiki_projects WHERE work_id = $1`, work).Scan(&id)
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/admin/wiki/%d", id), ""); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	var keys string
	s.db.QueryRow(`SELECT string_agg(key, ',') FROM wiki_llm_cache`).Scan(&keys)
	if keys != "k2" {
		t.Errorf("cache after deleting = %q", keys)
	}
}

func TestSpinOff_APersonSaysItBesideTheSeriesAndTheWikiGivesItALineOfItsOwn(t *testing.T) {
	s := newCatalogStack(t)
	s.configureWiki("https://modelo.exemplo.com/v1")
	series, books := s.readableSeries("Scythe", 3)
	if s.scalar(`SELECT spin_off::text FROM works WHERE id = $1`, books[2]) != "false" {
		t.Fatal("a work starts inside the main line")
	}
	if code := s.editWork(books[2], `,"spin_off":true`); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	if s.scalar(`SELECT spin_off::text FROM works WHERE id = $1`, books[2]) != "true" {
		t.Error("the choice was not kept")
	}
	d, _ := s.detail(ana, books[2])
	if !d.Metadata.SpinOff {
		t.Errorf("the metadata of the work does not say it: %+v", d.Metadata)
	}
	// The origin is the person, like any field changed by hand; an edit that does not say it leaves it as it is.
	if got := s.scalar(`SELECT source FROM work_field_sources WHERE work_id = $1 AND field = 'spin_off'`, books[2]); got != "manual" {
		t.Errorf("origin = %q", got)
	}
	if code := s.editWork(books[2], ``); code != 200 || s.scalar(`SELECT spin_off::text FROM works WHERE id = $1`, books[2]) != "true" {
		t.Errorf("an edit that does not say it changed it: %d", code)
	}
	// The tab of the wiki says how many books of the series are outside the line.
	lib := s.wikiLibrary("")
	if len(lib.Items) != 1 || lib.Items[0].ID != series || lib.Items[0].SpinOffs != 1 || lib.Items[0].Books != 3 {
		t.Errorf("library = %+v", lib.Items)
	}
	if code := s.editWork(books[2], `,"spin_off":false`); code != 200 || s.scalar(`SELECT spin_off::text FROM works WHERE id = $1`, books[2]) != "false" {
		t.Errorf("unchecking: %d", code)
	}
}
