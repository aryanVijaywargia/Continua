package catalog

import (
	"context"
	"testing"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestPublishStoreDefinitions_MatchesRuntimeRegistry(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	queries := enginedb.New(db.Pool)
	ctx := context.Background()

	registry, err := engineworkflow.NewRegistry(
		publicworkflow.Definition{
			Name:    "checkout",
			Version: "v1",
			Run: func(publicworkflow.Context) error {
				return nil
			},
		},
		publicworkflow.Definition{
			Name:    "shipping",
			Version: "v2",
			Run: func(publicworkflow.Context) error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if err := PublishDefinitions(ctx, queries, registry.List()); err != nil {
		t.Fatalf("PublishDefinitions() error = %v", err)
	}

	rows, err := queries.ListDefinitionCatalog(ctx)
	if err != nil {
		t.Fatalf("ListDefinitionCatalog() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 published definitions, got %+v", rows)
	}

	got := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		got[row.DefinitionName+"@"+row.DefinitionVersion] = struct{}{}
	}

	for _, definition := range registry.List() {
		key := definition.Name + "@" + definition.Version
		if _, ok := got[key]; !ok {
			t.Fatalf("expected published catalog to contain %s, got %+v", key, got)
		}
	}
}

func TestPublishStoreDefinitions_RemovesStaleDefinitions(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()

	initialDefinitions := []publicworkflow.Definition{
		{
			Name:    "checkout",
			Version: "v1",
			Run: func(publicworkflow.Context) error {
				return nil
			},
		},
		{
			Name:    "shipping",
			Version: "v2",
			Run: func(publicworkflow.Context) error {
				return nil
			},
		},
	}
	if err := PublishStoreDefinitions(ctx, store, initialDefinitions); err != nil {
		t.Fatalf("PublishStoreDefinitions(initial) error = %v", err)
	}

	if err := PublishStoreDefinitions(ctx, store, initialDefinitions[:1]); err != nil {
		t.Fatalf("PublishStoreDefinitions(updated) error = %v", err)
	}

	rows, err := store.ListDefinitionCatalog(ctx)
	if err != nil {
		t.Fatalf("ListDefinitionCatalog() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected stale definitions to be removed, got %+v", rows)
	}
	if rows[0].DefinitionName != "checkout" || rows[0].DefinitionVersion != "v1" {
		t.Fatalf("unexpected remaining definition %+v", rows[0])
	}
}
