package store

import (
	"context"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func (o *storeOps) UpsertDefinitionCatalogEntry(
	ctx context.Context,
	arg enginedb.UpsertDefinitionCatalogEntryParams,
) (enginedb.EngineDefinitionCatalog, error) {
	return mapResult(o.q.UpsertDefinitionCatalogEntry(ctx, arg))
}

func (o *storeOps) GetDefinitionCatalogEntry(
	ctx context.Context,
	arg enginedb.GetDefinitionCatalogEntryParams,
) (enginedb.EngineDefinitionCatalog, error) {
	return mapResult(o.q.GetDefinitionCatalogEntry(ctx, arg))
}

func (o *storeOps) ListDefinitionCatalog(ctx context.Context) ([]enginedb.EngineDefinitionCatalog, error) {
	return o.q.ListDefinitionCatalog(ctx)
}

func (o *storeOps) DeleteDefinitionCatalogEntry(
	ctx context.Context,
	arg enginedb.DeleteDefinitionCatalogEntryParams,
) (int64, error) {
	return o.q.DeleteDefinitionCatalogEntry(ctx, arg)
}
