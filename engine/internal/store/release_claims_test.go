package store

import (
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestReleaseRunsByClaimantReleasesOnlyOwnRunningClaims(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instanceA := ts.createInstance(t, projectID, "release-run-a")
	runA := ts.createRun(t, instanceA, 1)

	claimedA, err := ts.store.ClaimNextRun(ts.ctx, "drain-worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun(worker-a) error = %v", err)
	}
	if claimedA.ID != runA.ID {
		t.Fatalf("ClaimNextRun(worker-a) ID = %s, want %s", claimedA.ID, runA.ID)
	}

	instanceB := ts.createInstance(t, projectID, "release-run-b")
	runB := ts.createRun(t, instanceB, 1)
	claimedB, err := ts.store.ClaimNextRun(ts.ctx, "drain-worker-b", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun(worker-b) error = %v", err)
	}
	if claimedB.ID != runB.ID {
		t.Fatalf("ClaimNextRun(worker-b) ID = %s, want %s", claimedB.ID, runB.ID)
	}

	released, err := ts.store.ReleaseRunsByClaimant(ts.ctx, "drain-worker-a")
	if err != nil {
		t.Fatalf("ReleaseRunsByClaimant() error = %v", err)
	}
	if len(released) != 1 {
		t.Fatalf("ReleaseRunsByClaimant() returned %d runs, want 1", len(released))
	}
	if released[0].ID != runA.ID {
		t.Fatalf("ReleaseRunsByClaimant() run ID = %s, want %s", released[0].ID, runA.ID)
	}
	if released[0].Status != enginedb.EngineRunLifecycleStatusQueued ||
		released[0].ClaimedBy != nil || released[0].ClaimedAt.Valid || released[0].LeaseExpiresAt.Valid {
		t.Fatalf("released run has stale claim state: %+v", released[0])
	}
	if released[0].AttemptCount != 1 {
		t.Fatalf("released run attempt_count = %d, want unchanged value 1", released[0].AttemptCount)
	}

	refetchedA, err := ts.store.GetRun(ts.ctx, runA.ID)
	if err != nil {
		t.Fatalf("GetRun(run A) error = %v", err)
	}
	if refetchedA.Status != enginedb.EngineRunLifecycleStatusQueued ||
		refetchedA.ClaimedBy != nil || refetchedA.ClaimedAt.Valid || refetchedA.LeaseExpiresAt.Valid {
		t.Fatalf("persisted run A has stale claim state: %+v", refetchedA)
	}
	if refetchedA.AttemptCount != 1 {
		t.Fatalf("persisted run A attempt_count = %d, want unchanged value 1", refetchedA.AttemptCount)
	}

	refetchedB, err := ts.store.GetRun(ts.ctx, runB.ID)
	if err != nil {
		t.Fatalf("GetRun(run B) error = %v", err)
	}
	if refetchedB.Status != enginedb.EngineRunLifecycleStatusRunning ||
		refetchedB.ClaimedBy == nil || *refetchedB.ClaimedBy != "drain-worker-b" {
		t.Fatalf("run B changed while releasing worker A claims: %+v", refetchedB)
	}

	reclaimed, err := ts.store.ClaimNextRun(ts.ctx, "post-drain-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun(post-drain-worker) error = %v", err)
	}
	if reclaimed.ID != runA.ID {
		t.Fatalf("ClaimNextRun(post-drain-worker) ID = %s, want immediately released run %s", reclaimed.ID, runA.ID)
	}
}

