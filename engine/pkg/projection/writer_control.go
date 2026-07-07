// writer_control.go holds the Writer methods used by the platform-side
// engine-control service (internal/enginecontrol): locking the run+trace
// pair, lineage backfill, terminal-shell enforcement, and purge cleanup of
// projected detail and the engine history journal.
package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

// LockedRunTrace is the row-locked run+trace pair every control-plane
// maintenance operation works against.
type LockedRunTrace struct {
	// TraceID is the internal public.traces.id row id.
	TraceID                      uuid.UUID
	EngineProjectionState        *string
	EngineLastProjectedHistoryID *int64
	Run                          enginedb.EngineRun
}

// LockRunTrace locks the projected trace row (FOR UPDATE) and loads the
// matching engine run, scoped to the requesting project. Returns
// ErrRunNotFound when either side of the pair is missing.
func (w *Writer) LockRunTrace(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (*LockedRunTrace, error) {
	locked := &LockedRunTrace{}
	var (
		projectionState        pgtype.Text
		lastProjectedHistoryID pgtype.Int8
	)
	err := w.tx.QueryRow(ctx, `
		SELECT id,
		       engine_projection_state,
		       engine_last_projected_history_id
		FROM public.traces
		WHERE project_id = $1
		  AND engine_run_id = $2
		FOR UPDATE
	`, projectID, runID).Scan(&locked.TraceID, &projectionState, &lastProjectedHistoryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	locked.EngineProjectionState = pgTextPtr(projectionState)
	if lastProjectedHistoryID.Valid {
		value := lastProjectedHistoryID.Int64
		locked.EngineLastProjectedHistoryID = &value
	}

	run, err := enginedb.New(w.tx).GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	locked.Run = run
	return locked, nil
}

// BackfillTraceLineage refreshes the projected parent/root/child lineage
// columns on the trace shell from the engine's child-workflow linkage.
func (w *Writer) BackfillTraceLineage(
	ctx context.Context,
	traceID uuid.UUID,
	run *enginedb.EngineRun,
) error {
	if run == nil {
		return errors.New("run is required")
	}

	var (
		parentRunID any
		rootRunID   any
		childKey    any
		childDepth  int32
	)

	childWorkflow, err := enginedb.New(w.tx).GetChildWorkflowByChildInstanceForUpdate(ctx, enginedb.GetChildWorkflowByChildInstanceForUpdateParams{
		ProjectID:       run.ProjectID,
		ChildInstanceID: run.InstanceID,
	})
	switch {
	case err == nil:
		parentRunID = childWorkflow.ParentRunID
		rootRunID = childWorkflow.RootRunID
		childKey = childWorkflow.ChildKey
		childDepth = childWorkflow.ChildDepth
	case errors.Is(err, pgx.ErrNoRows):
		if run.ParentRunID.Valid {
			return fmt.Errorf("engine child workflow not found for child run %s", run.ID)
		}
		rootRunID = run.ID
		childDepth = 0
	default:
		return fmt.Errorf("load child workflow lineage: %w", err)
	}

	commandTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_parent_run_id = $2,
		    engine_root_run_id = $3,
		    engine_child_key = $4,
		    engine_child_depth = $5,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE id = $1
	`, traceID, parentRunID, rootRunID, childKey, childDepth)
	if err != nil {
		return fmt.Errorf("backfill engine trace lineage: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("backfill engine trace lineage matched zero rows for trace %s", traceID)
	}
	return nil
}

// EnsureTerminalShell forces the projected trace and root span of a terminal
// run into their terminal shape: lineage backfilled, summary counters zeroed,
// trace closed with terminal status/output, and the root span upserted.
func (w *Writer) EnsureTerminalShell(
	ctx context.Context,
	traceID uuid.UUID,
	run *enginedb.EngineRun,
) error {
	if run == nil {
		return errors.New("terminal shell requires run")
	}
	if err := w.BackfillTraceLineage(ctx, traceID, run); err != nil {
		return err
	}
	completedAt := terminalCompletedAt(run)
	traceStatus, spanStatus := TerminalStatuses(string(run.Status))
	outputPayload, err := TerminalOutputPayload(
		string(run.Status),
		run.Result,
		run.LastErrorCode,
		run.LastErrorMessage,
	)
	if err != nil {
		return err
	}

	summaryTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_run_status = $2,
		    engine_custom_status = $3,
		    engine_wait_state = $4,
		    engine_pending_activity_tasks = $5,
		    engine_pending_inbox_items = $6,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
	`, run.ID, stringPtr(string(run.Status)), cloneBytes(run.CustomStatus), cloneBytes(run.WaitingFor), int64(0), int64(0))
	if err != nil {
		return err
	}
	if summaryTag.RowsAffected() == 0 {
		return fmt.Errorf("update engine trace summary matched zero rows for run %s", run.ID)
	}

	traceTag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET status = $2,
		    end_time = $3::timestamptz,
		    output = $4::jsonb,
		    error_count = CASE
		        WHEN $2 IN ('failed', 'cancelled') THEN GREATEST(COALESCE(error_count, 0), 1)
		        ELSE error_count
		    END,
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE id = $1
	`, traceID, traceStatus, completedAt, outputPayload)
	if err != nil {
		return fmt.Errorf("ensure terminal trace shell: %w", err)
	}
	if traceTag.RowsAffected() == 0 {
		return fmt.Errorf("ensure terminal trace shell matched zero rows for trace %s", traceID)
	}

	spanID := RootSpanExternalID(run.ID)
	spanTag, err := w.tx.Exec(ctx, `
		INSERT INTO public.spans (
		    project_id,
		    trace_id,
		    span_id,
		    name,
		    type,
		    status,
		    status_message,
		    level,
		    start_time,
		    end_time,
		    output,
		    depth
		)
		SELECT
		    traces.project_id,
		    traces.id,
		    $2,
		    COALESCE(traces.name, 'Engine run'),
		    'chain',
		    $3,
		    $6::text,
		    'default',
		    COALESCE(traces.start_time, $4::timestamptz),
		    $4::timestamptz,
		    $5::jsonb,
		    0
		FROM public.traces AS traces
		WHERE traces.id = $1
		ON CONFLICT (trace_id, span_id) DO UPDATE
		SET status = EXCLUDED.status,
		    end_time = EXCLUDED.end_time,
		    output = EXCLUDED.output,
		    status_message = EXCLUDED.status_message,
		    duration_ms = CASE
		        WHEN EXCLUDED.end_time IS NOT NULL THEN EXTRACT(EPOCH FROM (EXCLUDED.end_time - public.spans.start_time)) * 1000
		        ELSE public.spans.duration_ms
		    END,
		    updated_at = NOW(),
		    version = COALESCE(public.spans.version, 1) + 1
	`, traceID, spanID, spanStatus, completedAt, outputPayload, run.LastErrorMessage)
	if err != nil {
		return fmt.Errorf("ensure terminal root span shell: %w", err)
	}
	if spanTag.RowsAffected() == 0 {
		return fmt.Errorf("ensure terminal root span shell found no trace %s for span %s", traceID, spanID)
	}
	return nil
}

// PurgeProjectionDetail deletes the projected span events and non-root spans
// of a trace, leaving only the summary shell (trace row plus root span).
func (w *Writer) PurgeProjectionDetail(ctx context.Context, traceID uuid.UUID) error {
	if _, err := w.tx.Exec(ctx, `
		DELETE FROM public.span_events
		WHERE trace_id = $1
	`, traceID); err != nil {
		return err
	}
	_, err := w.tx.Exec(ctx, `
		DELETE FROM public.spans
		WHERE trace_id = $1
		  AND parent_span_id IS NOT NULL
	`, traceID)
	return err
}

// MarkProjectionSummaryOnly flips the projection state to summary_only for
// traces still carrying projected detail. Returns the mutated row count.
func (w *Writer) MarkProjectionSummaryOnly(ctx context.Context, runID uuid.UUID) (int64, error) {
	tag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_projection_state = 'summary_only',
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
		  AND COALESCE(engine_projection_state, 'up_to_date') IN ('up_to_date', 'catching_up')
	`, runID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// MarkProjectionJournalExpired flips the projection state to journal_expired
// once the engine history journal has been deleted. Returns the mutated row
// count.
func (w *Writer) MarkProjectionJournalExpired(ctx context.Context, runID uuid.UUID) (int64, error) {
	tag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_projection_state = 'journal_expired',
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
		  AND COALESCE(engine_projection_state, 'up_to_date') IN ('up_to_date', 'catching_up', 'summary_only')
	`, runID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// MarkProjectionCatchingUp flips a summary_only trace back to catching_up so
// the projector re-projects retained history. Returns the mutated row count.
func (w *Writer) MarkProjectionCatchingUp(ctx context.Context, runID uuid.UUID) (int64, error) {
	tag, err := w.tx.Exec(ctx, `
		UPDATE public.traces
		SET engine_projection_state = 'catching_up',
		    engine_projection_updated_at = NOW(),
		    updated_at = NOW(),
		    version = COALESCE(version, 1) + 1
		WHERE engine_run_id = $1
		  AND COALESCE(engine_projection_state, 'up_to_date') = 'summary_only'
	`, runID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteRunJournal removes the engine-side history journal for a run as part
// of a full purge: activity-task history links, inbox history links, then the
// history rows themselves.
func (w *Writer) DeleteRunJournal(ctx context.Context, runID uuid.UUID) error {
	queries := enginedb.New(w.tx)
	if _, err := queries.ClearActivityTaskHistoryByRun(ctx, runID); err != nil {
		return err
	}
	if _, err := queries.ClearInboxHistoryByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true}); err != nil {
		return err
	}
	return queries.DeleteHistoryByRun(ctx, runID)
}

// LatestJournalHistoryID reports the newest history id retained for a run.
func (w *Writer) LatestJournalHistoryID(ctx context.Context, runID uuid.UUID) (int64, error) {
	return enginedb.New(w.tx).GetLatestHistoryIDByRun(ctx, runID)
}

func terminalCompletedAt(run *enginedb.EngineRun) time.Time {
	if run == nil {
		return time.Time{}
	}
	if run.CompletedAt.Valid {
		return run.CompletedAt.Time
	}
	return run.UpdatedAt
}
