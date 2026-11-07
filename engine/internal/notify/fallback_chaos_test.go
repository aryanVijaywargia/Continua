package notify_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/activity"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginenotify "github.com/continua-ai/continua/engine/internal/notify"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineworker "github.com/continua-ai/continua/engine/internal/worker"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestPollFallbackProgressesWhenNotificationsSuppressed(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	sideListener := newListeningConn(t, db,
		publicnotify.ChannelRuns,
		publicnotify.ChannelActivity,
		publicnotify.ChannelInbox,
	)

	listener := enginenotify.NewListener(db.Pool, discardLogger())
	runsWake := listener.Subscribe(publicnotify.ChannelRuns)
	inboxWake := listener.Subscribe(publicnotify.ChannelInbox)
	activityWake := listener.Subscribe(publicnotify.ChannelActivity)
	listenerDone := runListener(t, listener)
	waitForListenerHealthy(t, listener, listenerDone, 5*time.Second)
	drainWakes(runsWake)
	drainWakes(inboxWake)
	drainWakes(activityWake)

	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	store := enginestore.New(db.Pool).WithNotifyDisabled()
	definition := fallbackWorkflowDefinition()
	definitions, err := engineworkflow.NewRegistry(definition)
	if err != nil {
		t.Fatalf("workflow.NewRegistry() error = %v", err)
	}
	run := seedFallbackRun(t, store, projectID, definition, "notify-chaos-fallback")

	workflowWorker := engineworkflow.NewWorker(store, definitions, time.Second, discardLogger())
	activityWorker := activity.NewWorker(store, fallbackActivities(t), time.Second, discardLogger())
	loopsCtx, cancelLoops := context.WithCancel(context.Background())
	workflowWake := mergeWakeChannels(loopsCtx, runsWake, inboxWake)
	loopDone := make(chan error, 2)
	go func() {
		loopDone <- engineworker.RunLoopWithWake(
			loopsCtx,
			200*time.Millisecond,
			300*time.Millisecond,
			listener.Healthy,
			workflowWake,
			"notify-chaos-workflow",
			workflowWorker.PollOnce,
		)
	}()
	go func() {
		loopDone <- engineworker.RunLoopWithWake(
			loopsCtx,
			200*time.Millisecond,
			300*time.Millisecond,
			listener.Healthy,
			activityWake,
			"notify-chaos-activity",
			activityWorker.PollOnce,
		)
	}()
	defer func() {
		cancelLoops()
		for range 2 {
			select {
			case err := <-loopDone:
				if err != nil {
					t.Errorf("RunLoopWithWake() cleanup error = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Error("RunLoopWithWake() did not stop after cancellation")
			}
		}
	}()

	completed := waitForFallbackCompletion(t, store, run.ID, 15*time.Second)
	var result fallbackResult
	if err := json.Unmarshal(completed.Result, &result); err != nil {
		t.Fatalf("json.Unmarshal(run.Result) error = %v; raw = %s", err, string(completed.Result))
	}
	if result.CompletedSteps != 2 {
		t.Fatalf("run result completed_steps = %d, want 2", result.CompletedSteps)
	}

	quietCtx, quietCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer quietCancel()
	if notification, err := sideListener.WaitForNotification(quietCtx); err == nil {
		t.Fatalf("observed notification on %q with store notifications disabled", notification.Channel)
	} else if quietCtx.Err() == nil {
		t.Fatalf("side listener error = %v, want quiet timeout", err)
	}
}

func TestPollFallbackDrainsReadyBacklogBackToBack(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)
	store := enginestore.New(db.Pool).WithNotifyDisabled()
	definition := fallbackWorkflowDefinition()
	definitions, err := engineworkflow.NewRegistry(definition)
	if err != nil {
		t.Fatalf("workflow.NewRegistry() error = %v", err)
	}
	const backlog = 100
	for i := range backlog {
		seedFallbackRun(t, store, projectID, definition, fmt.Sprintf("notify-chaos-backlog-%d", i))
	}

	// A healthy listener with no notifications leaves only the fallback interval,
	// so each queue drains in time only if every claim triggers the next poll.
	loopsCtx, cancelLoops := context.WithCancel(context.Background())
	var loops errgroup.Group
	t.Cleanup(func() {
		cancelLoops()
		if err := loops.Wait(); err != nil {
			t.Errorf("RunLoopWithWake() cleanup error = %v", err)
		}
	})
	runLoop := func(ctx context.Context, workerID string, fn engineworker.IterationFunc) error {
		return engineworker.RunLoopWithWake(ctx, time.Minute, time.Minute, func() bool { return true }, nil, workerID, fn)
	}
	waitForQueues := func(runsReady, tasksPending int64) {
		t.Helper()
		var snapshot enginestore.RuntimeMetricsSnapshot
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			snapshot, err = store.SampleRuntimeMetrics(context.Background())
			if err != nil {
				t.Fatalf("SampleRuntimeMetrics() error = %v", err)
			}
			if snapshot.RunsReady == runsReady && snapshot.ActivityTasksPending == tasksPending {
				return
			}
		}
		t.Fatalf("queues after 10s: runs_ready = %d, activity_tasks_pending = %d; want %d, %d",
			snapshot.RunsReady, snapshot.ActivityTasksPending, runsReady, tasksPending)
	}

	// Stop and join the workflow loop before the activity phase, so it cannot
	// claim the runs that activity completions requeue.
	workflowCtx, stopWorkflow := context.WithCancel(loopsCtx)
	workflowDone := make(chan error, 1)
	go func() {
		workflowDone <- runLoop(workflowCtx, "backlog-workflow", engineworkflow.NewWorker(store, definitions, time.Minute, discardLogger()).PollOnce)
	}()
	waitForQueues(0, backlog)
	stopWorkflow()
	if err := <-workflowDone; err != nil {
		t.Fatalf("workflow RunLoopWithWake() error = %v", err)
	}

	loops.Go(func() error {
		return runLoop(loopsCtx, "backlog-activity", activity.NewWorker(store, fallbackActivities(t), time.Minute, discardLogger()).PollOnce)
	})
	waitForQueues(backlog, 0)
}

