package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

var ctx = context.Background()

// source is a populated instance: a database, a storage directory and a few files.
type source struct {
	t       *testing.T
	db      *sql.DB
	dsn     string
	storage string
}

func newSource(t *testing.T) *source {
	t.Helper()
	if _, err := tool("pg_dump"); err != nil {
		t.Skip("pg_dump is not installed")
	}
	if _, err := tool("pg_restore"); err != nil {
		t.Skip("pg_restore is not installed")
	}
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	useMatchingTools(t, db)
	s := &source{t: t, db: db, dsn: os.Getenv("TEST_DATABASE_URL"), storage: t.TempDir()}

	var ownerID, readerID string
	s.must(db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('boss', 'b@x', 'owner') RETURNING id`).Scan(&ownerID))
	s.must(db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('ana', 'a@x', 'reader') RETURNING id`).Scan(&readerID))
	for i, title := range []string{"Duna", "Neuromancer"} {
		rel := fmt.Sprintf("Autor %d/%s.epub", i, title)
		content := []byte("conteúdo de " + title)
		full := filepath.Join(s.storage, filepath.FromSlash(rel))
		s.must(os.MkdirAll(filepath.Dir(full), 0o755))
		s.must(os.WriteFile(full, content, 0o644))
		_, _, fid := testdb.AddWork(t, db, testdb.Work{Title: title, Path: rel, Format: "epub", Author: fmt.Sprintf("Autor %d", i)})
		s.exec(`UPDATE files SET sha256 = $2, size_bytes = $3 WHERE id = $1`, fid, sha(content), len(content))
		s.exec(`INSERT INTO notes (user_id, work_id, quote) SELECT $1, work_id, 'minha nota sobre ' || $2 FROM work_primary WHERE file_id = $3`, readerID, title, fid)
	}
	s.must(os.MkdirAll(filepath.Join(s.storage, "covers"), 0o755))
	s.must(os.WriteFile(filepath.Join(s.storage, "covers", "c1.jpg"), []byte("jpeg"), 0o644))
	// Credentials that must never travel in a package.
	s.exec(`INSERT INTO sessions (user_id, expires_at) VALUES ($1, now() + interval '1 day')`, ownerID)
	s.exec(`INSERT INTO app_tokens (user_id, name, token_hash) VALUES ($1, 'kobo', repeat('a', 64))`, readerID)
	s.exec(`INSERT INTO invitations (token_hash, role, expires_at) VALUES (repeat('b', 64), 'reader', now() + interval '1 day')`)
	return s
}

func sha(b []byte) string {
	f, _ := os.CreateTemp("", "sha")
	defer os.Remove(f.Name())
	f.Write(b)
	f.Close()
	s, _, _ := sumFile(f.Name())
	return s
}

func (s *source) must(err error) {
	s.t.Helper()
	if err != nil {
		s.t.Fatal(err)
	}
}

func (s *source) exec(q string, args ...any) {
	s.t.Helper()
	_, err := s.db.Exec(q, args...)
	s.must(err)
}

func (s *source) backup(includeFiles bool, passphrase string) ([]byte, Result) {
	s.t.Helper()
	var buf bytes.Buffer
	res, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: &buf,
		IncludeFiles: includeFiles, Passphrase: passphrase, TmpDir: s.t.TempDir()})
	if err != nil {
		s.t.Fatalf("create: %v", err)
	}
	return buf.Bytes(), res
}

// target is another, initially absent, database plus an empty storage directory.
type target struct {
	t       *testing.T
	name    string
	dsn     string
	storage string
	admin   *sql.DB
}

func newTarget(t *testing.T, base string, create bool) *target {
	t.Helper()
	c, err := parseConn(base)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := c.withDB("postgres").open()
	if err != nil {
		t.Fatal(err)
	}
	name := "codice_bk_target_" + randHex(3) + "_test"
	tg := &target{t: t, name: name, dsn: c.withDB(name).url.String(), storage: t.TempDir(), admin: admin}
	t.Cleanup(func() {
		rows, _ := admin.Query(`SELECT datname FROM pg_database WHERE datname LIKE $1`, name+"%")
		var names []string
		for rows != nil && rows.Next() {
			var n string
			rows.Scan(&n)
			names = append(names, n)
		}
		if rows != nil {
			rows.Close()
		}
		for _, n := range names {
			admin.Exec(`DROP DATABASE IF EXISTS ` + quoteIdent(n) + ` WITH (FORCE)`)
		}
		admin.Close()
	})
	if create {
		if _, err := admin.Exec(`CREATE DATABASE ` + quoteIdent(name)); err != nil {
			t.Fatal(err)
		}
	}
	return tg
}

