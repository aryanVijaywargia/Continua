package activity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

const testActivityType = "demo.activity"

func TestWorkerLateCompletionAfterTerminateReturnsNoOp(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	projectID := uuid.New()
	instance, run, task := createRunWithPendingActivity(t, store, projectID, "instance-activity-terminate", "ship-order")

	blocked := make(chan struct{})
	release := make(chan struct{})

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			close(blocked)
			<-release
			return []byte(`{"ok":true}`), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.PollOnce(ctx, "activity-worker")
	}()

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("activity handler did not reach blocking point before terminate")
	}

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	payload, err := enginehistory.MarshalPayload(enginehistory.WorkflowTerminatedPayload{
		ErrorCode:    "terminated",
		ErrorMessage: "run terminated by operator",
	})
	if err != nil {
		t.Fatalf("MarshalPayload(terminated) error = %v", err)
	}
	if _, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 3,
		EventType:  enginehistory.EventWorkflowTerminated,
		Payload:    payload,
	}); err != nil {
		t.Fatalf("AppendHistory(terminated) error = %v", err)
	}
	if _, err := tx.TransitionRunToTerminated(ctx, run.ID); err != nil {
		t.Fatalf("TransitionRunToTerminated() error = %v", err)
	}
	if _, err := tx.CancelOpenActivityTasksByRun(ctx, run.ID); err != nil {
		t.Fatalf("CancelOpenActivityTasksByRun() error = %v", err)
	}
	if _, err := tx.UpdateInstanceStatus(ctx, enginedb.UpdateInstanceStatusParams{
		ID:     instance.ID,
		Status: enginedb.EngineInstanceLifecycleStatusTerminated,
	}); err != nil {
		t.Fatalf("UpdateInstanceStatus() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	close(release)

	select {
	case err := <-workerDone:
		if err != nil {
			t.Fatalf("PollOnce() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activity worker did not finish after terminate")
	}

	terminatedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if terminatedRun.Status != enginedb.EngineRunLifecycleStatusTerminated {
		t.Fatalf("expected terminated run status, got %+v", terminatedRun)
	}

	terminatedInstance, err := store.GetInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if terminatedInstance.Status != enginedb.EngineInstanceLifecycleStatusTerminated {
		t.Fatalf("expected terminated instance status, got %+v", terminatedInstance)
	}

	cancelledTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if cancelledTask.Status != enginedb.EngineActivityTaskStatusCancelled {
		t.Fatalf("expected cancelled activity task after terminate wins, got %+v", cancelledTask)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 3 || historyRows[2].EventType != enginehistory.EventWorkflowTerminated {
		t.Fatalf("expected started + activity.scheduled + workflow.terminated history, got %+v", historyRows)
	}
}

func TestWorkerLongRunningLocalActivityHeartbeatsLease(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	_, _, task := createRunWithPendingActivity(
		t,
		store,
		enginetest.DefaultPlatformProjectID,
		"instance-activity-heartbeat",
		"long-running-local",
	)

	started := make(chan struct{})
	release := make(chan struct{})
	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-release
			return []byte(`{"ok":true}`), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	leaseTTL := 300 * time.Millisecond
	worker := NewWorker(store, registry, leaseTTL, nil)
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.PollOnce(ctx, "activity-worker")
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("activity handler did not start")
	}

	originalClaim, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(original claim) error = %v", err)
	}
	if !originalClaim.LeaseExpiresAt.Valid {
		t.Fatalf("expected claimed task lease_expires_at, got %+v", originalClaim)
	}
	originalLeaseExpiresAt := originalClaim.LeaseExpiresAt.Time

	var renewedTask enginedb.EngineActivityTask
	waitUntil(t, 5*time.Second, func() bool {
		current, err := store.GetActivityTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("GetActivityTask(mid-run) error = %v", err)
		}
		if !current.LeaseExpiresAt.Valid {
			return false
		}
		if current.LeaseExpiresAt.Time.After(originalLeaseExpiresAt) {
			renewedTask = current
		}
		return time.Now().After(originalLeaseExpiresAt.Add(2*leaseTTL)) &&
			current.LeaseExpiresAt.Time.After(originalLeaseExpiresAt)
	})

	_, err = store.ClaimNextActivityTask(ctx, "competing-worker", leaseTTL)
	if !errors.Is(err, enginestore.ErrNotFound) {
		t.Fatalf("expected renewed local activity lease to block competing claim, got %v", err)
	}
	if !renewedTask.LeaseExpiresAt.Valid || !renewedTask.LeaseExpiresAt.Time.After(originalLeaseExpiresAt) {
		t.Fatalf("expected heartbeat to advance lease beyond %s, got %+v", originalLeaseExpiresAt, renewedTask)
	}

	close(release)

	select {
	case err := <-workerDone:
		if err != nil {
			t.Fatalf("PollOnce() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activity worker did not finish after release")
	}

	completedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(completed) error = %v", err)
	}
	if completedTask.Status != enginedb.EngineActivityTaskStatusCompleted {
		t.Fatalf("expected completed activity task, got %+v", completedTask)
	}
	if completedTask.AttemptCount != 1 {
		t.Fatalf("expected attempt_count=1, got %+v", completedTask)
	}
	var completedOutput struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(completedTask.Output, &completedOutput); err != nil {
		t.Fatalf("json.Unmarshal(completed output) error = %v", err)
	}
	if !completedOutput.OK {
		t.Fatalf("expected completed output ok=true, got %s", string(completedTask.Output))
	}
}

