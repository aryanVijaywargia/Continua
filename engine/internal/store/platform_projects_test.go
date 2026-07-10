package store

import (
	"testing"

	"github.com/google/uuid"

	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestPlatformProjectExists(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuid.New()
	missingProjectID := uuid.New()

	exists, err := ts.store.PlatformProjectExists(ts.ctx, projectID)
	if err != nil {
		t.Fatalf("PlatformProjectExists() missing initial error = %v", err)
	}
	if exists {
		t.Fatal("PlatformProjectExists() returned true before project was inserted")
	}

	enginetest.EnsurePlatformProject(t, ts.db.Pool, projectID)

	exists, err = ts.store.PlatformProjectExists(ts.ctx, projectID)
	if err != nil {
		t.Fatalf("PlatformProjectExists() existing error = %v", err)
	}
	if !exists {
		t.Fatal("PlatformProjectExists() returned false for existing project")
	}

	exists, err = ts.store.PlatformProjectExists(ts.ctx, missingProjectID)
	if err != nil {
		t.Fatalf("PlatformProjectExists() missing error = %v", err)
	}
	if exists {
		t.Fatal("PlatformProjectExists() returned true for unrelated missing project")
	}
}
