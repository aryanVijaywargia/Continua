package activity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

// A local activity outcome must lock its run before its task (#352). An activation that
// cancels a run, and a terminate, lock the run and then cancel its open tasks. The test
// plays that side: it holds the run lock until the worker queues behind it, then cancels
// the run's open tasks. The worker must hold no task lock while it waits.
func TestWorkerOutcomeLocksRunBeforeTask(t *testing.T) {
	handlers := map[string]Handler{
		"complete": func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return []byte(`{"ok":true}`), nil
		},
		"fail": func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	}
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			db := enginetest.NewTestDatabase(t)
			store := enginestore.New(db.Pool)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, run, task := createWaitingRunWithPendingActivity(t, store, pendingActivityConfig{
				projectID:   enginetest.DefaultPlatformProjectID,
				instanceKey: "instance-activity-lock-order-" + name,
				activityKey: "lock-order",
				maxAttempts: 1,
			})

			canceller, err := db.Pool.Begin(ctx)
			if err != nil {
				t.Fatalf("Begin() error = %v", err)
			}
			defer func() { _ = canceller.Rollback(context.Background()) }()
			var cancellerPID int32
			if err := canceller.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&cancellerPID); err != nil {
				t.Fatalf("read backend pid: %v", err)
			}
			cancellerQueries := enginedb.New(canceller)
			if _, err := cancellerQueries.GetRunForUpdate(ctx, run.ID); err != nil {
				t.Fatalf("GetRunForUpdate() error = %v", err)
			}

			registry, err := NewRegistry(map[string]Handler{testActivityType: handler})
			if err != nil {
				t.Fatalf("NewRegistry() error = %v", err)
			}
			worker := NewWorker(store, registry, time.Minute, nil)
			var workerErr error
			workerDone := make(chan struct{})
			go func() {
				defer close(workerDone)
				_, workerErr = worker.PollOnce(ctx, "activity-worker")
			}()
			defer func() {
				cancel()
				<-workerDone
			}()
			waitForRunLockQueue(t, ctx, db.Pool, cancellerPID)

			if _, err := cancellerQueries.TransitionRunToTerminated(ctx, run.ID); err != nil {
				t.Fatalf("TransitionRunToTerminated() error = %v", err)
			}
			if _, err := cancellerQueries.CancelOpenActivityTasksByRun(ctx, run.ID); err != nil {
				t.Fatalf("CancelOpenActivityTasksByRun() error = %v", err)
			}
			if err := canceller.Commit(ctx); err != nil {
				t.Fatalf("Commit() error = %v", err)
			}
			select {
			case <-workerDone:
				if workerErr != nil {
					t.Fatalf("PollOnce() error = %v", workerErr)
				}
			case <-ctx.Done():
				t.Fatal("activity worker did not finish after the run was cancelled")
			}

			cancelledTask, err := store.GetActivityTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("GetActivityTask() error = %v", err)
			}
			if cancelledTask.Status != enginedb.EngineActivityTaskStatusCancelled {
				t.Fatalf("task status = %s, want cancelled: the outcome must not land after the run ended", cancelledTask.Status)
			}
		})
	}
}

// waitForRunLockQueue waits until a backend queues behind the holder's locks. Only the
// worker's transaction can, since each test has its own database.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func waitForRunLockQueue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, holderPID int32) {
	t.Helper()
	for {
		var queued bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid)))
		`, holderPID).Scan(&queued); err != nil {
			t.Fatalf("read lock queue behind pid %d: %v", holderPID, err)
		}
		if queued {
			return
		}
	}
}
