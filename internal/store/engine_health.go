package store

import (
	"context"
	"fmt"
	"time"
)

type EngineHealthSnapshot struct {
	ProjectorLagRows     int64
	RunsCatchingUp       int64
	RunsReady            int64
	ActivityTasksPending int64
	InboxPending         int64
	Workers              []EngineWorkerHealth
	SummaryOnlyRuns      int64
	JournalExpiredRuns   int64
}

type EngineWorkerHealth struct {
	ID            string
	LastClaimAt   time.Time
	ActiveLeases  int
	ExpiredLeases int
	Status        string
}

func (s *Store) GetEngineHealth(ctx context.Context, scope Scope) (EngineHealthSnapshot, error) {
	var snapshot EngineHealthSnapshot
	projectFilter := scope.nullableProjectFilter()

	err := s.pool.QueryRow(ctx, `
		WITH engine_traces AS (
		    SELECT t.engine_run_id,
		           t.engine_projection_state,
		           t.engine_latest_history_id,
		           t.engine_last_projected_history_id
		    FROM public.traces AS t
		    WHERE t.engine_run_id IS NOT NULL
		      AND ($1::uuid IS NULL OR t.project_id = $1)
		), trace_lag AS (
		    SELECT GREATEST(
		               GREATEST(
		                   COALESCE((
		                       SELECT MAX(h.id)
		                       FROM engine.history AS h
		                       WHERE h.run_id = t.engine_run_id
		                   ), 0),
		                   COALESCE(t.engine_latest_history_id, 0)
		               ) - COALESCE(t.engine_last_projected_history_id, 0),
		               0
		           ) AS lag_rows
		    FROM engine_traces AS t
		    WHERE COALESCE(t.engine_projection_state, '') NOT IN ('summary_only', 'journal_expired')
		)
		SELECT (
		           SELECT COUNT(*)
		           FROM engine.runs AS r
		           WHERE ($1::uuid IS NULL OR r.project_id = $1)
		             AND ((r.status = 'queued' AND r.ready_at <= NOW())
		               OR (r.status = 'running' AND r.lease_expires_at IS NOT NULL AND r.lease_expires_at < NOW()))
		       ),
		       (
		           SELECT COUNT(*)
		           FROM engine.activity_tasks AS task
		           WHERE ($1::uuid IS NULL OR task.project_id = $1)
		             AND task.execution_target = 'local'
		             AND ((task.status = 'queued' AND task.available_at <= NOW())
		               OR (task.status = 'claimed' AND task.lease_expires_at IS NOT NULL AND task.lease_expires_at < NOW()))
		       ),
		       (
		           SELECT COUNT(*)
		           FROM engine.inbox AS inbox
		           WHERE ($1::uuid IS NULL OR inbox.project_id = $1)
		             AND inbox.status = 'pending'
		             AND inbox.available_at <= NOW()
		       ),
		       COALESCE((SELECT SUM(lag_rows) FROM trace_lag), 0),
		       (SELECT COUNT(*) FROM engine_traces WHERE engine_projection_state = 'catching_up'),
		       (SELECT COUNT(*) FROM engine_traces WHERE engine_projection_state = 'summary_only'),
		       (SELECT COUNT(*) FROM engine_traces WHERE engine_projection_state = 'journal_expired')
	`, projectFilter).Scan(
		&snapshot.RunsReady,
		&snapshot.ActivityTasksPending,
		&snapshot.InboxPending,
		&snapshot.ProjectorLagRows,
		&snapshot.RunsCatchingUp,
		&snapshot.SummaryOnlyRuns,
		&snapshot.JournalExpiredRuns,
	)
	if err != nil {
		return EngineHealthSnapshot{}, fmt.Errorf("get engine health metrics: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		WITH worker_leases AS (
		    SELECT r.claimed_by AS id, r.claimed_at, r.lease_expires_at
		    FROM engine.runs AS r
		    WHERE r.claimed_by IS NOT NULL
		      AND ($1::uuid IS NULL OR r.project_id = $1)
		    UNION ALL
		    SELECT task.claimed_by AS id, task.claimed_at, task.lease_expires_at
		    FROM engine.activity_tasks AS task
		    WHERE task.claimed_by IS NOT NULL
		      AND ($1::uuid IS NULL OR task.project_id = $1)
		)
		SELECT id,
		       MAX(claimed_at),
		       COUNT(*) FILTER (WHERE lease_expires_at > NOW()),
		       COUNT(*) FILTER (WHERE lease_expires_at IS NOT NULL AND lease_expires_at <= NOW()),
		       CASE
		           WHEN COUNT(*) FILTER (WHERE lease_expires_at > NOW()) > 0 THEN 'active'
		           ELSE 'stale'
		       END
		FROM worker_leases
		GROUP BY id
		ORDER BY id
	`, projectFilter)
	if err != nil {
		return EngineHealthSnapshot{}, fmt.Errorf("list engine worker health: %w", err)
	}
	defer rows.Close()

	snapshot.Workers = make([]EngineWorkerHealth, 0)
	for rows.Next() {
		var worker EngineWorkerHealth
		if err := rows.Scan(
			&worker.ID,
			&worker.LastClaimAt,
			&worker.ActiveLeases,
			&worker.ExpiredLeases,
			&worker.Status,
		); err != nil {
			return EngineHealthSnapshot{}, fmt.Errorf("scan engine worker health: %w", err)
		}
		snapshot.Workers = append(snapshot.Workers, worker)
	}
	if err := rows.Err(); err != nil {
		return EngineHealthSnapshot{}, fmt.Errorf("iterate engine worker health: %w", err)
	}

	return snapshot, nil
}
