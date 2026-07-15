package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
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

const notifyLatencyActivityType = "notifytest.step"

type notifyLatencyValue struct {
	Value int `json:"value"`
}

type notifyLatencyResult struct {
	CompletedSteps int `json:"completed_steps"`
}

func TestRuntimeNotifyLatencyE2E(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	definition := notifyLatencyWorkflow("notifytest.three-steps", 3)

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:             db.DatabaseURL,
		Workflows:               []workflow.Definition{definition},
		Activities:              notifyLatencyActivities(),
		ProjectID:               &projectID,
		WorkflowPollInterval:    30 * time.Second,
		ActivityPollInterval:    30 * time.Second,
		MaintenancePollInterval: 30 * time.Second,
		NotifyFallbackInterval:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	store := enginestore.New(db.Pool)
	run := seedNotifyLatencyRun(t, store, projectID, definition, "notify-latency-three")
	cancel, done := runNotifyRuntime(t, rt)
	defer stopNotifyRuntime(t, cancel, done)

	completed := waitForNotifyRun(t, store, run.ID, 10*time.Second)
	var result notifyLatencyResult
	if err := json.Unmarshal(completed.Result, &result); err != nil {
		t.Fatalf("json.Unmarshal(run.Result) error = %v; raw = %s", err, string(completed.Result))
	}
	if result.CompletedSteps != 3 {
		t.Fatalf("run result completed_steps = %d, want 3", result.CompletedSteps)
	}
}

func TestRuntimeDisableNotifyStillCompletes(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	definition := notifyLatencyWorkflow("notifytest.disabled", 1)

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL:          db.DatabaseURL,
		Workflows:            []workflow.Definition{definition},
		Activities:           notifyLatencyActivities(),
		ProjectID:            &projectID,
		DisableNotify:        true,
		WorkflowPollInterval: 200 * time.Millisecond,
		ActivityPollInterval: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	store := enginestore.New(db.Pool)
	run := seedNotifyLatencyRun(t, store, projectID, definition, "notify-disabled")
	cancel, done := runNotifyRuntime(t, rt)
	defer stopNotifyRuntime(t, cancel, done)

	completed := waitForNotifyRun(t, store, run.ID, 15*time.Second)
	var result notifyLatencyResult
	if err := json.Unmarshal(completed.Result, &result); err != nil {
		t.Fatalf("json.Unmarshal(run.Result) error = %v; raw = %s", err, string(completed.Result))
	}
	if result.CompletedSteps != 1 {
		t.Fatalf("run result completed_steps = %d, want 1", result.CompletedSteps)
	}
}

func notifyLatencyWorkflow(name string, steps int) workflow.Definition {
	return workflow.Definition{
		Name:    name,
		Version: "v1",
		Run: func(ctx workflow.Context) error {
			value := notifyLatencyValue{}
			for step := 1; step <= steps; step++ {
				var next notifyLatencyValue
				if err := ctx.Activity(fmt.Sprintf("step-%d", step), notifyLatencyActivityType, value, &next); err != nil {
					return err
				}
				value = next
			}
			return ctx.SetResult(notifyLatencyResult{CompletedSteps: value.Value})
		},
	}
}

func notifyLatencyActivities() map[string]engineruntime.ActivityHandler {
	return map[string]engineruntime.ActivityHandler{
		notifyLatencyActivityType: func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			var input notifyLatencyValue
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			return json.Marshal(notifyLatencyValue{Value: input.Value + 1})
		},
	}
}

func seedNotifyLatencyRun(
	t *testing.T,
	store *enginestore.Store,
	projectID uuid.UUID,
	definition workflow.Definition,
	instanceKey string,
) enginedb.EngineRun {
	t.Helper()

	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    instanceKey,
		DefinitionName: definition.Name,
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: definition.Version,
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	input := json.RawMessage(`{"seeded":true}`)
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    definition.Name,
		DefinitionVersion: definition.Version,
		InstanceKey:       instanceKey,
		Input:             input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow started) error = %v", err)
	}
	started, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(workflow started) error = %v", err)
	}
	enginetest.SeedProjectionShell(t, store.Pool(), &instance, &run, definition.Name, definition.Version, input, started.ID)
	return run
}

func runNotifyRuntime(t *testing.T, rt *engineruntime.Runtime) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rt.Run(ctx) }()
	return cancel, done
}

func stopNotifyRuntime(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Runtime.Run() after cancellation error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Runtime.Run() did not stop within 5s after cancellation")
	}
}

func waitForNotifyRun(t *testing.T, store *enginestore.Store, runID uuid.UUID, timeout time.Duration) enginedb.EngineRun {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	var lastStatus enginedb.EngineRunLifecycleStatus
	for time.Now().Before(deadline) {
		run, err := store.GetRun(ctx, runID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		lastStatus = run.Status
		if run.Status == enginedb.EngineRunLifecycleStatusCompleted {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run did not complete within %s; last observed status = %s", timeout, lastStatus)
	return enginedb.EngineRun{}
}
