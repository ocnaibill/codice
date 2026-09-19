package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

const (
	idAna   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	idBob   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	idAdmin = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

type actor struct{ id, role string }

var (
	ana   = actor{idAna, "reader"}
	bob   = actor{idBob, "reader"}
	admin = actor{idAdmin, "admin"}
)

// catalogStack mounts the catalog handlers over a migrated database. Identity
// comes from test headers, standing in for the authentication middleware.
type catalogStack struct {
	t       *testing.T
	db      *sql.DB
	router  http.Handler
	storage string
}

func newCatalogStack(t *testing.T) *catalogStack {
	t.Helper()
	db := migratedDB(t)
	storageDir := t.TempDir()
	t.Setenv("CODICE_STORAGE_PATH", storageDir)

	for _, u := range []actor{ana, bob, admin} {
		name := map[string]string{idAna: "ana", idBob: "bob", idAdmin: "adm"}[u.id]
		if _, err := db.Exec(`INSERT INTO users (id, username, email, role) VALUES ($1, $2, $3, $4)`,
			u.id, name, name+"@x", u.role); err != nil {
			t.Fatal(err)
		}
	}

	lib := &LibraryHandler{DB: db}
	notes := &NotesHandler{DB: db}
	fav := &FavoritesHandler{DB: db}
	stats := &StatsHandler{DB: db}
	opds := &OPDSHandler{DB: db}
	media := &MediaHandler{DB: db}
	upload := &UploadHandler{DB: db}
	jobsAdmin := &JobsHandler{DB: db, StoragePath: storageDir}
	storageAdmin := &StorageHandler{Mover: &storage.Mover{DB: db, Root: storageDir}, DB: db, StoragePath: storageDir}
	byID := &FileByIDHandler{DB: db, StorageRoot: storageDir}
	pages := &PageHandler{DB: db}

	r := chi.NewRouter()
	r.Use(identityFromHeaders)
	r.Post("/upload", upload.HandleUpload)
	r.Get("/admin/jobs", jobsAdmin.List)
	r.Get("/admin/storage/reorganize", storageAdmin.PreviewReorganize)
	r.Get("/admin/storage/roots", storageAdmin.ListRoots)
	r.Post("/admin/storage/roots", storageAdmin.AddRoot)
	r.Delete("/admin/storage/roots/{id}", storageAdmin.RemoveRoot)
	r.Post("/admin/library/scan", storageAdmin.Scan)
	r.Post("/admin/library/move-to-managed", storageAdmin.MoveToManaged)
	r.Get("/admin/storage/cleanups", storageAdmin.ListCleanups)
	r.Post("/admin/storage/cleanups/retry", storageAdmin.RetryCleanups)
	r.Get("/file/{id}", byID.ServeHTTP)
	r.Get("/works/{id}/pages", pages.GetPages)
	r.Post("/admin/storage/reorganize", storageAdmin.Reorganize)
	r.Post("/admin/jobs/{id}/rerun", jobsAdmin.Rerun)
	r.Post("/admin/jobs/{id}/cancel", jobsAdmin.Cancel)
	r.Post("/works/bulk-import", upload.HandleBulkImport)
	r.Get("/works", lib.GetWorks)
	r.Get("/works/{id}", lib.GetWorkByID)
	r.Put("/works/{id}", lib.UpdateWork)
	r.Delete("/works/{id}", lib.DeleteWork)
	r.Post("/works/{id}/restore", lib.RestoreWork)
	r.Get("/works/{id}/candidates", lib.ListCandidates)
	r.Post("/works/{id}/candidates/{candidateID}/accept", lib.AcceptCandidate)
	r.Post("/works/{id}/candidates/{candidateID}/reject", lib.RejectCandidate)
	r.Patch("/works/{id}/progress", lib.UpdateProgress)
	r.Post("/works/{id}/reading-heartbeat", lib.ReadingHeartbeat)
	r.Post("/works/{id}/favorite", fav.AddFavorite)
	r.Delete("/works/{id}/favorite", fav.RemoveFavorite)
	r.Get("/favorites", fav.GetFavorites)
	r.Post("/works/{id}/notes", notes.CreateNote)
	r.Get("/notes", notes.ListNotes)
	r.Delete("/notes/{id}", notes.DeleteNote)
	r.Get("/stats", stats.GetStats)
	r.Get("/works/{id}/text", media.ServeText)
	r.Get("/opds/recent", opds.RecentFeed)
	r.Get("/opds/search", opds.SearchFeed)

	return &catalogStack{t: t, db: db, router: r, storage: storageDir}
}

// identityFromHeaders stands in for the authentication middleware in tests.
func identityFromHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, req.Header.Get("X-Test-User"))
		ctx = context.WithValue(ctx, middleware.UserRoleKey, req.Header.Get("X-Test-Role"))
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

