package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ProjectionBackfillFilter struct {
	ProjectID             uuid.UUID
	OlderThan             *time.Time
	EngineInstanceKey     string
	EngineDefinitionName  string
	EngineRunStatus       string
	EngineProjectionState string
	Limit                 int
}

type ProjectionBackfillCandidate struct {
	RunID           uuid.UUID
	TraceID         string
	ProjectionState string
}

func (s *Store) ListProjectionBackfillCandidates(
	ctx context.Context,
	filter *ProjectionBackfillFilter,
) ([]ProjectionBackfillCandidate, error) {
	if filter == nil {
		filter = &ProjectionBackfillFilter{}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	whereClauses := []string{
		"t.project_id = $1",
		"t.engine_run_id IS NOT NULL",
		"COALESCE(t.engine_projection_state, 'up_to_date') = $2",
		"hist.latest_retained_history_id > COALESCE(t.engine_last_projected_history_id, 0)",
	}
	args := []any{
		filter.ProjectID,
		strings.TrimSpace(filter.EngineProjectionState),
	}
	argNum := 3

	if filter.OlderThan != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_projection_updated_at < $%d", argNum))
		args = append(args, *filter.OlderThan)
		argNum++
	}

	if instanceKey := strings.TrimSpace(filter.EngineInstanceKey); instanceKey != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_instance_key = $%d", argNum))
		args = append(args, instanceKey)
		argNum++
	}

	if definitionName := strings.TrimSpace(filter.EngineDefinitionName); definitionName != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_definition_name = $%d", argNum))
		args = append(args, definitionName)
		argNum++
	}

	if runStatus := strings.ToLower(strings.TrimSpace(filter.EngineRunStatus)); runStatus != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(t.engine_run_status) = $%d", argNum))
		args = append(args, runStatus)
		argNum++
	}

	args = append(args, limit)

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT t.engine_run_id,
		       t.trace_id,
		       COALESCE(t.engine_projection_state, 'up_to_date') AS projection_state
		FROM public.traces AS t
		INNER JOIN engine.runs AS r
		    ON r.project_id = t.project_id
		   AND r.id = t.engine_run_id
		INNER JOIN LATERAL (
		    SELECT COALESCE(MAX(h.id), 0)::bigint AS latest_retained_history_id
		    FROM engine.history AS h
		    WHERE h.run_id = r.id
		) AS hist ON true
		WHERE %s
		ORDER BY COALESCE(t.engine_projection_updated_at, t.updated_at, t.created_at) ASC, t.id ASC
		LIMIT $%d
	`, strings.Join(whereClauses, " AND "), argNum), args...)
	if err != nil {
		return nil, fmt.Errorf("list projection backfill candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]ProjectionBackfillCandidate, 0, limit)
	for rows.Next() {
		var candidate ProjectionBackfillCandidate
		if err := rows.Scan(&candidate.RunID, &candidate.TraceID, &candidate.ProjectionState); err != nil {
			return nil, fmt.Errorf("scan projection backfill candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projection backfill candidates: %w", err)
	}

	return candidates, nil
}
