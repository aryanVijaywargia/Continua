package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestActivatorFailsRunWhenDefinitionVersionIsMissing(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	instance, run := createStartedRun(t, store, workflowTestCase{
		projectID:         uuidOrFatal(t),
		instanceKey:       "instance-version-mismatch",
		definitionName:    "demo",
		definitionVersion: "v-missing",
	})

	claimed, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}

	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	activator := NewActivator(store, registry)
	if err := activator.Activate(ctx, &claimed); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	updatedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updatedRun.Status != enginedb.EngineRunLifecycleStatusFailed {
		t.Fatalf("expected failed run status, got %+v", updatedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	lastEvent := historyRows[len(historyRows)-1]
	if lastEvent.EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected terminal workflow.failed event, got %+v", lastEvent)
	}
	if instance.InstanceKey != "instance-version-mismatch" {
		t.Fatalf("unexpected instance returned from setup: %+v", instance)
	}
}

func TestActivatorPersistsReplayMismatchFailure(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         uuidOrFatal(t),
		instanceKey:       "instance-replay-mismatch",
		definitionName:    "demo",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"name": "Ada"}),
	}
	instance, run := createStartedRun(t, store, testCase)
	appendHistoryEvent(t, store, testCase.projectID, instance.ID, run.ID, 2, enginehistory.EventActivityScheduled, enginehistory.ActivityScheduledPayload{
		ActivityKey:  "different",
		ActivityType: "demo.activity",
		Input:        testCase.input,
	})

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}
			var output map[string]string
			return ctx.Activity("expected", "demo.activity", input, &output)
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	claimed, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}

	activator := NewActivator(store, registry)
	if err := activator.Activate(ctx, &claimed); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	updatedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updatedRun.Status != enginedb.EngineRunLifecycleStatusFailed {
		t.Fatalf("expected failed run status, got %+v", updatedRun)
	}
	if updatedRun.LastErrorCode == nil || *updatedRun.LastErrorCode != "replay_mismatch" {
		t.Fatalf("expected replay_mismatch last_error_code, got %+v", updatedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 4 {
		t.Fatalf("expected started + scheduled + replay mismatch + workflow failed, got %+v", historyRows)
	}
	if historyRows[2].EventType != enginehistory.EventWorkflowReplayMismatch {
		t.Fatalf("expected workflow.replay_mismatch event, got %+v", historyRows[2])
	}
	if historyRows[3].EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected terminal workflow.failed event, got %+v", historyRows[3])
	}
}

func TestActivatorRejectsStaleClaimBeforeAppendingHistory(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         uuidOrFatal(t),
		instanceKey:       "instance-stale-claim",
		definitionName:    "stale-claim",
		definitionVersion: "v1",
	}
	_, run := createStartedRun(t, store, testCase)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "stale-claim",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			return ctx.SetResult(map[string]bool{"ok": true})
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	staleClaim, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() first claim error = %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE engine.runs
		SET lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, run.ID); err != nil {
		t.Fatalf("expire run lease: %v", err)
	}
	freshClaim, err := store.ClaimNextRun(ctx, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() reclaimed error = %v", err)
	}

	activator := NewActivator(store, registry)
	err = activator.Activate(ctx, &staleClaim)
	if !errors.Is(err, enginestore.ErrStaleClaim) {
		t.Fatalf("expected ErrStaleClaim, got %v", err)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 1 || historyRows[0].EventType != enginehistory.EventWorkflowStarted {
		t.Fatalf("expected no history mutation from stale activation, got %+v", historyRows)
	}

	if err := activator.Activate(ctx, &freshClaim); err != nil {
		t.Fatalf("Activate() fresh claim error = %v", err)
	}

	completedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if completedRun.Status != enginedb.EngineRunLifecycleStatusCompleted {
		t.Fatalf("expected fresh claim to complete run, got %+v", completedRun)
	}
}

