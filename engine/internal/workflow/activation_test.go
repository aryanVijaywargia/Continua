package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestActivatorSchedulesChildWorkflowAndCreatesLineage(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-parent-child",
		definitionName:    "checkout",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-123"}),
	}
	instance, run := createStartedRun(t, store, testCase)
	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Checkout Parent Trace", 1)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "checkout",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}
			var childOutput map[string]string
			return ctx.ChildWorkflow("charge-card", "billing", "v1", input, &childOutput)
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

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 3 {
		t.Fatalf("expected started + child scheduled + child started history, got %+v", historyRows)
	}
	if historyRows[1].EventType != enginehistory.EventChildWorkflowScheduled {
		t.Fatalf("expected child_workflow.scheduled event, got %+v", historyRows[1])
	}
	if historyRows[2].EventType != enginehistory.EventChildWorkflowStarted {
		t.Fatalf("expected child_workflow.started event, got %+v", historyRows[2])
	}

	childWorkflow, err := store.GetChildWorkflowByParentRunAndKey(ctx, enginedb.GetChildWorkflowByParentRunAndKeyParams{
		ProjectID:   testCase.projectID,
		ParentRunID: run.ID,
		ChildKey:    "charge-card",
	})
	if err != nil {
		t.Fatalf("GetChildWorkflowByParentRunAndKey() error = %v", err)
	}
	if childWorkflow.Status != enginedb.EngineChildWorkflowStatusActive {
		t.Fatalf("expected active child workflow, got %+v", childWorkflow)
	}
	if childWorkflow.RootRunID != run.ID {
		t.Fatalf("expected child workflow root run %s, got %+v", run.ID, childWorkflow)
	}
	if childWorkflow.ChildDepth != 1 {
		t.Fatalf("expected child workflow depth 1, got %+v", childWorkflow)
	}

	childRun, err := store.GetRun(ctx, childWorkflow.CurrentChildRunID)
	if err != nil {
		t.Fatalf("GetRun(child) error = %v", err)
	}
	if !childRun.ParentRunID.Valid || childRun.ParentRunID.Bytes != run.ID {
		t.Fatalf("expected child run parent %s, got %+v", run.ID, childRun)
	}
	if childRun.RootRunID != run.ID {
		t.Fatalf("expected child run root %s, got %+v", run.ID, childRun)
	}
	if childRun.ChildKey == nil || *childRun.ChildKey != "charge-card" {
		t.Fatalf("expected child run key charge-card, got %+v", childRun)
	}
	if childRun.ChildDepth != 1 {
		t.Fatalf("expected child run depth 1, got %+v", childRun)
	}

	var (
		traceID     uuid.UUID
		parentRunID pgtype.UUID
		rootRunID   pgtype.UUID
		childKey    *string
		childDepth  *int32
	)
	if err := db.Pool.QueryRow(ctx, `
		SELECT id, engine_parent_run_id, engine_root_run_id, engine_child_key, engine_child_depth
		FROM public.traces
		WHERE project_id = $1
		  AND engine_run_id = $2
	`, testCase.projectID, childRun.ID).Scan(&traceID, &parentRunID, &rootRunID, &childKey, &childDepth); err != nil {
		t.Fatalf("query child trace shell: %v", err)
	}
	if traceID == uuid.Nil {
		t.Fatalf("expected projected child trace shell id, got %s", traceID)
	}
	if !parentRunID.Valid || parentRunID.Bytes != run.ID {
		t.Fatalf("expected projected child trace parent %s, got %+v", run.ID, parentRunID)
	}
	if !rootRunID.Valid || rootRunID.Bytes != run.ID {
		t.Fatalf("expected projected child trace root %s, got %+v", run.ID, rootRunID)
	}
	if childKey == nil || *childKey != "charge-card" {
		t.Fatalf("expected projected child trace key charge-card, got %+v", childKey)
	}
	if childDepth == nil || *childDepth != 1 {
		t.Fatalf("expected projected child trace depth 1, got %+v", childDepth)
	}
}

func TestActivatorChildWorkflowBindingIdempotencyAndConflicts(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()
	activator := NewActivator(store, mustRegistry(t))

	parentInstance, parentRun := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-binding-parent",
		definitionName:    "checkout",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-binding"}),
	})
	otherParentInstance, otherParentRun := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-binding-other-parent",
		definitionName:    "checkout",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-binding-other"}),
	})

	child := &newChildWorkflow{Scheduled: enginehistory.ChildWorkflowScheduledPayload{
		ChildKey:          "charge-card",
		DefinitionName:    "billing",
		DefinitionVersion: "v1",
		Input:             mustJSON(t, map[string]string{"order_id": "ord-binding"}),
		ChildInstanceKey:  "custom-child-binding",
	}}

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	childInstance, childRun, childStartedEvent, createdChild, err := activator.createOrAttachChildExecution(ctx, tx, &parentInstance, &parentRun, child)
	if err != nil {
		t.Fatalf("createOrAttachChildExecution() first error = %v", err)
	}
	if !createdChild || childStartedEvent == nil {
		t.Fatalf("expected first child binding to create execution, created=%v event=%+v", createdChild, childStartedEvent)
	}

	attachedInstance, attachedRun, attachedStartedEvent, attachedCreated, err := activator.createOrAttachChildExecution(ctx, tx, &parentInstance, &parentRun, child)
	if err != nil {
		t.Fatalf("createOrAttachChildExecution() idempotent error = %v", err)
	}
	if attachedCreated || attachedStartedEvent != nil {
		t.Fatalf("expected idempotent attach without new child history, created=%v event=%+v", attachedCreated, attachedStartedEvent)
	}
	if attachedInstance.ID != childInstance.ID || attachedRun.ID != childRun.ID {
		t.Fatalf("expected idempotent attach to return existing child, instance=%s/%s run=%s/%s", attachedInstance.ID, childInstance.ID, attachedRun.ID, childRun.ID)
	}

	versionMismatch := *child
	versionMismatch.Scheduled.DefinitionVersion = "v2"
	_, _, _, _, err = activator.createOrAttachChildExecution(ctx, tx, &parentInstance, &parentRun, &versionMismatch)
	var coded codedWorkflowError
	if !errors.As(err, &coded) || coded.code != "instance_conflict" {
		t.Fatalf("expected instance_conflict for reused child_key with different definition, got %v", err)
	}

	_, _, _, _, err = activator.createOrAttachChildExecution(ctx, tx, &otherParentInstance, &otherParentRun, child)
	if !errors.As(err, &coded) || coded.code != "instance_conflict" {
		t.Fatalf("expected instance_conflict for child instance attached to another parent, got %v", err)
	}
}

