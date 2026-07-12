package store

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

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
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "activity-1",
		ActivityType:    "email.send",
		Input:           []byte(`{"to":"user@example.com"}`),
		AvailableAt:     time.Now().Add(-time.Minute),
		ExecutionTarget: "local",
		MaxAttempts:     1,
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

func TestLocalAndRemoteActivityTaskClaimIsolation(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-activity-targets")
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "activity.scheduled")

	remoteTask, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "remote-activity",
		ActivityType:    "email.send",
		Input:           []byte(`{"to":"remote@example.com"}`),
		AvailableAt:     time.Now().Add(-time.Minute),
		ExecutionTarget: "remote",
		MaxAttempts:     1,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask(remote) error = %v", err)
	}
	localTask, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "local-activity",
		ActivityType:    "email.send",
		Input:           []byte(`{"to":"local@example.com"}`),
		AvailableAt:     time.Now().Add(-time.Minute),
		ExecutionTarget: "local",
		MaxAttempts:     1,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask(local) error = %v", err)
	}

	claimedLocal, err := ts.store.ClaimNextActivityTask(ts.ctx, "local-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask() error = %v", err)
	}
	if claimedLocal.ID != localTask.ID {
		t.Fatalf("expected local worker to claim local task %s, got %s", localTask.ID, claimedLocal.ID)
	}

	claimedRemote, err := ts.store.ClaimRemoteActivityTasks(
		ts.ctx,
		projectID,
		"remote-worker",
		[]string{"email.send"},
		10,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("ClaimRemoteActivityTasks() error = %v", err)
	}
	if len(claimedRemote) != 1 || claimedRemote[0].ID != remoteTask.ID {
		t.Fatalf("expected remote worker to claim remote task %s, got %+v", remoteTask.ID, claimedRemote)
	}

	empty, err := ts.store.ClaimRemoteActivityTasks(
		ts.ctx,
		projectID,
		"remote-worker",
		[]string{"image.process"},
		10,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("ClaimRemoteActivityTasks(unmatched) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected unmatched remote type claim to return no tasks, got %+v", empty)
	}
}

func TestCompleteRemoteActivityTaskWithinGraceAccepted(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	task := ts.createRemoteLeaseTestTask(t, projectID, "complete-within-grace", "email.send", 1)
	claimed := ts.claimRemoteLeaseTestTasks(t, projectID, "worker-a", task.ActivityType, 1)
	ts.expireRemoteLeaseTestTask(t, task.ID)

	output := []byte(`{}`)
	completed, err := ts.store.WithLeaseCompletionGrace(30*time.Second).CompleteRemoteActivityTask(
		ts.ctx, projectID, claimed[0].ID, "worker-a", output,
	)
	if err != nil {
		t.Fatalf("CompleteRemoteActivityTask() error = %v", err)
	}
	if completed.Status != enginedb.EngineActivityTaskStatusCompleted {
		t.Fatalf("completed task status = %s, want completed", completed.Status)
	}
	if !bytes.Equal(completed.Output, output) {
		t.Fatalf("completed task output = %q, want %q", completed.Output, output)
	}

	persisted, err := ts.store.GetActivityTask(ts.ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if persisted.Status != enginedb.EngineActivityTaskStatusCompleted ||
		persisted.ClaimedBy != nil || persisted.LeaseExpiresAt.Valid {
		t.Fatalf("persisted completed task has stale lease state: %+v", persisted)
	}
	if !bytes.Equal(persisted.Output, output) {
		t.Fatalf("persisted output = %q, want %q", persisted.Output, output)
	}
}

func TestCompleteRemoteActivityTaskBeyondGraceRejected(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	task := ts.createRemoteLeaseTestTask(t, projectID, "complete-beyond-grace", "email.send", 1)
	ts.claimRemoteLeaseTestTasks(t, projectID, "worker-a", task.ActivityType, 1)
	ts.expireRemoteLeaseTestTask(t, task.ID)

	_, err := ts.store.WithLeaseCompletionGrace(5*time.Second).CompleteRemoteActivityTask(
		ts.ctx, projectID, task.ID, "worker-a", []byte(`{"late":true}`),
	)
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("grace completion error = %v, want ErrStaleClaim", err)
	}
	_, err = ts.store.CompleteRemoteActivityTask(
		ts.ctx, projectID, task.ID, "worker-a", []byte(`{"late":true}`),
	)
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("zero-grace completion error = %v, want ErrStaleClaim", err)
	}

	persisted, err := ts.store.GetActivityTask(ts.ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if persisted.Status != enginedb.EngineActivityTaskStatusClaimed ||
		persisted.ClaimedBy == nil || *persisted.ClaimedBy != "worker-a" || !persisted.LeaseExpiresAt.Valid {
		t.Fatalf("rejected completion changed task row: %+v", persisted)
	}
}

