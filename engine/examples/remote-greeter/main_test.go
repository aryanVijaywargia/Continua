package main

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
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestRemoteGreeterExampleCompletesViaSimulatedRemoteWorker(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{remoteGreeterDefinition()},
		Activities:              nil,
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	ctx := context.Background()
	store := enginestore.New(db.Pool)
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "remote-greeter-e2e",
		DefinitionName: remoteGreeterWorkflowName,
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: remoteGreeterWorkflowVersion,
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	input := mustJSON(t, map[string]string{"name": "Ada"})
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    remoteGreeterWorkflowName,
		DefinitionVersion: remoteGreeterWorkflowVersion,
		InstanceKey:       "remote-greeter-e2e",
		Input:             input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow started) error = %v", err)
	}
	startedEvent, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(workflow started) error = %v", err)
	}

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	traceName := remoteGreeterWorkflowName
	if err := publicprojection.NewWriter(tx.Tx()).CreateTraceShell(ctx, &instance, &run, &publicprojection.TraceShellSeed{}, &startedEvent, input, &traceName); err != nil {
		t.Fatalf("CreateTraceShell() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stopped := false
	go func() {
		done <- rt.Run(runCtx)
	}()
	defer func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop within 5s after test cleanup cancellation")
		}
	}()

	task := waitForActivityTask(t, store, run.ID)
	if task.ExecutionTarget != "remote" {
		t.Fatalf("activity task execution_target = %q, want %q", task.ExecutionTarget, "remote")
	}
	if task.ActivityType != composeGreetingActivityType {
		t.Fatalf("activity task type = %q, want %q", task.ActivityType, composeGreetingActivityType)
	}
	if task.Status != enginedb.EngineActivityTaskStatusQueued {
		t.Fatalf("pending activity task status = %q, want %q", task.Status, enginedb.EngineActivityTaskStatusQueued)
	}
	if task.ClaimedBy != nil {
		t.Fatalf("pending remote activity task claimed_by = %q, want nil", *task.ClaimedBy)
	}

	const workerID = "py-sim-worker"
	claimed, err := store.ClaimRemoteActivityTasks(ctx, projectID, workerID, []string{composeGreetingActivityType}, 1, time.Minute)
	if err != nil {
		t.Fatalf("ClaimRemoteActivityTasks() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("ClaimRemoteActivityTasks() claimed %d tasks, want 1", len(claimed))
	}
	var activityInput struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(claimed[0].Input, &activityInput); err != nil {
		t.Fatalf("json.Unmarshal(activity input) error = %v; raw = %s", err, string(claimed[0].Input))
	}
	if activityInput.Name != "Ada" {
		t.Fatalf("claimed activity input name = %q, want %q", activityInput.Name, "Ada")
	}

	output := mustJSON(t, map[string]string{"greeting": "hello, Ada"})
	if _, err := store.CompleteRemoteActivityTask(ctx, projectID, claimed[0].ID, workerID, output); err != nil {
		t.Fatalf("CompleteRemoteActivityTask() error = %v", err)
	}
	wake, err := store.WakeWaitingRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("WakeWaitingRun() error = %v", err)
	}
	if !wake.Applied {
		t.Fatalf("WakeWaitingRun() applied = false; run status = %s", wake.Run.Status)
	}

	completedRun := waitForCompletedRun(t, store, instance.ID)
	var result struct {
		Greeting string `json:"greeting"`
	}
	if err := json.Unmarshal(completedRun.Result, &result); err != nil {
		t.Fatalf("json.Unmarshal(run.Result) error = %v; raw = %s", err, string(completedRun.Result))
	}
	if result.Greeting != "hello, Ada" {
		t.Fatalf("run result greeting = %q, want %q", result.Greeting, "hello, Ada")
	}

	historyRows, err := store.GetHistoryByRun(ctx, completedRun.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	eventTypes := make([]string, 0, len(historyRows))
	for _, row := range historyRows {
		eventTypes = append(eventTypes, row.EventType)
	}
	wantSubsequence := []string{
		enginehistory.EventWorkflowStarted,
		enginehistory.EventActivityScheduled,
		enginehistory.EventActivityCompleted,
		enginehistory.EventWorkflowCompleted,
	}
	if !containsSubsequence(eventTypes, wantSubsequence) {
		t.Fatalf("history event types = %v, want subsequence %v", eventTypes, wantSubsequence)
	}

	cancel()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("Runtime.Run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run() did not stop within 5s after cancellation")
	}
}

func waitForActivityTask(t *testing.T, store *enginestore.Store, runID uuid.UUID) enginedb.EngineActivityTask {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		tasks, err := store.ListActivityTasksByRun(ctx, runID)
		if err != nil {
			t.Fatalf("ListActivityTasksByRun() error = %v", err)
		}
		if len(tasks) > 0 {
			return tasks[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("activity task was not created within 20s")
	return enginedb.EngineActivityTask{}
}

func waitForCompletedRun(t *testing.T, store *enginestore.Store, instanceID uuid.UUID) enginedb.EngineRun {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(20 * time.Second)
	var lastStatus enginedb.EngineRunLifecycleStatus
	for time.Now().Before(deadline) {
		runs, err := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
			InstanceID: instanceID,
			Limit:      1,
		})
		if err != nil {
			t.Fatalf("ListRunsByInstance() error = %v", err)
		}
		if len(runs) > 0 {
			lastStatus = runs[0].Status
			if runs[0].Status == enginedb.EngineRunLifecycleStatusCompleted {
				return runs[0]
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run did not complete within 20s; last observed status = %s", lastStatus)
	return enginedb.EngineRun{}
}

func containsSubsequence(haystack, needle []string) bool {
	if len(needle) == 0 {
		return true
	}
	next := 0
	for _, value := range haystack {
		if value == needle[next] {
			next++
			if next == len(needle) {
				return true
			}
		}
	}
	return false
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
