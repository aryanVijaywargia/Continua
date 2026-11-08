package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

// A run can continue as new while its child is still active: the child keeps the old
// run as its parent. The workflow instance is the logical parent, so terminate and the
// cancel cascade must reach the active children of every run of an instance, at every
// level of the subtree (#354).

// earlierRunChildFixture holds a workflow instance whose earlier run continued as new
// while its child "work" was still active. The child is queued and never activated.
type earlierRunChildFixture struct {
	pool         *pgxpool.Pool
	store        *enginestore.Store
	rootRunID    uuid.UUID // the run a test terminates
	earlierRunID uuid.UUID // the run that continued as new
	currentRunID uuid.UUID // the run it continued as, waiting for a signal
	childRunID   uuid.UUID // the active child of the earlier run
}

// continuingDefinition starts the child "work", and continues as new once its wait on
// the child fails, with the child still active. The next run waits for a signal. Both
// runs end cancelled when asked to.
func continuingDefinition() publicworkflow.Definition {
	return publicworkflow.Definition{Name: "continuing", Version: "v1", Run: func(ctx publicworkflow.Context) error {
		if ctx.CancellationRequested() {
			return publicworkflow.ErrCancelled
		}
		var input struct {
			Continued bool `json:"continued"`
		}
		if err := ctx.Input(&input); err != nil {
			return err
		}
		if input.Continued {
			return ctx.ReceiveSignal("finish", nil)
		}
		if err := ctx.ChildWorkflow("work", "worker", "v1", nil, nil); err == nil {
			return errors.New("child workflow ended before its wait failed")
		}
		return publicworkflow.ContinueAsNew(map[string]bool{"continued": true})
	}}
}

// cancellableWorker waits for a signal, and ends cancelled when asked to.
func cancellableWorker(ctx publicworkflow.Context) error {
	if ctx.CancellationRequested() {
		return publicworkflow.ErrCancelled
	}
	return ctx.ReceiveSignal("finish", nil)
}