func TestReleaseRunsByClaimantSkipsTerminalRuns(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "release-terminal-run")
	run := ts.createRun(t, instance, 1)

	claimed, err := ts.store.ClaimNextRun(ts.ctx, "drain-worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	claimant := "drain-worker-a"
	completed, err := ts.store.TransitionRunToCompleted(ts.ctx, enginedb.TransitionRunToCompletedParams{
		ID:           claimed.ID,
		ClaimedBy:    &claimant,
		Result:       []byte(`{"status":"done"}`),
		CustomStatus: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("TransitionRunToCompleted() error = %v", err)
	}
	if completed.Status != enginedb.EngineRunLifecycleStatusCompleted {
		t.Fatalf("TransitionRunToCompleted() status = %s, want completed", completed.Status)
	}

	released, err := ts.store.ReleaseRunsByClaimant(ts.ctx, "drain-worker-a")
	if err != nil {
		t.Fatalf("ReleaseRunsByClaimant() error = %v", err)
	}
	if len(released) != 0 {
		t.Fatalf("ReleaseRunsByClaimant() returned %d terminal runs, want 0", len(released))
	}
	refetched, err := ts.store.GetRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if refetched.Status != enginedb.EngineRunLifecycleStatusCompleted {
		t.Fatalf("terminal run status = %s after release, want completed", refetched.Status)
	}
}

func TestReleaseActivityTasksByClaimantReleasesOnlyOwnClaimedTasks(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "release-activity-tasks")
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "activity.scheduled")

	availableAt := time.Now().Add(-2 * time.Minute)
	task1, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "release-activity-1",
		ActivityType:    "draintest.block",
		Input:           []byte(`{}`),
		AvailableAt:     availableAt,
		ExecutionTarget: "local",
		MaxAttempts:     3,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask(task 1) error = %v", err)
	}
	task2, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "release-activity-2",
		ActivityType:    "draintest.block",
		Input:           []byte(`{}`),
		AvailableAt:     availableAt.Add(time.Second),
		ExecutionTarget: "local",
		MaxAttempts:     3,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask(task 2) error = %v", err)
	}

	claimed1, err := ts.store.ClaimNextActivityTask(ts.ctx, "drain-worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask(worker-a) error = %v", err)
	}
	if claimed1.ID != task1.ID {
		t.Fatalf("ClaimNextActivityTask(worker-a) ID = %s, want %s", claimed1.ID, task1.ID)
	}
	claimed2, err := ts.store.ClaimNextActivityTask(ts.ctx, "drain-worker-b", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask(worker-b) error = %v", err)
	}
	if claimed2.ID != task2.ID {
		t.Fatalf("ClaimNextActivityTask(worker-b) ID = %s, want %s", claimed2.ID, task2.ID)
	}

	released, err := ts.store.ReleaseActivityTasksByClaimant(ts.ctx, "drain-worker-a")
	if err != nil {
		t.Fatalf("ReleaseActivityTasksByClaimant() error = %v", err)
	}
	if len(released) != 1 {
		t.Fatalf("ReleaseActivityTasksByClaimant() returned %d tasks, want 1", len(released))
	}
	if released[0].ID != task1.ID {
		t.Fatalf("ReleaseActivityTasksByClaimant() task ID = %s, want %s", released[0].ID, task1.ID)
	}
	if released[0].Status != enginedb.EngineActivityTaskStatusQueued ||
		released[0].ClaimedBy != nil || released[0].LeaseExpiresAt.Valid {
		t.Fatalf("released task has stale claim state: %+v", released[0])
	}
	if released[0].AttemptCount != 1 {
		t.Fatalf("released task attempt_count = %d, want unchanged value 1", released[0].AttemptCount)
	}

	refetched1, err := ts.store.GetActivityTask(ts.ctx, task1.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(task 1) error = %v", err)
	}
	if refetched1.Status != enginedb.EngineActivityTaskStatusQueued ||
		refetched1.ClaimedBy != nil || refetched1.LeaseExpiresAt.Valid || refetched1.AttemptCount != 1 {
		t.Fatalf("persisted task 1 has wrong released state: %+v", refetched1)
	}

	refetched2, err := ts.store.GetActivityTask(ts.ctx, task2.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(task 2) error = %v", err)
	}
	if refetched2.Status != enginedb.EngineActivityTaskStatusClaimed ||
		refetched2.ClaimedBy == nil || *refetched2.ClaimedBy != "drain-worker-b" {
		t.Fatalf("task 2 changed while releasing worker A claims: %+v", refetched2)
	}

	reclaimed, err := ts.store.ClaimNextActivityTask(ts.ctx, "post-drain-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask(post-drain-worker) error = %v", err)
	}
	if reclaimed.ID != task1.ID {
		t.Fatalf("ClaimNextActivityTask(post-drain-worker) ID = %s, want immediately released task %s", reclaimed.ID, task1.ID)
	}
}