func (tg *target) open() *sql.DB {
	tg.t.Helper()
	c, _ := parseConn(tg.dsn)
	db, err := c.open()
	if err != nil {
		tg.t.Fatal(err)
	}
	tg.t.Cleanup(func() { db.Close() })
	return db
}

func (tg *target) restore(pkg []byte, edit ...func(*RestoreOptions)) (RestoreResult, error) {
	o := RestoreOptions{DatabaseURL: tg.dsn, StorageRoot: tg.storage, In: bytes.NewReader(pkg), TmpDir: tg.t.TempDir()}
	for _, e := range edit {
		e(&o)
	}
	return Restore(ctx, o)
}

// scalar opens the target, asks one question and closes again, so it never counts as
// "someone else using the database" in the overwrite tests.
func (tg *target) scalar(q string, args ...any) string {
	tg.t.Helper()
	c, _ := parseConn(tg.dsn)
	db, err := c.open()
	if err != nil {
		tg.t.Fatal(err)
	}
	defer db.Close()
	return scalar(tg.t, db, q, args...)
}

func scalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s.String
}

// rewrite rebuilds a package with a member changed, dropped or added, keeping the rest.
type member struct {
	h    tar.Header
	body []byte
}

func rewrite(t *testing.T, pkg []byte, edit func(name string, body []byte) (string, []byte, bool), extra ...member) []byte {
	t.Helper()
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	tr := tar.NewReader(bytes.NewReader(pkg))
	var manifest *tar.Header
	var manifestBody []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		name, nb, keep := edit(h.Name, body)
		if !keep {
			continue
		}
		if name == MemberManifest {
			hh := *h
			manifest, manifestBody = &hh, nb
			continue
		}
		hh := *h
		hh.Name, hh.Size = name, int64(len(nb))
		tw.WriteHeader(&hh)
		tw.Write(nb)
	}
	for _, x := range extra {
		h := x.h
		h.Size = int64(len(x.body))
		h.Typeflag = tar.TypeReg
		if h.Linkname != "" {
			h.Typeflag = tar.TypeSymlink
			h.Size = 0
		}
		tw.WriteHeader(&h)
		tw.Write(x.body)
	}
	if manifest != nil {
		manifest.Size = int64(len(manifestBody))
		tw.WriteHeader(manifest)
		tw.Write(manifestBody)
	}
	tw.Close()
	return out.Bytes()
}

func keepAll(name string, body []byte) (string, []byte, bool) { return name, body, true }

func TestCreate_PackageHoldsWhatItPromisesAndNoCredentials(t *testing.T) {
	s := newSource(t)
	pkg, res := s.backup(false, "")

	p, err := Read(bytes.NewReader(pkg), ReadOptions{TmpDir: t.TempDir()})
	if err != nil {
		t.Fatalf("a package just made does not verify: %v", err)
	}
	defer p.Close()
	m := p.Manifest
	if m.Format != FormatVersion || m.SchemaVersion != database.LatestVersion() || m.IncludesFiles {
		t.Errorf("manifest = %+v", m)
	}
	if m.Counts != (Counts{Users: 2, Works: 2, Files: 2, Notes: 2}) {
		t.Errorf("counts = %+v", m.Counts)
	}
	if len(p.Files) != 2 || p.Files[0].SHA256 == "" || p.Files[0].Mode != "managed" {
		t.Errorf("the list of files = %+v", p.Files)
	}
	if _, has := m.Members[MemberStorage+"Autor 0/Duna.epub"]; has {
		t.Error("the files are in a package made without --include-files")
	}
	if res.Bytes != int64(len(pkg)) || res.Encrypted {
		t.Errorf("result = %+v", res)
	}

	// Restored elsewhere, the credentials are simply not there.
	tg := newTarget(t, s.dsn, false)
	if _, err := tg.restore(pkg); err != nil {
		t.Fatal(err)
	}
	db := tg.open()
	for _, table := range credentialTables {
		if n := scalar(t, db, `SELECT count(*) FROM `+table); n != "0" {
			t.Errorf("%s came back with %s rows", table, n)
		}
	}
	if scalar(t, db, `SELECT count(*) FROM users`) != "2" {
		t.Error("the accounts did not come back")
	}
}

