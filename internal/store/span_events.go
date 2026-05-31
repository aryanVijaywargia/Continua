package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// InsertSpanEventTx inserts a span event within a transaction.
func (t *Tx) InsertSpanEvent(ctx context.Context, params *platform.InsertSpanEventParams) (uuid.UUID, error) {
	id, err := t.q.InsertSpanEvent(ctx, *params)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return id, err
}

// ListSpanEventsByTrace returns all events for a trace within the supplied scope.
func (s *Store) ListSpanEventsByTrace(ctx context.Context, scope Scope, traceUUID uuid.UUID) ([]platform.SpanEvent, error) {
	return s.q.ListSpanEventsByTrace(ctx, platform.ListSpanEventsByTraceParams{
		TraceID:         traceUUID,
		ProjectFilterID: scope.nullableProjectFilter(),
	})
}
