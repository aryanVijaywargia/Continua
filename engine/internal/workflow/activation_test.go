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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
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
	updatedInstance, err := store.GetInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if updatedInstance.Status != enginedb.EngineInstanceLifecycleStatusFailed {
		t.Fatalf("expected failed instance status, got %+v", updatedInstance)
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

func TestActivatorFailsRunWhenDefinitionVersionIsMissingWithoutStartedHistory(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      enginetest.DefaultPlatformProjectID,
		InstanceKey:    "instance-version-mismatch-no-history",
		DefinitionName: "demo",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         enginetest.DefaultPlatformProjectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v-missing",
		ReadyAt:           time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

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
	if len(historyRows) != 1 || historyRows[0].SequenceNo != 1 || historyRows[0].EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected single workflow.failed event at sequence 1, got %+v", historyRows)
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
	updatedInstance, err := store.GetInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if updatedInstance.Status != enginedb.EngineInstanceLifecycleStatusFailed {
		t.Fatalf("expected failed instance status, got %+v", updatedInstance)
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
	completedInstance, err := store.GetInstance(ctx, run.InstanceID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if completedInstance.Status != enginedb.EngineInstanceLifecycleStatusCompleted {
		t.Fatalf("expected completed instance status, got %+v", completedInstance)
	}
}

func TestActivatorContinueAsNewCreatesNewRunAndInheritedTraceShell(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-continue-as-new",
		definitionName:    "continue-demo",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]any{"cursor": 1}),
	}
	instance, run := createStartedRun(t, store, testCase)
	startedHistory := mustStartedHistory(t, store, run.ID)
	sessionID := insertSession(t, ctx, db.Pool, testCase.projectID, "session-continue-demo")
	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Original Trace", startedHistory.ID)
	updateProjectedTraceShellFields(t, ctx, db.Pool, run.ID, sessionID)

	if _, err := store.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    run.ProjectID,
		InstanceID:   run.InstanceID,
		RunID:        run.ID,
		ActivityKey:  "stale-activity",
		ActivityType: "demo.activity",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
		AvailableAt:  time.Now().Add(-time.Minute),
		MaxAttempts:  1,
	}); err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}
	timerPayload, err := enginehistory.MarshalPayload(enginehistory.TimerScheduledPayload{
		TimerKey: "stale-timer",
		DueAt:    time.Now().Add(time.Hour).UTC(),
	})
	if err != nil {
		t.Fatalf("MarshalPayload(timer) error = %v", err)
	}
	if _, err := store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   run.ProjectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "timer",
		Payload:     timerPayload,
		AvailableAt: time.Now().Add(time.Hour).UTC(),
	}); err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "continue-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			return publicworkflow.ContinueAsNew(json.RawMessage(`{"cursor":2,"phase":"next"}`))
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
		t.Fatalf("GetRun(old) error = %v", err)
	}
	if updatedRun.Status != enginedb.EngineRunLifecycleStatusContinuedAsNew {
		t.Fatalf("expected old run status continued_as_new, got %+v", updatedRun)
	}
	if !updatedRun.CompletedAt.Valid {
		t.Fatalf("expected old run completed_at to be set, got %+v", updatedRun)
	}
	if !updatedRun.ContinuedToRunID.Valid {
		t.Fatalf("expected old run continued_to_run_id, got %+v", updatedRun)
	}

	nextRun, err := store.GetRun(ctx, uuid.UUID(updatedRun.ContinuedToRunID.Bytes))
	if err != nil {
		t.Fatalf("GetRun(new) error = %v", err)
	}
	if nextRun.RunNumber != 2 {
		t.Fatalf("expected next run_number=2, got %+v", nextRun)
	}
	if nextRun.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected next run queued, got %+v", nextRun)
	}
	if !nextRun.ContinuedFromRunID.Valid || uuid.UUID(nextRun.ContinuedFromRunID.Bytes) != run.ID {
		t.Fatalf("expected bidirectional run linkage, got %+v", nextRun)
	}

	instanceAfter, err := store.GetInstance(ctx, run.InstanceID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if instanceAfter.Status != enginedb.EngineInstanceLifecycleStatusActive {
		t.Fatalf("expected instance to remain active, got %+v", instanceAfter)
	}

	orderedRuns, err := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
		InstanceID: run.InstanceID,
		Limit:      10,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("ListRunsByInstance() error = %v", err)
	}
	if len(orderedRuns) < 2 || orderedRuns[0].ID != nextRun.ID || orderedRuns[1].ID != run.ID {
		t.Fatalf("expected latest run ordering by run_number, got %+v", orderedRuns)
	}

	oldHistory, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun(old) error = %v", err)
	}
	lastOldEvent := oldHistory[len(oldHistory)-1]
	if lastOldEvent.EventType != enginehistory.EventWorkflowContinuedAsNew {
		t.Fatalf("expected old run terminal continuation event, got %+v", lastOldEvent)
	}
	var continuedPayload enginehistory.WorkflowContinuedAsNewPayload
	if err := enginehistory.UnmarshalPayload(lastOldEvent.Payload, &continuedPayload); err != nil {
		t.Fatalf("UnmarshalPayload(continued_as_new) error = %v", err)
	}
	assertRawJSONEqual(t, `{"cursor":2,"phase":"next"}`, continuedPayload.Input)

	newHistory, err := store.GetHistoryByRun(ctx, nextRun.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun(new) error = %v", err)
	}
	if len(newHistory) != 1 || newHistory[0].EventType != enginehistory.EventWorkflowStarted {
		t.Fatalf("expected fresh history on continuation run, got %+v", newHistory)
	}
	var newStarted enginehistory.WorkflowStartedPayload
	if err := enginehistory.UnmarshalPayload(newHistory[0].Payload, &newStarted); err != nil {
		t.Fatalf("UnmarshalPayload(workflow.started) error = %v", err)
	}
	assertRawJSONEqual(t, `{"cursor":2,"phase":"next"}`, newStarted.Input)

	var (
		newTraceID                string
		newSessionID              pgtype.UUID
		newTraceName              *string
		newUserID                 *string
		newTags                   []string
		newEnvironment            *string
		newRelease                *string
		newMetadata               []byte
		newTraceInput             []byte
		newTraceOutput            []byte
		newTraceStatus            string
		newTraceRunStatus         *string
		newTraceCustomStatus      []byte
		newTraceWaitState         []byte
		newPendingActivityTasks   *int64
		newPendingInboxItems      *int64
		newDefinitionName         *string
		newDefinitionVersion      *string
		newProjectionState        *string
		newLatestHistoryID        *int64
		newLastProjectedHistoryID *int64
	)
	if err := db.Pool.QueryRow(ctx, `
		SELECT trace_id,
		       session_id,
		       name,
		       user_id,
		       tags,
		       environment,
		       release,
		       metadata,
		       input,
		       output,
		       status,
		       engine_run_status,
		       engine_custom_status,
		       engine_wait_state,
		       engine_pending_activity_tasks,
		       engine_pending_inbox_items,
		       engine_definition_name,
		       engine_definition_version,
		       engine_projection_state,
		       engine_latest_history_id,
		       engine_last_projected_history_id
		FROM public.traces
		WHERE engine_run_id = $1
	`, nextRun.ID).Scan(
		&newTraceID,
		&newSessionID,
		&newTraceName,
		&newUserID,
		&newTags,
		&newEnvironment,
		&newRelease,
		&newMetadata,
		&newTraceInput,
		&newTraceOutput,
		&newTraceStatus,
		&newTraceRunStatus,
		&newTraceCustomStatus,
		&newTraceWaitState,
		&newPendingActivityTasks,
		&newPendingInboxItems,
		&newDefinitionName,
		&newDefinitionVersion,
		&newProjectionState,
		&newLatestHistoryID,
		&newLastProjectedHistoryID,
	); err != nil {
		t.Fatalf("query continuation trace shell: %v", err)
	}
	if newTraceID != engineTraceID(nextRun.ID) {
		t.Fatalf("expected derived continuation trace_id, got %q", newTraceID)
	}
	if !newSessionID.Valid || uuid.UUID(newSessionID.Bytes) != sessionID {
		t.Fatalf("expected inherited session link, got %+v", newSessionID)
	}
	if newTraceName == nil || *newTraceName != "Original Trace" {
		t.Fatalf("expected inherited trace name, got %+v", newTraceName)
	}
	if newUserID == nil || *newUserID != "user-123" {
		t.Fatalf("expected inherited user_id, got %+v", newUserID)
	}
	if len(newTags) != 2 || newTags[0] != "prod" || newTags[1] != "loop" {
		t.Fatalf("expected inherited tags, got %+v", newTags)
	}
	if newEnvironment == nil || *newEnvironment != "staging" {
		t.Fatalf("expected inherited environment, got %+v", newEnvironment)
	}
	if newRelease == nil || *newRelease != "release-2026.04" {
		t.Fatalf("expected inherited release, got %+v", newRelease)
	}
	assertRawJSONEqual(t, `{"source":"projected-trace"}`, newMetadata)
	assertRawJSONEqual(t, `{"cursor":2,"phase":"next"}`, newTraceInput)
	if len(newTraceOutput) != 0 {
		t.Fatalf("expected continuation trace output to stay null, got %s", newTraceOutput)
	}
	if newTraceStatus != "running" {
		t.Fatalf("expected continuation trace running, got %q", newTraceStatus)
	}
	if newTraceRunStatus == nil || *newTraceRunStatus != string(enginedb.EngineRunLifecycleStatusQueued) {
		t.Fatalf("expected continuation trace run status queued, got %+v", newTraceRunStatus)
	}
	if len(newTraceCustomStatus) != 0 || len(newTraceWaitState) != 0 {
		t.Fatalf("expected nil custom/wait state, got custom=%s wait=%s", newTraceCustomStatus, newTraceWaitState)
	}
	if newPendingActivityTasks == nil || *newPendingActivityTasks != 0 || newPendingInboxItems == nil || *newPendingInboxItems != 0 {
		t.Fatalf("expected zero pending work, got activities=%v inbox=%v", newPendingActivityTasks, newPendingInboxItems)
	}
	if newDefinitionName == nil || *newDefinitionName != instance.DefinitionName ||
		newDefinitionVersion == nil || *newDefinitionVersion != nextRun.DefinitionVersion {
		t.Fatalf("expected definition linkage on continuation trace, got name=%v version=%v", newDefinitionName, newDefinitionVersion)
	}
	if newProjectionState == nil || *newProjectionState != publicprojection.StateUpToDate.String() {
		t.Fatalf("expected up_to_date projection state, got %+v", newProjectionState)
	}
	if newLatestHistoryID == nil || *newLatestHistoryID != newHistory[0].ID || newLastProjectedHistoryID == nil || *newLastProjectedHistoryID != newHistory[0].ID {
		t.Fatalf("expected continuation trace checkpoints to match started history, got latest=%v last=%v", newLatestHistoryID, newLastProjectedHistoryID)
	}

	var (
		rootSpanName   string
		rootSpanStatus string
		rootSpanInput  []byte
	)
	if err := db.Pool.QueryRow(ctx, `
		SELECT name, status, input
		FROM public.spans
		WHERE trace_id = (
		        SELECT id
		        FROM public.traces
		        WHERE engine_run_id = $1
		    )
		  AND span_id = $2
	`, nextRun.ID, rootSpanExternalID(nextRun.ID)).Scan(&rootSpanName, &rootSpanStatus, &rootSpanInput); err != nil {
		t.Fatalf("query continuation root span: %v", err)
	}
	if rootSpanName != "Original Trace" || rootSpanStatus != "running" {
		t.Fatalf("unexpected continuation root span summary: name=%q status=%q", rootSpanName, rootSpanStatus)
	}
	assertRawJSONEqual(t, `{"cursor":2,"phase":"next"}`, rootSpanInput)

	cancelledTasks, err := store.ListCancelledActivityTasksByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListCancelledActivityTasksByRun() error = %v", err)
	}
	if len(cancelledTasks) != 1 || cancelledTasks[0].ActivityKey != "stale-activity" {
		t.Fatalf("expected cancelled open activity task, got %+v", cancelledTasks)
	}
	discardedTimers, err := store.ListDiscardedTimerInboxItemsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListDiscardedTimerInboxItemsByRun() error = %v", err)
	}
	if len(discardedTimers) != 1 {
		t.Fatalf("expected discarded timer inbox item, got %+v", discardedTimers)
	}
}

