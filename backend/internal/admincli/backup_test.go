package admincli_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/admincli"
	"github.com/ocnaibill/codice/backend/internal/backup"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

// useMatchingTools uses PostgreSQL tools of the server's major version, from the test
// container when the local ones differ, as a real install must (see the backup package).
func useMatchingTools(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(name); err != nil && os.Getenv(strings.ToUpper(name)) == "" {
			t.Skip(name + " is not installed")
		}
	}
	var server string
	if err := db.QueryRow(`SHOW server_version`).Scan(&server); err != nil {
		t.Fatal(err)
	}
	want, _ := strconv.Atoi(strings.SplitN(server, ".", 2)[0])
	out, _ := exec.Command("pg_dump", "--version").Output()
	if m := regexp.MustCompile(`(\d+)\.\d+`).FindStringSubmatch(string(out)); m != nil {
		if got, _ := strconv.Atoi(m[1]); got == want {
			return
		}
	}
	container := os.Getenv("TEST_PG_CONTAINER")
	if container == "" {
		container = "codice-test-pg"
	}
	dir := t.TempDir()
	for _, name := range []string{"pg_dump", "pg_restore"} {
		script := fmt.Sprintf("#!/bin/sh\nexec docker exec -i -e PGHOST=127.0.0.1 -e PGPORT=5432 -e PGUSER -e PGPASSWORD -e PGDATABASE -e PGSSLMODE %s %s \"$@\"\n", container, name)
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(script), 0o755)
		t.Setenv(strings.ToUpper(name), p)
	}
}

type site struct {
	t       *testing.T
	db      *sql.DB
	env     admincli.Env
	storage string
}

