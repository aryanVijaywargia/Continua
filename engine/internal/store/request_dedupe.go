package store

import (
	"context"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func (o *storeOps) CreateRequestDedupe(
	ctx context.Context,
	arg enginedb.CreateRequestDedupeParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.CreateRequestDedupe(ctx, arg))
}

func (o *storeOps) GetRequestDedupeByScopeAndKey(
	ctx context.Context,
	arg enginedb.GetRequestDedupeByScopeAndKeyParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.GetRequestDedupeByScopeAndKey(ctx, arg))
}

func (o *storeOps) FinalizeRequestDedupeWithResponse(
	ctx context.Context,
	arg enginedb.FinalizeRequestDedupeWithResponseParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.FinalizeRequestDedupeWithResponse(ctx, arg))
}

func (o *storeOps) FinalizeRequestDedupeWithError(
	ctx context.Context,
	arg enginedb.FinalizeRequestDedupeWithErrorParams,
) (enginedb.EngineRequestDedupe, error) {
	return mapResult(o.q.FinalizeRequestDedupeWithError(ctx, arg))
}
