package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// fileJobHandlers are the jobs that touch the file system, run inside the API
// process through the same queue as every other job.
func fileJobHandlers(mover *storage.Mover) map[string]jobs.Handler {
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
