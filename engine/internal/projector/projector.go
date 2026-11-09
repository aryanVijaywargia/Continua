// Package projector polls engine history and projects it into the platform
// debugger tables. It owns projection orchestration only — target selection,
// history decoding, event shaping, and terminal cleanup sequencing. Every
// actual write into public.traces / public.spans / public.span_events goes
// through the projection Writer (engine/pkg/projection), which is the single
// owner of the engine→platform write seam shared with the platform-side
// engine-control service.
package projector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicjsonraw "github.com/continua-ai/continua/engine/pkg/jsonraw"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
)

const (
	engineOriginalEventTypeKey = "__continua_original_event_type"
	defaultProjectorBatchSize  = int32(1000)
)

type Projector struct {
	store         *enginestore.Store
	projectFilter *uuid.UUID
	batchSize     int32
}

type projectionTarget struct {
	TraceID                uuid.UUID
	ProjectID              uuid.UUID
	TraceName              string
	RunID                  uuid.UUID
	LatestHistoryID        int64
	LastProjectedHistoryID int64
	ProjectionState        string
	ProjectionUpdatedAt    *time.Time
}

func (t *projectionTarget) traceRef() *publicprojection.TraceRef {
	return &publicprojection.TraceRef{
		ProjectID: t.ProjectID,
		TraceID:   t.TraceID,
		RunID:     t.RunID,
		TraceName: t.TraceName,
	}
}

type terminalProjection struct {
	RunStatus    enginedb.EngineRunLifecycleStatus
	Result       json.RawMessage
	ErrorCode    string
	ErrorMessage string
	Cleanup      terminalCleanup
}

// terminalCleanup closes the work a run's terminal transition abandoned: the spans of
// its cancelled activity tasks and the waits on its discarded timers. Every ending
// cancels its open tasks and discards its open inbox items, completed and failed runs
// too since #359.
type terminalCleanup struct {
	Resolution   string // resolution of the closed waits
	ErrorCode    string // error_code in the output of a closed activity span
	ErrorMessage string // status message of a closed activity span
}

// endedEarly builds the projection of a run that ended before its workflow code
// returned (cancelled, continued as new, terminated). Its open activity spans close
// with the run's own error code and message.
func endedEarly(
	status enginedb.EngineRunLifecycleStatus,
	resolution, errorCode, errorMessage string,
) *terminalProjection {
	return &terminalProjection{
		RunStatus:    status,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Cleanup:      terminalCleanup{Resolution: resolution, ErrorCode: errorCode, ErrorMessage: errorMessage},
	}
}

// finishedRunCleanup is the cleanup of a run that completed or failed. Only the run's
// end cancelled its open tasks, so a closed activity says that, not the run's result
// or error.
func finishedRunCleanup(ending string) terminalCleanup {
	return terminalCleanup{
		Resolution:   "cancelled",
		ErrorCode:    "cancelled",
		ErrorMessage: "activity cancelled: workflow " + ending,
	}
}

func New(store *enginestore.Store) *Projector {
	var projectFilter *uuid.UUID
	if store != nil {
		projectFilter = store.ProjectFilter()
	}
	return &Projector{store: store, projectFilter: projectFilter, batchSize: defaultProjectorBatchSize}
}

func (p *Projector) WithBatchSize(batchSize int32) *Projector {
	if batchSize > 0 {
		p.batchSize = batchSize
	}
	return p
}

func (p *Projector) BatchSize() int32 {
	return p.batchSize
}

