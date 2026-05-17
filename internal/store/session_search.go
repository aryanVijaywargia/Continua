package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// SessionSortBy controls session list sorting.
type SessionSortBy string

const (
	SessionSortByCreatedAt  SessionSortBy = "created_at"
	SessionSortByTraceCount SessionSortBy = "trace_count"
)

// SessionFilter defines filter and sorting options for session listing.
type SessionFilter struct {
	ProjectID uuid.UUID
	Query     string
	UserID    string
	SortBy    SessionSortBy
	SortDir   SortDirection
	Limit     int32
	Offset    int32
}

// SessionSearchResult contains session rows and the filtered total.
type SessionSearchResult struct {
	Sessions []SessionWithCount
	Total    int64
}

type sessionQueryPlan struct {
	whereClause  string
	countArgs    []any
	listArgs     []any
	hasSearch    bool
	exactArgNum  int
	prefixArgNum int
}

func normalizeSessionSortBy(sortBy SessionSortBy) SessionSortBy {
	if sortBy == SessionSortByTraceCount {
		return SessionSortByTraceCount
	}
	return SessionSortByCreatedAt
}

func buildSessionQueryPlan(filter *SessionFilter) sessionQueryPlan {
	projectIDArg := []any{filter.ProjectID}
	whereClauses := []string{"s.project_id = $1"}
	nextArgNum := 2

	trimmedQuery := strings.TrimSpace(filter.Query)
	hasSearch := trimmedQuery != ""
	if hasSearch {
		whereClauses = append(whereClauses,
			fmt.Sprintf("(s.id::text ILIKE $%d OR s.external_id ILIKE $%d OR COALESCE(s.name, '') ILIKE $%d OR COALESCE(s.user_id, '') ILIKE $%d)", nextArgNum, nextArgNum, nextArgNum, nextArgNum))
		projectIDArg = append(projectIDArg, "%"+trimmedQuery+"%")
		nextArgNum++
	}

	if filter.UserID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.user_id = $%d", nextArgNum))
		projectIDArg = append(projectIDArg, filter.UserID)
	}

	listArgs := append([]any{}, projectIDArg...)
	plan := sessionQueryPlan{
		whereClause: strings.Join(whereClauses, " AND "),
		countArgs:   projectIDArg,
		listArgs:    listArgs,
		hasSearch:   hasSearch,
	}

	if hasSearch {
		plan.exactArgNum = len(plan.listArgs) + 1
		plan.listArgs = append(plan.listArgs, trimmedQuery)
		plan.prefixArgNum = len(plan.listArgs) + 1
		plan.listArgs = append(plan.listArgs, trimmedQuery+"%")
	}

	return plan
}

func buildSessionOrderBy(filter *SessionFilter, plan *sessionQueryPlan) string {
	if plan.hasSearch {
		return fmt.Sprintf(`CASE
			WHEN LOWER(s.id::text) = LOWER($%d) THEN 0
			WHEN LOWER(s.external_id) = LOWER($%d) THEN 1
			WHEN LOWER(COALESCE(s.user_id, '')) = LOWER($%d) THEN 2
			WHEN s.id::text ILIKE $%d THEN 3
			WHEN s.external_id ILIKE $%d THEN 4
			WHEN COALESCE(s.user_id, '') ILIKE $%d THEN 5
			ELSE 6
		END ASC, s.created_at DESC, s.id DESC`, plan.exactArgNum, plan.exactArgNum, plan.exactArgNum, plan.prefixArgNum, plan.prefixArgNum, plan.prefixArgNum)
	}

	sortBy := normalizeSessionSortBy(filter.SortBy)
	sortDir := normalizeSortDirection(filter.SortDir)

	switch sortBy {
	case SessionSortByTraceCount:
		if sortDir == SortDirectionAsc {
			return "COALESCE(tc.cnt, 0) ASC, s.created_at DESC, s.id DESC"
		}
		return "COALESCE(tc.cnt, 0) DESC, s.created_at DESC, s.id DESC"
	default:
		if sortDir == SortDirectionAsc {
			return "s.created_at ASC, s.id ASC"
		}
		return "s.created_at DESC, s.id DESC"
	}
}

// ListSessionsFiltered returns sessions with dynamic filters, sorting, and accurate totals.
//
//nolint:gocritic // Pass-by-value keeps this immutable and avoids accidental caller mutation.
func (s *Store) ListSessionsFiltered(ctx context.Context, filter SessionFilter) (SessionSearchResult, error) {
	result := SessionSearchResult{}
	plan := buildSessionQueryPlan(&filter)

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM sessions s WHERE %s`, plan.whereClause)
	if err := s.pool.QueryRow(ctx, countQuery, plan.countArgs...).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count sessions: %w", err)
	}

	orderByClause := buildSessionOrderBy(&filter, &plan)
	limitArgNum := len(plan.listArgs) + 1
	offsetArgNum := len(plan.listArgs) + 2

	listQuery := fmt.Sprintf(`
		SELECT s.id, s.project_id, s.name, s.user_id, s.metadata, s.created_at, s.updated_at, s.external_id,
		       COALESCE(tc.cnt, 0) AS trace_count
		FROM sessions s
		LEFT JOIN (
			SELECT session_id, COUNT(*) AS cnt
			FROM traces
			WHERE project_id = $1
			GROUP BY session_id
		) tc ON tc.session_id = s.id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, plan.whereClause, orderByClause, limitArgNum, offsetArgNum)

	args := append(plan.listArgs, filter.Limit, filter.Offset)
	rows, err := s.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return result, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var session SessionWithCount
		if err := rows.Scan(
			&session.ID,
			&session.ProjectID,
			&session.Name,
			&session.UserID,
			&session.Metadata,
			&session.CreatedAt,
			&session.UpdatedAt,
			&session.ExternalID,
			&session.TraceCount,
		); err != nil {
			return result, fmt.Errorf("scan session: %w", err)
		}
		result.Sessions = append(result.Sessions, session)
	}

	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate sessions: %w", err)
	}

	return result, nil
}
