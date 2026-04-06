package projector

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
	if err := projectHistoryRow(fixture.ctx, tx, &target, &rows[0]); err != nil {
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

func TestProjectorBarrier_ConcurrentPurgeToSummaryOnlySkipsDetailWrites(t *testing.T) {
	fixture := newProjectorFixture(t)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())

	tx, err := fixture.store.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()

	target, err := selectProjectionTarget(fixture.ctx, tx.Tx())
	if err != nil {
		t.Fatalf("selectProjectionTarget() error = %v", err)
	}
	rows, err := tx.ListHistoryByRunAfterID(fixture.ctx, enginedb.ListHistoryByRunAfterIDParams{
		RunID: fixture.run.ID,
		ID:    fixture.startedHistory.ID,
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListHistoryByRunAfterID() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one pending history row, got %d", len(rows))
	}

	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateSummaryOnly.String())

	lastProjectedID, blocked, err := fixture.projector.projectHistoryRows(fixture.ctx, tx, &target, rows)
	if err != nil {
		t.Fatalf("projectHistoryRows() error = %v", err)
	}
	if !blocked {
		t.Fatal("expected projection barrier to block detail writes after purge flips state to summary_only")
	}
	if lastProjectedID != fixture.startedHistory.ID {
		t.Fatalf("expected checkpoint to remain at %d, got %d", fixture.startedHistory.ID, lastProjectedID)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-summary-only-barrier"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	assertProjectedDetailState(t, fixture, publicprojection.StateSummaryOnly.String(), fixture.startedHistory.ID, 0, 0)
}

func TestProjectorBarrier_JournalExpiredSkipsReprojection(t *testing.T) {
	fixture := newProjectorFixture(t)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateJournalExpired.String())

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-journal-expired-noop"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	assertProjectedDetailState(t, fixture, publicprojection.StateJournalExpired.String(), fixture.startedHistory.ID, 0, 0)
}

