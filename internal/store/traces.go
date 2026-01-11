package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// CreateTrace creates a new trace record.
func (s *Store) CreateTrace(ctx context.Context, params *platform.CreateTraceParams) (platform.Trace, error) {
	return s.q.CreateTrace(ctx, *params)
}

// CreateTraceTx creates a new trace within a transaction.
func (t *Tx) CreateTrace(ctx context.Context, params *platform.CreateTraceParams) (platform.Trace, error) {
	return t.q.CreateTrace(ctx, *params)
}

// GetTrace retrieves a trace by its internal UUID.
func (s *Store) GetTrace(ctx context.Context, id uuid.UUID) (platform.Trace, error) {
	trace, err := s.q.GetTrace(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Trace{}, ErrNotFound
	}
	return trace, err
}

// GetTraceByExternalID retrieves a trace by project ID and external trace_id.
func (s *Store) GetTraceByExternalID(ctx context.Context, projectID uuid.UUID, traceID string) (platform.Trace, error) {
	trace, err := s.q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Trace{}, ErrNotFound
	}
	return trace, err
}

// GetTraceByExternalIDTx retrieves a trace by external ID within a transaction.
func (t *Tx) GetTraceByExternalID(ctx context.Context, projectID uuid.UUID, traceID string) (platform.Trace, error) {
	trace, err := t.q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Trace{}, ErrNotFound
	}
	return trace, err
}

// GetTraceUUID retrieves just the internal UUID for a trace by external ID.
// This is useful for mapping external trace IDs to internal UUIDs during ingestion.
func (s *Store) GetTraceUUID(ctx context.Context, projectID uuid.UUID, traceID string) (uuid.UUID, error) {
	id, err := s.q.GetTraceUUID(ctx, platform.GetTraceUUIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

// GetTraceUUIDTx retrieves the internal UUID within a transaction.
func (t *Tx) GetTraceUUID(ctx context.Context, projectID uuid.UUID, traceID string) (uuid.UUID, error) {
	id, err := t.q.GetTraceUUID(ctx, platform.GetTraceUUIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

// ListTraces returns paginated traces for a project.
func (s *Store) ListTraces(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]platform.Trace, error) {
	return s.q.ListTraces(ctx, platform.ListTracesParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
}

// ListTracesBySession returns traces for a specific session.
func (s *Store) ListTracesBySession(ctx context.Context, params platform.ListTracesBySessionParams) ([]platform.Trace, error) {
	return s.q.ListTracesBySession(ctx, params)
}

// CountTraces returns the total number of traces for a project.
func (s *Store) CountTraces(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return s.q.CountTraces(ctx, projectID)
}

// UpdateTraceStatus updates a trace's status and end time.
func (s *Store) UpdateTraceStatus(ctx context.Context, params platform.UpdateTraceStatusParams) (platform.Trace, error) {
	return s.q.UpdateTraceStatus(ctx, params)
}

// UpdateTraceStatusTx updates trace status within a transaction.
func (t *Tx) UpdateTraceStatus(ctx context.Context, params platform.UpdateTraceStatusParams) (platform.Trace, error) {
	return t.q.UpdateTraceStatus(ctx, params)
}

// UpdateTraceRollups updates aggregated metrics on a trace.
func (s *Store) UpdateTraceRollups(ctx context.Context, params platform.UpdateTraceRollupsParams) error {
	return s.q.UpdateTraceRollups(ctx, params)
}

// UpdateTraceRollupsTx updates trace rollups within a transaction.
func (t *Tx) UpdateTraceRollups(ctx context.Context, params platform.UpdateTraceRollupsParams) error {
	return t.q.UpdateTraceRollups(ctx, params)
}

// UpsertTrace upserts a trace with patch semantics.
// NULL values don't overwrite existing values, and error status is never downgraded.
func (s *Store) UpsertTrace(ctx context.Context, params *platform.UpsertTraceParams) (platform.Trace, error) {
	return s.q.UpsertTrace(ctx, *params)
}

// UpsertTraceTx upserts a trace within a transaction.
func (t *Tx) UpsertTrace(ctx context.Context, params *platform.UpsertTraceParams) (platform.Trace, error) {
	return t.q.UpsertTrace(ctx, *params)
}