func (p *Projector) PollOnce(ctx context.Context, _ string) error {
	for processed := 0; processed < int(p.batchSize); processed++ {
		more, err := p.pollSingleHistoryRow(ctx)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
	return nil
}

func (p *Projector) projectHistoryRows(
	ctx context.Context,
	tx *enginestore.Tx,
	target *projectionTarget,
	rows []enginedb.EngineHistory,
) (lastProjectedID int64, blocked bool, err error) {
	if target == nil {
		return 0, false, errors.New("projection target is required")
	}
	writer := publicprojection.NewWriter(tx.Tx())
	lastProjectedID = target.LastProjectedHistoryID
	for i := range rows {
		row := &rows[i]
		applied, err := writer.WithProjectionBarrier(ctx, target.traceRef(), func() error {
			return projectHistoryRow(ctx, tx, writer, target, row)
		})
		if err != nil {
			return lastProjectedID, false, err
		}
		if !applied {
			return lastProjectedID, true, nil
		}
		lastProjectedID = row.ID
	}
	return lastProjectedID, false, nil
}

func projectHistoryRow(
	ctx context.Context,
	tx *enginestore.Tx,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
) error {
	if target == nil || row == nil {
		return errors.New("projection target and history row are required")
	}
	payload, err := publichistory.DecodePayload(row.EventType, row.Payload)
	if err != nil {
		return err
	}

	switch typed := payload.(type) {
	case *publichistory.WorkflowCompletedPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row, &terminalProjection{
			RunStatus: enginedb.EngineRunLifecycleStatusCompleted,
			Result:    publicjsonraw.Clone(typed.Result),
			Cleanup:   finishedRunCleanup("completed"),
		})
	case *publichistory.WorkflowFailedPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row, &terminalProjection{
			RunStatus:    enginedb.EngineRunLifecycleStatusFailed,
			ErrorCode:    typed.ErrorCode,
			ErrorMessage: typed.ErrorMessage,
			Cleanup:      finishedRunCleanup("failed"),
		})
	case *publichistory.WorkflowCancelledPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row,
			endedEarly(enginedb.EngineRunLifecycleStatusCancelled, "cancelled", "cancelled", "workflow cancelled"))
	case *publichistory.WorkflowContinuedAsNewPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row,
			endedEarly(enginedb.EngineRunLifecycleStatusContinuedAsNew, "continued_as_new", "continued_as_new", "workflow continued as new"))
	case *publichistory.WorkflowTerminatedPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row,
			endedEarly(enginedb.EngineRunLifecycleStatusTerminated, "terminated", typed.ErrorCode, typed.ErrorMessage))
	case *publichistory.ActivityScheduledPayload:
		return projectActivityScheduled(ctx, writer, target, row, typed)
	case *publichistory.ActivityCompletedPayload:
		return projectActivityOutcome(ctx, writer, target, row, completedActivity(typed))
	case *publichistory.ActivityFailedPayload:
		return projectActivityOutcome(ctx, writer, target, row, failedActivity(typed))
	case *publichistory.TimerScheduledPayload:
		return writer.EmitEvent(ctx, &publicprojection.Event{
			ProjectID:  target.ProjectID,
			TraceID:    target.TraceID,
			SpanID:     rootSpanExternalID(target.RunID),
			EventType:  "wait",
			Message:    nil,
			EventTS:    row.CreatedAt,
			Sequence:   row.SequenceNo*10 + 1,
			Payload:    mustMarshalMap(map[string]any{"wait_kind": "timer", "phase": "entered", "wait_id": "timer:" + typed.TimerKey}),
			HistoryID:  row.ID,
			RunID:      target.RunID,
			VariantKey: "timer_wait_entered",
		})
	case *publichistory.TimerFiredPayload:
		return writer.EmitEvent(ctx, &publicprojection.Event{
			ProjectID: target.ProjectID,
			TraceID:   target.TraceID,
			SpanID:    rootSpanExternalID(target.RunID),
			EventType: "wait",
			EventTS:   row.CreatedAt,
			Sequence:  row.SequenceNo*10 + 1,
			Payload: mustMarshalMap(map[string]any{
				"wait_kind":  "timer",
				"phase":      "resolved",
				"wait_id":    "timer:" + typed.TimerKey,
				"resolution": "fired",
			}),
			HistoryID:  row.ID,
			RunID:      target.RunID,
			VariantKey: "timer_wait_resolved",
		})
	default:
		return projectCustomHistoryEvent(ctx, writer, target, row)
	}
}

