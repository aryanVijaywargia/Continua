// Package projector owns all cross-schema writes from engine history into the
// platform debugger tables.
//
// Authoritative platform dependencies:
// - db/platform/migrations/postgres/000001_initial_schema.up.sql
// - db/platform/migrations/postgres/000013_engine_trace_linkage.up.sql
// - db/platform/queries/traces.sql
// - db/platform/queries/spans.sql
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
	engineTracePrefix            = "engine:"
	engineRootSpanPrefix         = "engine:root:"
	engineActivitySpanPrefix     = "engine:activity:"
	engineOriginalEventTypeKey   = "__continua_original_event_type"
	defaultProjectorBatchSize    = int32(1000)
	defaultProjectedEventLevel   = "info"
	defaultProjectedSpanLevel    = "default"
	defaultProjectedSpanTypeRoot = "chain"
	defaultProjectedSpanTypeTool = "tool"
)

var darkLaunchProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type Projector struct {
	store *enginestore.Store
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

func New(store *enginestore.Store) *Projector {
	return &Projector{store: store}
}

func (p *Projector) PollOnce(ctx context.Context, _ string) error {
	tx, err := p.store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	target, err := selectProjectionTarget(ctx, tx.Tx())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	historyRows, err := tx.ListHistoryByRunAfterID(ctx, enginedb.ListHistoryByRunAfterIDParams{
		RunID: target.RunID,
		ID:    target.LastProjectedHistoryID,
		Limit: defaultProjectorBatchSize,
	})
	if err != nil {
		return err
	}
	if len(historyRows) == 0 {
		if err := advanceProjectionCheckpoint(ctx, tx.Tx(), target.TraceID, target.RunID, target.LastProjectedHistoryID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	lastProjectedID, err := p.projectHistoryRows(ctx, tx.Tx(), &target, historyRows)
	if err != nil {
		return err
	}
	if err := refreshTraceCounters(ctx, tx.Tx(), target.TraceID); err != nil {
		return err
	}
	if err := advanceProjectionCheckpoint(ctx, tx.Tx(), target.TraceID, target.RunID, lastProjectedID); err != nil {
		return err
	}
	run, err := tx.GetRun(ctx, target.RunID)
	if err != nil {
		return err
	}
	if err := SyncProjectedRunSummary(ctx, tx.Tx(), &run); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func UpdateLatestHistory(
	ctx context.Context,
	tx pgx.Tx,
	runID uuid.UUID,
	latestHistoryID int64,
) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_latest_history_id = GREATEST(COALESCE(engine_latest_history_id, 0), $2),
		    engine_projection_state = CASE
		        WHEN COALESCE(engine_last_projected_history_id, 0) >= $2 THEN $3
		        ELSE $4
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, runID, latestHistoryID, publicprojection.StateUpToDate.String(), publicprojection.StateCatchingUp.String())
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("projected trace not found for run %s", runID)
	}
	return nil
}

func WriteTerminalSummary(
	ctx context.Context,
	tx pgx.Tx,
	runID uuid.UUID,
	runStatus enginedb.EngineRunLifecycleStatus,
	completedAt time.Time,
	result json.RawMessage,
	errorCode *string,
	errorMessage *string,
	latestHistoryID int64,
) error {
	traceStatus, spanStatus := terminalStatuses(runStatus)
	outputPayload, err := terminalOutputPayload(runStatus, result, errorCode, errorMessage)
	if err != nil {
		return err
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET status = $2,
		    end_time = $3,
		    output = $4,
		    error_count = CASE
		        WHEN $2 IN ('failed', 'cancelled') THEN GREATEST(COALESCE(error_count, 0), 1)
		        ELSE error_count
		    END,
		    engine_latest_history_id = GREATEST(COALESCE(engine_latest_history_id, 0), $5),
		    engine_projection_state = CASE
		        WHEN COALESCE(engine_last_projected_history_id, 0) >= $5 THEN $6
		        ELSE $7
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, runID, traceStatus, completedAt, outputPayload, latestHistoryID, publicprojection.StateUpToDate.String(), publicprojection.StateCatchingUp.String())
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("projected trace not found for run %s", runID)
	}

	rootSpanID := rootSpanExternalID(runID)
	commandTag, err = tx.Exec(ctx, `
		UPDATE public.spans
		SET status = $3,
		    end_time = $4,
		    output = $5,
		    status_message = $6,
		    duration_ms = CASE
		        WHEN $4 IS NOT NULL THEN EXTRACT(EPOCH FROM ($4 - start_time)) * 1000
		        ELSE duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE trace_id = (
		        SELECT id
		        FROM public.traces
		        WHERE engine_run_id = $1
		    )
		  AND span_id = $2
	`, runID, rootSpanID, spanStatus, completedAt, outputPayload, errorMessage)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("projected root span not found for run %s", runID)
	}

	return nil
}

