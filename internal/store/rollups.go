package store

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// TraceRollups contains aggregated values for a trace.
type TraceRollups struct {
	TotalSpans  int32
	TotalTokens int64
	TotalCost   pgtype.Numeric
	ErrorCount  int32
}

// ComputeAndUpdateTraceRollups computes rollup values for a trace and updates it.
// This is called after ingesting spans to update aggregate values.
func (s *Store) ComputeAndUpdateTraceRollups(ctx context.Context, traceID uuid.UUID) error {
	// Compute rollups from spans
	rollups, err := s.q.ComputeTraceRollups(ctx, traceID)
	if err != nil {
		return err
	}

	// Convert total_cost from interface{} to pgtype.Numeric
	var totalCost pgtype.Numeric
	if rollups.TotalCost != nil {
		if err := totalCost.Scan(rollups.TotalCost); err != nil {
			log.Printf("Warning: failed to convert total_cost: %v", err)
			// Continue with zero cost rather than failing
		}
	}

	// Update the trace with computed rollups
	return s.q.UpdateTraceRollups(ctx, platform.UpdateTraceRollupsParams{
		ID:          traceID,
		TotalSpans:  &rollups.TotalSpans,
		TotalTokens: &rollups.TotalTokens,
		TotalCost:   totalCost,
		ErrorCount:  &rollups.ErrorCount,
	})
}

// ComputeAndUpdateTraceRollupsTx computes and updates rollups within a transaction.
func (s *Store) ComputeAndUpdateTraceRollupsTx(ctx context.Context, tx *Tx, traceID uuid.UUID) error {
	// Compute rollups from spans using transaction's query
	rollups, err := tx.q.ComputeTraceRollups(ctx, traceID)
	if err != nil {
		return err
	}

	// Convert total_cost from interface{} to pgtype.Numeric
	var totalCost pgtype.Numeric
	if rollups.TotalCost != nil {
		if err := totalCost.Scan(rollups.TotalCost); err != nil {
			log.Printf("Warning: failed to convert total_cost: %v", err)
		}
	}

	// Update the trace with computed rollups using transaction's query
	return tx.q.UpdateTraceRollups(ctx, platform.UpdateTraceRollupsParams{
		ID:          traceID,
		TotalSpans:  &rollups.TotalSpans,
		TotalTokens: &rollups.TotalTokens,
		TotalCost:   totalCost,
		ErrorCount:  &rollups.ErrorCount,
	})
}

// GetTraceVersion returns the version number of a trace for optimistic concurrency.
// Used by rollup worker to detect if trace was modified during processing.
func (s *Store) GetTraceVersion(ctx context.Context, traceID uuid.UUID) (int32, error) {
	version, err := s.q.GetTraceVersion(ctx, traceID)
	if err != nil {
		return 0, err
	}
	if version == nil {
		return 0, nil
	}
	return *version, nil
}
