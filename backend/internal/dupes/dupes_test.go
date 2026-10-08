package dupes_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/dupes"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

var ctx = context.Background()

func TestNormalize(t *testing.T) {
	titles := map[string]string{
		"A Guerra dos Tronos!":              "a guerra dos tronos",
		"  a GUERRA   dos tronos ":          "a guerra dos tronos",
		"Códice: Ação e Reação (2ª edição)": "codice acao e reacao",
		"Duna [Edição de bolso]":            "duna",
		"":                                  "",
		"!!!":                               "",
	}
	for in, want := range titles {
		if got := dupes.NormalizeTitle(in); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
	isbns := map[string]string{
		"978-85-7657-313-5": "9788576573135", "9788576573135": "9788576573135", "85-7657-313-x": "857657313X",
		"12345": "", "isbn: 9780441172719": "9780441172719", "": "", "not a number": "",
	}
	for in, want := range isbns {
		if got := dupes.NormalizeISBN(in); got != want {
			t.Errorf("NormalizeISBN(%q) = %q, want %q", in, got, want)
		}
	}
}

type env struct {
	t  *testing.T
	db *sql.DB
}

func newEnv(t *testing.T) *env {
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return &env{t, db}
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

// work creates a work with one file; isbn goes on its edition.
func (e *env) work(title, author, isbn, format, path string) int {
	e.t.Helper()
	id, _, _ := testdb.AddWork(e.t, e.db, testdb.Work{Title: title, Path: path, Format: format, Author: author})
	if isbn != "" {
		e.exec(`UPDATE editions SET isbn = $2 WHERE work_id = $1 AND is_primary`, id, isbn)
	}
	return id
}

func (e *env) pairs() string {
	return e.scalar(`SELECT COALESCE(string_agg(work_a || '-' || work_b || ':' || reason || ':' || state, ',' ORDER BY id), '') FROM duplicate_candidates`)
}

func TestDetect_ProposesOnlyWhatIsRealEvidence(t *testing.T) {
	e := newEnv(t)
	epub := e.work("Duna", "Frank Herbert", "978-85-7657-313-5", "epub", "a.epub")
	pdf := e.work("duna!", "frank herbert", "", "pdf", "b.pdf")                 // same title and author, other format
	other := e.work("Outro livro", "Alguém", "9788576573135", "epub", "c.epub") // same ISBN, different title
	e.work("Duna", "Outro Autor", "", "epub", "d.epub")                         // same title, different author: NOT a hint
	e.work("Duna", "", "", "epub", "e.epub")                                    // same title, author unknown: NOT a hint
	e.work("Neuromancer", "William Gibson", "", "epub", "f.epub")
	retired := e.work("Duna", "Frank Herbert", "", "cbz", "g.cbz")
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retired)

	n, err := dupes.DetectAll(ctx, e.db)
	if err != nil || n != 2 {
		t.Fatalf("found %d (%v), want 2", n, err)
	}
	want := map[string]bool{
		itoa(epub) + "-" + itoa(pdf) + ":title_author:pending": true,
		itoa(epub) + "-" + itoa(other) + ":isbn:pending":       true,
	}
	for _, got := range splitPairs(e.pairs()) {
		if !want[got] {
			t.Errorf("unexpected candidate %s (all: %s)", got, e.pairs())
		}
		delete(want, got)
	}
	for missing := range want {
		t.Errorf("missing candidate %s", missing)
	}

	// Running again finds nothing new, and one work compared alone finds nothing new either.
	if n, _ := dupes.DetectAll(ctx, e.db); n != 0 {
		t.Errorf("second pass found %d", n)
	}
	if n, _ := dupes.Detect(ctx, e.db, pdf); n != 0 {
		t.Errorf("single-work pass found %d", n)
	}
	// A retired work is not compared at all.
	if n, _ := dupes.Detect(ctx, e.db, retired); n != 0 {
		t.Errorf("a retired work produced %d candidates", n)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func splitPairs(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func TestDismiss_IsRememberedAndNewWorksAreComparedOnTheirOwn(t *testing.T) {
	e := newEnv(t)
	a := e.work("Duna", "Frank Herbert", "", "epub", "a.epub")
	b := e.work("Duna", "Frank Herbert", "", "pdf", "b.pdf")
	dupes.DetectAll(ctx, e.db)
	var id int64
	e.db.QueryRow(`SELECT id FROM duplicate_candidates`).Scan(&id)

	pending := mustList(t, e)
	if len(pending) != 1 || pending[0].A.Title != "Duna" || pending[0].B.Formats[0] != "pdf" || pending[0].Reason != "title_author" {
		t.Fatalf("pending = %+v", pending)
	}
	if err := dupes.Dismiss(ctx, e.db, id, ""); err != nil {
		t.Fatal(err)
	}
	if err := dupes.Dismiss(ctx, e.db, id, ""); err != dupes.ErrNotFound {
		t.Errorf("dismissing twice: %v", err)
	}
	// It does not come back, however many times detection runs.
	dupes.DetectAll(ctx, e.db)
	dupes.Detect(ctx, e.db, a)
	dupes.Detect(ctx, e.db, b)
	if got := e.pairs(); got == "" || e.scalar(`SELECT count(*) FROM duplicate_candidates WHERE state = 'pending'`) != "0" {
		t.Errorf("a dismissed pair came back: %s", got)
	}
	// A third copy arriving later is compared against both, and only the new pairs are proposed.
	c := e.work("Duna", "Frank Herbert", "", "cbz", "c.cbz")
	n, _ := dupes.Detect(ctx, e.db, c)
	if n != 2 {
		t.Errorf("the new work matched %d others, want 2", n)
	}
}

func TestLink_MergesEverythingThatBelongsToThePeopleAndKeepsTheBytes(t *testing.T) {
	e := newEnv(t)
	epub := e.work("Duna", "Frank Herbert", "", "epub", "a.epub")
	pdf := e.work("Duna", "Frank Herbert", "", "pdf", "b.pdf")
	pdfFile := e.scalar(`SELECT file_id FROM work_primary WHERE work_id = $1`, pdf)
	e.exec(`INSERT INTO users (id, username, email, role) VALUES
		('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'ana', 'a@x', 'reader'), ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'bob', 'b@x', 'reader')`)
	e.exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', $1, 'nota no pdf')`, pdf)
	e.exec(`INSERT INTO reading_progress (user_id, file_id, position) VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', $1, 'p-200')`, pdfFile)
	e.exec(`INSERT INTO favorites (user_id, work_id) VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', $1), ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', $2), ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', $1)`, pdf, epub)
	e.exec(`INSERT INTO tags (name) VALUES ('Sci-Fi') ON CONFLICT DO NOTHING`)
	e.exec(`INSERT INTO work_tags SELECT $1, id FROM tags WHERE name = 'Sci-Fi'`, pdf)
	dupes.DetectAll(ctx, e.db)
	var id int64
	e.db.QueryRow(`SELECT id FROM duplicate_candidates`).Scan(&id)

	if err := dupes.Link(ctx, e.db, id, 99999, ""); err != dupes.ErrBadKeep {
		t.Errorf("keeping a work outside the pair: %v", err)
	}
	if err := dupes.Link(ctx, e.db, 999, epub, ""); err != dupes.ErrNotFound {
		t.Errorf("unknown candidate: %v", err)
	}
	if e.scalar(`SELECT count(*) FROM works`) != "2" {
		t.Fatal("a refused link changed something")
	}

	if err := dupes.Link(ctx, e.db, id, epub, ""); err != nil {
		t.Fatal(err)
	}
	// One work with two editions and two files; the PDF's own file keeps its id.
	// The absorbed work is retired, not deleted: what it was can be put back by separating its editions.
	if e.scalar(`SELECT count(*) FROM works WHERE retired_at IS NULL`) != "1" || e.scalar(`SELECT count(*) FROM works`) != "2" {
		t.Error("the absorbed work should be retired, and only it")
	}
	if got := e.scalar(`SELECT retired_at IS NOT NULL FROM works WHERE id = $1`, pdf); got != "true" {
		t.Error("the absorbed work is still active")
	}
	if got := e.scalar(`SELECT count(*) || '/' || count(*) FILTER (WHERE is_primary) FROM editions WHERE work_id = $1`, epub); got != "2/1" {
		t.Errorf("editions/primary = %s, want 2/1 (the kept work's own edition stays primary)", got)
	}
	if got := e.scalar(`SELECT string_agg(f.format, ',' ORDER BY f.format) FROM files f JOIN editions ed ON ed.id = f.edition_id WHERE ed.work_id = $1`, epub); got != "epub,pdf" {
		t.Errorf("files = %q", got)
	}
	if got := e.scalar(`SELECT position FROM reading_progress WHERE file_id = $1`, pdfFile); got != "p-200" {
		t.Errorf("the reading position was lost: %q", got)
	}
	if got := e.scalar(`SELECT count(*) FROM notes WHERE work_id = $1 AND quote = 'nota no pdf'`, epub); got != "1" {
		t.Error("the note did not follow")
	}
	if got := e.scalar(`SELECT count(*) FROM favorites WHERE work_id = $1`, epub); got != "2" {
		t.Errorf("favorites = %s, want ana's and bob's, once each", got)
	}
	if got := e.scalar(`SELECT count(*) FROM work_tags WHERE work_id = $1`, epub); got != "1" {
		t.Error("the tag did not follow")
	}
	if got := e.scalar(`SELECT details->>'absorbed' FROM audit_log WHERE action = 'duplicate.link'`); got != itoa(pdf) {
		t.Errorf("audit = %q", got)
	}
	if e.scalar(`SELECT count(*) FROM duplicate_candidates WHERE state = 'pending'`) != "0" {
		t.Error("the resolved pair is still listed")
	}
	if got := e.scalar(`SELECT state FROM duplicate_candidates`); got != "linked" {
		t.Errorf("the pair is remembered as %q, not as linked", got)
	}
}

func TestLink_RefusesWhenAWorkWasRetiredMeanwhile(t *testing.T) {
	e := newEnv(t)
	a := e.work("Duna", "Frank Herbert", "", "epub", "a.epub")
	e.work("Duna", "Frank Herbert", "", "pdf", "b.pdf")
	dupes.DetectAll(ctx, e.db)
	var id int64
	e.db.QueryRow(`SELECT id FROM duplicate_candidates`).Scan(&id)
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, a)
	if err := dupes.Link(ctx, e.db, id, a, ""); err != dupes.ErrRetired {
		t.Errorf("linking with a retired work: %v", err)
	}
	if e.scalar(`SELECT count(*) FROM works`) != "2" {
		t.Error("a refused link changed something")
	}
	if pending := mustList(t, e); len(pending) != 0 {
		t.Errorf("a pair with a retired work is still listed: %+v", pending)
	}
}

func TestDetect_TheSameAuthorWrittenAnotherWayIsTheSameAuthor(t *testing.T) {
	e := newEnv(t)
	a := e.work("Dune", "Frank Herbert", "", "epub", "a.epub")
	b := e.work("Dune", "Herbert, Frank", "", "pdf", "b.pdf")
	c := e.work("Dune", "Brian Herbert", "", "epub", "c.epub")
	if n, err := dupes.DetectAll(ctx, e.db); err != nil || n != 1 {
		t.Fatalf("pairs = %d, %v", n, err)
	}
	if got := e.pairs(); got != itoa(a)+"-"+itoa(b)+":title_author:pending" {
		t.Errorf("pairs = %q (another Herbert, %d, must not be proposed)", got, c)
	}
}

func TestListPending_PagesThroughThePairsInOrder(t *testing.T) {
	e := newEnv(t)
	var ids []int64
	for i := 0; i < 7; i++ {
		title := fmt.Sprintf("Livro %d", i)
		e.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`,
			e.work(title, "Autora", "", "epub", fmt.Sprintf("a%d.epub", i)), e.work(title, "Autora", "", "pdf", fmt.Sprintf("b%d.pdf", i)))
		var id int64
		e.db.QueryRow(`SELECT max(id) FROM duplicate_candidates`).Scan(&id)
		ids = append(ids, id)
	}
	// A pair with a retired work is not counted nor listed.
	gone := e.work("Fora", "Autora", "", "epub", "gone.epub")
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)
	e.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`, gone, e.work("Fora", "Autora", "", "pdf", "gone.pdf"))

	var seen []int64
	var after int64
	for turn := 1; ; turn++ {
		page, err := dupes.ListPending(ctx, e.db, after, 3)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 7 {
			t.Fatalf("total = %d, want 7 on every page", page.Total)
		}
		wantMore := turn < 3
		if page.More != wantMore {
			t.Fatalf("page %d: more = %v, want %v", turn, page.More, wantMore)
		}
		for _, c := range page.Data {
			seen = append(seen, c.ID)
			if c.A.Author != "Autora" || len(c.A.Formats) != 1 || c.A.Formats[0] != "epub" || len(c.B.Formats) != 1 || c.B.Formats[0] != "pdf" {
				t.Errorf("summary lost on the way: %+v", c)
			}
		}
		if !page.More {
			break
		}
		after = page.Data[len(page.Data)-1].ID
	}
	if fmt.Sprint(seen) != fmt.Sprint(ids) {
		t.Errorf("pages gave %v, want every pair once, in order: %v", seen, ids)
	}
}

func TestListPending_ALastPageOfExactlyTheLimitHasNoMore(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 2; i++ {
		e.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`,
			e.work(fmt.Sprintf("T%d", i), "A", "", "epub", fmt.Sprintf("a%d.epub", i)), e.work(fmt.Sprintf("T%d", i), "A", "", "pdf", fmt.Sprintf("b%d.pdf", i)))
	}
	page, err := dupes.ListPending(ctx, e.db, 0, 2)
	if err != nil || len(page.Data) != 2 || page.More || page.Total != 2 {
		t.Fatalf("%v %+v", err, page)
	}
}

func TestListPending_DescribesAWorkWithSeveralFormatsAndNoFile(t *testing.T) {
	e := newEnv(t)
	a := e.work("Duna", "Frank Herbert", "9788576573135", "epub", "a.epub")
	b := e.work("Duna", "Frank Herbert", "", "pdf", "b.pdf")
	var edition int
	e.db.QueryRow(`SELECT id FROM editions WHERE work_id = $1`, a).Scan(&edition)
	e.exec(`INSERT INTO files (edition_id, format) VALUES ($1, 'mobi'), ($1, NULL)`, edition) // a file whose format is not known is not a format
	e.exec(`INSERT INTO duplicate_candidates (work_a, work_b, reason) VALUES ($1, $2, 'title_author')`, a, b)
	page, err := dupes.ListPending(ctx, e.db, 0, 10)
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("%v %+v", err, page)
	}
	if got := page.Data[0].A; got.ISBN != "9788576573135" || fmt.Sprint(got.Formats) != "[epub mobi]" {
		t.Errorf("a = %+v", got)
	}
	if got := page.Data[0].B; got.ISBN != "" || fmt.Sprint(got.Formats) != "[pdf]" {
		t.Errorf("b = %+v", got)
	}
}

// A work goes by more names than its main title (#185): the ones kept for it, and the titles of its editions. Two works by the
// same author that share a name, whichever it is, are a hint.
func TestDetect_AnyNameOfAWorkCounts(t *testing.T) {
	e := newEnv(t)
	duna := e.work("Duna", "Frank Herbert", "", "epub", "a.epub")
	dune := e.work("Dune", "Frank Herbert", "", "epub", "b.epub") // the English title, a work of its own
	e.work("Dune", "Outro Autor", "", "epub", "c.epub")           // the same name by another author: not a hint
	e.work("Dune", "", "", "epub", "d.epub")                      // author unknown: not a hint
	e.work("Neuromancer", "William Gibson", "", "epub", "f.epub") // nothing in common

	if n, _ := dupes.DetectAll(ctx, e.db); n != 0 {
		t.Fatalf("found %d with the titles as they are, want 0", n)
	}
	// Duna goes by Dune too: kept for the work.
	e.exec(`INSERT INTO work_titles (work_id, title, language, title_key) VALUES ($1, 'Dune', 'en', $2)`, duna, dupes.NormalizeTitle("Dune"))
	n, err := dupes.DetectAll(ctx, e.db)
	if err != nil || n != 1 {
		t.Fatalf("found %d (%v) with an alternative title, want 1", n, err)
	}
	if got, want := e.pairs(), itoa(duna)+"-"+itoa(dune)+":title_author:pending"; got != want {
		t.Errorf("pairs = %s, want %s", got, want)
	}
	// The same through the title of an edition, found by the one work alone.
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`DELETE FROM work_titles`)
	e.exec(`UPDATE editions SET title = 'DUNE (edição de bolso)' WHERE work_id = $1`, duna)
	if n, _ := dupes.Detect(ctx, e.db, duna); n != 1 {
		t.Errorf("an edition title found %d, want 1", n)
	}
	// And the other way: the name is on the other work.
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`UPDATE editions SET title = 'Duna' WHERE work_id = $1`, duna)
	e.exec(`INSERT INTO work_titles (work_id, title, title_key) VALUES ($1, 'Duna', $2)`, dune, dupes.NormalizeTitle("Duna"))
	if n, _ := dupes.Detect(ctx, e.db, dune); n != 1 {
		t.Errorf("the name kept on the other work found %d, want 1", n)
	}
	// Titles with commas are read whole.
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`DELETE FROM work_titles`)
	e.exec(`UPDATE editions SET title = ''`)
	e.exec(`INSERT INTO work_titles (work_id, title, title_key) VALUES ($1, 'Duna, ou o planeta dos "vermes"', $2), ($3, 'Duna, ou o planeta dos "vermes"', $2)`,
		duna, dupes.NormalizeTitle(`Duna, ou o planeta dos "vermes"`), dune)
	if n, _ := dupes.Detect(ctx, e.db, duna); n != 1 {
		t.Errorf("a title with commas and quotes found %d, want 1", n)
	}
	// What is retired is not compared, with whatever names it has.
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, dune)
	if n, _ := dupes.DetectAll(ctx, e.db); n != 0 {
		t.Errorf("a retired work found %d", n)
	}
}

// The main title counts by itself, whatever the title of the edition says.
func TestDetect_TheMainTitleCountsWhateverTheEditionsSay(t *testing.T) {
	e := newEnv(t)
	duna := e.work("Duna", "Frank Herbert", "", "epub", "a.epub")
	dune := e.work("Dune", "Frank Herbert", "", "epub", "b.epub")
	e.exec(`UPDATE editions SET title = 'Edição A' WHERE work_id = $1`, duna)
	e.exec(`UPDATE editions SET title = 'Edição B' WHERE work_id = $1`, dune)
	e.exec(`INSERT INTO work_titles (work_id, title, title_key) VALUES ($1, 'Dune', $2)`, duna, dupes.NormalizeTitle("Dune"))
	// The alternative title of one is the main title of the other, from either side.
	if n, _ := dupes.DetectAll(ctx, e.db); n != 1 {
		t.Fatalf("found %d, want 1", n)
	}
	e.exec(`DELETE FROM duplicate_candidates`)
	e.exec(`DELETE FROM work_titles`)
	e.exec(`INSERT INTO work_titles (work_id, title, title_key) VALUES ($1, 'Duna', $2)`, dune, dupes.NormalizeTitle("Duna"))
	if n, _ := dupes.DetectAll(ctx, e.db); n != 1 {
		t.Errorf("found %d from the other side, want 1", n)
	}
}