func TestWorkerLocalActivityLeaseLossCancelsHandlerContext(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	_, _, task := createRunWithPendingActivity(
		t,
		store,
		enginetest.DefaultPlatformProjectID,
		"instance-activity-lease-lost",
		"lease-lost-local",
	)

	started := make(chan struct{})
	cancelled := make(chan struct{})
	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return []byte(`{"ok":true}`), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, 200*time.Millisecond, nil)
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.PollOnce(ctx, "activity-worker")
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("activity handler did not start")
	}

	if _, err := db.Pool.Exec(ctx, `
		UPDATE engine.activity_tasks
		SET claimed_by = 'other-worker',
		    lease_expires_at = NOW() + INTERVAL '1 minute',
		    updated_at = NOW()
		WHERE id = $1
	`, task.ID); err != nil {
		t.Fatalf("lease loss setup update error = %v", err)
	}

	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("activity handler context was not cancelled after lease loss")
	}

	select {
	case err := <-workerDone:
		if err != nil {
			t.Fatalf("PollOnce() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activity worker did not finish after lease loss")
	}

	staleTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask(stale) error = %v", err)
	}
	if staleTask.Status == enginedb.EngineActivityTaskStatusCompleted {
		t.Fatalf("expected stale worker not to complete task, got %+v", staleTask)
	}
	if staleTask.ClaimedBy == nil || *staleTask.ClaimedBy != "other-worker" {
		t.Fatalf("expected lease loss to preserve other worker claim, got %+v", staleTask)
	}
}

func TestComputeRetryDelayMS(t *testing.T) {
	initial := int64(333)
	maxBackoff := int64(1000)

	testCases := []struct {
		name         string
		attemptCount int32
		maxAttempts  int32
		multiplier   float64
		wantDelayMS  int64
	}{
		{name: "first retry uses initial backoff", attemptCount: 1, maxAttempts: 5, multiplier: 1.5, wantDelayMS: 333},
		{name: "second retry rounds up fractional milliseconds", attemptCount: 2, maxAttempts: 5, multiplier: 1.5, wantDelayMS: 500},
		{name: "later retries cap at max backoff", attemptCount: 5, maxAttempts: 5, multiplier: 1.5, wantDelayMS: 1000},
		{name: "constant multiplier keeps the same backoff", attemptCount: 3, maxAttempts: 5, multiplier: 1.0, wantDelayMS: 333},
		{name: "single attempt policy still computes initial backoff deterministically", attemptCount: 1, maxAttempts: 1, multiplier: 1.5, wantDelayMS: 333},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			delay, err := computeRetryDelayMS(&enginedb.EngineActivityTask{
				ID:                uuid.New(),
				AttemptCount:      tc.attemptCount,
				MaxAttempts:       tc.maxAttempts,
				InitialBackoffMs:  &initial,
				MaxBackoffMs:      &maxBackoff,
				BackoffMultiplier: &tc.multiplier,
			})
			if err != nil {
				t.Fatalf("computeRetryDelayMS() error = %v", err)
			}
			if delay != tc.wantDelayMS {
				t.Fatalf("expected retry delay %dms, got %dms", tc.wantDelayMS, delay)
			}
		})
	}
}

