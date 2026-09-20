package admincli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ocnaibill/codice/backend/internal/backup"
)

func needInstall(env Env) error {
	if env.DatabaseURL == "" {
		return errors.New("the backup commands need DATABASE_URL")
	}
	return nil
}

// passphrase reads the passphrase from a file, or from CODICE_BACKUP_PASSPHRASE. It is
// never taken from a command-line argument, where the process list would show it.
func passphrase(env Env, file string) (string, error) {
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading the passphrase file: %w", err)
		}
		return strings.TrimRight(string(raw), "\r\n"), nil
	}
	return env.getenv("CODICE_BACKUP_PASSPHRASE"), nil
}

func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func backupCmd(ctx context.Context, db *sql.DB, env Env, args []string, out io.Writer) error {
	fs := flags("backup", out)
	dir := fs.String("dir", "", "directory for the package (named codice-backup-<time>.tar)")
	outPath := fs.String("out", "", "file for the package, or - for standard output")
	include := fs.Bool("include-files", false, "also put the managed files and covers in the package")
	passFile := fs.String("passphrase-file", "", "file holding the passphrase that encrypts the package")
	tmp := fs.String("tmp-dir", env.TmpDir, "where the dump waits while the package is assembled")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := needInstall(env); err != nil {
		return err
	}
	if (*dir == "") == (*outPath == "") {
		return errors.New("give exactly one of --dir DIR, --out FILE or --out - (a package is never written to the terminal by accident)")
	}
	pass, err := passphrase(env, *passFile)
	if err != nil {
		return err
	}

	stream := *outPath == "-"
	msg := out
	var sink io.Writer = out
	var path, partial string
	if stream {
		msg = env.Stderr
		if msg == nil {
			msg = io.Discard
		}
	} else {
		if *dir != "" {
			if err := os.MkdirAll(*dir, 0o700); err != nil {
				return err
			}
			path = filepath.Join(*dir, backup.PackageName(time.Now(), pass != ""))
		} else {
			path = *outPath
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; it is not overwritten", path)
		}
		// Written aside and renamed at the end: a backup that fails leaves no half file that
		// could be mistaken for a good one.
		partial = path + ".partial"
		f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		defer os.Remove(partial)
		sink = f
	}

	res, err := backup.Create(ctx, backup.CreateOptions{
		DB: db, DatabaseURL: env.DatabaseURL, StorageRoot: env.StorageRoot, Out: sink,
		IncludeFiles: *include, Passphrase: pass, TmpDir: *tmp,
	})
	if err != nil {
		return err
	}
	if !stream {
		if err := sink.(*os.File).Sync(); err != nil {
			return err
		}
		if err := os.Rename(partial, path); err != nil {
			return err
		}
	}
	if rerr := backup.Record(ctx, db, res); rerr != nil {
		fmt.Fprintf(msg, "Warning: the backup was made but could not be recorded: %v\n", rerr)
	}

	m := res.Manifest
	fmt.Fprintf(msg, "Backup made in %s: %s", res.Duration.Round(time.Millisecond), human(res.Bytes))
	if path != "" {
		fmt.Fprintf(msg, " -> %s", path)
	}
	fmt.Fprintf(msg, "\n  database: schema %d, %d accounts, %d works, %d notes\n", m.SchemaVersion, m.Counts.Users, m.Counts.Works, m.Counts.Notes)
	fmt.Fprintf(msg, "  files: %d listed with their hashes (%s)", m.Files.Total, human(m.Files.Bytes))
	if m.IncludesFiles {
		fmt.Fprintf(msg, ", %d included in the package, %d missing on disk", m.Files.Included, m.Files.Missing)
	} else {
		fmt.Fprint(msg, "; the files themselves are NOT in the package (use --include-files, or back them up with your own tool)")
	}
	fmt.Fprintln(msg)
	if res.Encrypted {
		fmt.Fprintln(msg, "  encrypted: yes. Without the passphrase this package cannot be opened; keep it off this server.")
	} else {
		fmt.Fprintln(msg, "  encrypted: NO. It holds password hashes and every person's notes: protect it, or set a passphrase.")
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(msg, "  warning: %s\n", w)
	}
	return nil
}

