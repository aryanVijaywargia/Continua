// Package projector owns all cross-schema writes from engine history into the
// platform debugger tables, including terminal debugger cleanup when
// workflow.cancelled or workflow.terminated history rows are projected. Purge
// is the only coordinated co-writer for engine projection state, and it must
// go through the platform-side CAS helpers before deleting detail rows.
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
	terminalHistoryMetadataKey   = "__continua_terminal_history_id"
)

var darkLaunchProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

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

func UpdateLatestHistory(
	ctx context.Context,
	tx pgx.Tx,
	projectID uuid.UUID,
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
	if commandTag.RowsAffected() == 0 && projectID != darkLaunchProjectID {
		return fmt.Errorf("projected trace not found for run %s", runID)
	}
	return nil
}

func WriteTerminalSummary(
	ctx context.Context,
	tx pgx.Tx,
	projectID uuid.UUID,
	runID uuid.UUID,
	runStatus enginedb.EngineRunLifecycleStatus,
	completedAt time.Time,
	result json.RawMessage,
	errorCode *string,
	errorMessage *string,
	latestHistoryID int64,
) error {
	traceStatus, spanStatus := publicprojection.TerminalStatuses(string(runStatus))
	outputPayload, err := publicprojection.TerminalOutputPayload(string(runStatus), result, errorCode, errorMessage)
	if err != nil {
		return err
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET status = $2,
		    end_time = $3::timestamptz,
		    output = $4::jsonb,
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
	if commandTag.RowsAffected() == 0 && projectID != darkLaunchProjectID {
		return fmt.Errorf("projected trace not found for run %s", runID)
	}

	rootSpanID := rootSpanExternalID(runID)
	commandTag, err = tx.Exec(ctx, `
		UPDATE public.spans
		SET status = $3,
		    end_time = $4::timestamptz,
		    output = $5::jsonb,
		    status_message = $6::text,
		    duration_ms = CASE
		        WHEN $4::timestamptz IS NOT NULL THEN EXTRACT(EPOCH FROM ($4::timestamptz - start_time)) * 1000
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
	`, runID, rootSpanID, spanStatus, completedAt, outputPayload, nullableText(errorMessage))
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 && projectID != darkLaunchProjectID {
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

	// CountOpenInboxByRun excludes cancel rows so projected counts mirror the
	// public pending-work surface instead of internal control intents.
	pendingInboxItems, err := queries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: run.ID, Valid: true})
	if err != nil {
		return err
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_run_status = $2,
		    engine_custom_status = $3::jsonb,
		    engine_wait_state = $4::jsonb,
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
	tx *enginestore.Tx,
	target *projectionTarget,
	rows []enginedb.EngineHistory,
) (lastProjectedID int64, blocked bool, err error) {
	if target == nil {
		return 0, false, errors.New("projection target is required")
	}
	lastProjectedID = target.LastProjectedHistoryID
	for i := range rows {
		row := &rows[i]
		applied, err := withProjectionBarrier(ctx, tx.Tx(), target, func() error {
			return projectHistoryRow(ctx, tx, target, row)
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
		return projectTerminalHistoryRow(ctx, tx, target, row, &terminalProjection{
			RunStatus: enginedb.EngineRunLifecycleStatusCompleted,
			Result:    cloneRaw(typed.Result),
		})
	case *publichistory.WorkflowFailedPayload:
		return projectTerminalHistoryRow(ctx, tx, target, row, &terminalProjection{
			RunStatus:    enginedb.EngineRunLifecycleStatusFailed,
			ErrorCode:    typed.ErrorCode,
			ErrorMessage: typed.ErrorMessage,
		})
	case *publichistory.WorkflowCancelledPayload:
		return projectTerminalHistoryRow(ctx, tx, target, row, &terminalProjection{
			RunStatus:     enginedb.EngineRunLifecycleStatusCancelled,
			ErrorCode:     "cancelled",
			ErrorMessage:  "workflow cancelled",
			CleanupReason: "cancelled",
		})
	case *publichistory.WorkflowContinuedAsNewPayload:
		return projectTerminalHistoryRow(ctx, tx, target, row, &terminalProjection{
			RunStatus:     enginedb.EngineRunLifecycleStatusContinuedAsNew,
			ErrorCode:     "continued_as_new",
			ErrorMessage:  "workflow continued as new",
			CleanupReason: "continued_as_new",
		})
	case *publichistory.WorkflowTerminatedPayload:
		return projectTerminalHistoryRow(ctx, tx, target, row, &terminalProjection{
			RunStatus:     enginedb.EngineRunLifecycleStatusTerminated,
			ErrorCode:     typed.ErrorCode,
			ErrorMessage:  typed.ErrorMessage,
			CleanupReason: "terminated",
		})
	case *publichistory.ActivityScheduledPayload:
		return projectActivityScheduled(ctx, tx.Tx(), target, row, typed)
	case *publichistory.ActivityCompletedPayload:
		return projectActivityCompleted(ctx, tx.Tx(), target, row, typed)
	case *publichistory.ActivityFailedPayload:
		return projectActivityFailed(ctx, tx.Tx(), target, row, typed)
	case *publichistory.TimerScheduledPayload:
		return emitProjectedEvent(ctx, tx.Tx(), &projectedEvent{
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
		return emitProjectedEvent(ctx, tx.Tx(), &projectedEvent{
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
		return projectCustomHistoryEvent(ctx, tx.Tx(), target, row)
	}
}

func projectTerminalHistoryRow(
	ctx context.Context,
	tx *enginestore.Tx,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	projection *terminalProjection,
) error {
	if err := projectCustomHistoryEvent(ctx, tx.Tx(), target, row); err != nil {
		return err
	}

	if err := writeTerminalProjection(ctx, tx.Tx(), target, row.CreatedAt, projection); err != nil {
		return err
	}

	if projection.CleanupReason != "" {
		if err := projectTerminalCleanup(ctx, tx, target, row, projection); err != nil {
			return err
		}
	}

	return nil
}

func writeTerminalProjection(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	completedAt time.Time,
	projection *terminalProjection,
) error {
	if target == nil || projection == nil {
		return errors.New("projection target is required")
	}

	traceStatus, spanStatus := publicprojection.TerminalStatuses(string(projection.RunStatus))
	outputPayload, err := publicprojection.TerminalOutputPayload(
		string(projection.RunStatus),
		projection.Result,
		stringPtr(projection.ErrorCode),
		stringPtr(projection.ErrorMessage),
	)
	if err != nil {
		return err
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE public.traces
		SET status = $2,
		    end_time = $3::timestamptz,
		    output = $4::jsonb,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE id = $1
	`, target.TraceID, traceStatus, completedAt, outputPayload)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 && target.ProjectID != darkLaunchProjectID {
		return fmt.Errorf("projected trace not found for run %s", target.RunID)
	}

	if err := upsertTerminalRootSpan(
		ctx,
		tx,
		target,
		spanStatus,
		completedAt,
		outputPayload,
		stringPtr(projection.ErrorMessage),
	); err != nil {
		return err
	}

	return nil
}

func upsertTerminalRootSpan(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	spanStatus string,
	completedAt time.Time,
	outputPayload json.RawMessage,
	statusMessage *string,
) error {
	if target == nil {
		return errors.New("projection target is required")
	}

	commandTag, err := tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    name,
		    type,
		    status,
		    level,
		    start_time,
		    end_time,
		    output,
		    status_message,
		    duration_ms,
		    depth
		)
		SELECT $1,
		       t.id,
		       $3,
		       COALESCE(NULLIF($4, ''), NULLIF(t.name, ''), 'workflow'),
		       $5,
		       $6,
		       $7,
		       COALESCE(t.start_time, $8::timestamptz),
		       $8::timestamptz,
		       $9::jsonb,
		       $10::text,
		       EXTRACT(EPOCH FROM ($8::timestamptz - COALESCE(t.start_time, $8::timestamptz))) * 1000,
		       0
		FROM public.traces AS t
		WHERE t.id = $2
		ON CONFLICT (trace_id, span_id) DO UPDATE
		SET status = EXCLUDED.status,
		    end_time = EXCLUDED.end_time,
		    output = EXCLUDED.output,
		    status_message = EXCLUDED.status_message,
		    duration_ms = EXCLUDED.duration_ms,
		    updated_at = NOW(),
		    version = COALESCE(spans.version, 1) + 1
	`, target.ProjectID, target.TraceID, rootSpanExternalID(target.RunID), target.TraceName, defaultProjectedSpanTypeRoot, spanStatus, defaultProjectedSpanLevel, completedAt, outputPayload, statusMessage)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 && target.ProjectID != darkLaunchProjectID {
		return fmt.Errorf("projected trace not found for run %s", target.RunID)
	}
	return nil
}

func projectTerminalCleanup(
	ctx context.Context,
	tx *enginestore.Tx,
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
		if err := closeTerminalActivitySpan(ctx, tx.Tx(), target, row, &task, projection); err != nil {
			return err
		}
		if err := emitProjectedEvent(ctx, tx.Tx(), &projectedEvent{
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

		if err := emitProjectedEvent(ctx, tx.Tx(), &projectedEvent{
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

func closeTerminalActivitySpan(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	row *enginedb.EngineHistory,
	task *enginedb.EngineActivityTask,
	projection *terminalProjection,
) error {
	if target == nil || row == nil || task == nil || projection == nil {
		return errors.New("projection target, history row, and activity task are required")
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE public.spans
		SET status = 'failed',
		    status_message = $3,
		    end_time = $4::timestamptz,
		    output = $5::jsonb,
		    metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object($6::text, to_jsonb($7::bigint)),
		    duration_ms = CASE
		        WHEN start_time IS NOT NULL THEN EXTRACT(EPOCH FROM ($4::timestamptz - start_time)) * 1000
		        ELSE duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE trace_id = $1
		  AND span_id = $2
		  AND end_time IS NULL
	`, target.TraceID, activitySpanExternalID(target.RunID, task.ActivityKey), projection.ErrorMessage, row.CreatedAt, mustMarshalMap(map[string]any{
		"error_code":    projection.ErrorCode,
		"error_message": projection.ErrorMessage,
	}), terminalHistoryMetadataKey, row.ID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return nil
	}
	return nil
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
		ON CONFLICT (project_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, event.ProjectID, event.TraceID, event.SpanID, event.EventType, defaultProjectedEventLevel, event.EventTS, event.Sequence, event.Message, event.Payload, idempotencyKey)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return nil
	}
	return nil
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

func advanceProjectionCheckpoint(
	ctx context.Context,
	tx pgx.Tx,
	traceID uuid.UUID,
	runID uuid.UUID,
	lastProjectedHistoryID int64,
) error {
	commandTag, err := tx.Exec(ctx, `
		WITH latest AS (
		    SELECT COALESCE(MAX(id), 0) AS latest_history_id
		    FROM engine.history
		    WHERE run_id = $2
		)
		UPDATE public.traces
		SET engine_latest_history_id = GREATEST(COALESCE(public.traces.engine_latest_history_id, 0), latest.latest_history_id),
		    engine_last_projected_history_id = CASE
		        WHEN COALESCE(public.traces.engine_last_projected_history_id, 0) < $3 THEN $3
		        ELSE public.traces.engine_last_projected_history_id
		    END,
		    engine_projection_state = CASE
		        WHEN latest.latest_history_id <= GREATEST(COALESCE(public.traces.engine_last_projected_history_id, 0), $3) THEN $4
		        ELSE $5
		    END,
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW()
		FROM latest
		WHERE public.traces.id = $1
		  AND public.traces.engine_run_id = $2
	`, traceID, runID, lastProjectedHistoryID, publicprojection.StateUpToDate.String(), publicprojection.StateCatchingUp.String())
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("projection checkpoint target missing for run %s", runID)
	}
	return nil
}

func advanceProjectionCheckpointWithBarrier(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	lastProjectedHistoryID int64,
) (bool, error) {
	if target == nil {
		return false, errors.New("projection target is required")
	}
	return withProjectionBarrier(ctx, tx, target, func() error {
		return advanceProjectionCheckpoint(ctx, tx, target.TraceID, target.RunID, lastProjectedHistoryID)
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

	historyRows, err := tx.ListHistoryByRunAfterID(ctx, enginedb.ListHistoryByRunAfterIDParams{
		RunID: target.RunID,
		ID:    target.LastProjectedHistoryID,
		Limit: 1,
	})
	if err != nil {
		return false, err
	}
	if len(historyRows) == 0 {
		if _, err := advanceProjectionCheckpointWithBarrier(ctx, tx.Tx(), &target, target.LastProjectedHistoryID); err != nil {
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
	if err := refreshTraceCounters(ctx, tx.Tx(), target.TraceID); err != nil {
		return false, err
	}
	applied, err := advanceProjectionCheckpointWithBarrier(ctx, tx.Tx(), &target, lastProjectedID)
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
	if err := SyncProjectedRunSummary(ctx, tx.Tx(), &run); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func withProjectionBarrier(
	ctx context.Context,
	tx pgx.Tx,
	target *projectionTarget,
	write func() error,
) (bool, error) {
	if target == nil {
		return false, errors.New("projection target is required")
	}
	if write == nil {
		return false, errors.New("projection write is required")
	}

	var projectionState string
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(engine_projection_state, $3)
		FROM public.traces
		WHERE id = $1
		  AND engine_run_id = $2
		FOR UPDATE
	`, target.TraceID, target.RunID, publicprojection.StateUpToDate.String()).Scan(&projectionState)
	if err != nil {
		return false, err
	}
	if projectionState == publicprojection.StateSummaryOnly.String() ||
		projectionState == publicprojection.StateJournalExpired.String() {
		return false, nil
	}

	if err := write(); err != nil {
		return false, err
	}
	return true, nil
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

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
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