func (s *catalogStack) do(a actor, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func (s *catalogStack) exec(q string, args ...any) {
	s.t.Helper()
	if _, err := s.db.Exec(q, args...); err != nil {
		s.t.Fatalf("%v\n%s", err, q)
	}
}

func (s *catalogStack) scalar(q string, args ...any) string {
	s.t.Helper()
	var v sql.NullString
	if err := s.db.QueryRow(q, args...).Scan(&v); err != nil {
		s.t.Fatalf("%v\n%s", err, q)
	}
	return v.String
}

// addWork inserts a work the way the upload handler and the worker do: in the
// legacy columns. The database projects it onto editions, files and contributors.
func (s *catalogStack) addWork(title, author, file, format string) int {
	s.t.Helper()
	var personID sql.NullInt64
	if author != "" {
		s.db.QueryRow(`INSERT INTO person (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`, author).Scan(&personID)
	}
	var id int
	if err := s.db.QueryRow(`INSERT INTO works (original_title, file_path, format, author_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		title, file, format, personID).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *catalogStack) addEdition(workID int, language string) int {
	s.t.Helper()
	var id int
	if err := s.db.QueryRow(`INSERT INTO editions (work_id, title, language, is_primary) VALUES ($1, 'ed', $2, FALSE) RETURNING id`,
		workID, language).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *catalogStack) addFile(editionID int, format, path, mode string) int64 {
	s.t.Helper()
	var id int64
	if err := s.db.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, $2) RETURNING id`, editionID, format).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	s.exec(`INSERT INTO storage_locations (file_id, mode, path) VALUES ($1, $2, $3)`, id, mode, path)
	return id
}

func (s *catalogStack) primaryFile(workID int) int64 {
	s.t.Helper()
	var id int64
	if err := s.db.QueryRow(`SELECT file_id FROM work_primary WHERE work_id = $1`, workID).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

type workList struct {
	Data  []Work `json:"data"`
	Total int    `json:"total"`
}

func (s *catalogStack) list(a actor, query string) workList {
	s.t.Helper()
	rec := s.do(a, "GET", "/works"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /works%s: %d %s", query, rec.Code, rec.Body.String())
	}
	var l workList
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		s.t.Fatal(err)
	}
	return l
}

func (s *catalogStack) detail(a actor, id int) (Work, int) {
	s.t.Helper()
	rec := s.do(a, "GET", fmt.Sprintf("/works/%d", id), "")
	var w Work
	json.Unmarshal(rec.Body.Bytes(), &w)
	return w, rec.Code
}

func ids(works []Work) []int {
	out := []int{}
	for _, w := range works {
		out = append(out, w.ID)
	}
	return out
}

func sameIDs(got []int, want ...int) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}

