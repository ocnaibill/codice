package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/storage"
)

type referencedList struct {
	Data []struct {
		FileID       int64
		WorkID       int
		Title        string
		Author       string
		Format       string
		SizeBytes    *int64
		RootID       *int
		Root         string
		Path         string
		State        string
		Availability string
		Transfer     *struct {
			JobID     int64
			State     string
			LastError string
		}
	}
	Total   int
	Summary map[string]int
}

func (s *catalogStack) referenced(query string) referencedList {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/storage/referenced"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("referenced%s: %d %s", query, rec.Code, rec.Body)
	}
	var out referencedList
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (s *catalogStack) scanRoot(dir string) int {
	s.t.Helper()
	if rec := s.addRoot(admin, dir); rec.Code != 201 {
		s.t.Fatalf("add root: %d %s", rec.Code, rec.Body)
	}
	if _, err := (&storage.Scanner{DB: s.db}).Scan(context.Background(), dir, "", idAdmin); err != nil {
		s.t.Fatal(err)
	}
	return mustAtoi(s.scalar(`SELECT id FROM storage_roots WHERE path = $1`, dir))
}

func paths(l referencedList) string {
	var out []string
	for _, f := range l.Data {
		out = append(out, f.Path)
	}
	return strings.Join(out, ",")
}

func TestReferenced_ListsWhatTheLibraryOnlyPointsAtWithItsRootFormatSizeAndState(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	rootID := s.scanRoot(lib)
	s.addWork("Gerenciada", "X", "m.epub", "epub") // a managed file is not in the list

	l := s.referenced("")
	if l.Total != 3 || len(l.Data) != 3 {
		t.Fatalf("total = %d, listed = %d", l.Total, len(l.Data))
	}
	if got := paths(l); got != "Livros/Duna.epub,Notas/notas pessoais.txt,Quadrinhos/Watchmen 01.cbz" {
		t.Errorf("paths = %s: by directory and path", got)
	}
	f := l.Data[0]
	if f.Format != "epub" || f.Root != lib || f.RootID == nil || *f.RootID != rootID || f.State != "ok" || f.Availability != "available" ||
		f.SizeBytes == nil || *f.SizeBytes <= 0 || f.Title == "" || f.FileID == 0 || f.WorkID == 0 || f.Transfer != nil {
		t.Errorf("first = %+v", f)
	}
	if l.Summary["ok"] != 3 || l.Summary["missing"] != 0 || l.Summary["conflict"] != 0 {
		t.Errorf("summary = %v", l.Summary)
	}
}

func TestReferenced_FiltersByRootStateAndTextAndPages(t *testing.T) {
	s := newCatalogStack(t)
	lib := referencedLibrary(t)
	other, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(other, "Outro.txt"), []byte("um texto diferente de tudo o mais"), 0o644)
	root1 := s.scanRoot(lib)
	root2 := s.scanRoot(other)
	s.exec(`UPDATE storage_locations SET state = 'missing' WHERE path = 'Notas/notas pessoais.txt'`)
	s.exec(`UPDATE storage_locations SET state = 'conflict' WHERE path = 'Quadrinhos/Watchmen 01.cbz'`)

	if got := s.referenced("").Total; got != 4 {
		t.Fatalf("all = %d", got)
	}
	byRoot := s.referenced(fmt.Sprintf("?rootId=%d", root2))
	if byRoot.Total != 1 || paths(byRoot) != "Outro.txt" {
		t.Errorf("root 2 = %+v", byRoot)
	}
	if one := s.referenced(fmt.Sprintf("?rootId=%d", root1)); one.Total != 3 {
		t.Errorf("root 1 = %d", one.Total)
	}
	// The summary counts the directory, not the other filters: it is what the person is looking at.
	missing := s.referenced(fmt.Sprintf("?rootId=%d&state=missing", root1))
	if missing.Total != 1 || paths(missing) != "Notas/notas pessoais.txt" || missing.Data[0].State != "missing" {
		t.Errorf("missing = %+v", missing)
	}
	if missing.Summary["ok"] != 1 || missing.Summary["missing"] != 1 || missing.Summary["conflict"] != 1 {
		t.Errorf("summary with a state filter = %v, want 1/1/1 of the directory", missing.Summary)
	}
	if conflict := s.referenced("?state=conflict"); conflict.Total != 1 || paths(conflict) != "Quadrinhos/Watchmen 01.cbz" {
		t.Errorf("conflict = %+v", conflict)
	}

	if q := s.referenced("?q=" + url.QueryEscape("watchmen")); q.Total != 1 {
		t.Errorf("by path, any case: %+v", q)
	}
	if q := s.referenced("?q=" + url.QueryEscape("Quadrinhos")); q.Total != 1 || paths(q) != "Quadrinhos/Watchmen 01.cbz" {
		t.Errorf("by the directory in the path: %+v", q)
	}
	if q := s.referenced("?q=" + url.QueryEscape("%")); q.Total != 0 {
		t.Errorf("a %% is text, not a wildcard: %d", q.Total)
	}
	if q := s.referenced("?q=" + url.QueryEscape("_")); q.Total != 0 {
		t.Errorf("a _ is text too: %d", q.Total)
	}
	if q := s.referenced("?q=" + url.QueryEscape("  ")); q.Total != 4 {
		t.Errorf("blank text filters nothing: %d", q.Total)
	}

	page1 := s.referenced("?limit=3&offset=0")
	page2 := s.referenced("?limit=3&offset=3")
	if page1.Total != 4 || len(page1.Data) != 3 || len(page2.Data) != 1 || page1.Data[2].Path == page2.Data[0].Path {
		t.Errorf("pages: %d + %d of %d", len(page1.Data), len(page2.Data), page1.Total)
	}
}

