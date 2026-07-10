package main

import (
	"context"
	"testing"

	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"

	"github.com/jackc/pgx/v5"
)

func TestResolveStartDefinitionVersionOmittedUsesLatestRegisteredVersion(t *testing.T) {
	run := func(publicworkflow.Context) error { return nil }
	registry, err := engineworkflow.NewRegistry(
		publicworkflow.Definition{Name: "demo", Version: "v1", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v10", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v2", Run: run},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	version, ok := resolveStartDefinitionVersion(registry, "demo", "")
	if !ok {
		t.Fatalf("resolveStartDefinitionVersion() returned ok=false")
	}
	if version != "v10" {
		t.Fatalf("resolved version = %q, want v10", version)
	}
}

func TestResolveStartDefinitionVersionExplicitUsesExactMatch(t *testing.T) {
	run := func(publicworkflow.Context) error { return nil }
	registry, err := engineworkflow.NewRegistry(
		publicworkflow.Definition{Name: "demo", Version: "v1", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v2", Run: run},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	version, ok := resolveStartDefinitionVersion(registry, "demo", "v1")
	if !ok {
		t.Fatalf("resolveStartDefinitionVersion() returned ok=false")
	}
	if version != "v1" {
		t.Fatalf("resolved version = %q, want v1", version)
	}

	if _, ok := resolveStartDefinitionVersion(registry, "demo", "v10"); ok {
		t.Fatalf("resolveStartDefinitionVersion() returned ok=true for unregistered explicit version")
	}
}

func TestEnsureDarkLaunchProjectProvisionsProjectInTransaction(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	store := enginestore.New(db.Pool)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `DELETE FROM public.projects WHERE id = $1`, darkLaunchProjectID); err != nil {
		t.Fatalf("delete seeded dark-launch project: %v", err)
	}

	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureDarkLaunchProject(ctx, tx); err != nil {
		t.Fatalf("ensureDarkLaunchProject() error = %v", err)
	}
	existsBeforeCommit, err := store.PlatformProjectExists(ctx, darkLaunchProjectID)
	if err != nil {
		t.Fatalf("PlatformProjectExists() before commit error = %v", err)
	}
	if existsBeforeCommit {
		t.Fatal("project should not be visible outside the transaction before commit")
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	existsAfterCommit, err := store.PlatformProjectExists(ctx, darkLaunchProjectID)
	if err != nil {
		t.Fatalf("PlatformProjectExists() after commit error = %v", err)
	}
	if !existsAfterCommit {
		t.Fatal("project should exist after committing dark-launch bootstrap")
	}

	secondTx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() second error = %v", err)
	}
	defer func() {
		_ = secondTx.Rollback(ctx)
	}()
	if err := ensureDarkLaunchProject(ctx, secondTx); err != nil {
		t.Fatalf("ensureDarkLaunchProject() second error = %v", err)
	}
	if err := secondTx.Commit(ctx); err != nil {
		t.Fatalf("Commit() second error = %v", err)
	}
}
