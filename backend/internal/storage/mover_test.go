package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/storage"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

var ctx = context.Background()

type env struct {
	t     *testing.T
	db    *sql.DB
	root  string
	mover *storage.Mover
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	return &env{t: t, db: db, root: root, mover: &storage.Mover{DB: db, Root: root}}
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		e.t.Fatalf("%v\n%s", err, q)
	}
}

func (e *env) scalar(q string, args ...any) string {
	e.t.Helper()
	var s sql.NullString
	if err := e.db.QueryRow(q, args...).Scan(&s); err != nil {
		e.t.Fatalf("%v\n%s", err, q)
	}
	return s.String
}

// addWork creates a work the way ingestion does (legacy columns, projected by the
// database) and writes its file to disk at the flat ingestion path.
func (e *env) addWork(title, author, file, format, lang, publisher, date string) (workID int, fileID int64) {
	e.t.Helper()
	workID, _, fileID = testdb.AddWork(e.t, e.db, testdb.Work{
		Title: title, Path: file, Format: format, Author: author, Language: lang, Publisher: publisher, Date: date})
	e.write(file, "content of "+file)
	return workID, fileID
}

func (e *env) write(rel, content string) {
	e.t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) has(rel string) bool {
	_, err := os.Lstat(filepath.Join(e.root, filepath.FromSlash(rel)))
	return err == nil
}

func (e *env) locationOf(fileID int64) string {
	return e.scalar(`SELECT path || '|' || state || '|' || COALESCE(moving_to, '') FROM storage_locations WHERE file_id = $1`, fileID)
}

func TestMove_RelocatesTheFileAndKeepsTheDatabaseInStep(t *testing.T) {
	e := newEnv(t)
	work, file := e.addWork("Duna", "Frank Herbert", "ab12_Duna.epub", "epub", "pt", "Aleph", "2017")
	target := "Frank Herbert/Duna/Português — Aleph — 2017/Duna.epub"

	if err := e.mover.Move(ctx, file, target); err != nil {
		t.Fatal(err)
	}
	if e.has("ab12_Duna.epub") || !e.has(target) {
		t.Error("the file is not where the database says")
	}
	if got := e.locationOf(file); got != target+"|ok|" {
		t.Errorf("location = %q", got)
	}
	if got := e.scalar(`SELECT organized_at IS NOT NULL FROM files WHERE id = $1`, file); got != "true" {
		t.Error("the file was not marked organized")
	}
	if got := e.scalar(`SELECT file_path FROM work_primary WHERE work_id = $1`, work); got != target {
		t.Errorf("work_primary = %q", got)
	}

	// Moving again to the same place is harmless, and moving on cleans up the emptied folders.
	if err := e.mover.Move(ctx, file, target); err != nil {
		t.Errorf("idempotent move: %v", err)
	}
	e.mover.Move(ctx, file, "Outro/Duna.epub")
	if e.has("Frank Herbert") {
		t.Error("the folders left empty by the move were not removed")
	}
	if entries, _ := os.ReadDir(e.root); len(entries) != 1 {
		t.Errorf("root = %v, want only the new author folder", entries)
	}
}