func TestActivatorContinueAsNewFailsWhenProjectedTraceShellIsMissing(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-continue-missing-shell",
		definitionName:    "continue-demo",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]any{"cursor": 1}),
	}
	_, run := createStartedRun(t, store, testCase)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "continue-demo",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			return publicworkflow.ContinueAsNew(map[string]any{"cursor": 2})
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
	err = activator.Activate(ctx, &claimed)
	if !errors.Is(err, enginestore.ErrInvariant) {
		t.Fatalf("expected ErrInvariant for missing projected trace shell, got %v", err)
	}

	updatedRun, getErr := store.GetRun(ctx, run.ID)
	if getErr != nil {
		t.Fatalf("GetRun() error = %v", getErr)
	}
	if updatedRun.Status != enginedb.EngineRunLifecycleStatusRunning {
		t.Fatalf("expected run state rollback on invariant failure, got %+v", updatedRun)
	}
	runs, listErr := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
		InstanceID: run.InstanceID,
		Limit:      10,
		Offset:     0,
	})
	if listErr != nil {
		t.Fatalf("ListRunsByInstance() error = %v", listErr)
	}
	if len(runs) != 1 {
		t.Fatalf("expected no continuation run to be created, got %+v", runs)
	}
}

func TestActivatorContinueAsNewPreservesChainAcrossMultipleHops(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-continue-chain",
		definitionName:    "continue-chain",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]any{"cursor": 1}),
	}
	instance, run := createStartedRun(t, store, testCase)
	startedHistory := mustStartedHistory(t, store, run.ID)
	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Continuation Chain", startedHistory.ID)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "continue-chain",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input struct {
				Cursor int `json:"cursor"`
			}
			if err := ctx.Input(&input); err != nil {
				return err
			}
			if input.Cursor < 3 {
				return publicworkflow.ContinueAsNew(map[string]any{"cursor": input.Cursor + 1})
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	activator := NewActivator(store, registry)
	for {
		claimed, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
		if errors.Is(err, enginestore.ErrNotFound) {
			break
		}
		if err != nil {
			t.Fatalf("ClaimNextRun() error = %v", err)
		}
		if err := activator.Activate(ctx, &claimed); err != nil {
			t.Fatalf("Activate() error = %v", err)
		}
	}

	runs, err := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
		InstanceID: instance.ID,
		Limit:      10,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("ListRunsByInstance() error = %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected three chained runs, got %+v", runs)
	}
	if runs[0].RunNumber != 3 || runs[1].RunNumber != 2 || runs[2].RunNumber != 1 {
		t.Fatalf("expected ordering by descending run_number, got %+v", runs)
	}
	if runs[0].Status != enginedb.EngineRunLifecycleStatusCompleted {
		t.Fatalf("expected final run to complete, got %+v", runs[0])
	}
	if runs[1].Status != enginedb.EngineRunLifecycleStatusContinuedAsNew {
		t.Fatalf("expected middle run to be continued_as_new, got %+v", runs[1])
	}
	if runs[2].Status != enginedb.EngineRunLifecycleStatusContinuedAsNew {
		t.Fatalf("expected first run to be continued_as_new, got %+v", runs[2])
	}
	if !runs[0].ContinuedFromRunID.Valid || uuid.UUID(runs[0].ContinuedFromRunID.Bytes) != runs[1].ID {
		t.Fatalf("expected run 3 to continue from run 2, got %+v", runs[0])
	}
	if !runs[1].ContinuedFromRunID.Valid || uuid.UUID(runs[1].ContinuedFromRunID.Bytes) != runs[2].ID {
		t.Fatalf("expected run 2 to continue from run 1, got %+v", runs[1])
	}
	if !runs[1].ContinuedToRunID.Valid || uuid.UUID(runs[1].ContinuedToRunID.Bytes) != runs[0].ID {
		t.Fatalf("expected run 2 to continue to run 3, got %+v", runs[1])
	}
	if !runs[2].ContinuedToRunID.Valid || uuid.UUID(runs[2].ContinuedToRunID.Bytes) != runs[1].ID {
		t.Fatalf("expected run 1 to continue to run 2, got %+v", runs[2])
	}
}

