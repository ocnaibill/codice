package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

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
func startFileJobs(ctx context.Context, runner *jobs.Runner, mover *storage.Mover) {
	if res, err := mover.Recover(ctx); err != nil {
		log.Printf("storage: could not settle interrupted moves: %v", err)
	} else if res != (storage.RecoverResult{}) {
		log.Printf("storage: settled interrupted moves: %+v", res)
	}
	go runner.Loop(ctx, 5*time.Second)
}
