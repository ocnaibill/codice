package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/testdb"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func setup(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func newWork(t *testing.T, db *sql.DB, title string) int {
	t.Helper()
	id, _, _ := testdb.AddWork(t, db, testdb.Work{Title: title, Path: title + ".epub"})
	return id
}

func enqueue(t *testing.T, db *sql.DB, workID, priority int) int64 {
	t.Helper()
	id, err := jobs.Enqueue(ctx, db, jobs.TypeIngest, workID, map[string]any{"file_path": "/x"}, priority, "")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// claim runs jobs_claim as owner and returns the job id, or 0 when nothing was given.
func claim(t *testing.T, db *sql.DB, owner string, maxRunning int) int64 {
	t.Helper()
	var id sql.NullInt64
	err := db.QueryRow(`SELECT id FROM jobs_claim($1, 120, $2)`, owner, maxRunning).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return id.Int64
}

func state(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT state FROM jobs WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func scalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return s.String
}

func fail(t *testing.T, db *sql.DB, id int64, owner, kind, msg string) string {
	t.Helper()
	return scalar(t, db, `SELECT jobs_fail($1, $2, $3, $4)`, id, owner, kind, msg)
}

func complete(t *testing.T, db *sql.DB, id int64, owner string) bool {
	t.Helper()
	return scalar(t, db, `SELECT jobs_complete($1, $2)`, id, owner) == "true"
}

func heartbeat(t *testing.T, db *sql.DB, id int64, owner string) string {
	t.Helper()
	return scalar(t, db, `SELECT jobs_heartbeat($1, $2, 120)`, id, owner)
}

func TestEnqueue_IsTransactionalAndOneLiveJobPerWork(t *testing.T) {
	db := setup(t)
	w := newWork(t, db, "duna")

	// A rolled-back transaction leaves no job behind (DEC-067).
	tx, _ := db.Begin()
	if _, err := jobs.Enqueue(ctx, tx, jobs.TypeIngest, w, nil, 0, ""); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	if n := scalar(t, db, `SELECT count(*) FROM jobs`); n != "0" {
		t.Fatalf("a rolled-back enqueue left %s jobs", n)
	}

	first := enqueue(t, db, w, jobs.PriorityBatch)
	again := enqueue(t, db, w, jobs.PriorityManual)
	if first != again {
		t.Errorf("enqueueing twice created a second live job: %d and %d", first, again)
	}
	if got := scalar(t, db, `SELECT media_status FROM works WHERE id = $1`, w); got != "QUEUED" {
		t.Errorf("media_status = %q, want QUEUED", got)
	}

	// Once finished, the work can be processed again.
	id := claim(t, db, "w1", 5)
	complete(t, db, id, "w1")
	if next := enqueue(t, db, w, 0); next == first {
		t.Error("a finished job must not be reused")
	}
}

func TestClaim_ManualBeforeBatchThenOldestFirst(t *testing.T) {
	db := setup(t)
	batchOld := enqueue(t, db, newWork(t, db, "a"), jobs.PriorityBatch)
	manual := enqueue(t, db, newWork(t, db, "b"), jobs.PriorityManual)
	batchNew := enqueue(t, db, newWork(t, db, "c"), jobs.PriorityBatch)

	var got []int64
	for i := 0; i < 3; i++ {
		got = append(got, claim(t, db, "w", 10))
	}
	want := []int64{manual, batchOld, batchNew}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("claim order = %v, want %v", got, want)
		}
	}
	if claim(t, db, "w", 10) != 0 {
		t.Error("claimed a job when none was left")
	}
}

