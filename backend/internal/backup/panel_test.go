package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func panelFor(s *source, dir, passFile string) Panel {
	return Panel{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Dir: dir, PassphraseFile: passFile, TmpDir: s.t.TempDir()}
}

func writePass(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frase")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateFile_WritesPrivatelyAndNeverOverwrites(t *testing.T) {
	s := newSource(t)
	path := filepath.Join(t.TempDir(), "p.tar")
	if _, err := CreateFile(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, TmpDir: s.t.TempDir()}, path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 || info.Size() == 0 {
		t.Fatalf("package: %v %v", info, err)
	}
	if _, err := os.Stat(path + ".partial"); err == nil {
		t.Error("the aside file was left behind")
	}
	if _, err := CreateFile(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage}, path); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("an existing package: %v", err)
	}
}

func TestCreateFile_AFailedBackupLeavesNothing(t *testing.T) {
	s := newSource(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "p.tar")
	_, err := CreateFile(ctx, CreateOptions{DB: s.db, DatabaseURL: "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable", StorageRoot: s.storage, TmpDir: s.t.TempDir()}, path)
	if err == nil {
		t.Fatal("a backup of a database that does not answer was accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}

func TestPanel_IsOffUnlessBothTheFolderAndThePassphraseFileAreGiven(t *testing.T) {
	for _, p := range []Panel{{}, {Dir: "/b"}, {PassphraseFile: "/f"}} {
		if p.Enabled() {
			t.Errorf("%+v must be off", p)
		}
	}
	if !(Panel{Dir: "/b", PassphraseFile: "/f"}).Enabled() {
		t.Error("both given: it must be on")
	}
	if _, _, err := (Panel{}).Make(ctx); err != ErrPanelOff {
		t.Errorf("Make when off: %v", err)
	}
	if _, err := (Panel{}).Check(ctx, "codice-backup-20261004-030000.tar"); err != ErrPanelOff {
		t.Errorf("Check when off: %v", err)
	}
}

func TestPanelMake_MakesAnEncryptedPackageInTheFolderAndRecordsIt(t *testing.T) {
	s := newSource(t)
	dir := filepath.Join(t.TempDir(), "backups")
	p := panelFor(s, dir, writePass(t, "uma frase longa e boa\n"))

	path, res, err := p.Make(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir || !packageRe.MatchString(filepath.Base(path)) || !strings.HasSuffix(path, ".tar.age") {
		t.Fatalf("path = %s", path)
	}
	if !res.Encrypted || !res.Manifest.IncludesFiles {
		t.Errorf("a package made from the panel is encrypted and has the files: %+v", res)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Errorf("the folder is %v, want 0700", info.Mode().Perm())
	}
	last, _ := Last(ctx, s.db)
	if last == nil || last.Path != path || last.Name != filepath.Base(path) || !last.Encrypted {
		t.Errorf("last = %+v", last)
	}
	// And it opens with the passphrase, and not with another.
	f, _ := os.Open(path)
	defer f.Close()
	if _, err := Verify(ctx, VerifyOptions{In: f, Passphrase: "uma frase longa e boa"}); err != nil {
		t.Errorf("verify with the passphrase: %v", err)
	}
}

func TestPanelMake_NeedsAUsablePassphraseAndLeavesNothingWithoutOne(t *testing.T) {
	s := newSource(t)
	for name, file := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "nao-existe"),
		"empty":   writePass(t, ""),
		"short":   writePass(t, "curta\n"),
	} {
		dir := filepath.Join(t.TempDir(), "backups")
		if _, _, err := panelFor(s, dir, file).Make(ctx); err != ErrNoPassphrase {
			t.Errorf("%s: %v, want ErrNoPassphrase", name, err)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%s: the folder was made although nothing could be written", name)
		}
	}
}

func TestPanelPackages_ListsOnlyPackagesNewestFirst(t *testing.T) {
	s := newSource(t)
	dir := t.TempDir()
	for _, n := range []string{
		"codice-backup-20261001-030000.tar", "codice-backup-20261003-030000.tar.age", "codice-backup-20261002-030000.tar.age",
		"codice-backup-20261004-030000.tar.age.partial", "notas.txt", "codice-backup-2026.tar", "x.tar.age",
	} {
		os.WriteFile(filepath.Join(dir, n), []byte("0123456789"), 0o600)
	}
	os.Mkdir(filepath.Join(dir, "codice-backup-20261005-030000.tar"), 0o700) // a folder, not a package
	got, err := panelFor(s, dir, "x").Packages()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	want := []string{"codice-backup-20261003-030000.tar.age", "codice-backup-20261002-030000.tar.age", "codice-backup-20261001-030000.tar"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("packages = %v, want %v", names, want)
	}
	if got[0].Bytes != 10 || !got[0].Encrypted || got[2].Encrypted || !got[0].At.Equal(time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("details: %+v", got)
	}
	if none, err := panelFor(s, filepath.Join(dir, "nao-existe"), "x").Packages(); err != nil || len(none) != 0 {
		t.Errorf("a folder that is not there has no packages: %v %v", none, err)
	}
	if none, _ := (Panel{}).Packages(); none != nil {
		t.Errorf("no folder given: %v", none)
	}
}

func TestPanelCheck_RehearsesTheRestoreAndRecordsWhichPackage(t *testing.T) {
	s := newSource(t)
	dir := filepath.Join(t.TempDir(), "backups")
	p := panelFor(s, dir, writePass(t, "uma frase longa e boa"))
	path, _, err := p.Make(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	res, err := p.Check(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deep == nil {
		t.Error("the check must rehearse the restore, not only read the package")
	}
	v, _ := LastVerified(ctx, s.db)
	if v == nil || v.Name != name || !v.Deep {
		t.Errorf("verified = %+v", v)
	}
}

func TestPanelCheck_OnlyChecksPackagesOfTheFolderByName(t *testing.T) {
	s := newSource(t)
	dir := filepath.Join(t.TempDir(), "backups")
	os.MkdirAll(dir, 0o700)
	outside := filepath.Join(filepath.Dir(dir), "codice-backup-20261001-030000.tar")
	os.WriteFile(outside, []byte("x"), 0o600)
	p := panelFor(s, dir, writePass(t, "uma frase longa e boa"))
	for _, name := range []string{
		"", "../codice-backup-20261001-030000.tar", outside, "backups/../codice-backup-20261001-030000.tar",
		"codice-backup-20261001-030000.tar/../x", "x.tar", "codice-backup-20261001-030000.tar", // the last one is not there
	} {
		if _, err := p.Check(ctx, name); err != ErrNotAPackage {
			t.Errorf("Check(%q) = %v, want ErrNotAPackage", name, err)
		}
	}
	// A link to a file elsewhere is not a package of the folder.
	os.Symlink(outside, filepath.Join(dir, "codice-backup-20261002-030000.tar"))
	if _, err := p.Check(ctx, "codice-backup-20261002-030000.tar"); err != ErrNotAPackage {
		t.Errorf("a symlink: %v", err)
	}
	if v, _ := LastVerified(ctx, s.db); v != nil {
		t.Errorf("nothing was checked: %+v", v)
	}
}

func TestPanelCheck_ADamagedPackageFailsAndIsNotRecorded(t *testing.T) {
	s := newSource(t)
	dir := filepath.Join(t.TempDir(), "backups")
	p := panelFor(s, dir, writePass(t, "uma frase longa e boa"))
	path, _, err := p.Make(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	raw[len(raw)/2] ^= 0xff
	os.WriteFile(path, raw, 0o600)
	if _, err := p.Check(ctx, filepath.Base(path)); err == nil {
		t.Fatal("a damaged package passed")
	}
	if v, _ := LastVerified(ctx, s.db); v != nil {
		t.Errorf("a failed check was recorded: %+v", v)
	}
}

func TestPanelCheck_AnEncryptedPackageNeedsThePassphraseFile(t *testing.T) {
	s := newSource(t)
	dir := filepath.Join(t.TempDir(), "backups")
	pass := writePass(t, "uma frase longa e boa")
	path, _, err := panelFor(s, dir, pass).Make(ctx)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(pass)
	if _, err := panelFor(s, dir, pass).Check(ctx, filepath.Base(path)); err != ErrNoPassphrase {
		t.Errorf("without the passphrase: %v", err)
	}
	other := writePass(t, "outra frase, bem diferente")
	if _, err := panelFor(s, dir, other).Check(ctx, filepath.Base(path)); err == nil {
		t.Error("a wrong passphrase passed")
	}
}

func TestPanelMake_AcceptsAPassphraseOfExactlyTheMinimumLength(t *testing.T) {
	s := newSource(t)
	if MinPassphrase != 8 {
		t.Skip("the test writes 8 characters")
	}
	dir := filepath.Join(t.TempDir(), "backups")
	if _, _, err := panelFor(s, dir, writePass(t, "12345678\n")).Make(ctx); err != nil {
		t.Errorf("8 characters are the minimum and must do: %v", err)
	}
	if _, _, err := panelFor(s, dir, writePass(t, "1234567\n")).Make(ctx); err != ErrNoPassphrase {
		t.Errorf("7 characters: %v, want ErrNoPassphrase", err)
	}
}

func TestPanelValidate_NeedsBothSettingsOrNeitherAndAnAbsoluteFolder(t *testing.T) {
	cases := []struct {
		name     string
		p        Panel
		warning  bool
		fatalErr bool
	}{
		{"neither", Panel{}, false, false},
		{"both, absolute", Panel{Dir: "/backups", PassphraseFile: "/app/ldap/frase"}, false, false},
		{"only the folder", Panel{Dir: "/backups"}, true, false},
		{"only the passphrase", Panel{PassphraseFile: "/app/ldap/frase"}, true, false},
		{"both, relative folder", Panel{Dir: "backups", PassphraseFile: "/app/ldap/frase"}, false, true},
	}
	for _, c := range cases {
		warning, err := c.p.Validate()
		if (warning != "") != c.warning || (err != nil) != c.fatalErr {
			t.Errorf("%s: warning %q, err %v", c.name, warning, err)
		}
	}
}