func TestComputeRetryDelayMSIsDeterministic(t *testing.T) {
	initial := int64(333)
	maxBackoff := int64(1000)
	multiplier := 1.5
	task := &enginedb.EngineActivityTask{
		ID:                uuid.New(),
		AttemptCount:      2,
		MaxAttempts:       5,
		InitialBackoffMs:  &initial,
		MaxBackoffMs:      &maxBackoff,
		BackoffMultiplier: &multiplier,
	}

	firstDelay, err := computeRetryDelayMS(task)
	if err != nil {
		t.Fatalf("computeRetryDelayMS() first call error = %v", err)
	}
	secondDelay, err := computeRetryDelayMS(task)
	if err != nil {
		t.Fatalf("computeRetryDelayMS() second call error = %v", err)
	}
	if firstDelay != secondDelay {
		t.Fatalf("expected deterministic retry delay, got first=%d second=%d", firstDelay, secondDelay)
	}
}

func TestWorkerRetryableFailureSchedulesRetryWithoutWakingRun(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	initial := int64(333)
	maxBackoff := int64(1000)
	multiplier := 1.5
	instance, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-retry",
		activityKey:       "compose-greeting",
		maxAttempts:       3,
		initialBackoffMS:  &initial,
		maxBackoffMS:      &maxBackoff,
		backoffMultiplier: &multiplier,
	})

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	before := time.Now()
	worker := NewWorker(store, registry, time.Minute, nil)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	retriedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if retriedTask.Status != enginedb.EngineActivityTaskStatusQueued {
		t.Fatalf("expected queued retry task, got %+v", retriedTask)
	}
	if retriedTask.AttemptCount != 1 {
		t.Fatalf("expected attempt_count=1 after first failed claim, got %+v", retriedTask)
	}
	if !retriedTask.AvailableAt.After(before.Add(250 * time.Millisecond)) {
		t.Fatalf("expected retry available_at to move into the future, got %+v", retriedTask)
	}

	waitingRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if waitingRun.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected run to remain waiting while retry is scheduled, got %+v", waitingRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 3 {
		t.Fatalf("expected started + activity.scheduled + activity.retry_scheduled history, got %+v", historyRows)
	}
	if historyRows[2].EventType != enginehistory.EventActivityRetryScheduled {
		t.Fatalf("expected activity.retry_scheduled history event, got %+v", historyRows[2])
	}

	decoded, err := enginehistory.DecodePayload(historyRows[2].EventType, historyRows[2].Payload)
	if err != nil {
		t.Fatalf("DecodePayload(activity.retry_scheduled) error = %v", err)
	}
	retryPayload, ok := decoded.(*enginehistory.ActivityRetryScheduledPayload)
	if !ok {
		t.Fatalf("expected ActivityRetryScheduledPayload, got %T", decoded)
	}
	if retryPayload.ActivityKey != task.ActivityKey || retryPayload.ActivityType != task.ActivityType {
		t.Fatalf("unexpected retry payload %+v", retryPayload)
	}
	if retryPayload.FailedAttempt != 1 {
		t.Fatalf("expected failed attempt 1, got %+v", retryPayload)
	}
	if retryPayload.ErrorCode != "activity_failed" || retryPayload.ErrorMessage != "boom" {
		t.Fatalf("unexpected retry payload error summary %+v", retryPayload)
	}
	if !retryPayload.NextAvailableAt.Equal(retriedTask.AvailableAt) {
		t.Fatalf("expected retry payload next_available_at=%s, got %+v", retriedTask.AvailableAt, retryPayload)
	}
	if instance.InstanceKey != "instance-activity-retry" {
		t.Fatalf("unexpected instance returned from setup: %+v", instance)
	}
}