func openInput(path string, in io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		return in, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

func verifyCmd(ctx context.Context, env Env, args []string, in io.Reader, out io.Writer) error {
	fs := flags("verify-backup", out)
	deep := fs.Bool("deep", false, "also rehearse the restore in a temporary database")
	passFile := fs.String("passphrase-file", "", "file holding the passphrase, if the package is encrypted")
	tmp := fs.String("tmp-dir", env.TmpDir, "where the dump waits while the package is read")
	// The file may come before or after the flags.
	var files []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		args = fs.Args()
		if len(args) > 0 {
			files = append(files, args[0])
			args = args[1:]
		}
	}
	if len(files) != 1 {
		return errors.New("give the package to verify (a file, or - for standard input)")
	}
	pass, err := passphrase(env, *passFile)
	if err != nil {
		return err
	}
	if *deep {
		if err := needInstall(env); err != nil {
			return err
		}
	}
	r, closeIn, err := openInput(files[0], in)
	if err != nil {
		return err
	}
	defer closeIn()

	res, err := backup.Verify(ctx, backup.VerifyOptions{In: r, Passphrase: pass, TmpDir: *tmp, DatabaseURL: env.DatabaseURL, Deep: *deep})
	if err != nil {
		return err
	}
	m := res.Manifest
	fmt.Fprintf(out, "The package is intact: %s read and checked in %s (%s/s).\n", human(res.PackageBytes), res.Reading.Round(time.Millisecond), human(rate(res.PackageBytes, res.Reading)))
	fmt.Fprintf(out, "  made: %s, schema %d, PostgreSQL %s%s\n", m.CreatedAt.Format(time.RFC3339), m.SchemaVersion, m.ServerVersion, map[bool]string{true: ", encrypted", false: ""}[res.Encrypted])
	fmt.Fprintf(out, "  database: %d accounts, %d works, %d notes; the dump lists %d objects\n", m.Counts.Users, m.Counts.Works, m.Counts.Notes, res.DumpEntries)
	fmt.Fprintf(out, "  files: %d listed", m.Files.Total)
	if m.IncludesFiles {
		fmt.Fprintf(out, ", %d included", m.Files.Included)
	}
	fmt.Fprintln(out)
	if res.Deep != nil {
		fmt.Fprintf(out, "Restore rehearsal: restored into a temporary database in %s and it matches (%d accounts, %d works, %d notes). The temporary database was dropped.\n",
			res.Deep.Duration.Round(time.Millisecond), res.Deep.Restored.Users, res.Deep.Restored.Works, res.Deep.Restored.Notes)
		fmt.Fprintf(out, "  Reading the package plus the database restore: about %s. The files are not part of this rehearsal: see \"ensaio de restauração\" in the README to time them.\n",
			(res.Reading + res.Deep.Duration).Round(time.Millisecond))
	}
	return nil
}

