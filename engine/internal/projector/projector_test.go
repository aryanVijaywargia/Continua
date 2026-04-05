package projector

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
)

func TestProjectorPollOnce_RestartSafeForProjectedActivityRows(t *testing.T) {
	fixture := newProjectorFixture(t)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "send-email",
		ActivityType: "demo.email",
		Input:        mustJSON(t, map[string]any{"to": "ada@example.com"}),
	})
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())

	tx, err := fixture.store.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	target, err := selectProjectionTarget(fixture.ctx, tx.Tx())
	if err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("selectProjectionTarget() error = %v", err)
	}
	rows, err := tx.ListHistoryByRunAfterID(fixture.ctx, enginedb.ListHistoryByRunAfterIDParams{
		RunID: fixture.run.ID,
		ID:    fixture.startedHistory.ID,
		Limit: defaultProjectorBatchSize,
	})
	if err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("ListHistoryByRunAfterID() error = %v", err)
	}
	if len(rows) != 1 {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("expected 1 history row, got %d", len(rows))
	}
	if err := projectHistoryRow(fixture.ctx, tx.Tx(), &target, &rows[0]); err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("projectHistoryRow() error = %v", err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-test"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	var traceState string
	var lastProjectedHistoryID int64
	var totalSpans int32
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT engine_projection_state,
		       engine_last_projected_history_id,
		       total_spans
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&traceState, &lastProjectedHistoryID, &totalSpans); err != nil {
		t.Fatalf("query projected trace: %v", err)
	}
	if traceState != publicprojection.StateUpToDate.String() {
		t.Fatalf("expected trace to be up_to_date, got %q", traceState)
	}
	if lastProjectedHistoryID != activityHistory.ID {
		t.Fatalf("expected last projected history id %d, got %d", activityHistory.ID, lastProjectedHistoryID)
	}
	if totalSpans != 2 {
		t.Fatalf("expected root span plus activity span, got %d spans", totalSpans)
	}

	var activitySpanCount int
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, fixture.traceID, activitySpanExternalID(fixture.run.ID, "send-email")).Scan(&activitySpanCount); err != nil {
		t.Fatalf("count activity spans: %v", err)
	}
	if activitySpanCount != 1 {
		t.Fatalf("expected exactly one projected activity span, got %d", activitySpanCount)
	}

	rowsAfterRestart, err := fixture.db.Pool.Query(fixture.ctx, `
		SELECT event_type, payload
		FROM public.span_events
		WHERE trace_id = $1
		ORDER BY sequence ASC, created_at ASC
	`, fixture.traceID)
	if err != nil {
		t.Fatalf("query projected events: %v", err)
	}
	defer rowsAfterRestart.Close()

	type projectedEventRow struct {
		eventType string
		payload   []byte
	}

	var events []projectedEventRow
	for rowsAfterRestart.Next() {
		var row projectedEventRow
		if err := rowsAfterRestart.Scan(&row.eventType, &row.payload); err != nil {
			t.Fatalf("scan projected event: %v", err)
		}
		events = append(events, row)
	}
	if err := rowsAfterRestart.Err(); err != nil {
		t.Fatalf("iterate projected events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected two projected events after restart-safe replay, got %d", len(events))
	}
	if events[0].eventType != "effect" || events[1].eventType != "wait" {
		t.Fatalf("unexpected projected event types: %+v", events)
	}

	var effectPayload map[string]any
	if err := json.Unmarshal(events[0].payload, &effectPayload); err != nil {
		t.Fatalf("json.Unmarshal(effect payload) error = %v", err)
	}
	if effectPayload["effect_id"] != "activity:send-email" {
		t.Fatalf("unexpected effect payload: %+v", effectPayload)
	}

	var waitPayload map[string]any
	if err := json.Unmarshal(events[1].payload, &waitPayload); err != nil {
		t.Fatalf("json.Unmarshal(wait payload) error = %v", err)
	}
	if waitPayload["wait_kind"] != "activity" || waitPayload["phase"] != "entered" {
		t.Fatalf("unexpected wait payload: %+v", waitPayload)
	}
}

