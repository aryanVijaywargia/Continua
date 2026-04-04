package store

import (
	"context"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func (o *storeOps) CreateInstance(ctx context.Context, arg enginedb.CreateInstanceParams) (enginedb.EngineInstance, error) {
	return mapResult(o.q.CreateInstance(ctx, arg))
}

func (o *storeOps) GetInstance(ctx context.Context, id uuid.UUID) (enginedb.EngineInstance, error) {
	return mapResult(o.q.GetInstance(ctx, id))
}

func (o *storeOps) GetInstanceByProjectAndKey(
	ctx context.Context,
	arg enginedb.GetInstanceByProjectAndKeyParams,
) (enginedb.EngineInstance, error) {
	return mapResult(o.q.GetInstanceByProjectAndKey(ctx, arg))
}

func (o *storeOps) ListInstancesByProject(
	ctx context.Context,
	arg enginedb.ListInstancesByProjectParams,
) ([]enginedb.EngineInstance, error) {
	return o.q.ListInstancesByProject(ctx, arg)
}

func (o *storeOps) UpdateInstanceStatus(
	ctx context.Context,
	arg enginedb.UpdateInstanceStatusParams,
) (enginedb.EngineInstance, error) {
	return mapResult(o.q.UpdateInstanceStatus(ctx, arg))
}
