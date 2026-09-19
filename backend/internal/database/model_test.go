package database_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return n
}

func str(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return s.String
}

// pgCode returns the SQLSTATE of a PostgreSQL error, or "" for anything else.
func pgCode(err error) string {
	var pe *pq.Error
	if errors.As(err, &pe) {
		return string(pe.Code)
	}
	return ""
}

func expectCode(t *testing.T, err error, code, what string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: accepted, want SQLSTATE %s", what, code)
		return
	}
	if got := pgCode(err); got != code {
		t.Errorf("%s: SQLSTATE %q (%v), want %s", what, got, err, code)
	}
}

func migrated(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

const (
	userA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	userB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func addUsers(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO users (id, username, email, role) VALUES
		($1, 'ana', 'ana@x', 'reader'), ($2, 'bob', 'bob@x', 'reader')`, userA, userB)
}

func TestExpand_CopiesLegacyDataIntoTheNewModel(t *testing.T) {
	db := testdb.Open(t)
	if err := database.MigrateTo(db, 1); err != nil {
		t.Fatal(err)
	}
	addUsers(t, db)
	mustExec(t, db, `
		INSERT INTO person (id, name) VALUES (1, 'Frank Herbert'), (2, 'Unknown Author');
		INSERT INTO works (id, original_title, file_path, format, language, publisher, publication_date, isbn, author_id) VALUES
			(1, 'Duna', 'duna.epub', 'epub', 'pt', 'Aleph', '2017', '9788576573135', 1),
			(2, 'Sem arquivo', NULL, NULL, NULL, NULL, NULL, NULL, 2),
			(3, 'Sem capa', 'sem-capa.pdf', 'pdf', 'en', NULL, NULL, NULL, 1);
		INSERT INTO editions (work_id, title, cover_url) VALUES (1, 'Duna', '/covers/duna.jpg');
		INSERT INTO user_progress (user_id, work_id, progress, percent_complete, completed_at, reading_seconds) VALUES
			('`+userA+`', 1, 'epubcfi(/6/4)', 42.5, NULL, 120),
			('`+userB+`', 1, 'epubcfi(/6/8)', 100, now(), 3600),
			('`+userA+`', 2, 'ignored', 10, NULL, 5);
		INSERT INTO notes (user_id, work_id, quote) VALUES
			('`+userA+`', 1, 'Fear is the mind-killer'),
			('`+userA+`', 2, 'nota de obra sem autor');
		SELECT setval('works_id_seq', 3);`)

	if err := database.Migrate(db); err != nil {
		t.Fatalf("expand migration: %v", err)
	}

	// Editions: every work has exactly one primary edition, with the legacy
	// bibliographic fields and the existing cover.
	if n := count(t, db, `SELECT COUNT(*) FROM editions WHERE is_primary`); n != 3 {
		t.Errorf("primary editions = %d, want 3 (one per work)", n)
	}
	if got := str(t, db, `SELECT cover_url FROM editions WHERE work_id = 1`); got != "/covers/duna.jpg" {
		t.Errorf("cover lost: %q", got)
	}
	if got := str(t, db, `SELECT publisher FROM editions WHERE work_id = 1`); got != "Aleph" {
		t.Errorf("publisher = %q", got)
	}
	if got := str(t, db, `SELECT language FROM editions WHERE work_id = 3`); got != "en" {
		t.Errorf("language of a work that had no edition row = %q", got)
	}

	// Files and locations: one per work that had a file_path.
	if n := count(t, db, `SELECT COUNT(*) FROM files`); n != 2 {
		t.Errorf("files = %d, want 2", n)
	}
	if got := str(t, db, `SELECT l.path FROM work_primary wp JOIN storage_locations l ON l.file_id = wp.file_id WHERE wp.work_id = 1`); got != "duna.epub" {
		t.Errorf("location path = %q", got)
	}
	if got := str(t, db, `SELECT file_format FROM work_primary WHERE work_id = 1`); got != "epub" {
		t.Errorf("format = %q", got)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM work_primary WHERE work_id = 2 AND file_id IS NULL`); n != 1 {
		t.Error("a work without a file must have a primary edition but no file")
	}

	// Contributors.
	if got := str(t, db, `SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = 1 AND c.role = 'author'`); got != "Frank Herbert" {
		t.Errorf("author = %q", got)
	}

	// Progress moves to the file; the legacy table is untouched.
	if n := count(t, db, `SELECT COUNT(*) FROM reading_progress`); n != 2 {
		t.Errorf("reading_progress rows = %d, want 2 (progress on a work without a file has nothing to point at)", n)
	}
	if got := str(t, db, `SELECT rp.position FROM reading_progress rp JOIN work_primary wp ON wp.file_id = rp.file_id WHERE rp.user_id = $1 AND wp.work_id = 1`, userA); got != "epubcfi(/6/4)" {
		t.Errorf("position = %q", got)
	}
	if n := count(t, db, `SELECT reading_seconds FROM reading_progress WHERE user_id = $1`, userB); n != 3600 {
		t.Errorf("reading seconds = %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM reading_progress WHERE user_id = $1 AND completed_at IS NOT NULL`, userB); n != 1 {
		t.Error("completion lost")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM user_progress`); n != 3 {
		t.Errorf("legacy user_progress was modified: %d rows", n)
	}

	// Notes carry their own bibliographic reference.
	if got := str(t, db, `SELECT source_title FROM notes WHERE quote LIKE 'Fear%'`); got != "Duna" {
		t.Errorf("source_title = %q", got)
	}
	if got := str(t, db, `SELECT source_author FROM notes WHERE quote LIKE 'Fear%'`); got != "Frank Herbert" {
		t.Errorf("source_author = %q", got)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM notes WHERE quote LIKE 'nota de obra%' AND source_author IS NULL`); n != 1 {
		t.Error("an unknown author must be stored as absent, not invented")
	}
}

func TestExpand_LegacyWritersStillFeedTheNewModel(t *testing.T) {
	db := migrated(t)
	mustExec(t, db, `INSERT INTO person (id, name) VALUES (1, 'Frank Herbert'), (2, 'Ursula Le Guin')`)

	// The upload handler inserts a bare work with its file.
	mustExec(t, db, `INSERT INTO works (id, original_title, file_path) VALUES (10, 'upload.epub', '1_upload.epub')`)
	if n := count(t, db, `SELECT COUNT(*) FROM editions WHERE work_id = 10 AND is_primary`); n != 1 {
		t.Fatalf("primary edition not created: %d", n)
	}
	if got := str(t, db, `SELECT file_path FROM work_primary WHERE work_id = 10`); got != "1_upload.epub" {
		t.Errorf("file location = %q", got)
	}

	// The worker fills metadata in the legacy columns.
	mustExec(t, db, `UPDATE works SET format='epub', language='pt', publisher='Aleph', isbn='9788576573135', publication_date='2017', author_id=1 WHERE id = 10`)
	if got := str(t, db, `SELECT publisher FROM editions WHERE work_id = 10 AND is_primary`); got != "Aleph" {
		t.Errorf("edition publisher = %q", got)
	}
	if got := str(t, db, `SELECT file_format FROM work_primary WHERE work_id = 10`); got != "epub" {
		t.Errorf("file format = %q", got)
	}
	if got := str(t, db, `SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = 10`); got != "Frank Herbert" {
		t.Errorf("contributor = %q", got)
	}

	// The worker stores the cover with the upsert it has always used, now
	// naming the primary-edition index as the conflict target.
	mustExec(t, db, `INSERT INTO editions (work_id, title, cover_url) VALUES (10, 'Duna', '/covers/duna.jpg')
		ON CONFLICT (work_id) WHERE is_primary DO UPDATE SET cover_url = EXCLUDED.cover_url`)
	if n := count(t, db, `SELECT COUNT(*) FROM editions WHERE work_id = 10`); n != 1 {
		t.Errorf("cover upsert created a second edition: %d", n)
	}
	if got := str(t, db, `SELECT cover_url FROM work_primary WHERE work_id = 10`); got != "/covers/duna.jpg" {
		t.Errorf("cover = %q", got)
	}

	// Changing the author replaces the primary contributor; clearing it removes it.
	mustExec(t, db, `UPDATE works SET author_id = 2 WHERE id = 10`)
	if got := str(t, db, `SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = 10`); got != "Ursula Le Guin" {
		t.Errorf("contributor after change = %q", got)
	}
	mustExec(t, db, `UPDATE works SET author_id = NULL WHERE id = 10`)
	if n := count(t, db, `SELECT COUNT(*) FROM work_contributors WHERE work_id = 10`); n != 0 {
		t.Errorf("contributor not removed: %d", n)
	}

	// Moving the file updates its location, not its identity.
	fileID := count(t, db, `SELECT file_id FROM work_primary WHERE work_id = 10`)
	mustExec(t, db, `UPDATE works SET file_path = 'moved/upload.epub' WHERE id = 10`)
	if got := str(t, db, `SELECT file_path FROM work_primary WHERE work_id = 10`); got != "moved/upload.epub" {
		t.Errorf("location after move = %q", got)
	}
	if again := count(t, db, `SELECT file_id FROM work_primary WHERE work_id = 10`); again != fileID {
		t.Errorf("file identity changed when the path did: %d -> %d", fileID, again)
	}
}

func TestModel_WorkWithTwoEditionsAndThreeFiles(t *testing.T) {
	db := migrated(t)
	mustExec(t, db, `INSERT INTO works (id, original_title, file_path, format) VALUES (1, 'Duna', 'duna-pt.epub', 'epub')`)
	primary := count(t, db, `SELECT id FROM editions WHERE work_id = 1`)

	// A second edition, in English, with two formats; the Portuguese edition gets a PDF too.
	var en int
	if err := db.QueryRow(`INSERT INTO editions (work_id, title, language, publisher, is_primary)
		VALUES (1, 'Dune', 'en', 'Ace', FALSE) RETURNING id`).Scan(&en); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO files (edition_id, format, sha256) VALUES
		($1, 'epub', repeat('a', 64)), ($1, 'pdf', repeat('b', 64)), ($2, 'pdf', repeat('c', 64))`, en, primary)

	if n := count(t, db, `SELECT COUNT(*) FROM files f JOIN editions e ON e.id = f.edition_id WHERE e.work_id = 1`); n != 4 {
		// three new files plus the one created for the legacy file_path
		t.Errorf("files of the work = %d, want 4", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM editions WHERE work_id = 1`); n != 2 {
		t.Errorf("editions = %d, want 2", n)
	}
	// Code that thinks in one file per work still sees exactly one row.
	if n := count(t, db, `SELECT COUNT(*) FROM work_primary WHERE work_id = 1`); n != 1 {
		t.Errorf("work_primary rows = %d, want 1 (the work must not be duplicated)", n)
	}
	if got := str(t, db, `SELECT file_path FROM work_primary WHERE work_id = 1`); got != "duna-pt.epub" {
		t.Errorf("primary file = %q", got)
	}
}

func TestModel_Invariants(t *testing.T) {
	db := migrated(t)
	addUsers(t, db)
	mustExec(t, db, `INSERT INTO person (id, name) VALUES (1, 'Frank Herbert')`)
	mustExec(t, db, `INSERT INTO works (id, original_title, file_path) VALUES (1, 'Duna', 'duna.epub'), (2, 'Outro', 'outro.epub')`)
	primary := count(t, db, `SELECT edition_id FROM work_primary WHERE work_id = 1`)

	t.Run("only one primary edition per work", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO editions (work_id, title) VALUES (1, 'segunda')`) // is_primary defaults to true
		expectCode(t, err, "23505", "second primary edition")
	})

	t.Run("an edition needs a work", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO editions (work_id, title, is_primary) VALUES (NULL, 'órfã', FALSE)`)
		expectCode(t, err, "23514", "edition without a work")
	})

	t.Run("identical bytes are one file", func(t *testing.T) {
		mustExec(t, db, `INSERT INTO files (edition_id, format, sha256) VALUES ($1, 'pdf', repeat('d', 64))`, primary)
		_, err := db.Exec(`INSERT INTO files (edition_id, format, sha256) VALUES ($1, 'pdf', repeat('d', 64))`, primary)
		expectCode(t, err, "23505", "duplicate sha256")
		// Files whose hash is not known yet do not collide with each other.
		mustExec(t, db, `INSERT INTO files (edition_id, format) VALUES ($1, 'cbz'), ($1, 'cbz')`, primary)
	})

	t.Run("a location is unique", func(t *testing.T) {
		fid := count(t, db, `SELECT file_id FROM work_primary WHERE work_id = 2`)
		_, err := db.Exec(`INSERT INTO storage_locations (file_id, mode, path) VALUES ($1, 'managed', 'duna.epub')`, fid)
		expectCode(t, err, "23505", "same managed path twice")
	})

	t.Run("contributor roles", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO work_contributors (work_id, person_id, role) VALUES (1, 1, 'ghostwriter')`)
		expectCode(t, err, "23514", "unknown role")
		mustExec(t, db, `INSERT INTO work_contributors (work_id, person_id, role, position) VALUES (1, 1, 'translator', 0)`)
		_, err = db.Exec(`INSERT INTO work_contributors (work_id, person_id, role) VALUES (1, 1, 'translator')`)
		expectCode(t, err, "23505", "same person, same role twice")
	})

	t.Run("progress is per user and per file", func(t *testing.T) {
		pt := count(t, db, `SELECT file_id FROM work_primary WHERE work_id = 1`)
		var en int
		db.QueryRow(`INSERT INTO editions (work_id, title, is_primary) VALUES (1, 'Dune', FALSE) RETURNING id`).Scan(&en)
		var pdf int
		db.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, 'pdf') RETURNING id`, en).Scan(&pdf)

		mustExec(t, db, `INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES
			($1, $2, 'cap-3', 30), ($1, $3, 'p-200', 80), ($4, $2, 'cap-9', 90)`, userA, pt, pdf, userB)
		if got := str(t, db, `SELECT position FROM reading_progress WHERE user_id = $1 AND file_id = $2`, userA, pt); got != "cap-3" {
			t.Errorf("position in the first file = %q", got)
		}
		if got := str(t, db, `SELECT position FROM reading_progress WHERE user_id = $1 AND file_id = $2`, userA, pdf); got != "p-200" {
			t.Errorf("position in the second file = %q", got)
		}
		_, err := db.Exec(`INSERT INTO reading_progress (user_id, file_id) VALUES ($1, $2)`, userA, pt)
		expectCode(t, err, "23505", "two rows for the same user and file")
		_, err = db.Exec(`INSERT INTO reading_progress (user_id, file_id, locator) VALUES ($1, $2, '{"k":1}')`, userB, pdf)
		expectCode(t, err, "23514", "locator without a version")

		mustExec(t, db, `DELETE FROM files WHERE id = $1`, pdf)
		if n := count(t, db, `SELECT COUNT(*) FROM reading_progress WHERE user_id = $1`, userA); n != 1 {
			t.Errorf("deleting one file must remove only its progress: %d rows left for the user", n)
		}
	})

	t.Run("notes survive the deletion of their work", func(t *testing.T) {
		mustExec(t, db, `INSERT INTO works (id, original_title, author_id) VALUES (50, 'Efêmera', 1)`)
		mustExec(t, db, `INSERT INTO notes (user_id, work_id, quote) VALUES ($1, 50, 'frase importante')`, userA)
		// The reference was filled from the work by the trigger.
		if got := str(t, db, `SELECT source_title || ' / ' || source_author FROM notes WHERE quote = 'frase importante'`); got != "Efêmera / Frank Herbert" {
			t.Fatalf("reference = %q", got)
		}

		mustExec(t, db, `DELETE FROM works WHERE id = 50`)

		if n := count(t, db, `SELECT COUNT(*) FROM notes WHERE quote = 'frase importante'`); n != 1 {
			t.Fatal("the note was deleted together with its work")
		}
		if n := count(t, db, `SELECT COUNT(*) FROM notes WHERE quote = 'frase importante' AND work_id IS NULL AND source_work_id = 50`); n != 1 {
			t.Error("the link should be cleared while the source id is kept for later reconciliation")
		}
		if got := str(t, db, `SELECT source_title || ' / ' || source_author FROM notes WHERE quote = 'frase importante'`); got != "Efêmera / Frank Herbert" {
			t.Errorf("reference after deletion = %q", got)
		}
	})

	t.Run("a note needs a source reference", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, NULL, 'solta')`, userA)
		expectCode(t, err, "23502", "note with neither work nor reference")
	})

	t.Run("the owner authenticates locally only", func(t *testing.T) {
		mustExec(t, db, `INSERT INTO users (id, username, email, role) VALUES
			('cccccccc-cccc-4ccc-8ccc-cccccccccccc', 'chefe', 'c@x', 'owner')`)
		_, err := db.Exec(`INSERT INTO external_identities (user_id, provider, subject) VALUES
			('cccccccc-cccc-4ccc-8ccc-cccccccccccc', 'ldap', 'uuid-owner')`)
		expectCode(t, err, "23514", "identity for the owner")

		mustExec(t, db, `INSERT INTO external_identities (user_id, provider, subject) VALUES ($1, 'ldap', 'uuid-ana')`, userA)
		_, err = db.Exec(`INSERT INTO external_identities (user_id, provider, subject) VALUES ($1, 'ldap', 'uuid-ana')`, userB)
		expectCode(t, err, "23505", "same subject for two accounts")

		// Handing the ownership to an account that has an external identity is refused.
		mustExec(t, db, `UPDATE users SET role = 'admin' WHERE username = 'chefe'`)
		_, err = db.Exec(`UPDATE users SET role = 'owner' WHERE id = $1`, userA)
		expectCode(t, err, "23514", "promoting an externally-authenticated account to owner")
		// An account without one can receive it.
		mustExec(t, db, `UPDATE users SET role = 'owner' WHERE id = $1`, userB)
	})

	t.Run("audit entries outlive their actor", func(t *testing.T) {
		mustExec(t, db, `INSERT INTO users (id, username, email, role) VALUES ('dddddddd-dddd-4ddd-8ddd-dddddddddddd', 'temp', 't@x', 'admin')`)
		mustExec(t, db, `INSERT INTO audit_log (actor_id, actor_username, action, target_type, target_id, details)
			VALUES ('dddddddd-dddd-4ddd-8ddd-dddddddddddd', 'temp', 'work.retire', 'work', '1', '{"reason":"teste"}')`)
		mustExec(t, db, `DELETE FROM users WHERE username = 'temp'`)
		if n := count(t, db, `SELECT COUNT(*) FROM audit_log WHERE actor_id IS NULL AND actor_username = 'temp'`); n != 1 {
			t.Error("the audit entry must remain after its actor is deleted, with the username snapshot")
		}
	})
}

func TestExpand_CanBeRolledBackAndReapplied(t *testing.T) {
	db := migrated(t)
	mustExec(t, db, `INSERT INTO works (id, original_title, file_path, format) VALUES (1, 'Duna', 'duna.epub', 'epub')`)

	if err := database.RollbackTo(db, 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	for _, tbl := range []string{"files", "storage_locations", "work_contributors", "reading_progress", "audit_log", "external_identities"} {
		if tableExists(t, db, tbl) {
			t.Errorf("%s still exists after rolling back", tbl)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM works WHERE original_title = 'Duna'`); n != 1 {
		t.Error("legacy data was lost by the rollback")
	}
	// The old worker upsert works again.
	mustExec(t, db, `INSERT INTO editions (work_id, title, cover_url) VALUES (1, 'Duna', '/c.jpg')
		ON CONFLICT (work_id) DO UPDATE SET cover_url = EXCLUDED.cover_url`)

	if err := database.Migrate(db); err != nil {
		t.Fatalf("re-applying after rollback: %v", err)
	}
	if got := str(t, db, `SELECT file_path FROM work_primary WHERE work_id = 1`); got != "duna.epub" {
		t.Errorf("data was not projected again: %q", got)
	}
	if got := str(t, db, `SELECT cover_url FROM work_primary WHERE work_id = 1`); got != "/c.jpg" {
		t.Errorf("cover lost across rollback and re-apply: %q", got)
	}
}