func TestWorkerRetryScheduledSequenceContinuesFromMaxHistory(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	initial := int64(333)
	maxBackoff := int64(1000)
	multiplier := 1.5
	instance, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-retry-max-sequence",
		activityKey:       "compose-greeting",
		maxAttempts:       3,
		initialBackoffMS:  &initial,
		maxBackoffMS:      &maxBackoff,
		backoffMultiplier: &multiplier,
	})

	if _, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  run.ProjectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 10,
		EventType:  "activity.out_of_band_marker",
		Payload:    []byte(`{"event":"activity.out_of_band_marker"}`),
	}); err != nil {
		t.Fatalf("AppendHistory(high sequence marker) error = %v", err)
	}

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	retriedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if retriedTask.Status != enginedb.EngineActivityTaskStatusQueued {
		t.Fatalf("expected queued retry task, got %+v", retriedTask)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 4 {
		t.Fatalf("expected started + activity.scheduled + marker + retry history, got %+v", historyRows)
	}

	retryHistory := historyRows[len(historyRows)-1]
	if retryHistory.EventType != enginehistory.EventActivityRetryScheduled {
		t.Fatalf("expected final history row to be activity.retry_scheduled, got %+v", retryHistory)
	}
	if retryHistory.SequenceNo != 11 {
		t.Fatalf("expected retry_scheduled sequence_no 11 after max sequence 10, got %+v", retryHistory)
	}
}

func TestWorkerSingleAttemptFailureWakesWaitingRun(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	_, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:   enginetest.DefaultPlatformProjectID,
		instanceKey: "instance-activity-single-attempt",
		activityKey: "compose-greeting",
		maxAttempts: 1,
	})

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	failedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if failedTask.Status != enginedb.EngineActivityTaskStatusFailed {
		t.Fatalf("expected failed activity task, got %+v", failedTask)
	}

	queuedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if queuedRun.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected single-attempt failure to wake waiting run, got %+v", queuedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 2 {
		t.Fatalf("expected no retry history rows on single-attempt failure, got %+v", historyRows)
	}
}

func TestWorkerNonRetryableFailureBypassesRetry(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	_, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:   enginetest.DefaultPlatformProjectID,
		instanceKey: "instance-activity-non-retryable",
		activityKey: "compose-greeting",
		maxAttempts: 3,
	})

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, publicworkflow.NonRetryableError(errors.New("stop"))
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	failedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if failedTask.Status != enginedb.EngineActivityTaskStatusFailed {
		t.Fatalf("expected failed activity task, got %+v", failedTask)
	}

	queuedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if queuedRun.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected non-retryable failure to wake waiting run, got %+v", queuedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 2 {
		t.Fatalf("expected no retry history rows for non-retryable failure, got %+v", historyRows)
	}
}

func TestWorkerExhaustedRetriesWakesWaitingRun(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	initial := int64(1)
	maxBackoff := int64(1)
	multiplier := 1.0
	_, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-exhausted-retries",
		activityKey:       "compose-greeting",
		maxAttempts:       2,
		initialBackoffMS:  &initial,
		maxBackoffMS:      &maxBackoff,
		backoffMultiplier: &multiplier,
	})

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() first attempt error = %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() second attempt error = %v", err)
	}

	failedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if failedTask.Status != enginedb.EngineActivityTaskStatusFailed {
		t.Fatalf("expected exhausted retry task to fail, got %+v", failedTask)
	}
	if failedTask.AttemptCount != 2 {
		t.Fatalf("expected exhausted retry task attempt_count=2, got %+v", failedTask)
	}
	if failedTask.LastErrorCode == nil || *failedTask.LastErrorCode != "activity_failed" {
		t.Fatalf("expected failed task error_code=activity_failed, got %+v", failedTask)
	}
	if failedTask.LastErrorMessage == nil || *failedTask.LastErrorMessage != "boom" {
		t.Fatalf("expected failed task error_message=boom, got %+v", failedTask)
	}

	queuedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if queuedRun.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected exhausted retries to wake waiting run, got %+v", queuedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 3 {
		t.Fatalf("expected only retry scheduling history before activation observes failure, got %+v", historyRows)
	}
	if historyRows[2].EventType != enginehistory.EventActivityRetryScheduled {
		t.Fatalf("expected activity.retry_scheduled history row before exhausted failure wake, got %+v", historyRows[2])
	}
}

