package store

import (
	"context"
	"time"

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

func (o *storeOps) TouchDefinitionCatalogEntry(
	ctx context.Context,
	arg enginedb.TouchDefinitionCatalogEntryParams,
) (int64, error) {
	return o.q.TouchDefinitionCatalogEntry(ctx, arg)
}

func (o *storeOps) SetDefinitionCatalogRuntimePublishedAt(
	ctx context.Context,
	arg enginedb.SetDefinitionCatalogRuntimePublishedAtParams,
) (time.Time, error) {
	return mapResult(o.q.SetDefinitionCatalogRuntimePublishedAt(ctx, arg))
}

func (o *storeOps) SetDefinitionCatalogEnabled(
	ctx context.Context,
	arg enginedb.SetDefinitionCatalogEnabledParams,
) (int64, error) {
	return o.q.SetDefinitionCatalogEnabled(ctx, arg)
}

func (o *storeOps) DeleteDefinitionCatalogEntry(
	ctx context.Context,
	arg enginedb.DeleteDefinitionCatalogEntryParams,
) (int64, error) {
	return o.q.DeleteDefinitionCatalogEntry(ctx, arg)
}