func SyncProjectedRunSummary(
	ctx context.Context,
	tx pgx.Tx,
	run *enginedb.EngineRun,
) error {
	if run == nil {
		return errors.New("run is required")
	}
	queries := enginedb.New(tx)
	pendingActivityTasks, err := queries.CountOpenActivityTasksByRun(ctx, run.ID)
	if err != nil {
		return err
	}

	pendingInboxItems, err := queries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: run.ID, Valid: true})
	if err != nil {
		return err
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_run_status = $2,
		    engine_custom_status = $3,
		    engine_wait_state = $4,
		    engine_pending_activity_tasks = $5,
		    engine_pending_inbox_items = $6,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, run.ID, string(run.Status), cloneRaw(run.CustomStatus), cloneRaw(run.WaitingFor), pendingActivityTasks, pendingInboxItems)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 && run.ProjectID != darkLaunchProjectID {
		return fmt.Errorf("projected trace not found for run %s", run.ID)
	}
	return nil
}

func (p *Projector) projectHistoryRows(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	rows []enginedb.EngineHistory,
) (int64, error) {
	if target == nil {
		return 0, errors.New("projection target is required")
	}
	lastProjectedID := target.LastProjectedHistoryID
	for i := range rows {
		row := &rows[i]
		if err := projectHistoryRow(ctx, tx, target, row); err != nil {
			return lastProjectedID, err
		}
		lastProjectedID = row.ID
	}
	return lastProjectedID, nil
}