func TestWorkerMissingHandlerFailsImmediatelyWithoutRetry(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	initial := int64(250)
	maxBackoff := int64(250)
	multiplier := 1.0
	_, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-missing-handler",
		activityKey:       "compose-greeting",
		activityType:      "demo.missing",
		maxAttempts:       3,
		initialBackoffMS:  &initial,
		maxBackoffMS:      &maxBackoff,
		backoffMultiplier: &multiplier,
	})

	registry, err := NewRegistry(map[string]Handler{})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	if err := worker.PollOnce(ctx, "activity-worker"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	failedTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if failedTask.Status != enginedb.EngineActivityTaskStatusFailed {
		t.Fatalf("expected missing handler to fail task immediately, got %+v", failedTask)
	}
	if failedTask.AttemptCount != 1 {
		t.Fatalf("expected missing handler to stop after claimed attempt_count=1, got %+v", failedTask)
	}
	if failedTask.LastErrorCode == nil || *failedTask.LastErrorCode != "activity_not_registered" {
		t.Fatalf("expected activity_not_registered code, got %+v", failedTask)
	}

	queuedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if queuedRun.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected missing handler to wake waiting run, got %+v", queuedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 2 {
		t.Fatalf("expected no retry history rows for missing handler, got %+v", historyRows)
	}
}

func TestWorkerStaleRetryClaimReturnsNil(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	initial := int64(500)
	maxBackoff := int64(500)
	multiplier := 1.0
	_, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-stale-retry",
		activityKey:       "compose-greeting",
		maxAttempts:       2,
		initialBackoffMS:  &initial,
		maxBackoffMS:      &maxBackoff,
		backoffMultiplier: &multiplier,
	})

	blocked := make(chan struct{})
	release := make(chan struct{})

	registry, err := NewRegistry(map[string]Handler{
		testActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			close(blocked)
			<-release
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute, nil)
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.PollOnce(ctx, "activity-worker")
	}()

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("activity handler did not reach blocking point before stale retry simulation")
	}

	_, err = db.Pool.Exec(ctx, `
		UPDATE engine.activity_tasks
		SET claimed_by = 'other-worker',
		    updated_at = NOW()
		WHERE id = $1
	`, task.ID)
	if err != nil {
		t.Fatalf("stale retry setup update error = %v", err)
	}

	close(release)

	select {
	case err := <-workerDone:
		if err != nil {
			t.Fatalf("PollOnce() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activity worker did not finish after stale retry simulation")
	}

	staleTask, err := store.GetActivityTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetActivityTask() error = %v", err)
	}
	if staleTask.Status != enginedb.EngineActivityTaskStatusClaimed {
		t.Fatalf("expected stale retry CAS miss to leave task claimed by another worker, got %+v", staleTask)
	}
	if staleTask.ClaimedBy == nil || *staleTask.ClaimedBy != "other-worker" {
		t.Fatalf("expected stale retry CAS miss to preserve other worker claim, got %+v", staleTask)
	}

	waitingRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if waitingRun.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected stale retry CAS miss to leave run waiting, got %+v", waitingRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 2 {
		t.Fatalf("expected stale retry CAS miss to avoid appending retry history, got %+v", historyRows)
	}
}

func createRunWithPendingActivity(
	t *testing.T,
	store *enginestore.Store,
	projectID uuid.UUID,
	instanceKey string,
	activityKey string,
) (enginedb.EngineInstance, enginedb.EngineRun, enginedb.EngineActivityTask) {
	t.Helper()

	return createRunWithPendingActivityConfig(t, store, pendingActivityConfig{
		projectID:   projectID,
		instanceKey: instanceKey,
		activityKey: activityKey,
		maxAttempts: 1,
		waiting:     false,
	})
}

