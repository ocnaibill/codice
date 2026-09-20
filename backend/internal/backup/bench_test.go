package backup

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

// TestBenchmark_TimesABackupAndARestoreOfASyntheticInstance is not a test: it measures. It
// only runs when CODICE_BENCH is set to "works,fileKiB,notesPerWork", for example
//
//	CODICE_BENCH=1500,128,2 go test ./internal/backup -run Benchmark -v -timeout 1h
//
// It builds an instance of that size, then times a backup with the files, a verification
// with the rehearsal, and a full restore, and prints the split by phase. The numbers depend
// on the machine and the disk: use them for their shape (what grows with what), and time the
// real instance with `codice-admin verify-backup --deep` and a rehearsal on the target.
func TestBenchmark_TimesABackupAndARestoreOfASyntheticInstance(t *testing.T) {
	spec := os.Getenv("CODICE_BENCH")
	if spec == "" {
		t.Skip("set CODICE_BENCH=works,fileKiB,notesPerWork to run")
	}
	parts := strings.Split(spec, ",")
	if len(parts) != 3 {
		t.Fatal("CODICE_BENCH must be works,fileKiB,notesPerWork")
	}
	works, _ := strconv.Atoi(parts[0])
	fileKiB, _ := strconv.Atoi(parts[1])
	notesPer, _ := strconv.Atoi(parts[2])

	s := newSource(t)
	// An empty library: newSource's fixture is only used for its plumbing.
	s.exec(`TRUNCATE works, person RESTART IDENTITY CASCADE`)
	s.must(os.RemoveAll(s.storage))
	s.must(os.MkdirAll(s.storage, 0o755))
	var ownerID string
	s.must(s.db.QueryRow(`SELECT id FROM users WHERE role = 'owner'`).Scan(&ownerID))

	content := make([]byte, fileKiB*1024)
	titles, paths, sums := make([]string, works), make([]string, works), make([]string, works)
	for i := 0; i < works; i++ {
		rand.Read(content) // different bytes per file, so the hashes are real
		h := sha256.Sum256(content)
		sums[i] = hex.EncodeToString(h[:])
		titles[i] = fmt.Sprintf("Obra %06d", i)
		paths[i] = fmt.Sprintf("Autor %04d/%s.epub", i%500, titles[i])
		full := filepath.Join(s.storage, filepath.FromSlash(paths[i]))
		s.must(os.MkdirAll(filepath.Dir(full), 0o755))
		s.must(os.WriteFile(full, content, 0o644))
	}
	s.exec(`INSERT INTO works (original_title) SELECT unnest($1::text[])`, pq.Array(titles))
	s.exec(`INSERT INTO editions (work_id, title, is_primary) SELECT id, original_title, TRUE FROM works`)
	s.exec(`INSERT INTO files (edition_id, format, sha256, size_bytes)
	        SELECT e.id, 'epub', s.sum, $3 FROM editions e JOIN unnest($1::text[], $2::text[]) AS s(title, sum) ON s.title = e.title`,
		pq.Array(titles), pq.Array(sums), fileKiB*1024)
	s.exec(`INSERT INTO storage_locations (file_id, mode, path)
	        SELECT f.id, 'managed', s.path FROM files f JOIN editions e ON e.id = f.edition_id
	        JOIN unnest($1::text[], $2::text[]) AS s(title, path) ON s.title = e.title`, pq.Array(titles), pq.Array(paths))
	s.exec(`INSERT INTO notes (user_id, work_id, quote) SELECT $1, w.id, 'nota ' || g || ' sobre ' || w.original_title
	        FROM works w, generate_series(1, $2) g`, ownerID, notesPer)

	// A backup with the files, into a file (as a real one would go).
	pkgPath := filepath.Join(t.TempDir(), "bench.tar")
	out, err := os.Create(pkgPath)
	s.must(err)
	t0 := time.Now()
	res, err := Create(ctx, CreateOptions{DB: s.db, DatabaseURL: s.dsn, StorageRoot: s.storage, Out: out, IncludeFiles: true, TmpDir: t.TempDir()})
	s.must(err)
	s.must(out.Close())
	createTime := time.Since(t0)

	open := func() *os.File {
		f, err := os.Open(pkgPath)
		s.must(err)
		return f
	}
	f := open()
	v, err := Verify(ctx, VerifyOptions{In: f, DatabaseURL: s.dsn, TmpDir: t.TempDir(), Deep: true})
	f.Close()
	s.must(err)

	tg := newTarget(t, s.dsn, true)
	f = open()
	r, err := Restore(ctx, RestoreOptions{DatabaseURL: tg.dsn, StorageRoot: tg.storage, In: f, TmpDir: t.TempDir()})
	f.Close()
	s.must(err)

	mib := func(n int64) float64 { return float64(n) / (1 << 20) }
	dump := res.Manifest.Members[MemberDump].Size
	fileBytes := int64(works) * int64(fileKiB) * 1024
	rate := func(n int64, d time.Duration) string { return fmt.Sprintf("%.0f MiB/s", mib(n)/d.Seconds()) }
	t.Logf(`
  instance   %d works, %d files of %d KiB (%.0f MiB of files), %d notes
  package    %.1f MiB (database dump %.1f MiB)
  backup     %s   (%s of package)
  verify     read+check %s (%s)   database rehearsal %s   total %s
  restore    %s = read+check %s + files %s (%s) + database %s + finish %s`,
		works, works, fileKiB, mib(fileBytes), works*notesPer,
		mib(res.Bytes), mib(dump),
		createTime.Round(time.Millisecond), rate(res.Bytes, createTime),
		v.Reading.Round(time.Millisecond), rate(v.PackageBytes, v.Reading), v.Deep.Duration.Round(time.Millisecond), (v.Reading + v.Deep.Duration).Round(time.Millisecond),
		r.Duration.Round(time.Millisecond), r.Timings.Read.Round(time.Millisecond), r.Timings.Files.Round(time.Millisecond), rate(fileBytes, r.Timings.Files),
		r.Timings.Database.Round(time.Millisecond), r.Timings.Finish.Round(time.Millisecond))
	if r.FilesVerified != works || r.FilesMissing != 0 || r.FilesMismatched != 0 {
		t.Errorf("the restore did not bring everything back: %+v", r)
	}
}