func TestActivatorPersistsActivityRetryOptions(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-options",
		definitionName:    "activity-options",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"name": "Ada"}),
	}
	_, run := createStartedRun(t, store, testCase)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "activity-options",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}

			var output map[string]string
			return ctx.ActivityWithOptions("fetch", "demo.activity", input, &output, publicworkflow.ActivityOptions{
				RetryPolicy: &publicworkflow.RetryPolicy{
					MaxAttempts:       3,
					InitialBackoff:    1500 * time.Millisecond,
					MaxBackoff:        5 * time.Second,
					BackoffMultiplier: 2.5,
				},
			})
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

	task, err := store.GetActivityTaskByRunAndKey(ctx, enginedb.GetActivityTaskByRunAndKeyParams{
		RunID:       run.ID,
		ActivityKey: "fetch",
	})
	if err != nil {
		t.Fatalf("GetActivityTaskByRunAndKey() error = %v", err)
	}
	if task.MaxAttempts != 3 {
		t.Fatalf("expected max_attempts=3, got %+v", task)
	}
	if task.InitialBackoffMs == nil || *task.InitialBackoffMs != 1500 {
		t.Fatalf("expected initial_backoff_ms=1500, got %+v", task)
	}
	if task.MaxBackoffMs == nil || *task.MaxBackoffMs != 5000 {
		t.Fatalf("expected max_backoff_ms=5000, got %+v", task)
	}
	if task.BackoffMultiplier == nil || *task.BackoffMultiplier != 2.5 {
		t.Fatalf("expected backoff_multiplier=2.5, got %+v", task)
	}
}

