package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// CreateSpan creates a new span record.
func (s *Store) CreateSpan(ctx context.Context, params *platform.CreateSpanParams) (platform.Span, error) {
	return s.q.CreateSpan(ctx, *params)
}

// CreateSpanTx creates a new span within a transaction.
func (t *Tx) CreateSpan(ctx context.Context, params *platform.CreateSpanParams) (platform.Span, error) {
	return t.q.CreateSpan(ctx, *params)
}

// GetSpan retrieves a span by its internal UUID.
func (s *Store) GetSpan(ctx context.Context, id uuid.UUID) (platform.Span, error) {
	span, err := s.q.GetSpan(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Span{}, ErrNotFound
	}
	return span, err
}

// GetSpanByExternalID retrieves a span by trace UUID and external span_id.
func (s *Store) GetSpanByExternalID(ctx context.Context, traceUUID uuid.UUID, spanID string) (platform.Span, error) {
	span, err := s.q.GetSpanByExternalID(ctx, platform.GetSpanByExternalIDParams{
		TraceID: traceUUID,
		SpanID:  spanID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Span{}, ErrNotFound
	}
	return span, err
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

// ListSpansByTrace returns all spans for a trace.
func (s *Store) ListSpansByTrace(ctx context.Context, traceUUID uuid.UUID) ([]platform.Span, error) {
	return s.q.ListSpansByTrace(ctx, traceUUID)
}

// ListSpansSummaryByTrace returns a summary view of spans for a trace.
func (s *Store) ListSpansSummaryByTrace(ctx context.Context, traceUUID uuid.UUID) ([]platform.ListSpansSummaryByTraceRow, error) {
	return s.q.ListSpansSummaryByTrace(ctx, traceUUID)
}

// CountSpansByTrace returns the number of spans in a trace.
func (s *Store) CountSpansByTrace(ctx context.Context, traceUUID uuid.UUID) (int64, error) {
	return s.q.CountSpansByTrace(ctx, traceUUID)
}

// UpdateSpanStatus updates a span's status, end time, and message.
func (s *Store) UpdateSpanStatus(ctx context.Context, params platform.UpdateSpanStatusParams) (platform.Span, error) {
	return s.q.UpdateSpanStatus(ctx, params)
}

// UpdateSpanStatusTx updates span status within a transaction.
func (t *Tx) UpdateSpanStatus(ctx context.Context, params platform.UpdateSpanStatusParams) (platform.Span, error) {
	return t.q.UpdateSpanStatus(ctx, params)
}

// UpdateSpanOutput updates a span's output data.
func (s *Store) UpdateSpanOutput(ctx context.Context, params platform.UpdateSpanOutputParams) (platform.Span, error) {
	return s.q.UpdateSpanOutput(ctx, params)
}

// UpdateSpanOutputTx updates span output within a transaction.
func (t *Tx) UpdateSpanOutput(ctx context.Context, params platform.UpdateSpanOutputParams) (platform.Span, error) {
	return t.q.UpdateSpanOutput(ctx, params)
}

// UpdateSpanTokens updates token counts and cost for a span.
func (s *Store) UpdateSpanTokens(ctx context.Context, params platform.UpdateSpanTokensParams) (platform.Span, error) {
	return s.q.UpdateSpanTokens(ctx, params)
}

// UpdateSpanTokensTx updates span tokens within a transaction.
func (t *Tx) UpdateSpanTokens(ctx context.Context, params platform.UpdateSpanTokensParams) (platform.Span, error) {
	return t.q.UpdateSpanTokens(ctx, params)
}

// UpsertSpan upserts a span with patch semantics.
// NULL values don't overwrite existing values, and error status is never downgraded.
func (s *Store) UpsertSpan(ctx context.Context, params *platform.UpsertSpanParams) (platform.Span, error) {
	return s.q.UpsertSpan(ctx, *params)
}

// UpsertSpanTx upserts a span within a transaction.
func (t *Tx) UpsertSpan(ctx context.Context, params *platform.UpsertSpanParams) (platform.Span, error) {
	return t.q.UpsertSpan(ctx, *params)
}