func fallbackActivities(t *testing.T) *activity.Registry {
	t.Helper()

	activities, err := activity.NewRegistry(map[string]activity.Handler{
		"notifychaos.step": func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			var value fallbackValue
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			return json.Marshal(fallbackValue{Value: value.Value + 1})
		},
	})
	if err != nil {
		t.Fatalf("activity.NewRegistry() error = %v", err)
	}
	return activities
}

type fallbackValue struct {
	Value int `json:"value"`
}

type fallbackResult struct {
	CompletedSteps int `json:"completed_steps"`
}

func fallbackWorkflowDefinition() publicworkflow.Definition {
	return publicworkflow.Definition{
		Name:    "notifychaos.workflow",
		Version: "v1",
		Run: func(ctx publicworkflow.Context) error {
			value := fallbackValue{}
			for step := 1; step <= 2; step++ {
				var next fallbackValue
				if err := ctx.Activity(fmt.Sprintf("step-%d", step), "notifychaos.step", value, &next); err != nil {
					return err
				}
				value = next
			}
			return ctx.SetResult(fallbackResult{CompletedSteps: value.Value})
		},
	}
}

func seedFallbackRun(
	t *testing.T,
	store *enginestore.Store,
	projectID uuid.UUID,
	definition publicworkflow.Definition,
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
	input := json.RawMessage(`{"notifications":"suppressed"}`)
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    definition.Name,
		DefinitionVersion: definition.Version,
		InstanceKey:       instance.InstanceKey,
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

func mergeWakeChannels(ctx context.Context, inputs ...<-chan struct{}) <-chan struct{} {
	out := make(chan struct{}, 64)
	for _, input := range inputs {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-input:
					select {
					case out <- struct{}{}:
					default:
					}
				}
			}
		}()
	}
	return out
}

func waitForFallbackCompletion(
	t *testing.T,
	store *enginestore.Store,
	runID uuid.UUID,
	timeout time.Duration,
) enginedb.EngineRun {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastStatus enginedb.EngineRunLifecycleStatus
	for time.Now().Before(deadline) {
		run, err := store.GetRun(context.Background(), runID)
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
