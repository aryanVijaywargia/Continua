package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TraceFilter defines filter options for trace search.
type TraceFilter struct {
	Scope                   Scope      // Resolved read scope: Bound(projectID) or Unbounded
	Query                   string     // Full-text search query
	Status                  string     // running, completed, failed
	StartTimeFrom           *time.Time // Filter traces starting at or after this time
	StartTimeTo             *time.Time // Filter traces starting at or before this time
	UserID                  string     // Filter by user_id
	SessionID               *uuid.UUID // Filter by session_id
	EngineRunID             *uuid.UUID // Filter by engine_run_id
	EngineInstanceKey       string     // Filter by engine_instance_key
	EngineDefinitionName    string     // Filter by engine_definition_name
	EngineDefinitionVersion string     // Filter by engine_definition_version
	EngineRunStatus         string     // Filter by engine_run_status
	EngineParentRunID       *uuid.UUID // Filter by engine_parent_run_id
	EngineRootRunID         *uuid.UUID // Filter by engine_root_run_id
	EngineChildKey          string     // Filter by engine_child_key
	EngineChildDepth        *int32     // Filter by engine_child_depth
	EngineProjectionState   string     // Filter by engine_projection_state
	EngineOnly              bool       // Filter to engine-backed traces
	HasErrors               *bool      // Filter by error_count > 0
	MinDurationMs           *int64     // Filter by duration in milliseconds
	SortDir                 SortDirection
	Limit                   int32
	Offset                  int32
}

// TraceSearchResult contains the results of a trace search.
type TraceSearchResult struct {
	Traces []TraceRead
	Total  int64
}

type TraceFilterValidationError struct {
	Field string
	Value string
}

func (e *TraceFilterValidationError) Error() string {
	return fmt.Sprintf("invalid %s %q", e.Field, e.Value)
}

