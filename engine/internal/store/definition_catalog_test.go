package store

import (
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestUpsertDefinitionCatalogEntry_RefreshesHeartbeatPreservesEnabled(t *testing.T) {
	ts := newTestStore(t)

	if _, err := ts.store.UpsertDefinitionCatalogEntry(ts.ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	}); err != nil {
		t.Fatalf("UpsertDefinitionCatalogEntry(initial) error = %v", err)
	}
	backdated := backdateDefinitionCatalogRuntime(t, ts, "checkout", "v1")
	setDefinitionCatalogEnabled(t, ts, "checkout", "v1", false)

	row, err := ts.store.UpsertDefinitionCatalogEntry(ts.ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	if err != nil {
		t.Fatalf("UpsertDefinitionCatalogEntry(refresh) error = %v", err)
	}

	if !row.RuntimePublishedAt.After(backdated) {
		t.Fatalf("expected runtime_published_at to refresh after %s, got %s", backdated, row.RuntimePublishedAt)
	}
	if row.Enabled {
		t.Fatalf("expected upsert to preserve enabled=false, got %+v", row)
	}
}

func TestTouchDefinitionCatalogEntry_UpdatesHeartbeat(t *testing.T) {
	ts := newTestStore(t)

	if _, err := ts.store.UpsertDefinitionCatalogEntry(ts.ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	}); err != nil {
		t.Fatalf("UpsertDefinitionCatalogEntry(initial) error = %v", err)
	}
	backdated := backdateDefinitionCatalogRuntime(t, ts, "checkout", "v1")

	affected, err := ts.store.TouchDefinitionCatalogEntry(ts.ctx, enginedb.TouchDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	if err != nil {
		t.Fatalf("TouchDefinitionCatalogEntry(existing) error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected TouchDefinitionCatalogEntry to affect 1 row, got %d", affected)
	}

	row, err := ts.store.GetDefinitionCatalogEntry(ts.ctx, enginedb.GetDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	if err != nil {
		t.Fatalf("GetDefinitionCatalogEntry() error = %v", err)
	}
	if !row.RuntimePublishedAt.After(backdated) {
		t.Fatalf("expected runtime_published_at to refresh after %s, got %s", backdated, row.RuntimePublishedAt)
	}

	affected, err = ts.store.TouchDefinitionCatalogEntry(ts.ctx, enginedb.TouchDefinitionCatalogEntryParams{
		DefinitionName:    "missing",
		DefinitionVersion: "v1",
	})
	if err != nil {
		t.Fatalf("TouchDefinitionCatalogEntry(missing) error = %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected TouchDefinitionCatalogEntry to affect 0 rows for a missing definition, got %d", affected)
	}
}

func backdateDefinitionCatalogRuntime(
	t *testing.T,
	ts *testStore,
	definitionName string,
	definitionVersion string,
) time.Time {
	t.Helper()

	backdated, err := ts.store.SetDefinitionCatalogRuntimePublishedAt(ts.ctx, enginedb.SetDefinitionCatalogRuntimePublishedAtParams{
		DefinitionName:     definitionName,
		DefinitionVersion:  definitionVersion,
		RuntimePublishedAt: time.Now().Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("backdate definition catalog runtime: %v", err)
	}
	return backdated
}

func setDefinitionCatalogEnabled(
	t *testing.T,
	ts *testStore,
	definitionName string,
	definitionVersion string,
	enabled bool,
) {
	t.Helper()

	affected, err := ts.store.SetDefinitionCatalogEnabled(ts.ctx, enginedb.SetDefinitionCatalogEnabledParams{
		DefinitionName:    definitionName,
		DefinitionVersion: definitionVersion,
		Enabled:           enabled,
	})
	if err != nil {
		t.Fatalf("set definition catalog enabled: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected to update 1 definition catalog row, updated %d", affected)
	}
}
