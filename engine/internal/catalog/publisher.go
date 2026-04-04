package catalog

import (
	"context"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/store"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
	"github.com/jackc/pgx/v5"
)

type definitionCatalogWriter interface {
	UpsertDefinitionCatalogEntry(context.Context, enginedb.UpsertDefinitionCatalogEntryParams) (enginedb.EngineDefinitionCatalog, error)
	ListDefinitionCatalog(context.Context) ([]enginedb.EngineDefinitionCatalog, error)
	DeleteDefinitionCatalogEntry(context.Context, enginedb.DeleteDefinitionCatalogEntryParams) (int64, error)
}

func PublishDefinitions(
	ctx context.Context,
	writer definitionCatalogWriter,
	definitions []publicworkflow.Definition,
) error {
	liveDefinitions := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		liveDefinitions[definition.Name+"@"+definition.Version] = struct{}{}
		if _, err := writer.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
			DefinitionName:    definition.Name,
			DefinitionVersion: definition.Version,
		}); err != nil {
			return err
		}
	}

	rows, err := writer.ListDefinitionCatalog(ctx)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if _, ok := liveDefinitions[row.DefinitionName+"@"+row.DefinitionVersion]; ok {
			continue
		}
		if _, err := writer.DeleteDefinitionCatalogEntry(ctx, enginedb.DeleteDefinitionCatalogEntryParams{
			DefinitionName:    row.DefinitionName,
			DefinitionVersion: row.DefinitionVersion,
		}); err != nil {
			return err
		}
	}

	return nil
}

func PublishStoreDefinitions(ctx context.Context, engineStore *store.Store, definitions []publicworkflow.Definition) error {
	tx, err := engineStore.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := PublishDefinitions(ctx, tx, definitions); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