func restoreCmd(ctx context.Context, env Env, args []string, in io.Reader, out io.Writer) error {
	fs := flags("restore", out)
	inPath := fs.String("in", "", "the package to restore (a file, or - for standard input)")
	overwrite := fs.Bool("overwrite", false, "allow replacing a database that already holds data (it is kept, renamed)")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	skipHash := fs.Bool("skip-hash", false, "check that files exist and have the right size, without reading them all")
	passFile := fs.String("passphrase-file", "", "file holding the passphrase, if the package is encrypted")
	tmp := fs.String("tmp-dir", env.TmpDir, "where the dump waits while the package is read")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := needInstall(env); err != nil {
		return err
	}
	if *inPath == "" {
		return errors.New("--in is required")
	}
	if env.StorageRoot == "" {
		return errors.New("the storage directory is unknown: set CODICE_STORAGE_PATH")
	}
	pass, err := passphrase(env, *passFile)
	if err != nil {
		return err
	}
	u, err := url.Parse(env.DatabaseURL)
	if err != nil {
		return errors.New("DATABASE_URL is not valid")
	}
	dbName := strings.TrimPrefix(u.Path, "/")

	if *overwrite && !*yes {
		if *inPath == "-" {
			return errors.New("with the package on standard input there is no way to ask: add --yes")
		}
		fmt.Fprintf(out, "This replaces the database %q with the package. The current one is kept under another name.\n", dbName)
		fmt.Fprintf(out, "Stop the API and the worker first. Type the database name %q to confirm: ", dbName)
		line, _ := readLine(in)
		if strings.TrimSpace(line) != dbName {
			return errors.New("not confirmed; nothing was changed")
		}
	}

	r, closeIn, err := openInput(*inPath, in)
	if err != nil {
		return err
	}
	defer closeIn()
	res, err := backup.Restore(ctx, backup.RestoreOptions{
		DatabaseURL: env.DatabaseURL, StorageRoot: env.StorageRoot, In: r, Passphrase: pass, TmpDir: *tmp,
		Overwrite: *overwrite, SkipHash: *skipHash,
	})
	if err != nil {
		if errors.Is(err, backup.ErrNotEmpty) {
			return fmt.Errorf("%w: nothing was changed. To replace it, add --overwrite (the current database is kept)", err)
		}
		return err
	}

	m := res.Manifest
	fmt.Fprintf(out, "Restored in %s from a package made %s.\n", res.Duration.Round(time.Millisecond), m.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "  database: %d accounts, %d works, %d notes (schema brought to this version)\n", m.Counts.Users, m.Counts.Works, m.Counts.Notes)
	fmt.Fprintf(out, "  files: %d brought in, %d were already there, %d verified", res.FilesRestored, res.FilesSkipped, res.FilesVerified)
	if res.FilesMismatched > 0 {
		fmt.Fprintf(out, ", %d with DIFFERENT content", res.FilesMismatched)
	}
	if res.FilesMissing > 0 {
		fmt.Fprintf(out, ", %d MISSING (marked as missing in the library)", res.FilesMissing)
	}
	if res.Referenced > 0 {
		fmt.Fprintf(out, "; %d live in your own folders and were not checked", res.Referenced)
	}
	fmt.Fprintln(out)
	if res.KeptDatabase != "" {
		fmt.Fprintf(out, "  the previous database was kept as %q. When you are sure, drop it with: DROP DATABASE %q;\n", res.KeptDatabase, res.KeptDatabase)
	}
	t := res.Timings
	fmt.Fprintf(out, "  time: %s read and checked (%s, %s/s), %s files, %s database, %s finishing\n",
		t.Read.Round(time.Millisecond), human(res.PackageBytes), human(rate(res.PackageBytes, t.Read)),
		t.Files.Round(time.Millisecond), t.Database.Round(time.Millisecond), t.Finish.Round(time.Millisecond))
	fmt.Fprintln(out, "Everyone has to sign in again, and app tokens and invitations must be issued again: none of them travel in a package.")
	fmt.Fprintln(out, "Now start the API and the worker; the owner will see a notice that the instance was restored.")
	return nil
}

func readLine(in io.Reader) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return b.String(), nil
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			return b.String(), err
		}
	}
}

func pruneCmd(args []string, out io.Writer) error {
	fs := flags("prune-backups", out)
	dir := fs.String("dir", "", "directory holding the packages")
	d := fs.Int("daily", backup.DefaultPolicy.Daily, "daily packages to keep")
	w := fs.Int("weekly", backup.DefaultPolicy.Weekly, "weekly packages to keep")
	m := fs.Int("monthly", backup.DefaultPolicy.Monthly, "monthly packages to keep")
	yes := fs.Bool("yes", false, "actually delete (without it, only the plan is shown)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("--dir is required")
	}
	if *d < 0 || *w < 0 || *m < 0 {
		return errors.New("the numbers to keep cannot be negative")
	}
	res, err := backup.Prune(*dir, backup.Policy{Daily: *d, Weekly: *w, Monthly: *m}, !*yes)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Keeping %d package(s); ", len(res.Keep))
	if *yes {
		fmt.Fprintf(out, "deleted %d.\n", len(res.Delete))
	} else {
		fmt.Fprintf(out, "%d would be deleted (nothing was: add --yes).\n", len(res.Delete))
	}
	for _, n := range res.Delete {
		fmt.Fprintf(out, "  %s %s\n", map[bool]string{true: "deleted", false: "would delete"}[*yes], n)
	}
	return nil
}

// rate is bytes per second, guarding against a zero duration on tiny inputs.
func rate(n int64, d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64(float64(n) / d.Seconds())
}