func projectTerminalHistoryRow(
	ctx context.Context,
	tx *enginestore.Tx,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	projection *terminalProjection,
) error {
	if projection == nil {
		return errors.New("terminal projection is required")
	}
	if err := projectCustomHistoryEvent(ctx, writer, target, row); err != nil {
		return err
	}

	if err := writer.WriteTerminalProjection(ctx, target.traceRef(), row.CreatedAt, &publicprojection.TerminalUpdate{
		RunStatus:    projection.RunStatus,
		Result:       projection.Result,
		ErrorCode:    projection.ErrorCode,
		ErrorMessage: projection.ErrorMessage,
	}); err != nil {
		return err
	}

	return projectTerminalCleanup(ctx, tx, writer, target, row, &projection.Cleanup)
}

func projectTerminalCleanup(
	ctx context.Context,
	tx *enginestore.Tx,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	cleanup *terminalCleanup,
) error {
	endedActivities, err := tx.ListEndedActivityTasksByRun(ctx, target.RunID)
	if err != nil {
		return err
	}

	cancelledActivities, err := tx.ListCancelledActivityTasksByRun(ctx, target.RunID)
	if err != nil {
		return err
	}

	discardedTimers, err := tx.ListDiscardedTimerInboxItemsByRun(ctx, target.RunID)
	if err != nil {
		return err
	}

	sequence := row.SequenceNo*10 + 1
	for i := range endedActivities {
		closed, err := closeUnrecordedActivitySpan(ctx, writer, target, row, &endedActivities[i], sequence)
		if err != nil {
			return err
		}
		if closed {
			sequence += 2
		}
	}

	for i := range cancelledActivities {
		task := cancelledActivities[i]
		if err := writer.CloseTerminalActivitySpan(
			ctx,
			target.traceRef(),
			row.ID,
			row.CreatedAt,
			task.ActivityKey,
			cleanup.ErrorCode,
			cleanup.ErrorMessage,
		); err != nil {
			return err
		}
		if err := writer.EmitEvent(ctx, &publicprojection.Event{
			ProjectID: target.ProjectID,
			TraceID:   target.TraceID,
			SpanID:    rootSpanExternalID(target.RunID),
			EventType: "wait",
			EventTS:   row.CreatedAt,
			Sequence:  sequence,
			Payload: mustMarshalMap(map[string]any{
				"wait_kind":  "activity",
				"phase":      "resolved",
				"wait_id":    "activity:" + task.ActivityKey,
				"resolution": cleanup.Resolution,
			}),
			HistoryID:  row.ID,
			RunID:      target.RunID,
			VariantKey: cleanup.Resolution + ":activity:" + task.ActivityKey,
		}); err != nil {
			return err
		}
		sequence++
	}

	for i := range discardedTimers {
		inboxRow := discardedTimers[i]
		timerPayload := publichistory.TimerScheduledPayload{}
		if err := publichistory.UnmarshalPayload(inboxRow.Payload, &timerPayload); err != nil {
			return fmt.Errorf("decode discarded timer inbox payload: %w", err)
		}

		if err := writer.EmitEvent(ctx, &publicprojection.Event{
			ProjectID: target.ProjectID,
			TraceID:   target.TraceID,
			SpanID:    rootSpanExternalID(target.RunID),
			EventType: "wait",
			EventTS:   row.CreatedAt,
			Sequence:  sequence,
			Payload: mustMarshalMap(map[string]any{
				"wait_kind":  "timer",
				"phase":      "resolved",
				"wait_id":    "timer:" + timerPayload.TimerKey,
				"resolution": cleanup.Resolution,
			}),
			HistoryID:  row.ID,
			RunID:      target.RunID,
			VariantKey: cleanup.Resolution + ":timer:" + timerPayload.TimerKey,
		}); err != nil {
			return err
		}
		sequence++
	}

	return nil
}

