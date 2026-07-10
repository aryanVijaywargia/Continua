package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestHeartbeatStoreDefinitions_RefreshesAllRegisteredDefinitions(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()
	definitions := heartbeatTestDefinitions(t)

	if err := PublishDefinitions(ctx, store, definitions); err != nil {
		t.Fatalf("PublishDefinitions() error = %v", err)
	}
	checkoutBackdated := backdatePublishedRuntime(ctx, t, db, "checkout", "v1")
	shippingBackdated := backdatePublishedRuntime(ctx, t, db, "shipping", "v2")
	setPublishedDefinitionEnabled(ctx, t, db, "shipping", "v2", false)

	if err := HeartbeatStoreDefinitions(ctx, store, definitions); err != nil {
		t.Fatalf("HeartbeatStoreDefinitions() error = %v", err)
	}

	checkout := getPublishedDefinition(ctx, t, db, "checkout", "v1")
	if !checkout.RuntimePublishedAt.After(checkoutBackdated) {
		t.Fatalf("expected checkout runtime_published_at to refresh after %s, got %s",
			checkoutBackdated, checkout.RuntimePublishedAt)
	}
	if !checkout.Enabled {
		t.Fatalf("expected checkout to remain enabled, got %+v", checkout)
	}

	shipping := getPublishedDefinition(ctx, t, db, "shipping", "v2")
	if !shipping.RuntimePublishedAt.After(shippingBackdated) {
		t.Fatalf("expected shipping runtime_published_at to refresh after %s, got %s",
			shippingBackdated, shipping.RuntimePublishedAt)
	}
	if shipping.Enabled {
		t.Fatalf("expected heartbeat to preserve shipping enabled=false, got %+v", shipping)
	}
}

func TestHeartbeatStoreDefinitions_ReinsertsMissingRow(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()
	definitions := heartbeatTestDefinitions(t)

	if err := PublishDefinitions(ctx, store, definitions); err != nil {
		t.Fatalf("PublishDefinitions() error = %v", err)
	}
	deleted, err := store.DeleteDefinitionCatalogEntry(ctx, enginedb.DeleteDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	if err != nil {
		t.Fatalf("DeleteDefinitionCatalogEntry() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected to delete 1 checkout definition, deleted %d", deleted)
	}

	if err := HeartbeatStoreDefinitions(ctx, store, definitions); err != nil {
		t.Fatalf("HeartbeatStoreDefinitions() error = %v", err)
	}

	_, err = store.GetDefinitionCatalogEntry(ctx, enginedb.GetDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	if err != nil {
		t.Fatalf("expected heartbeat to reinsert checkout@v1, got error %v", err)
	}
}

func heartbeatTestDefinitions(t *testing.T) []publicworkflow.Definition {
	t.Helper()

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
	return registry.List()
}

func backdatePublishedRuntime(
	ctx context.Context,
	t *testing.T,
	db *enginetest.TestDatabase,
	definitionName string,
	definitionVersion string,
) time.Time {
	t.Helper()

	var backdated time.Time
	err := db.Pool.QueryRow(ctx, `
		UPDATE engine.definition_catalog
		SET runtime_published_at = NOW() - INTERVAL '10 minutes'
		WHERE definition_name = $1
		  AND definition_version = $2
		RETURNING runtime_published_at
	`, definitionName, definitionVersion).Scan(&backdated)
	if err != nil {
		t.Fatalf("backdate definition catalog runtime: %v", err)
	}
	return backdated
}

func setPublishedDefinitionEnabled(
	ctx context.Context,
	t *testing.T,
	db *enginetest.TestDatabase,
	definitionName string,
	definitionVersion string,
	enabled bool,
) {
	t.Helper()

	tag, err := db.Pool.Exec(ctx, `
		UPDATE engine.definition_catalog
		SET enabled = $3
		WHERE definition_name = $1
		  AND definition_version = $2
	`, definitionName, definitionVersion, enabled)
	if err != nil {
		t.Fatalf("set definition catalog enabled: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected to update 1 definition catalog row, updated %d", tag.RowsAffected())
	}
}

func getPublishedDefinition(
	ctx context.Context,
	t *testing.T,
	db *enginetest.TestDatabase,
	definitionName string,
	definitionVersion string,
) enginedb.EngineDefinitionCatalog {
	t.Helper()

	row, err := enginedb.New(db.Pool).GetDefinitionCatalogEntry(ctx, enginedb.GetDefinitionCatalogEntryParams{
		DefinitionName:    definitionName,
		DefinitionVersion: definitionVersion,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			t.Fatalf("expected definition %s@%s to exist", definitionName, definitionVersion)
		}
		t.Fatalf("GetDefinitionCatalogEntry() error = %v", err)
	}
	return row
}
