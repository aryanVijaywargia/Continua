package store

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// ComputeAndUpdateTraceRollups computes rollup values for a trace and updates it.
// This is called after ingesting spans to update aggregate values.
func (s *Store) ComputeAndUpdateTraceRollups(ctx context.Context, traceID uuid.UUID) error {
	// Compute rollups from spans
	rollups, err := s.q.ComputeTraceRollups(ctx, traceID)
	if err != nil {
		return err
	}

	totalCost := numericFromAny(rollups.TotalCost)

	// Update the trace with computed rollups
	return s.q.UpdateTraceRollups(ctx, platform.UpdateTraceRollupsParams{
		ID:             traceID,
		TotalSpans:     &rollups.TotalSpans,
		TotalTokensIn:  rollups.TotalTokensIn,
		TotalTokensOut: rollups.TotalTokensOut,
		TotalCost:      totalCost,
		ErrorCount:     &rollups.ErrorCount,
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

func numericFromAny(value any) pgtype.Numeric {
	var totalCost pgtype.Numeric
	if value == nil {
		return totalCost
	}

	switch v := value.(type) {
	case pgtype.Numeric:
		return v
	case *pgtype.Numeric:
		if v != nil {
			return *v
		}
		return totalCost
	case []byte:
		if err := totalCost.Scan(string(v)); err != nil {
			log.Printf("Warning: failed to convert total_cost []byte: %v", err)
		}
		return totalCost
	default:
		if err := totalCost.Scan(v); err != nil {
			log.Printf("Warning: failed to convert total_cost: %v", err)
		}
		return totalCost
	}
}
