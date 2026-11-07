package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/activity"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
	"github.com/continua-ai/continua/internal/ingest"
	platformstore "github.com/continua-ai/continua/internal/store"
)

// The parent's trace id sorts before the child's in byte order, which platform ingest
// uses, but after it under a linguistic collation, so lock queries must compare bytes.
const (
	parentWakeParentTraceID = "engine:Parent"
	parentWakeChildTraceID  = "engine:child"
)

type parentWakeFixture struct {
	db         *enginetest.TestDatabase
	store      *enginestore.Store
	activator  *Activator
	parentRun  enginedb.EngineRun
	childRunID uuid.UUID
}

// newParentWakeFixture leaves a parent run waiting on its queued child run, with both
// projected traces renamed so the parent's trace comes first in ingest's lock order.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func newParentWakeFixture(t *testing.T, ctx context.Context, childRun func(publicworkflow.Context) error) parentWakeFixture {
	t.Helper()

	db := enginetest.NewTestDatabase(t)
	db.Pool = newLockOrderPool(t, ctx, db.DatabaseURL)
	store := enginestore.New(db.Pool)
	_, parentRun := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-parent-wake",
		definitionName:    "checkout-parent-wake",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{"order_id": "ord-parent-wake"}),
	})
	activator := NewActivator(store, mustRegistry(t,
		childWaitingParentDefinition("checkout-parent-wake", nil),
		publicworkflow.Definition{Name: "billing", Version: "v1", Run: childRun},
	))
	activateNextRun(t, ctx, store, activator, "worker-parent")
	_, childWorkflow := assertParentWaitingOnChild(t, ctx, store, parentRun.ID)

	for runID, traceID := range map[uuid.UUID]string{
		parentRun.ID:                    parentWakeParentTraceID,
		childWorkflow.CurrentChildRunID: parentWakeChildTraceID,
	} {
		if _, err := db.Pool.Exec(ctx, `UPDATE public.traces SET trace_id = $2 WHERE engine_run_id = $1`, runID, traceID); err != nil {
			t.Fatalf("rename trace of run %s: %v", runID, err)
		}
	}
	return parentWakeFixture{
		db:         db,
		store:      store,
		activator:  activator,
		parentRun:  parentRun,
		childRunID: childWorkflow.CurrentChildRunID,
	}
}

// newLockOrderPool returns a pool large enough for the lock-order tests. They hold a gate
// transaction and up to three queued operations at once, and still need a connection for
// the pg_blocking_pids query. pgxpool defaults to max(4, NumCPU) connections, so on a
// 4-CPU CI runner the queued operations took every connection and the query timed out.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func newLockOrderPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	cfg.MaxConns = 16
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect lock-order pool: %v", err)
	}
	// Registered after the test database's cleanup, so it closes before the drop.
	t.Cleanup(pool.Close)
	return pool
}

