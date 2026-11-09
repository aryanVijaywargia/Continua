package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

// A cancelled run's cascade asks each active child to cancel. Only maintenance wakes runs
// on its own, and only for due timers, so the cascade must also wake a child that waits,
// or the child never sees the cancel (#355).

// abandoningDefinition starts the child "work" and, once its wait on the child fails,
// waits for a signal with the child still active. It ends cancelled when asked to.
func abandoningDefinition() publicworkflow.Definition {
	return publicworkflow.Definition{Name: "abandoning", Version: "v1", Run: func(ctx publicworkflow.Context) error {
		waitErr := ctx.ChildWorkflow("work", "worker", "v1", nil, nil)
		if ctx.CancellationRequested() {
			return publicworkflow.ErrCancelled
		}
		if waitErr == nil {
			return errors.New("child workflow ended before its wait failed")
		}
		return ctx.ReceiveSignal("finish", nil)
	}}
}

// grandchildWaitingWorker starts the child "leaf" and waits on it.
func grandchildWaitingWorker(ctx publicworkflow.Context) error {
	if ctx.CancellationRequested() {
		return publicworkflow.ErrCancelled
	}
	return ctx.ChildWorkflow("leaf", "leaf", "v1", nil, nil)
}

func cascadeWakeActivator(t *testing.T, store *enginestore.Store, worker func(publicworkflow.Context) error) *Activator {
	t.Helper()
	return NewActivator(store, mustRegistry(t,
		continuingDefinition(),
		ordersDefinition(),
		abandoningDefinition(),
		publicworkflow.Definition{Name: "worker", Version: "v1", Run: worker},
		publicworkflow.Definition{Name: "leaf", Version: "v1", Run: cancellableWorker},
	))
}

// runReadyRuns claims and activates runs through the worker claim path until none is
// ready, the way the workflow worker loop does.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func runReadyRuns(t *testing.T, ctx context.Context, store *enginestore.Store, activator *Activator) {
	t.Helper()
	for range 20 {
		claimed, err := store.ClaimNextRun(ctx, "worker-loop", time.Minute)
		if errors.Is(err, enginestore.ErrNotFound) {
			return
		}
		if err != nil {
			t.Fatalf("ClaimNextRun() error = %v", err)
		}
		if err := activator.Activate(ctx, &claimed); err != nil {
			t.Fatalf("Activate(%s) error = %v", claimed.ID, err)
		}
	}
	t.Fatal("runs were still ready after 20 activations")
}

// requestCancelAndClaim asks a waiting run to cancel the way the API does (a cancel inbox
// item and a wake), and claims it through the worker claim path.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func requestCancelAndClaim(t *testing.T, ctx context.Context, store *enginestore.Store, runID uuid.UUID) enginedb.EngineRun {
	t.Helper()
	requestCancel(t, ctx, store, runID)
	woken, err := store.WakeWaitingRun(ctx, runID)
	if err != nil || !woken.Applied {
		t.Fatalf("WakeWaitingRun(%s) = applied %v, error %v; want applied", runID, woken.Applied, err)
	}
	claimed, err := store.ClaimNextRun(ctx, "worker-cancel", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if claimed.ID != runID {
		t.Fatalf("ClaimNextRun() claimed run %s, want %s", claimed.ID, runID)
	}
	return claimed
}

