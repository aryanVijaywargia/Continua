package activity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestWorkerLateCompletionAfterTerminateReturnsNoOp(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	projectID := uuid.New()
	instance, run, task := createRunWithPendingActivity(t, store, projectID, "instance-activity-terminate", "ship-order")

	blocked := make(chan struct{})
	release := make(chan struct{})

	registry, err := NewRegistry(map[string]Handler{
		"demo.activity": func(context.Context, json.RawMessage) (json.RawMessage, error) {
			close(blocked)
			<-release
			return []byte(`{"ok":true}`), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	worker := NewWorker(store, registry, time.Minute)
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

func createRunWithPendingActivity(
	t *testing.T,
	store *enginestore.Store,
	projectID uuid.UUID,
	instanceKey string,
	activityKey string,
) (enginedb.EngineInstance, enginedb.EngineRun, enginedb.EngineActivityTask) {
	t.Helper()

	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    instanceKey,
		DefinitionName: "activity-terminate",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
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
		InstanceKey:       instanceKey,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(started) error = %v", err)
	}
	if _, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	}); err != nil {
		t.Fatalf("AppendHistory(started) error = %v", err)
	}

	activityPayload, err := enginehistory.MarshalPayload(enginehistory.ActivityScheduledPayload{
		ActivityKey:  activityKey,
		ActivityType: "demo.activity",
		Input:        mustJSON(t, map[string]string{"step": "work"}),
	})
	if err != nil {
		t.Fatalf("MarshalPayload(activity scheduled) error = %v", err)
	}
	activityHistory, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
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
		ProjectID:    projectID,
		InstanceID:   instance.ID,
		RunID:        run.ID,
		HistoryID:    &activityHistory.ID,
		ActivityKey:  activityKey,
		ActivityType: "demo.activity",
		Input:        mustJSON(t, map[string]string{"step": "work"}),
		AvailableAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}

	return instance, run, task
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
