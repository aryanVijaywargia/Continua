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
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
)

const (
	engineOriginalEventTypeKey = "__continua_original_event_type"
	defaultProjectorBatchSize  = int32(1000)
)

type Projector struct {
	store         *enginestore.Store
	projectFilter *uuid.UUID
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
	RunStatus     enginedb.EngineRunLifecycleStatus
	Result        json.RawMessage
	ErrorCode     string
	ErrorMessage  string
	CleanupReason string
}

func New(store *enginestore.Store) *Projector {
	var projectFilter *uuid.UUID
	if store != nil {
		projectFilter = store.ProjectFilter()
	}
	return &Projector{store: store, projectFilter: projectFilter}
}

func (p *Projector) PollOnce(ctx context.Context, _ string) error {
	for processed := 0; processed < int(defaultProjectorBatchSize); processed++ {
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
			Result:    cloneRaw(typed.Result),
		})
	case *publichistory.WorkflowFailedPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row, &terminalProjection{
			RunStatus:    enginedb.EngineRunLifecycleStatusFailed,
			ErrorCode:    typed.ErrorCode,
			ErrorMessage: typed.ErrorMessage,
		})
	case *publichistory.WorkflowCancelledPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row, &terminalProjection{
			RunStatus:     enginedb.EngineRunLifecycleStatusCancelled,
			ErrorCode:     "cancelled",
			ErrorMessage:  "workflow cancelled",
			CleanupReason: "cancelled",
		})
	case *publichistory.WorkflowContinuedAsNewPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row, &terminalProjection{
			RunStatus:     enginedb.EngineRunLifecycleStatusContinuedAsNew,
			ErrorCode:     "continued_as_new",
			ErrorMessage:  "workflow continued as new",
			CleanupReason: "continued_as_new",
		})
	case *publichistory.WorkflowTerminatedPayload:
		return projectTerminalHistoryRow(ctx, tx, writer, target, row, &terminalProjection{
			RunStatus:     enginedb.EngineRunLifecycleStatusTerminated,
			ErrorCode:     typed.ErrorCode,
			ErrorMessage:  typed.ErrorMessage,
			CleanupReason: "terminated",
		})
	case *publichistory.ActivityScheduledPayload:
		return projectActivityScheduled(ctx, writer, target, row, typed)
	case *publichistory.ActivityCompletedPayload:
		return projectActivityCompleted(ctx, writer, target, row, typed)
	case *publichistory.ActivityFailedPayload:
		return projectActivityFailed(ctx, writer, target, row, typed)
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

	if projection.CleanupReason != "" {
		if err := projectTerminalCleanup(ctx, tx, writer, target, row, projection); err != nil {
			return err
		}
	}

	return nil
}

func projectTerminalCleanup(
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
	cancelledActivities, err := tx.ListCancelledActivityTasksByRun(ctx, target.RunID)
	if err != nil {
		return err
	}

	discardedTimers, err := tx.ListDiscardedTimerInboxItemsByRun(ctx, target.RunID)
	if err != nil {
		return err
	}

	sequence := row.SequenceNo*10 + 1
	for i := range cancelledActivities {
		task := cancelledActivities[i]
		if err := writer.CloseTerminalActivitySpan(
			ctx,
			target.traceRef(),
			row.ID,
			row.CreatedAt,
			task.ActivityKey,
			projection.ErrorCode,
			projection.ErrorMessage,
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
				"resolution": projection.CleanupReason,
			}),
			HistoryID:  row.ID,
			RunID:      target.RunID,
			VariantKey: projection.CleanupReason + ":activity:" + task.ActivityKey,
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
				"resolution": projection.CleanupReason,
			}),
			HistoryID:  row.ID,
			RunID:      target.RunID,
			VariantKey: projection.CleanupReason + ":timer:" + timerPayload.TimerKey,
		}); err != nil {
			return err
		}
		sequence++
	}

	return nil
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
		Input:        cloneRaw(payload.Input),
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

func projectActivityCompleted(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	payload *publichistory.ActivityCompletedPayload,
) error {
	if err := writer.UpsertActivitySpan(ctx, target.traceRef(), &publicprojection.ActivitySpanUpdate{
		ActivityKey:  payload.ActivityKey,
		ActivityType: payload.ActivityType,
		Status:       "completed",
		EndTime:      &row.CreatedAt,
		Output:       cloneRaw(payload.Output),
	}); err != nil {
		return err
	}

	if err := writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID: target.ProjectID,
		TraceID:   target.TraceID,
		SpanID:    rootSpanExternalID(target.RunID),
		EventType: "decision",
		EventTS:   row.CreatedAt,
		Sequence:  row.SequenceNo*10 + 1,
		Payload: mustMarshalMap(map[string]any{
			"question": "activity:" + payload.ActivityKey + ":outcome",
			"chosen":   "completed",
		}),
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "activity_decision_completed",
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
			"wait_kind":  "activity",
			"phase":      "resolved",
			"wait_id":    "activity:" + payload.ActivityKey,
			"resolution": "completed",
		}),
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "activity_wait_resolved_completed",
	})
}

func projectActivityFailed(
	ctx context.Context,
	writer *publicprojection.Writer,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	payload *publichistory.ActivityFailedPayload,
) error {
	if err := writer.UpsertActivitySpan(ctx, target.traceRef(), &publicprojection.ActivitySpanUpdate{
		ActivityKey:   payload.ActivityKey,
		ActivityType:  payload.ActivityType,
		Status:        "failed",
		EndTime:       &row.CreatedAt,
		StatusMessage: stringPtr(strings.TrimSpace(payload.ErrorMessage)),
		Output: mustMarshalMap(map[string]any{
			"error_code":    payload.ErrorCode,
			"error_message": payload.ErrorMessage,
		}),
	}); err != nil {
		return err
	}

	if err := writer.EmitEvent(ctx, &publicprojection.Event{
		ProjectID: target.ProjectID,
		TraceID:   target.TraceID,
		SpanID:    rootSpanExternalID(target.RunID),
		EventType: "decision",
		EventTS:   row.CreatedAt,
		Sequence:  row.SequenceNo*10 + 1,
		Payload: mustMarshalMap(map[string]any{
			"question":  "activity:" + payload.ActivityKey + ":outcome",
			"chosen":    "failed",
			"reasoning": payload.ErrorMessage,
		}),
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "activity_decision_failed",
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
			"wait_kind":  "activity",
			"phase":      "resolved",
			"wait_id":    "activity:" + payload.ActivityKey,
			"resolution": "failed",
		}),
		HistoryID:  row.ID,
		RunID:      target.RunID,
		VariantKey: "activity_wait_resolved_failed",
	})
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

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
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
