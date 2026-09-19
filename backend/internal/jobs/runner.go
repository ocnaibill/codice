package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lib/pq"
)

// Claimed is a job handed to a handler.
type Claimed struct {
	ID       int64
	Type     string
	WorkID   *int
	Payload  json.RawMessage
	Attempts int
}

// Handler does the work of one job type. Returning nil completes the job.
// Return Permanent(err) for a failure that will happen again however often it
// is retried; any other error is treated as temporary. The context is cancelled
// when an admin cancels the job or the job's lease is lost.
type Handler func(ctx context.Context, job Claimed) error

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent marks an error as one that retrying cannot fix (DEC-068).
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

// Runner takes jobs of some types from the queue and runs them in this process,
// through the same SQL functions the Python worker uses. It is how the API
// process does file-system work (organizing, scanning, transferring) with the
// same leases, retries and visibility as any other job.
type Runner struct {
	DB           *sql.DB
	Owner        string
	Types        []string
	MaxRunning   int
	LeaseSeconds int
	Handlers     map[string]Handler
	Heartbeat    time.Duration // 0 means a quarter of the lease
}

// NewOwnerName is unique per process.
func NewOwnerName(prefix string) string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%s-%d-%d", prefix, host, os.Getpid(), time.Now().UnixNano()%1_000_000)
}

func (r *Runner) heartbeatEvery() time.Duration {
	if r.Heartbeat > 0 {
		return r.Heartbeat
	}
	return time.Duration(r.LeaseSeconds) * time.Second / 4
}

// RunOnce takes and runs one job. It reports whether a job was taken.
func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	var j Claimed
	var workID sql.NullInt64
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, type, work_id, payload, attempts FROM jobs_claim($1, $2, $3, $4)`,
		r.Owner, r.LeaseSeconds, r.MaxRunning, pq.Array(r.Types)).Scan(&j.ID, &j.Type, &workID, &j.Payload, &j.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if workID.Valid {
		id := int(workID.Int64)
		j.WorkID = &id
	}

	handler := r.Handlers[j.Type]
	if handler == nil {
		r.fail(ctx, j.ID, "permanent", "no handler for job type "+j.Type)
		return true, nil
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lost := make(chan struct{})
	cancelled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(r.heartbeatEvery())
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				var status string
				if err := r.DB.QueryRowContext(ctx, `SELECT jobs_heartbeat($1, $2, $3)`, j.ID, r.Owner, r.LeaseSeconds).Scan(&status); err != nil {
					log.Printf("jobs: heartbeat of %d failed: %v", j.ID, err)
					continue
				}
				switch status {
				case "cancel":
					close(cancelled)
					cancel()
					return
				case "lost":
					close(lost)
					cancel()
					return
				}
			}
		}
	}()

	herr := handler(jobCtx, j)
	close(done)

	select {
	case <-lost:
		log.Printf("jobs: %d was taken over by another worker; stopping", j.ID)
		return true, nil
	case <-cancelled:
		r.DB.ExecContext(ctx, `SELECT jobs_cancel_ack($1, $2)`, j.ID, r.Owner)
		return true, nil
	default:
	}

	if herr == nil {
		var ok bool
		if err := r.DB.QueryRowContext(ctx, `SELECT jobs_complete($1, $2)`, j.ID, r.Owner).Scan(&ok); err != nil {
			log.Printf("jobs: could not complete %d: %v", j.ID, err)
		}
		return true, nil
	}
	kind := "temporary"
	var perm permanentError
	if errors.As(herr, &perm) {
		kind = "permanent"
	}
	r.fail(ctx, j.ID, kind, herr.Error())
	return true, nil
}

func (r *Runner) fail(ctx context.Context, id int64, kind, msg string) {
	var outcome string
	if err := r.DB.QueryRowContext(ctx, `SELECT jobs_fail($1, $2, $3, $4)`, id, r.Owner, kind, msg).Scan(&outcome); err != nil {
		log.Printf("jobs: could not report the failure of %d: %v", id, err)
		return
	}
	log.Printf("jobs: %d failed (%s, %s): %s", id, kind, outcome, msg)
}

// Loop runs jobs until ctx is cancelled, looking again at once after each job
// and sleeping for interval when there is nothing to do.
func (r *Runner) Loop(ctx context.Context, interval time.Duration) {
	for ctx.Err() == nil {
		took, err := r.RunOnce(ctx)
		if err != nil {
			log.Printf("jobs: queue unreachable: %v", err)
		}
		if took && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
