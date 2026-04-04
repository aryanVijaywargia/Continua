package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateActivityTask(
	ctx context.Context,
	arg enginedb.CreateActivityTaskParams,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.CreateActivityTask(ctx, arg))
}

func (o *storeOps) GetActivityTask(ctx context.Context, id uuid.UUID) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.GetActivityTask(ctx, id))
}

func (o *storeOps) GetActivityTaskByRunAndKey(
	ctx context.Context,
	arg enginedb.GetActivityTaskByRunAndKeyParams,
) (enginedb.EngineActivityTask, error) {
	return mapResult(o.q.GetActivityTaskByRunAndKey(ctx, arg))
}

func (o *storeOps) ListActivityTasksByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineActivityTask, error) {
	return o.q.ListActivityTasksByRun(ctx, runID)
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
	id uuid.UUID,
	claimedBy string,
	output []byte,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.CompleteActivityTask(ctx, enginedb.CompleteActivityTaskParams{
		ID:        id,
		ClaimedBy: nullableWorkerID(claimedBy),
		Output:    output,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyActivityTaskCASMiss(ctx, id, err)
}

func (o *storeOps) FailActivityTask(
	ctx context.Context,
	id uuid.UUID,
	claimedBy string,
	errorCode *string,
	errorMessage *string,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.FailActivityTask(ctx, enginedb.FailActivityTaskParams{
		ID:               id,
		ClaimedBy:        nullableWorkerID(claimedBy),
		LastErrorCode:    errorCode,
		LastErrorMessage: errorMessage,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyActivityTaskCASMiss(ctx, id, err)
}

func (o *storeOps) classifyActivityTaskCASMiss(ctx context.Context, id uuid.UUID, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return normalizeError(err)
	}

	if _, lookupErr := o.GetActivityTask(ctx, id); lookupErr != nil {
		return lookupErr
	}
	return ErrStaleClaim
}
