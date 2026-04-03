package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateRun(ctx context.Context, arg enginedb.CreateRunParams) (enginedb.EngineRun, error) {
	return mapResult(o.q.CreateRun(ctx, arg))
}

func (o *storeOps) GetRun(ctx context.Context, id uuid.UUID) (enginedb.EngineRun, error) {
	return mapResult(o.q.GetRun(ctx, id))
}

func (o *storeOps) ListRunsByInstance(
	ctx context.Context,
	arg enginedb.ListRunsByInstanceParams,
) ([]enginedb.EngineRun, error) {
	return o.q.ListRunsByInstance(ctx, arg)
}

func (o *storeOps) UpdateRunStatus(
	ctx context.Context,
	arg enginedb.UpdateRunStatusParams,
) (enginedb.EngineRun, error) {
	return mapResult(o.q.UpdateRunStatus(ctx, arg))
}

func (o *storeOps) ClaimNextRun(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (enginedb.EngineRun, error) {
	return mapResult(o.q.ClaimNextRun(ctx, enginedb.ClaimNextRunParams{
		ClaimedBy:           nullableWorkerID(workerID),
		LeaseDurationMicros: leaseDurationMicros(leaseDuration),
	}))
}
