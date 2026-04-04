package store

import (
	"context"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateActivityTask(
	ctx context.Context,
	arg enginedb.CreateActivityTaskParams,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.CreateActivityTask(ctx, arg))
}

func (o *storeOps) GetActivityTaskByRunAndKey(
	ctx context.Context,
	arg enginedb.GetActivityTaskByRunAndKeyParams,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.GetActivityTaskByRunAndKey(ctx, arg))
}

func (o *storeOps) ClaimNextActivityTask(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.ClaimNextActivityTask(ctx, enginedb.ClaimNextActivityTaskParams{
		ClaimedBy:           nullableWorkerID(workerID),
		LeaseDurationMicros: leaseDurationMicros(leaseDuration),
	}))
}

func (o *storeOps) CompleteActivityTask(
	ctx context.Context,
	arg enginedb.CompleteActivityTaskParams,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.CompleteActivityTask(ctx, arg))
}

func (o *storeOps) FailActivityTask(
	ctx context.Context,
	arg enginedb.FailActivityTaskParams,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.FailActivityTask(ctx, arg))
}
