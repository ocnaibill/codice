package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/jobs"
)

func newRunner(t *testing.T, types []string, h map[string]jobs.Handler) *jobs.Runner {
	t.Helper()
	db := setup(t)
	return &jobs.Runner{DB: db, Owner: "go-test", Types: types, MaxRunning: 5, LeaseSeconds: 60, Handlers: h, Heartbeat: 20 * time.Millisecond}
}

func enqueueType(t *testing.T, r *jobs.Runner, jobType string) int64 {
	t.Helper()
	w := newWork(t, r.DB, jobType+"-work")
	id, err := jobs.Enqueue(ctx, r.DB, jobType, w, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRunner_OnlyTakesItsOwnTypes(t *testing.T) {
	ran := []string{}
	r := newRunner(t, []string{"organize"}, map[string]jobs.Handler{
		"organize": func(ctx context.Context, j jobs.Claimed) error { ran = append(ran, j.Type); return nil },
	})
	ingest := enqueueType(t, r, "ingest")
	organize := enqueueType(t, r, "organize")

	took, err := r.RunOnce(ctx)
	if err != nil || !took {
		t.Fatalf("RunOnce: %v %v", took, err)
	}
	if state(t, r.DB, organize) != "succeeded" {
		t.Error("its own job was not run")
	}
	if state(t, r.DB, ingest) != "pending" {
		t.Error("the Go runner took a job meant for the Python worker")
	}
	if took, _ := r.RunOnce(ctx); took {
		t.Error("it claimed a job of another type")
	}
}

func TestRunner_TheRunningLimitCountsOnlyItsOwnTypes(t *testing.T) {
	r := newRunner(t, []string{"organize"}, map[string]jobs.Handler{})
	r.MaxRunning = 1
	// A long-running ingest job (owned by the Python worker) must not use up the
	// Go runner's slot, and the other way round.
	ing := enqueueType(t, r, "ingest")
	if scalar(t, r.DB, `SELECT count(*) FROM jobs_claim('py', 60, 1, ARRAY['ingest'])`) != "1" {
		t.Fatal("python claim failed")
	}
	org := enqueueType(t, r, "organize")
	took, _ := r.RunOnce(ctx) // no handler registered: the job fails permanently, but it was claimed
	if !took || state(t, r.DB, org) != "failed" {
		t.Errorf("the organize job was blocked by a running ingest job (state %s)", state(t, r.DB, org))
	}
	_ = ing
}

func TestRunner_ClassifiesErrorsAndCompletes(t *testing.T) {
	r := newRunner(t, []string{"a", "b", "c"}, map[string]jobs.Handler{
		"a": func(ctx context.Context, j jobs.Claimed) error {
			return jobs.Permanent(errors.New("this file is unreadable"))
		},
		"b": func(ctx context.Context, j jobs.Claimed) error { return errors.New("disk busy") },
		"c": func(ctx context.Context, j jobs.Claimed) error { return nil },
	})
	a, b, c := enqueueType(t, r, "a"), enqueueType(t, r, "b"), enqueueType(t, r, "c")
	for i := 0; i < 3; i++ {
		if took, err := r.RunOnce(ctx); !took || err != nil {
			t.Fatalf("run %d: %v %v", i, took, err)
		}
	}
	if got := scalar(t, r.DB, `SELECT state || '/' || error_kind || '/' || attempts FROM jobs WHERE id = $1`, a); got != "failed/permanent/1" {
		t.Errorf("permanent: %q", got)
	}
	if got := scalar(t, r.DB, `SELECT state || '/' || error_kind FROM jobs WHERE id = $1`, b); got != "pending/temporary" {
		t.Errorf("temporary: %q (it must wait and retry)", got)
	}
	if state(t, r.DB, c) != "succeeded" {
		t.Errorf("success: %s", state(t, r.DB, c))
	}
}

func TestRunner_CancellationStopsTheHandlerCooperatively(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	r := newRunner(t, []string{"slow"}, map[string]jobs.Handler{
		"slow": func(ctx context.Context, j jobs.Claimed) error {
			close(started)
			select {
			case <-ctx.Done():
				close(stopped)
				return ctx.Err()
			case <-time.After(10 * time.Second):
				return nil
			}
		},
	})
	id := enqueueType(t, r, "slow")
	go func() {
		<-started
		jobs.Cancel(ctx, r.DB, id)
	}()
	if took, _ := r.RunOnce(ctx); !took {
		t.Fatal("nothing ran")
	}
	select {
	case <-stopped:
	default:
		t.Error("the handler was not told to stop")
	}
	if state(t, r.DB, id) != "cancelled" {
		t.Errorf("state = %s, want cancelled (not failed, not retried)", state(t, r.DB, id))
	}
}

func enqueueNamed(t *testing.T, r *jobs.Runner, jobType, name string) int64 {
	t.Helper()
	w := newWork(t, r.DB, name)
	id, err := jobs.Enqueue(ctx, r.DB, jobType, w, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// poolRun starts LoopPool and returns what a test needs: how many handlers are inside, the peak, and a way to let them go.
type poolProbe struct {
	inside  atomic.Int32
	peak    atomic.Int32
	started atomic.Int32
	release chan struct{}
}

func newPoolProbe() *poolProbe { return &poolProbe{release: make(chan struct{}, 100)} }

func (p *poolProbe) handler(ctx context.Context, j jobs.Claimed) error {
	n := p.inside.Add(1)
	p.started.Add(1)
	for {
		cur := p.peak.Load()
		if n <= cur || p.peak.CompareAndSwap(cur, n) {
			break
		}
	}
	defer p.inside.Add(-1)
	select {
	case <-p.release:
	case <-ctx.Done():
	}
	return nil
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoopPool_RunsAsManyAtOnceAsAskedAndNoMore(t *testing.T) {
	probe := newPoolProbe()
	var want atomic.Int32
	want.Store(3)
	r := newRunner(t, []string{"dedupe"}, map[string]jobs.Handler{"dedupe": probe.handler})
	r.MaxRunningFn = func() int { return int(want.Load()) }
	for i := 0; i < 8; i++ {
		enqueueNamed(t, r, "dedupe", fmt.Sprintf("dedupe-work-%d", i))
	}
	poolCtx, stop := context.WithCancel(ctx)
	defer stop()
	go r.LoopPool(poolCtx, 20*time.Millisecond, func() int { return int(want.Load()) })

	waitUntil(t, "three running", func() bool { return probe.inside.Load() == 3 })
	time.Sleep(150 * time.Millisecond)
	if got := probe.inside.Load(); got != 3 {
		t.Fatalf("running = %d, want exactly 3", got)
	}
	if peak := probe.peak.Load(); peak > 3 {
		t.Errorf("peak = %d, the pool went above the number asked", peak)
	}
	// Let them all finish: the eight are done, three at a time.
	for i := 0; i < 8; i++ {
		probe.release <- struct{}{}
	}
	waitUntil(t, "all eight started", func() bool { return probe.started.Load() == 8 })
	if peak := probe.peak.Load(); peak > 3 {
		t.Errorf("peak = %d while draining, want at most 3", peak)
	}
}

func TestLoopPool_GrowsWhileItRuns_AndShrinksWithoutCuttingAJob(t *testing.T) {
	probe := newPoolProbe()
	var want atomic.Int32
	want.Store(1)
	r := newRunner(t, []string{"dedupe"}, map[string]jobs.Handler{"dedupe": probe.handler})
	r.MaxRunningFn = func() int { return int(want.Load()) }
	for i := 0; i < 6; i++ {
		enqueueNamed(t, r, "dedupe", fmt.Sprintf("dedupe-work-%d", i))
	}
	poolCtx, stop := context.WithCancel(ctx)
	defer stop()
	go r.LoopPool(poolCtx, 20*time.Millisecond, func() int { return int(want.Load()) })

	waitUntil(t, "one running", func() bool { return probe.inside.Load() == 1 })
	time.Sleep(100 * time.Millisecond)
	if got := probe.inside.Load(); got != 1 {
		t.Fatalf("running = %d, want 1", got)
	}
	want.Store(3) // the owner raises it: within a couple of seconds there are three
	waitUntil(t, "three running after raising", func() bool { return probe.inside.Load() == 3 })

	want.Store(1) // and lowers it: the three that are running are NOT cut
	time.Sleep(300 * time.Millisecond)
	if got := probe.inside.Load(); got != 3 {
		t.Fatalf("lowering the number cut a running job: %d running, want 3", got)
	}
	probe.release <- struct{}{} // one finishes: 2 left, which is still above 1, and nothing new may start
	time.Sleep(300 * time.Millisecond)
	if got := probe.inside.Load(); got != 2 {
		t.Fatalf("after one finished with the limit at 1: %d running, want 2 (none started)", got)
	}
	probe.release <- struct{}{}
	time.Sleep(300 * time.Millisecond)
	if got := probe.inside.Load(); got != 1 {
		t.Fatalf("running = %d, want 1", got)
	}
	// With the limit at 1 and one running, no extra job starts: the started count stops moving.
	before := probe.started.Load()
	time.Sleep(300 * time.Millisecond)
	if probe.started.Load() != before {
		t.Error("a job started while the limit was full")
	}
}

func TestLoopPool_StopsWhenTheContextEnds(t *testing.T) {
	probe := newPoolProbe()
	r := newRunner(t, []string{"dedupe"}, map[string]jobs.Handler{"dedupe": probe.handler})
	r.MaxRunningFn = func() int { return 2 }
	poolCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { r.LoopPool(poolCtx, 20*time.Millisecond, func() int { return 2 }); close(done) }()
	time.Sleep(100 * time.Millisecond)
	stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("LoopPool did not return after its context ended")
	}
}

func TestRunner_MaxRunningFnWinsOverMaxRunning(t *testing.T) {
	r := newRunner(t, []string{"organize"}, map[string]jobs.Handler{
		"organize": func(ctx context.Context, j jobs.Claimed) error { return nil },
	})
	r.MaxRunning = 5
	r.MaxRunningFn = func() int { return 0 } // none may run: nothing is claimed
	enqueueType(t, r, "organize")
	if took, err := r.RunOnce(ctx); err != nil || took {
		t.Fatalf("RunOnce with a function that says zero = %v, %v; want nothing taken", took, err)
	}
	r.MaxRunningFn = nil // without the function the field counts
	if took, err := r.RunOnce(ctx); err != nil || !took {
		t.Fatalf("RunOnce without the function = %v, %v; want the job taken", took, err)
	}
}