//nolint:revive // Keep testing.T first in test helper signatures.
func renameRunTrace(t *testing.T, ctx context.Context, pool *pgxpool.Pool, runID uuid.UUID, traceID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE public.traces SET trace_id = $2 WHERE engine_run_id = $1`, runID, traceID); err != nil {
		t.Fatalf("rename trace of run %s: %v", runID, err)
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func assertRunWaiting(t *testing.T, ctx context.Context, store *enginestore.Store, runID uuid.UUID) {
	t.Helper()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", runID, err)
	}
	if run.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("run %s has status %q, want waiting", runID, run.Status)
	}
}

// newAbandonedChildFixture leaves an "abandoning" run waiting for a signal while its own
// child "work" is still active and waits for a signal too. It returns both runs.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func newAbandonedChildFixture(t *testing.T, ctx context.Context) (pool *pgxpool.Pool, store *enginestore.Store, runID, childRunID uuid.UUID) {
	t.Helper()
	db := enginetest.NewTestDatabase(t)
	pool = newLockOrderPool(t, ctx, db.DatabaseURL)
	store = enginestore.New(pool)
	activator := cascadeWakeActivator(t, store, cancellableWorker)
	_, run := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-abandoning",
		definitionName:    "abandoning",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{}),
	})
	activateRunByID(t, ctx, pool, store, activator, run.ID, "worker-start")
	childRunID = activeChildRunID(t, ctx, pool, run.ID)
	// At the continuation-follow limit the run's wait on its child fails, and it moves on
	// to wait for a signal while the child is still active.
	if _, err := pool.Exec(ctx, `UPDATE engine.child_workflows SET continuation_count = $2 WHERE current_child_run_id = $1`,
		childRunID, maxContinuationFollowDepth); err != nil {
		t.Fatalf("set continuation count: %v", err)
	}
	activateRunByID(t, ctx, pool, store, activator, run.ID, "worker-wait-failed")
	runReadyRuns(t, ctx, store, activator)
	assertRunWaiting(t, ctx, store, run.ID)
	assertRunWaiting(t, ctx, store, childRunID)
	return pool, store, run.ID, childRunID
}

// A child that waits on a signal (or on its own child) ends up waiting forever unless the
// cascade wakes it. The woken child's projected status must change in the same
// transaction, and its next activation, through the normal claim loop, sees the cancel.
func TestActivatorCancelCascadeWakesWaitingChild(t *testing.T) {
	testCases := []struct {
		name string
		// setup returns the run to cancel and its active child, both waiting.
		setup func(t *testing.T, ctx context.Context) (*enginestore.Store, *Activator, uuid.UUID, uuid.UUID)
		// childWaitsOnGrandchild: a run that waits on its own child records the cancel
		// and keeps waiting, as it does for a cancel through the API.
		childWaitsOnGrandchild bool
	}{
		{
			name: "child of the cancelled run",
			setup: func(t *testing.T, ctx context.Context) (*enginestore.Store, *Activator, uuid.UUID, uuid.UUID) {
				_, store, runID, childRunID := newAbandonedChildFixture(t, ctx)
				return store, cascadeWakeActivator(t, store, cancellableWorker), runID, childRunID
			},
		},
		{
			name: "child of an earlier run, root",
			setup: func(t *testing.T, ctx context.Context) (*enginestore.Store, *Activator, uuid.UUID, uuid.UUID) {
				f := newEarlierRunChildFixture(t, ctx, false, cancellableWorker)
				activator := cascadeWakeActivator(t, f.store, cancellableWorker)
				runReadyRuns(t, ctx, f.store, activator)
				return f.store, activator, f.currentRunID, f.childRunID
			},
		},
		{
			name: "child of an earlier run, nested",
			setup: func(t *testing.T, ctx context.Context) (*enginestore.Store, *Activator, uuid.UUID, uuid.UUID) {
				f := newEarlierRunChildFixture(t, ctx, true, cancellableWorker)
				activator := cascadeWakeActivator(t, f.store, cancellableWorker)
				runReadyRuns(t, ctx, f.store, activator)
				return f.store, activator, f.currentRunID, f.childRunID
			},
		},
		{
			name: "child waits on a grandchild",
			setup: func(t *testing.T, ctx context.Context) (*enginestore.Store, *Activator, uuid.UUID, uuid.UUID) {
				f := newEarlierRunChildFixture(t, ctx, false, grandchildWaitingWorker)
				activator := cascadeWakeActivator(t, f.store, grandchildWaitingWorker)
				runReadyRuns(t, ctx, f.store, activator)
				return f.store, activator, f.currentRunID, f.childRunID
			},
			childWaitsOnGrandchild: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			store, activator, runID, childRunID := testCase.setup(t, ctx)
			assertRunWaiting(t, ctx, store, childRunID)

			claimed := requestCancelAndClaim(t, ctx, store, runID)
			if err := activator.Activate(ctx, &claimed); err != nil {
				t.Fatalf("Activate(cancel) error = %v", err)
			}
			assertRunCancelled(t, ctx, store, runID)
			assertRunQueuedAndProjected(t, ctx, store, childRunID)

			runReadyRuns(t, ctx, store, activator)
			if !testCase.childWaitsOnGrandchild {
				assertRunCancelled(t, ctx, store, childRunID)
				return
			}
			assertRunWaiting(t, ctx, store, childRunID)
			var pendingCancels, recordedCancels int
			if err := store.Pool().QueryRow(ctx, `
				SELECT
				    (SELECT count(*) FROM engine.inbox WHERE run_id = $1 AND kind = 'cancel' AND status = 'pending'),
				    (SELECT count(*) FROM engine.history WHERE run_id = $1 AND event_type = $2)
			`, childRunID, enginehistory.EventCancelRequested).Scan(&pendingCancels, &recordedCancels); err != nil {
				t.Fatalf("read child cancel state: %v", err)
			}
			if pendingCancels != 0 || recordedCancels != 1 {
				t.Fatalf("child has %d pending and %d recorded cancels, want 0 and 1", pendingCancels, recordedCancels)
			}
		})
	}
}

// The cascade writes the woken child's trace. Real ingest takes the child's and the
// cancelled run's traces in byte order, so the cascade must lock the child's trace in the
// same ordered statement as the other traces, after every run lock, or the two deadlock
// (40P01). Both trace orders are covered, for a cancelled root run and for a cancelled
// run that wakes its own parent.
func TestActivatorCancelCascadeWakeQueuesBehindIngestInTraceOrder(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, childTraceFirst := range []bool{true, false} {
			name := "root"
			if nested {
				name = "nested"
			}
			if childTraceFirst {
				name += ", child trace first"
			} else {
				name += ", cancelled run trace first"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				f := newEarlierRunChildFixture(t, ctx, nested, cancellableWorker)
				activator := cascadeWakeActivator(t, f.store, cancellableWorker)
				runReadyRuns(t, ctx, f.store, activator)
				assertRunWaiting(t, ctx, f.store, f.childRunID)

				childTrace, runTrace := "engine:1-child", "engine:0-cancelled"
				if childTraceFirst {
					childTrace, runTrace = "engine:0-child", "engine:1-cancelled"
				}
				renameRunTrace(t, ctx, f.pool, f.childRunID, childTrace)
				renameRunTrace(t, ctx, f.pool, f.currentRunID, runTrace)
				if nested {
					renameRunTrace(t, ctx, f.pool, f.rootRunID, "engine:2-root")
				}
				claimed := requestCancelAndClaim(t, ctx, f.store, f.currentRunID)

				// The gate holds the trace ingest locks first, so ingest holds nothing while it
				// waits.
				gate, gatePID := lockTraceInGate(t, ctx, f.pool, min(childTrace, runTrace))
				batch := startIngestBatch(t, ctx, f.pool, childTrace, runTrace)
				waitForLockQueue(t, ctx, f.pool, gatePID, batch)
				activation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &claimed) })
				waitForLockQueue(t, ctx, f.pool, gatePID, batch, activation)

				if err := gate.Rollback(ctx); err != nil {
					t.Fatalf("release gate: %v", err)
				}
				if err := batch.wait(ctx); err != nil {
					t.Fatalf("ProcessBatch() error = %v", err)
				}
				if err := activation.wait(ctx); err != nil {
					t.Fatalf("Activate(cancel) error = %v", err)
				}
				assertRunCancelled(t, ctx, f.store, f.currentRunID)
				assertRunQueuedAndProjected(t, ctx, f.store, f.childRunID)
				runReadyRuns(t, ctx, f.store, activator)
				assertRunCancelled(t, ctx, f.store, f.childRunID)
			})
		}
	}
}

// The child's own activation holds the child's parent run and its own run while it
// commits a wait. The cancel activation's cascade queues behind it, and must then see the
// committed wait and wake the child: no wake may be lost between the child's wait and the
// cascade.
func TestActivatorCancelCascadeWakesChildThatCommitsWaitMeanwhile(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "root"
		if nested {
			name = "nested"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			f := newEarlierRunChildFixture(t, ctx, nested, cancellableWorker)
			activator := cascadeWakeActivator(t, f.store, cancellableWorker)
			renameRunTrace(t, ctx, f.pool, f.childRunID, "engine:child-355")

			// The gate pauses the child's activation at its projected trace write, after it
			// decided to wait for a signal and while it holds the earlier run and its own run.
			childClaim, err := f.store.ClaimNextRun(ctx, "worker-child", time.Minute)
			if err != nil || childClaim.ID != f.childRunID {
				t.Fatalf("ClaimNextRun() = run %s, error %v; want the child run %s", childClaim.ID, err, f.childRunID)
			}
			gate, gatePID := lockTraceInGate(t, ctx, f.pool, "engine:child-355")
			childActivation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &childClaim) })
			waitForLockQueue(t, ctx, f.pool, gatePID, childActivation)

			cancelClaim := requestCancelAndClaim(t, ctx, f.store, f.currentRunID)
			cancelActivation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &cancelClaim) })
			waitForLockQueue(t, ctx, f.pool, gatePID, childActivation, cancelActivation)
			if childActivation.finished() || cancelActivation.finished() {
				t.Fatal("an activation finished before the gate opened; the test no longer orders the child's wait before the cascade")
			}

			if err := gate.Rollback(ctx); err != nil {
				t.Fatalf("release gate: %v", err)
			}
			if err := childActivation.wait(ctx); err != nil {
				t.Fatalf("Activate(child) error = %v", err)
			}
			if err := cancelActivation.wait(ctx); err != nil {
				t.Fatalf("Activate(cancel) error = %v", err)
			}
			assertRunCancelled(t, ctx, f.store, f.currentRunID)
			assertRunQueuedAndProjected(t, ctx, f.store, f.childRunID)
			runReadyRuns(t, ctx, f.store, activator)
			assertRunCancelled(t, ctx, f.store, f.childRunID)
		})
	}
}

// A writer that locks only the child run (a signal, a timer wake, a cancel through the
// API) then writes the child's trace. The cascade must lock the waiting child's run
// before any trace, or it holds the child's trace while it waits for the child's run and
// the two deadlock (40P01). The writer rolls back, so the child still waits and the
// cascade must wake it.
func TestActivatorCancelCascadeLocksWaitingChildRunBeforeTraces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newEarlierRunChildFixture(t, ctx, false, cancellableWorker)
	activator := cascadeWakeActivator(t, f.store, cancellableWorker)
	runReadyRuns(t, ctx, f.store, activator)
	claimed := requestCancelAndClaim(t, ctx, f.store, f.currentRunID)

	writer, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin child writer: %v", err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	var writerPID int32
	if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatalf("child writer pid: %v", err)
	}
	if _, err := writer.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR NO KEY UPDATE`, f.childRunID); err != nil {
		t.Fatalf("lock child run: %v", err)
	}

	activation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &claimed) })
	waitForLockQueue(t, ctx, f.pool, writerPID, activation)
	if _, err := writer.Exec(ctx, `UPDATE public.traces SET updated_at = NOW() WHERE engine_run_id = $1`, f.childRunID); err != nil {
		t.Fatalf("child writer lost a lock-order conflict on the child's trace: %v", err)
	}
	if err := writer.Rollback(ctx); err != nil {
		t.Fatalf("release child writer: %v", err)
	}
	if err := activation.wait(ctx); err != nil {
		t.Fatalf("Activate(cancel) error = %v", err)
	}
	assertRunCancelled(t, ctx, f.store, f.currentRunID)
	assertRunQueuedAndProjected(t, ctx, f.store, f.childRunID)
	runReadyRuns(t, ctx, f.store, activator)
	assertRunCancelled(t, ctx, f.store, f.childRunID)
}
