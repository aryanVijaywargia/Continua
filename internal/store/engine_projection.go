package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

type EngineRetentionCandidate struct {
	ProjectID       uuid.UUID
	RunID           uuid.UUID
	TraceID         uuid.UUID
	ProjectionState string
	CompletedAt     time.Time
}

func (s *Store) ListProjectionRetentionCandidates(
	ctx context.Context,
	before time.Time,
	limit int,
) ([]EngineRetentionCandidate, error) {
	return s.listEngineRetentionCandidates(ctx, before, limit, []string{"up_to_date", "catching_up"})
}

func (s *Store) ListHistoryRetentionCandidates(
	ctx context.Context,
	before time.Time,
	limit int,
) ([]EngineRetentionCandidate, error) {
	return s.listEngineRetentionCandidates(ctx, before, limit, []string{"summary_only", "up_to_date", "catching_up"})
}

func (s *Store) listEngineRetentionCandidates(
	ctx context.Context,
	before time.Time,
	limit int,
	states []string,
) ([]EngineRetentionCandidate, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT r.project_id,
		       r.id,
		       t.id,
		       t.engine_projection_state,
		       r.completed_at
		FROM engine.runs AS r
		INNER JOIN public.traces AS t
		    ON t.project_id = r.project_id
		   AND t.engine_run_id = r.id
		WHERE r.status IN ('completed', 'failed', 'cancelled', 'terminated', 'continued_as_new')
		  AND r.completed_at IS NOT NULL
		  AND r.completed_at < $1
		  AND t.engine_projection_state = ANY($2::text[])
		ORDER BY r.completed_at ASC, r.id ASC
		LIMIT $3
	`, before, states, limit)
	if err != nil {
		return nil, fmt.Errorf("list engine retention candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]EngineRetentionCandidate, 0, limit)
	for rows.Next() {
		var candidate EngineRetentionCandidate
		if err := rows.Scan(
			&candidate.ProjectID,
			&candidate.RunID,
			&candidate.TraceID,
			&candidate.ProjectionState,
			&candidate.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan engine retention candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate engine retention candidates: %w", err)
	}

	return candidates, nil
}

func (t *Tx) GetTraceByProjectAndEngineRunIDForUpdate(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (platform.Trace, error) {
	trace, err := t.q.GetTraceByProjectAndEngineRunIDForUpdate(ctx, platform.GetTraceByProjectAndEngineRunIDForUpdateParams{
		ProjectID:   projectID,
		EngineRunID: pgUUID(runID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Trace{}, ErrNotFound
	}
	return trace, err
}

func (t *Tx) DeleteSpanEventsByTrace(ctx context.Context, traceID uuid.UUID) error {
	return t.q.DeleteSpanEventsByTrace(ctx, traceID)
}

func (t *Tx) DeleteNonRootSpansByTrace(ctx context.Context, traceID uuid.UUID) error {
	return t.q.DeleteNonRootSpansByTrace(ctx, traceID)
}

func (t *Tx) FlipProjectionStateToSummaryOnly(ctx context.Context, runID uuid.UUID) (int64, error) {
	return t.q.FlipProjectionStateToSummaryOnly(ctx, pgUUID(runID))
}

func (t *Tx) FlipProjectionStateToJournalExpired(ctx context.Context, runID uuid.UUID) (int64, error) {
	return t.q.FlipProjectionStateToJournalExpired(ctx, pgUUID(runID))
}

func (t *Tx) FlipProjectionStateToCatchingUp(ctx context.Context, runID uuid.UUID) (int64, error) {
	return t.q.FlipProjectionStateToCatchingUp(ctx, pgUUID(runID))
}

func (t *Tx) BackfillEngineTraceLineage(ctx context.Context, traceID uuid.UUID, run *enginedb.EngineRun) error {
	if t == nil || run == nil {
		return errors.New("transaction and run are required")
	}

	var (
		parentRunID any
		rootRunID   any
		childKey    any
		childDepth  int32
	)

	childWorkflow, err := enginedb.New(t.Tx()).GetChildWorkflowByChildInstanceForUpdate(ctx, enginedb.GetChildWorkflowByChildInstanceForUpdateParams{
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

	commandTag, err := t.tx.Exec(ctx, `
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

func (t *Tx) EnsureTerminalTraceShell(
	ctx context.Context,
	traceID uuid.UUID,
	status string,
	completedAt time.Time,
	output json.RawMessage,
) error {
	commandTag, err := t.tx.Exec(ctx, `
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
	`, traceID, status, completedAt, output)
	if err != nil {
		return fmt.Errorf("ensure terminal trace shell: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("ensure terminal trace shell matched zero rows for trace %s", traceID)
	}
	return nil
}

func (t *Tx) EnsureTerminalRootSpanShell(
	ctx context.Context,
	traceID uuid.UUID,
	spanID string,
	status string,
	completedAt time.Time,
	output json.RawMessage,
	statusMessage *string,
) error {
	commandTag, err := t.tx.Exec(ctx, `
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
		WHERE trace_id = $1
		  AND span_id = $2
	`, traceID, spanID, status, completedAt, output, statusMessage)
	if err != nil {
		return fmt.Errorf("ensure terminal root span shell: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("ensure terminal root span shell matched zero rows for trace %s span %s", traceID, spanID)
	}
	return nil
}
