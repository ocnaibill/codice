package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"github.com/ocnaibill/codice/backend/internal/dupes"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// fileJobHandlers are the jobs that touch the file system, run inside the API
// process through the same queue as every other job.
func fileJobHandlers(db *sql.DB, mover *storage.Mover) map[string]jobs.Handler {
	return map[string]jobs.Handler{
		// Put a freshly analysed file at its layout path.
		"organize": func(ctx context.Context, j jobs.Claimed) error {
			if j.WorkID == nil {
				return jobs.Permanent(errors.New("organize needs a work"))
			}
			err := mover.OrganizeWork(ctx, *j.WorkID)
			if errors.Is(err, storage.ErrNotMovable) || errors.Is(err, os.ErrNotExist) {
				return jobs.Permanent(err) // retrying will not bring a missing file back
			}
			return err
		},
		// Look for works that may be the same book. It only proposes.
		"dedupe": func(ctx context.Context, j jobs.Claimed) error {
			var err error
			if j.WorkID != nil {
				_, err = dupes.Detect(ctx, db, *j.WorkID)
			} else {
				_, err = dupes.DetectAll(ctx, db)
			}
			return err
		},
		// Move a referenced file into the managed storage, then remove the original
		// if it is still exactly what was copied.
		"transfer": func(ctx context.Context, j jobs.Claimed) error {
			var payload struct {
				FileID int64 `json:"file_id"`
			}
			if err := json.Unmarshal(j.Payload, &payload); err != nil || payload.FileID == 0 {
				return jobs.Permanent(errors.New("the job has no file"))
			}
			res, err := (&storage.Transferrer{Mover: mover}).MoveToManaged(ctx, payload.FileID)
			switch {
			case errors.Is(err, storage.ErrNotReferenced), errors.Is(err, storage.ErrSourceMissing),
				errors.Is(err, storage.ErrSourceChanged), errors.Is(err, storage.ErrUnsafePath):
				return jobs.Permanent(err) // retrying will not change these
			case err != nil:
				return err
			}
			log.Printf("transfer of file %d: %+v", payload.FileID, *res)
			return nil
		},
		// Catalogue an authorised directory without touching its files.
		"scan": func(ctx context.Context, j jobs.Claimed) error {
			var payload struct {
				RootID int    `json:"root_id"`
				Subdir string `json:"subdir"`
			}
			if err := json.Unmarshal(j.Payload, &payload); err != nil {
				return jobs.Permanent(err)
			}
			var root, actor string
			err := db.QueryRowContext(ctx, `
				SELECT r.path, COALESCE(j.created_by::text, '') FROM storage_roots r, jobs j WHERE r.id = $1 AND j.id = $2`,
				payload.RootID, j.ID).Scan(&root, &actor)
			if errors.Is(err, sql.ErrNoRows) {
				return jobs.Permanent(errors.New("the root is no longer authorised"))
			}
			if err != nil {
				return err
			}
			report, err := (&storage.Scanner{DB: db}).Scan(ctx, root, payload.Subdir, actor)
			if err != nil {
				if errors.Is(err, storage.ErrUnsafePath) {
					return jobs.Permanent(err)
				}
				return err
			}
			log.Printf("scan of %s: %+v", root, *report)
			return nil
		},
	}
}

// startFileJobs settles moves interrupted by a previous crash, then runs the file
// jobs in the background until ctx is cancelled.
func startFileJobs(ctx context.Context, runner *jobs.Runner, mover *storage.Mover, trash *storage.Trash) {
	if res, err := mover.Recover(ctx); err != nil {
		log.Printf("storage: could not settle interrupted moves: %v", err)
	} else if res != (storage.RecoverResult{}) {
		log.Printf("storage: settled interrupted moves: %+v", res)
	}
	go runner.Loop(ctx, 5*time.Second)
	// The automatic cleanup of the trash, when the owner turns it on. Each pass
	// deletes only what has reached its own date; with the policy off nothing has one.
	go func() {
		for {
			if n, err := trash.PurgeExpired(ctx); err != nil {
				log.Printf("trash: automatic cleanup failed: %v", err)
			} else if n > 0 {
				log.Printf("trash: automatic cleanup removed %d item(s)", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
		}
	}()
}
