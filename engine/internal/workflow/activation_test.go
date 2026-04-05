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
		projectID:         enginetest.DefaultPlatformProjectID,
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
		projectID:         enginetest.DefaultPlatformProjectID,
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
		projectID:         enginetest.DefaultPlatformProjectID,
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

func TestActivatorWaitingDecisionRefreshesProjectedTraceSummary(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-projected-wait",
		definitionName:    "projected-wait",
		definitionVersion: "v1",
	}
	instance, run := createStartedRun(t, store, testCase)

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 1 {
		t.Fatalf("expected started history row, got %+v", historyRows)
	}

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO public.traces (
		    id,
		    project_id,
		    trace_id,
		    name,
		    status,
		    start_time,
		    engine_run_id,
		    engine_instance_key,
		    engine_run_status,
		    engine_pending_activity_tasks,
		    engine_pending_inbox_items,
		    engine_definition_name,
		    engine_definition_version,
		    engine_projection_state,
		    engine_latest_history_id,
		    engine_last_projected_history_id,
		    engine_projection_updated_at
		)
		VALUES (
		    $1,
		    $2,
		    $3,
		    $4,
		    'running',
		    NOW(),
		    $5,
		    $6,
		    'queued',
		    0,
		    0,
		    $7,
		    $8,
		    'up_to_date',
		    $9,
		    $9,
		    NOW()
		)
	`, uuidOrFatal(t), testCase.projectID, "engine:"+run.ID.String(), "Projected Wait", run.ID, instance.InstanceKey, instance.DefinitionName, run.DefinitionVersion, historyRows[0].ID); err != nil {
		t.Fatalf("insert projected trace shell: %v", err)
	}

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "projected-wait",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var signal map[string]string
			return ctx.ReceiveSignal("approval", &signal)
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

	var engineRunStatus string
	var waitState []byte
	var pendingActivityTasks int64
	var pendingInboxItems int64
	if err := db.Pool.QueryRow(ctx, `
		SELECT engine_run_status,
		       engine_wait_state,
		       engine_pending_activity_tasks,
		       engine_pending_inbox_items
		FROM public.traces
		WHERE engine_run_id = $1
	`, run.ID).Scan(&engineRunStatus, &waitState, &pendingActivityTasks, &pendingInboxItems); err != nil {
		t.Fatalf("query projected trace summary: %v", err)
	}

	if engineRunStatus != string(enginedb.EngineRunLifecycleStatusWaiting) {
		t.Fatalf("expected projected waiting run status, got %q", engineRunStatus)
	}
	if pendingActivityTasks != 0 || pendingInboxItems != 0 {
		t.Fatalf("expected no pending work for signal wait, got activity=%d inbox=%d", pendingActivityTasks, pendingInboxItems)
	}

	var waitPayload map[string]any
	if err := json.Unmarshal(waitState, &waitPayload); err != nil {
		t.Fatalf("json.Unmarshal(waitState) error = %v", err)
	}
	if waitPayload["kind"] != "signal" || waitPayload["signal_name"] != "approval" {
		t.Fatalf("unexpected projected wait state: %+v", waitPayload)
	}
}

func TestActivatorCancellationTransitionsRunToCancelledAndProjectsTerminalSummary(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-cancelled",
		definitionName:    "cancelled-workflow",
		definitionVersion: "v1",
	}
	instance, run := createStartedRun(t, store, testCase)

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO public.traces (
		    id,
		    project_id,
		    trace_id,
		    name,
		    status,
		    start_time,
		    engine_run_id,
		    engine_instance_key,
		    engine_run_status,
		    engine_pending_activity_tasks,
		    engine_pending_inbox_items,
		    engine_definition_name,
		    engine_definition_version,
		    engine_projection_state,
		    engine_latest_history_id,
		    engine_last_projected_history_id,
		    engine_projection_updated_at
		)
		VALUES (
		    $1,
		    $2,
		    $3,
		    $4,
		    'running',
		    NOW(),
		    $5,
		    $6,
		    'queued',
		    0,
		    0,
		    $7,
		    $8,
		    'up_to_date',
		    $9,
		    $9,
		    NOW()
		)
	`, uuidOrFatal(t), testCase.projectID, "engine:"+run.ID.String(), "Cancelled Run", run.ID, instance.InstanceKey, instance.DefinitionName, run.DefinitionVersion, historyRows[0].ID); err != nil {
		t.Fatalf("insert projected trace shell: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    name,
		    type,
		    status,
		    level,
		    start_time,
		    depth
		)
		SELECT project_id, id, $2, 'Cancelled Run', 'chain', 'running', 'default', NOW(), 0
		FROM public.traces
		WHERE engine_run_id = $1
	`, run.ID, "engine:root:"+run.ID.String()); err != nil {
		t.Fatalf("insert projected root span: %v", err)
	}

	cancelPayload, err := enginehistory.MarshalPayload(enginehistory.CancelRequestedPayload{})
	if err != nil {
		t.Fatalf("MarshalPayload(cancel) error = %v", err)
	}
	if _, err := store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   run.ProjectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "cancel",
		Payload:     cancelPayload,
		AvailableAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateInboxItem(cancel) error = %v", err)
	}

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "cancelled-workflow",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			if ctx.CancellationRequested() {
				return publicworkflow.ErrCancelled
			}
			return ctx.SetResult(map[string]bool{"ok": true})
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
	if updatedRun.Status != enginedb.EngineRunLifecycleStatusCancelled {
		t.Fatalf("expected cancelled run status, got %+v", updatedRun)
	}
	if updatedRun.LastErrorCode == nil || *updatedRun.LastErrorCode != "cancelled" {
		t.Fatalf("expected cancelled error code, got %+v", updatedRun)
	}

	var traceStatus string
	var traceRunStatus string
	var traceOutput []byte
	if err := db.Pool.QueryRow(ctx, `
		SELECT status, engine_run_status, output
		FROM public.traces
		WHERE engine_run_id = $1
	`, run.ID).Scan(&traceStatus, &traceRunStatus, &traceOutput); err != nil {
		t.Fatalf("query projected trace summary: %v", err)
	}
	if traceStatus != "cancelled" || traceRunStatus != "cancelled" {
		t.Fatalf("expected cancelled projected summary, got trace=%q run=%q", traceStatus, traceRunStatus)
	}

	var outputPayload map[string]any
	if err := json.Unmarshal(traceOutput, &outputPayload); err != nil {
		t.Fatalf("json.Unmarshal(trace output) error = %v", err)
	}
	if outputPayload["status"] != "cancelled" || outputPayload["error_code"] != "cancelled" {
		t.Fatalf("unexpected cancelled terminal payload %+v", outputPayload)
	}
}

func TestActivatorLateSignalWakeIsNotStranded(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
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