// A child activation that wakes its parent writes both projected traces. Real ingest
// holds the parent's trace, which sorts first, and then asks for the child's. The
// activation must not hold the child's trace while it waits for the parent's, or the two
// deadlock (40P01). The woken parent's projected status must change in the same
// transaction as the wake (#348).
func TestActivatorParentWakeQueuesBehindIngestInTraceOrder(t *testing.T) {
	testCases := []struct {
		name              string
		childRun          func(publicworkflow.Context) error
		continuationCount int32
	}{
		{
			name: "child completed",
			childRun: func(ctx publicworkflow.Context) error {
				return ctx.SetResult(map[string]string{"status": "charged"})
			},
		},
		{
			name: "child failed",
			childRun: func(publicworkflow.Context) error {
				return codedWorkflowError{code: "card_declined", message: "card declined"}
			},
		},
		{
			name: "child cancelled",
			childRun: func(publicworkflow.Context) error {
				return publicworkflow.ErrCancelled
			},
		},
		{
			name: "continuation depth reached",
			childRun: func(publicworkflow.Context) error {
				return publicworkflow.ContinueAsNew(map[string]any{"continued": true})
			},
			continuationCount: maxContinuationFollowDepth - 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			fixture := newParentWakeFixture(t, ctx, testCase.childRun)
			pool := fixture.db.Pool
			if _, err := pool.Exec(ctx, `UPDATE engine.child_workflows SET continuation_count = $2 WHERE current_child_run_id = $1`,
				fixture.childRunID, testCase.continuationCount); err != nil {
				t.Fatalf("seed continuation count: %v", err)
			}

			gate, gatePID := lockTraceInGate(t, ctx, pool, parentWakeParentTraceID)
			batch := startIngestBatch(t, ctx, pool, parentWakeParentTraceID, parentWakeChildTraceID)
			waitForLockQueue(t, ctx, pool, gatePID, batch)

			claimed := forceClaimRun(t, ctx, pool, fixture.store, fixture.childRunID, "worker-child")
			activation := startAsync(t, cancel, func() error { return fixture.activator.Activate(ctx, &claimed) })
			waitForLockQueue(t, ctx, pool, gatePID, batch, activation)

			if err := gate.Rollback(ctx); err != nil {
				t.Fatalf("release gate: %v", err)
			}
			if err := batch.wait(ctx); err != nil {
				t.Fatalf("ProcessBatch() error = %v", err)
			}
			if err := activation.wait(ctx); err != nil {
				t.Fatalf("Activate(child) error = %v", err)
			}
			assertParentQueuedAndProjected(t, ctx, fixture.store, fixture.parentRun.ID)
		})
	}
}

// The parent's activation decides to wait on a child it reads as active. If the child
// finishes before that wait commits, the child must still wake the parent: no wake may
// be lost. The child's terminal commit therefore queues on the parent's run lock, before
// it takes any trace lock, so it cannot leave the parent's trace outside the ordered set
// either. Real ingest queues on the parent's trace at the same time.
func TestActivatorChildTerminalWaitsForParentCommittingWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newParentWakeFixture(t, ctx, func(ctx publicworkflow.Context) error {
		return ctx.SetResult(map[string]string{"status": "charged"})
	})
	pool := fixture.db.Pool

	// The gate pauses the parent's activation at its projected trace write, after it read
	// the child as active and while it holds the parent's run lock.
	gate, gatePID := lockTraceInGate(t, ctx, pool, parentWakeParentTraceID)
	parentClaim := forceClaimRun(t, ctx, pool, fixture.store, fixture.parentRun.ID, "worker-parent")
	parentActivation := startAsync(t, cancel, func() error { return fixture.activator.Activate(ctx, &parentClaim) })
	waitForLockQueue(t, ctx, pool, gatePID, parentActivation)

	batch := startIngestBatch(t, ctx, pool, parentWakeParentTraceID, parentWakeChildTraceID)
	waitForLockQueue(t, ctx, pool, gatePID, parentActivation, batch)

	childClaim := forceClaimRun(t, ctx, pool, fixture.store, fixture.childRunID, "worker-child")
	childActivation := startAsync(t, cancel, func() error { return fixture.activator.Activate(ctx, &childClaim) })
	waitForLockQueue(t, ctx, pool, gatePID, parentActivation, batch, childActivation)

	if err := gate.Rollback(ctx); err != nil {
		t.Fatalf("release gate: %v", err)
	}
	for name, op := range map[string]*asyncOp{"parent activation": parentActivation, "ingest": batch, "child activation": childActivation} {
		if err := op.wait(ctx); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	assertParentQueuedAndProjected(t, ctx, fixture.store, fixture.parentRun.ID)
}