// closeUnrecordedActivitySpan closes the open span of a task that a worker completed or
// failed after the run's last activation began, so no history row records its outcome
// (#364). The span and its events show the task's stored result at its completion time
// (the terminal row's time if the task has none). A span that an activity history row
// already closed stays as it is, and so does a span that an earlier projection of this
// terminal row closed.
func closeUnrecordedActivitySpan(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	task *enginedb.EngineActivityTask,
	sequence int32,
) (bool, error) {
	endedAt := row.CreatedAt
	if task.CompletedAt.Valid {
		endedAt = task.CompletedAt.Time
	}
	outcome := endedActivity(task)
	span := outcome.span
	span.EndTime = &endedAt
	closed, err := writer.CloseOpenActivitySpan(ctx, target.traceRef(), row.ID, &span)
	if err != nil || !closed {
		return false, err
	}
	return true, outcome.emitEvents(ctx, writer, target, row.ID, endedAt, sequence, ":activity:"+task.ActivityKey)
}

func projectActivityScheduled(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	payload *publichistory.ActivityScheduledPayload,
) error {
	if err := writer.UpsertActivitySpan(ctx, target.traceRef(), &publicprojection.ActivitySpanUpdate{
		ActivityKey:  payload.ActivityKey,
		ActivityType: payload.ActivityType,
		Status:       "running",
		StartTime:    row.CreatedAt,
		Input:        publicjsonraw.Clone(payload.Input),
	}); err != nil {
		return err
	}

	if err := writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID: target.ProjectID,
		TraceID:   target.TraceID,
		SpanID:    rootSpanExternalID(target.RunID),
		EventType: "effect",
		EventTS:   row.CreatedAt,
		Sequence:  row.SequenceNo*10 + 1,
		Payload: mustMarshalMap(map[string]any{
			"effect_kind":              "activity",
			"has_external_side_effect": true,
			"effect_id":                "activity:" + payload.ActivityKey,
		}),
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "activity_effect",
	}); err != nil {
		return err
	}

	return writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID: target.ProjectID,
		TraceID:   target.TraceID,
		SpanID:    rootSpanExternalID(target.RunID),
		EventType: "wait",
		EventTS:   row.CreatedAt,
		Sequence:  row.SequenceNo*10 + 2,
		Payload: mustMarshalMap(map[string]any{
			"wait_kind": "activity",
			"phase":     "entered",
			"wait_id":   "activity:" + payload.ActivityKey,
		}),
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "activity_wait_entered",
	})
}

// activityOutcome is how an activity task ended: the final state of its span (without
// an end time) and the payload of its outcome decision event.
type activityOutcome struct {
	span     publicprojection.ActivitySpanUpdate
	decision map[string]any
}

func completedActivity(payload *publichistory.ActivityCompletedPayload) *activityOutcome {
	return &activityOutcome{
		span: publicprojection.ActivitySpanUpdate{
			ActivityKey:  payload.ActivityKey,
			ActivityType: payload.ActivityType,
			Status:       "completed",
			Output:       publicjsonraw.Clone(payload.Output),
		},
		decision: map[string]any{
			"question": "activity:" + payload.ActivityKey + ":outcome",
			"chosen":   "completed",
		},
	}
}

func failedActivity(payload *publichistory.ActivityFailedPayload) *activityOutcome {
	return &activityOutcome{
		span: publicprojection.ActivitySpanUpdate{
			ActivityKey:   payload.ActivityKey,
			ActivityType:  payload.ActivityType,
			Status:        "failed",
			StatusMessage: stringPtr(strings.TrimSpace(payload.ErrorMessage)),
			Output: mustMarshalMap(map[string]any{
				"error_code":    payload.ErrorCode,
				"error_message": payload.ErrorMessage,
			}),
		},
		decision: map[string]any{
			"question":  "activity:" + payload.ActivityKey + ":outcome",
			"chosen":    "failed",
			"reasoning": payload.ErrorMessage,
		},
	}
}

// endedActivity is the outcome a completed or failed task stores, built the way a
// replay records it in history.
func endedActivity(task *enginedb.EngineActivityTask) *activityOutcome {
	if task.Status == enginedb.EngineActivityTaskStatusCompleted {
		return completedActivity(&publichistory.ActivityCompletedPayload{
			ActivityKey:  task.ActivityKey,
			ActivityType: task.ActivityType,
			Output:       task.Output,
		})
	}
	return failedActivity(&publichistory.ActivityFailedPayload{
		ActivityKey:  task.ActivityKey,
		ActivityType: task.ActivityType,
		ErrorCode:    stringValue(task.LastErrorCode),
		ErrorMessage: stringValue(task.LastErrorMessage),
	})
}