func TestActivatorLateSignalWakeIsNotStranded(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         uuidOrFatal(t),
		instanceKey:       "instance-late-signal",
		definitionName:    "late-signal",
		definitionVersion: "v1",
	}
	_, run := createStartedRun(t, store, testCase)

	blocked := make(chan struct{})
	release := make(chan struct{})
	var blockedOnce sync.Once
	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "late-signal",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			blockedOnce.Do(func() {
				close(blocked)
			})
			<-release

			var signal map[string]string
			if err := ctx.ReceiveSignal("approval", &signal); err != nil {
				return err
			}
			return ctx.SetResult(signal)
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	claimed, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}

	activator := NewActivator(store, registry)
	activationDone := make(chan error, 1)
	go func() {
		activationDone <- activator.Activate(ctx, &claimed)
	}()

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("workflow did not reach blocking point before signal")
	}

	signalBody := mustJSON(t, map[string]string{"approval": "yes"})
	type signalResult struct {
		wakeApplied bool
		err         error
	}
	signalDone := make(chan signalResult, 1)
	go func() {
		tx, err := store.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			signalDone <- signalResult{err: err}
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		payload, err := enginehistory.MarshalPayload(enginehistory.SignalReceivedPayload{
			SignalName: "approval",
			Payload:    signalBody,
		})
		if err != nil {
			signalDone <- signalResult{err: err}
			return
		}

		if _, err := tx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
			ProjectID:   run.ProjectID,
			InstanceID:  run.InstanceID,
			RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
			Kind:        "signal",
			Payload:     payload,
			AvailableAt: time.Now(),
		}); err != nil {
			signalDone <- signalResult{err: err}
			return
		}

		wake, err := tx.WakeWaitingRun(ctx, run.ID)
		if err != nil {
			signalDone <- signalResult{err: err}
			return
		}
		if err := tx.Commit(ctx); err != nil {
			signalDone <- signalResult{err: err}
			return
		}
		signalDone <- signalResult{wakeApplied: wake.Applied}
	}()

	close(release)

	select {
	case err := <-activationDone:
		if err != nil {
			t.Fatalf("Activate() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activation did not finish")
	}

	var control signalResult
	select {
	case control = <-signalDone:
		if control.err != nil {
			t.Fatalf("signal transaction error = %v", control.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal transaction did not finish")
	}
	if !control.wakeApplied {
		t.Fatal("expected late signal wake to requeue the waiting run")
	}

	queuedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if queuedRun.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected queued run after late signal wake, got %+v", queuedRun)
	}

	reclaimed, err := store.ClaimNextRun(ctx, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() second activation error = %v", err)
	}
	if err := activator.Activate(ctx, &reclaimed); err != nil {
		t.Fatalf("Activate() second pass error = %v", err)
	}

	completedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() completed error = %v", err)
	}
	if completedRun.Status != enginedb.EngineRunLifecycleStatusCompleted {
		t.Fatalf("expected completed run after second activation, got %+v", completedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	eventTypes := make([]string, 0, len(historyRows))
	for _, row := range historyRows {
		eventTypes = append(eventTypes, row.EventType)
	}
	want := []string{
		enginehistory.EventWorkflowStarted,
		enginehistory.EventSignalReceived,
		enginehistory.EventWorkflowCompleted,
	}
	if len(eventTypes) != len(want) {
		t.Fatalf("unexpected history length: got %v want %v", eventTypes, want)
	}
	for index := range want {
		if eventTypes[index] != want[index] {
			t.Fatalf("unexpected history order: got %v want %v", eventTypes, want)
		}
	}
}

type workflowTestCase struct {
	projectID         uuid.UUID
	instanceKey       string
	definitionName    string
	definitionVersion string
	input             json.RawMessage
}

func createStartedRun(
	t *testing.T,
	store *enginestore.Store,
	testCase workflowTestCase,
) (enginedb.EngineInstance, enginedb.EngineRun) {
	t.Helper()

	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      testCase.projectID,
		InstanceKey:    testCase.instanceKey,
		DefinitionName: testCase.definitionName,
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         testCase.projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: testCase.definitionVersion,
		ReadyAt:           time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	appendHistoryEvent(t, store, testCase.projectID, instance.ID, run.ID, 1, enginehistory.EventWorkflowStarted, enginehistory.WorkflowStartedPayload{
		DefinitionName:    testCase.definitionName,
		DefinitionVersion: testCase.definitionVersion,
		InstanceKey:       testCase.instanceKey,
		Input:             testCase.input,
	})

	return instance, run
}

func appendHistoryEvent(
	t *testing.T,
	store *enginestore.Store,
	projectID uuid.UUID,
	instanceID uuid.UUID,
	runID uuid.UUID,
	sequenceNo int32,
	eventType string,
	payload any,
) {
	t.Helper()

	raw, err := enginehistory.MarshalPayload(payload)
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}
	if _, err := store.AppendHistory(context.Background(), enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instanceID,
		RunID:      runID,
		SequenceNo: sequenceNo,
		EventType:  eventType,
		Payload:    raw,
	}); err != nil {
		t.Fatalf("AppendHistory() error = %v", err)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func uuidOrFatal(t *testing.T) uuid.UUID {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7() error = %v", err)
	}
	return id
}