func TestCatalog_WorkWithManyEditionsIsOneRowAndDetailListsAll(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna-pt.epub", "epub")
	other := s.addWork("Neuromancer", "William Gibson", "neuro.epub", "epub")

	en := s.addEdition(duna, "en")
	s.addFile(en, "epub", "dune-en.epub", "managed")
	s.addFile(en, "pdf", "dune-en.pdf", "managed")
	ptEdition := int(s.primaryFile(duna)) // any id, only used below through SQL
	_ = ptEdition
	s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, duna)), "pdf", "duna-pt.pdf", "managed")

	l := s.list(ana, "")
	if l.Total != 2 || len(l.Data) != 2 || !sameIDs(ids(l.Data), duna, other) {
		t.Fatalf("a work with two editions and four files must appear once: total=%d ids=%v", l.Total, ids(l.Data))
	}

	w, code := s.detail(ana, duna)
	if code != 200 || len(w.Editions) != 2 {
		t.Fatalf("detail: code=%d editions=%d", code, len(w.Editions))
	}
	if !w.Editions[0].IsPrimary || w.Editions[0].Language == "en" {
		t.Errorf("the primary edition must come first: %+v", w.Editions[0])
	}
	if w.Editions[1].Language != "en" || len(w.Editions[1].Files) != 2 {
		t.Errorf("english edition = %+v", w.Editions[1])
	}
	total := 0
	for _, e := range w.Editions {
		total += len(e.Files)
	}
	if total != 4 {
		t.Errorf("files across editions = %d, want 4", total)
	}
	for _, e := range w.Editions {
		for _, f := range e.Files {
			if f.URL == "" || f.Availability != "available" {
				t.Errorf("file without url or availability: %+v", f)
			}
		}
	}
	if w.FileURL != "/files/duna-pt.epub" || w.Author != "Frank Herbert" {
		t.Errorf("card fields: url=%q author=%q", w.FileURL, w.Author)
	}
}

func mustInt(s *catalogStack, q string, args ...any) int64 {
	s.t.Helper()
	var v int64
	if err := s.db.QueryRow(q, args...).Scan(&v); err != nil {
		s.t.Fatalf("%v\n%s", err, q)
	}
	return v
}