func TestReferenced_AnAbsurdPageSizeFallsBackToTheDefault(t *testing.T) {
	defaultPage, maxPage := referencedPage, referencedMaxPage
	referencedPage, referencedMaxPage = 2, 3
	defer func() { referencedPage, referencedMaxPage = defaultPage, maxPage }()
	s := newCatalogStack(t)
	s.scanRoot(referencedLibrary(t))
	s.addWork("Ref", "X", "m.epub", "epub")
	for _, c := range []struct {
		q    string
		want int
	}{{"", 2}, {"?limit=3", 3}, {"?limit=4", 2}, {"?limit=0", 2}, {"?limit=-1", 2}, {"?limit=abc", 2}} {
		if got := len(s.referenced(c.q).Data); got != c.want {
			t.Errorf("%q: %d files, want %d", c.q, got, c.want)
		}
	}
}

func TestReferenced_RefusesFiltersThatAreNotFilters(t *testing.T) {
	s := newCatalogStack(t)
	for _, q := range []string{"?rootId=abc", "?rootId=0", "?rootId=-1", "?state=moving", "?state=OK", "?q=" + strings.Repeat("x", 201)} {
		if code := s.do(admin, "GET", "/admin/storage/referenced"+q, "").Code; code != 400 {
			t.Errorf("%s: %d, want 400", q, code)
		}
	}
}

func TestReferenced_LeavesOutAWorkThatWasRetired(t *testing.T) {
	s := newCatalogStack(t)
	s.scanRoot(referencedLibrary(t))
	id := mustAtoi(s.scalar(`SELECT work_id FROM work_primary WHERE file_path = 'Livros/Duna.epub'`))
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, id)
	l := s.referenced("")
	if l.Total != 2 || strings.Contains(paths(l), "Duna") || l.Summary["ok"] != 2 {
		t.Errorf("after retiring: %+v", l)
	}
}

func TestReferenced_ShowsATransferThatIsWaitingRunningOrFailedButNotOneThatEndedWell(t *testing.T) {
	s := newCatalogStack(t)
	s.scanRoot(referencedLibrary(t))
	fileOf := func(path string) (int64, int) {
		var f int64
		var w int
		s.db.QueryRow(`SELECT file_id, work_id FROM work_primary WHERE file_path = $1`, path).Scan(&f, &w)
		return f, w
	}
	add := func(path, state, errText string) int64 {
		f, w := fileOf(path)
		var id int64
		s.db.QueryRow(`INSERT INTO jobs (type, work_id, payload, state, last_error) VALUES ('transfer', $1, jsonb_build_object('file_id', $2::bigint), $3, NULLIF($4, '')) RETURNING id`,
			w, f, state, errText).Scan(&id)
		return id
	}
	pending := add("Livros/Duna.epub", "pending", "")
	add("Notas/notas pessoais.txt", "failed", "an older failure")
	failedOld := add("Notas/notas pessoais.txt", "failed", "the original file is missing: x")
	add("Notas/notas pessoais.txt", "succeeded", "")
	cancelled := add("Quadrinhos/Watchmen 01.cbz", "cancelled", "")
	_ = cancelled
	l := s.referenced("")
	by := map[string]*struct {
		id         int64
		state, err string
	}{}
	for _, f := range l.Data {
		if f.Transfer != nil {
			by[f.Path] = &struct {
				id         int64
				state, err string
			}{f.Transfer.JobID, f.Transfer.State, f.Transfer.LastError}
		}
	}
	if d := by["Livros/Duna.epub"]; d == nil || d.id != pending || d.state != "pending" {
		t.Errorf("waiting = %+v", d)
	}
	if n := by["Notas/notas pessoais.txt"]; n == nil || n.id != failedOld || n.state != "failed" || !strings.Contains(n.err, "missing") {
		t.Errorf("the last one that did not end well = %+v", n)
	}
	if w := by["Quadrinhos/Watchmen 01.cbz"]; w != nil {
		t.Errorf("a cancelled transfer is not shown: %+v", w)
	}
	// The newest of several that are alive or failed is the one shown.
	s.exec(`UPDATE jobs SET state = 'succeeded' WHERE id = $1`, pending)
	running := add("Livros/Duna.epub", "running", "")
	l = s.referenced("")
	if l.Data[0].Transfer == nil || l.Data[0].Transfer.JobID != running || l.Data[0].Transfer.State != "running" {
		t.Errorf("newest = %+v", l.Data[0].Transfer)
	}
}