func TestProjectorPollOnce_DoesNotOverwriteTerminalTraceSummary(t *testing.T) {
	fixture := newProjectorFixture(t)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "send-email",
		ActivityType: "demo.email",
		Input:        mustJSON(t, map[string]any{"to": "ada@example.com"}),
	})
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())

	completedAt := time.Now().UTC().Round(time.Microsecond)
	result := mustJSON(t, map[string]bool{"ok": true})

	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    result = $2,
		    completed_at = $3,
		    updated_at = $3
		WHERE id = $1
	`, fixture.run.ID, result, completedAt); err != nil {
		t.Fatalf("mark run completed: %v", err)
	}

	tx, err := fixture.store.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := WriteTerminalSummary(
		fixture.ctx,
		tx.Tx(),
		fixture.projectID,
		fixture.run.ID,
		enginedb.EngineRunLifecycleStatusCompleted,
		completedAt,
		result,
		nil,
		nil,
		activityHistory.ID,
	); err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("WriteTerminalSummary() error = %v", err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-terminal-guard"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	var traceStatus string
	var traceEndTime time.Time
	var traceOutput []byte
	var runStatus string
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status,
		       end_time,
		       output,
		       engine_run_status
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&traceStatus, &traceEndTime, &traceOutput, &runStatus); err != nil {
		t.Fatalf("query terminal trace summary: %v", err)
	}
	if traceStatus != "completed" {
		t.Fatalf("expected completed trace status, got %q", traceStatus)
	}
	if !traceEndTime.Equal(completedAt) {
		t.Fatalf("expected completed end_time %s, got %s", completedAt, traceEndTime)
	}
	if !jsonEqual(traceOutput, result) {
		t.Fatalf("expected terminal trace output %s, got %s", result, traceOutput)
	}
	if runStatus != string(enginedb.EngineRunLifecycleStatusCompleted) {
		t.Fatalf("expected projected run status completed, got %q", runStatus)
	}

	var rootSpanStatus string
	var rootSpanEndTime time.Time
	var rootSpanOutput []byte
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status,
		       end_time,
		       output
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, fixture.traceID, rootSpanExternalID(fixture.run.ID)).Scan(&rootSpanStatus, &rootSpanEndTime, &rootSpanOutput); err != nil {
		t.Fatalf("query terminal root span: %v", err)
	}
	if rootSpanStatus != "completed" {
		t.Fatalf("expected completed root span status, got %q", rootSpanStatus)
	}
	if !rootSpanEndTime.Equal(completedAt) {
		t.Fatalf("expected completed root span end_time %s, got %s", completedAt, rootSpanEndTime)
	}
	if !jsonEqual(rootSpanOutput, result) {
		t.Fatalf("expected terminal root span output %s, got %s", result, rootSpanOutput)
	}
}

func jsonEqual(left, right []byte) bool {
	var leftCompact bytes.Buffer
	if err := json.Compact(&leftCompact, left); err != nil {
		return false
	}

	var rightCompact bytes.Buffer
	if err := json.Compact(&rightCompact, right); err != nil {
		return false
	}

	return leftCompact.String() == rightCompact.String()
}

func TestAdvanceProjectionCheckpoint_IsMonotonicForStaleUpdates(t *testing.T) {
	fixture := newProjectorFixture(t)

	tx, err := fixture.store.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := advanceProjectionCheckpoint(fixture.ctx, tx.Tx(), fixture.traceID, fixture.run.ID, 0); err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("advanceProjectionCheckpoint() error = %v", err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	var lastProjectedHistoryID int64
	var traceState string
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT engine_last_projected_history_id,
		       engine_projection_state
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&lastProjectedHistoryID, &traceState); err != nil {
		t.Fatalf("query checkpoint state: %v", err)
	}
	if lastProjectedHistoryID != fixture.startedHistory.ID {
		t.Fatalf("expected checkpoint to remain at %d, got %d", fixture.startedHistory.ID, lastProjectedHistoryID)
	}
	if traceState != publicprojection.StateUpToDate.String() {
		t.Fatalf("expected state to remain up_to_date, got %q", traceState)
	}
}

func TestProjectionStateAdvancesAcrossStartActivationAndProjector(t *testing.T) {
	fixture := newProjectorFixture(t)

	var latestHistoryID int64
	var lastProjectedHistoryID int64
	var traceState string
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT engine_latest_history_id,
		       engine_last_projected_history_id,
		       engine_projection_state
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&latestHistoryID, &lastProjectedHistoryID, &traceState); err != nil {
		t.Fatalf("query start projection state: %v", err)
	}
	if latestHistoryID != fixture.startedHistory.ID || lastProjectedHistoryID != fixture.startedHistory.ID {
		t.Fatalf("expected start shell to begin caught up, got latest=%d last=%d", latestHistoryID, lastProjectedHistoryID)
	}
	if traceState != publicprojection.StateUpToDate.String() {
		t.Fatalf("expected start shell to be up_to_date, got %q", traceState)
	}

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})

	tx, err := fixture.store.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := UpdateLatestHistory(fixture.ctx, tx.Tx(), fixture.projectID, fixture.run.ID, activityHistory.ID); err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatalf("UpdateLatestHistory() error = %v", err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT engine_latest_history_id,
		       engine_last_projected_history_id,
		       engine_projection_state
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&latestHistoryID, &lastProjectedHistoryID, &traceState); err != nil {
		t.Fatalf("query post-activation projection state: %v", err)
	}
	if latestHistoryID != activityHistory.ID {
		t.Fatalf("expected activation to advance latest history to %d, got %d", activityHistory.ID, latestHistoryID)
	}
	if lastProjectedHistoryID != fixture.startedHistory.ID {
		t.Fatalf("expected last projected history id to remain at %d before projector catch-up, got %d", fixture.startedHistory.ID, lastProjectedHistoryID)
	}
	if traceState != publicprojection.StateCatchingUp.String() {
		t.Fatalf("expected activation to move trace into catching_up, got %q", traceState)
	}

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-state-transition"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT engine_latest_history_id,
		       engine_last_projected_history_id,
		       engine_projection_state
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&latestHistoryID, &lastProjectedHistoryID, &traceState); err != nil {
		t.Fatalf("query post-projector projection state: %v", err)
	}
	if latestHistoryID != activityHistory.ID || lastProjectedHistoryID != activityHistory.ID {
		t.Fatalf("expected projector to catch up to history id %d, got latest=%d last=%d", activityHistory.ID, latestHistoryID, lastProjectedHistoryID)
	}
	if traceState != publicprojection.StateUpToDate.String() {
		t.Fatalf("expected projector to restore up_to_date, got %q", traceState)
	}
}

func TestSyncProjectedRunSummary_MissingProjectedTraceFailsForPublicRuns(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	instance, run := createStartedRun(t, store, workflowTestCase{
		projectID:         projectID,
		instanceKey:       "instance-missing-trace",
		definitionName:    "missing-trace",
		definitionVersion: "v1",
	})

	_ = instance

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	err = SyncProjectedRunSummary(ctx, tx.Tx(), &run)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("expected SyncProjectedRunSummary to fail when projected trace is missing")
	}
}

func TestSyncProjectedRunSummary_AllowsDarkLaunchRunsWithoutProjectedTrace(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	_, run := createStartedRun(t, store, workflowTestCase{
		projectID:         darkLaunchProjectID,
		instanceKey:       "instance-darklaunch",
		definitionName:    "darklaunch",
		definitionVersion: "v1",
	})

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := SyncProjectedRunSummary(ctx, tx.Tx(), &run); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SyncProjectedRunSummary() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}
}

type projectorFixture struct {
	t              *testing.T
	ctx            context.Context
	db             *enginetest.TestDatabase
	store          *enginestore.Store
	projector      *Projector
	projectID      uuid.UUID
	instance       enginedb.EngineInstance
	run            enginedb.EngineRun
	traceID        uuid.UUID
	startedHistory enginedb.EngineHistory
}

func newProjectorFixture(t *testing.T) *projectorFixture {
	t.Helper()

	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	instance, run := createStartedRun(t, store, workflowTestCase{
		projectID:         projectID,
		instanceKey:       "instance-" + uuid.NewString()[:8],
		definitionName:    "projector-demo",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]any{"cart_id": "cart-123"}),
	})

	historyRows, err := store.GetHistoryByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(historyRows) != 1 {
		t.Fatalf("expected one started history row, got %d", len(historyRows))
	}

	traceID := uuid.New()
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
		    $9,
		    $10,
		    $10,
		    NOW()
		)
	`, traceID, projectID, engineTracePrefix+run.ID.String(), "Projected Demo", run.ID, instance.InstanceKey, instance.DefinitionName, run.DefinitionVersion, publicprojection.StateUpToDate.String(), historyRows[0].ID); err != nil {
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
		VALUES ($1, $2, $3, $4, 'chain', 'running', 'default', NOW(), 0)
	`, projectID, traceID, rootSpanExternalID(run.ID), "Projected Demo"); err != nil {
		t.Fatalf("insert projected root span: %v", err)
	}

	return &projectorFixture{
		t:              t,
		ctx:            ctx,
		db:             db,
		store:          store,
		projector:      New(store),
		projectID:      projectID,
		instance:       instance,
		run:            run,
		traceID:        traceID,
		startedHistory: historyRows[0],
	}
}

func (f *projectorFixture) appendHistoryEvent(sequenceNo int32, eventType string, payload any) enginedb.EngineHistory {
	f.t.Helper()

	raw, err := publichistory.MarshalPayload(payload)
	if err != nil {
		f.t.Fatalf("MarshalPayload() error = %v", err)
	}
	row, err := f.store.AppendHistory(f.ctx, enginedb.AppendHistoryParams{
		ProjectID:  f.projectID,
		InstanceID: f.instance.ID,
		RunID:      f.run.ID,
		SequenceNo: sequenceNo,
		EventType:  eventType,
		Payload:    raw,
	})
	if err != nil {
		f.t.Fatalf("AppendHistory() error = %v", err)
	}
	return row
}

func (f *projectorFixture) setTraceProjection(latestHistoryID, lastProjectedHistoryID int64, projectionState string) {
	f.t.Helper()

	if _, err := f.db.Pool.Exec(f.ctx, `
		UPDATE public.traces
		SET engine_latest_history_id = $2,
		    engine_last_projected_history_id = $3,
		    engine_projection_state = $4,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, f.traceID, latestHistoryID, lastProjectedHistoryID, projectionState); err != nil {
		f.t.Fatalf("update trace projection state: %v", err)
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

	raw, err := publichistory.MarshalPayload(publichistory.WorkflowStartedPayload{
		DefinitionName:    testCase.definitionName,
		DefinitionVersion: testCase.definitionVersion,
		InstanceKey:       testCase.instanceKey,
		Input:             testCase.input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}
	if _, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  testCase.projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  publichistory.EventWorkflowStarted,
		Payload:    raw,
	}); err != nil {
		t.Fatalf("AppendHistory() error = %v", err)
	}

	return instance, run
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