func TestRoundTrip_RestoresNotesFilesAndCoversIntoACleanInstance(t *testing.T) {
	s := newSource(t)
	pkg, res := s.backup(true, "")
	if !res.Manifest.IncludesFiles || res.Manifest.Files.Included != 2 || len(res.Warnings) != 0 {
		t.Fatalf("result = %+v", res)
	}

	tg := newTarget(t, s.dsn, true) // an empty database, as a fresh install has
	out, err := tg.restore(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if out.FilesRestored != 3 || out.FilesVerified != 2 || out.FilesMissing != 0 || out.FilesMismatched != 0 || out.KeptDatabase != "" {
		t.Errorf("result = %+v", out)
	}
	db := tg.open()
	if got := scalar(t, db, `SELECT string_agg(quote, '|' ORDER BY quote) FROM notes`); got != "minha nota sobre Duna|minha nota sobre Neuromancer" {
		t.Errorf("notes = %q", got)
	}
	// The files open again, byte for byte, and so does the cover.
	for _, rel := range []string{"Autor 0/Duna.epub", "Autor 1/Neuromancer.epub", "covers/c1.jpg"} {
		if _, err := os.Stat(filepath.Join(tg.storage, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s did not come back: %v", rel, err)
		}
	}
	if v, _ := database.Version(db); v != database.LatestVersion() {
		t.Errorf("schema version after restore = %d, want %d", v, database.LatestVersion())
	}
	// The owner is told, and the restore is on the record.
	if scalar(t, db, `SELECT count(*) FROM security_notices WHERE kind = 'instance_restored' AND user_id = (SELECT id FROM users WHERE role = 'owner')`) != "1" {
		t.Error("the owner is not told at the next sign-in")
	}
	if scalar(t, db, `SELECT count(*) FROM audit_log WHERE action = 'backup.restore'`) != "1" {
		t.Error("the restore was not audited")
	}
	if _, err := os.Stat(filepath.Join(tg.storage)); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(tg.storage)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".restore-") {
			t.Error("the staging directory was left behind")
		}
	}
}

func TestRestore_WithoutTheFilesTellsWhichAreMissingAndMarksThem(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(false, "")

	tg := newTarget(t, s.dsn, true)
	// Only one of the two files exists on the new machine, and it has been damaged.
	if err := os.MkdirAll(filepath.Join(tg.storage, "Autor 0"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tg.storage, "Autor 0", "Duna.epub"), []byte("conteúdo de Duna"), 0o644)
	os.MkdirAll(filepath.Join(tg.storage, "Autor 1"), 0o755)
	os.WriteFile(filepath.Join(tg.storage, "Autor 1", "Neuromancer.epub"), []byte("outro conteúdo qualquer!"), 0o644)
	os.Remove(filepath.Join(tg.storage, "Autor 0", "Duna.epub")) // now Duna is missing
	os.WriteFile(filepath.Join(tg.storage, "Autor 0", "Duna.epub"), []byte("conteúdo de Duna"), 0o644)

	out, err := tg.restore(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if out.FilesVerified != 1 || out.FilesMismatched != 1 || out.FilesMissing != 0 {
		t.Errorf("result = %+v; one intact, one with a different content", out)
	}

	// A file that is simply absent is marked so the interface can say so.
	tg2 := newTarget(t, s.dsn, true)
	out2, err := tg2.restore(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if out2.FilesMissing != 2 {
		t.Fatalf("result = %+v", out2)
	}
	db := tg2.open()
	if scalar(t, db, `SELECT count(*) FROM storage_locations WHERE state = 'missing'`) != "2" ||
		scalar(t, db, `SELECT count(*) FROM files WHERE availability = 'missing'`) != "2" {
		t.Error("the missing files were not marked as missing")
	}
}

func TestEncryption_NeedsThePassphraseAndHidesTheContent(t *testing.T) {
	s := newSource(t)
	pkg, res := s.backup(false, "uma-frase-bem-longa")
	if !res.Encrypted {
		t.Error("not reported as encrypted")
	}
	for _, leak := range []string{"PGDMP", "database.dump", "minha nota", "Duna", "manifest"} {
		if bytes.Contains(pkg, []byte(leak)) {
			t.Errorf("the encrypted package contains %q in the clear", leak)
		}
	}
	if _, err := Read(bytes.NewReader(pkg), ReadOptions{TmpDir: t.TempDir()}); !errors.Is(err, ErrEncrypted) {
		t.Errorf("no passphrase: %v, want ErrEncrypted", err)
	}
	if _, err := Read(bytes.NewReader(pkg), ReadOptions{Passphrase: "outra-frase-errada", TmpDir: t.TempDir()}); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("wrong passphrase: %v, want ErrWrongPassphrase", err)
	}
	p, err := Read(bytes.NewReader(pkg), ReadOptions{Passphrase: "uma-frase-bem-longa", TmpDir: t.TempDir()})
	if err != nil {
		t.Fatalf("right passphrase: %v", err)
	}
	p.Close()

	// A flipped byte inside the ciphertext is caught too.
	bad := append([]byte(nil), pkg...)
	bad[len(bad)/2] ^= 0xff
	if _, err := Read(bytes.NewReader(bad), ReadOptions{Passphrase: "uma-frase-bem-longa", TmpDir: t.TempDir()}); err == nil {
		t.Error("a damaged encrypted package was accepted")
	}
	// And a passphrase too short to mean anything is refused before anything is written.
	if _, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: io.Discard, Passphrase: "curta"}); err == nil {
		t.Error("a short passphrase was accepted")
	}
}

