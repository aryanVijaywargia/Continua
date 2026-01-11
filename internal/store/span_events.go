package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// InsertSpanEvent inserts a new span event.
// Returns the event ID if successful, or uuid.Nil if the event was a duplicate (idempotent).
func (s *Store) InsertSpanEvent(ctx context.Context, params *platform.InsertSpanEventParams) (uuid.UUID, error) {
	id, err := s.q.InsertSpanEvent(ctx, *params)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING - this is an idempotent duplicate
		return uuid.Nil, nil
	}
	return id, err
}

// InsertSpanEventTx inserts a span event within a transaction.
func (t *Tx) InsertSpanEvent(ctx context.Context, params *platform.InsertSpanEventParams) (uuid.UUID, error) {
	id, err := t.q.InsertSpanEvent(ctx, *params)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return id, err
}

// GetSpanEvent retrieves a span event by its internal UUID.
func (s *Store) GetSpanEvent(ctx context.Context, id uuid.UUID) (platform.SpanEvent, error) {
	event, err := s.q.GetSpanEvent(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.SpanEvent{}, ErrNotFound
	}
	return event, err
}

// ListSpanEventsBySpan returns all events for a specific span.
func (s *Store) ListSpanEventsBySpan(ctx context.Context, traceUUID uuid.UUID, spanID string) ([]platform.SpanEvent, error) {
	return s.q.ListSpanEventsBySpan(ctx, platform.ListSpanEventsBySpanParams{
		TraceID: traceUUID,
		SpanID:  spanID,
	})
}

// ListSpanEventsByTrace returns all events for a trace.
func (s *Store) ListSpanEventsByTrace(ctx context.Context, traceUUID uuid.UUID) ([]platform.SpanEvent, error) {
	return s.q.ListSpanEventsByTrace(ctx, traceUUID)
}

// CountOrphanEvents returns the count of events whose span doesn't exist yet.
func (s *Store) CountOrphanEvents(ctx context.Context, traceUUID uuid.UUID) (int64, error) {
	return s.q.CountOrphanEvents(ctx, traceUUID)
}

// ListOrphanEvents returns events whose span doesn't exist yet.
func (s *Store) ListOrphanEvents(ctx context.Context, traceUUID uuid.UUID) ([]platform.SpanEvent, error) {
	return s.q.ListOrphanEvents(ctx, traceUUID)
}
