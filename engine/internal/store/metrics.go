package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// RuntimeMetricsSnapshot contains the current engine queue and projector state.
type RuntimeMetricsSnapshot struct {
	RunsReady            int64
	ActivityTasksPending int64
	InboxPending         int64
	ProjectorLagRows     int64
}

// SampleRuntimeMetrics returns a project-scoped snapshot of claimable work and
// projector lag.
func (s *Store) SampleRuntimeMetrics(ctx context.Context) (RuntimeMetricsSnapshot, error) {
	var snapshot RuntimeMetricsSnapshot
	projectFilter := pgtype.UUID{}
	if s.projectFilter != nil {
		projectFilter = pgtype.UUID{Bytes: *s.projectFilter, Valid: true}
	}

	err := s.pool.QueryRow(ctx, `
		WITH trace_lag AS (
		    SELECT GREATEST(
		               COALESCE((
		                   SELECT MAX(h.id)
		                   FROM engine.history AS h
		                   WHERE h.run_id = t.engine_run_id
		               ), COALESCE(t.engine_latest_history_id, 0), 0)
		               - COALESCE(t.engine_last_projected_history_id, 0),
		               0
		           ) AS lag_rows
		    FROM public.traces AS t
		    WHERE t.engine_run_id IS NOT NULL
		      AND ($1::uuid IS NULL OR t.project_id = $1)
		      AND COALESCE(t.engine_projection_state, '') NOT IN ('summary_only', 'journal_expired')
		)
		SELECT (
		           SELECT COUNT(*)
		           FROM engine.runs AS r
		           WHERE ($1::uuid IS NULL OR r.project_id = $1)
		             AND ((r.status = 'queued' AND r.ready_at <= NOW())
		               OR (r.status = 'running' AND r.lease_expires_at IS NOT NULL AND r.lease_expires_at < NOW()))
		       ) AS runs_ready,
		       (
		           SELECT COUNT(*)
		           FROM engine.activity_tasks AS task
		           WHERE ($1::uuid IS NULL OR task.project_id = $1)
		             AND task.execution_target = 'local'
		             AND ((task.status = 'queued' AND task.available_at <= NOW())
		               OR (task.status = 'claimed' AND task.lease_expires_at IS NOT NULL AND task.lease_expires_at < NOW()))
		       ) AS activity_tasks_pending,
		       (
		           SELECT COUNT(*)
		           FROM engine.inbox AS inbox
		           WHERE ($1::uuid IS NULL OR inbox.project_id = $1)
		             AND inbox.status = 'pending'
		             AND inbox.available_at <= NOW()
		       ) AS inbox_pending,
		       COALESCE((SELECT SUM(lag_rows) FROM trace_lag), 0) AS projector_lag_rows
	`, projectFilter).Scan(
		&snapshot.RunsReady,
		&snapshot.ActivityTasksPending,
		&snapshot.InboxPending,
		&snapshot.ProjectorLagRows,
	)
	if err != nil {
		return RuntimeMetricsSnapshot{}, err
	}
	return snapshot, nil
}