func TestRead_RefusesEveryKindOfBadPackage(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(true, "")
	read := func(b []byte) error {
		p, err := Read(bytes.NewReader(b), ReadOptions{TmpDir: t.TempDir(), StageDir: filepath.Join(t.TempDir(), "stage")})
		p.Close()
		return err
	}

	flip := func(name string, body []byte) (string, []byte, bool) {
		if name == MemberDump {
			b := append([]byte(nil), body...)
			b[len(b)/2] ^= 0xff
			return name, b, true
		}
		return name, body, true
	}
	drop := func(victim string) func(string, []byte) (string, []byte, bool) {
		return func(name string, body []byte) (string, []byte, bool) { return name, body, name != victim }
	}
	newer := func(name string, body []byte) (string, []byte, bool) {
		if name == MemberManifest {
			var m Manifest
			json.Unmarshal(body, &m)
			m.SchemaVersion = database.LatestVersion() + 1
			b, _ := json.Marshal(m)
			return name, b, true
		}
		return name, body, true
	}
	format := func(name string, body []byte) (string, []byte, bool) {
		if name == MemberManifest {
			var m Manifest
			json.Unmarshal(body, &m)
			m.Format = 99
			b, _ := json.Marshal(m)
			return name, b, true
		}
		return name, body, true
	}

	cases := map[string]struct {
		pkg  []byte
		want error
	}{
		"a damaged dump":                {rewrite(t, pkg, flip), ErrCorrupt},
		"a missing dump":                {rewrite(t, pkg, drop(MemberDump)), ErrCorrupt},
		"a missing list of files":       {rewrite(t, pkg, drop(MemberFiles)), ErrCorrupt},
		"a missing stored file":         {rewrite(t, pkg, drop(MemberStorage+"Autor 0/Duna.epub")), ErrCorrupt},
		"no manifest (truncated)":       {rewrite(t, pkg, drop(MemberManifest)), ErrCorrupt},
		"a newer schema":                {rewrite(t, pkg, newer), ErrNewerSchema},
		"an unknown format":             {rewrite(t, pkg, format), ErrCorrupt},
		"garbage":                       {[]byte("isto não é um pacote de backup"), ErrCorrupt},
		"an empty input":                {nil, ErrCorrupt},
		"a cut in the middle":           {pkg[:len(pkg)/2], ErrCorrupt},
		"a member that escapes storage": {rewrite(t, pkg, keepAll, member{h: tar.Header{Name: "storage/../../etc/cron.d/x"}}), ErrCorrupt},
		"an absolute member":            {rewrite(t, pkg, keepAll, member{h: tar.Header{Name: "/etc/passwd"}}), ErrCorrupt},
		"a member outside the layout":   {rewrite(t, pkg, keepAll, member{h: tar.Header{Name: "extra.sh"}}), ErrCorrupt},
		"a symbolic link":               {rewrite(t, pkg, keepAll, member{h: tar.Header{Name: "storage/link", Linkname: "/etc/passwd"}}), ErrCorrupt},
	}
	for name, c := range cases {
		if err := read(c.pkg); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	// A member that is in the package but not in the manifest is refused as well.
	extra := rewrite(t, pkg, func(name string, body []byte) (string, []byte, bool) {
		if name == MemberManifest {
			var m Manifest
			json.Unmarshal(body, &m)
			delete(m.Members, MemberStorage+"Autor 0/Duna.epub")
			b, _ := json.Marshal(m)
			return name, b, true
		}
		return name, body, true
	})
	if err := read(extra); !errors.Is(err, ErrCorrupt) {
		t.Errorf("a member missing from the manifest: %v", err)
	}
}

func TestRestore_ARefusedPackageLeavesTheInstanceAsItWas(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(true, "")
	bad := rewrite(t, pkg, func(name string, body []byte) (string, []byte, bool) {
		if name == MemberDump {
			b := append([]byte(nil), body...)
			b[len(b)/2] ^= 0xff
			return name, b, true
		}
		return name, body, true
	})

	tg := newTarget(t, s.dsn, true)
	if _, err := tg.restore(bad); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("restore: %v, want ErrCorrupt", err)
	}
	if n := scalar(t, tg.open(), `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`); n != "0" {
		t.Errorf("a refused package created %s tables", n)
	}
	entries, _ := os.ReadDir(tg.storage)
	if len(entries) != 0 {
		t.Errorf("a refused package left %d entries in the storage", len(entries))
	}
}