type transfersAnswer struct {
	Data []struct {
		JobID      int64
		FileID     int64
		Title      string
		Format     string
		Outcome    string
		Error      string
		Origin     string
		Reason     string
		FinishedAt *string
	}
}

func (s *catalogStack) transfers(ids ...int64) (transfersAnswer, int) {
	s.t.Helper()
	var parts []string
	for _, id := range ids {
		parts = append(parts, fmt.Sprint(id))
	}
	rec := s.do(admin, "GET", "/admin/storage/transfers?ids="+strings.Join(parts, ","), "")
	var out transfersAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func TestTransfers_SayHowEachOneStandsAndWhyWhenItDidNotWork(t *testing.T) {
	s := newCatalogStack(t)
	s.scanRoot(referencedLibrary(t))
	type row struct {
		file int64
		work int
	}
	get := func(path string) row {
		var r row
		s.db.QueryRow(`SELECT file_id, work_id FROM work_primary WHERE file_path = $1`, path).Scan(&r.file, &r.work)
		return r
	}
	job := func(r row, state, errText string) int64 {
		var id int64
		s.db.QueryRow(`INSERT INTO jobs (type, work_id, payload, state, last_error, finished_at) VALUES ('transfer', $1, jsonb_build_object('file_id', $2::bigint), $3, NULLIF($4, ''),
			CASE WHEN $3::varchar IN ('succeeded', 'failed', 'cancelled') THEN now() END) RETURNING id`, r.work, r.file, state, errText).Scan(&id)
		return id
	}
	duna, notas, watch := get("Livros/Duna.epub"), get("Notas/notas pessoais.txt"), get("Quadrinhos/Watchmen 01.cbz")
	queued := job(duna, "pending", "")
	running := job(notas, "running", "")
	failed := job(watch, "failed", "the original file changed since it was catalogued")
	cancelled := job(watch, "cancelled", "")

	// Two that were moved: one whose original is gone, one whose original stays waiting.
	movedGone := s.addWork("Movida", "X", "movida.epub", "epub")
	movedKept := s.addWork("Movida com original", "X", "kept.epub", "epub")
	fileOfWork := func(w int) row {
		var r row
		s.db.QueryRow(`SELECT file_id FROM work_primary WHERE work_id = $1`, w).Scan(&r.file)
		r.work = w
		return r
	}
	gone, kept := fileOfWork(movedGone), fileOfWork(movedKept)
	doneGone := job(gone, "succeeded", "an earlier attempt failed")
	doneKept := job(kept, "succeeded", "")
	s.exec(`INSERT INTO storage_cleanups (path, sha256, file_id, reason) VALUES ('/origem/livro.epub', $1, $2, 'the original is read-only')`, strings.Repeat("a", 64), kept.file)

	// A cleanup row of a file whose transfer has not finished is not the origin of anything yet.
	s.exec(`INSERT INTO storage_cleanups (path, sha256, file_id, reason) VALUES ('/origem/duna.epub', $1, $2, 'left over')`, strings.Repeat("b", 64), duna.file)

	got, code := s.transfers(queued, running, failed, cancelled, doneGone, doneKept)
	if code != 200 || len(got.Data) != 6 {
		t.Fatalf("answer %d: %+v", code, got)
	}
	by := map[int64]int{}
	for i, d := range got.Data {
		by[d.JobID] = i
	}
	check := func(id int64, outcome string) int {
		i, ok := by[id]
		if !ok || got.Data[i].Outcome != outcome {
			t.Fatalf("job %d: %+v, want %s", id, got.Data, outcome)
		}
		return i
	}
	if d := got.Data[check(queued, "queued")]; d.Title == "" || d.FileID != duna.file || d.Format != "epub" || d.FinishedAt != nil || d.Origin != "" || d.Reason != "" {
		t.Errorf("queued = %+v", d)
	}
	check(running, "running")
	if d := got.Data[check(failed, "failed")]; d.Error != "the original file changed since it was catalogued" || d.FinishedAt == nil {
		t.Errorf("failed = %+v", d)
	}
	if d := got.Data[check(cancelled, "cancelled")]; d.Error != "" {
		t.Errorf("cancelled says no error: %+v", d)
	}
	if d := got.Data[check(doneGone, "moved")]; d.Origin != "" || d.Reason != "" || d.Error != "" {
		t.Errorf("moved = %+v", d)
	}
	if d := got.Data[check(doneKept, "moved_original_kept")]; d.Origin != "/origem/livro.epub" || d.Reason != "the original is read-only" {
		t.Errorf("moved, original kept = %+v", d)
	}
}

func TestTransfers_AskForNothingOrTooMuchOrWhatIsNotAJobAndGetNothingOfAnotherKind(t *testing.T) {
	s := newCatalogStack(t)
	for _, q := range []string{"", "?ids=", "?ids=abc", "?ids=1,x", "?ids=0", "?ids=-4"} {
		if code := s.do(admin, "GET", "/admin/storage/transfers"+q, "").Code; code != 400 {
			t.Errorf("%q: %d, want 400", q, code)
		}
	}
	many := make([]int64, 501)
	for i := range many {
		many[i] = int64(i + 1)
	}
	if _, code := s.transfers(many...); code != 400 {
		t.Errorf("501 ids: %d", code)
	}
	if _, code := s.transfers(many[:500]...); code != 200 {
		t.Errorf("500 ids: %d", code)
	}
	w := s.addWork("X", "Y", "x.epub", "epub")
	var other int64
	s.db.QueryRow(`INSERT INTO jobs (type, work_id, payload, state) VALUES ('dedupe', $1, '{}', 'pending') RETURNING id`, w).Scan(&other)
	got, code := s.transfers(other, 99999)
	if code != 200 || len(got.Data) != 0 {
		t.Errorf("another kind of job, and one that does not exist: %d %+v", code, got)
	}
}

func TestMoveToManaged_TwoFilesOfOneWorkAreNotSilentlyMergedIntoOneTransfer(t *testing.T) {
	s := newCatalogStack(t)
	s.scanRoot(referencedLibrary(t))
	var first, work int64
	s.db.QueryRow(`SELECT file_id, work_id FROM work_primary WHERE file_path = 'Livros/Duna.epub'`).Scan(&first, &work)
	// A second referenced file in the same work (another edition's file).
	var second int64
	s.db.QueryRow(`SELECT file_id FROM work_primary WHERE file_path = 'Notas/notas pessoais.txt'`).Scan(&second)
	var secondWork int64
	s.db.QueryRow(`SELECT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = $1`, second).Scan(&secondWork)
	s.exec(`UPDATE editions SET work_id = $1, is_primary = false WHERE id = (SELECT edition_id FROM files WHERE id = $2)`, work, second)

	rec := s.do(admin, "POST", "/admin/library/move-to-managed", fmt.Sprintf(`{"fileIds":[%d,%d]}`, first, second))
	if rec.Code != 202 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out struct {
		Data []struct {
			FileID int64
			JobID  int64
			Error  string
		}
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Data) != 2 || out.Data[0].JobID == 0 || out.Data[0].Error != "" {
		t.Fatalf("first = %+v", out.Data)
	}
	if out.Data[1].JobID != 0 || !strings.Contains(out.Data[1].Error, "another file of this work is being moved") {
		t.Errorf("second = %+v: it must not point at the job that moves the other file", out.Data[1])
	}
	if got := s.scalar(`SELECT count(*) || '/' || max((payload->>'file_id')::bigint) FROM jobs WHERE type = 'transfer'`); got != fmt.Sprintf("1/%d", first) {
		t.Errorf("jobs = %s", got)
	}
}