func TestCatalog_ListFilters(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	watch := s.addWork("Watchmen", "Alan Moore", "watchmen.cbz", "cbz")
	audio := s.addWork("Hobbit", "J.R.R. Tolkien", "hobbit.m4b", "m4b")
	s.exec(`INSERT INTO tags (name) VALUES ('Fantasy') ON CONFLICT DO NOTHING`)
	s.exec(`INSERT INTO work_tags SELECT $1, id FROM tags WHERE name = 'Fantasy'`, audio)

	s.exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, idAna, watch)
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, 'cap-2', 30)`, idAna, s.primaryFile(duna))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idAna, s.primaryFile(audio))

	cases := map[string][]int{
		"":                        {duna, watch, audio},
		"?search=dun":             {duna},
		"?search=MOORE":           {watch}, // by author, case-insensitive
		"?search=nada":            {},
		"?formatGroup=ebooks":     {duna},
		"?formatGroup=comics":     {watch},
		"?formatGroup=audio":      {audio},
		"?favorite=true":          {watch},
		"?inProgress=true":        {duna}, // a finished book is not in progress
		"?favorite=true&search=w": {watch},
	}
	for q, want := range cases {
		if got := ids(s.list(ana, q).Data); !sameIDs(got, want...) {
			t.Errorf("GET /works%s = %v, want %v", q, got, want)
		}
	}

	// Personal state belongs to the caller only.
	if got := ids(s.list(bob, "?favorite=true").Data); len(got) != 0 {
		t.Errorf("bob sees ana's favorites: %v", got)
	}
	l := s.list(ana, "")
	for _, w := range l.Data {
		switch w.ID {
		case duna:
			if w.ReadingProgress != "cap-2" || w.PercentComplete != 30 || w.Completed {
				t.Errorf("duna progress = %+v", w)
			}
		case audio:
			if !w.Completed || len(w.Tags) != 1 || w.Tags[0] != "Fantasy" {
				t.Errorf("hobbit = %+v", w)
			}
		case watch:
			if !w.IsFavorite {
				t.Error("watchmen should be a favorite")
			}
		}
	}

	// Pagination.
	page := s.list(ana, "?limit=2&page=2")
	if page.Total != 3 || len(page.Data) != 1 {
		t.Errorf("page 2 of 3 with limit 2: total=%d rows=%d", page.Total, len(page.Data))
	}
}

func TestProgress_IsPerFileNotPerWork(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	epub := s.primaryFile(duna)
	pdf := s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, duna)), "pdf", "duna.pdf", "managed")
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	otherFile := s.primaryFile(other)

	// No fileId: the primary file. With fileId: that file.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), `{"progress":"cap-3","percent":30}`); rec.Code != 200 {
		t.Fatalf("progress on the primary file: %d", rec.Code)
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), fmt.Sprintf(`{"progress":"p-200","percent":80,"fileId":%d}`, pdf)); rec.Code != 200 {
		t.Fatalf("progress on the pdf: %d", rec.Code)
	}
	if got := s.scalar(`SELECT position FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub); got != "cap-3" {
		t.Errorf("epub position = %q", got)
	}
	if got := s.scalar(`SELECT position FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, pdf); got != "p-200" {
		t.Errorf("pdf position = %q", got)
	}

	// The detail shows each file's own progress; the card shows the primary's.
	w, _ := s.detail(ana, duna)
	perFile := map[int64]float64{}
	for _, e := range w.Editions {
		for _, f := range e.Files {
			perFile[f.ID] = f.PercentComplete
		}
	}
	if perFile[epub] != 30 || perFile[pdf] != 80 {
		t.Errorf("per-file percent = %v", perFile)
	}
	if w.ReadingProgress != "cap-3" {
		t.Errorf("the card must show the primary file's position, got %q", w.ReadingProgress)
	}
	// Another user has nothing.
	if wb, _ := s.detail(bob, duna); wb.ReadingProgress != "" || wb.PercentComplete != 0 {
		t.Errorf("bob sees ana's progress: %+v", wb)
	}

	// Revision counts position writes; a partial update keeps what it did not send.
	s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), `{"progress":"cap-4"}`)
	if got := s.scalar(`SELECT revision || ':' || percent_complete FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub); got != "2:30" {
		t.Errorf("revision and percent after a position-only update = %q, want 2:30 (two writes to this file)", got)
	}

	// Completion and un-completion.
	s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), `{"progress":"fim","percent":100,"completed":true}`)
	if w, _ := s.detail(ana, duna); !w.Completed {
		t.Error("completed flag not stored")
	}
	s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), `{"progress":"cap-1","completed":false}`)
	if w, _ := s.detail(ana, duna); w.Completed {
		t.Error("completed=false must clear completion")
	}

	// Heartbeats accumulate per file.
	for i := 0; i < 2; i++ {
		s.do(ana, "POST", fmt.Sprintf("/works/%d/reading-heartbeat", duna), fmt.Sprintf(`{"seconds":30,"fileId":%d}`, pdf))
	}
	if got := s.scalar(`SELECT reading_seconds FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, pdf); got != "60" {
		t.Errorf("pdf reading seconds = %q, want 60", got)
	}
	if got := s.scalar(`SELECT reading_seconds FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, epub); got != "0" {
		t.Errorf("epub reading seconds = %q, want 0", got)
	}

	// A file of another work cannot be written through this work.
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), fmt.Sprintf(`{"progress":"x","fileId":%d}`, otherFile)); rec.Code != 404 {
		t.Errorf("foreign file id: %d, want 404", rec.Code)
	}
	if rec := s.do(ana, "PATCH", "/works/999999/progress", `{"progress":"x"}`); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
	if rec := s.do(ana, "PATCH", "/works/abc/progress", `{"progress":"x"}`); rec.Code != 404 {
		t.Errorf("malformed id: %d, want 404", rec.Code)
	}
	// A work without any file has nothing to read.
	s.exec(`INSERT INTO works (original_title) VALUES ('Sem arquivo')`)
	empty := int(mustInt(s, `SELECT id FROM works WHERE original_title = 'Sem arquivo'`))
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", empty), `{"progress":"x"}`); rec.Code != 404 {
		t.Errorf("work without a file: %d, want 404", rec.Code)
	}
}