func TestRemoteActivityTaskReclaimRejectsStaleOwnerDespiteGrace(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	task := ts.createRemoteLeaseTestTask(t, projectID, "reclaimed-task", "email.send", 2)
	ts.claimRemoteLeaseTestTasks(t, projectID, "worker-a", task.ActivityType, 1)
	ts.expireRemoteLeaseTestTask(t, task.ID)

	reclaimed := ts.claimRemoteLeaseTestTasks(t, projectID, "worker-b", task.ActivityType, 1)
	if reclaimed[0].ID != task.ID || reclaimed[0].AttemptCount != 2 ||
		reclaimed[0].ClaimedBy == nil || *reclaimed[0].ClaimedBy != "worker-b" {
		t.Fatalf("worker-b reclaim = %+v, want task %s at attempt 2", reclaimed[0], task.ID)
	}

	_, err := ts.store.WithLeaseCompletionGrace(30*time.Second).CompleteRemoteActivityTask(
		ts.ctx, projectID, task.ID, "worker-a", []byte(`{"owner":"worker-a"}`),
	)
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale owner completion error = %v, want ErrStaleClaim", err)
	}
	persisted, err := ts.store.GetActivityTask(ts.ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if persisted.Status != enginedb.EngineActivityTaskStatusClaimed ||
		persisted.ClaimedBy == nil || *persisted.ClaimedBy != "worker-b" {
		t.Fatalf("stale owner changed reclaimed task: %+v", persisted)
	}

	completed, err := ts.store.CompleteRemoteActivityTask(
		ts.ctx, projectID, task.ID, "worker-b", []byte(`{"owner":"worker-b"}`),
	)
	if err != nil {
		t.Fatalf("current owner CompleteRemoteActivityTask() error = %v", err)
	}
	if completed.Status != enginedb.EngineActivityTaskStatusCompleted {
		t.Fatalf("current owner completion status = %s, want completed", completed.Status)
	}
}