func TestActivatorChildWorkflowConflictFailsDeterministically(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	seedActivator := NewActivator(store, mustRegistry(t))
	existingParentInstance, existingParentRun := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-binding-existing-parent",
		definitionName:    "checkout",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-existing"}),
	})

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() seed error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, _, _, _, err := seedActivator.createOrAttachChildExecution(ctx, tx, &existingParentInstance, &existingParentRun, &newChildWorkflow{
		Scheduled: enginehistory.ChildWorkflowScheduledPayload{
			ChildKey:          "charge-card",
			DefinitionName:    "billing",
			DefinitionVersion: "v1",
			Input:             mustJSON(t, map[string]string{"order_id": "ord-existing"}),
			ChildInstanceKey:  "custom-child-binding-conflict",
		},
	}); err != nil {
		t.Fatalf("createOrAttachChildExecution() seed error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() seed error = %v", err)
	}

	instance, run := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-binding-conflict-parent",
		definitionName:    "checkout-binding-conflict",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-conflict"}),
	})
	insertProjectedTraceShell(t, ctx, db.Pool, instance, run, "Binding Conflict Parent", 1)

	registry, err := NewRegistry(publicworkflow.Definition{
		Name:    "checkout-binding-conflict",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}
			var childOutput map[string]string
			return ctx.ChildWorkflowWithOptions(
				"charge-card",
				"billing",
				"v2",
				input,
				&childOutput,
				publicworkflow.ChildWorkflowOptions{InstanceKey: "custom-child-binding-conflict"},
			)
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	activator := NewActivator(store, registry)
	claimed := forceClaimRun(t, ctx, db.Pool, store, run.ID, "worker-binding-conflict")
	if err := activator.Activate(ctx, &claimed); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	failedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if failedRun.Status != enginedb.EngineRunLifecycleStatusFailed {
		t.Fatalf("expected failed run status, got %+v", failedRun)
	}
	if failedRun.LastErrorCode == nil || *failedRun.LastErrorCode != "instance_conflict" {
		t.Fatalf("expected instance_conflict last error code, got %+v", failedRun)
	}

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	assertHistoryEndsWith(t, historyRows, enginehistory.EventWorkflowStarted, enginehistory.EventWorkflowFailed)
	var failure enginehistory.WorkflowFailedPayload
	if err := enginehistory.UnmarshalPayload(historyRows[len(historyRows)-1].Payload, &failure); err != nil {
		t.Fatalf("UnmarshalPayload(workflow.failed) error = %v", err)
	}
	if failure.ErrorCode != "instance_conflict" {
		t.Fatalf("expected workflow.failed instance_conflict, got %+v", failure)
	}

	_, err = store.GetChildWorkflowByParentRunAndKey(ctx, enginedb.GetChildWorkflowByParentRunAndKeyParams{
		ProjectID:   run.ProjectID,
		ParentRunID: run.ID,
		ChildKey:    "charge-card",
	})
	if !errors.Is(err, enginestore.ErrNotFound) {
		t.Fatalf("expected no child relationship row for failed parent, got err=%v", err)
	}
}