func TestRetireRestorePurge(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	keep := s.addWork("Neuromancer", "William Gibson", "neuro.epub", "epub")

	// Real files: one managed by the server and one only referenced. The
	// referenced one sits inside the storage directory on purpose: only its mode
	// keeps it from being deleted.
	managed := filepath.Join(s.storage, "duna.epub")
	os.WriteFile(managed, []byte("managed"), 0o644)
	os.MkdirAll(filepath.Join(s.storage, "ref"), 0o755)
	external := filepath.Join(s.storage, "ref", "mine.epub")
	os.WriteFile(external, []byte("external"), 0o644)
	s.addFile(int(mustInt(s, `SELECT edition_id FROM work_primary WHERE work_id = $1`, duna)), "epub", "ref/mine.epub", "referenced")

	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":"Fear is the mind-killer"}`)
	s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), `{"progress":"cap-1","percent":10}`)
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", duna), "")
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idAna, s.primaryFile(keep))

	// --- retire
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d", duna), ""); rec.Code != 200 {
		t.Fatalf("retire: %d", rec.Code)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d", duna), ""); rec.Code != 200 {
		t.Errorf("retiring twice must be harmless: %d", rec.Code)
	}
	if n := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.retire'`); n != "1" {
		t.Errorf("retire audited %s times, want 1", n)
	}

	if got := ids(s.list(ana, "").Data); !sameIDs(got, keep) {
		t.Errorf("a retired work must be hidden from readers: %v", got)
	}
	if _, code := s.detail(ana, duna); code != 404 {
		t.Errorf("reader detail of a retired work: %d, want 404", code)
	}
	if got := ids(s.list(admin, "?retired=true").Data); !sameIDs(got, duna) {
		t.Errorf("staff must be able to list retired works: %v", got)
	}
	if got := ids(s.list(ana, "?retired=true").Data); !sameIDs(got, keep) {
		t.Errorf("a reader must not get retired works with retired=true: %v", got)
	}
	if w, code := s.detail(admin, duna); code != 200 || !w.Retired {
		t.Errorf("staff detail: code=%d retired=%v", code, w.Retired)
	}
	if rec := s.do(ana, "GET", "/favorites", ""); strings.Contains(rec.Body.String(), "Duna") {
		t.Error("favorites still list a retired work")
	}
	if rec := s.do(ana, "GET", "/opds/recent", ""); strings.Contains(rec.Body.String(), "Duna") || !strings.Contains(rec.Body.String(), "Neuromancer") {
		t.Errorf("OPDS feed after retirement: %s", rec.Body.String())
	}
	if rec := s.do(ana, "GET", "/opds/search?q=duna", ""); strings.Contains(rec.Body.String(), "Duna") {
		t.Error("OPDS search returns a retired work")
	}
	if rec := s.do(ana, "GET", fmt.Sprintf("/works/%d/text", duna), ""); rec.Code != 404 {
		t.Errorf("serving the file of a retired work: %d, want 404", rec.Code)
	}
	if rec := s.do(ana, "PATCH", fmt.Sprintf("/works/%d/progress", duna), `{"progress":"x"}`); rec.Code != 404 {
		t.Errorf("progress on a retired work: %d, want 404", rec.Code)
	}
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":"nova"}`); rec.Code != 404 {
		t.Errorf("new note on a retired work: %d, want 404", rec.Code)
	}
	// Existing notes stay, marked as coming from an unavailable source.
	var notes struct{ Data []Note }
	json.Unmarshal(s.do(ana, "GET", "/notes", "").Body.Bytes(), &notes)
	if len(notes.Data) != 1 || notes.Data[0].SourceAvailable || notes.Data[0].WorkTitle != "Duna" || notes.Data[0].WorkAuthor != "Frank Herbert" {
		t.Errorf("note of a retired work = %+v", notes.Data)
	}
	if rec := s.do(ana, "GET", "/stats", ""); !strings.Contains(rec.Body.String(), `"worksTotal":1`) {
		t.Errorf("stats count retired works: %s", rec.Body.String())
	}
	// Files and history are untouched.
	if _, err := os.Stat(managed); err != nil {
		t.Error("retiring must not delete files")
	}

	// --- restore
	if rec := s.do(admin, "POST", fmt.Sprintf("/works/%d/restore", duna), ""); rec.Code != 200 {
		t.Fatalf("restore: %d", rec.Code)
	}
	if got := ids(s.list(ana, "").Data); !sameIDs(got, duna, keep) {
		t.Errorf("restored work missing: %v", got)
	}
	if w, _ := s.detail(ana, duna); w.ReadingProgress != "cap-1" || !w.IsFavorite {
		t.Errorf("progress and favorite must survive retirement: %+v", w)
	}
	json.Unmarshal(s.do(ana, "GET", "/notes", "").Body.Bytes(), &notes)
	if !notes.Data[0].SourceAvailable {
		t.Error("the note should point to an available source again")
	}

	// --- purge needs the work to be retired first
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d?purge=true", duna), ""); rec.Code != 409 {
		t.Fatalf("purging an active work: %d, want 409", rec.Code)
	}
	if _, err := os.Stat(managed); err != nil {
		t.Fatal("a refused purge deleted a file")
	}
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", duna), "")
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d?purge=true", duna), ""); rec.Code != 200 {
		t.Fatalf("purge: %d", rec.Code)
	}

	if n := s.scalar(`SELECT COUNT(*) FROM works WHERE id = $1`, duna); n != "0" {
		t.Error("work still in the database")
	}
	if n := s.scalar(`SELECT COUNT(*) FROM editions WHERE work_id = $1`, duna); n != "0" {
		t.Error("editions still in the database")
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Error("the managed file must be deleted on purge")
	}
	if _, err := os.Stat(external); err != nil {
		t.Error("a referenced file must never be deleted, even when it lies under the storage directory")
	}

	// The note survives with its reference, and now points to nothing.
	json.Unmarshal(s.do(ana, "GET", "/notes", "").Body.Bytes(), &notes)
	if len(notes.Data) != 1 {
		t.Fatalf("the note was lost with its work: %+v", notes.Data)
	}
	n := notes.Data[0]
	if n.SourceAvailable || n.WorkID != nil || n.WorkTitle != "Duna" || n.WorkAuthor != "Frank Herbert" || n.Quote != "Fear is the mind-killer" {
		t.Errorf("note after purge = %+v", n)
	}

	// The audit trail names the actor.
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE target_type = 'work' AND target_id = $1`, fmt.Sprint(duna)); got != "work.retire,work.restore,work.retire,work.purge" {
		t.Errorf("audit trail = %q", got)
	}
	if got := s.scalar(`SELECT DISTINCT actor_username FROM audit_log WHERE action = 'work.purge'`); got != "adm" {
		t.Errorf("audit actor = %q", got)
	}

	if rec := s.do(admin, "DELETE", "/works/999999", ""); rec.Code != 404 {
		t.Errorf("retiring an unknown work: %d, want 404", rec.Code)
	}
}

func TestUpdateWork_ConfirmsFieldsAndKeepsNoteReferencesInStep(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Dune", "F. Herbert", "duna.epub", "epub")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":"da Ana"}`)
	s.do(bob, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":"do Bob"}`)

	// Correct the title only.
	rec := s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"Duna","author":"F. Herbert","tags":["Sci-Fi","Clássico"]}`)
	if rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT title_lock::text || author_lock::text FROM works WHERE id = $1`, duna); got != "truefalse" {
		t.Errorf("locks = %q, want only the changed field locked", got)
	}
	if got := s.scalar(`SELECT string_agg(DISTINCT source_title, ',') FROM notes WHERE work_id = $1`, duna); got != "Duna" {
		t.Errorf("note references after a title correction = %q", got)
	}
	// The personal text is untouched.
	if got := s.scalar(`SELECT quote FROM notes WHERE user_id = $1`, idAna); got != "da Ana" {
		t.Errorf("note text changed: %q", got)
	}
	if w, _ := s.detail(ana, duna); len(w.Tags) != 2 {
		t.Errorf("tags = %v", w.Tags)
	}

	// Correct the author: locks it, updates the reference, contributors follow.
	s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"Duna","author":"Frank Herbert","tags":[]}`)
	if got := s.scalar(`SELECT author_lock::text FROM works WHERE id = $1`, duna); got != "true" {
		t.Error("author not locked after a correction")
	}
	if got := s.scalar(`SELECT DISTINCT source_author FROM notes WHERE work_id = $1`, duna); got != "Frank Herbert" {
		t.Errorf("note author reference = %q", got)
	}
	if got := s.scalar(`SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = $1`, duna); got != "Frank Herbert" {
		t.Errorf("contributor = %q", got)
	}
	if w, _ := s.detail(ana, duna); w.Author != "Frank Herbert" {
		t.Errorf("author on the card = %q", w.Author)
	}

	// Saving without changes locks nothing new and writes no audit entry.
	before := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.update'`)
	s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"Duna","author":"Frank Herbert","tags":[]}`)
	if after := s.scalar(`SELECT COUNT(*) FROM audit_log WHERE action = 'work.update'`); after != before {
		t.Errorf("an unchanged save was audited: %s -> %s", before, after)
	}
	if before != "2" {
		t.Errorf("audit entries for the two real corrections = %s, want 2", before)
	}
	if got := s.scalar(`SELECT details->'title'->>'from' FROM audit_log WHERE action = 'work.update' ORDER BY id LIMIT 1`); got != "Dune" {
		t.Errorf("audit details = %q", got)
	}

	// A retired work keeps the last confirmed reference on its notes.
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", duna), "")
	s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"Título depois de retirada","author":"Frank Herbert","tags":[]}`)
	if got := s.scalar(`SELECT DISTINCT source_title FROM notes WHERE work_id = $1`, duna); got != "Duna" {
		t.Errorf("reference of a retired work changed: %q", got)
	}

	// Validation.
	if rec := s.do(admin, "PUT", fmt.Sprintf("/works/%d", duna), `{"title":"   ","author":"x"}`); rec.Code != 400 {
		t.Errorf("blank title: %d, want 400", rec.Code)
	}
	if rec := s.do(admin, "PUT", "/works/999999", `{"title":"x","author":"y"}`); rec.Code != 404 {
		t.Errorf("unknown work: %d, want 404", rec.Code)
	}
}

