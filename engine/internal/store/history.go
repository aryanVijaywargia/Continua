package store

import (
	"context"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) AppendHistory(ctx context.Context, arg enginedb.AppendHistoryParams) (enginedb.EngineHistory, error) {
	return mapResult(o.q.AppendHistory(ctx, arg))
}

func (o *storeOps) GetHistoryByRun(ctx context.Context, runID uuid.UUID) ([]enginedb.EngineHistory, error) {
	return o.q.GetHistoryByRun(ctx, runID)
}

func (o *storeOps) GetLatestHistoryIDByRun(ctx context.Context, runID uuid.UUID) (int64, error) {
	return o.q.GetLatestHistoryIDByRun(ctx, runID)
}

func (o *storeOps) ListHistoryByRunAfterID(
	ctx context.Context,
	arg enginedb.ListHistoryByRunAfterIDParams,
) ([]enginedb.EngineHistory, error) {
	return o.q.ListHistoryByRunAfterID(ctx, arg)
}

func (o *storeOps) ListHistoryByRunAfterSequence(
	ctx context.Context,
	arg enginedb.ListHistoryByRunAfterSequenceParams,
) ([]enginedb.EngineHistory, error) {
	return o.q.ListHistoryByRunAfterSequence(ctx, arg)
}

func (o *storeOps) GetHistoryByInstance(
	ctx context.Context,
	instanceID uuid.UUID,
) ([]enginedb.EngineHistory, error) {
	return o.q.GetHistoryByInstance(ctx, instanceID)
}

func (o *storeOps) DeleteHistoryByRun(ctx context.Context, runID uuid.UUID) error {
	return o.q.DeleteHistoryByRun(ctx, runID)
}
