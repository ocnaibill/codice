// Package jobs enqueues and administers asynchronous work. The state machine
// itself (claim, heartbeat, complete, fail with retries) lives in PostgreSQL
// functions (migration 00005) so every worker shares one tested implementation.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Job types.
const TypeIngest = "ingest" // analyze a file: extract metadata, cover, pages

// TypeProviderTest asks one metadata provider a fixed, public question to see whether it answers (DEC-145).
const TypeProviderTest = "provider_test"

// Priorities: manual work is served before batch scans (DEC-069).
const (
	PriorityBatch  = 0
	PriorityManual = 10
)

// WakeupStream is the Redis stream workers block on. It only carries "there may
// be something to do"; losing it costs at most one polling interval.
const WakeupStream = "codice_jobs_wakeup"

// Execer is satisfied by *sql.DB and *sql.Tx, so a job can be created in the same
// transaction as the record it works on (DEC-067).
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Enqueue creates a pending job. Call it inside the transaction that creates the
// work, so a crash between the two can neither lose the job nor leave a job
// without its work. Enqueueing again while one is live for the same work is a
// no-op that returns the existing job id.
func Enqueue(ctx context.Context, q Execer, jobType string, workID int, payload map[string]any, priority int, createdBy string) (int64, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = q.QueryRowContext(ctx, `
		INSERT INTO jobs (type, work_id, payload, priority, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid)
		ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO UPDATE SET updated_at = now()
		RETURNING id`, jobType, workID, body, priority, createdBy).Scan(&id)
	if err != nil {
		return 0, err
	}
	if jobType == TypeIngest {
		if _, err := q.ExecContext(ctx, `UPDATE works SET media_status = 'QUEUED' WHERE id = $1 AND media_status IN ('UNKNOWN', 'ERROR')`, workID); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// Notify wakes the workers. It is best effort and must be called after the
// transaction commits: if Redis is down the job simply waits in the database.
func Notify(ctx context.Context, rdb *redis.Client, jobID int64) error {
	if rdb == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: WakeupStream, MaxLen: 1000, Approx: true,
		Values: map[string]any{"job_id": jobID},
	}).Err()
}

// Job is the administrative view of a job.
type Job struct {
	ID         int64      `json:"id"`
	Type       string     `json:"type"`
	WorkID     *int       `json:"workId"`
	WorkTitle  string     `json:"workTitle,omitempty"`
	State      string     `json:"state"`
	Priority   int        `json:"priority"`
	Attempts   int        `json:"attempts"`
	MaxTries   int        `json:"maxAttempts"`
	RunAt      time.Time  `json:"runAt"`
	Cancelling bool       `json:"cancelRequested"`
	LastError  string     `json:"lastError,omitempty"`
	ErrorKind  string     `json:"errorKind,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

var (
	ErrNotFound = errors.New("job not found")
	ErrState    = errors.New("job is not in a state that allows this")
)

// List returns jobs newest first, optionally of one state and one type, with the count of
// every state for the summary (of all jobs, whatever the filter).
func List(ctx context.Context, db *sql.DB, state, jobType string, limit int) ([]Job, map[string]int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT j.id, j.type, j.work_id, COALESCE(w.original_title, ''), j.state, j.priority, j.attempts,
		       j.max_attempts, j.run_at, j.cancel_requested, COALESCE(j.last_error, ''), COALESCE(j.error_kind, ''),
		       j.created_at, j.started_at, j.finished_at
		FROM jobs j LEFT JOIN works w ON w.id = j.work_id
		WHERE ($1 = '' OR j.state = $1) AND ($2 = '' OR j.type = $2)
		ORDER BY j.id DESC LIMIT $3`, state, jobType, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		var workID sql.NullInt64
		if err := rows.Scan(&j.ID, &j.Type, &workID, &j.WorkTitle, &j.State, &j.Priority, &j.Attempts, &j.MaxTries,
			&j.RunAt, &j.Cancelling, &j.LastError, &j.ErrorKind, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
			return nil, nil, err
		}
		if workID.Valid {
			id := int(workID.Int64)
			j.WorkID = &id
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	counts := map[string]int{"pending": 0, "running": 0, "succeeded": 0, "failed": 0, "cancelled": 0}
	crow, err := db.QueryContext(ctx, `SELECT state, count(*) FROM jobs GROUP BY state`)
	if err != nil {
		return nil, nil, err
	}
	defer crow.Close()
	for crow.Next() {
		var s string
		var n int
		if err := crow.Scan(&s, &n); err != nil {
			return nil, nil, err
		}
		counts[s] = n
	}
	return out, counts, crow.Err()
}

// Rerun puts a failed or cancelled job back in the queue with a fresh set of attempts.
func Rerun(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `
		UPDATE jobs SET state = 'pending', attempts = 0, run_at = now(), cancel_requested = FALSE,
		       last_error = NULL, error_kind = NULL, finished_at = NULL, updated_at = now()
		WHERE id = $1 AND state IN ('failed', 'cancelled')`, id)
	return affected(ctx, db, res, err, id)
}

