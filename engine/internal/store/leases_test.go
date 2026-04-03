package store

import (
	"errors"
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestClaimNextRunLeaseLifecycle(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-runs")
	run := ts.createRun(t, instance, 1)

	claimed, err := ts.store.ClaimNextRun(ts.ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if claimed.ID != run.ID {
		t.Fatalf("expected to claim run %s, got %s", run.ID, claimed.ID)
	}
	if claimed.Status != enginedb.EngineRunLifecycleStatusRunning {
		t.Fatalf("expected claimed run status running, got %s", claimed.Status)
	}

	_, err = ts.store.ClaimNextRun(ts.ctx, "worker-b", time.Minute)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected active lease to block duplicate claim, got %v", err)
	}

	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.runs
		SET lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, run.ID); err != nil {
		t.Fatalf("expire run lease: %v", err)
	}

	reclaimed, err := ts.store.ClaimNextRun(ts.ctx, "worker-c", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() reclaim error = %v", err)
	}
	if reclaimed.ID != run.ID || reclaimed.AttemptCount != 2 {
		t.Fatalf("expected reclaimed run with attempt_count=2, got id=%s attempts=%d", reclaimed.ID, reclaimed.AttemptCount)
	}
}

func TestClaimNextActivityTaskLeaseLifecycle(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-activity")
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "activity.scheduled")

	task, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    projectID,
		InstanceID:   instance.ID,
		RunID:        run.ID,
		HistoryID:    &history.ID,
		ActivityKey:  "activity-1",
		ActivityType: "email.send",
		Input:        []byte(`{"to":"user@example.com"}`),
		AvailableAt:  time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}

	claimed, err := ts.store.ClaimNextActivityTask(ts.ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask() error = %v", err)
	}
	if claimed.ID != task.ID || claimed.Status != enginedb.EngineActivityTaskStatusClaimed {
		t.Fatalf("expected claimed task %s in claimed status, got %+v", task.ID, claimed)
	}

	_, err = ts.store.ClaimNextActivityTask(ts.ctx, "worker-b", time.Minute)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected active lease to block duplicate activity claim, got %v", err)
	}

	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.activity_tasks
		SET lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, task.ID); err != nil {
		t.Fatalf("expire activity lease: %v", err)
	}

	reclaimed, err := ts.store.ClaimNextActivityTask(ts.ctx, "worker-c", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask() reclaim error = %v", err)
	}
	if reclaimed.ID != task.ID || reclaimed.AttemptCount != 2 {
		t.Fatalf("expected reclaimed activity task with attempt_count=2, got id=%s attempts=%d", reclaimed.ID, reclaimed.AttemptCount)
	}
}

func TestClaimNextInboxItemLeaseLifecycle(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-inbox")
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "signal.received")

	inbox, err := ts.store.CreateInboxItem(ts.ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  instance.ID,
		RunID:       enginetest.NullableUUID(run.ID),
		HistoryID:   &history.ID,
		Kind:        "signal",
		Payload:     []byte(`{"name":"wake"}`),
		AvailableAt: time.Now().Add(-time.Minute),
		DedupeKey:   enginetest.Ptr("signal-1"),
	})
	if err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}

	claimed, err := ts.store.ClaimNextInboxItem(ts.ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextInboxItem() error = %v", err)
	}
	if claimed.ID != inbox.ID || claimed.Status != enginedb.EngineInboxStatusClaimed {
		t.Fatalf("expected claimed inbox item %s in claimed status, got %+v", inbox.ID, claimed)
	}

	_, err = ts.store.ClaimNextInboxItem(ts.ctx, "worker-b", time.Minute)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected active lease to block duplicate inbox claim, got %v", err)
	}

	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.inbox
		SET lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, inbox.ID); err != nil {
		t.Fatalf("expire inbox lease: %v", err)
	}

	reclaimed, err := ts.store.ClaimNextInboxItem(ts.ctx, "worker-c", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextInboxItem() reclaim error = %v", err)
	}
	if reclaimed.ID != inbox.ID {
		t.Fatalf("expected reclaimed inbox item %s, got %s", inbox.ID, reclaimed.ID)
	}
}
