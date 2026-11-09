package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/activity"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

const finishedRunActivityType = "finished.run.work"

// A run that completes or fails while one of its activities is still open cancels that
// task and discards its open inbox items in the same transaction (#359). The local
// worker that still runs the task then gets the stale result for every outcome: the
// task stays cancelled, it is not queued again, and the finished run gets no history.
//
// The run first waits on the activity, and the worker claims it. A signal then wakes
// the run, and the next activation ends it: "failed" because the definition version is
// gone (a deploy dropped it), "completed" because the workflow code swallows the
// engine's wait for the activity.
func TestFinishedRunCancelsOpenWork(t *testing.T) {
	endings := []struct {
		name        string
		definitions []publicworkflow.Definition
		status      enginedb.EngineRunLifecycleStatus
		event       string
	}{
		{
			name: "completed",
			definitions: []publicworkflow.Definition{{
				Name:    "finished-run",
				Version: "v1",
				Run: func(ctx publicworkflow.Context) error {
					func() {
						defer func() { _ = recover() }()
						_ = ctx.Activity("work", finishedRunActivityType, nil, nil)
					}()
					return nil
				},
			}},
			status: enginedb.EngineRunLifecycleStatusCompleted,
			event:  enginehistory.EventWorkflowCompleted,
		},
		{
			name:   "failed",
			status: enginedb.EngineRunLifecycleStatusFailed,
			event:  enginehistory.EventWorkflowFailed,
		},
	}
	outcomes := []struct {
		name        string
		err         error
		maxAttempts int32
	}{
		{name: "complete", maxAttempts: 1},
		{name: "fail", err: errors.New("boom"), maxAttempts: 1},
		{name: "retry", err: errors.New("try again"), maxAttempts: 3},
	}
	for _, ending := range endings {
		for _, outcome := range outcomes {
			t.Run(ending.name+"/"+outcome.name, func(t *testing.T) {
				db := enginetest.NewTestDatabase(t)
				store := enginestore.New(db.Pool)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				_, run := createStartedRun(t, store, workflowTestCase{
					projectID:         enginetest.DefaultPlatformProjectID,
					instanceKey:       "instance-finished-run-" + ending.name + "-" + outcome.name,
					definitionName:    "finished-run",
					definitionVersion: "v1",
				})
				waitOnActivity := NewActivator(store, mustRegistry(t, publicworkflow.Definition{
					Name:    "finished-run",
					Version: "v1",
					Run: func(ctx publicworkflow.Context) error {
						return ctx.ActivityWithOptions("work", finishedRunActivityType, nil, nil, publicworkflow.ActivityOptions{
							RetryPolicy: &publicworkflow.RetryPolicy{
								MaxAttempts:       int(outcome.maxAttempts),
								InitialBackoff:    time.Millisecond,
								MaxBackoff:        time.Millisecond,
								BackoffMultiplier: 1,
							},
						})
					},
				}))
				activateNextRun(t, ctx, store, waitOnActivity, "workflow-worker")

				started := make(chan struct{})
				release := make(chan struct{})
				registry, err := activity.NewRegistry(map[string]activity.Handler{
					finishedRunActivityType: func(context.Context, json.RawMessage) (json.RawMessage, error) {
						close(started)
						<-release
						if outcome.err != nil {
							return nil, outcome.err
						}
						return json.RawMessage(`{"ok":true}`), nil
					},
				})
				if err != nil {
					t.Fatalf("activity.NewRegistry() error = %v", err)
				}
				worker := activity.NewWorker(store, registry, time.Minute, nil)
				var workerErr error
				workerDone := make(chan struct{})
				go func() {
					defer close(workerDone)
					_, workerErr = worker.PollOnce(ctx, "activity-worker")
				}()
				select {
				case <-started:
				case <-workerDone:
					t.Fatalf("activity worker did not run the task: %v", workerErr)
				case <-ctx.Done():
					t.Fatal("activity worker did not claim the task")
				}

				// A signal the workflow has not received yet wakes the run.
				signal, err := enginehistory.MarshalPayload(enginehistory.SignalReceivedPayload{SignalName: "nudge"})
				if err != nil {
					t.Fatalf("MarshalPayload(signal) error = %v", err)
				}
				if _, err := store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
					ProjectID:   run.ProjectID,
					InstanceID:  run.InstanceID,
					RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
					Kind:        "signal",
					Payload:     signal,
					AvailableAt: time.Now().Add(-time.Second),
				}); err != nil {
					t.Fatalf("CreateInboxItem(signal) error = %v", err)
				}
				if _, err := store.WakeWaitingRun(ctx, run.ID); err != nil {
					t.Fatalf("WakeWaitingRun() error = %v", err)
				}
				activateNextRun(t, ctx, store, NewActivator(store, mustRegistry(t, ending.definitions...)), "workflow-worker")

				finished, err := store.GetRun(ctx, run.ID)
				if err != nil {
					t.Fatalf("GetRun() error = %v", err)
				}
				if finished.Status != ending.status {
					t.Fatalf("run status = %s, want %s", finished.Status, ending.status)
				}
				tasks, err := store.ListActivityTasksByRun(ctx, run.ID)
				if err != nil {
					t.Fatalf("ListActivityTasksByRun() error = %v", err)
				}
				if len(tasks) != 1 {
					t.Fatalf("activity tasks = %d, want 1", len(tasks))
				}
				if tasks[0].Status != enginedb.EngineActivityTaskStatusCancelled {
					t.Fatalf("task status after the run finished = %s, want cancelled", tasks[0].Status)
				}
				task := tasks[0]
				var openInbox int
				if err := db.Pool.QueryRow(ctx, `
					SELECT COUNT(*)
					FROM engine.inbox
					WHERE run_id = $1
					  AND status IN ('pending', 'claimed')
				`, run.ID).Scan(&openInbox); err != nil {
					t.Fatalf("count open inbox items: %v", err)
				}
				if openInbox != 0 {
					t.Fatalf("open inbox items = %d, want 0 after the run finished", openInbox)
				}
				if _, err := store.HeartbeatLocalActivityTask(ctx, task.ID, "activity-worker"); !errors.Is(err, enginestore.ErrStaleClaim) {
					t.Fatalf("HeartbeatLocalActivityTask() error = %v, want ErrStaleClaim", err)
				}
				historyAtFinish, err := store.GetHistoryByRun(ctx, run.ID)
				if err != nil {
					t.Fatalf("GetHistoryByRun() error = %v", err)
				}
				assertHistoryEndsWith(t, historyAtFinish, ending.event)

				close(release)
				select {
				case <-workerDone:
				case <-ctx.Done():
					t.Fatal("activity worker did not finish")
				}
				if workerErr != nil {
					t.Fatalf("PollOnce() error = %v", workerErr)
				}

				reported, err := store.GetActivityTask(ctx, task.ID)
				if err != nil {
					t.Fatalf("GetActivityTask() error = %v", err)
				}
				if reported.Status != enginedb.EngineActivityTaskStatusCancelled {
					t.Fatalf("task status after the late %s report = %s, want cancelled", outcome.name, reported.Status)
				}
				historyAfterReport, err := store.GetHistoryByRun(ctx, run.ID)
				if err != nil {
					t.Fatalf("GetHistoryByRun() error = %v", err)
				}
				if got, want := historyEventTypes(historyAfterReport), historyEventTypes(historyAtFinish); !slices.Equal(got, want) {
					t.Fatalf("history after the late %s report = %v, want %v", outcome.name, got, want)
				}
			})
		}
	}
}
