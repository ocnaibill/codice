package storage_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/jobs"
)

func TestOrganize_HappensByItselfAfterTheFileIsAnalysed(t *testing.T) {
	e := newEnv(t)
	work, file := e.addWork("Duna", "Frank Herbert", "ab12_Duna.epub", "epub", "pt", "Aleph", "2017")
	other, _ := e.addWork("Outra", "X", "cd34_Outra.epub", "epub", "", "", "")

	ingest, err := jobs.Enqueue(ctx, e.db, jobs.TypeIngest, work, map[string]any{"file_path": "x"}, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	otherIngest, _ := jobs.Enqueue(ctx, e.db, jobs.TypeIngest, other, nil, 10, "")

	// Analysis has not finished: nothing is organized, no organize job exists.
	if n := e.scalar(`SELECT count(*) FROM jobs WHERE type = 'organize'`); n != "0" {
		t.Fatalf("organize jobs before the analysis finished: %s", n)
	}

	// The Python worker finishes the first ingest job (through the queue's own function).
	e.scalar(`SELECT id FROM jobs_claim('py', 60, 5, ARRAY['ingest'])`)
	if got := e.scalar(`SELECT jobs_complete($1, 'py')`, ingest); got != "true" {
		t.Fatalf("complete: %s", got)
	}
	// The queue itself asks for the layout to be applied, once, for that work only.
	if got := e.scalar(`SELECT string_agg(work_id::text || ':' || state, ',') FROM jobs WHERE type = 'organize'`); got != fmt.Sprintf("%d:pending", work) {
		t.Fatalf("organize jobs = %q", got)
	}
	_ = otherIngest

	runner := &jobs.Runner{DB: e.db, Owner: "api", Types: []string{"organize"}, MaxRunning: 2, LeaseSeconds: 60,
		Heartbeat: 20 * time.Millisecond,
		Handlers: map[string]jobs.Handler{
			"organize": func(ctx context.Context, j jobs.Claimed) error { return e.mover.OrganizeWork(ctx, *j.WorkID) },
		}}
	if took, err := runner.RunOnce(ctx); err != nil || !took {
		t.Fatalf("RunOnce: %v %v", took, err)
	}

	want := "Frank Herbert/Duna/Português — Aleph — 2017/Duna.epub"
	if got := strings.Split(e.locationOf(file), "|")[0]; got != want || !e.has(want) || e.has("ab12_Duna.epub") {
		t.Errorf("after the job: location %q", e.locationOf(file))
	}
	// The other work's analysis has not finished, so its file stays where ingestion put it.
	if !e.has("cd34_Outra.epub") {
		t.Error("a work whose analysis is not finished was organized")
	}
	if took, _ := runner.RunOnce(ctx); took {
		t.Error("there was nothing else to do")
	}

	// Finishing the same ingest job again (a re-analysis) creates one more organize
	// job only if none is live, and it finds everything already in place.
	if n := e.scalar(`SELECT count(*) FROM jobs WHERE type = 'organize' AND work_id = $1`, work); n != "1" {
		t.Errorf("organize jobs for the work = %s", n)
	}
}