// ListTracesFiltered returns traces matching the filter criteria.
// Supports full-text search on trace name, user_id, and span names.
// Results are ordered by relevance when a search query is provided.
//
//nolint:gocritic // Pass-by-value keeps this immutable and avoids accidental caller mutation.
func (s *Store) ListTracesFiltered(ctx context.Context, filter TraceFilter) (TraceSearchResult, error) {
	result := TraceSearchResult{}
	if err := validateTraceFilter(&filter); err != nil {
		return result, err
	}

	// Build the base query
	var whereClauses []string
	var args []any
	argNum := 1
	fromClause := "FROM traces t LEFT JOIN sessions sess ON sess.id = t.session_id AND sess.project_id = t.project_id"

	// Enforce the resolved read scope in SQL: bound scopes see exactly one
	// project, unbounded (operator/admin) scopes see all projects.
	whereClauses = append(whereClauses, fmt.Sprintf("($%d::uuid IS NULL OR t.project_id = $%d::uuid)", argNum, argNum))
	args = append(args, filter.Scope.nullableProjectFilter())
	argNum++

	// Trim whitespace from query - per spec, whitespace-only queries should be ignored
	searchQuery := strings.TrimSpace(filter.Query)
	hasSearchQuery := searchQuery != ""
	searchArgNum := 0

	// Full-text search query - searches both trace fields and span names
	if hasSearchQuery {
		searchArgNum = argNum
		fromClause = fmt.Sprintf("%s CROSS JOIN (SELECT plainto_tsquery('english', $%d) AS search_query) q", fromClause, searchArgNum)
		// Search generated FTS vectors plus ID-style fields that operators paste
		// from related screens.
		// Use EXISTS for efficient span search with proper project scoping
		whereClauses = append(whereClauses, fmt.Sprintf(`(
			t.search_vector @@ q.search_query
			OR t.trace_id ILIKE '%%' || $%[1]d || '%%'
			OR COALESCE(t.name, '') ILIKE '%%' || $%[1]d || '%%'
			OR COALESCE(t.user_id, '') ILIKE '%%' || $%[1]d || '%%'
			OR COALESCE(sess.external_id, '') ILIKE '%%' || $%[1]d || '%%'
			OR EXISTS (
				SELECT 1 FROM spans s
				WHERE s.trace_id = t.id
				AND s.project_id = t.project_id
				AND (
					s.search_vector @@ q.search_query
					OR COALESCE(s.name, '') ILIKE '%%' || $%[1]d || '%%'
					OR s.span_id ILIKE '%%' || $%[1]d || '%%'
				)
			)
		)`, searchArgNum))
		args = append(args, searchQuery)
		argNum++
	}

	// Status filter (case-insensitive mapping)
	if filter.Status != "" {
		status := strings.ToLower(filter.Status)
		// Map API status values to database values
		switch status {
		case "running":
			whereClauses = append(whereClauses, fmt.Sprintf("t.status = $%d", argNum))
			args = append(args, "running")
			argNum++
		case "completed":
			// Include 'ok' as an alias for completed per spec
			whereClauses = append(whereClauses, "(t.status = 'completed' OR t.status = 'ok')")
		case "failed", "error":
			// Accept both "failed" and "error" to match FAILED status
			// Also include "cancelled" as a failed state per spec
			whereClauses = append(whereClauses, "(t.status = 'failed' OR t.status = 'error' OR t.status = 'cancelled')")
		}
	}

	// Time range filters
	// Use COALESCE(start_time, server_received_at) for time filtering
	if filter.StartTimeFrom != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("COALESCE(t.start_time, t.server_received_at) >= $%d", argNum))
		args = append(args, *filter.StartTimeFrom)
		argNum++
	}
	if filter.StartTimeTo != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("COALESCE(t.start_time, t.server_received_at) <= $%d", argNum))
		args = append(args, *filter.StartTimeTo)
		argNum++
	}

	// User ID filter
	if filter.UserID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.user_id = $%d", argNum))
		args = append(args, filter.UserID)
		argNum++
	}

	if filter.EngineInstanceKey != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_instance_key = $%d", argNum))
		args = append(args, filter.EngineInstanceKey)
		argNum++
	}

	if filter.EngineRunID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_run_id = $%d", argNum))
		args = append(args, *filter.EngineRunID)
		argNum++
	}

	if filter.EngineDefinitionName != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_definition_name = $%d", argNum))
		args = append(args, filter.EngineDefinitionName)
		argNum++
	}

	if filter.EngineDefinitionVersion != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_definition_version = $%d", argNum))
		args = append(args, filter.EngineDefinitionVersion)
		argNum++
	}

	if filter.EngineRunStatus != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_run_status = $%d", argNum))
		args = append(args, strings.ToLower(strings.TrimSpace(filter.EngineRunStatus)))
		argNum++
	}

	if filter.EngineParentRunID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_parent_run_id = $%d", argNum))
		args = append(args, *filter.EngineParentRunID)
		argNum++
	}

	if filter.EngineRootRunID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_root_run_id = $%d", argNum))
		args = append(args, *filter.EngineRootRunID)
		argNum++
	}

	if filter.EngineChildKey != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_child_key = $%d", argNum))
		args = append(args, filter.EngineChildKey)
		argNum++
	}

	if filter.EngineChildDepth != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_child_depth = $%d", argNum))
		args = append(args, *filter.EngineChildDepth)
		argNum++
	}

	if filter.EngineProjectionState != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("t.engine_projection_state = $%d", argNum))
		args = append(args, strings.TrimSpace(filter.EngineProjectionState))
		argNum++
	}

	if filter.EngineOnly {
		whereClauses = append(whereClauses, "t.engine_run_id IS NOT NULL")
	}

	// Session ID filter
	if filter.SessionID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("t.session_id = $%d", argNum))
		args = append(args, *filter.SessionID)
		argNum++
	}

	// Has errors filter
	if filter.HasErrors != nil && *filter.HasErrors {
		whereClauses = append(whereClauses, "t.error_count > 0")
	}

	// Duration filter
	// Duration = end_time - start_time (or now() - start_time for running traces)
	if filter.MinDurationMs != nil {
		whereClauses = append(whereClauses,
			fmt.Sprintf("EXTRACT(EPOCH FROM (COALESCE(t.end_time, now()) - COALESCE(t.start_time, t.server_received_at))) * 1000 >= $%d", argNum))
		args = append(args, *filter.MinDurationMs)
		argNum++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	// Count query - count distinct traces
	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT t.id) %s WHERE %s`, fromClause, whereClause)

	err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&result.Total)
	if err != nil {
		return result, fmt.Errorf("count traces: %w", err)
	}

	// Build ORDER BY clause
	// If search query is provided, order by relevance with time tie-breaker
	// Trace matches must rank higher than span-only matches.
	var orderByClause string
	var rankingSelectClause string
	const sortTimeSelectClause = ", COALESCE(t.start_time, t.server_received_at) AS sort_time"
	if hasSearchQuery {
		traceRankExpr := "ts_rank(t.search_vector, q.search_query)"
		spanRankExpr := `COALESCE((
				SELECT MAX(ts_rank(s.search_vector, q.search_query))
				FROM spans s
				WHERE s.trace_id = t.id AND s.project_id = t.project_id
			), 0)`

		rankingSelectClause = fmt.Sprintf(`,
			CASE WHEN %s > 0 THEN 1 ELSE 0 END AS trace_match_priority,
			GREATEST(%s, %s) AS combined_rank
		`, traceRankExpr, traceRankExpr, spanRankExpr)
		orderByClause = "trace_match_priority DESC, combined_rank DESC, sort_time DESC, t.id DESC"
	} else {
		sortDirectionSQL := "DESC"
		idDirectionSQL := "DESC"
		if normalizeSortDirection(filter.SortDir) == SortDirectionAsc {
			sortDirectionSQL = "ASC"
			idDirectionSQL = "ASC"
		}
		orderByClause = fmt.Sprintf("sort_time %s, t.id %s", sortDirectionSQL, idDirectionSQL)
	}

	// Main query with limit and offset
	limitArg := argNum
	offsetArg := argNum + 1

	selectQuery := fmt.Sprintf(`
		SELECT DISTINCT t.id, t.project_id, t.session_id, t.trace_id, t.name, t.user_id, t.tags, t.environment, t.release,
		       t.metadata, t.input, t.output, t.status, t.start_time, t.end_time, t.server_received_at, t.duration_ms, t.total_spans, t.total_cost,
		       t.error_count, t.version, t.created_at, t.updated_at, t.search_vector, t.total_tokens_in, t.total_tokens_out,
		       t.engine_run_id, t.engine_definition_name, t.engine_definition_version, t.engine_projection_state,
		       t.engine_latest_history_id, t.engine_last_projected_history_id, t.engine_projection_updated_at, t.engine_instance_key,
		       t.engine_run_status, t.engine_custom_status, t.engine_wait_state, t.engine_pending_activity_tasks, t.engine_pending_inbox_items,
		       t.engine_parent_run_id, t.engine_root_run_id, t.engine_child_key, t.engine_child_depth,
		       sess.external_id AS session_external_id%s%s
		%s
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, rankingSelectClause, sortTimeSelectClause, fromClause, whereClause, orderByClause, limitArg, offsetArg)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return result, fmt.Errorf("query traces: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var trace TraceRead
		var traceMatchPriority int
		var combinedRank float32
		var sortTime time.Time

		scanArgs := []any{
			&trace.ID, &trace.ProjectID, &trace.SessionID, &trace.TraceID, &trace.Name, &trace.UserID,
			&trace.Tags, &trace.Environment, &trace.Release, &trace.Metadata, &trace.Input, &trace.Output,
			&trace.Status, &trace.StartTime, &trace.EndTime, &trace.ServerReceivedAt, &trace.DurationMs,
			&trace.TotalSpans, &trace.TotalCost, &trace.ErrorCount, &trace.Version, &trace.CreatedAt,
			&trace.UpdatedAt, &trace.SearchVector, &trace.TotalTokensIn, &trace.TotalTokensOut,
			&trace.EngineRunID, &trace.EngineDefinitionName, &trace.EngineDefinitionVersion, &trace.EngineProjectionState,
			&trace.EngineLatestHistoryID, &trace.EngineLastProjectedHistoryID, &trace.EngineProjectionUpdatedAt, &trace.EngineInstanceKey,
			&trace.EngineRunStatus, &trace.EngineCustomStatus, &trace.EngineWaitState, &trace.EnginePendingActivityTasks, &trace.EnginePendingInboxItems,
			&trace.EngineParentRunID, &trace.EngineRootRunID, &trace.EngineChildKey, &trace.EngineChildDepth,
			&trace.SessionExternalID,
		}
		if hasSearchQuery {
			scanArgs = append(scanArgs, &traceMatchPriority, &combinedRank)
		}
		scanArgs = append(scanArgs, &sortTime)

		err := rows.Scan(scanArgs...)
		if err != nil {
			return result, fmt.Errorf("scan trace: %w", err)
		}
		result.Traces = append(result.Traces, trace)
	}

	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate traces: %w", err)
	}

	return result, nil
}

func validateTraceFilter(filter *TraceFilter) error {
	if filter == nil {
		return nil
	}
	if filter.EngineRunStatus != "" {
		value := strings.ToLower(strings.TrimSpace(filter.EngineRunStatus))
		switch value {
		case "queued", "running", "waiting", "suspended", "quarantined", "completed", "failed", "cancelled", "terminated", "continued_as_new":
		default:
			return &TraceFilterValidationError{Field: "engine_run_status", Value: filter.EngineRunStatus}
		}
	}

	if filter.EngineProjectionState != "" {
		value := strings.TrimSpace(filter.EngineProjectionState)
		switch value {
		case "up_to_date", "catching_up", "summary_only", "journal_expired":
		default:
			return &TraceFilterValidationError{Field: "engine_projection_state", Value: filter.EngineProjectionState}
		}
	}

	return nil
}