func TestActivatorChildContinueAsNewBlocksParentUntilTerminalRun(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-child-continue-parent",
		definitionName:    "checkout-child-continue",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-child-continue"}),
	}
	instance, parentRun := createStartedRun(t, store, testCase)
	insertProjectedTraceShell(t, ctx, db.Pool, instance, parentRun, "Child Continue Parent", 1)

	continueChild := false
	registry, err := NewRegistry(
		childWaitingParentDefinition("checkout-child-continue", func(err error) error { return err }),
		publicworkflow.Definition{
			Name:    "billing",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				if continueChild {
					return publicworkflow.ContinueAsNew(map[string]any{"continued": true})
				}
				return ctx.SetResult(map[string]string{"status": "authorized"})
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	activator := NewActivator(store, registry)

	activateNextRun(t, ctx, store, activator, "worker-parent")
	parentAfterSchedule, childWorkflow := assertParentWaitingOnChild(t, ctx, store, parentRun.ID)
	childRunID := childWorkflow.CurrentChildRunID

	continueChild = true
	activateRunByID(t, ctx, db.Pool, store, activator, childRunID, "worker-child-continue")
	childWorkflow = getChildWorkflow(t, ctx, store, parentAfterSchedule.ProjectID, parentAfterSchedule.ID)
	if childWorkflow.ContinuationCount != 1 {
		t.Fatalf("expected one child continuation, got %+v", childWorkflow)
	}
	parentStillWaiting, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent) error = %v", err)
	}
	if parentStillWaiting.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected parent to remain waiting after child ContinueAsNew, got %+v", parentStillWaiting)
	}
	if childWorkflow.CurrentChildRunID == childRunID {
		t.Fatalf("expected child current run to advance after ContinueAsNew, got %+v", childWorkflow)
	}

	continueChild = false
	terminalChildRunID := childWorkflow.CurrentChildRunID
	activateRunByID(t, ctx, db.Pool, store, activator, terminalChildRunID, "worker-child-terminal")
	childWorkflow = getChildWorkflow(t, ctx, store, parentRun.ProjectID, parentRun.ID)
	if childWorkflow.Status != enginedb.EngineChildWorkflowStatusCompleted {
		t.Fatalf("expected completed child workflow, got %+v", childWorkflow)
	}
	if !childWorkflow.TerminalChildRunID.Valid || childWorkflow.TerminalChildRunID.Bytes != terminalChildRunID {
		t.Fatalf("expected terminal child run %s, got %+v", terminalChildRunID, childWorkflow)
	}
	parentAfterChildTerminal, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent terminal wake) error = %v", err)
	}
	if parentAfterChildTerminal.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected parent to wake only after terminal child run, got %+v", parentAfterChildTerminal)
	}

	activateRunByID(t, ctx, db.Pool, store, activator, parentRun.ID, "worker-parent-complete")
	completedParent, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent completed) error = %v", err)
	}
	if completedParent.Status != enginedb.EngineRunLifecycleStatusCompleted {
		t.Fatalf("expected completed parent, got %+v", completedParent)
	}
	assertRawJSONEqual(t, `{"status":"authorized"}`, completedParent.Result)
	parentHistory, err := store.GetHistoryByRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun(parent) error = %v", err)
	}
	assertHistoryEndsWith(t, parentHistory, enginehistory.EventChildWorkflowCompleted, enginehistory.EventWorkflowCompleted)
}