func projectHistoryRow(
	ctx context.Context,
	tx pgx.Tx,
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
	case *publichistory.ActivityScheduledPayload:
		return projectActivityScheduled(ctx, tx, target, row, typed)
	case *publichistory.ActivityCompletedPayload:
		return projectActivityCompleted(ctx, tx, target, row, typed)
	case *publichistory.ActivityFailedPayload:
		return projectActivityFailed(ctx, tx, target, row, typed)
	case *publichistory.TimerScheduledPayload:
		return emitProjectedEvent(ctx, tx, &projectedEvent{
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
		return emitProjectedEvent(ctx, tx, &projectedEvent{
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
		return projectCustomHistoryEvent(ctx, tx, target, row)
	}
}

func projectActivityScheduled(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	payload *publichistory.ActivityScheduledPayload,
) error {
	if err := upsertActivitySpan(ctx, tx, target, &activitySpanUpdate{
		ActivityKey:  payload.ActivityKey,
		ActivityType: payload.ActivityType,
		Status:       "running",
		StartTime:    row.CreatedAt,
		Input:        cloneRaw(payload.Input),
	}); err != nil {
		return err
	}

	if err := emitProjectedEvent(ctx, tx, &projectedEvent{
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

	return emitProjectedEvent(ctx, tx, &projectedEvent{
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
	tx pgx.Tx,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	payload *publichistory.ActivityCompletedPayload,
) error {
	if err := upsertActivitySpan(ctx, tx, target, &activitySpanUpdate{
		ActivityKey:  payload.ActivityKey,
		ActivityType: payload.ActivityType,
		Status:       "completed",
		EndTime:      &row.CreatedAt,
		Output:       cloneRaw(payload.Output),
	}); err != nil {
		return err
	}

	if err := emitProjectedEvent(ctx, tx, &projectedEvent{
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

	return emitProjectedEvent(ctx, tx, &projectedEvent{
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
	tx pgx.Tx,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	payload *publichistory.ActivityFailedPayload,
) error {
	if err := upsertActivitySpan(ctx, tx, target, &activitySpanUpdate{
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

	if err := emitProjectedEvent(ctx, tx, &projectedEvent{
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

	return emitProjectedEvent(ctx, tx, &projectedEvent{
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
	tx pgx.Tx,
	target *projectionTarget,
	row *enginedb.EngineHistory,
) error {
	payload, err := mergeOriginalEventType(row.Payload, row.EventType)
	if err != nil {
		return err
	}

	message := row.EventType
	return emitProjectedEvent(ctx, tx, &projectedEvent{
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

type activitySpanUpdate struct {
	ActivityKey   string
	ActivityType  string
	Status        string
	StartTime     time.Time
	EndTime       *time.Time
	Input         json.RawMessage
	Output        json.RawMessage
	StatusMessage *string
}

func upsertActivitySpan(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	update *activitySpanUpdate,
) error {
	if target == nil || update == nil {
		return errors.New("projection target and activity update are required")
	}
	spanID := activitySpanExternalID(target.RunID, update.ActivityKey)
	commandTag, err := tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    parent_span_id,
		    name,
		    type,
		    status,
		    status_message,
		    level,
		    start_time,
		    end_time,
		    input,
		    output,
		    depth
		)
		VALUES (
		    $1,
		    $2,
		    $3,
		    $4,
		    $5,
		    $6,
		    $7,
		    $8,
		    $9,
		    $10,
		    $11,
		    $12,
		    $13,
		    1
		)
		ON CONFLICT (trace_id, span_id) DO UPDATE SET
		    name = COALESCE(EXCLUDED.name, public.spans.name),
		    type = COALESCE(EXCLUDED.type, public.spans.type),
		    status = COALESCE(EXCLUDED.status, public.spans.status),
		    status_message = COALESCE(EXCLUDED.status_message, public.spans.status_message),
		    start_time = COALESCE(public.spans.start_time, EXCLUDED.start_time),
		    end_time = COALESCE(EXCLUDED.end_time, public.spans.end_time),
		    input = COALESCE(public.spans.input, EXCLUDED.input),
		    output = COALESCE(EXCLUDED.output, public.spans.output),
		    duration_ms = CASE
		        WHEN COALESCE(EXCLUDED.end_time, public.spans.end_time) IS NOT NULL
		         AND COALESCE(public.spans.start_time, EXCLUDED.start_time) IS NOT NULL
		        THEN EXTRACT(EPOCH FROM (
		            COALESCE(EXCLUDED.end_time, public.spans.end_time) - COALESCE(public.spans.start_time, EXCLUDED.start_time)
		        )) * 1000
		        ELSE public.spans.duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(public.spans.version, 1) + 1
	`, target.ProjectID, target.TraceID, spanID, stringPtr(rootSpanExternalID(target.RunID)), update.ActivityType, defaultProjectedSpanTypeTool, update.Status, update.StatusMessage, defaultProjectedSpanLevel, update.StartTime, update.EndTime, update.Input, update.Output)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("failed to upsert activity span %s for run %s", update.ActivityKey, target.RunID)
	}
	return nil
}

type projectedEvent struct {
	ProjectID  uuid.UUID
	TraceID    uuid.UUID
	SpanID     string
	EventType  string
	Message    *string
	EventTS    time.Time
	Sequence   int32
	Payload    json.RawMessage
	HistoryID  int64
	RunID      uuid.UUID
	VariantKey string
}

func emitProjectedEvent(ctx context.Context, tx pgx.Tx, event *projectedEvent) error {
	if event == nil {
		return errors.New("projected event is required")
	}
	idempotencyKey := fmt.Sprintf("%s:%d:%s", event.RunID.String(), event.HistoryID, event.VariantKey)
	commandTag, err := tx.Exec(ctx, `
		INSERT INTO public.span_events (
		    project_id,
		    trace_id,
		    span_id,
		    event_type,
		    level,
		    event_ts,
		    server_ingested_at,
		    sequence,
		    message,
		    payload,
		    idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, $9, $10)
		ON CONFLICT (project_id, idempotency_key) DO NOTHING
	`, event.ProjectID, event.TraceID, event.SpanID, event.EventType, defaultProjectedEventLevel, event.EventTS, event.Sequence, event.Message, event.Payload, idempotencyKey)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func selectProjectionTarget(ctx context.Context, tx pgx.Tx) (projectionTarget, error) {
	var target projectionTarget
	var projectionUpdatedAt pgtypeTimestamptz
	err := tx.QueryRow(ctx, `
		SELECT id,
		       project_id,
		       COALESCE(name, trace_id),
		       engine_run_id,
		       COALESCE(engine_latest_history_id, 0),
		       COALESCE(engine_last_projected_history_id, 0),
		       COALESCE(engine_projection_state, ''),
		       engine_projection_updated_at
		FROM public.traces
		WHERE engine_run_id IS NOT NULL
		  AND COALESCE(engine_last_projected_history_id, 0) < COALESCE(engine_latest_history_id, 0)
		ORDER BY engine_projection_updated_at ASC NULLS FIRST, updated_at ASC, id ASC
		LIMIT 1
	`).Scan(
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

func advanceProjectionCheckpoint(
	ctx context.Context,
	tx pgx.Tx,
	traceID uuid.UUID,
	runID uuid.UUID,
	lastProjectedHistoryID int64,
) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_last_projected_history_id = CASE
		        WHEN COALESCE(engine_last_projected_history_id, 0) < $3 THEN $3
		        ELSE engine_last_projected_history_id
		    END,
		    engine_projection_state = CASE
		        WHEN COALESCE(engine_latest_history_id, 0) <= GREATEST(COALESCE(engine_last_projected_history_id, 0), $3) THEN $4
		        ELSE $5
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		  AND engine_run_id = $2
	`, traceID, runID, lastProjectedHistoryID, publicprojection.StateUpToDate.String(), publicprojection.StateCatchingUp.String())
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("projection checkpoint target missing for run %s", runID)
	}
	return nil
}

func refreshTraceCounters(ctx context.Context, tx pgx.Tx, traceID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		WITH counts AS (
		    SELECT COUNT(*)::integer AS total_spans,
		           COUNT(*) FILTER (WHERE status IN ('failed', 'error'))::integer AS error_count
		    FROM public.spans
		    WHERE trace_id = $1
		)
		UPDATE public.traces
		SET total_spans = counts.total_spans,
		    error_count = counts.error_count,
		    updated_at = NOW()
		FROM counts
		WHERE public.traces.id = $1
	`, traceID)
	return err
}

func terminalStatuses(status enginedb.EngineRunLifecycleStatus) (traceStatus, spanStatus string) {
	switch status {
	case enginedb.EngineRunLifecycleStatusCompleted:
		return "completed", "completed"
	case enginedb.EngineRunLifecycleStatusCancelled:
		return "cancelled", "failed"
	default:
		return "failed", "failed"
	}
}

func terminalOutputPayload(
	status enginedb.EngineRunLifecycleStatus,
	result json.RawMessage,
	errorCode *string,
	errorMessage *string,
) (json.RawMessage, error) {
	if status == enginedb.EngineRunLifecycleStatusCompleted {
		return cloneRaw(result), nil
	}
	return json.Marshal(map[string]any{
		"error_code":    derefString(errorCode),
		"error_message": derefString(errorMessage),
		"status":        string(status),
	})
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
	return engineRootSpanPrefix + runID.String()
}

func activitySpanExternalID(runID uuid.UUID, activityKey string) string {
	return engineActivitySpanPrefix + runID.String() + ":" + activityKey
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

func derefString(value *string) string {
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