// Run locks follow one order: ancestor before descendant. TerminateRun on a parent locks
// the parent run and then its children. A completing child must not hold its own run
// while it waits for the parent's, or the two deadlock.
func TestActivatorChildTerminalLocksParentRunBeforeChildRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newParentWakeFixture(t, ctx, func(ctx publicworkflow.Context) error {
		return ctx.SetResult(map[string]string{"status": "charged"})
	})
	pool := fixture.db.Pool

	terminate, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin terminate-like tx: %v", err)
	}
	defer func() { _ = terminate.Rollback(context.Background()) }()
	var terminatePID int32
	if err := terminate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&terminatePID); err != nil {
		t.Fatalf("terminate-like tx pid: %v", err)
	}
	if _, err := terminate.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR NO KEY UPDATE`, fixture.parentRun.ID); err != nil {
		t.Fatalf("lock parent run: %v", err)
	}

	claimed := forceClaimRun(t, ctx, pool, fixture.store, fixture.childRunID, "worker-child")
	activation := startAsync(t, cancel, func() error { return fixture.activator.Activate(ctx, &claimed) })
	waitForLockQueue(t, ctx, pool, terminatePID, activation)

	if _, err := terminate.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR NO KEY UPDATE`, fixture.childRunID); err != nil {
		t.Fatalf("terminate-like tx lost a run lock-order conflict on the child: %v", err)
	}
	if err := terminate.Rollback(ctx); err != nil {
		t.Fatalf("release terminate-like tx: %v", err)
	}
	if err := activation.wait(ctx); err != nil {
		t.Fatalf("Activate(child) error = %v", err)
	}
	assertParentQueuedAndProjected(t, ctx, fixture.store, fixture.parentRun.ID)
}

// A grandchild's activation can insert a run that names the root run through the
// root_run_id foreign key: the next run of a continue-as-new, or a new child. The foreign
// key check locks the root row (KEY SHARE). TerminateRun on the root locks the root, then
// waits for the child, which the grandchild's activation holds as its parent. Run locks
// are FOR NO KEY UPDATE, which KEY SHARE does not wait for, or the two deadlock (40P01).
func TestActivatorGrandchildLocksRootRunBeforeParentRun(t *testing.T) {
	testCases := []struct {
		name          string
		grandchildRun func(publicworkflow.Context) error
	}{
		{
			name: "grandchild continues as new",
			grandchildRun: func(publicworkflow.Context) error {
				return publicworkflow.ContinueAsNew(nil)
			},
		},
		{
			name: "grandchild starts a child",
			grandchildRun: func(ctx publicworkflow.Context) error {
				return ctx.ChildWorkflow("print-receipt", "printer", "v1", nil, nil)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			billing := func(ctx publicworkflow.Context) error {
				return ctx.ChildWorkflow("issue-receipt", "receipts", "v1", nil, nil)
			}
			fixture := newParentWakeFixture(t, ctx, billing)
			pool := fixture.db.Pool
			rootRunID := fixture.parentRun.ID

			// The grandchild pauses inside its activation, after the activation took its run
			// locks and before it commits the decision that inserts the new run.
			paused, release := make(chan struct{}), make(chan struct{})
			activator := NewActivator(fixture.store, mustRegistry(t,
				childWaitingParentDefinition("checkout-parent-wake", nil),
				publicworkflow.Definition{Name: "billing", Version: "v1", Run: billing},
				publicworkflow.Definition{Name: "receipts", Version: "v1", Run: func(wctx publicworkflow.Context) error {
					close(paused)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					return testCase.grandchildRun(wctx)
				}},
				publicworkflow.Definition{Name: "printer", Version: "v1", Run: func(publicworkflow.Context) error { return nil }},
			))
			activateRunByID(t, ctx, pool, fixture.store, activator, fixture.childRunID, "worker-child")
			var grandchildRunID uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT current_child_run_id FROM engine.child_workflows WHERE parent_run_id = $1`,
				fixture.childRunID).Scan(&grandchildRunID); err != nil {
				t.Fatalf("read grandchild run: %v", err)
			}

			claimed := forceClaimRun(t, ctx, pool, fixture.store, grandchildRunID, "worker-grandchild")
			activation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &claimed) })
			select {
			case <-paused:
			case <-ctx.Done():
				t.Fatalf("grandchild activation never ran its workflow: %v", ctx.Err())
			}

			// TerminateRun(root) locks the root, then the descendants by depth.
			terminate, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin terminate-like tx: %v", err)
			}
			defer func() { _ = terminate.Rollback(context.Background()) }()
			var terminatePID int32
			if err := terminate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&terminatePID); err != nil {
				t.Fatalf("terminate-like tx pid: %v", err)
			}
			terminateLocks := startAsync(t, cancel, func() error {
				for _, runID := range []uuid.UUID{rootRunID, fixture.childRunID, grandchildRunID} {
					if _, err := terminate.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR NO KEY UPDATE`, runID); err != nil {
						return err
					}
				}
				return nil
			})
			waitUntilBlocked(t, ctx, pool, terminatePID)

			close(release)
			if err := activation.wait(ctx); err != nil {
				t.Fatalf("Activate(grandchild) error = %v", err)
			}
			if err := terminateLocks.wait(ctx); err != nil {
				t.Fatalf("terminate-like tx lost a run lock-order conflict: %v", err)
			}
			if err := terminate.Rollback(ctx); err != nil {
				t.Fatalf("release terminate-like tx: %v", err)
			}
			var inserted int
			if err := pool.QueryRow(ctx, `
				SELECT count(*) FROM engine.runs
				WHERE root_run_id = $1 AND (continued_from_run_id = $2 OR parent_run_id = $2)
			`, rootRunID, grandchildRunID).Scan(&inserted); err != nil {
				t.Fatalf("count runs inserted by the grandchild: %v", err)
			}
			if inserted != 1 {
				t.Fatalf("expected the grandchild to insert one run under root %s, got %d", rootRunID, inserted)
			}
		})
	}
}