func TestClaim_OnlyTheConfiguredNumberRunAtOnce(t *testing.T) {
	db := setup(t)
	for i := 0; i < 5; i++ {
		enqueue(t, db, newWork(t, db, string(rune('a'+i))), 0)
	}
	first := claim(t, db, "w1", 1)
	if first == 0 {
		t.Fatal("nothing claimed")
	}
	if claim(t, db, "w2", 1) != 0 {
		t.Error("a second heavy job started while one was running (DEC-069)")
	}
	complete(t, db, first, "w1")
	if claim(t, db, "w2", 1) == 0 {
		t.Error("the next job did not start after the first finished")
	}

	// Workers racing for a limit of one: exactly one wins, every round. A single
	// round could pass by luck of timing, so the race is repeated.
	for round := 0; round < 40; round++ {
		db.Exec(`UPDATE jobs SET state = 'cancelled', lease_owner = NULL, lease_expires_at = NULL WHERE state IN ('pending', 'running')`)
		for i := 0; i < 4; i++ {
			enqueue(t, db, newWork(t, db, fmt.Sprintf("r%d-%d", round, i)), 0)
		}
		var wg sync.WaitGroup
		var won int32
		start := make(chan struct{})
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				if claim(t, db, fmt.Sprintf("racer%d", i), 1) != 0 {
					atomic.AddInt32(&won, 1)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if won != 1 {
			t.Fatalf("round %d: %d workers started a job at the same time with a limit of 1", round, won)
		}
	}
}

func TestClaim_ConcurrentWorkersNeverShareAJob(t *testing.T) {
	db := setup(t)
	for i := 0; i < 10; i++ {
		enqueue(t, db, newWork(t, db, "w"+string(rune('a'+i))), 0)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[int64]int{}
	start := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if id := claim(t, db, "worker"+string(rune('0'+i)), 100); id != 0 {
				mu.Lock()
				seen[id]++
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if len(seen) != 10 {
		t.Errorf("10 workers claimed %d distinct jobs, want 10", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("job %d was claimed %d times", id, n)
		}
	}
}

func TestFailure_TemporaryErrorsRetryWithGrowingWaitThenStop(t *testing.T) {
	db := setup(t)
	id := enqueue(t, db, newWork(t, db, "duna"), 0)

	for attempt, wait := range []string{"30 seconds", "2 minutes"} {
		if got := claim(t, db, "w", 5); got != id {
			t.Fatalf("attempt %d: claimed %d", attempt+1, got)
		}
		if r := fail(t, db, id, "w", "temporary", "redis timeout"); r != "retry" {
			t.Fatalf("attempt %d: result %q, want retry", attempt+1, r)
		}
		if state(t, db, id) != "pending" {
			t.Fatalf("attempt %d: state %s", attempt+1, state(t, db, id))
		}
		// The wait is the one the decision fixed, and the job is not claimable during it.
		if ok := scalar(t, db, `SELECT (run_at - now()) BETWEEN interval '`+wait+`' - interval '5 seconds' AND interval '`+wait+`' FROM jobs WHERE id = $1`, id); ok != "true" {
			t.Errorf("attempt %d: waiting time is not about %s", attempt+1, wait)
		}
		if claim(t, db, "w", 5) != 0 {
			t.Fatalf("attempt %d: claimed during the wait", attempt+1)
		}
		db.Exec(`UPDATE jobs SET run_at = now() WHERE id = $1`, id) // the wait is over
	}

	// Third attempt: no attempts left, so it fails and stays failed.
	claim(t, db, "w", 5)
	if r := fail(t, db, id, "w", "temporary", "still down"); r != "failed" {
		t.Fatalf("after 3 attempts: %q, want failed", r)
	}
	if got := scalar(t, db, `SELECT state || '/' || error_kind || '/' || attempts FROM jobs WHERE id = $1`, id); got != "failed/temporary/3" {
		t.Errorf("final state = %q", got)
	}
	if claim(t, db, "w", 5) != 0 {
		t.Error("a failed job was claimed again without a rerun")
	}
	// An admin can rerun it, with a fresh set of attempts.
	if err := jobs.Rerun(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, db, `SELECT state || '/' || attempts FROM jobs WHERE id = $1`, id); got != "pending/0" {
		t.Errorf("after rerun = %q", got)
	}
	if claim(t, db, "w", 5) != id {
		t.Error("the rerun job was not claimable")
	}
}

func TestFailure_PermanentErrorsAreNotRetried(t *testing.T) {
	db := setup(t)
	id := enqueue(t, db, newWork(t, db, "corrompido"), 0)
	claim(t, db, "w", 5)
	if r := fail(t, db, id, "w", "permanent", "not a valid EPUB"); r != "failed" {
		t.Fatalf("result %q, want failed", r)
	}
	if got := scalar(t, db, `SELECT state || '/' || error_kind || '/' || attempts FROM jobs WHERE id = $1`, id); got != "failed/permanent/1" {
		t.Errorf("state = %q", got)
	}
	if claim(t, db, "w", 5) != 0 {
		t.Error("a permanent failure was retried")
	}
}

func TestFailure_MessageIsCut(t *testing.T) {
	db := setup(t)
	id := enqueue(t, db, newWork(t, db, "x"), 0)
	claim(t, db, "w", 5)
	fail(t, db, id, "w", "permanent", strings.Repeat("stack frame ", 1000))
	if n := scalar(t, db, `SELECT length(last_error) FROM jobs WHERE id = $1`, id); n != "500" {
		t.Errorf("stored error is %s characters, want 500", n)
	}
}

func TestLease_AVanishedWorkersJobIsTakenOver(t *testing.T) {
	db := setup(t)
	id := enqueue(t, db, newWork(t, db, "duna"), 0)
	if claim(t, db, "dead", 1) != id {
		t.Fatal("first claim failed")
	}
	// While the lease is fresh nobody else gets it, even with room to run.
	if claim(t, db, "other", 5) != 0 {
		t.Error("a job with a live lease was taken by another worker")
	}

	db.Exec(`UPDATE jobs SET lease_expires_at = now() - interval '1 minute' WHERE id = $1`, id) // the worker died
	if claim(t, db, "rescuer", 1) != id {
		t.Fatal("the job of a vanished worker was not taken over")
	}
	if got := scalar(t, db, `SELECT lease_owner || '/' || attempts FROM jobs WHERE id = $1`, id); got != "rescuer/2" {
		t.Errorf("after takeover = %q", got)
	}

	// The old worker, waking up late, can neither report nor extend anything.
	if complete(t, db, id, "dead") {
		t.Error("a worker that lost its lease completed the job")
	}
	if heartbeat(t, db, id, "dead") != "lost" {
		t.Error("a stale heartbeat was accepted")
	}
	if r := fail(t, db, id, "dead", "temporary", "late"); r != "lost" {
		t.Errorf("a stale failure was accepted: %q", r)
	}
	if !complete(t, db, id, "rescuer") || state(t, db, id) != "succeeded" {
		t.Error("the new owner could not complete the job")
	}
}

func TestLease_NoAttemptsLeftMeansFailedNotEndlessRetries(t *testing.T) {
	db := setup(t)
	id := enqueue(t, db, newWork(t, db, "duna"), 0)
	claim(t, db, "w", 5)
	db.Exec(`UPDATE jobs SET attempts = max_attempts, lease_expires_at = now() - interval '1 minute' WHERE id = $1`, id)
	if claim(t, db, "rescuer", 5) != 0 {
		t.Error("a job that used every attempt was handed out again")
	}
	if got := scalar(t, db, `SELECT state || '/' || error_kind FROM jobs WHERE id = $1`, id); got != "failed/temporary" {
		t.Errorf("state = %q", got)
	}
}

func TestHeartbeat_ExtendsTheLeaseAndReportsCancellation(t *testing.T) {
	db := setup(t)
	id := enqueue(t, db, newWork(t, db, "duna"), 0)
	claim(t, db, "w", 5)
	db.Exec(`UPDATE jobs SET lease_expires_at = now() + interval '5 seconds' WHERE id = $1`, id)
	if heartbeat(t, db, id, "w") != "ok" {
		t.Fatal("heartbeat refused")
	}
	if ok := scalar(t, db, `SELECT lease_expires_at > now() + interval '100 seconds' FROM jobs WHERE id = $1`, id); ok != "true" {
		t.Error("the lease was not extended")
	}
	if err := jobs.Cancel(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	if heartbeat(t, db, id, "w") != "cancel" {
		t.Error("the worker was not told to stop")
	}
}

func TestCancel(t *testing.T) {
	db := setup(t)

	// Pending: stops at once and is never claimed.
	p := enqueue(t, db, newWork(t, db, "p"), 0)
	if err := jobs.Cancel(ctx, db, p); err != nil {
		t.Fatal(err)
	}
	if state(t, db, p) != "cancelled" || claim(t, db, "w", 5) != 0 {
		t.Error("a cancelled pending job stayed alive")
	}

	// Running: cooperative. The worker acknowledges and nothing is published.
	r := enqueue(t, db, newWork(t, db, "r"), 0)
	claim(t, db, "w", 5)
	jobs.Cancel(ctx, db, r)
	if state(t, db, r) != "running" {
		t.Error("a running job must not be cut off abruptly")
	}
	if scalar(t, db, `SELECT jobs_cancel_ack($1, 'w')`, r) != "true" || state(t, db, r) != "cancelled" {
		t.Error("cancel acknowledgement failed")
	}

	// Running, but the worker vanished before it could acknowledge: it ends cancelled, not retried.
	v := enqueue(t, db, newWork(t, db, "v"), 0)
	claim(t, db, "dead", 5)
	jobs.Cancel(ctx, db, v)
	db.Exec(`UPDATE jobs SET lease_expires_at = now() - interval '1 minute' WHERE id = $1`, v)
	claim(t, db, "rescuer", 5)
	if state(t, db, v) != "cancelled" {
		t.Errorf("a cancelled job of a vanished worker is %s", state(t, db, v))
	}

	if err := jobs.Cancel(ctx, db, 999999); !errors.Is(err, jobs.ErrNotFound) {
		t.Errorf("unknown job: %v", err)
	}
	if err := jobs.Cancel(ctx, db, p); !errors.Is(err, jobs.ErrState) {
		t.Errorf("cancelling a cancelled job: %v", err)
	}
	if err := jobs.Rerun(ctx, db, 999999); !errors.Is(err, jobs.ErrNotFound) {
		t.Errorf("rerun of an unknown job: %v", err)
	}
	live := enqueue(t, db, newWork(t, db, "live"), 0)
	if err := jobs.Rerun(ctx, db, live); !errors.Is(err, jobs.ErrState) {
		t.Errorf("rerunning a pending job: %v", err)
	}
	// A cancelled job can be rerun.
	if err := jobs.Rerun(ctx, db, p); err != nil || state(t, db, p) != "pending" {
		t.Errorf("rerun of a cancelled job: %v / %s", err, state(t, db, p))
	}
}

func TestList_FiltersAndCounts(t *testing.T) {
	db := setup(t)
	a := enqueue(t, db, newWork(t, db, "a"), 0)
	enqueue(t, db, newWork(t, db, "b"), 0)
	claim(t, db, "w", 5)
	fail(t, db, a, "w", "permanent", "broken")

	all, counts, err := jobs.List(ctx, db, "", 50)
	if err != nil || len(all) != 2 {
		t.Fatalf("list: %v %d", err, len(all))
	}
	if counts["failed"] != 1 || counts["pending"] != 1 || counts["running"] != 0 {
		t.Errorf("counts = %v", counts)
	}
	failed, _, _ := jobs.List(ctx, db, "failed", 50)
	if len(failed) != 1 || failed[0].WorkTitle != "a" || failed[0].LastError != "broken" || failed[0].ErrorKind != "permanent" {
		t.Errorf("failed jobs = %+v", failed)
	}
}

func TestNotify_NeverBlocksOrFailsTheCaller(t *testing.T) {
	if err := jobs.Notify(ctx, nil, 1); err != nil {
		t.Errorf("nil client: %v", err)
	}
	// Redis is down: the error is reported, quickly, and only for logging.
	down := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: 0})
	defer down.Close()
	start := time.Now()
	if err := jobs.Notify(ctx, down, 1); err == nil {
		t.Error("expected an error when Redis is unreachable")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("Notify took %v with Redis down", time.Since(start))
	}
}