func newSite(t *testing.T) *site {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	useMatchingTools(t, db)
	dsn := os.Getenv("TEST_DATABASE_URL")
	storage := t.TempDir()
	hash := "$2a$04$abcdefghijklmnopqrstuuA5Zj2v0vXo1kWbM4uT5f4l7z1s2q3wK"
	db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ('boss', 'b@x', $1, 'owner'), ('ana', 'a@x', $1, 'reader')`, hash)
	os.MkdirAll(filepath.Join(storage, "A"), 0o755)
	os.WriteFile(filepath.Join(storage, "A", "duna.epub"), []byte("duna"), 0o644)
	_, _, fid := testdb.AddWork(t, db, testdb.Work{Title: "Duna", Path: "A/duna.epub", Format: "epub"})
	db.Exec(`UPDATE files SET size_bytes = 4, sha256 = encode(sha256('duna'), 'hex') WHERE id = $1`, fid)
	return &site{t: t, db: db, storage: storage, env: admincli.Env{DatabaseURL: dsn, StorageRoot: storage, TmpDir: t.TempDir(), Getenv: func(string) string { return "" }}}
}

func (s *site) run(in string, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	env := s.env
	env.Stderr = &errOut
	err := admincli.RunEnv(context.Background(), s.db, env, args, strings.NewReader(in), &out)
	return out.String(), errOut.String(), err
}

func TestBackup_WritesANamedPrivatePackageAndRecordsIt(t *testing.T) {
	s := newSite(t)
	dir := filepath.Join(t.TempDir(), "backups")
	out, _, err := s.run("", "backup", "--dir", dir, "--include-files")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 || !regexp.MustCompile(`^codice-backup-\d{8}-\d{6}\.tar$`).MatchString(files[0].Name()) {
		t.Fatalf("files = %v", files)
	}
	info, _ := files[0].Info()
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the package is readable by others: %v", info.Mode())
	}
	for _, want := range []string{"Backup made", "2 accounts", "1 works", "1 included in the package", "encrypted: NO"} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary does not say %q:\n%s", want, out)
		}
	}
	if l, _ := backup.Last(context.Background(), s.db); l == nil || !l.IncludesFiles {
		t.Errorf("last backup = %+v", l)
	}
}

func TestBackup_NeverWritesToTheTerminalByAccidentOrOverAnExistingFile(t *testing.T) {
	s := newSite(t)
	if _, _, err := s.run("", "backup"); err == nil {
		t.Error("a backup with nowhere to go was accepted")
	}
	if _, _, err := s.run("", "backup", "--dir", t.TempDir(), "--out", "x.tar"); err == nil {
		t.Error("both --dir and --out were accepted")
	}
	target := filepath.Join(t.TempDir(), "meu.tar")
	os.WriteFile(target, []byte("um backup antigo"), 0o600)
	if _, _, err := s.run("", "backup", "--out", target); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("an existing file: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "um backup antigo" {
		t.Error("the existing backup was overwritten")
	}
	// A failed backup leaves no half file behind.
	bad := s.env
	bad.DatabaseURL = "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable"
	out := filepath.Join(t.TempDir(), "falha.tar")
	err := admincli.RunEnv(context.Background(), s.db, bad, []string{"backup", "--out", out}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("a backup against an unreachable database succeeded")
	}
	if _, serr := os.Stat(out); serr == nil {
		t.Error("a file was left after a failed backup")
	}
	if _, serr := os.Stat(out + ".partial"); serr == nil {
		t.Error("a partial file was left after a failed backup")
	}
}

func TestBackup_ToStandardOutputKeepsMessagesOffThePackage(t *testing.T) {
	s := newSite(t)
	out, errOut, err := s.run("", "backup", "--out", "-")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Backup made") || !strings.Contains(errOut, "Backup made") {
		t.Errorf("the summary must go to standard error only:\nstdout=%q\nstderr=%q", out[:min(60, len(out))], errOut)
	}
	p, err := backup.Read(strings.NewReader(out), backup.ReadOptions{TmpDir: t.TempDir()})
	if err != nil {
		t.Fatalf("what came out on standard output is not a clean package: %v", err)
	}
	p.Close()
}

func TestBackup_EncryptedNeedsThePassphraseFromAFileOrTheEnvironmentNeverAnArgument(t *testing.T) {
	s := newSite(t)
	s.env.Getenv = func(k string) string {
		if k == "CODICE_BACKUP_PASSPHRASE" {
			return "frase-do-ambiente-123"
		}
		return ""
	}
	dir := t.TempDir()
	out, _, err := s.run("", "backup", "--dir", dir)
	if err != nil || !strings.Contains(out, "encrypted: yes") {
		t.Fatalf("%v\n%s", err, out)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 || !strings.HasSuffix(files[0].Name(), ".tar.age") {
		t.Fatalf("files = %v", files)
	}
	pkg := filepath.Join(dir, files[0].Name())

	s.env.Getenv = func(string) string { return "" }
	if _, _, err := s.run("", "verify-backup", pkg); err == nil || !strings.Contains(err.Error(), "encrypted") {
		t.Errorf("no passphrase: %v", err)
	}
	pf := filepath.Join(t.TempDir(), "pass")
	os.WriteFile(pf, []byte("frase-do-ambiente-123\n"), 0o600)
	if out, _, err := s.run("", "verify-backup", pkg, "--passphrase-file", pf); err != nil || !strings.Contains(out, "intact") || !strings.Contains(out, "encrypted") {
		t.Errorf("with the file: %v\n%s", err, out)
	}
	if _, _, err := s.run("", "backup", "--dir", t.TempDir(), "--passphrase-file", filepath.Join(t.TempDir(), "nao-existe")); err == nil {
		t.Error("a missing passphrase file was accepted")
	}
}

func TestVerifyBackup_ConfirmsAnIntactPackageRehearsesARestoreAndRefusesADamagedOne(t *testing.T) {
	s := newSite(t)
	dir := t.TempDir()
	s.run("", "backup", "--dir", dir)
	files, _ := os.ReadDir(dir)
	pkg := filepath.Join(dir, files[0].Name())

	out, _, err := s.run("", "verify-backup", pkg)
	if err != nil || !strings.Contains(out, "The package is intact") || strings.Contains(out, "rehearsal") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, _, err = s.run("", "verify-backup", "--deep", pkg)
	if err != nil || !strings.Contains(out, "Restore rehearsal") || !strings.Contains(out, "matches") || !strings.Contains(out, "/s)") || !strings.Contains(out, "Reading the package plus") {
		t.Fatalf("deep: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(pkg)
	raw[len(raw)/2] ^= 0xff
	bad := filepath.Join(dir, "danificado.tar")
	os.WriteFile(bad, raw, 0o600)
	if _, _, err := s.run("", "verify-backup", bad); err == nil {
		t.Error("a damaged package was reported intact")
	}
	if _, _, err := s.run("", "verify-backup"); err == nil {
		t.Error("no package given was accepted")
	}
	// From standard input as well.
	if out, _, err := s.run(string(raw[:0])+"", "verify-backup", "-"); err == nil {
		t.Errorf("an empty standard input was accepted: %s", out)
	}
}

func newTargetURL(t *testing.T, base string) (string, *sql.DB, string) {
	t.Helper()
	name := "codice_cli_target_" + fmt.Sprint(time.Now().UnixNano()%1e9) + "_test"
	admin, err := sql.Open("postgres", regexp.MustCompile(`/[^/?]+\?`).ReplaceAllString(base, "/postgres?"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatal(err)
	}
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
			admin.Exec(`DROP DATABASE IF EXISTS "` + n + `" WITH (FORCE)`)
		}
		admin.Close()
	})
	return regexp.MustCompile(`/[^/?]+\?`).ReplaceAllString(base, "/"+name+"?"), admin, name
}

func TestRestore_IntoACleanDatabaseThenRefusesToReplaceItWithoutTheFlagAndTheTypedName(t *testing.T) {
	s := newSite(t)
	dir := t.TempDir()
	s.run("", "backup", "--dir", dir, "--include-files")
	files, _ := os.ReadDir(dir)
	pkg := filepath.Join(dir, files[0].Name())

	targetURL, _, targetName := newTargetURL(t, s.env.DatabaseURL)
	dest := s
	dest.env.DatabaseURL = targetURL
	dest.env.StorageRoot = t.TempDir()

	out, _, err := dest.run("", "restore", "--in", pkg)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	for _, want := range []string{"Restored in", "2 accounts", "1 brought in", "1 verified", "sign in again", "time:", "read and checked", "database", "finishing"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not say %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dest.env.StorageRoot, "A", "duna.epub")); err != nil {
		t.Error("the file did not come back")
	}
	time.Sleep(300 * time.Millisecond)

	// Now it has data: a plain restore is refused with a way forward.
	if _, _, err := dest.run("", "restore", "--in", pkg); err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("restore over data: %v", err)
	}
	// With the flag it still asks for the name; a wrong one changes nothing.
	if _, _, err := dest.run("nome-errado\n", "restore", "--in", pkg, "--overwrite"); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Errorf("a wrong confirmation: %v", err)
	}
	// The package on standard input leaves no room to ask.
	if _, _, err := dest.run("", "restore", "--in", "-", "--overwrite"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("stdin without --yes: %v", err)
	}
	out, _, err = dest.run(targetName+"\n", "restore", "--in", pkg, "--overwrite")
	if err != nil {
		t.Fatalf("with the name typed: %v", err)
	}
	if !strings.Contains(out, "was kept as") || !strings.Contains(out, "DROP DATABASE") {
		t.Errorf("the report must say where the old database went:\n%s", out)
	}
}

func TestPruneBackups_ShowsThePlanAndOnlyDeletesWithYes(t *testing.T) {
	s := newSite(t)
	dir := t.TempDir()
	for i := 0; i < 12; i++ {
		at := time.Date(2026, 9, 1+i, 3, 0, 0, 0, time.UTC)
		os.WriteFile(filepath.Join(dir, backup.PackageName(at, false)), []byte("x"), 0o600)
	}
	os.WriteFile(filepath.Join(dir, "notas.txt"), []byte("não é um pacote"), 0o600)

	out, _, err := s.run("", "prune-backups", "--dir", dir, "--daily", "3", "--weekly", "0", "--monthly", "0")
	if err != nil || !strings.Contains(out, "nothing was") || !strings.Contains(out, "would delete") {
		t.Fatalf("%v\n%s", err, out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 13 {
		t.Fatalf("the dry run deleted files: %d left", len(entries))
	}
	if out, _, err = s.run("", "prune-backups", "--dir", dir, "--daily", "3", "--weekly", "0", "--monthly", "0", "--yes"); err != nil || !strings.Contains(out, "deleted 9") {
		t.Fatalf("%v\n%s", err, out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 4 {
		t.Errorf("%d entries left, want 3 packages and the note", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, "notas.txt")); err != nil {
		t.Error("prune touched a file that is not a package")
	}
	if _, _, err := s.run("", "prune-backups"); err == nil {
		t.Error("prune without --dir was accepted")
	}
}

func TestBackupCommands_NeedTheInstallationDetails(t *testing.T) {
	s := newSite(t)
	var out bytes.Buffer
	for _, args := range [][]string{{"backup", "--out", "-"}, {"restore", "--in", "x"}} {
		if err := admincli.Run(context.Background(), s.db, args, strings.NewReader(""), &out); err == nil {
			t.Errorf("%v ran without DATABASE_URL", args)
		}
	}
}
