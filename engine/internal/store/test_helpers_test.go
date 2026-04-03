package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

type testStore struct {
	store *Store
	db    *enginetest.TestDatabase
	ctx   context.Context
}

func newTestStore(t *testing.T) *testStore {
	t.Helper()

	db := enginetest.NewTestDatabase(t)
	return &testStore{
		store: New(db.Pool),
		db:    db,
		ctx:   context.Background(),
	}
}

func (ts *testStore) createInstance(t *testing.T, projectID uuid.UUID, key string) enginedb.EngineInstance {
	t.Helper()

	instance, err := ts.store.CreateInstance(ts.ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    key,
		DefinitionName: "workflow.demo",
		Metadata:       []byte(`{"source":"test"}`),
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	return instance
}

func (ts *testStore) createRun(t *testing.T, instance enginedb.EngineInstance, runNumber int32) enginedb.EngineRun {
	t.Helper()

	run, err := ts.store.CreateRun(ts.ctx, enginedb.CreateRunParams{
		ProjectID:         instance.ProjectID,
		InstanceID:        instance.ID,
		RunNumber:         runNumber,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	return run
}

func (ts *testStore) createHistory(
	t *testing.T,
	projectID uuid.UUID,
	instanceID uuid.UUID,
	runID uuid.UUID,
	sequenceNo int32,
	eventType string,
) enginedb.EngineHistory {
	t.Helper()

	history, err := ts.store.AppendHistory(ts.ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instanceID,
		RunID:      runID,
		SequenceNo: sequenceNo,
		EventType:  eventType,
		Payload:    []byte(`{"event":"` + eventType + `"}`),
	})
	if err != nil {
		t.Fatalf("AppendHistory() error = %v", err)
	}
	return history
}