func TestRestore_WillNotOverwriteWithoutBeingToldAndNeverDestroysTheOldData(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(false, "")

	// A running instance with its own data.
	tg := newTarget(t, s.dsn, true)
	if _, err := tg.restore(pkg); err != nil {
		t.Fatal(err)
	}
	live := tg.open()
	if _, err := live.Exec(`INSERT INTO users (username, email, role) VALUES ('novo', 'n@x', 'reader')`); err != nil {
		t.Fatal(err)
	}
	live.Close()
	time.Sleep(300 * time.Millisecond)

	if _, err := tg.restore(pkg); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("restoring over data: %v, want ErrNotEmpty", err)
	}
	if tg.scalar(`SELECT count(*) FROM users`) != "3" {
		t.Fatal("a refused restore changed the data")
	}

	// Someone still connected: refused, not forced.
	busy := tg.open()
	if err := busy.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := tg.restore(pkg, func(o *RestoreOptions) { o.Overwrite = true }); !errors.Is(err, ErrBusy) {
		t.Fatalf("with a connection open: %v, want ErrBusy", err)
	}
	busy.Close()
	time.Sleep(300 * time.Millisecond)

	out, err := tg.restore(pkg, func(o *RestoreOptions) { o.Overwrite = true })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.KeptDatabase, "_before_restore_") {
		t.Fatalf("the previous database was not kept: %+v", out)
	}
	if tg.scalar(`SELECT count(*) FROM users`) != "2" {
		t.Error("the restored database does not hold the package's data")
	}
	kept, _ := parseConn(tg.dsn)
	old, err := kept.withDB(out.KeptDatabase).open()
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if scalar(t, old, `SELECT count(*) FROM users WHERE username = 'novo'`) != "1" {
		t.Error("the data that was there before is gone; it must be kept")
	}
}

func TestRestore_RefusesToReplaceADifferentFileBeforeTouchingTheDatabase(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(true, "")
	tg := newTarget(t, s.dsn, true)
	os.MkdirAll(filepath.Join(tg.storage, "Autor 0"), 0o755)
	os.WriteFile(filepath.Join(tg.storage, "Autor 0", "Duna.epub"), []byte("um arquivo diferente que já estava aqui"), 0o644)

	if _, err := tg.restore(pkg); err == nil || !strings.Contains(err.Error(), "would be overwritten") {
		t.Fatalf("restore: %v, want a refusal to overwrite", err)
	}
	if n := scalar(t, tg.open(), `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`); n != "0" {
		t.Errorf("the database was touched (%s tables)", n)
	}
	got, _ := os.ReadFile(filepath.Join(tg.storage, "Autor 0", "Duna.epub"))
	if string(got) != "um arquivo diferente que já estava aqui" {
		t.Error("the existing file was overwritten")
	}
	// The same file already there is fine and is left alone.
	tg2 := newTarget(t, s.dsn, true)
	os.MkdirAll(filepath.Join(tg2.storage, "Autor 0"), 0o755)
	os.WriteFile(filepath.Join(tg2.storage, "Autor 0", "Duna.epub"), []byte("conteúdo de Duna"), 0o644)
	out, err := tg2.restore(pkg)
	if err != nil || out.FilesSkipped != 1 {
		t.Errorf("an identical file: %v %+v", err, out)
	}
}

