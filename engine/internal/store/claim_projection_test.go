package store

import (
	"context"
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

// A shutdown release can commit after a claim takes its statement snapshot but
// before it locks the run. The claim must still project running onto the
// trace, which the release has just set to queued.
func TestClaimNextRunProjectsRunningAfterConcurrentRelease(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	run := ts.createProjectedRun(t, projectID, "claim-after-release")
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.runs
		SET status = 'running', claimed_by = 'old-worker', claimed_at = NOW() - INTERVAL '1 hour',
		    lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, run.ID); err != nil {
		t.Fatalf("expire run lease: %v", err)
	}
	if _, err := ts.db.Pool.Exec(ts.ctx, `UPDATE public.traces SET engine_run_status = 'running' WHERE engine_run_id = $1`, run.ID); err != nil {
		t.Fatalf("mark projected run running: %v", err)
	}

	// The claim connection resolves NOW() to this shadow. Its first call, made
	// after the statement snapshot is taken and before any row lock, waits on an
	// advisory lock that the test holds.
	const barrierKey = 347001
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		CREATE SCHEMA claim_barrier;
		CREATE FUNCTION claim_barrier.now() RETURNS timestamptz LANGUAGE plpgsql VOLATILE AS $$
		BEGIN
			IF current_setting('claim_barrier.pause', true) = 'on' THEN
				PERFORM set_config('claim_barrier.pause', 'off', false);
				PERFORM pg_catalog.pg_advisory_lock(347001);
				PERFORM pg_catalog.pg_advisory_unlock(347001);
			END IF;
			RETURN pg_catalog.now();
		END $$;
	`); err != nil {
		t.Fatalf("create now() barrier: %v", err)
	}
	blocker, err := ts.db.Pool.Acquire(ts.ctx)
	if err != nil {
		t.Fatalf("acquire blocker connection: %v", err)
	}
	defer blocker.Release()
	if _, err := blocker.Exec(ts.ctx, "SELECT pg_advisory_lock($1)", barrierKey); err != nil {
		t.Fatalf("take barrier lock: %v", err)
	}
	unlocked := false
	unlock := func() {
		if unlocked {
			return
		}
		unlocked = true
		if _, err := blocker.Exec(ts.ctx, "SELECT pg_advisory_unlock($1)", barrierKey); err != nil {
			t.Errorf("release barrier lock: %v", err)
		}
	}
	defer unlock()

	claimConn, err := ts.db.Pool.Acquire(ts.ctx)
	if err != nil {
		t.Fatalf("acquire claim connection: %v", err)
	}
	defer func() {
		_, _ = claimConn.Exec(context.WithoutCancel(ts.ctx), "RESET search_path; RESET claim_barrier.pause")
		claimConn.Release()
	}()
	if _, err := claimConn.Exec(ts.ctx, "SET search_path TO claim_barrier, pg_catalog, public; SET claim_barrier.pause TO 'on'"); err != nil {
		t.Fatalf("arm claim barrier: %v", err)
	}
	claimPID := claimConn.Conn().PgConn().PID()
	claimedBy := "new-worker"
	done := make(chan error, 1)
	go func() {
		_, err := enginedb.New(claimConn).ClaimNextRun(ts.ctx, enginedb.ClaimNextRunParams{
			ClaimedBy:           &claimedBy,
			LeaseDurationMicros: time.Minute.Microseconds(),
		})
		done <- err
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err := ts.db.Pool.QueryRow(ts.ctx, `
			SELECT COALESCE(wait_event_type = 'Lock' AND wait_event = 'advisory', false)
			FROM pg_stat_activity WHERE pid = $1
		`, claimPID).Scan(&waiting); err != nil {
			t.Fatalf("read claim wait state: %v", err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("claim finished before the barrier: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("claim did not reach the barrier within 10s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, err := ts.store.ReleaseRunsByClaimant(ts.ctx, "old-worker"); err != nil {
		t.Fatalf("ReleaseRunsByClaimant() error = %v", err)
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ClaimNextRun() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("claim did not finish within 10s after the release")
	}

	claimed, err := ts.store.GetRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if claimed.Status != enginedb.EngineRunLifecycleStatusRunning {
		t.Fatalf("run status after claim = %q, want running", claimed.Status)
	}
	if got := ts.projectedRunStatus(t, run.ID); got != string(enginedb.EngineRunLifecycleStatusRunning) {
		t.Fatalf("projected run status after claim = %q, want running", got)
	}
}
