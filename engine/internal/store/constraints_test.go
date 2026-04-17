package store

import (
	"errors"
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestConstraintViolationsMapToErrAlreadyExists(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-constraints")
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "run.started")

	_, err := ts.store.CreateInstance(ts.ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    instance.InstanceKey,
		DefinitionName: "workflow.demo",
		Metadata:       []byte(`{}`),
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected instance uniqueness error, got %v", err)
	}

	_, err = ts.store.CreateRun(ts.ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         run.RunNumber,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now(),
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected run uniqueness error, got %v", err)
	}

	_, err = ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "activity-1",
		ActivityType:    "email.send",
		Input:           []byte(`{"to":"user@example.com"}`),
		AvailableAt:     time.Now(),
		ExecutionTarget: "local",
		MaxAttempts:     1,
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}

	_, err = ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		HistoryID:       &history.ID,
		ActivityKey:     "activity-1",
		ActivityType:    "email.send",
		Input:           []byte(`{"to":"user@example.com"}`),
		AvailableAt:     time.Now(),
		ExecutionTarget: "local",
		MaxAttempts:     1,
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected activity uniqueness error, got %v", err)
	}

	_, err = ts.store.CreateRequestDedupe(ts.ctx, enginedb.CreateRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: "start-workflow",
		RequestKey:   "req-1",
		InstanceID:   enginetest.NullableUUID(instance.ID),
		RunID:        enginetest.NullableUUID(run.ID),
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateRequestDedupe() error = %v", err)
	}

	_, err = ts.store.CreateRequestDedupe(ts.ctx, enginedb.CreateRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: "start-workflow",
		RequestKey:   "req-1",
		InstanceID:   enginetest.NullableUUID(instance.ID),
		RunID:        enginetest.NullableUUID(run.ID),
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected request dedupe uniqueness error, got %v", err)
	}

	dedupeKey := "signal:dedupe"
	_, err = ts.store.CreateInboxItem(ts.ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  instance.ID,
		RunID:       enginetest.NullableUUID(run.ID),
		HistoryID:   &history.ID,
		Kind:        "signal",
		Payload:     []byte(`{"name":"wake"}`),
		AvailableAt: time.Now(),
		DedupeKey:   &dedupeKey,
	})
	if err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}

	_, err = ts.store.CreateInboxItem(ts.ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  instance.ID,
		RunID:       enginetest.NullableUUID(run.ID),
		HistoryID:   &history.ID,
		Kind:        "signal",
		Payload:     []byte(`{"name":"wake"}`),
		AvailableAt: time.Now(),
		DedupeKey:   &dedupeKey,
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected inbox dedupe uniqueness error, got %v", err)
	}
}