func TestVerify_ChecksThePackageAndRehearsesTheRestore(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(true, "")

	res, err := Verify(ctx, VerifyOptions{In: bytes.NewReader(pkg), DatabaseURL: s.dsn, TmpDir: t.TempDir()})
	if err != nil || res.Deep != nil || res.DumpEntries == 0 || res.Manifest.Counts.Users != 2 {
		t.Fatalf("verify: %v %+v", err, res)
	}
	res, err = Verify(ctx, VerifyOptions{In: bytes.NewReader(pkg), DatabaseURL: s.dsn, TmpDir: t.TempDir(), Deep: true})
	if err != nil || res.Deep == nil || res.Deep.Restored != res.Manifest.Counts || res.Deep.Duration <= 0 {
		t.Fatalf("deep verify: %v %+v", err, res)
	}
	// The scratch database is gone, and nothing else changed.
	c, _ := parseConn(s.dsn)
	admin, _ := c.withDB("postgres").open()
	defer admin.Close()
	if scalar(t, admin, `SELECT count(*) FROM pg_database WHERE datname LIKE 'codice_verify_%'`) != "0" {
		t.Error("the rehearsal left a scratch database behind")
	}

	bad := pkg[:len(pkg)-200]
	if _, err := Verify(ctx, VerifyOptions{In: bytes.NewReader(bad), TmpDir: t.TempDir()}); !errors.Is(err, ErrCorrupt) {
		t.Errorf("a truncated package: %v", err)
	}
}

func TestCreate_WarnsAboutWhatCannotBeIncludedAndKeepsGoing(t *testing.T) {
	s := newSource(t)
	os.Remove(filepath.Join(s.storage, "Autor 1", "Neuromancer.epub"))
	os.WriteFile(filepath.Join(s.storage, "Autor 0", "Duna.epub"), []byte("alterado no disco depois de catalogado"), 0o644)

	_, res := s.backup(true, "")
	joined := strings.Join(res.Warnings, "\n")
	if res.Manifest.Files.Missing != 1 || res.Manifest.Files.Included != 1 {
		t.Errorf("files = %+v", res.Manifest.Files)
	}
	if !strings.Contains(joined, "not on disk") || !strings.Contains(joined, "does not match the hash") {
		t.Errorf("warnings = %q", joined)
	}
}

func TestCreate_AFileThatCannotBeReadStopsTheBackupInsteadOfLeavingItOut(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	s := newSource(t)
	locked := filepath.Join(s.storage, "Autor 0", "Duna.epub")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o644)

	var out bytes.Buffer
	_, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: &out, IncludeFiles: true, TmpDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "cannot be read") || !strings.Contains(err.Error(), "Duna.epub") {
		t.Fatalf("an unreadable file: %v", err)
	}
	// Without the files there is nothing to read, and the backup goes through.
	out.Reset()
	if _, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: &out, TmpDir: t.TempDir()}); err != nil {
		t.Errorf("without files: %v", err)
	}
}

