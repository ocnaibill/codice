package jobs_test

import (
	"context"
	"errors"
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
