package store

import (
	"context"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

// Child lifecycle transactions span runs, history, inbox, and wake-state updates.
// The activator owns that orchestration; store wrappers here expose the query primitives.
//
//nolint:gocritic // Mirror sqlc's generated value-based params in thin store wrappers.
func (o *storeOps) CreateChildWorkflow(
	ctx context.Context,
	arg enginedb.CreateChildWorkflowParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.CreateChildWorkflow(ctx, arg))
}

func (o *storeOps) GetChildWorkflowByParentRunAndKey(
	ctx context.Context,
	arg enginedb.GetChildWorkflowByParentRunAndKeyParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.GetChildWorkflowByParentRunAndKey(ctx, arg))
}

func (o *storeOps) GetChildWorkflowByParentRunAndKeyForUpdate(
	ctx context.Context,
	arg enginedb.GetChildWorkflowByParentRunAndKeyForUpdateParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.GetChildWorkflowByParentRunAndKeyForUpdate(ctx, arg))
}

func (o *storeOps) GetChildWorkflowByChildInstanceForUpdate(
	ctx context.Context,
	arg enginedb.GetChildWorkflowByChildInstanceForUpdateParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.GetChildWorkflowByChildInstanceForUpdate(ctx, arg))
}

func (o *storeOps) GetChildWorkflowByCurrentChildRunForUpdate(
	ctx context.Context,
	arg enginedb.GetChildWorkflowByCurrentChildRunForUpdateParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.GetChildWorkflowByCurrentChildRunForUpdate(ctx, arg))
}

func (o *storeOps) GetChildWorkflowOutcomeByParentRunAndKey(
	ctx context.Context,
	arg enginedb.GetChildWorkflowOutcomeByParentRunAndKeyParams,
) (enginedb.GetChildWorkflowOutcomeByParentRunAndKeyRow, error) {
	return mapResult(o.q.GetChildWorkflowOutcomeByParentRunAndKey(ctx, arg))
}

func (o *storeOps) ListChildWorkflowOutcomesByParentRun(
	ctx context.Context,
	projectID uuid.UUID,
	parentRunID uuid.UUID,
) ([]enginedb.ListChildWorkflowOutcomesByParentRunRow, error) {
	return o.q.ListChildWorkflowOutcomesByParentRun(ctx, enginedb.ListChildWorkflowOutcomesByParentRunParams{
		ProjectID:   projectID,
		ParentRunID: parentRunID,
	})
}

func (o *storeOps) ListChildWorkflowsByParentRun(
	ctx context.Context,
	projectID uuid.UUID,
	parentRunID uuid.UUID,
) ([]enginedb.EngineChildWorkflow, error) {
	return o.q.ListChildWorkflowsByParentRun(ctx, enginedb.ListChildWorkflowsByParentRunParams{
		ProjectID:   projectID,
		ParentRunID: parentRunID,
	})
}

func (o *storeOps) ListActiveChildWorkflowsByParentRun(
	ctx context.Context,
	projectID uuid.UUID,
	parentRunID uuid.UUID,
) ([]enginedb.EngineChildWorkflow, error) {
	return o.q.ListActiveChildWorkflowsByParentRun(ctx, enginedb.ListActiveChildWorkflowsByParentRunParams{
		ProjectID:   projectID,
		ParentRunID: parentRunID,
	})
}

func (o *storeOps) ListActiveChildWorkflowDescendants(
	ctx context.Context,
	projectID uuid.UUID,
	parentRunID uuid.UUID,
) ([]enginedb.ListActiveChildWorkflowDescendantsRow, error) {
	return o.q.ListActiveChildWorkflowDescendants(ctx, enginedb.ListActiveChildWorkflowDescendantsParams{
		ProjectID:   projectID,
		ParentRunID: parentRunID,
	})
}

func (o *storeOps) UpdateChildWorkflowTerminal(
	ctx context.Context,
	arg enginedb.UpdateChildWorkflowTerminalParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.UpdateChildWorkflowTerminal(ctx, arg))
}

func (o *storeOps) MarkChildWorkflowParentWaitFailed(
	ctx context.Context,
	arg enginedb.MarkChildWorkflowParentWaitFailedParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.MarkChildWorkflowParentWaitFailed(ctx, arg))
}

func (o *storeOps) UpdateChildWorkflowContinuation(
	ctx context.Context,
	arg enginedb.UpdateChildWorkflowContinuationParams,
) (enginedb.EngineChildWorkflow, error) {
	return mapResult(o.q.UpdateChildWorkflowContinuation(ctx, arg))
}