func TestNotes_ArePrivateAndSurviveTheirSource(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "", "duna.epub", "epub") // no author: absence must be stated, not invented

	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":"da Ana"}`); rec.Code != 201 {
		t.Fatalf("create: %d", rec.Code)
	}
	s.do(bob, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":"do Bob"}`)
	if rec := s.do(ana, "POST", "/works/999999/notes", `{"quote":"x"}`); rec.Code != 404 {
		t.Errorf("note on an unknown work: %d, want 404", rec.Code)
	}
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", duna), `{"quote":""}`); rec.Code != 400 {
		t.Errorf("empty note: %d, want 400", rec.Code)
	}

	var got struct{ Data []Note }
	json.Unmarshal(s.do(ana, "GET", "/notes", "").Body.Bytes(), &got)
	if len(got.Data) != 1 || got.Data[0].Quote != "da Ana" {
		t.Fatalf("ana's notes = %+v", got.Data)
	}
	if got.Data[0].WorkAuthor != "Unknown Author" || !got.Data[0].SourceAvailable {
		t.Errorf("note = %+v", got.Data[0])
	}

	// Bob cannot delete Ana's note, and Ana can.
	noteID := got.Data[0].ID
	s.do(bob, "DELETE", fmt.Sprintf("/notes/%d", noteID), "")
	if n := s.scalar(`SELECT COUNT(*) FROM notes WHERE user_id = $1`, idAna); n != "1" {
		t.Error("bob deleted ana's note")
	}
	s.do(ana, "DELETE", fmt.Sprintf("/notes/%d", noteID), "")
	if n := s.scalar(`SELECT COUNT(*) FROM notes WHERE user_id = $1`, idAna); n != "0" {
		t.Error("ana could not delete her own note")
	}
}