// newEarlierRunChildFixture builds the instance at the root (nested false) or as the
// child "billing" of a root that waits on it (nested true). worker runs the child "work".
//
//nolint:revive // Keep testing.T first in test helper signatures.
func newEarlierRunChildFixture(
	t *testing.T,
	ctx context.Context,
	nested bool,
	worker func(publicworkflow.Context) error,
) earlierRunChildFixture {
	t.Helper()

	db := enginetest.NewTestDatabase(t)
	pool := newLockOrderPool(t, ctx, db.DatabaseURL)
	store := enginestore.New(pool)
	definitions := []publicworkflow.Definition{
		continuingDefinition(),
		{Name: "worker", Version: "v1", Run: worker},
	}
	rootDefinition := "continuing"
	if nested {
		rootDefinition = "orders"
		definitions = append(definitions, publicworkflow.Definition{Name: "orders", Version: "v1", Run: func(ctx publicworkflow.Context) error {
			return ctx.ChildWorkflow("billing", "continuing", "v1", nil, nil)
		}})
	}
	activator := NewActivator(store, mustRegistry(t, definitions...))
	_, rootRun := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-earlier-run-children",
		definitionName:    rootDefinition,
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{}),
	})
	activateRunByID(t, ctx, pool, store, activator, rootRun.ID, "worker-root")

	earlierRunID := rootRun.ID
	if nested {
		earlierRunID = activeChildRunID(t, ctx, pool, rootRun.ID)
		activateRunByID(t, ctx, pool, store, activator, earlierRunID, "worker-billing")
	}
	childRunID := activeChildRunID(t, ctx, pool, earlierRunID)

	// At the continuation-follow limit the earlier run's wait on its child fails, so it
	// continues as new while the child is still active.
	if _, err := pool.Exec(ctx, `UPDATE engine.child_workflows SET continuation_count = $2 WHERE current_child_run_id = $1`,
		childRunID, maxContinuationFollowDepth); err != nil {
		t.Fatalf("set continuation count: %v", err)
	}
	activateRunByID(t, ctx, pool, store, activator, earlierRunID, "worker-continue")
	earlierRun, err := store.GetRun(ctx, earlierRunID)
	if err != nil {
		t.Fatalf("GetRun(earlier) error = %v", err)
	}
	if earlierRun.Status != enginedb.EngineRunLifecycleStatusContinuedAsNew || !earlierRun.ContinuedToRunID.Valid {
		t.Fatalf("expected the earlier run to continue as new, got %+v", earlierRun)
	}
	currentRunID := uuid.UUID(earlierRun.ContinuedToRunID.Bytes)
	activateRunByID(t, ctx, pool, store, activator, currentRunID, "worker-current")

	return earlierRunChildFixture{
		pool:         pool,
		store:        store,
		rootRunID:    rootRun.ID,
		earlierRunID: earlierRunID,
		currentRunID: currentRunID,
		childRunID:   childRunID,
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func activeChildRunID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, parentRunID uuid.UUID) uuid.UUID {
	t.Helper()
	var childRunID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT current_child_run_id FROM engine.child_workflows WHERE parent_run_id = $1 AND status = 'active'`,
		parentRunID).Scan(&childRunID); err != nil {
		t.Fatalf("read active child of run %s: %v", parentRunID, err)
	}
	return childRunID
}

// terminateTarget is the run a test terminates: the instance's current run when it is
// the root, else the root above it.
func (f *earlierRunChildFixture) terminateTarget() uuid.UUID {
	if f.rootRunID == f.earlierRunID {
		return f.currentRunID
	}
	return f.rootRunID
}

func TestTerminateRunTerminatesChildrenOfEarlierRuns(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "root continued as new"
		if nested {
			name = "child in the subtree continued as new"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			fixture := newEarlierRunChildFixture(t, ctx, nested, cancellableWorker)
			handler := newEngineControlAPI(t, ctx, fixture.pool)

			if err := terminateRunOverAPI(ctx, handler, fixture.terminateTarget()); err != nil {
				t.Fatalf("TerminateRun error = %v", err)
			}
			// root case: earlier, current, child. nested case: root, earlier, current, child.
			wantRuns := 3
			if nested {
				wantRuns = 4
			}
			assertAllRunsEnded(t, ctx, fixture.pool, wantRuns)
		})
	}
}

// The earlier run's child continues as new while a terminate runs. The child's activation
// holds the earlier run and the child, so termination must lock the earlier run before it
// lists that run's children; then it finds the run the child continued as. A list taken
// before the wait finds the old child run, and leaves the new one queued under a
// terminated workflow.
func TestTerminateRunLocksEarlierRunBeforeListingItsChildren(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "root continued as new"
		if nested {
			name = "child in the subtree continued as new"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			fixture := newEarlierRunChildFixture(t, ctx, nested, cancellableWorker)
			handler := newEngineControlAPI(t, ctx, fixture.pool)

			// The child pauses inside its activation, after the activation locked the
			// earlier run and the child run, and before it commits its continue-as-new.
			paused, release := make(chan struct{}), make(chan struct{})
			activator := NewActivator(fixture.store, mustRegistry(t,
				publicworkflow.Definition{Name: "worker", Version: "v1", Run: func(publicworkflow.Context) error {
					close(paused)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					return publicworkflow.ContinueAsNew(nil)
				}},
			))
			claimed := forceClaimRun(t, ctx, fixture.pool, fixture.store, fixture.childRunID, "worker-child")
			activation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &claimed) })
			select {
			case <-paused:
			case <-ctx.Done():
				t.Fatalf("child activation never ran its workflow: %v", ctx.Err())
			}

			terminate := startAsync(t, cancel, func() error { return terminateRunOverAPI(ctx, handler, fixture.terminateTarget()) })
			waitForRunLockWait(t, ctx, fixture.pool, terminate, "GetRunByProjectAndIDForUpdate")

			close(release)
			if err := activation.wait(ctx); err != nil {
				t.Fatalf("Activate(child) error = %v", err)
			}
			if err := terminate.wait(ctx); err != nil {
				t.Fatalf("TerminateRun error = %v", err)
			}
			// The runs of the fixture, plus the run the child continued as.
			wantRuns := 4
			if nested {
				wantRuns = 5
			}
			assertAllRunsEnded(t, ctx, fixture.pool, wantRuns)
		})
	}
}

func TestActivatorCancelCascadeReachesChildrenOfEarlierRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newEarlierRunChildFixture(t, ctx, false, cancellableWorker)
	activator := NewActivator(fixture.store, mustRegistry(t,
		continuingDefinition(),
		publicworkflow.Definition{Name: "worker", Version: "v1", Run: cancellableWorker},
	))

	requestCancel(t, ctx, fixture.store, fixture.currentRunID)
	activateRunByID(t, ctx, fixture.pool, fixture.store, activator, fixture.currentRunID, "worker-cancel")
	assertRunCancelled(t, ctx, fixture.store, fixture.currentRunID)

	var cascaded int
	if err := fixture.pool.QueryRow(ctx, `SELECT count(*) FROM engine.inbox WHERE run_id = $1 AND kind = 'cancel' AND status = 'pending'`,
		fixture.childRunID).Scan(&cascaded); err != nil {
		t.Fatalf("count child cancel items: %v", err)
	}
	if cascaded != 1 {
		t.Fatalf("expected one cancel item for the earlier run's child, got %d", cascaded)
	}
	activateRunByID(t, ctx, fixture.pool, fixture.store, activator, fixture.childRunID, "worker-child")
	assertRunCancelled(t, ctx, fixture.store, fixture.childRunID)
}

// The earlier run's child continues as new while the current run's cancellation commits.
// The cascade must lock the earlier run before it lists that run's children, so it
// enqueues the cancel on the run the child continued as. A list taken without the lock
// finds the old child run, whose continue-as-new then leaves the cancel behind.
func TestActivatorCancelCascadeLocksEarlierRunBeforeListingItsChildren(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newEarlierRunChildFixture(t, ctx, false, cancellableWorker)
	requestCancel(t, ctx, fixture.store, fixture.currentRunID)

	paused, release := make(chan struct{}), make(chan struct{})
	childActivator := NewActivator(fixture.store, mustRegistry(t,
		publicworkflow.Definition{Name: "worker", Version: "v1", Run: func(publicworkflow.Context) error {
			close(paused)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return publicworkflow.ContinueAsNew(nil)
		}},
	))
	childClaim := forceClaimRun(t, ctx, fixture.pool, fixture.store, fixture.childRunID, "worker-child")
	childActivation := startAsync(t, cancel, func() error { return childActivator.Activate(ctx, &childClaim) })
	select {
	case <-paused:
	case <-ctx.Done():
		t.Fatalf("child activation never ran its workflow: %v", ctx.Err())
	}

	cancelActivator := NewActivator(fixture.store, mustRegistry(t, continuingDefinition()))
	cancelClaim := forceClaimRun(t, ctx, fixture.pool, fixture.store, fixture.currentRunID, "worker-cancel")
	cancelActivation := startAsync(t, cancel, func() error { return cancelActivator.Activate(ctx, &cancelClaim) })
	waitForRunLockWait(t, ctx, fixture.pool, cancelActivation, "GetRunForUpdate")

	close(release)
	if err := childActivation.wait(ctx); err != nil {
		t.Fatalf("Activate(child) error = %v", err)
	}
	if err := cancelActivation.wait(ctx); err != nil {
		t.Fatalf("Activate(cancel) error = %v", err)
	}
	assertRunCancelled(t, ctx, fixture.store, fixture.currentRunID)
	nextChildRunID := activeChildRunID(t, ctx, fixture.pool, fixture.earlierRunID)
	if nextChildRunID == fixture.childRunID {
		t.Fatalf("expected the child to continue as new")
	}
	var cascaded int
	if err := fixture.pool.QueryRow(ctx, `SELECT count(*) FROM engine.inbox WHERE run_id = $1 AND kind = 'cancel' AND status = 'pending'`,
		nextChildRunID).Scan(&cascaded); err != nil {
		t.Fatalf("count child cancel items: %v", err)
	}
	if cascaded != 1 {
		t.Fatalf("expected one cancel item for the run the child continued as, got %d", cascaded)
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func requestCancel(t *testing.T, ctx context.Context, store *enginestore.Store, runID uuid.UUID) {
	t.Helper()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", runID, err)
	}
	payload, err := enginehistory.MarshalPayload(enginehistory.CancelRequestedPayload{})
	if err != nil {
		t.Fatalf("marshal cancel payload: %v", err)
	}
	if _, err := store.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   run.ProjectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "cancel",
		Payload:     payload,
		AvailableAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("request cancellation of run %s: %v", runID, err)
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func assertRunCancelled(t *testing.T, ctx context.Context, store *enginestore.Store, runID uuid.UUID) {
	t.Helper()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", runID, err)
	}
	if run.Status != enginedb.EngineRunLifecycleStatusCancelled {
		t.Fatalf("run %s has status %q, want cancelled", runID, run.Status)
	}
}

// assertAllRunsEnded checks that the database holds wantRuns runs, each terminated or
// continued as new, with no active child relationship left, and that each projected
// trace points at its run's last history event.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func assertAllRunsEnded(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantRuns int) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT r.id, r.status,
		       t.engine_latest_history_id, (SELECT max(h.id) FROM engine.history AS h WHERE h.run_id = r.id)
		FROM engine.runs AS r
		JOIN public.traces AS t ON t.engine_run_id = r.id
		ORDER BY r.child_depth, r.created_at
	`)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	defer rows.Close()
	runs := 0
	for rows.Next() {
		var (
			runID            uuid.UUID
			status           enginedb.EngineRunLifecycleStatus
			projectedHistory int64
			latestHistory    int64
		)
		if err := rows.Scan(&runID, &status, &projectedHistory, &latestHistory); err != nil {
			t.Fatalf("scan run: %v", err)
		}
		runs++
		if status != enginedb.EngineRunLifecycleStatusTerminated && status != enginedb.EngineRunLifecycleStatusContinuedAsNew {
			t.Errorf("run %s has status %q, want terminated or continued_as_new", runID, status)
		}
		if projectedHistory != latestHistory {
			t.Errorf("run %s trace points at history %d, want its last event %d", runID, projectedHistory, latestHistory)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read runs: %v", err)
	}
	if runs != wantRuns {
		t.Fatalf("expected %d runs, got %d", wantRuns, runs)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM engine.child_workflows WHERE status = 'active'`).Scan(&active); err != nil {
		t.Fatalf("count active child relationships: %v", err)
	}
	if active != 0 {
		t.Fatalf("expected no active child relationship, got %d", active)
	}
}

// reattachingDefinition starts its child under a fixed instance key and continues as
// new while the child is still active. When the next run is cancelled, its code asks
// for the same child instance again; the engine rejects that (instance_conflict), and
// the workflow honours the cancel.
func reattachingDefinition() publicworkflow.Definition {
	opts := publicworkflow.ChildWorkflowOptions{InstanceKey: "fixed-child-instance"}
	return publicworkflow.Definition{Name: "reattaching", Version: "v1", Run: func(ctx publicworkflow.Context) error {
		var input struct {
			Continued bool `json:"continued"`
		}
		if err := ctx.Input(&input); err != nil {
			return err
		}
		if !input.Continued {
			if err := ctx.ChildWorkflowWithOptions("work", "worker", "v1", nil, nil, opts); err == nil {
				return errors.New("child ended before its wait failed")
			}
			return publicworkflow.ContinueAsNew(map[string]bool{"continued": true})
		}
		if ctx.CancellationRequested() {
			_ = ctx.ChildWorkflowWithOptions("work", "worker", "v1", nil, nil, opts)
			return publicworkflow.ErrCancelled
		}
		return ctx.ReceiveSignal("finish", nil)
	}}
}

// The child binding check reads a relationship that belongs to an earlier run. It must
// not lock that row: the earlier run's child holds the earlier run while it updates the
// row, and the cancel cascade then waits on the earlier run (40P01 if the row is locked).
func TestActivatorCancelCascadeDoesNotLockEarlierRunRelationship(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := enginetest.NewTestDatabase(t)
	pool := newLockOrderPool(t, ctx, db.DatabaseURL)
	store := enginestore.New(pool)
	activator := NewActivator(store, mustRegistry(t,
		reattachingDefinition(),
		publicworkflow.Definition{Name: "worker", Version: "v1", Run: cancellableWorker},
	))
	_, earlierRun := createStartedRun(t, store, workflowTestCase{
		projectID:         enginetest.DefaultPlatformProjectID,
		instanceKey:       "instance-reattach",
		definitionName:    "reattaching",
		definitionVersion: "v1",
		input:             mustJSON(t, map[string]string{}),
	})
	activateRunByID(t, ctx, pool, store, activator, earlierRun.ID, "worker-earlier")
	childRunID := activeChildRunID(t, ctx, pool, earlierRun.ID)
	// Make the parent's wait on the child fail, so the parent continues as new.
	if _, err := pool.Exec(ctx, `UPDATE engine.child_workflows SET continuation_count = $2 WHERE current_child_run_id = $1`,
		childRunID, maxContinuationFollowDepth); err != nil {
		t.Fatalf("set continuation count: %v", err)
	}
	activateRunByID(t, ctx, pool, store, activator, earlierRun.ID, "worker-continue")
	continued, err := store.GetRun(ctx, earlierRun.ID)
	if err != nil {
		t.Fatalf("get earlier run: %v", err)
	}
	if continued.Status != enginedb.EngineRunLifecycleStatusContinuedAsNew {
		t.Fatalf("earlier run status = %s, want continued_as_new", continued.Status)
	}
	currentRunID := uuid.UUID(continued.ContinuedToRunID.Bytes)
	activateRunByID(t, ctx, pool, store, activator, currentRunID, "worker-current")
	requestCancel(t, ctx, store, currentRunID)

	// The earlier run's child pauses inside its activation, holding the earlier run
	// and its own run.
	paused, release := make(chan struct{}), make(chan struct{})
	childActivator := NewActivator(store, mustRegistry(t,
		publicworkflow.Definition{Name: "worker", Version: "v1", Run: func(publicworkflow.Context) error {
			close(paused)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return publicworkflow.ContinueAsNew(nil)
		}},
	))
	childClaim := forceClaimRun(t, ctx, pool, store, childRunID, "worker-child")
	childActivation := startAsync(t, cancel, func() error { return childActivator.Activate(ctx, &childClaim) })
	select {
	case <-paused:
	case <-ctx.Done():
		t.Fatalf("child activation never ran: %v", ctx.Err())
	}

	// The current run's cancel activation checks the child binding, then its cascade
	// waits on the earlier run.
	cancelClaim := forceClaimRun(t, ctx, pool, store, currentRunID, "worker-cancel")
	cancelActivation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &cancelClaim) })
	waitForRunLockWait(t, ctx, pool, cancelActivation, "GetRunForUpdate")

	close(release)
	if err := childActivation.wait(ctx); err != nil {
		t.Fatalf("child activation: %v", err)
	}
	if err := cancelActivation.wait(ctx); err != nil {
		t.Fatalf("cancel activation: %v", err)
	}
	assertRunCancelled(t, ctx, store, currentRunID)
}
