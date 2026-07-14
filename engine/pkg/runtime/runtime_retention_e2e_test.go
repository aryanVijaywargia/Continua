package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestRuntimeRetentionGCsTerminalRun_E2E(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	definition := workflow.Definition{
		Name:    "usertest.retention",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			return ctx.SetResult(map[string]bool{"completed": true})
		},
	}
	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{definition},
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 25 * time.Millisecond,
		RetentionTerminalRuns:   time.Millisecond,
		RetentionDedupeGrace:    time.Millisecond,
		RetentionBatchSize:      10,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	ctx := context.Background()
	engineStore := enginestore.New(db.Pool)
	instance, err := engineStore.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "runtime-retention-e2e",
		DefinitionName: definition.Name,
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := engineStore.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: definition.Version,
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	input := mustJSON(t, map[string]string{"purpose": "retention-e2e"})
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    definition.Name,
		DefinitionVersion: definition.Version,
		InstanceKey:       instance.InstanceKey,
		Input:             input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow started) error = %v", err)
	}
	started, err := engineStore.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID: projectID, InstanceID: instance.ID, RunID: run.ID,
		SequenceNo: 1, EventType: enginehistory.EventWorkflowStarted, Payload: startedPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(workflow started) error = %v", err)
	}
	enginetest.SeedProjectionShell(t, db.Pool, &instance, &run, definition.Name, definition.Version, input, started.ID)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stopped := false
	go func() {
		done <- rt.Run(runCtx)
	}()
	defer func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop within 5s after test cleanup cancellation")
		}
	}()

	completed := waitForCompletedRun(t, engineStore, instance.ID)
	deadline := time.Now().Add(30 * time.Second)
	remainingHistory := int64(-1)
	for time.Now().Before(deadline) {
		if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM engine.history WHERE run_id = $1`, completed.ID).Scan(&remainingHistory); err != nil {
			t.Fatalf("count retained history: %v", err)
		}
		if remainingHistory == 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if remainingHistory != 0 {
		t.Fatalf("engine.history rows for terminal run = %d after retention deadline, want 0", remainingHistory)
	}

	var traceID uuid.UUID
	var traceStatus, projectionState string
	if err := db.Pool.QueryRow(ctx, `
		SELECT id, status, COALESCE(engine_projection_state, '')
		FROM public.traces
		WHERE engine_run_id = $1
	`, completed.ID).Scan(&traceID, &traceStatus, &projectionState); err != nil {
		t.Fatalf("query retained projected trace: %v", err)
	}
	if traceStatus != "completed" || projectionState != "journal_expired" {
		t.Fatalf("retained trace status/projection_state = %q/%q, want completed/journal_expired", traceStatus, projectionState)
	}
	var rootSpanExists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM public.spans
			WHERE trace_id = $1 AND depth = 0
		)
	`, traceID).Scan(&rootSpanExists); err != nil {
		t.Fatalf("query retained root span: %v", err)
	}
	if !rootSpanExists {
		t.Fatal("projected root span was deleted by terminal-run retention")
	}

	cancel()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("Runtime.Run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run() did not stop within 5s after cancellation")
	}
}