func TestFavoritesAndStats(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("Vol 1", "Alan Moore", "v1.cbz", "cbz")
	b := s.addWork("Vol 2", "Alan Moore", "v2.cbz", "cbz")
	c := s.addWork("Solo", "Alguém", "solo.epub", "epub")
	s.exec(`UPDATE works SET series = 'Watchmen' WHERE id IN ($1, $2)`, a, b)
	s.exec(`UPDATE editions SET cover_url = '/covers/a.jpg' WHERE work_id = $1`, a)

	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", a), "")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/favorite", c), "")
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at, reading_seconds) VALUES ($1, $2, 'fim', 100, now(), 500)`, idAna, s.primaryFile(b))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, reading_seconds) VALUES ($1, $2, 'cap-1', 20, 100)`, idAna, s.primaryFile(c))

	var fav struct {
		Data []FavoriteSeriesItem
	}
	json.Unmarshal(s.do(ana, "GET", "/favorites", "").Body.Bytes(), &fav)
	bySeries := map[string]FavoriteSeriesItem{}
	for _, f := range fav.Data {
		bySeries[f.SeriesLabel] = f
	}
	if w := bySeries["Watchmen"]; w.SeriesTotal != 2 || w.SeriesCompleted != 1 || w.Author != "Alan Moore" || w.CoverURL != "/covers/a.jpg" {
		t.Errorf("series widget = %+v", w)
	}
	if s := bySeries["Solo"]; s.SeriesTotal != 1 || s.SeriesCompleted != 0 {
		t.Errorf("a work without a series counts as a series of one: %+v", s)
	}

	var st DashboardStats
	json.Unmarshal(s.do(ana, "GET", "/stats", "").Body.Bytes(), &st)
	if st.WorksTotal != 3 || st.LibraryBreakdown.Mangas != 2 || st.LibraryBreakdown.Livros != 1 {
		t.Errorf("library stats = %+v", st)
	}
	if st.InProgressCount != 1 || st.InProgressBreakdown.Livros != 1 {
		t.Errorf("in progress = %+v", st)
	}
	if st.CompletedThisMonth != 1 || st.CompletedBreakdown.Mangas != 1 {
		t.Errorf("completed = %+v", st)
	}
	if st.TotalReadingSeconds != 600 {
		t.Errorf("reading seconds = %d, want 600", st.TotalReadingSeconds)
	}
	// 3 works, only "Vol 1" has an author and a cover... all three have authors; only one has a cover.
	if st.CatalogedPercent != 33 {
		t.Errorf("cataloged percent = %d, want 33", st.CatalogedPercent)
	}

	var other DashboardStats
	json.Unmarshal(s.do(bob, "GET", "/stats", "").Body.Bytes(), &other)
	if other.InProgressCount != 0 || other.CompletedThisMonth != 0 || other.TotalReadingSeconds != 0 {
		t.Errorf("bob sees ana's personal stats: %+v", other)
	}
}

func TestOPDSFeedsListWorksWithTheirAuthors(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.addWork("Watchmen", "Alan Moore", "w.cbz", "cbz")

	rec := s.do(ana, "GET", "/opds/recent", "")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Duna") || !strings.Contains(body, "Frank Herbert") || !strings.Contains(body, "application/epub+zip") {
		t.Errorf("recent feed: %d %s", rec.Code, body)
	}
	rec = s.do(ana, "GET", "/opds/search?q=moore", "")
	if !strings.Contains(rec.Body.String(), "Watchmen") || strings.Contains(rec.Body.String(), "Duna") {
		t.Errorf("search by author: %s", rec.Body.String())
	}
}
