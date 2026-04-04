package store

import (
	"context"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func (o *storeOps) GetProjectionCheckpoint(
	ctx context.Context,
	arg enginedb.GetProjectionCheckpointParams,
) (enginedb.EngineProjectionCheckpoint, error) {
	return mapResult(o.q.GetProjectionCheckpoint(ctx, arg))
}

func (o *storeOps) AdvanceProjectionCheckpoint(
	ctx context.Context,
	arg enginedb.AdvanceProjectionCheckpointParams,
) (enginedb.EngineProjectionCheckpoint, error) {
	return mapStaleCheckpointResult(o.q.AdvanceProjectionCheckpoint(ctx, arg))
}