func TestCreate_TheDumpAndTheListComeFromOneSnapshot(t *testing.T) {
	s := newSource(t)
	// Works are added while packages are made. Whatever is in a package, the list of files
	// and the counts of the dump must agree with each other.
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			tx, err := s.db.Begin()
			if err != nil {
				return
			}
			var w, e, f int64
			ok := tx.QueryRow(`INSERT INTO works (original_title) VALUES ($1) RETURNING id`, fmt.Sprintf("Extra %d", i)).Scan(&w) == nil &&
				tx.QueryRow(`INSERT INTO editions (work_id, title, is_primary) VALUES ($1, 'x', TRUE) RETURNING id`, w).Scan(&e) == nil &&
				tx.QueryRow(`INSERT INTO files (edition_id, format) VALUES ($1, 'epub') RETURNING id`, e).Scan(&f) == nil
			if ok {
				_, err = tx.Exec(`INSERT INTO storage_locations (file_id, mode, path) VALUES ($1, 'managed', $2)`, f, fmt.Sprintf("extra/%d.epub", i))
			}
			if ok && err == nil {
				tx.Commit()
			} else {
				tx.Rollback()
			}
		}
	}()
	for i := 0; i < 6; i++ {
		pkg, _ := s.backup(false, "")
		p, err := Read(bytes.NewReader(pkg), ReadOptions{TmpDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Files) != p.Manifest.Counts.Files {
			t.Errorf("run %d: %d files listed but the snapshot counted %d", i, len(p.Files), p.Manifest.Counts.Files)
		}
		p.Close()
		// The rehearsal restores the dump and compares it with what the manifest counted in
		// the snapshot: they only agree if the dump was taken from that same snapshot.
		if _, err := Verify(ctx, VerifyOptions{In: bytes.NewReader(pkg), DatabaseURL: s.dsn, TmpDir: t.TempDir(), Deep: true}); err != nil {
			t.Errorf("run %d: the dump does not agree with the snapshot the list came from: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestCreate_WithoutThePostgresToolsSaysSo(t *testing.T) {
	s := newSource(t)
	t.Setenv("PATH", "")
	t.Setenv("PG_DUMP", "")
	if _, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: io.Discard}); !errors.Is(err, ErrNoTool) {
		t.Errorf("no pg_dump: %v, want ErrNoTool", err)
	}
}

func TestRecord_KeepsTheLatestBackupForTheInterface(t *testing.T) {
	s := newSource(t)
	if l, err := Last(ctx, s.db); err != nil || l != nil {
		t.Fatalf("before any backup: %v %v", l, err)
	}
	_, res := s.backup(true, "")
	if err := Record(ctx, s.db, res); err != nil {
		t.Fatal(err)
	}
	l, err := Last(ctx, s.db)
	if err != nil || l == nil || !l.IncludesFiles || l.Files != 2 || l.Bytes != res.Bytes || l.Encrypted {
		t.Errorf("last = %+v %v", l, err)
	}
	if scalar(t, s.db, `SELECT count(*) FROM audit_log WHERE action = 'backup.create'`) != "1" {
		t.Error("the backup was not audited")
	}
}

func TestPrune_KeepsTheNewestOfEachDayWeekAndMonth(t *testing.T) {
	dir := t.TempDir()
	day := func(d string) time.Time { x, _ := time.Parse("2006-01-02", d); return x.Add(3 * time.Hour) }
	var made []string
	// Two a day for 40 days, plus files that are not packages.
	for i := 0; i < 40; i++ {
		for _, h := range []int{0, 12} {
			at := day("2026-08-01").AddDate(0, 0, i).Add(time.Duration(h) * time.Hour)
			name := PackageName(at, i%2 == 0)
			os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
			made = append(made, name)
		}
	}
	for _, other := range []string{"notes.txt", "codice-backup-2026-08-01.tar", "codice-backup-20260801-000000.zip", "outro.tar"} {
		os.WriteFile(filepath.Join(dir, other), []byte("keep me"), 0o644)
	}

	dry, err := Prune(dir, DefaultPolicy, true)
	if err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != len(made)+4 {
		t.Fatalf("a dry run deleted files: %d left", len(entries))
	}
	if len(dry.Keep) == 0 || len(dry.Delete) == 0 || len(dry.Keep)+len(dry.Delete) != len(made) {
		t.Fatalf("keep %d delete %d of %d", len(dry.Keep), len(dry.Delete), len(made))
	}
	// The newest is always kept, and the last seven days have one each.
	newest := PackageName(day("2026-08-01").AddDate(0, 0, 39).Add(12*time.Hour), false)
	kept := map[string]bool{}
	for _, k := range dry.Keep {
		kept[strings.TrimSuffix(k, ".age")] = true
	}
	if !kept[strings.TrimSuffix(newest, ".age")] {
		t.Error("the newest package would be deleted")
	}
	for i := 0; i < 7; i++ {
		d := day("2026-08-01").AddDate(0, 0, 39-i).Format("20060102")
		found := false
		for k := range kept {
			if strings.Contains(k, "-"+d+"-") {
				found = true
			}
		}
		if !found {
			t.Errorf("no package kept for day %s inside the last seven", d)
		}
	}
	// Far fewer than everything is kept: 7 days + up to 4 weeks + up to 3 months, overlapping.
	if len(dry.Keep) > 7+4+3 {
		t.Errorf("kept %d, more than the policy allows", len(dry.Keep))
	}

	if _, err := Prune(dir, DefaultPolicy, false); err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{"notes.txt", "codice-backup-2026-08-01.tar", "codice-backup-20260801-000000.zip", "outro.tar"} {
		if _, err := os.Stat(filepath.Join(dir, other)); err != nil {
			t.Errorf("prune touched %s, which is not a package", other)
		}
	}
	left := 0
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if packageRe.MatchString(e.Name()) {
			left++
		}
	}
	if left != len(dry.Keep) {
		t.Errorf("%d packages left, the plan said %d", left, len(dry.Keep))
	}
}

// useMatchingTools makes the PostgreSQL tools the same major version as the test server,
// which is what a real install must do. If the local ones differ it runs the tools from
// inside the test container, the way a wrapper on PATH would.
func useMatchingTools(t *testing.T, db *sql.DB) {
	t.Helper()
	var server string
	if err := db.QueryRow(`SHOW server_version`).Scan(&server); err != nil {
		t.Fatal(err)
	}
	if local, err := toolMajor(ctx, "pg_dump"); err == nil && local == major(server) {
		return
	}
	container := os.Getenv("TEST_PG_CONTAINER")
	if container == "" {
		container = "codice-test-pg"
	}
	dir := t.TempDir()
	for _, name := range []string{"pg_dump", "pg_restore"} {
		script := fmt.Sprintf("#!/bin/sh\nexec docker exec -i -e PGHOST=127.0.0.1 -e PGPORT=5432 -e PGUSER -e PGPASSWORD -e PGDATABASE -e PGSSLMODE %s %s \"$@\"\n", container, name)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(strings.ToUpper(name), p)
	}
	if local, err := toolMajor(ctx, "pg_dump"); err != nil || local != major(server) {
		t.Skipf("no pg_dump %d available (the local one differs and the container %s cannot be used: %v)", major(server), container, err)
	}
}