// RerunFailed puts every failed job of a work back in the queue, with a fresh set of attempts: the one button for "try
// them all again" after a bad hour (a folder that was not mounted, a service that was down). It answers with the ids it
// requeued and how many failed jobs it left as they are, which are the ones it must not touch: a job without a work
// (a scan, a backup: those are looked at one by one), and one whose work already has a live job of the same kind
// (asking twice is not a way to run it twice, DEC-069). When a work has several failed jobs of one kind, the newest is
// the one that returns. An ingestion keeps the path the file had when it was created, and the file may have moved since,
// so the paths are refreshed from where the files are now, as for one job.
func RerunFailed(ctx context.Context, db *sql.DB, storagePath string) (requeued []int64, left int, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE jobs j SET payload = jsonb_set(j.payload, '{file_path}', to_jsonb(
		       CASE WHEN l.mode = 'managed' THEN $1::text || '/' || l.path ELSE l.root || '/' || l.path END))
		FROM work_primary wp JOIN storage_locations l ON l.file_id = wp.file_id
		WHERE j.type = 'ingest' AND j.state = 'failed' AND wp.work_id = j.work_id`, storagePath); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, `
		WITH candidates AS (
			SELECT DISTINCT ON (j.type, j.work_id) j.id FROM jobs j
			WHERE j.state = 'failed' AND j.work_id IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM jobs a WHERE a.type = j.type AND a.work_id = j.work_id AND a.state IN ('pending', 'running'))
			ORDER BY j.type, j.work_id, j.id DESC)
		UPDATE jobs SET state = 'pending', attempts = 0, run_at = now(), cancel_requested = FALSE,
		       last_error = NULL, error_kind = NULL, finished_at = NULL, updated_at = now()
		WHERE id IN (SELECT id FROM candidates)
		RETURNING id`)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		requeued = append(requeued, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE state = 'failed'`).Scan(&left); err != nil {
		return nil, 0, err
	}
	return requeued, left, tx.Commit()
}

// Cancel stops a pending job at once and asks a running one to stop; the worker
// notices at its next heartbeat and publishes nothing.
func Cancel(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `
		UPDATE jobs SET
		    state = CASE WHEN state = 'pending' THEN 'cancelled' ELSE state END,
		    cancel_requested = TRUE,
		    finished_at = CASE WHEN state = 'pending' THEN now() ELSE finished_at END,
		    updated_at = now()
		WHERE id = $1 AND state IN ('pending', 'running')`, id)
	return affected(ctx, db, res, err, id)
}

// affected turns "no row changed" into a precise error: unknown job, or a job in
// a state that does not allow the action.
func affected(ctx context.Context, db *sql.DB, res sql.Result, err error, id int64) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM jobs WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return fmt.Errorf("%w (job %s)", ErrState, strconv.FormatInt(id, 10))
}
