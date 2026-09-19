package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/filecheck"
	"github.com/ocnaibill/codice/backend/internal/jobs"
)

// errTooLarge: the file exceeds the size limit.
var errTooLarge = errors.New("file is too large")

// errDuplicate carries the record that already holds the same bytes (DEC-027).
type errDuplicate struct {
	WorkID  int
	FileID  int64
	Title   string
	Retired bool
}

func (e *errDuplicate) Error() string { return "a file with identical content already exists" }

// Content validation lives in filecheck, shared with the scanner.
var errUnsupportedFormat = filecheck.ErrUnsupported

type errBadContent = filecheck.BadContent

func validateContent(path, ext string) error { return filecheck.Validate(path, ext) }

// ingestResult describes a file that was accepted.
type ingestResult struct {
	WorkID int
	JobID  int64
}

// ingestOptions tunes one ingestion.
type ingestOptions struct {
	Actor    string
	Priority int
	MaxBytes int64
}

// ingest is the single path by which a file enters the library, for uploads and
// for bulk import. It streams the bytes to a uniquely named staging file while
// hashing them, checks size and content, refuses bytes already stored, and then
// publishes the file, the work and the job together: the work, its file record
// and the job are created in one transaction (DEC-067), so a crash cannot leave
// a job without its work or a work nobody will process.
func (h *UploadHandler) ingest(ctx context.Context, src io.Reader, filename string, opts ingestOptions) (*ingestResult, error) {
	safeName := filepath.Base(filename)
	ext := strings.ToLower(filepath.Ext(safeName))
	if !SupportedFormats[ext] {
		return nil, errUnsupportedFormat
	}

	storage := resolveStoragePath()
	staging := filepath.Join(storage, ".staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(staging, "in-*"+ext)
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	published := false
	defer func() {
		tmp.Close()
		if !published {
			os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	limit := opts.MaxBytes
	size, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(src, limit+1))
	if err != nil {
		return nil, err
	}
	if size > limit {
		return nil, errTooLarge
	}
	if size == 0 {
		return nil, &errBadContent{Ext: ext, Why: "empty file"}
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := validateContent(tmpPath, ext); err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))

	if dup, err := h.findByHash(ctx, sum); err != nil {
		return nil, err
	} else if dup != nil {
		return nil, dup
	}

	// A name nobody else can have: time-independent randomness, so two uploads of
	// the same file name in the same second never collide.
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return nil, err
	}
	stored := fmt.Sprintf("%s_%s", hex.EncodeToString(rnd[:]), safeName)
	finalPath := filepath.Join(storage, stored)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, err
	}
	published = true
	cleanup := func() { os.Remove(finalPath) }

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		cleanup()
		return nil, err
	}
	defer tx.Rollback()

	var workID int
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO works (original_title, file_path, format) VALUES ($1, $2, $3) RETURNING id`,
		safeName, stored, strings.TrimPrefix(ext, ".")).Scan(&workID); err != nil {
		cleanup()
		return nil, err
	}
	// The database projects the work onto its primary edition and file (migration
	// 00002); record the facts about the bytes on that file.
	if _, err := tx.ExecContext(ctx, `
		UPDATE files SET sha256 = $1, size_bytes = $2
		WHERE id = (SELECT file_id FROM work_primary WHERE work_id = $3)`, sum, size, workID); err != nil {
		cleanup()
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23505" {
			// Another upload of the same bytes won the race.
			if dup, ferr := h.findByHash(ctx, sum); ferr == nil && dup != nil {
				return nil, dup
			}
		}
		return nil, err
	}
	jobID, err := jobs.Enqueue(ctx, tx, jobs.TypeIngest, workID, map[string]any{"file_path": mustAbs(finalPath)}, opts.Priority, opts.Actor)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return nil, err
	}

	if err := jobs.Notify(ctx, h.RedisClient, jobID); err != nil {
		// Not an error for the caller: the job waits in the database and a worker
		// picks it up when it polls.
		log.Printf("job %d saved; workers were not woken (%v)", jobID, err)
	}
	return &ingestResult{WorkID: workID, JobID: jobID}, nil
}

func mustAbs(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// findByHash returns the record already holding these bytes, if any.
func (h *UploadHandler) findByHash(ctx context.Context, sum string) (*errDuplicate, error) {
	var d errDuplicate
	err := h.DB.QueryRowContext(ctx, `
		SELECT w.id, f.id, w.original_title, w.retired_at IS NOT NULL
		FROM files f JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id
		WHERE f.sha256 = $1 LIMIT 1`, sum).Scan(&d.WorkID, &d.FileID, &d.Title, &d.Retired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}
