package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CreateFile writes a package to path and records nothing. It is written aside and renamed at the
// end, so a backup that fails leaves no half file that could be mistaken for a good one, and an
// existing file is never overwritten. The directory must exist.
func CreateFile(ctx context.Context, o CreateOptions, path string) (Result, error) {
	if _, err := os.Stat(path); err == nil {
		return Result{}, fmt.Errorf("%s already exists; it is not overwritten", path)
	}
	partial := path + ".partial"
	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	defer os.Remove(partial)
	o.Out = f
	res, err := Create(ctx, o)
	if err != nil {
		return Result{}, err
	}
	if err := f.Sync(); err != nil {
		return Result{}, err
	}
	if err := os.Rename(partial, path); err != nil {
		return Result{}, err
	}
	return res, nil
}

var (
	// ErrPanelOff: the backup buttons of the interface are not set up (DEC-123).
	ErrPanelOff = errors.New("backups from the panel are not set up: CODICE_BACKUP_DIR and CODICE_BACKUP_PASSPHRASE_FILE are needed")
	// ErrNoPassphrase: the file that holds the passphrase is missing, empty or too short.
	ErrNoPassphrase = fmt.Errorf("the passphrase file is missing, unreadable or shorter than %d characters", MinPassphrase)
	// ErrNotAPackage: the name is not one of the packages in the backup folder.
	ErrNotAPackage = errors.New("that is not a package in the backup folder")
)

// Panel is what the owner's buttons run (DEC-123): make a package, and check one. The passphrase
// is read from a file on the server and never passes through the browser; a package made here is
// always encrypted, and goes only to Dir, which is fixed by whoever installs the server, never by a
// request. A restore is not here, and neither is a download: they stay with codice-admin.
type Panel struct {
	DB             *sql.DB
	DatabaseURL    string
	StorageRoot    string
	Dir            string
	PassphraseFile string
	TmpDir         string
}

// Enabled says whether the buttons can work at all.
func (p Panel) Enabled() bool { return p.Dir != "" && p.PassphraseFile != "" }

// Validate is the check made at startup: the buttons need both settings or neither, and the folder is an
// absolute path, because a relative one would be read from wherever the server happened to start. It
// returns a warning for the half-set case (the buttons stay off) and an error for the one that stops the
// server.
func (p Panel) Validate() (warning string, err error) {
	if (p.Dir == "") != (p.PassphraseFile == "") {
		return "Backups from the panel are OFF: they need both CODICE_BACKUP_DIR and CODICE_BACKUP_PASSPHRASE_FILE", nil
	}
	if p.Enabled() && !filepath.IsAbs(p.Dir) {
		return "", errors.New("CODICE_BACKUP_DIR must be an absolute path")
	}
	return "", nil
}

func (p Panel) passphrase() (string, error) {
	raw, err := os.ReadFile(p.PassphraseFile)
	if err != nil {
		return "", ErrNoPassphrase
	}
	pass := strings.TrimRight(string(raw), "\r\n")
	if len(pass) < MinPassphrase {
		return "", ErrNoPassphrase
	}
	return pass, nil
}

// Make makes one encrypted package, with the files, in Dir, and records it for the interface.
func (p Panel) Make(ctx context.Context) (string, Result, error) {
	if !p.Enabled() {
		return "", Result{}, ErrPanelOff
	}
	pass, err := p.passphrase()
	if err != nil {
		return "", Result{}, err
	}
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return "", Result{}, err
	}
	path := filepath.Join(p.Dir, PackageName(time.Now(), true))
	res, err := CreateFile(ctx, CreateOptions{
		DB: p.DB, DatabaseURL: p.DatabaseURL, StorageRoot: p.StorageRoot,
		IncludeFiles: true, Passphrase: pass, TmpDir: p.TmpDir,
	}, path)
	if err != nil {
		return "", Result{}, err
	}
	if err := Record(ctx, p.DB, res, path); err != nil {
		// The package exists and is good; only the note for the interface is missing.
		return path, res, fmt.Errorf("the package was made but could not be recorded: %w", err)
	}
	return path, res, nil
}

// PackageInfo is one package in the backup folder.
type PackageInfo struct {
	Name      string    `json:"name"`
	Bytes     int64     `json:"bytes"`
	At        time.Time `json:"at"`
	Encrypted bool      `json:"encrypted"`
}

// Packages lists the packages in Dir, newest first. Only files named like packages are listed.
func (p Panel) Packages() ([]PackageInfo, error) {
	if p.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(p.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []PackageInfo
	for _, e := range entries {
		m := packageRe.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		at, err := time.Parse("20060102-150405", m[1])
		info, ierr := e.Info()
		if err != nil || ierr != nil {
			continue
		}
		out = append(out, PackageInfo{Name: e.Name(), Bytes: info.Size(), At: at.UTC(), Encrypted: m[2] != ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}

// Check reads the package called name end to end and rehearses its restore in a temporary database
// (verify-backup --deep), and records which package was checked. The name must be one of the
// packages Packages lists: a request never names a path.
func (p Panel) Check(ctx context.Context, name string) (VerifyResult, error) {
	if !p.Enabled() {
		return VerifyResult{}, ErrPanelOff
	}
	if !packageRe.MatchString(name) {
		return VerifyResult{}, ErrNotAPackage
	}
	path := filepath.Join(p.Dir, name)
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return VerifyResult{}, ErrNotAPackage
	}
	var pass string
	if strings.HasSuffix(name, ".age") {
		var err error
		if pass, err = p.passphrase(); err != nil {
			return VerifyResult{}, err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return VerifyResult{}, err
	}
	defer f.Close()
	res, err := Verify(ctx, VerifyOptions{In: f, Passphrase: pass, TmpDir: p.TmpDir, DatabaseURL: p.DatabaseURL, Deep: true})
	if err != nil {
		return VerifyResult{}, err
	}
	return res, RecordVerified(ctx, p.DB, name, res.Deep != nil)
}