func TestActivatorChildTerminalOutcomesRecordMatchingParentAndChildHistory(t *testing.T) {
	testCases := []struct {
		name                    string
		childRun                func(publicworkflow.Context) error
		activateChild           bool
		terminateChild          bool
		wantChildRunStatus      enginedb.EngineRunLifecycleStatus
		wantChildWorkflowStatus enginedb.EngineChildWorkflowStatus
		wantChildTerminalEvent  string
		wantParentTerminalEvent string
		wantChildErrorCode      string
		wantChildErrorMessage   string
		wantChildTerminalState  string
		wantParentResultJSON    string
	}{
		{
			name: "failed",
			childRun: func(ctx publicworkflow.Context) error {
				return codedWorkflowError{
					code:    "card_declined",
					message: "card declined",
				}
			},
			activateChild:           true,
			wantChildRunStatus:      enginedb.EngineRunLifecycleStatusFailed,
			wantChildWorkflowStatus: enginedb.EngineChildWorkflowStatusFailed,
			wantChildTerminalEvent:  enginehistory.EventWorkflowFailed,
			wantParentTerminalEvent: enginehistory.EventChildWorkflowFailed,
			wantChildErrorCode:      "card_declined",
			wantChildErrorMessage:   "card declined",
			wantChildTerminalState:  "failed",
			wantParentResultJSON:    `{"handled":"failed"}`,
		},
		{
			name: "cancelled",
			childRun: func(ctx publicworkflow.Context) error {
				return publicworkflow.ErrCancelled
			},
			activateChild:           true,
			wantChildRunStatus:      enginedb.EngineRunLifecycleStatusCancelled,
			wantChildWorkflowStatus: enginedb.EngineChildWorkflowStatusCancelled,
			wantChildTerminalEvent:  enginehistory.EventWorkflowCancelled,
			wantParentTerminalEvent: enginehistory.EventChildWorkflowCancelled,
			wantChildErrorCode:      "cancelled",
			wantChildErrorMessage:   "workflow cancelled",
			wantChildTerminalState:  "cancelled",
			wantParentResultJSON:    `{"handled":"cancelled"}`,
		},
		{
			name: "terminated",
			childRun: func(ctx publicworkflow.Context) error {
				var signal map[string]string
				return ctx.ReceiveSignal("hold", &signal)
			},
			terminateChild:          true,
			wantChildRunStatus:      enginedb.EngineRunLifecycleStatusTerminated,
			wantChildWorkflowStatus: enginedb.EngineChildWorkflowStatusTerminated,
			wantChildTerminalEvent:  enginehistory.EventWorkflowTerminated,
			wantParentTerminalEvent: enginehistory.EventChildWorkflowTerminated,
			wantChildErrorCode:      "terminated",
			wantChildErrorMessage:   "run terminated by operator",
			wantChildTerminalState:  "terminated",
			wantParentResultJSON:    `{"handled":"terminated"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := enginetest.NewTestDatabase(t)
			store := enginestore.New(db.Pool)
			ctx := context.Background()

			parentDefinitionName := "checkout-child-terminal-" + testCase.name
			instance, parentRun := createStartedRun(t, store, workflowTestCase{
				projectID:         enginetest.DefaultPlatformProjectID,
				instanceKey:       "instance-" + testCase.name + "-parent",
				definitionName:    parentDefinitionName,
				definitionVersion: "v1",
				input:             mustJSON(t, map[string]string{"order_id": "ord-" + testCase.name}),
			})
			insertProjectedTraceShell(t, ctx, db.Pool, instance, parentRun, "Child Terminal Parent "+testCase.name, 1)

			registry := mustRegistry(
				t,
				publicworkflow.Definition{
					Name:    parentDefinitionName,
					Version: "v1",
					Run: func(ctx publicworkflow.Context) error {
						var input map[string]string
						if err := ctx.Input(&input); err != nil {
							return err
						}

						var childOutput map[string]string
						err := ctx.ChildWorkflow("charge-card", "billing", "v1", input, &childOutput)
						if err == nil {
							return errors.New("expected child terminal error")
						}

						var childErr *publicworkflow.ChildWorkflowError
						if !errors.As(err, &childErr) {
							return err
						}
						if childErr.Code() != testCase.wantChildErrorCode ||
							childErr.Message() != testCase.wantChildErrorMessage ||
							childErr.TerminalState() != testCase.wantChildTerminalState {
							return fmt.Errorf(
								"unexpected child terminal outcome code=%q message=%q terminal_state=%q",
								childErr.Code(),
								childErr.Message(),
								childErr.TerminalState(),
							)
						}

						return ctx.SetResult(map[string]string{"handled": childErr.TerminalState()})
					},
				},
				publicworkflow.Definition{
					Name:    "billing",
					Version: "v1",
					Run:     testCase.childRun,
				},
			)
			activator := NewActivator(store, registry)

			activateNextRun(t, ctx, store, activator, "worker-parent-"+testCase.name)
			_, childWorkflow := assertParentWaitingOnChild(t, ctx, store, parentRun.ID)
			childRunID := childWorkflow.CurrentChildRunID

			switch {
			case testCase.activateChild:
				activateRunByID(t, ctx, db.Pool, store, activator, childRunID, "worker-child-"+testCase.name)
			case testCase.terminateChild:
				forceTerminateChildRunForTest(t, ctx, store, childRunID)
			default:
				t.Fatalf("invalid child terminal test case configuration: %+v", testCase)
			}

			childRun, err := store.GetRun(ctx, childRunID)
			if err != nil {
				t.Fatalf("GetRun(child) error = %v", err)
			}
			if childRun.Status != testCase.wantChildRunStatus {
				t.Fatalf("expected child run status %s, got %+v", testCase.wantChildRunStatus, childRun)
			}

			childHistory, err := store.GetHistoryByRun(ctx, childRunID)
			if err != nil {
				t.Fatalf("GetHistoryByRun(child) error = %v", err)
			}
			assertHistoryEndsWith(t, childHistory, testCase.wantChildTerminalEvent)
			assertChildTerminalHistoryPayload(t, childHistory[len(childHistory)-1], childRunID, testCase.wantChildTerminalEvent, testCase.wantChildErrorCode, testCase.wantChildErrorMessage)

			relationship := getChildWorkflow(t, ctx, store, parentRun.ProjectID, parentRun.ID)
			if relationship.Status != testCase.wantChildWorkflowStatus {
				t.Fatalf("expected child workflow status %s, got %+v", testCase.wantChildWorkflowStatus, relationship)
			}
			if !relationship.TerminalChildRunID.Valid || relationship.TerminalChildRunID.Bytes != childRunID {
				t.Fatalf("expected terminal child run %s, got %+v", childRunID, relationship)
			}

			parentQueued, err := store.GetRun(ctx, parentRun.ID)
			if err != nil {
				t.Fatalf("GetRun(parent queued) error = %v", err)
			}
			if parentQueued.Status != enginedb.EngineRunLifecycleStatusQueued {
				t.Fatalf("expected parent to queue after child terminal outcome, got %+v", parentQueued)
			}

			activateRunByID(t, ctx, db.Pool, store, activator, parentRun.ID, "worker-parent-complete-"+testCase.name)
			completedParent, err := store.GetRun(ctx, parentRun.ID)
			if err != nil {
				t.Fatalf("GetRun(parent completed) error = %v", err)
			}
			if completedParent.Status != enginedb.EngineRunLifecycleStatusCompleted {
				t.Fatalf("expected completed parent run, got %+v", completedParent)
			}
			assertRawJSONEqual(t, testCase.wantParentResultJSON, completedParent.Result)

			parentHistory, err := store.GetHistoryByRun(ctx, parentRun.ID)
			if err != nil {
				t.Fatalf("GetHistoryByRun(parent) error = %v", err)
			}
			assertHistoryEndsWith(t, parentHistory, testCase.wantParentTerminalEvent, enginehistory.EventWorkflowCompleted)
			assertParentChildTerminalEventPayload(
				t,
				parentHistory[len(parentHistory)-2],
				childRunID,
				testCase.wantParentTerminalEvent,
				testCase.wantChildErrorCode,
				testCase.wantChildErrorMessage,
			)
		})
	}
}

func TestActivatorChildContinuationDepthWaitFailureAndLateTerminalGuard(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-child-follow-depth",
		definitionName:    "checkout-child-follow-depth",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-follow-depth"}),
	}
	instance, parentRun := createStartedRun(t, store, testCase)
	insertProjectedTraceShell(t, ctx, db.Pool, instance, parentRun, "Child Follow Depth Parent", 1)

	continueChild := true
	registry, err := NewRegistry(
		childWaitingParentDefinition("checkout-child-follow-depth", func(err error) error {
			var childErr *publicworkflow.ChildWorkflowError
			if !errors.As(err, &childErr) || childErr.TerminalState() != "wait_failed" {
				return err
			}
			return nil
		}),
		publicworkflow.Definition{
			Name:    "billing",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				if continueChild {
					return publicworkflow.ContinueAsNew(map[string]any{"continued": true})
				}
				return ctx.SetResult(map[string]string{"status": "too-late"})
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	activator := NewActivator(store, registry)

	activateNextRun(t, ctx, store, activator, "worker-parent")
	parentAfterSchedule, childWorkflow := assertParentWaitingOnChild(t, ctx, store, parentRun.ID)
	if _, err := db.Pool.Exec(ctx, `
		UPDATE engine.child_workflows
		SET continuation_count = 30
		WHERE id = $1
	`, childWorkflow.ID); err != nil {
		t.Fatalf("seed child continuation count: %v", err)
	}

	activateRunByID(t, ctx, db.Pool, store, activator, childWorkflow.CurrentChildRunID, "worker-child-31")
	childWorkflow = getChildWorkflow(t, ctx, store, parentRun.ProjectID, parentRun.ID)
	if childWorkflow.ContinuationCount != maxContinuationFollowDepth-1 {
		t.Fatalf("expected below-limit continuation count, got %+v", childWorkflow)
	}
	parentStillWaiting, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent below limit) error = %v", err)
	}
	if parentStillWaiting.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected below-limit child continuation not to wake parent, got %+v", parentStillWaiting)
	}

	activateRunByID(t, ctx, db.Pool, store, activator, childWorkflow.CurrentChildRunID, "worker-child-32")
	childWorkflow = getChildWorkflow(t, ctx, store, parentRun.ProjectID, parentRun.ID)
	if childWorkflow.ContinuationCount != maxContinuationFollowDepth {
		t.Fatalf("expected max continuation count, got %+v", childWorkflow)
	}
	if childWorkflow.Status != enginedb.EngineChildWorkflowStatusActive || childWorkflow.TerminalChildRunID.Valid {
		t.Fatalf("expected child to remain active at follow-depth guard, got %+v", childWorkflow)
	}
	parentWoken, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent woken) error = %v", err)
	}
	if parentWoken.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected 32nd continuation to wake parent, got %+v", parentWoken)
	}

	activateRunByID(t, ctx, db.Pool, store, activator, parentRun.ID, "worker-parent-wait-failed")
	childWorkflow = getChildWorkflow(t, ctx, store, parentRun.ProjectID, parentRun.ID)
	if !childWorkflow.ParentWaitFailedAt.Valid ||
		childWorkflow.ParentWaitErrorCode == nil ||
		*childWorkflow.ParentWaitErrorCode != childWaitFailedContinuationCode {
		t.Fatalf("expected durable parent wait failed marker, got %+v", childWorkflow)
	}
	parentAfterWaitFailed, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent after wait_failed) error = %v", err)
	}
	if parentAfterWaitFailed.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected parent to wait on unrelated signal after handled wait_failed, got %+v", parentAfterWaitFailed)
	}
	assertWaitingForSignal(t, parentAfterWaitFailed, "after-child-wait-failed")
	parentHistory, err := store.GetHistoryByRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun(parent wait_failed) error = %v", err)
	}
	if !historyContainsEvent(parentHistory, enginehistory.EventChildWorkflowWaitFailed) {
		t.Fatalf("expected child_workflow.wait_failed history event, got %+v", parentHistory)
	}

	continueChild = false
	activateRunByID(t, ctx, db.Pool, store, activator, childWorkflow.CurrentChildRunID, "worker-child-late-terminal")
	parentAfterLateTerminal, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent after late terminal) error = %v", err)
	}
	if parentAfterLateTerminal.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected late terminal child not to wake unrelated parent wait, got %+v", parentAfterLateTerminal)
	}
	assertWaitingForSignal(t, parentAfterLateTerminal, "after-child-wait-failed")
	lateTerminalRelationship := getChildWorkflow(t, ctx, store, parentRun.ProjectID, parentAfterSchedule.ID)
	if lateTerminalRelationship.Status != enginedb.EngineChildWorkflowStatusCompleted {
		t.Fatalf("expected late terminal update to complete child relationship, got %+v", lateTerminalRelationship)
	}
}

func TestActivatorChildTerminalWakeGuardRequiresMatchingWaitIdentity(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-child-wake-guard",
		definitionName:    "checkout-child-wake-guard",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-wake-guard"}),
	}
	instance, parentRun := createStartedRun(t, store, testCase)
	insertProjectedTraceShell(t, ctx, db.Pool, instance, parentRun, "Child Wake Guard Parent", 1)

	registry, err := NewRegistry(
		childWaitingParentDefinition("checkout-child-wake-guard", func(err error) error { return err }),
		publicworkflow.Definition{
			Name:    "billing",
			Version: "v1",
			Run: func(ctx publicworkflow.Context) error {
				return ctx.SetResult(map[string]string{"status": "authorized"})
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	activator := NewActivator(store, registry)

	activateNextRun(t, ctx, store, activator, "worker-parent")
	_, childWorkflow := assertParentWaitingOnChild(t, ctx, store, parentRun.ID)
	wrongWait, err := enginehistory.MarshalPayload(enginehistory.ChildWorkflowWait{
		Kind:     enginehistory.WaitKindChildWorkflow,
		ChildKey: "other-child",
	})
	if err != nil {
		t.Fatalf("MarshalPayload(wrong wait) error = %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE engine.runs
		SET waiting_for = $2
		WHERE id = $1
	`, parentRun.ID, wrongWait); err != nil {
		t.Fatalf("set wrong parent wait identity: %v", err)
	}

	activateRunByID(t, ctx, db.Pool, store, activator, childWorkflow.CurrentChildRunID, "worker-child-terminal")
	parentAfterTerminal, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent after terminal) error = %v", err)
	}
	if parentAfterTerminal.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected terminal child not to wake mismatched parent wait, got %+v", parentAfterTerminal)
	}
	var wait enginehistory.ChildWorkflowWait
	if err := json.Unmarshal(parentAfterTerminal.WaitingFor, &wait); err != nil {
		t.Fatalf("decode parent wait: %v", err)
	}
	if wait.ChildKey != "other-child" {
		t.Fatalf("expected mismatched child wait to remain unchanged, got %+v", wait)
	}
}

