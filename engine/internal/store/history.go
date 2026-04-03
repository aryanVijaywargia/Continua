package store

import (
	"context"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func (o *storeOps) AppendHistory(ctx context.Context, arg enginedb.AppendHistoryParams) (enginedb.EngineHistory, error) {
	return mapResult(o.q.AppendHistory(ctx, arg))
}

func (o *storeOps) GetHistoryByRun(ctx context.Context, runID uuid.UUID) ([]enginedb.EngineHistory, error) {
	return o.q.GetHistoryByRun(ctx, runID)
}

func (o *storeOps) GetHistoryByInstance(
	ctx context.Context,
	instanceID uuid.UUID,
) ([]enginedb.EngineHistory, error) {
	return o.q.GetHistoryByInstance(ctx, instanceID)
}
