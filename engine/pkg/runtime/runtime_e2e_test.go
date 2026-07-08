package runtime_test

import (
	"context"
	"encoding/json"
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

func TestRuntimeRunsUserWorkflowEndToEnd(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)

	workflowDefinition := workflow.Definition{
		Name:    "usertest.greeter",
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			var in struct {
				Name string `json:"name"`
			}
			if err := ctx.Input(&in); err != nil {
				return err
			}

			var out struct {
				Greeting string `json:"greeting"`
			}
			if err := ctx.Activity("greet", "usertest.greet", in, &out); err != nil {
				return err
			}

			return ctx.SetResult(out)
		},
	}
	activities := map[string]engineruntime.ActivityHandler{
		"usertest.greet": func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			return json.Marshal(struct {
				Greeting string `json:"greeting"`
			}{
				Greeting: "hello, " + in.Name,
			})
		},
	}

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{workflowDefinition},
		Activities:              activities,
		ProjectID:               &projectID,
		WorkflowPollInterval:    25 * time.Millisecond,
		ActivityPollInterval:    25 * time.Millisecond,
		MaintenancePollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	ctx := context.Background()
	store := enginestore.New(db.Pool)
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "runtime-e2e",
		DefinitionName: "usertest.greeter",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	input := mustJSON(t, map[string]string{"name": "Ada"})
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    "usertest.greeter",
		DefinitionVersion: "v1",
		InstanceKey:       "runtime-e2e",
		Input:             input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow started) error = %v", err)
	}
	if _, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	}); err != nil {
		t.Fatalf("AppendHistory(workflow started) error = %v", err)
	}

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

	completedRun := waitForCompletedRun(t, store, instance.ID)

	var result struct {
		Greeting string `json:"greeting"`
	}
	if err := json.Unmarshal(completedRun.Result, &result); err != nil {
		t.Fatalf("json.Unmarshal(run.Result) error = %v; raw = %s", err, string(completedRun.Result))
	}
	if result.Greeting != "hello, Ada" {
		t.Fatalf("run result greeting = %q, want %q", result.Greeting, "hello, Ada")
	}

	historyRows, err := store.GetHistoryByRun(ctx, completedRun.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	eventTypes := make([]string, 0, len(historyRows))
	for _, row := range historyRows {
		eventTypes = append(eventTypes, row.EventType)
	}
	wantSubsequence := []string{
		enginehistory.EventWorkflowStarted,
		enginehistory.EventActivityScheduled,
		enginehistory.EventActivityCompleted,
		enginehistory.EventWorkflowCompleted,
	}
	if !containsSubsequence(eventTypes, wantSubsequence) {
		t.Fatalf("history event types = %v, want subsequence %v", eventTypes, wantSubsequence)
	}

	var catalogExists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM engine.definition_catalog
			WHERE definition_name = $1
			  AND definition_version = $2
		)
	`, "usertest.greeter", "v1").Scan(&catalogExists); err != nil {
		t.Fatalf("query definition catalog error = %v", err)
	}
	if !catalogExists {
		t.Fatal("definition catalog missing usertest.greeter@v1")
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

func waitForCompletedRun(t *testing.T, store *enginestore.Store, instanceID uuid.UUID) enginedb.EngineRun {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(20 * time.Second)
	var lastStatus enginedb.EngineRunLifecycleStatus
	for time.Now().Before(deadline) {
		runs, err := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
			InstanceID: instanceID,
			Limit:      1,
		})
		if err != nil {
			t.Fatalf("ListRunsByInstance() error = %v", err)
		}
		if len(runs) > 0 {
			lastStatus = runs[0].Status
			if runs[0].Status == enginedb.EngineRunLifecycleStatusCompleted {
				return runs[0]
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run did not complete within 20s; last observed status = %s", lastStatus)
	return enginedb.EngineRun{}
}

func containsSubsequence(haystack, needle []string) bool {
	if len(needle) == 0 {
		return true
	}
	next := 0
	for _, value := range haystack {
		if value == needle[next] {
			next++
			if next == len(needle) {
				return true
			}
		}
	}
	return false
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
