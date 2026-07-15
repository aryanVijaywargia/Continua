package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
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

func TestRuntimeDrainCompletesInFlightActivityWithinGrace(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)

	definition := drainWorkflowDefinition()
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var handlerCancelled atomic.Bool
	activities := map[string]engineruntime.ActivityHandler{
		"draintest.block": func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			startedOnce.Do(func() { close(started) })
			select {
			case <-release:
				return json.RawMessage(`{"value":"released"}`), nil
			case <-ctx.Done():
				handlerCancelled.Store(true)
				return nil, ctx.Err()
			}
		},
	}
	store, instance, run := seedDrainRun(t, db, projectID, "drain-completes-within-grace")

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{definition},
		Activities:              activities,
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 100 * time.Millisecond,
		ShutdownGrace:           5 * time.Second,
		RunLeaseTTL:             time.Minute,
		ActivityLeaseTTL:        5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rt.Run(runCtx) }()
	stopped := false
	defer func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop within 5s during test cleanup")
		}
	}()

	waitForDrainActivityStart(t, started)
	cancel()
	time.Sleep(100 * time.Millisecond)
	close(release)
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("Runtime.Run() after drain cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run() did not finish draining within 5s")
	}
	if handlerCancelled.Load() {
		t.Fatal("in-flight activity handler observed context cancellation before voluntary completion")
	}

	tasks, err := store.ListActivityTasksByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListActivityTasksByRun() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("ListActivityTasksByRun() returned %d tasks, want 1", len(tasks))
	}
	if tasks[0].Status != enginedb.EngineActivityTaskStatusCompleted {
		t.Fatalf("activity task status after drain = %s, want completed", tasks[0].Status)
	}

	secondRuntime, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{definition},
		Activities:              activities,
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 100 * time.Millisecond,
		ShutdownGrace:           5 * time.Second,
		RunLeaseTTL:             time.Minute,
		ActivityLeaseTTL:        5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runtime.New(second runtime) error = %v", err)
	}
	secondCtx, secondCancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { secondDone <- secondRuntime.Run(secondCtx) }()
	secondStopped := false
	defer func() {
		if secondStopped {
			return
		}
		secondCancel()
		select {
		case <-secondDone:
		case <-time.After(5 * time.Second):
			t.Error("second runtime did not stop within 5s during test cleanup")
		}
	}()

	completedRun := waitForDrainCompletedRun(t, store, instance.ID, 10*time.Second)
	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(completedRun.Result, &result); err != nil {
		t.Fatalf("json.Unmarshal(run.Result) error = %v; raw = %s", err, string(completedRun.Result))
	}
	if result.Value != "released" {
		t.Fatalf("completed run result value = %q, want %q", result.Value, "released")
	}

	secondCancel()
	select {
	case err := <-secondDone:
		secondStopped = true
		if err != nil {
			t.Fatalf("second Runtime.Run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second Runtime.Run() did not stop within 5s")
	}
}

func TestRuntimeDrainReleasesLeasesAtGraceExpiry(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)

	started := make(chan struct{})
	activities := map[string]engineruntime.ActivityHandler{
		"draintest.block": func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	store, _, run := seedDrainRun(t, db, projectID, "drain-releases-at-expiry")

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{drainWorkflowDefinition()},
		Activities:              activities,
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 100 * time.Millisecond,
		ShutdownGrace:           200 * time.Millisecond,
		RunLeaseTTL:             time.Minute,
		ActivityLeaseTTL:        5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rt.Run(runCtx) }()
	stopped := false
	defer func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop within 5s during test cleanup")
		}
	}()

	waitForDrainActivityStart(t, started)
	cancel()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("Runtime.Run() after grace expiry error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run() did not return within 5s after grace expiry")
	}

	tasks, err := store.ListActivityTasksByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListActivityTasksByRun() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("ListActivityTasksByRun() returned %d tasks, want 1", len(tasks))
	}
	task := tasks[0]
	if task.Status != enginedb.EngineActivityTaskStatusQueued || task.ClaimedBy != nil || task.LeaseExpiresAt.Valid {
		t.Fatalf("activity task was not released at grace expiry: %+v", task)
	}

	reclaimed, err := enginestore.New(db.Pool).ClaimNextActivityTask(
		context.Background(),
		"post-drain-worker",
		time.Minute,
	)
	if err != nil {
		t.Fatalf("ClaimNextActivityTask(post-drain-worker) error = %v", err)
	}
	if reclaimed.ID != task.ID {
		t.Fatalf("ClaimNextActivityTask(post-drain-worker) ID = %s, want released task %s", reclaimed.ID, task.ID)
	}
}

func drainWorkflowDefinition() workflow.Definition {
	return workflow.Definition{
		Name:    "draintest.wf",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var output struct {
				Value string `json:"value"`
			}
			if err := ctx.Activity("block", "draintest.block", struct{}{}, &output); err != nil {
				return err
			}
			return ctx.SetResult(output)
		},
	}
}

func seedDrainRun(
	t *testing.T,
	db *enginetest.TestDatabase,
	projectID uuid.UUID,
	instanceKey string,
) (*enginestore.Store, enginedb.EngineInstance, enginedb.EngineRun) {
	t.Helper()

	ctx := context.Background()
	store := enginestore.New(db.Pool)
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    instanceKey,
		DefinitionName: "draintest.wf",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	input := json.RawMessage(`{}`)
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    "draintest.wf",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
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
	defer func() { _ = tx.Rollback(ctx) }()
	traceName := "draintest.wf"
	if err := publicprojection.NewWriter(tx.Tx()).CreateTraceShell(
		ctx,
		&instance,
		&run,
		&publicprojection.TraceShellSeed{},
		&startedEvent,
		input,
		&traceName,
	); err != nil {
		t.Fatalf("CreateTraceShell() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	return store, instance, run
}

func waitForDrainActivityStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("activity handler did not start within 10s")
	}
}

func waitForDrainCompletedRun(
	t *testing.T,
	store *enginestore.Store,
	instanceID uuid.UUID,
	timeout time.Duration,
) enginedb.EngineRun {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(timeout)
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
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("run did not complete within %s; last observed status = %s", timeout, lastStatus)
	return enginedb.EngineRun{}
}