func TestCreate_RefusesAPgDumpNewerThanTheServer(t *testing.T) {
	s := newSource(t)
	fake := filepath.Join(t.TempDir(), "pg_dump")
	os.WriteFile(fake, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'pg_dump (PostgreSQL) 99.0'; exit 0; fi\necho should not run >&2; exit 1\n"), 0o755)
	t.Setenv("PG_DUMP", fake)
	if _, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: io.Discard}); !errors.Is(err, ErrToolTooNew) {
		t.Errorf("a pg_dump newer than the server: %v, want ErrToolTooNew", err)
	}
}

func TestRestore_RefusesAPackageFromANewerServerBeforeChangingAnything(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(false, "")
	future := rewrite(t, pkg, func(name string, body []byte) (string, []byte, bool) {
		if name == MemberManifest {
			var m Manifest
			json.Unmarshal(body, &m)
			m.ServerVersion = "99.1"
			b, _ := json.Marshal(m)
			return name, b, true
		}
		return name, body, true
	})
	tg := newTarget(t, s.dsn, true)
	if _, err := tg.restore(future); !errors.Is(err, ErrServerTooOld) {
		t.Fatalf("restore: %v, want ErrServerTooOld", err)
	}
	if n := scalar(t, tg.open(), `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`); n != "0" {
		t.Errorf("the target was touched (%d tables)", len(n))
	}
}

func TestMajor(t *testing.T) {
	for in, want := range map[string]int{"16.14": 16, "pg_dump (PostgreSQL) 18.1": 18, "15.2 (Debian 15.2-1)": 15, "": 0, "banana": 0} {
		if got := major(in); got != want {
			t.Errorf("major(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRead_NothingIsEverWrittenOutsideTheStagingDirectory(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(true, "")
	outer := t.TempDir()
	stage := filepath.Join(outer, "stage")
	// A member whose name climbs out of the staging directory, with content.
	evil := rewrite(t, pkg, keepAll,
		member{h: tar.Header{Name: "storage/../escaped.txt"}, body: []byte("pwned")},
		member{h: tar.Header{Name: "storage/../../escaped2.txt"}, body: []byte("pwned")})
	p, err := Read(bytes.NewReader(evil), ReadOptions{TmpDir: t.TempDir(), StageDir: stage})
	p.Close()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("read: %v, want ErrCorrupt", err)
	}
	for _, escaped := range []string{filepath.Join(outer, "escaped.txt"), filepath.Join(filepath.Dir(outer), "escaped2.txt"), filepath.Join(stage, "..", "escaped.txt")} {
		if _, err := os.Stat(escaped); err == nil {
			t.Errorf("%s was written: a package could put a file anywhere", escaped)
			os.Remove(escaped)
		}
	}
}

func TestRestore_ACopyThatDoesNotMatchThePackageIsNeverSwappedIn(t *testing.T) {
	s := newSource(t)
	pkg, _ := s.backup(false, "")
	// The manifest is not part of the checksums, so a wrong one can only be caught by
	// looking at what the restored copy actually holds.
	lying := rewrite(t, pkg, func(name string, body []byte) (string, []byte, bool) {
		if name == MemberManifest {
			var m Manifest
			json.Unmarshal(body, &m)
			m.Counts.Users = 99
			b, _ := json.Marshal(m)
			return name, b, true
		}
		return name, body, true
	})

	tg := newTarget(t, s.dsn, true)
	if _, err := tg.restore(pkg); err != nil { // an instance with real data
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	_, err := tg.restore(lying, func(o *RestoreOptions) { o.Overwrite = true })
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("restore: %v, want ErrCorrupt", err)
	}
	if tg.scalar(`SELECT count(*) FROM users`) != "2" {
		t.Error("the running database was replaced by a copy that did not match")
	}
	if tg.scalar(`SELECT count(*) FROM pg_database WHERE datname LIKE '%\_restore\_%' OR datname LIKE '%before_restore%'`) != "0" {
		t.Error("a refused restore left databases behind")
	}
}
