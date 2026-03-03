package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// TraceFilter defines filter options for trace search.
type TraceFilter struct {
	ProjectID     uuid.UUID
	Query         string     // Full-text search query
	Status        string     // running, completed, failed
	StartTimeFrom *time.Time // Filter traces starting at or after this time
	StartTimeTo   *time.Time // Filter traces starting at or before this time
	UserID        string     // Filter by user_id
	SessionID     *uuid.UUID // Filter by session_id
	HasErrors     *bool      // Filter by error_count > 0
	MinDurationMs *int64     // Filter by duration in milliseconds
	Limit         int32
	Offset        int32
}

// TraceWithTotal represents a trace with the total count for pagination.
type TraceWithTotal struct {
	platform.Trace
	Total int64
}

// TraceSearchResult contains the results of a trace search.
type TraceSearchResult struct {
	Traces []platform.Trace
	Total  int64
}

// ListTracesFiltered returns traces matching the filter criteria.
// Supports full-text search on trace name, user_id, and span names.
// Results are ordered by relevance when a search query is provided.
//
//nolint:gocritic // Pass-by-value keeps this immutable and avoids accidental caller mutation.
func (s *Store) ListTracesFiltered(ctx context.Context, filter TraceFilter) (TraceSearchResult, error) {
	result := TraceSearchResult{}

	// Build the base query
	var whereClauses []string
	var args []any
	argNum := 1
	fromClause := "FROM traces t"

	// Always filter by project
	whereClauses = append(whereClauses, fmt.Sprintf("t.project_id = $%d", argNum))
	args = append(args, filter.ProjectID)
	argNum++

	// Trim whitespace from query - per spec, whitespace-only queries should be ignored
	searchQuery := strings.TrimSpace(filter.Query)
	hasSearchQuery := searchQuery != ""
	searchArgNum := 0

	// Full-text search query - searches both trace fields and span names
	if hasSearchQuery {
		searchArgNum = argNum
		fromClause = fmt.Sprintf("FROM traces t CROSS JOIN (SELECT plainto_tsquery('english', $%d) AS search_query) q", searchArgNum)
		// Search traces.search_vector OR any span.search_vector matches
		// Use EXISTS for efficient span search with proper project scoping
		whereClauses = append(whereClauses, `(
			t.search_vector @@ q.search_query
			OR EXISTS (
				SELECT 1 FROM spans s
				WHERE s.trace_id = t.id
				AND s.project_id = t.project_id
				AND s.search_vector @@ q.search_query
			)
		)`)
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
		orderByClause = "trace_match_priority DESC, combined_rank DESC, sort_time DESC"
	} else {
		orderByClause = "sort_time DESC"
	}

	// Main query with limit and offset
	limitArg := argNum
	offsetArg := argNum + 1

	selectQuery := fmt.Sprintf(`
		SELECT DISTINCT t.id, t.project_id, t.session_id, t.trace_id, t.name, t.status, t.user_id, t.tags, t.environment, t.release,
		       t.metadata, t.start_time, t.end_time, t.server_received_at, t.total_spans, t.total_tokens_in, t.total_tokens_out, t.total_cost,
		       t.error_count, t.created_at, t.updated_at%s%s
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
		var t platform.Trace
		var traceMatchPriority int
		var combinedRank float32
		var sortTime time.Time

		scanArgs := []any{
			&t.ID, &t.ProjectID, &t.SessionID, &t.TraceID, &t.Name, &t.Status, &t.UserID,
			&t.Tags, &t.Environment, &t.Release, &t.Metadata, &t.StartTime, &t.EndTime,
			&t.ServerReceivedAt, &t.TotalSpans, &t.TotalTokensIn, &t.TotalTokensOut, &t.TotalCost, &t.ErrorCount,
			&t.CreatedAt, &t.UpdatedAt,
		}
		if hasSearchQuery {
			scanArgs = append(scanArgs, &traceMatchPriority, &combinedRank)
		}
		scanArgs = append(scanArgs, &sortTime)

		err := rows.Scan(scanArgs...)
		if err != nil {
			return result, fmt.Errorf("scan trace: %w", err)
		}
		result.Traces = append(result.Traces, t)
	}

	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate traces: %w", err)
	}

	return result, nil
}