// emitEvents writes the outcome's decision event at sequence and its resolved activity
// wait at sequence+1. variantSuffix keeps the keys of several outcomes that one history
// row projects apart.
func (o *activityOutcome) emitEvents(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	historyID int64,
	at time.Time,
	sequence int32,
	variantSuffix string,
) error {
	status := o.span.Status
	if err := writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID:  target.ProjectID,
		TraceID:    target.TraceID,
		SpanID:     rootSpanExternalID(target.RunID),
		EventType:  "decision",
		EventTS:    at,
		Sequence:   sequence,
		Payload:    mustMarshalMap(o.decision),
		HistoryID:  historyID,
		RunID:      target.RunID,
		VariantKey: "activity_decision_" + status + variantSuffix,
	}); err != nil {
		return err
	}

	return writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID: target.ProjectID,
		TraceID:   target.TraceID,
		SpanID:    rootSpanExternalID(target.RunID),
		EventType: "wait",
		EventTS:   at,
		Sequence:  sequence + 1,
		Payload: mustMarshalMap(map[string]any{
			"wait_kind":  "activity",
			"phase":      "resolved",
			"wait_id":    "activity:" + o.span.ActivityKey,
			"resolution": status,
		}),
		HistoryID:  historyID,
		RunID:      target.RunID,
		VariantKey: "activity_wait_resolved_" + status + variantSuffix,
	})
}

// projectActivityOutcome projects the activity.completed or activity.failed history row.
func projectActivityOutcome(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	outcome *activityOutcome,
) error {
	span := outcome.span
	span.EndTime = &row.CreatedAt
	if err := writer.UpsertActivitySpan(ctx, target.traceRef(), &span); err != nil {
		return err
	}
	return outcome.emitEvents(ctx, writer, target, row.ID, row.CreatedAt, row.SequenceNo*10+1, "")
}

func projectCustomHistoryEvent(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
) error {
	payload, err := mergeOriginalEventType(row.Payload, row.EventType)
	if err != nil {
		return err
	}

	message := row.EventType
	return writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID:  target.ProjectID,
		TraceID:    target.TraceID,
		SpanID:     rootSpanExternalID(target.RunID),
		EventType:  "custom",
		Message:    &message,
		EventTS:    row.CreatedAt,
		Sequence:   row.SequenceNo * 10,
		Payload:    payload,
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "custom_" + sanitizeVariant(row.EventType),
	})
}

func selectProjectionTarget(ctx context.Context, tx pgx.Tx, projectFilter *uuid.UUID) (projectionTarget, error) {
	var target projectionTarget
	var projectionUpdatedAt pgtypeTimestamptz
	err := tx.QueryRow(ctx, `
		WITH targets AS (
		    SELECT t.id,
		           t.project_id,
		           COALESCE(t.name, t.trace_id) AS trace_name,
		           t.engine_run_id,
		           COALESCE((
		               SELECT MAX(h.id)
		               FROM engine.history AS h
		               WHERE h.run_id = t.engine_run_id
		           ), COALESCE(t.engine_latest_history_id, 0), 0) AS latest_history_id,
		           COALESCE(t.engine_last_projected_history_id, 0) AS last_projected_history_id,
		           COALESCE(t.engine_projection_state, '') AS projection_state,
		           t.engine_projection_updated_at,
		           t.updated_at
		    FROM public.traces AS t
		WHERE t.engine_run_id IS NOT NULL
		  AND ($1::uuid IS NULL OR t.project_id = $1)
		  AND COALESCE(t.engine_projection_state, '') NOT IN ('summary_only', 'journal_expired')
		)
		SELECT id,
		       project_id,
		       trace_name,
		       engine_run_id,
		       latest_history_id,
		       last_projected_history_id,
		       projection_state,
		       engine_projection_updated_at
		FROM targets
		WHERE last_projected_history_id < latest_history_id
		ORDER BY engine_projection_updated_at ASC NULLS FIRST, updated_at ASC, id ASC
		LIMIT 1
	`, nullableProjectFilter(projectFilter)).Scan(
		&target.TraceID,
		&target.ProjectID,
		&target.TraceName,
		&target.RunID,
		&target.LatestHistoryID,
		&target.LastProjectedHistoryID,
		&target.ProjectionState,
		&projectionUpdatedAt,
	)
	if err != nil {
		return projectionTarget{}, err
	}
	if projectionUpdatedAt.Valid {
		target.ProjectionUpdatedAt = &projectionUpdatedAt.Time
	}
	return target, nil
}