type pendingActivityConfig struct {
	projectID         uuid.UUID
	instanceKey       string
	activityKey       string
	activityType      string
	maxAttempts       int32
	initialBackoffMS  *int64
	maxBackoffMS      *int64
	backoffMultiplier *float64
	waiting           bool
}

func createRunWithPendingActivityConfig(
	t *testing.T,
	store *enginestore.Store,
	cfg pendingActivityConfig,
) (enginedb.EngineInstance, enginedb.EngineRun, enginedb.EngineActivityTask) {
	t.Helper()

	ctx := context.Background()
	if cfg.activityType == "" {
		cfg.activityType = testActivityType
	}
	enginetest.EnsurePlatformProject(t, store.Pool(), cfg.projectID)

	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      cfg.projectID,
		InstanceKey:    cfg.instanceKey,
		DefinitionName: "activity-terminate",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         cfg.projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    "activity-terminate",
		DefinitionVersion: "v1",
		InstanceKey:       cfg.instanceKey,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(started) error = %v", err)
	}
	startedHistory, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  cfg.projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(started) error = %v", err)
	}
	enginetest.SeedProjectionShell(t, store.Pool(), &instance, &run, "activity-terminate", "v1", nil, startedHistory.ID)

	activityPayload, err := enginehistory.MarshalPayload(enginehistory.ActivityScheduledPayload{
		ActivityKey:  cfg.activityKey,
		ActivityType: cfg.activityType,
		Input:        mustJSON(t, map[string]string{"step": "work"}),
	})
	if err != nil {
		t.Fatalf("MarshalPayload(activity scheduled) error = %v", err)
	}
	activityHistory, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  cfg.projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 2,
		EventType:  enginehistory.EventActivityScheduled,
		Payload:    activityPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(activity scheduled) error = %v", err)
	}

	task, err := store.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:         cfg.projectID,
		InstanceID:        instance.ID,
		RunID:             run.ID,
		HistoryID:         &activityHistory.ID,
		ActivityKey:       cfg.activityKey,
		ActivityType:      cfg.activityType,
		Input:             mustJSON(t, map[string]string{"step": "work"}),
		AvailableAt:       time.Now().UTC(),
		ExecutionTarget:   "local",
		MaxAttempts:       cfg.maxAttempts,
		InitialBackoffMs:  cfg.initialBackoffMS,
		MaxBackoffMs:      cfg.maxBackoffMS,
		BackoffMultiplier: cfg.backoffMultiplier,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}

	if !cfg.waiting {
		return instance, run, task
	}

	claimedRun, err := store.ClaimNextRun(ctx, "run-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if claimedRun.ID != run.ID {
		t.Fatalf("expected claimed run %s, got %+v", run.ID, claimedRun)
	}

	waitingFor, err := enginehistory.MarshalPayload(enginehistory.ActivityWait{
		Kind:         enginehistory.WaitKindActivity,
		ActivityKey:  cfg.activityKey,
		ActivityType: cfg.activityType,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(activity wait) error = %v", err)
	}
	waitingRun, err := store.TransitionRunToWaiting(ctx, enginedb.TransitionRunToWaitingParams{
		ID:           run.ID,
		ClaimedBy:    claimedRun.ClaimedBy,
		WaitingFor:   waitingFor,
		CustomStatus: []byte(`{"phase":"activity"}`),
	})
	if err != nil {
		t.Fatalf("TransitionRunToWaiting() error = %v", err)
	}

	return instance, waitingRun, task
}

func createWaitingRunWithPendingActivity(
	t *testing.T,
	store *enginestore.Store,
	cfg pendingActivityConfig,
) (enginedb.EngineInstance, enginedb.EngineRun, enginedb.EngineActivityTask) {
	t.Helper()
	cfg.waiting = true
	return createRunWithPendingActivityConfig(t, store, cfg)
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatalf("condition was not met within %s", timeout)
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