func TestRetryAndFailRemoteActivityTaskWithinGraceAccepted(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	failTask := ts.createRemoteLeaseTestTask(t, projectID, "fail-within-grace", "email.fail", 1)
	retryTask := ts.createRemoteLeaseTestTask(t, projectID, "retry-within-grace", "email.retry", 2)
	ts.claimRemoteLeaseTestTasks(t, projectID, "worker-a", failTask.ActivityType, 1)
	ts.claimRemoteLeaseTestTasks(t, projectID, "worker-a", retryTask.ActivityType, 1)
	ts.expireRemoteLeaseTestTask(t, failTask.ID)
	ts.expireRemoteLeaseTestTask(t, retryTask.ID)

	code := "remote_error"
	message := "remote activity failed"
	graceStore := ts.store.WithLeaseCompletionGrace(30 * time.Second)
	failed, err := graceStore.FailRemoteActivityTask(
		ts.ctx, projectID, failTask.ID, "worker-a", &code, &message,
	)
	if err != nil {
		t.Fatalf("FailRemoteActivityTask() error = %v", err)
	}
	if failed.Status != enginedb.EngineActivityTaskStatusFailed || failed.LastErrorCode == nil ||
		*failed.LastErrorCode != code || failed.LastErrorMessage == nil || *failed.LastErrorMessage != message {
		t.Fatalf("failed task = %+v, want failed status and persisted error", failed)
	}

	retryStartedAt := time.Now()
	retried, err := graceStore.RetryRemoteActivityTask(
		ts.ctx, projectID, retryTask.ID, "worker-a", 1000, &code, &message,
	)
	if err != nil {
		t.Fatalf("RetryRemoteActivityTask() error = %v", err)
	}
	if retried.Status != enginedb.EngineActivityTaskStatusQueued || retried.ClaimedBy != nil ||
		!retried.AvailableAt.After(retryStartedAt) {
		t.Fatalf("retried task = %+v, want unclaimed queued task available in future", retried)
	}

	persistedFailed, err := ts.store.GetActivityTask(ts.ctx, failTask.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(failed) error = %v", err)
	}
	if persistedFailed.Status != enginedb.EngineActivityTaskStatusFailed ||
		persistedFailed.LastErrorCode == nil || *persistedFailed.LastErrorCode != code ||
		persistedFailed.LastErrorMessage == nil || *persistedFailed.LastErrorMessage != message {
		t.Fatalf("persisted failed task = %+v", persistedFailed)
	}
	persistedRetried, err := ts.store.GetActivityTask(ts.ctx, retryTask.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(retried) error = %v", err)
	}
	if persistedRetried.Status != enginedb.EngineActivityTaskStatusQueued ||
		persistedRetried.ClaimedBy != nil || persistedRetried.LeaseExpiresAt.Valid ||
		!persistedRetried.AvailableAt.After(retryStartedAt) {
		t.Fatalf("persisted retried task = %+v", persistedRetried)
	}
}

func TestHeartbeatRemoteActivityTaskIgnoresCompletionGrace(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	task := ts.createRemoteLeaseTestTask(t, projectID, "heartbeat-strict", "email.send", 1)
	ts.claimRemoteLeaseTestTasks(t, projectID, "worker-a", task.ActivityType, 1)
	ts.expireRemoteLeaseTestTask(t, task.ID)

	_, err := ts.store.WithLeaseCompletionGrace(30*time.Second).HeartbeatRemoteActivityTask(
		ts.ctx, projectID, task.ID, "worker-a",
	)
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("HeartbeatRemoteActivityTask() error = %v, want ErrStaleClaim", err)
	}
}

func (ts *testStore) createRemoteLeaseTestTask(
	t *testing.T,
	projectID uuid.UUID,
	activityKey string,
	activityType string,
	maxAttempts int32,
) enginedb.EngineActivityTask {
	t.Helper()
	instance := ts.createInstance(t, projectID, "instance-"+activityKey)
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "activity.scheduled")
	task, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     activityKey,
		ActivityType:    activityType,
		Input:           []byte(`{"source":"lease-test"}`),
		AvailableAt:     time.Now().Add(-time.Minute),
		ExecutionTarget: "remote",
		MaxAttempts:     maxAttempts,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}
	return task
}

func (ts *testStore) claimRemoteLeaseTestTasks(
	t *testing.T,
	projectID uuid.UUID,
	workerID string,
	activityType string,
	maxTasks int32,
) []enginedb.EngineActivityTask {
	t.Helper()
	tasks, err := ts.store.ClaimRemoteActivityTasks(
		ts.ctx, projectID, workerID, []string{activityType}, maxTasks, time.Minute,
	)
	if err != nil {
		t.Fatalf("ClaimRemoteActivityTasks() error = %v", err)
	}
	if len(tasks) != int(maxTasks) {
		t.Fatalf("ClaimRemoteActivityTasks() returned %d tasks, want %d", len(tasks), maxTasks)
	}
	return tasks
}

func (ts *testStore) expireRemoteLeaseTestTask(t *testing.T, taskID uuid.UUID) {
	t.Helper()
	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.activity_tasks
		SET lease_expires_at = NOW() - INTERVAL '10 seconds'
		WHERE id = $1
	`, taskID); err != nil {
		t.Fatalf("expire remote activity lease: %v", err)
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