func nullableProjectFilter(projectFilter *uuid.UUID) pgtype.UUID {
	if projectFilter == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *projectFilter, Valid: true}
}

func advanceProjectionCheckpointWithBarrier(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	lastProjectedHistoryID int64,
) (bool, error) {
	if target == nil {
		return false, errors.New("projection target is required")
	}
	return writer.WithProjectionBarrier(ctx, target.traceRef(), func() error {
		return writer.AdvanceCheckpoint(ctx, target.TraceID, target.RunID, lastProjectedHistoryID)
	})
}

func (p *Projector) pollSingleHistoryRow(ctx context.Context) (bool, error) {
	tx, err := p.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	target, err := selectProjectionTarget(ctx, tx.Tx(), p.projectFilter)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	writer := publicprojection.NewWriter(tx.Tx())

	historyRows, err := tx.ListHistoryByRunAfterID(ctx, enginedb.ListHistoryByRunAfterIDParams{
		RunID: target.RunID,
		ID:    target.LastProjectedHistoryID,
		Limit: 1,
	})
	if err != nil {
		return false, err
	}
	if len(historyRows) == 0 {
		if _, err := advanceProjectionCheckpointWithBarrier(ctx, writer, &target, target.LastProjectedHistoryID); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}

	lastProjectedID, blocked, err := p.projectHistoryRows(ctx, tx, &target, historyRows)
	if err != nil {
		return false, err
	}
	if blocked {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := writer.RefreshTraceCounters(ctx, target.TraceID); err != nil {
		return false, err
	}
	applied, err := advanceProjectionCheckpointWithBarrier(ctx, writer, &target, lastProjectedID)
	if err != nil {
		return false, err
	}
	if !applied {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	run, err := tx.GetRun(ctx, target.RunID)
	if err != nil {
		return false, err
	}
	if err := writer.SyncRunSummary(ctx, &run); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	p.store.Metrics().AddProjectorRowsProjected(len(historyRows))
	return true, nil
}

func mergeOriginalEventType(raw json.RawMessage, originalEventType string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal(map[string]any{engineOriginalEventTypeKey: originalEventType})
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return json.Marshal(map[string]any{
			engineOriginalEventTypeKey: originalEventType,
			"engine_payload":           raw,
		})
	}

	payload[engineOriginalEventTypeKey] = originalEventType
	return json.Marshal(payload)
}

func rootSpanExternalID(runID uuid.UUID) string {
	return publicprojection.RootSpanExternalID(runID)
}

func activitySpanExternalID(runID uuid.UUID, activityKey string) string {
	return publicprojection.ActivitySpanExternalID(runID, activityKey)
}

func sanitizeVariant(value string) string {
	replacer := strings.NewReplacer(".", "_", ":", "_", "/", "_", " ", "_")
	return replacer.Replace(value)
}

func mustMarshalMap(value map[string]any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type pgtypeTimestamptz struct {
	Time  time.Time
	Valid bool
}

func (p *pgtypeTimestamptz) Scan(src any) error {
	switch value := src.(type) {
	case time.Time:
		p.Time = value
		p.Valid = true
		return nil
	case nil:
		p.Valid = false
		return nil
	default:
		return fmt.Errorf("unsupported timestamptz source %T", src)
	}
}