func TestActivatorDefaultsActivityRetryOptionsToSingleAttempt(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-activity-default-options",
		definitionName:    "activity-default-options",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"name": "Ada"}),
	}
	_, run := createStartedRun(t, store, testCase)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "activity-default-options",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}

			var output map[string]string
			return ctx.Activity("fetch", "demo.activity", input, &output)
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

	task, err := store.GetActivityTaskByRunAndKey(ctx, enginedb.GetActivityTaskByRunAndKeyParams{
		RunID:       run.ID,
		ActivityKey: "fetch",
	})
	if err != nil {
		t.Fatalf("GetActivityTaskByRunAndKey() error = %v", err)
	}
	if task.MaxAttempts != 1 {
		t.Fatalf("expected max_attempts=1, got %+v", task)
	}
	if task.InitialBackoffMs != nil || task.MaxBackoffMs != nil || task.BackoffMultiplier != nil {
		t.Fatalf("expected nil backoff columns for Activity(), got %+v", task)
	}
}

func TestActivatorRejectsStaleClaimAfterTerminate(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-stale-after-terminate",
		definitionName:    "stale-after-terminate",
		definitionVersion: "v1",
	}
	instance, run := createStartedRun(t, store, testCase)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "stale-after-terminate",
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
		t.Fatalf("ClaimNextRun() error = %v", err)
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
		ProjectID:  run.ProjectID,
		InstanceID: run.InstanceID,
		RunID:      run.ID,
		SequenceNo: 2,
		EventType:  enginehistory.EventWorkflowTerminated,
		Payload:    payload,
	}); err != nil {
		t.Fatalf("AppendHistory(terminated) error = %v", err)
	}
	if _, err := tx.TransitionRunToTerminated(ctx, run.ID); err != nil {
		t.Fatalf("TransitionRunToTerminated() error = %v", err)
	}
	if _, err := tx.UpdateInstanceStatus(ctx, enginedb.UpdateInstanceStatusParams{
		ID:     run.InstanceID,
		Status: enginedb.EngineInstanceLifecycleStatusTerminated,
	}); err != nil {
		t.Fatalf("UpdateInstanceStatus() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	activator := NewActivator(store, registry)
	err = activator.Activate(ctx, &staleClaim)
	if !errors.Is(err, enginestore.ErrStaleClaim) {
		t.Fatalf("expected ErrStaleClaim after terminate, got %v", err)
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

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 2 || historyRows[1].EventType != enginehistory.EventWorkflowTerminated {
		t.Fatalf("expected started + workflow.terminated history, got %+v", historyRows)
	}
}

func TestActivatorWaitingDecisionDoesNotMutateProjectedTraceSummary(t *testing.T) {
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

	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Projected Wait", historyRows[0].ID)

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

	if engineRunStatus != string(enginedb.EngineRunLifecycleStatusQueued) {
		t.Fatalf("expected projected summary to remain unchanged before projector catch-up, got %q", engineRunStatus)
	}
	if pendingActivityTasks != 0 || pendingInboxItems != 0 {
		t.Fatalf("expected no pending work for signal wait, got activity=%d inbox=%d", pendingActivityTasks, pendingInboxItems)
	}
	if len(waitState) != 0 {
		t.Fatalf("expected projected wait state to remain nil before projector catch-up, got %s", waitState)
	}
}

func TestActivatorFailureMovesProjectedTraceIntoCatchingUpWithoutProjectingTerminalSummary(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-projected-failure",
		definitionName:    "missing-definition",
		definitionVersion: "v-missing",
	}
	instance, run := createStartedRun(t, store, testCase)

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Projected Failure", historyRows[0].ID)

	registry, err := NewRegistry()
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

	historyAfterFailure, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	lastEvent := historyAfterFailure[len(historyAfterFailure)-1]
	if lastEvent.EventType != enginehistory.EventWorkflowFailed {
		t.Fatalf("expected terminal workflow.failed event, got %+v", lastEvent)
	}

	var traceStatus string
	var traceRunStatus string
	var traceOutput []byte
	var traceProjectionState string
	var latestHistoryID int64
	var lastProjectedHistoryID int64
	if err := db.Pool.QueryRow(ctx, `
			SELECT status,
			       engine_run_status,
			       output,
			       engine_projection_state,
			       engine_latest_history_id,
			       engine_last_projected_history_id
			FROM public.traces
			WHERE engine_run_id = $1
		`, run.ID).Scan(
		&traceStatus,
		&traceRunStatus,
		&traceOutput,
		&traceProjectionState,
		&latestHistoryID,
		&lastProjectedHistoryID,
	); err != nil {
		t.Fatalf("query projected trace summary: %v", err)
	}
	if traceStatus != "running" || traceRunStatus != "queued" {
		t.Fatalf("expected projected summary to remain unchanged before projector catch-up, got trace=%q run=%q", traceStatus, traceRunStatus)
	}
	if len(traceOutput) != 0 {
		t.Fatalf("expected projected terminal output to remain empty before projector catch-up, got %s", traceOutput)
	}
	if traceProjectionState != publicprojection.StateCatchingUp.String() {
		t.Fatalf("expected projected trace state to move to catching_up, got %q", traceProjectionState)
	}
	if latestHistoryID != lastEvent.ID {
		t.Fatalf("expected latest history checkpoint to advance to %d before catch-up, got %d", lastEvent.ID, latestHistoryID)
	}
	if lastProjectedHistoryID != historyRows[0].ID {
		t.Fatalf("expected projector checkpoint to remain at %d before catch-up, got %d", historyRows[0].ID, lastProjectedHistoryID)
	}
}

func TestActivatorCancellationTransitionsRunToCancelledWithoutProjectingTerminalSummary(t *testing.T) {
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
	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Cancelled Run", historyRows[0].ID)
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
	cancelInbox, err := store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   run.ProjectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "cancel",
		Payload:     cancelPayload,
		AvailableAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateInboxItem(cancel) error = %v", err)
	}
	signalPayload, err := enginehistory.MarshalPayload(enginehistory.SignalReceivedPayload{
		SignalName: "approval",
		Payload:    mustJSON(t, map[string]bool{"approved": true}),
	})
	if err != nil {
		t.Fatalf("MarshalPayload(signal) error = %v", err)
	}
	signalInbox, err := store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   run.ProjectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "signal",
		Payload:     signalPayload,
		AvailableAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateInboxItem(signal) error = %v", err)
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
	updatedInstance, err := store.GetInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if updatedInstance.Status != enginedb.EngineInstanceLifecycleStatusCancelled {
		t.Fatalf("expected cancelled instance status, got %+v", updatedInstance)
	}

	historyAfterCancel, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	lastEvent := historyAfterCancel[len(historyAfterCancel)-1]
	if lastEvent.EventType != enginehistory.EventWorkflowCancelled {
		t.Fatalf("expected terminal workflow.cancelled event, got %+v", lastEvent)
	}

	var traceStatus string
	var traceRunStatus string
	var traceOutput []byte
	var traceProjectionState string
	var latestHistoryID int64
	var lastProjectedHistoryID int64
	if err := db.Pool.QueryRow(ctx, `
			SELECT status,
			       engine_run_status,
			       output,
			       engine_projection_state,
			       engine_latest_history_id,
			       engine_last_projected_history_id
			FROM public.traces
			WHERE engine_run_id = $1
		`, run.ID).Scan(
		&traceStatus,
		&traceRunStatus,
		&traceOutput,
		&traceProjectionState,
		&latestHistoryID,
		&lastProjectedHistoryID,
	); err != nil {
		t.Fatalf("query projected trace summary: %v", err)
	}
	if traceStatus != "running" || traceRunStatus != "queued" {
		t.Fatalf("expected projected summary to remain unchanged before projector catch-up, got trace=%q run=%q", traceStatus, traceRunStatus)
	}
	if len(traceOutput) != 0 {
		t.Fatalf("expected projected terminal output to remain empty before projector catch-up, got %s", traceOutput)
	}
	if traceProjectionState != "up_to_date" {
		t.Fatalf("expected projected trace state to remain unchanged before projector catch-up, got %q", traceProjectionState)
	}
	if latestHistoryID != historyRows[0].ID {
		t.Fatalf("expected latest history checkpoint to remain at %d before catch-up, got %d", historyRows[0].ID, latestHistoryID)
	}
	if lastProjectedHistoryID != historyRows[0].ID {
		t.Fatalf("expected projector checkpoint to stay at %d before catch-up, got %d", historyRows[0].ID, lastProjectedHistoryID)
	}

	var rootSpanStatus string
	if err := db.Pool.QueryRow(ctx, `
		SELECT status
		FROM public.spans
		WHERE trace_id = (
		        SELECT id
		        FROM public.traces
		        WHERE engine_run_id = $1
		    )
		  AND span_id = $2
	`, run.ID, "engine:root:"+run.ID.String()).Scan(&rootSpanStatus); err != nil {
		t.Fatalf("query projected root span: %v", err)
	}
	if rootSpanStatus != "running" {
		t.Fatalf("expected projected root span to remain running before projector catch-up, got %q", rootSpanStatus)
	}

	var cancelInboxStatus enginedb.EngineInboxStatus
	if err := db.Pool.QueryRow(ctx, `
		SELECT status
		FROM engine.inbox
		WHERE id = $1
	`, cancelInbox.ID).Scan(&cancelInboxStatus); err != nil {
		t.Fatalf("query cancel inbox status: %v", err)
	}
	if cancelInboxStatus != enginedb.EngineInboxStatusProcessed {
		t.Fatalf("expected consumed cancel inbox to be processed, got %q", cancelInboxStatus)
	}

	var signalInboxStatus enginedb.EngineInboxStatus
	if err := db.Pool.QueryRow(ctx, `
		SELECT status
		FROM engine.inbox
		WHERE id = $1
	`, signalInbox.ID).Scan(&signalInboxStatus); err != nil {
		t.Fatalf("query signal inbox status: %v", err)
	}
	if signalInboxStatus != enginedb.EngineInboxStatusDiscarded {
		t.Fatalf("expected unrelated signal inbox to be discarded, got %q", signalInboxStatus)
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

//nolint:revive // Keep testing.T first in test helper signatures.
func insertProjectedTraceShell(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
	instance enginedb.EngineInstance,
	run enginedb.EngineRun,
	traceName string,
	latestHistoryID int64,
) {
	t.Helper()

	if _, err := pool.Exec(ctx, `
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
	`, uuidOrFatal(t), run.ProjectID, "engine:"+run.ID.String(), traceName, run.ID, instance.InstanceKey, instance.DefinitionName, run.DefinitionVersion, latestHistoryID); err != nil {
		t.Fatalf("insert projected trace shell: %v", err)
	}
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

func mustStartedHistory(t *testing.T, store *enginestore.Store, runID uuid.UUID) enginedb.EngineHistory {
	t.Helper()

	historyRows, err := store.GetHistoryByRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) == 0 {
		t.Fatalf("expected started history row for run %s", runID)
	}
	return historyRows[0]
}

//nolint:revive // Keep testing.T first in test helper signatures.
func insertSession(
	t *testing.T,
	ctx context.Context,
	pool interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	projectID uuid.UUID,
	externalID string,
) uuid.UUID {
	t.Helper()

	var sessionID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO public.sessions (project_id, external_id, name, user_id, metadata)
		VALUES ($1, $2, 'Session Name', 'session-user', '{"team":"runtime"}')
		RETURNING id
	`, projectID, externalID).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return sessionID
}

//nolint:revive // Keep testing.T first in test helper signatures.
func updateProjectedTraceShellFields(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
	runID uuid.UUID,
	sessionID uuid.UUID,
) {
	t.Helper()

	if _, err := pool.Exec(ctx, `
		UPDATE public.traces
		SET session_id = $2,
		    name = 'Original Trace',
		    user_id = 'user-123',
		    tags = ARRAY['prod', 'loop'],
		    environment = 'staging',
		    release = 'release-2026.04',
		    metadata = '{"source":"projected-trace"}'
		WHERE engine_run_id = $1
	`, runID, sessionID); err != nil {
		t.Fatalf("update projected trace shell fields: %v", err)
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

func assertRawJSONEqual(t *testing.T, expected string, actual json.RawMessage) {
	t.Helper()

	expectedCanonical := canonicalRawJSON(t, json.RawMessage(expected))
	actualCanonical := canonicalRawJSON(t, actual)
	if actualCanonical != expectedCanonical {
		t.Fatalf("unexpected JSON payload: got %s, want %s", actual, expected)
	}
}

func canonicalRawJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%s) error = %v", raw, err)
	}
	return string(normalized)
}

func uuidOrFatal(t *testing.T) uuid.UUID {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7() error = %v", err)
	}
	return id
}
