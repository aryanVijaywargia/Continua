package store

import (
	"testing"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

// Claim and shutdown release change engine.runs.status without a history row,
// so the projector never sees them; they must project the status themselves
// (#347).

func (ts *testStore) createProjectedRun(t *testing.T, projectID uuid.UUID, key string) enginedb.EngineRun {
	t.Helper()

	enginetest.EnsurePlatformProject(t, ts.db.Pool, projectID)
	instance := ts.createInstance(t, projectID, key)
	run := ts.createRun(t, instance, 1)
	started := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "workflow.started")
	enginetest.SeedProjectionShell(t, ts.db.Pool, &instance, &run, instance.DefinitionName, run.DefinitionVersion, []byte(`{}`), started.ID)
	if got := ts.projectedRunStatus(t, run.ID); got != string(enginedb.EngineRunLifecycleStatusQueued) {
		t.Fatalf("seeded projected run status = %q, want queued", got)
	}
	return run
}

func (ts *testStore) projectedRunStatus(t *testing.T, runID uuid.UUID) string {
	t.Helper()

	var status string
	if err := ts.db.Pool.QueryRow(ts.ctx, `
		SELECT engine_run_status
		FROM public.traces
		WHERE engine_run_id = $1
	`, runID).Scan(&status); err != nil {
		t.Fatalf("read projected run status: %v", err)
	}
	return status
}

func TestClaimNextRunProjectsRunningStatus(t *testing.T) {
	tests := []struct {
		name          string
		projectFilter bool
	}{
		{name: "all projects"},
		{name: "project filter", projectFilter: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestStore(t)
			projectID := uuidOrFatal(t)
			run := ts.createProjectedRun(t, projectID, "claim-projection")

			claimer := ts.store
			if tt.projectFilter {
				claimer = ts.store.WithProjectFilter(projectID)
			}
			claimed, err := claimer.ClaimNextRun(ts.ctx, "claim-worker", time.Minute)
			if err != nil {
				t.Fatalf("ClaimNextRun() error = %v", err)
			}
			if claimed.ID != run.ID {
				t.Fatalf("ClaimNextRun() ID = %s, want %s", claimed.ID, run.ID)
			}
			if got := ts.projectedRunStatus(t, run.ID); got != string(enginedb.EngineRunLifecycleStatusRunning) {
				t.Fatalf("projected run status after claim = %q, want running", got)
			}
		})
	}
}

func TestReleaseRunsByClaimantProjectsQueuedStatus(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	runA := ts.createProjectedRun(t, projectID, "release-projection-a")
	runB := ts.createProjectedRun(t, projectID, "release-projection-b")
	for range 2 {
		if _, err := ts.store.ClaimNextRun(ts.ctx, "release-worker", time.Minute); err != nil {
			t.Fatalf("ClaimNextRun() error = %v", err)
		}
	}
	// Force the claimed status onto the traces so the test does not depend on
	// the claim projecting it.
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE public.traces
		SET engine_run_status = 'running'
		WHERE engine_run_id = ANY($1::uuid[])
	`, []uuid.UUID{runA.ID, runB.ID}); err != nil {
		t.Fatalf("mark projected runs running: %v", err)
	}

	released, err := ts.store.ReleaseRunsByClaimant(ts.ctx, "release-worker")
	if err != nil {
		t.Fatalf("ReleaseRunsByClaimant() error = %v", err)
	}
	if len(released) != 2 {
		t.Fatalf("ReleaseRunsByClaimant() released %d runs, want 2", len(released))
	}
	for _, runID := range []uuid.UUID{runA.ID, runB.ID} {
		if got := ts.projectedRunStatus(t, runID); got != string(enginedb.EngineRunLifecycleStatusQueued) {
			t.Fatalf("projected status of run %s after release = %q, want queued", runID, got)
		}
	}
}