// waitUntilBlocked waits until the backend pid queues behind another backend's lock.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func waitUntilBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid int32) {
	t.Helper()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1)) > 0`, pid).Scan(&blocked); err != nil {
			t.Fatalf("read blockers of pid %d: %v", pid, err)
		}
		if blocked {
			return
		}
	}
}

// A parent's cancelled activation enqueues a cancel inbox item for each active child.
// The item names the child run, so its foreign key check locks that run (KEY SHARE),
// after the activation locked the traces. An activity retry on the child holds the
// child's run and then waits for the child's trace, which ingest holds while it waits for
// the parent's trace, which the activation holds. If the run lock blocked KEY SHARE, the
// three would deadlock (40P01) and roll back the cancellation.
func TestActivatorCancelCascadeDoesNotWaitForChildRunLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	billing := func(ctx publicworkflow.Context) error {
		err := ctx.ChildWorkflow("issue-receipt", "receipts", "v1", nil, nil)
		if ctx.CancellationRequested() {
			return publicworkflow.ErrCancelled
		}
		return err
	}
	fixture := newParentWakeFixture(t, ctx, billing)
	pool := fixture.db.Pool
	activator := NewActivator(fixture.store, mustRegistry(t,
		childWaitingParentDefinition("checkout-parent-wake", nil),
		publicworkflow.Definition{Name: "billing", Version: "v1", Run: billing},
		publicworkflow.Definition{Name: "receipts", Version: "v1", Run: func(ctx publicworkflow.Context) error {
			return ctx.ActivityWithOptions("print", "print", nil, nil, publicworkflow.ActivityOptions{
				RetryPolicy: &publicworkflow.RetryPolicy{MaxAttempts: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffMultiplier: 1},
			})
		}},
	))
	billingRunID := fixture.childRunID
	activateRunByID(t, ctx, pool, fixture.store, activator, billingRunID, "worker-billing")
	var receiptsRunID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT current_child_run_id FROM engine.child_workflows WHERE parent_run_id = $1`,
		billingRunID).Scan(&receiptsRunID); err != nil {
		t.Fatalf("read receipts run: %v", err)
	}
	activateRunByID(t, ctx, pool, fixture.store, activator, receiptsRunID, "worker-receipts")

	// Ingest locks the receipts trace before the billing trace.
	for runID, traceID := range map[uuid.UUID]string{receiptsRunID: "engine:0-receipts", billingRunID: "engine:1-billing"} {
		if _, err := pool.Exec(ctx, `UPDATE public.traces SET trace_id = $2 WHERE engine_run_id = $1`, runID, traceID); err != nil {
			t.Fatalf("rename trace of run %s: %v", runID, err)
		}
	}
	// At the continuation-follow limit, billing observes its cancellation while receipts
	// is still active.
	if _, err := pool.Exec(ctx, `UPDATE engine.child_workflows SET continuation_count = $2 WHERE current_child_run_id = $1`,
		receiptsRunID, maxContinuationFollowDepth); err != nil {
		t.Fatalf("set continuation count: %v", err)
	}
	billingRun, err := fixture.store.GetRun(ctx, billingRunID)
	if err != nil {
		t.Fatalf("GetRun(billing) error = %v", err)
	}
	cancelPayload, err := enginehistory.MarshalPayload(enginehistory.CancelRequestedPayload{})
	if err != nil {
		t.Fatalf("marshal cancel payload: %v", err)
	}
	if _, err := fixture.store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   billingRun.ProjectID,
		InstanceID:  billingRun.InstanceID,
		RunID:       pgtype.UUID{Bytes: billingRunID, Valid: true},
		Kind:        "cancel",
		Payload:     cancelPayload,
		AvailableAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("request billing cancellation: %v", err)
	}
	billingClaim := forceClaimRun(t, ctx, pool, fixture.store, billingRunID, "worker-cancel")

	// The activity fails once. Its retry locks the receipts run, then queues on the gate,
	// which holds the activity task row.
	entered, release := make(chan struct{}), make(chan struct{})
	registry, err := activity.NewRegistry(map[string]activity.Handler{"print": func(context.Context, json.RawMessage) (json.RawMessage, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, errors.New("printer jammed")
	}})
	if err != nil {
		t.Fatalf("activity registry: %v", err)
	}
	worker := activity.NewWorker(fixture.store, registry, time.Minute, nil)
	retry := startAsync(t, cancel, func() error {
		_, err := worker.PollOnce(ctx, "worker-activity")
		return err
	})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatalf("activity never ran: %v", ctx.Err())
	}
	tasks, err := fixture.store.ListActivityTasksByRun(ctx, receiptsRunID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("ListActivityTasksByRun(receipts) = %d tasks, error %v; want 1 task", len(tasks), err)
	}
	gate, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin gate: %v", err)
	}
	defer func() { _ = gate.Rollback(context.Background()) }()
	var gatePID int32
	if err := gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
		t.Fatalf("gate pid: %v", err)
	}
	if _, err := gate.Exec(ctx, `SELECT id FROM engine.activity_tasks WHERE id = $1 FOR UPDATE`, tasks[0].ID); err != nil {
		t.Fatalf("gate lock activity task: %v", err)
	}
	close(release)
	waitForLockQueue(t, ctx, pool, gatePID, retry)

	activation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &billingClaim) })
	waitForLockQueue(t, ctx, pool, gatePID, retry, activation)
	batch := startIngestBatch(t, ctx, pool, "engine:0-receipts", "engine:1-billing")
	waitForLockQueue(t, ctx, pool, gatePID, retry, activation, batch)

	if err := gate.Rollback(ctx); err != nil {
		t.Fatalf("release gate: %v", err)
	}
	for name, op := range map[string]*asyncOp{"billing cancel activation": activation, "ingest": batch, "activity retry": retry} {
		if err := op.wait(ctx); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	cancelled, err := fixture.store.GetRun(ctx, billingRunID)
	if err != nil {
		t.Fatalf("GetRun(billing) error = %v", err)
	}
	if cancelled.Status != enginedb.EngineRunLifecycleStatusCancelled {
		t.Fatalf("expected billing to be cancelled, got status %q", cancelled.Status)
	}
	var cascaded int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM engine.inbox WHERE run_id = $1 AND kind = 'cancel'`,
		receiptsRunID).Scan(&cascaded); err != nil {
		t.Fatalf("count receipts cancel items: %v", err)
	}
	if cascaded != 1 {
		t.Fatalf("expected one cancel item for receipts, got %d", cascaded)
	}
}

type asyncOp struct {
	done chan struct{}
	err  error
}

// startAsync runs fn in a goroutine bounded by the test: cleanup cancels the shared ctx
// and joins the goroutine, so a failed assertion cannot leak a blocked transaction.
func startAsync(t *testing.T, cancel context.CancelFunc, fn func() error) *asyncOp {
	t.Helper()
	op := &asyncOp{done: make(chan struct{})}
	go func() {
		defer close(op.done)
		op.err = fn()
	}()
	t.Cleanup(func() {
		cancel()
		<-op.done
	})
	return op
}

func (op *asyncOp) finished() bool {
	select {
	case <-op.done:
		return true
	default:
		return false
	}
}

func (op *asyncOp) wait(ctx context.Context) error {
	select {
	case <-op.done:
		return op.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// lockTraceInGate holds a projected trace row the way ingest's LockTraceUUID does.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func lockTraceInGate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, traceID string) (pgx.Tx, int32) {
	t.Helper()
	gate, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin gate: %v", err)
	}
	t.Cleanup(func() { _ = gate.Rollback(context.Background()) })
	var pid int32
	if err := gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("gate pid: %v", err)
	}
	if _, err := gate.Exec(ctx, `SELECT id FROM public.traces WHERE project_id = $1 AND trace_id = $2 FOR NO KEY UPDATE`,
		enginetest.DefaultPlatformProjectID, traceID); err != nil {
		t.Fatalf("gate lock %s: %v", traceID, err)
	}
	return gate, pid
}

// startIngestBatch writes one span under each trace through the platform ingest write path.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func startIngestBatch(t *testing.T, ctx context.Context, pool *pgxpool.Pool, traceIDs ...string) *asyncOp {
	t.Helper()
	platform := platformstore.New(pool)
	req := &ingest.IngestRequest{BatchKey: "parent-wake-" + uuid.NewString()}
	for _, traceID := range traceIDs {
		req.Spans = append(req.Spans, ingest.SpanInput{
			TraceID:   traceID,
			SpanID:    "ingest-" + traceID,
			Name:      "ingest span",
			StartTime: time.Now().UTC(),
		})
	}
	return startAsync(t, func() {}, func() error {
		tx, err := platform.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := ingest.NewProcessor(platform, nil).ProcessBatch(ctx, tx, enginetest.DefaultPlatformProjectID, req); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

// waitForLockQueue waits until every unfinished op queues, directly or through another
// op, behind the holder's locks. An op that finishes instead never queued, which happens
// on code without the lock order; the assertions after the wait then report that.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func waitForLockQueue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, holderPID int32, ops ...*asyncOp) {
	t.Helper()
	for {
		var queued int
		if err := pool.QueryRow(ctx, `
			WITH RECURSIVE queue(pid) AS (
			    SELECT $1::int
			    UNION
			    SELECT a.pid FROM pg_stat_activity AS a JOIN queue AS q ON q.pid = ANY(pg_blocking_pids(a.pid))
			)
			SELECT count(*) - 1 FROM queue
		`, holderPID).Scan(&queued); err != nil {
			t.Fatalf("read lock queue behind pid %d: %v", holderPID, err)
		}
		want := 0
		for _, op := range ops {
			if !op.finished() {
				want++
			}
		}
		if queued >= want {
			return
		}
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func assertParentQueuedAndProjected(t *testing.T, ctx context.Context, store *enginestore.Store, parentRunID uuid.UUID) {
	t.Helper()
	parent, err := store.GetRun(ctx, parentRunID)
	if err != nil {
		t.Fatalf("GetRun(parent) error = %v", err)
	}
	if parent.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected the child to wake its parent, got parent status %q", parent.Status)
	}
	var projected string
	if err := store.Pool().QueryRow(ctx, `SELECT engine_run_status FROM public.traces WHERE engine_run_id = $1`, parentRunID).Scan(&projected); err != nil {
		t.Fatalf("query parent projected run status: %v", err)
	}
	if projected != string(enginedb.EngineRunLifecycleStatusQueued) {
		t.Fatalf("expected parent projected run status %q after the wake, got %q", enginedb.EngineRunLifecycleStatusQueued, projected)
	}
}
