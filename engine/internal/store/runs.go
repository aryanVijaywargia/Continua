package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

type WakeWaitingRunResult struct {
	Run     enginedb.EngineRun
	Applied bool
}

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateRun(ctx context.Context, arg enginedb.CreateRunParams) (enginedb.EngineRun, error) {
	return mapResult(o.q.CreateRun(ctx, arg))
}

//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateChildRun(ctx context.Context, arg enginedb.CreateChildRunParams) (enginedb.EngineRun, error) {
	return mapResult(o.q.CreateChildRun(ctx, arg))
}

func (o *storeOps) GetRun(ctx context.Context, id uuid.UUID) (enginedb.EngineRun, error) {
	return mapResult(o.q.GetRun(ctx, id))
}

func (o *storeOps) GetRunForUpdate(ctx context.Context, id uuid.UUID) (enginedb.EngineRun, error) {
	return mapResult(o.q.GetRunForUpdate(ctx, id))
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

func (o *storeOps) TransitionRunToWaiting(
	ctx context.Context,
	arg enginedb.TransitionRunToWaitingParams,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToWaiting(ctx, arg)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunCASMiss(ctx, arg.ID, err)
}

func (o *storeOps) TransitionRunToCompleted(
	ctx context.Context,
	arg enginedb.TransitionRunToCompletedParams,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToCompleted(ctx, arg)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunCASMiss(ctx, arg.ID, err)
}

func (o *storeOps) TransitionRunToFailed(
	ctx context.Context,
	arg enginedb.TransitionRunToFailedParams,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToFailed(ctx, arg)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunCASMiss(ctx, arg.ID, err)
}

func (o *storeOps) TransitionRunToQuarantined(
	ctx context.Context,
	arg enginedb.TransitionRunToQuarantinedParams,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToQuarantined(ctx, arg)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunCASMiss(ctx, arg.ID, err)
}

func (o *storeOps) TransitionRunToCancelled(
	ctx context.Context,
	arg enginedb.TransitionRunToCancelledParams,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToCancelled(ctx, arg)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunInvariantMiss(ctx, arg.ID, "cancel", err)
}

func (o *storeOps) TransitionRunToContinuedAsNew(
	ctx context.Context,
	arg enginedb.TransitionRunToContinuedAsNewParams,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToContinuedAsNew(ctx, arg)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunCASMiss(ctx, arg.ID, err)
}

func (o *storeOps) TransitionRunToTerminated(
	ctx context.Context,
	id uuid.UUID,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToTerminated(ctx, id)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunInvariantMiss(ctx, id, "terminate", err)
}

func (o *storeOps) TransitionRunToQueuedFromQuarantined(
	ctx context.Context,
	id uuid.UUID,
) (enginedb.EngineRun, error) {
	run, err := o.q.TransitionRunToQueuedFromQuarantined(ctx, id)
	if err == nil {
		return run, nil
	}
	return enginedb.EngineRun{}, o.classifyRunInvariantMiss(ctx, id, "resume quarantine", err)
}

func (o *storeOps) WakeWaitingRun(ctx context.Context, id uuid.UUID) (WakeWaitingRunResult, error) {
	run, err := o.q.WakeWaitingRun(ctx, id)
	if err == nil {
		return WakeWaitingRunResult{Run: run, Applied: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return WakeWaitingRunResult{}, normalizeError(err)
	}

	current, lookupErr := o.GetRun(ctx, id)
	if lookupErr != nil {
		return WakeWaitingRunResult{}, lookupErr
	}
	return WakeWaitingRunResult{Run: current, Applied: false}, nil
}

func (o *storeOps) WakeWaitingChildWorkflowRun(
	ctx context.Context,
	arg enginedb.WakeWaitingChildWorkflowRunParams,
) (WakeWaitingRunResult, error) {
	run, err := o.q.WakeWaitingChildWorkflowRun(ctx, arg)
	if err == nil {
		return WakeWaitingRunResult{Run: run, Applied: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return WakeWaitingRunResult{}, normalizeError(err)
	}

	current, lookupErr := o.GetRun(ctx, arg.ID)
	if lookupErr != nil {
		return WakeWaitingRunResult{}, lookupErr
	}
	return WakeWaitingRunResult{Run: current, Applied: false}, nil
}

func (o *storeOps) ClaimNextRun(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (enginedb.EngineRun, error) {
	if o.projectFilter != nil {
		return mapResult(o.q.ClaimNextRunByProject(ctx, enginedb.ClaimNextRunByProjectParams{
			ProjectFilterID:     *o.projectFilter,
			ClaimedBy:           nullableWorkerID(workerID),
			LeaseDurationMicros: leaseDurationMicros(leaseDuration),
		}))
	}
	return mapResult(o.q.ClaimNextRun(ctx, enginedb.ClaimNextRunParams{
		ClaimedBy:           nullableWorkerID(workerID),
		LeaseDurationMicros: leaseDurationMicros(leaseDuration),
	}))
}

func (o *storeOps) ReleaseRunsByClaimant(
	ctx context.Context,
	claimant string,
) ([]enginedb.EngineRun, error) {
	return o.q.ReleaseRunsByClaimant(ctx, &claimant)
}

func (o *storeOps) classifyRunCASMiss(ctx context.Context, id uuid.UUID, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return normalizeError(err)
	}

	if _, lookupErr := o.GetRun(ctx, id); lookupErr != nil {
		return lookupErr
	}
	return ErrStaleClaim
}

func (o *storeOps) classifyRunInvariantMiss(ctx context.Context, id uuid.UUID, operation string, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return normalizeError(err)
	}

	current, err := o.GetRun(ctx, id)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: run %s transition %s returned zero rows with current status %s", ErrInvariant, id, operation, current.Status)
}