func TestProjectorBarrier_ConcurrentFullPurgeBlocksCheckpointWithoutHistoryRows(t *testing.T) {
	fixture := newProjectorFixture(t)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())

	tx, err := fixture.store.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()

	target, err := selectProjectionTarget(fixture.ctx, tx.Tx())
	if err != nil {
		t.Fatalf("selectProjectionTarget() error = %v", err)
	}

	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		DELETE FROM engine.history
		WHERE run_id = $1
		  AND id = $2
	`, fixture.run.ID, activityHistory.ID); err != nil {
		t.Fatalf("delete projected history during concurrent purge: %v", err)
	}
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateJournalExpired.String())

	rows, err := tx.ListHistoryByRunAfterID(fixture.ctx, enginedb.ListHistoryByRunAfterIDParams{
		RunID: fixture.run.ID,
		ID:    fixture.startedHistory.ID,
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListHistoryByRunAfterID() error = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected zero remaining history rows after purge, got %d", len(rows))
	}

	applied, err := advanceProjectionCheckpointWithBarrier(fixture.ctx, tx.Tx(), &target, target.LastProjectedHistoryID)
	if err != nil {
		t.Fatalf("advanceProjectionCheckpointWithBarrier() error = %v", err)
	}
	if applied {
		t.Fatal("expected checkpoint advance to be blocked by journal_expired barrier")
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	assertProjectedDetailState(t, fixture, publicprojection.StateJournalExpired.String(), fixture.startedHistory.ID, 0, 0)
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

func TestProjectorPollOnce_RepairFromSummaryOnlyRebuildsDetailFromCheckpoint(t *testing.T) {
	fixture := newProjectorFixture(t)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})

	// Simulate a purged shell that still has retained history after an operator-triggered repair
	// flips the trace back to catching_up.
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateSummaryOnly.String())
	assertProjectedDetailState(t, fixture, publicprojection.StateSummaryOnly.String(), fixture.startedHistory.ID, 0, 0)
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-repair-catch-up"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	assertProjectedDetailState(t, fixture, publicprojection.StateUpToDate.String(), activityHistory.ID, 1, 2)

	var activitySpanCount int
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, fixture.traceID, activitySpanExternalID(fixture.run.ID, "ship-order")).Scan(&activitySpanCount); err != nil {
		t.Fatalf("count activity spans: %v", err)
	}
	if activitySpanCount != 1 {
		t.Fatalf("expected exactly one repaired activity span, got %d", activitySpanCount)
	}
}

func TestProjectorPollOnce_TerminatedHistoryProjectsFailureAndCleanup(t *testing.T) {
	fixture := newProjectorFixture(t)
	baseTime := time.Now().UTC().Round(time.Microsecond)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})
	timerHistory := fixture.appendHistoryEvent(3, publichistory.EventTimerScheduled, publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(5 * time.Minute),
	})

	if _, err := fixture.store.CreateActivityTask(fixture.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    fixture.projectID,
		InstanceID:   fixture.instance.ID,
		RunID:        fixture.run.ID,
		HistoryID:    &activityHistory.ID,
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
		AvailableAt:  baseTime.Add(time.Minute),
	}); err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}

	timerPayload, err := publichistory.MarshalPayload(publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("MarshalPayload(timer) error = %v", err)
	}
	if _, err := fixture.store.CreateInboxItem(fixture.ctx, enginedb.CreateInboxItemParams{
		ProjectID:   fixture.projectID,
		InstanceID:  fixture.instance.ID,
		RunID:       pgtype.UUID{Bytes: fixture.run.ID, Valid: true},
		HistoryID:   &timerHistory.ID,
		Kind:        "timer",
		Payload:     timerPayload,
		AvailableAt: baseTime.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateInboxItem(timer) error = %v", err)
	}

	fixture.setTraceProjection(timerHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())
	if err := fixture.projector.PollOnce(fixture.ctx, "projector-terminal-prep"); err != nil {
		t.Fatalf("PollOnce() initial projection error = %v", err)
	}

	if _, err := fixture.store.CancelOpenActivityTasksByRun(fixture.ctx, fixture.run.ID); err != nil {
		t.Fatalf("CancelOpenActivityTasksByRun() error = %v", err)
	}
	if _, err := fixture.store.DiscardOpenInboxItemsByRun(fixture.ctx, fixture.run.ID); err != nil {
		t.Fatalf("DiscardOpenInboxItemsByRun() error = %v", err)
	}

	terminatedAt := baseTime.Add(6 * time.Minute)
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.runs
		SET status = 'terminated',
		    waiting_for = NULL,
		    result = NULL,
		    completed_at = $2,
		    last_error_code = 'terminated',
		    last_error_message = 'run terminated by operator',
		    updated_at = $2
		WHERE id = $1
	`, fixture.run.ID, terminatedAt); err != nil {
		t.Fatalf("mark run terminated: %v", err)
	}
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.instances
		SET status = 'terminated',
		    updated_at = $2
		WHERE id = $1
	`, fixture.instance.ID, terminatedAt); err != nil {
		t.Fatalf("mark instance terminated: %v", err)
	}
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE public.traces
		SET engine_wait_state = $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, fixture.traceID, mustJSON(t, map[string]any{"kind": "signal", "signal_name": "approval"})); err != nil {
		t.Fatalf("seed projected wait state: %v", err)
	}

	terminalHistory := fixture.appendHistoryEvent(4, publichistory.EventWorkflowTerminated, publichistory.WorkflowTerminatedPayload{
		ErrorCode:    "terminated",
		ErrorMessage: "run terminated by operator",
	})

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-terminated"); err != nil {
		t.Fatalf("PollOnce() terminal projection error = %v", err)
	}

	var traceStatus string
	var runStatus string
	var waitState []byte
	var pendingActivityTasks int64
	var pendingInboxItems int64
	var traceOutput []byte
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status,
		       engine_run_status,
		       engine_wait_state,
		       engine_pending_activity_tasks,
		       engine_pending_inbox_items,
		       output
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&traceStatus, &runStatus, &waitState, &pendingActivityTasks, &pendingInboxItems, &traceOutput); err != nil {
		t.Fatalf("query terminated trace summary: %v", err)
	}
	if traceStatus != "failed" {
		t.Fatalf("expected failed trace status for terminated run, got %q", traceStatus)
	}
	if runStatus != string(enginedb.EngineRunLifecycleStatusTerminated) {
		t.Fatalf("expected projected terminated engine status, got %q", runStatus)
	}
	if len(waitState) != 0 {
		t.Fatalf("expected terminal projection to clear wait state, got %s", waitState)
	}
	if pendingActivityTasks != 0 || pendingInboxItems != 0 {
		t.Fatalf("expected terminal projection to clear pending counts, got activity=%d inbox=%d", pendingActivityTasks, pendingInboxItems)
	}

	var outputPayload map[string]any
	if err := json.Unmarshal(traceOutput, &outputPayload); err != nil {
		t.Fatalf("json.Unmarshal(trace output) error = %v", err)
	}
	if outputPayload["error_code"] != "terminated" || outputPayload["error_message"] != "run terminated by operator" {
		t.Fatalf("unexpected terminated trace output: %+v", outputPayload)
	}

	var rootSpanStatus string
	var rootSpanMessage *string
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status, status_message
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, fixture.traceID, rootSpanExternalID(fixture.run.ID)).Scan(&rootSpanStatus, &rootSpanMessage); err != nil {
		t.Fatalf("query terminated root span: %v", err)
	}
	if rootSpanStatus != "failed" {
		t.Fatalf("expected failed root span for terminated run, got %q", rootSpanStatus)
	}
	if rootSpanMessage == nil || *rootSpanMessage != "run terminated by operator" {
		t.Fatalf("expected terminated root span message, got %+v", rootSpanMessage)
	}

	var activitySpanStatus string
	var activitySpanMessage *string
	var activitySpanMetadata []byte
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status, status_message, metadata
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, fixture.traceID, activitySpanExternalID(fixture.run.ID, "ship-order")).Scan(&activitySpanStatus, &activitySpanMessage, &activitySpanMetadata); err != nil {
		t.Fatalf("query terminated activity span: %v", err)
	}
	if activitySpanStatus != "failed" {
		t.Fatalf("expected failed activity span after termination, got %q", activitySpanStatus)
	}
	if activitySpanMessage == nil || *activitySpanMessage != "run terminated by operator" {
		t.Fatalf("expected terminated activity span message, got %+v", activitySpanMessage)
	}
	var metadata map[string]any
	if err := json.Unmarshal(activitySpanMetadata, &metadata); err != nil {
		t.Fatalf("json.Unmarshal(activity span metadata) error = %v", err)
	}
	if metadata[terminalHistoryMetadataKey] != float64(terminalHistory.ID) {
		t.Fatalf("expected terminal history metadata %d, got %+v", terminalHistory.ID, metadata)
	}

	resolvedWaits := queryResolvedWaitEvents(t, fixture, "terminated")
	if len(resolvedWaits) != 2 {
		t.Fatalf("expected 2 terminated wait resolution events, got %+v", resolvedWaits)
	}
	if resolvedWaits[0].Sequence != terminalHistory.SequenceNo*10+1 || resolvedWaits[0].WaitID != "activity:ship-order" {
		t.Fatalf("unexpected first terminated wait event: %+v", resolvedWaits[0])
	}
	if resolvedWaits[1].Sequence != terminalHistory.SequenceNo*10+2 || resolvedWaits[1].WaitID != "timer:deadline" {
		t.Fatalf("unexpected second terminated wait event: %+v", resolvedWaits[1])
	}

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-terminated-idempotent"); err != nil {
		t.Fatalf("PollOnce() idempotent projection error = %v", err)
	}
	resolvedWaitsAfterReplay := queryResolvedWaitEvents(t, fixture, "terminated")
	if len(resolvedWaitsAfterReplay) != len(resolvedWaits) {
		t.Fatalf("expected terminated cleanup replay to stay idempotent, got before=%d after=%d", len(resolvedWaits), len(resolvedWaitsAfterReplay))
	}
}

func TestProjectorPollOnce_CancelledHistoryClearsPureSignalWaitWithoutSyntheticSignalEvent(t *testing.T) {
	fixture := newProjectorFixture(t)
	cancelledAt := time.Now().UTC().Round(time.Microsecond)

	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.runs
		SET status = 'cancelled',
		    waiting_for = NULL,
		    result = NULL,
		    completed_at = $2,
		    last_error_code = 'cancelled',
		    last_error_message = 'workflow cancelled',
		    updated_at = $2
		WHERE id = $1
	`, fixture.run.ID, cancelledAt); err != nil {
		t.Fatalf("mark run cancelled: %v", err)
	}
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.instances
		SET status = 'cancelled',
		    updated_at = $2
		WHERE id = $1
	`, fixture.instance.ID, cancelledAt); err != nil {
		t.Fatalf("mark instance cancelled: %v", err)
	}
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE public.traces
		SET engine_wait_state = $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, fixture.traceID, mustJSON(t, map[string]any{"kind": "signal", "signal_name": "approval"})); err != nil {
		t.Fatalf("seed signal wait state: %v", err)
	}

	fixture.appendHistoryEvent(2, publichistory.EventWorkflowCancelled, publichistory.WorkflowCancelledPayload{})

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-cancelled-signal"); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	var traceStatus string
	var waitState []byte
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT status, engine_wait_state
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&traceStatus, &waitState); err != nil {
		t.Fatalf("query cancelled trace summary: %v", err)
	}
	if traceStatus != "cancelled" {
		t.Fatalf("expected cancelled trace status, got %q", traceStatus)
	}
	if len(waitState) != 0 {
		t.Fatalf("expected cancelled projection to clear pure signal wait state, got %s", waitState)
	}
	if resolvedWaits := queryResolvedWaitEvents(t, fixture, "cancelled"); len(resolvedWaits) != 0 {
		t.Fatalf("expected no synthetic signal wait resolution events, got %+v", resolvedWaits)
	}
}

func TestProjectorPollOnce_ClearsWaitStateOnEveryTerminalTransition(t *testing.T) {
	testCases := []struct {
		name           string
		runStatus      enginedb.EngineRunLifecycleStatus
		instanceStatus enginedb.EngineInstanceLifecycleStatus
		eventType      string
		payload        any
		result         []byte
		errorCode      *string
		errorMessage   *string
	}{
		{
			name:           "completed",
			runStatus:      enginedb.EngineRunLifecycleStatusCompleted,
			instanceStatus: enginedb.EngineInstanceLifecycleStatusCompleted,
			eventType:      publichistory.EventWorkflowCompleted,
			payload: publichistory.WorkflowCompletedPayload{
				Result: mustJSON(t, map[string]bool{"ok": true}),
			},
			result: mustJSON(t, map[string]bool{"ok": true}),
		},
		{
			name:           "failed",
			runStatus:      enginedb.EngineRunLifecycleStatusFailed,
			instanceStatus: enginedb.EngineInstanceLifecycleStatusFailed,
			eventType:      publichistory.EventWorkflowFailed,
			payload: publichistory.WorkflowFailedPayload{
				ErrorCode:    "failed",
				ErrorMessage: "workflow failed",
			},
			errorCode:    enginetest.Ptr("failed"),
			errorMessage: enginetest.Ptr("workflow failed"),
		},
		{
			name:           "cancelled",
			runStatus:      enginedb.EngineRunLifecycleStatusCancelled,
			instanceStatus: enginedb.EngineInstanceLifecycleStatusCancelled,
			eventType:      publichistory.EventWorkflowCancelled,
			payload:        publichistory.WorkflowCancelledPayload{},
			errorCode:      enginetest.Ptr("cancelled"),
			errorMessage:   enginetest.Ptr("workflow cancelled"),
		},
		{
			name:           "terminated",
			runStatus:      enginedb.EngineRunLifecycleStatusTerminated,
			instanceStatus: enginedb.EngineInstanceLifecycleStatusTerminated,
			eventType:      publichistory.EventWorkflowTerminated,
			payload: publichistory.WorkflowTerminatedPayload{
				ErrorCode:    "terminated",
				ErrorMessage: "run terminated by operator",
			},
			errorCode:    enginetest.Ptr("terminated"),
			errorMessage: enginetest.Ptr("run terminated by operator"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newProjectorFixture(t)
			completedAt := time.Now().UTC().Round(time.Microsecond)

			if _, err := fixture.db.Pool.Exec(fixture.ctx, `
				UPDATE engine.runs
				SET status = $2,
				    waiting_for = NULL,
				    result = $3,
				    completed_at = $4,
				    last_error_code = $5,
				    last_error_message = $6,
				    updated_at = $4
				WHERE id = $1
			`, fixture.run.ID, tc.runStatus, tc.result, completedAt, tc.errorCode, tc.errorMessage); err != nil {
				t.Fatalf("mark run terminal: %v", err)
			}
			if _, err := fixture.db.Pool.Exec(fixture.ctx, `
				UPDATE engine.instances
				SET status = $2,
				    updated_at = $3
				WHERE id = $1
			`, fixture.instance.ID, tc.instanceStatus, completedAt); err != nil {
				t.Fatalf("mark instance terminal: %v", err)
			}
			if _, err := fixture.db.Pool.Exec(fixture.ctx, `
				UPDATE public.traces
				SET engine_wait_state = $2::jsonb,
				    updated_at = NOW()
				WHERE id = $1
			`, fixture.traceID, mustJSON(t, map[string]any{"kind": "signal", "signal_name": "approval"})); err != nil {
				t.Fatalf("seed wait state: %v", err)
			}

			terminalHistory := fixture.appendHistoryEvent(2, tc.eventType, tc.payload)
			fixture.setTraceProjection(terminalHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())

			if err := fixture.projector.PollOnce(fixture.ctx, "projector-terminal-clear-"+tc.name); err != nil {
				t.Fatalf("PollOnce() error = %v", err)
			}

			var waitState []byte
			if err := fixture.db.Pool.QueryRow(fixture.ctx, `
				SELECT engine_wait_state
				FROM public.traces
				WHERE id = $1
			`, fixture.traceID).Scan(&waitState); err != nil {
				t.Fatalf("query wait state: %v", err)
			}
			if len(waitState) != 0 {
				t.Fatalf("expected terminal projection to clear wait state, got %s", waitState)
			}
		})
	}
}

func TestProjectorPollOnce_TerminatedCleanupSkipsAlreadyCompletedActivities(t *testing.T) {
	fixture := newProjectorFixture(t)
	baseTime := time.Now().UTC().Round(time.Microsecond)

	activityHistory := fixture.appendHistoryEvent(2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
	})
	activityTask, err := fixture.store.CreateActivityTask(fixture.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    fixture.projectID,
		InstanceID:   fixture.instance.ID,
		RunID:        fixture.run.ID,
		HistoryID:    &activityHistory.ID,
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Input:        mustJSON(t, map[string]any{"order_id": "ord-123"}),
		AvailableAt:  baseTime,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}
	fixture.setTraceProjection(activityHistory.ID, fixture.startedHistory.ID, publicprojection.StateCatchingUp.String())
	if err := fixture.projector.PollOnce(fixture.ctx, "projector-activity-started"); err != nil {
		t.Fatalf("PollOnce(activity scheduled) error = %v", err)
	}

	completedHistory := fixture.appendHistoryEvent(3, publichistory.EventActivityCompleted, publichistory.ActivityCompletedPayload{
		ActivityKey:  "ship-order",
		ActivityType: "demo.ship",
		Output:       mustJSON(t, map[string]bool{"ok": true}),
	})
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.activity_tasks
		SET status = 'completed',
		    output = $2,
		    completed_at = $3,
		    updated_at = $3
		WHERE id = $1
	`, activityTask.ID, mustJSON(t, map[string]bool{"ok": true}), baseTime.Add(time.Second)); err != nil {
		t.Fatalf("mark activity task completed: %v", err)
	}
	fixture.setTraceProjection(completedHistory.ID, activityHistory.ID, publicprojection.StateCatchingUp.String())
	if err := fixture.projector.PollOnce(fixture.ctx, "projector-activity-completed"); err != nil {
		t.Fatalf("PollOnce(activity completed) error = %v", err)
	}

	timerHistory := fixture.appendHistoryEvent(4, publichistory.EventTimerScheduled, publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(5 * time.Minute),
	})
	timerPayload, err := publichistory.MarshalPayload(publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("MarshalPayload(timer) error = %v", err)
	}
	if _, err := fixture.store.CreateInboxItem(fixture.ctx, enginedb.CreateInboxItemParams{
		ProjectID:   fixture.projectID,
		InstanceID:  fixture.instance.ID,
		RunID:       pgtype.UUID{Bytes: fixture.run.ID, Valid: true},
		HistoryID:   &timerHistory.ID,
		Kind:        "timer",
		Payload:     timerPayload,
		AvailableAt: baseTime.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateInboxItem(timer) error = %v", err)
	}

	terminatedAt := baseTime.Add(2 * time.Second)
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.runs
		SET status = 'terminated',
		    waiting_for = NULL,
		    completed_at = $2,
		    last_error_code = 'terminated',
		    last_error_message = 'run terminated by operator',
		    updated_at = $2
		WHERE id = $1
	`, fixture.run.ID, terminatedAt); err != nil {
		t.Fatalf("mark run terminated: %v", err)
	}
	if _, err := fixture.db.Pool.Exec(fixture.ctx, `
		UPDATE engine.instances
		SET status = 'terminated',
		    updated_at = $2
		WHERE id = $1
	`, fixture.instance.ID, terminatedAt); err != nil {
		t.Fatalf("mark instance terminated: %v", err)
	}
	if _, err := fixture.store.DiscardOpenInboxItemsByRun(fixture.ctx, fixture.run.ID); err != nil {
		t.Fatalf("DiscardOpenInboxItemsByRun() error = %v", err)
	}

	terminalHistory := fixture.appendHistoryEvent(5, publichistory.EventWorkflowTerminated, publichistory.WorkflowTerminatedPayload{
		ErrorCode:    "terminated",
		ErrorMessage: "run terminated by operator",
	})
	fixture.setTraceProjection(terminalHistory.ID, completedHistory.ID, publicprojection.StateCatchingUp.String())

	if err := fixture.projector.PollOnce(fixture.ctx, "projector-terminate-after-completion"); err != nil {
		t.Fatalf("PollOnce(terminated) error = %v", err)
	}

	resolvedWaits := queryResolvedWaitEvents(t, fixture, "terminated")
	if len(resolvedWaits) != 1 {
		t.Fatalf("expected only timer cleanup event after completed activity, got %+v", resolvedWaits)
	}
	if resolvedWaits[0].WaitID != "timer:deadline" {
		t.Fatalf("expected terminated cleanup to resolve only the timer wait, got %+v", resolvedWaits)
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

type resolvedWaitEvent struct {
	Sequence int32
	WaitID   string
}

func queryResolvedWaitEvents(t *testing.T, fixture *projectorFixture, resolution string) []resolvedWaitEvent {
	t.Helper()

	rows, err := fixture.db.Pool.Query(fixture.ctx, `
		SELECT sequence, payload
		FROM public.span_events
		WHERE trace_id = $1
		  AND event_type = 'wait'
		ORDER BY sequence ASC, created_at ASC
	`, fixture.traceID)
	if err != nil {
		t.Fatalf("query wait events: %v", err)
	}
	defer rows.Close()

	var resolved []resolvedWaitEvent
	for rows.Next() {
		var sequence int32
		var payload []byte
		if err := rows.Scan(&sequence, &payload); err != nil {
			t.Fatalf("scan wait event: %v", err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatalf("json.Unmarshal(wait event payload) error = %v", err)
		}
		if decoded["resolution"] != resolution {
			continue
		}
		waitID, _ := decoded["wait_id"].(string)
		resolved = append(resolved, resolvedWaitEvent{
			Sequence: sequence,
			WaitID:   waitID,
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate wait events: %v", err)
	}

	return resolved
}

func assertProjectedDetailState(
	t *testing.T,
	fixture *projectorFixture,
	wantProjectionState string,
	wantLastProjectedHistoryID int64,
	wantActivitySpans int,
	wantSpanEvents int,
) {
	t.Helper()

	var projectionState string
	var lastProjectedHistoryID int64
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT engine_projection_state,
		       engine_last_projected_history_id
		FROM public.traces
		WHERE id = $1
	`, fixture.traceID).Scan(&projectionState, &lastProjectedHistoryID); err != nil {
		t.Fatalf("query trace projection state: %v", err)
	}
	if projectionState != wantProjectionState {
		t.Fatalf("expected projection state %q, got %q", wantProjectionState, projectionState)
	}
	if lastProjectedHistoryID != wantLastProjectedHistoryID {
		t.Fatalf("expected last projected history id %d, got %d", wantLastProjectedHistoryID, lastProjectedHistoryID)
	}

	var activitySpanCount int
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM public.spans
		WHERE trace_id = $1
		  AND span_id LIKE 'engine:activity:%'
	`, fixture.traceID).Scan(&activitySpanCount); err != nil {
		t.Fatalf("count activity spans: %v", err)
	}
	if activitySpanCount != wantActivitySpans {
		t.Fatalf("expected %d activity spans, got %d", wantActivitySpans, activitySpanCount)
	}

	var spanEventCount int
	if err := fixture.db.Pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM public.span_events
		WHERE trace_id = $1
	`, fixture.traceID).Scan(&spanEventCount); err != nil {
		t.Fatalf("count span events: %v", err)
	}
	if spanEventCount != wantSpanEvents {
		t.Fatalf("expected %d span events, got %d", wantSpanEvents, spanEventCount)
	}
}
