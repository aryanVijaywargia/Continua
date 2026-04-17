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

func (o *storeOps) ListOpenActivityTasksByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineActivityTask, error) {
	return o.q.ListOpenActivityTasksByRun(ctx, runID)
}

func (o *storeOps) ListCancelledActivityTasksByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineActivityTask, error) {
	return o.q.ListCancelledActivityTasksByRun(ctx, runID)
}

func (o *storeOps) ClaimNextActivityTask(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (enginedb.EngineActivityTask, error) {
	if o.projectFilter != nil {
		return mapResult(o.q.ClaimNextActivityTaskByProject(ctx, enginedb.ClaimNextActivityTaskByProjectParams{
			ProjectFilterID:     *o.projectFilter,
			ClaimedBy:           nullableWorkerID(workerID),
			LeaseDurationMicros: leaseDurationMicros(leaseDuration),
		}))
	}
	return mapResult(o.q.ClaimNextActivityTask(ctx, enginedb.ClaimNextActivityTaskParams{
		ClaimedBy:           nullableWorkerID(workerID),
		LeaseDurationMicros: leaseDurationMicros(leaseDuration),
	}))
}

func (o *storeOps) ClaimRemoteActivityTasks(
	ctx context.Context,
	projectID uuid.UUID,
	workerID string,
	activityTypes []string,
	maxTasks int32,
	leaseDuration time.Duration,
) ([]enginedb.EngineActivityTask, error) {
	leaseDurationMS := leaseDuration.Milliseconds()
	return o.q.ClaimRemoteActivityTasks(ctx, enginedb.ClaimRemoteActivityTasksParams{
		ProjectFilterID: projectID,
		ClaimedBy:       nullableWorkerID(workerID),
		ActivityTypes:   activityTypes,
		MaxTasks:        maxTasks,
		LeaseDurationMs: &leaseDurationMS,
	})
}

func (o *storeOps) HeartbeatRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	id uuid.UUID,
	claimedBy string,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.HeartbeatRemoteActivityTask(ctx, enginedb.HeartbeatRemoteActivityTaskParams{
		ID:        id,
		ProjectID: projectID,
		ClaimedBy: nullableWorkerID(claimedBy),
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyRemoteActivityTaskCASMiss(ctx, projectID, id, err)
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

func (o *storeOps) CompleteRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	id uuid.UUID,
	claimedBy string,
	output []byte,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.CompleteRemoteActivityTask(ctx, enginedb.CompleteRemoteActivityTaskParams{
		ID:        id,
		ProjectID: projectID,
		ClaimedBy: nullableWorkerID(claimedBy),
		Output:    output,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyRemoteActivityTaskCASMiss(ctx, projectID, id, err)
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

func (o *storeOps) RetryRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	id uuid.UUID,
	claimedBy string,
	retryDelayMS int64,
	errorCode *string,
	errorMessage *string,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.RetryRemoteActivityTask(ctx, enginedb.RetryRemoteActivityTaskParams{
		ID:               id,
		ProjectID:        projectID,
		ClaimedBy:        nullableWorkerID(claimedBy),
		RetryDelayMs:     retryDelayMS,
		LastErrorCode:    errorCode,
		LastErrorMessage: errorMessage,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyRemoteActivityTaskCASMiss(ctx, projectID, id, err)
}

func (o *storeOps) FailRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	id uuid.UUID,
	claimedBy string,
	errorCode *string,
	errorMessage *string,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.FailRemoteActivityTask(ctx, enginedb.FailRemoteActivityTaskParams{
		ID:               id,
		ProjectID:        projectID,
		ClaimedBy:        nullableWorkerID(claimedBy),
		LastErrorCode:    errorCode,
		LastErrorMessage: errorMessage,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyRemoteActivityTaskCASMiss(ctx, projectID, id, err)
}

func (o *storeOps) RetryActivityTask(
	ctx context.Context,
	id uuid.UUID,
	claimedBy string,
	retryDelayMS int64,
) (enginedb.EngineActivityTask, error) {
	task, err := o.q.RetryActivityTask(ctx, enginedb.RetryActivityTaskParams{
		ID:           id,
		ClaimedBy:    nullableWorkerID(claimedBy),
		RetryDelayMs: retryDelayMS,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, o.classifyActivityTaskCASMiss(ctx, id, err)
}

func (o *storeOps) CancelOpenActivityTasksByRun(
	ctx context.Context,
	runID uuid.UUID,
) ([]enginedb.EngineActivityTask, error) {
	return o.q.CancelOpenActivityTasksByRun(ctx, runID)
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

func (o *storeOps) classifyRemoteActivityTaskCASMiss(
	ctx context.Context,
	projectID uuid.UUID,
	id uuid.UUID,
	err error,
) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return normalizeError(err)
	}

	_, lookupErr := o.q.GetActivityTaskRemoteConflictState(ctx, enginedb.GetActivityTaskRemoteConflictStateParams{
		ID:        id,
		ProjectID: projectID,
	})
	if lookupErr != nil {
		return normalizeError(lookupErr)
	}
	return ErrStaleClaim
}