func TestActivatorCancelledDecisionEnqueuesActiveChildCancelIdempotently(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	testCase := workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-child-cancel-parent",
		definitionName:    "checkout-child-cancel",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-cancel-child"}),
	}
	instance, parentRun := createStartedRun(t, store, testCase)
	insertProjectedTraceShell(t, ctx, db.Pool, instance, parentRun, "Child Cancel Parent", 1)

	registry, err := NewRegistry(childWaitingParentDefinition("checkout-child-cancel", func(err error) error { return err }))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	activator := NewActivator(store, registry)

	activateNextRun(t, ctx, store, activator, "worker-parent")
	_, childWorkflow := assertParentWaitingOnChild(t, ctx, store, parentRun.ID)
	claimedParent := forceClaimRun(t, ctx, db.Pool, store, parentRun.ID, "worker-parent-cancel")

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	parentInstance, err := tx.GetInstance(ctx, claimedParent.InstanceID)
	if err != nil {
		t.Fatalf("GetInstance(parent) error = %v", err)
	}
	parentRunForDecision, err := tx.GetRun(ctx, claimedParent.ID)
	if err != nil {
		t.Fatalf("GetRun(parent) error = %v", err)
	}
	decision := activationDecision{
		Kind:         decisionCancelled,
		NextSequence: 4,
		Events: []queuedHistoryEvent{{
			EventType: enginehistory.EventWorkflowCancelled,
			Payload:   mustMarshalPayload(enginehistory.WorkflowCancelledPayload{}),
		}},
	}
	if err := activator.commitDecision(ctx, tx, &parentInstance, &parentRunForDecision, parentRunForDecision.ClaimedBy, &decision); err != nil {
		t.Fatalf("commitDecision(cancelled) error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit(cancelled) error = %v", err)
	}

	tx, err = store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx(second cancel cascade) error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	parentAfterCancel, err := tx.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(parent cancelled) error = %v", err)
	}
	if err := activator.enqueueChildCancelCascade(ctx, tx, &parentAfterCancel); err != nil {
		t.Fatalf("enqueueChildCancelCascade() second error = %v", err)
	}
	if err := activator.enqueueChildCancelCascade(ctx, tx, &parentAfterCancel); err != nil {
		t.Fatalf("enqueueChildCancelCascade() third error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit(second cancel cascade) error = %v", err)
	}

	var cancelInboxCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM engine.inbox
		WHERE run_id = $1
		  AND kind = 'cancel'
		  AND dedupe_key = $2
	`, childWorkflow.CurrentChildRunID, "cancel:"+childWorkflow.CurrentChildRunID.String()).Scan(&cancelInboxCount); err != nil {
		t.Fatalf("count child cancel inbox: %v", err)
	}
	if cancelInboxCount != 1 {
		t.Fatalf("expected exactly one child cancel inbox row, got %d", cancelInboxCount)
	}
	cancelledParent, err := store.GetRun(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetRun(cancelled parent) error = %v", err)
	}
	if cancelledParent.Status != enginedb.EngineRunLifecycleStatusCancelled {
		t.Fatalf("expected cancelled parent, got %+v", cancelledParent)
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

func mustRegistry(t *testing.T, defs ...publicworkflow.Definition) *Registry {
	t.Helper()

	registry, err := NewRegistry(defs...)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func childWaitingParentDefinition(
	definitionName string,
	handleErr func(error) error,
) publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    definitionName,
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			var input map[string]string
			if err := ctx.Input(&input); err != nil {
				return err
			}

			var childOutput map[string]string
			err := ctx.ChildWorkflow("charge-card", "billing", "v1", input, &childOutput)
			if err != nil {
				if handleErr != nil {
					if handledErr := handleErr(err); handledErr != nil {
						return handledErr
					}
				}

				var signal map[string]string
				if err := ctx.ReceiveSignal("after-child-wait-failed", &signal); err != nil {
					return err
				}
				return ctx.SetResult(map[string]string{"signal": signal["status"]})
			}

			return ctx.SetResult(map[string]string{"status": childOutput["status"]})
		},
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func activateNextRun(
	t *testing.T,
	ctx context.Context,
	store *enginestore.Store,
	activator *Activator,
	workerID string,
) {
	t.Helper()

	claimed, err := store.ClaimNextRun(ctx, workerID, time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if err := activator.Activate(ctx, &claimed); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func forceClaimRun(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
	store *enginestore.Store,
	runID uuid.UUID,
	workerID string,
) enginedb.EngineRun {
	t.Helper()

	if _, err := pool.Exec(ctx, `
		UPDATE engine.runs
		SET status = 'running',
		    claimed_by = $2,
		    claimed_at = NOW(),
		    lease_expires_at = NOW() + INTERVAL '1 minute',
		    attempt_count = attempt_count + 1,
		    updated_at = NOW()
		WHERE id = $1
	`, runID, workerID); err != nil {
		t.Fatalf("force claim run %s: %v", runID, err)
	}

	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", runID, err)
	}
	if run.Status != enginedb.EngineRunLifecycleStatusRunning {
		t.Fatalf("expected running run after force claim, got %+v", run)
	}
	if run.ClaimedBy == nil || *run.ClaimedBy != workerID {
		t.Fatalf("expected claimed_by=%s after force claim, got %+v", workerID, run)
	}
	return run
}

//nolint:revive // Keep testing.T first in test helper signatures.
func activateRunByID(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
	store *enginestore.Store,
	activator *Activator,
	runID uuid.UUID,
	workerID string,
) {
	t.Helper()

	claimed := forceClaimRun(t, ctx, pool, store, runID, workerID)
	if err := activator.Activate(ctx, &claimed); err != nil {
		t.Fatalf("Activate(%s) error = %v", runID, err)
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func forceTerminateChildRunForTest(
	t *testing.T,
	ctx context.Context,
	store *enginestore.Store,
	childRunID uuid.UUID,
) {
	t.Helper()

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	run, err := tx.GetRunForUpdate(ctx, childRunID)
	if err != nil {
		t.Fatalf("GetRunForUpdate(%s) error = %v", childRunID, err)
	}
	historyRows, err := tx.GetHistoryByRun(ctx, childRunID)
	if err != nil {
		t.Fatalf("GetHistoryByRun(%s) error = %v", childRunID, err)
	}
	requireSequence := int32(1)
	if len(historyRows) > 0 {
		requireSequence = historyRows[len(historyRows)-1].SequenceNo + 1
	}
	payload, err := enginehistory.MarshalPayload(enginehistory.WorkflowTerminatedPayload{
		ErrorCode:    "terminated",
		ErrorMessage: "run terminated by operator",
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow.terminated) error = %v", err)
	}
	if _, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  run.ProjectID,
		InstanceID: run.InstanceID,
		RunID:      run.ID,
		SequenceNo: requireSequence,
		EventType:  enginehistory.EventWorkflowTerminated,
		Payload:    payload,
	}); err != nil {
		t.Fatalf("AppendHistory(workflow.terminated) error = %v", err)
	}
	terminatedRun, err := tx.TransitionRunToTerminated(ctx, run.ID)
	if err != nil {
		t.Fatalf("TransitionRunToTerminated(%s) error = %v", run.ID, err)
	}
	if _, err := tx.UpdateInstanceStatus(ctx, enginedb.UpdateInstanceStatusParams{
		ID:     run.InstanceID,
		Status: enginedb.EngineInstanceLifecycleStatusTerminated,
	}); err != nil {
		t.Fatalf("UpdateInstanceStatus(terminated) error = %v", err)
	}
	childWorkflow, err := tx.UpdateChildWorkflowTerminal(ctx, enginedb.UpdateChildWorkflowTerminalParams{
		ProjectID:          terminatedRun.ProjectID,
		CurrentChildRunID:  terminatedRun.ID,
		TerminalChildRunID: pgtype.UUID{Bytes: terminatedRun.ID, Valid: true},
		Status:             enginedb.EngineChildWorkflowStatusTerminated,
	})
	if err != nil {
		t.Fatalf("UpdateChildWorkflowTerminal(%s) error = %v", terminatedRun.ID, err)
	}
	wokenParent, err := tx.WakeWaitingChildWorkflowRun(ctx, enginedb.WakeWaitingChildWorkflowRunParams{
		ID:       childWorkflow.ParentRunID,
		ChildKey: childWorkflow.ChildKey,
	})
	if err != nil {
		t.Fatalf("WakeWaitingChildWorkflowRun(%s,%s) error = %v", childWorkflow.ParentRunID, childWorkflow.ChildKey, err)
	}
	if !wokenParent.Applied {
		t.Fatalf("expected force-terminated child to wake parent, got %+v", wokenParent)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit(force terminate child) error = %v", err)
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func assertParentWaitingOnChild(
	t *testing.T,
	ctx context.Context,
	store *enginestore.Store,
	parentRunID uuid.UUID,
) (enginedb.EngineRun, enginedb.EngineChildWorkflow) {
	t.Helper()

	run, err := store.GetRun(ctx, parentRunID)
	if err != nil {
		t.Fatalf("GetRun(parent) error = %v", err)
	}
	if run.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected waiting parent run, got %+v", run)
	}

	var wait enginehistory.ChildWorkflowWait
	if err := json.Unmarshal(run.WaitingFor, &wait); err != nil {
		t.Fatalf("decode parent wait: %v", err)
	}
	if wait.Kind != enginehistory.WaitKindChildWorkflow || wait.ChildKey != "charge-card" {
		t.Fatalf("expected child workflow wait %q, got %+v", "charge-card", wait)
	}

	childWorkflow := getChildWorkflow(t, ctx, store, run.ProjectID, parentRunID)
	if childWorkflow.Status != enginedb.EngineChildWorkflowStatusActive {
		t.Fatalf("expected active child relationship, got %+v", childWorkflow)
	}
	return run, childWorkflow
}

//nolint:revive // Keep testing.T first in test helper signatures.
func getChildWorkflow(
	t *testing.T,
	ctx context.Context,
	store *enginestore.Store,
	projectID uuid.UUID,
	parentRunID uuid.UUID,
) enginedb.EngineChildWorkflow {
	t.Helper()

	childWorkflow, err := store.GetChildWorkflowByParentRunAndKey(ctx, enginedb.GetChildWorkflowByParentRunAndKeyParams{
		ProjectID:   projectID,
		ParentRunID: parentRunID,
		ChildKey:    "charge-card",
	})
	if err != nil {
		t.Fatalf("GetChildWorkflowByParentRunAndKey(%s) error = %v", "charge-card", err)
	}
	return childWorkflow
}

func assertWaitingForSignal(t *testing.T, run enginedb.EngineRun, signalName string) {
	t.Helper()

	if run.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected waiting run, got %+v", run)
	}

	var wait enginehistory.SignalWait
	if err := json.Unmarshal(run.WaitingFor, &wait); err != nil {
		t.Fatalf("decode signal wait: %v", err)
	}
	if wait.Kind != enginehistory.WaitKindSignal || wait.SignalName != signalName {
		t.Fatalf("expected waiting on signal %q, got %+v", signalName, wait)
	}
}

func historyContainsEvent(rows []enginedb.EngineHistory, eventType string) bool {
	for i := range rows {
		if rows[i].EventType == eventType {
			return true
		}
	}
	return false
}

func assertHistoryEndsWith(t *testing.T, rows []enginedb.EngineHistory, want ...string) {
	t.Helper()

	if len(rows) < len(want) {
		t.Fatalf("history too short: got %d rows, want suffix %v", len(rows), want)
	}
	offset := len(rows) - len(want)
	for i := range want {
		if rows[offset+i].EventType != want[i] {
			t.Fatalf("unexpected history suffix: got %v want %v", historyEventTypes(rows[offset:]), want)
		}
	}
}

func historyEventTypes(rows []enginedb.EngineHistory) []string {
	eventTypes := make([]string, 0, len(rows))
	for i := range rows {
		eventTypes = append(eventTypes, rows[i].EventType)
	}
	return eventTypes
}

func assertChildTerminalHistoryPayload(
	t *testing.T,
	row enginedb.EngineHistory,
	childRunID uuid.UUID,
	eventType string,
	errorCode string,
	errorMessage string,
) {
	t.Helper()

	switch eventType {
	case enginehistory.EventWorkflowFailed:
		payload := mustHistoryPayload[enginehistory.WorkflowFailedPayload](t, row.Payload)
		if payload.ErrorCode != errorCode || payload.ErrorMessage != errorMessage {
			t.Fatalf("unexpected child workflow.failed payload: %+v", payload)
		}
	case enginehistory.EventWorkflowCancelled:
		_ = mustHistoryPayload[enginehistory.WorkflowCancelledPayload](t, row.Payload)
	case enginehistory.EventWorkflowTerminated:
		payload := mustHistoryPayload[enginehistory.WorkflowTerminatedPayload](t, row.Payload)
		if payload.ErrorCode != errorCode || payload.ErrorMessage != errorMessage {
			t.Fatalf("unexpected child workflow.terminated payload: %+v", payload)
		}
	default:
		t.Fatalf("unsupported child terminal event type %s for run %s", eventType, childRunID)
	}
}

func assertParentChildTerminalEventPayload(
	t *testing.T,
	row enginedb.EngineHistory,
	childRunID uuid.UUID,
	eventType string,
	errorCode string,
	errorMessage string,
) {
	t.Helper()

	switch eventType {
	case enginehistory.EventChildWorkflowFailed:
		payload := mustHistoryPayload[enginehistory.ChildWorkflowFailedPayload](t, row.Payload)
		if payload.ErrorCode != errorCode || payload.ErrorMessage != errorMessage || payload.TerminalChildRunID != childRunID.String() {
			t.Fatalf("unexpected parent child_workflow.failed payload: %+v", payload)
		}
	case enginehistory.EventChildWorkflowCancelled:
		payload := mustHistoryPayload[enginehistory.ChildWorkflowCancelledPayload](t, row.Payload)
		if payload.ErrorCode != errorCode || payload.ErrorMessage != errorMessage || payload.TerminalChildRunID != childRunID.String() {
			t.Fatalf("unexpected parent child_workflow.cancelled payload: %+v", payload)
		}
	case enginehistory.EventChildWorkflowTerminated:
		payload := mustHistoryPayload[enginehistory.ChildWorkflowTerminatedPayload](t, row.Payload)
		if payload.ErrorCode != errorCode || payload.ErrorMessage != errorMessage || payload.TerminalChildRunID != childRunID.String() {
			t.Fatalf("unexpected parent child_workflow.terminated payload: %+v", payload)
		}
	default:
		t.Fatalf("unsupported parent child terminal event type %s", eventType)
	}
}

func mustHistoryPayload[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()

	var payload T
	if err := enginehistory.UnmarshalPayload(raw, &payload); err != nil {
		t.Fatalf("UnmarshalPayload() error = %v", err)
	}
	return payload
}
