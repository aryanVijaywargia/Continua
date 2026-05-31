package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// CreateSpanTx creates a new span within a transaction.
func (t *Tx) CreateSpan(ctx context.Context, params *platform.CreateSpanParams) (platform.Span, error) {
	return t.q.CreateSpan(ctx, *params)
}

// GetSpanByExternalIDTx retrieves a span by external ID within a transaction.
func (t *Tx) GetSpanByExternalID(ctx context.Context, traceUUID uuid.UUID, spanID string) (platform.Span, error) {
	span, err := t.q.GetSpanByExternalID(ctx, platform.GetSpanByExternalIDParams{
		TraceID: traceUUID,
		SpanID:  spanID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Span{}, ErrNotFound
	}
	return span, err
}

// ListSpansByTrace returns all spans for a trace within the supplied scope.
func (s *Store) ListSpansByTrace(ctx context.Context, scope Scope, traceUUID uuid.UUID) ([]platform.Span, error) {
	return s.q.ListSpansByTrace(ctx, platform.ListSpansByTraceParams{
		TraceID:         traceUUID,
		ProjectFilterID: scope.nullableProjectFilter(),
	})
}

// UpdateSpanStatusTx updates span status within a transaction.
func (t *Tx) UpdateSpanStatus(ctx context.Context, params platform.UpdateSpanStatusParams) (platform.Span, error) {
	return t.q.UpdateSpanStatus(ctx, params)
}

// UpdateSpanOutputTx updates span output within a transaction.
func (t *Tx) UpdateSpanOutput(ctx context.Context, params platform.UpdateSpanOutputParams) (platform.Span, error) {
	return t.q.UpdateSpanOutput(ctx, params)
}

// UpdateSpanTokensTx updates span tokens within a transaction.
func (t *Tx) UpdateSpanTokens(ctx context.Context, params platform.UpdateSpanTokensParams) (platform.Span, error) {
	return t.q.UpdateSpanTokens(ctx, params)
}

// UpsertSpanTx upserts a span within a transaction.
func (t *Tx) UpsertSpan(ctx context.Context, params *platform.UpsertSpanParams) (platform.Span, error) {
	return t.q.UpsertSpan(ctx, *params)
}