func TestMove_NeverOverwritesAndNeverLeavesTheRoot(t *testing.T) {
	e := newEnv(t)
	_, a := e.addWork("A", "X", "a.epub", "epub", "", "", "")
	_, b := e.addWork("B", "X", "b.epub", "epub", "", "", "")

	// Taken in the database.
	e.mover.Move(ctx, a, "X/A.epub")
	if err := e.mover.Move(ctx, b, "X/A.epub"); !errors.Is(err, storage.ErrTargetTaken) {
		t.Errorf("destination used by another file: %v", err)
	}
	// Taken on disk by something the database does not know about.
	e.write("X/Stray.epub", "someone's file")
	if err := e.mover.Move(ctx, b, "X/Stray.epub"); !errors.Is(err, storage.ErrTargetTaken) {
		t.Errorf("destination present on disk: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(e.root, "X/Stray.epub")); string(got) != "someone's file" {
		t.Errorf("an existing file was overwritten: %q", got)
	}
	if got := e.locationOf(b); got != "b.epub|ok|" || !e.has("b.epub") {
		t.Errorf("a refused move changed something: %q", got)
	}
	// Unsafe destinations.
	for _, bad := range []string{"../escape.epub", "/abs.epub", "a/../../b.epub", "", ".", "x/\x00y"} {
		if err := e.mover.Move(ctx, b, bad); !errors.Is(err, storage.ErrUnsafePath) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	// A file that is not on disk, and one with no managed location.
	os.Remove(filepath.Join(e.root, "b.epub"))
	if err := e.mover.Move(ctx, b, "X/B.epub"); !errors.Is(err, storage.ErrNotMovable) {
		t.Errorf("missing source: %v", err)
	}
	if err := e.mover.Move(ctx, 999999, "X/Z.epub"); !errors.Is(err, storage.ErrNotMovable) {
		t.Errorf("unknown file: %v", err)
	}
}

func TestMove_ConcurrentMovesOfTheSameFileLeaveExactlyOneCopy(t *testing.T) {
	e := newEnv(t)
	_, f := e.addWork("A", "X", "a.epub", "epub", "", "", "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	start := make(chan struct{})
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if err := e.mover.Move(ctx, f, "X/"+strings.Repeat("n", i+1)+".epub"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()

	loc := e.locationOf(f)
	count := 0
	filepath.Walk(e.root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			count++
		}
		return nil
	})
	if count != 1 || !strings.HasSuffix(strings.Split(loc, "|")[1], "ok") {
		t.Errorf("after racing moves: %d files on disk, location %q (wins=%d)", count, loc, wins)
	}
	if !e.has(strings.Split(loc, "|")[0]) {
		t.Errorf("the database points to a missing file: %q", loc)
	}
}

func TestRecover_SettlesEveryInterruptedMove(t *testing.T) {
	e := newEnv(t)
	// Four files interrupted at different points. State 'moving' with an intended
	// destination is what a crash after step 1 leaves behind.
	_, neverLeft := e.addWork("Never left", "X", "n.epub", "epub", "", "", "")
	_, arrived := e.addWork("Arrived", "X", "r.epub", "epub", "", "", "")
	_, both := e.addWork("Both", "X", "b.epub", "epub", "", "", "")
	_, neither := e.addWork("Neither", "X", "z.epub", "epub", "", "", "")

	for _, f := range []struct {
		id int64
		to string
	}{{neverLeft, "X/Never.epub"}, {arrived, "X/Arrived.epub"}, {both, "X/Both.epub"}, {neither, "X/Neither.epub"}} {
		e.exec(`UPDATE storage_locations SET state = 'moving', moving_to = $1 WHERE file_id = $2`, f.to, f.id)
	}
	os.Rename(filepath.Join(e.root, "r.epub"), filepath.Join(e.root, "Arrived.epub.tmp")) // crash after the rename:
	os.MkdirAll(filepath.Join(e.root, "X"), 0o755)
	os.Rename(filepath.Join(e.root, "Arrived.epub.tmp"), filepath.Join(e.root, "X/Arrived.epub"))
	e.write("X/Both.epub", "copy at the destination") // b.epub is still at the origin too
	os.Remove(filepath.Join(e.root, "z.epub"))        // in neither place

	res, err := e.mover.Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reverted != 1 || res.Completed != 1 || res.Conflicts != 1 || res.Missing != 1 {
		t.Errorf("result = %+v, want one of each", res)
	}
	if got := e.locationOf(neverLeft); got != "n.epub|ok|" {
		t.Errorf("never left: %q", got)
	}
	if got := e.locationOf(arrived); got != "X/Arrived.epub|ok|" {
		t.Errorf("arrived: %q", got)
	}
	if got := e.scalar(`SELECT organized_at IS NOT NULL FROM files WHERE id = $1`, arrived); got != "true" {
		t.Error("a completed move must be marked organized")
	}
	if got := e.locationOf(both); !strings.Contains(got, "|conflict|") {
		t.Errorf("both: %q, want a conflict for an admin to resolve", got)
	}
	if !e.has("b.epub") || !e.has("X/Both.epub") {
		t.Error("recovery deleted a file it was not sure about")
	}
	if got := e.locationOf(neither); !strings.Contains(got, "|missing|") {
		t.Errorf("neither: %q", got)
	}
	// Nothing is left in the limbo state, and a second run has nothing to do.
	if n := e.scalar(`SELECT count(*) FROM storage_locations WHERE state = 'moving'`); n != "0" {
		t.Errorf("%s locations still moving", n)
	}
	if again, _ := e.mover.Recover(ctx); again != (storage.RecoverResult{}) {
		t.Errorf("second recovery = %+v", again)
	}
}

func TestPlan_UsesTheEditionAndAuthorsAndBreaksTiesByFileId(t *testing.T) {
	e := newEnv(t)
	_, duna := e.addWork("Duna", "Frank Herbert", "1_duna.epub", "epub", "pt", "Aleph", "2017-03-01")
	_, twin := e.addWork("Duna", "Frank Herbert", "2_duna.epub", "epub", "pt", "Aleph", "2017")
	_, comic := e.addWork("Watchmen", "Alan Moore", "3_w.cbz", "cbz", "en", "DC", "1987")
	e.exec(`UPDATE works SET series = 'Watchmen', series_index = 1 WHERE original_title = 'Watchmen'`)
	e.exec(`INSERT INTO person (name) VALUES ('Dave Gibbons') ON CONFLICT DO NOTHING`)
	e.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT w.id, p.id, 'author', 1 FROM works w, person p WHERE w.original_title = 'Watchmen' AND p.name = 'Dave Gibbons'`)

	plan, err := e.mover.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	to := map[int64]string{}
	for _, m := range plan.Moves {
		to[m.FileID] = m.To
	}
	if got := to[duna]; got != "Frank Herbert/Duna/Português — Aleph — 2017/Duna.epub" {
		t.Errorf("first Duna: %q", got)
	}
	// The second file with the same metadata does not overwrite; it gets an id-derived suffix.
	if got := to[twin]; !strings.HasPrefix(got, "Frank Herbert/Duna/Português — Aleph — 2017/Duna [") || !strings.HasSuffix(got, "].epub") {
		t.Errorf("second Duna: %q", got)
	}
	if got := to[comic]; got != "Watchmen/01 - Watchmen/Watchmen.cbz" {
		t.Errorf("comic: %q", got)
	}
	if plan.Hash == "" || len(plan.Skipped) != 0 {
		t.Errorf("plan = %+v", plan)
	}

	// Nothing has moved: a preview is only a preview.
	if !e.has("1_duna.epub") {
		t.Error("planning moved a file")
	}
	// The same library plans the same way every time.
	again, _ := e.mover.Plan(ctx)
	if again.Hash != plan.Hash {
		t.Error("the plan is not deterministic")
	}
}

func TestReorganize_IsExplicitAndRevalidatesThePreview(t *testing.T) {
	e := newEnv(t)
	work, file := e.addWork("Dune", "F. Herbert", "1_dune.epub", "epub", "en", "", "")
	e.mover.OrganizeWork(ctx, work) // initial layout, from the metadata known at that time
	initial := strings.Split(e.locationOf(file), "|")[0]
	if initial != "F. Herbert/Dune/Inglês/Dune.epub" {
		t.Fatalf("initial layout = %q", initial)
	}

	// Correcting the metadata does NOT move the file (DEC-037)...
	e.exec(`UPDATE works SET original_title = 'Duna' WHERE id = $1`, work)
	e.exec(`DELETE FROM work_contributors WHERE work_id = $1`, work)
	e.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'author', 0 FROM person WHERE name = 'F. Herbert'`, work)
	e.exec(`UPDATE person SET name = 'Frank Herbert' WHERE name = 'F. Herbert'`)
	if got := strings.Split(e.locationOf(file), "|")[0]; got != initial || !e.has(initial) {
		t.Fatalf("a metadata correction moved the file to %q", got)
	}
	// ...but the preview shows what a reorganization would do.
	plan, _ := e.mover.Plan(ctx)
	if len(plan.Moves) != 1 || plan.Moves[0].To != "Frank Herbert/Duna/Inglês/Duna.epub" {
		t.Fatalf("plan = %+v", plan.Moves)
	}

	// A stale preview is refused.
	if _, err := e.mover.Apply(ctx, "not-the-hash"); !errors.Is(err, storage.ErrPlanChanged) {
		t.Errorf("stale preview: %v", err)
	}
	if !e.has(initial) {
		t.Fatal("a refused reorganization moved something")
	}
	// The library changes after the preview: the preview no longer applies.
	e.addWork("Nova", "Outra", "9_n.epub", "epub", "", "", "")
	if _, err := e.mover.Apply(ctx, plan.Hash); !errors.Is(err, storage.ErrPlanChanged) {
		t.Errorf("the library changed after the preview: %v", err)
	}

	// Confirming exactly what was shown works.
	fresh, _ := e.mover.Plan(ctx)
	res, err := e.mover.Apply(ctx, fresh.Hash)
	if err != nil || res.Moved != len(fresh.Moves) || len(res.Failures) != 0 {
		t.Fatalf("apply: %+v %v", res, err)
	}
	if got := strings.Split(e.locationOf(file), "|")[0]; got != "Frank Herbert/Duna/Inglês/Duna.epub" || !e.has(got) {
		t.Errorf("after reorganizing: %q", got)
	}
	// Once organized there is nothing left to do.
	if left, _ := e.mover.Plan(ctx); len(left.Moves) != 0 {
		t.Errorf("a second plan still wants to move %d files", len(left.Moves))
	}
}
