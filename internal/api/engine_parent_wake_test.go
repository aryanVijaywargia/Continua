package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

// The parent's trace id sorts before the child's in byte order, which platform ingest
// uses, but after it under a linguistic collation, so lock queries must compare bytes.
const (
	terminateWakeParentTraceID = "engine:Parent"
	terminateWakeChildTraceID  = "engine:child"
)

// Terminating a child wakes its waiting parent, which writes the parent's projected trace
// as well as the child's. Real ingest holds the parent's trace, which sorts first, and
// then asks for the child's. Termination must not hold the child's trace while it waits
// for the parent's, or the two deadlock (40P01). The woken parent's projected status
// must change in the same transaction as the wake (#348).
func TestTerminateEngineRun_WakesExternalParentBehindIngestInTraceOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	platformStore, engineQueries, server, projectID, parentRun, childRun := seedTerminateWakeRuns(t, ctx, "waiting")
	pool := platformStore.Pool()

	gate, err := platformStore.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = gate.Rollback(context.Background()) }()
	var gatePID int32
	require.NoError(t, gate.Tx().QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID))
	_, err = gate.LockTraceUUID(ctx, projectID, terminateWakeParentTraceID)
	require.NoError(t, err)

	batch := startTerminateWakeIngest(t, ctx, cancel, platformStore, projectID)
	waitForTerminateLockQueue(t, ctx, pool, gatePID, batch)
	terminate := startTerminateEngineRun(t, ctx, cancel, server, projectID, childRun.ID)
	waitForTerminateLockQueue(t, ctx, pool, gatePID, batch, terminate)

	require.NoError(t, gate.Rollback(ctx))
	require.NoError(t, batch.wait(ctx), "ProcessBatch")
	require.NoError(t, terminate.wait(ctx), "terminate")
	require.Equal(t, http.StatusOK, terminate.rec.Code, terminate.rec.Body.String())
	assertTerminatedRunState(t, ctx, engineQueries, projectID, childRun.ID)
	assertParentRunQueuedAndProjected(t, ctx, pool, engineQueries, projectID, parentRun.ID)
}

// The parent's activation decides to wait on a child it reads as active. If the child is
// terminated before that wait commits, termination must still wake the parent: no wake
// may be lost. Termination therefore queues on the parent's run lock, before it takes any
// trace lock, so it cannot leave the parent's trace outside the ordered set either. Real
// ingest queues on the parent's trace at the same time.
func TestTerminateEngineRun_WaitsForParentCommittingWaitOnChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	platformStore, engineQueries, server, projectID, parentRun, childRun := seedTerminateWakeRuns(t, ctx, "running")
	pool := platformStore.Pool()

	// An activation-like transaction for the parent: lock the run, then commit its wait on
	// the child and project it, as the engine's decisionWaiting branch does.
	parentActivation, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = parentActivation.Rollback(context.Background()) }()
	var parentPID int32
	require.NoError(t, parentActivation.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&parentPID))
	_, err = parentActivation.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR UPDATE`, parentRun.ID)
	require.NoError(t, err)

	terminate := startTerminateEngineRun(t, ctx, cancel, server, projectID, childRun.ID)
	waitForTerminateLockQueue(t, ctx, pool, parentPID, terminate)

	setParentRunStatus(t, ctx, parentActivation, parentRun.ID, "waiting")
	batch := startTerminateWakeIngest(t, ctx, cancel, platformStore, projectID)
	waitForTerminateLockQueue(t, ctx, pool, parentPID, terminate, batch)

	require.NoError(t, parentActivation.Commit(ctx))
	require.NoError(t, batch.wait(ctx), "ProcessBatch")
	require.NoError(t, terminate.wait(ctx), "terminate")
	require.Equal(t, http.StatusOK, terminate.rec.Code, terminate.rec.Body.String())
	assertTerminatedRunState(t, ctx, engineQueries, projectID, childRun.ID)
	assertParentRunQueuedAndProjected(t, ctx, pool, engineQueries, projectID, parentRun.ID)
}

// Run locks follow one order: ancestor before descendant. A grandchild's activation locks
// its parent (the child) and then itself. Terminating the root must lock the child before
// the grandchild, not in the deepest-first order it terminates them in, or the two
// deadlock.
func TestTerminateEngineRun_LocksDescendantRunsAncestorFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	pool := platformStore.Pool()
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminate-run-order",
		RequestKey:        "req-terminate-run-order",
	}))
	rootRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{ProjectID: projectID, ID: start.RunId})
	require.NoError(t, err)
	rootInstance, err := engineQueries.GetInstance(ctx, rootRun.InstanceID)
	require.NoError(t, err)
	childInstance, childRun := seedEngineChildRun(t, ctx, platformStore, engineQueries,
		rootInstance, rootRun, "charge-card", "billing", "instance-terminate-run-order-child")
	_, grandchildRun := seedEngineChildRun(t, ctx, platformStore, engineQueries,
		childInstance, childRun, "issue-receipt", "receipts", "instance-terminate-run-order-grandchild")

	grandchildActivation, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = grandchildActivation.Rollback(context.Background()) }()
	var activationPID int32
	require.NoError(t, grandchildActivation.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&activationPID))
	_, err = grandchildActivation.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR UPDATE`, childRun.ID)
	require.NoError(t, err)

	terminate := startTerminateEngineRun(t, ctx, cancel, server, projectID, rootRun.ID)
	waitForTerminateLockQueue(t, ctx, pool, activationPID, terminate)

	_, err = grandchildActivation.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR UPDATE`, grandchildRun.ID)
	require.NoError(t, err, "grandchild activation lost a run lock-order conflict")
	require.NoError(t, grandchildActivation.Rollback(ctx))
	require.NoError(t, terminate.wait(ctx), "terminate")
	require.Equal(t, http.StatusOK, terminate.rec.Code, terminate.rec.Body.String())
	for _, runID := range []uuid.UUID{rootRun.ID, childRun.ID, grandchildRun.ID} {
		assertTerminatedRunState(t, ctx, engineQueries, projectID, runID)
	}
}

// seedTerminateWakeRuns seeds a parent run with one active child run, both projected,
// with the traces renamed so the parent's trace comes first in ingest's lock order.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func seedTerminateWakeRuns(
	t *testing.T,
	ctx context.Context,
	parentStatus string,
) (*store.Store, *enginedb.Queries, *Server, uuid.UUID, enginedb.EngineRun, enginedb.EngineRun) {
	t.Helper()
	_, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminate-parent-wake",
		RequestKey:        "req-terminate-parent-wake",
	}))
	parentRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{ProjectID: projectID, ID: start.RunId})
	require.NoError(t, err)
	parentInstance, err := engineQueries.GetInstance(ctx, parentRun.InstanceID)
	require.NoError(t, err)
	_, childRun := seedEngineChildRun(t, ctx, platformStore, engineQueries,
		parentInstance, parentRun, "charge-card", "billing", "instance-terminate-parent-wake-child")

	pool := platformStore.Pool()
	setParentRunStatus(t, ctx, pool, parentRun.ID, parentStatus)
	for runID, traceID := range map[uuid.UUID]string{parentRun.ID: terminateWakeParentTraceID, childRun.ID: terminateWakeChildTraceID} {
		_, err := pool.Exec(ctx, `UPDATE public.traces SET trace_id = $2 WHERE engine_run_id = $1`, runID, traceID)
		require.NoError(t, err)
	}
	return platformStore, engineQueries, server, projectID, parentRun, childRun
}

// setParentRunStatus moves the parent to "waiting" on the child, or to "running" under a
// worker claim, and projects that status as the engine does.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func setParentRunStatus(t *testing.T, ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, parentRunID uuid.UUID, status string,
) {
	t.Helper()
	var traceID string
	require.NoError(t, db.QueryRow(ctx, `
		WITH run AS (
		    UPDATE engine.runs
		    SET status = $2::text::engine.run_lifecycle_status,
		        waiting_for = CASE WHEN $2 = 'waiting'
		            THEN jsonb_build_object('kind', 'child_workflow', 'child_key', 'charge-card') END,
		        claimed_by = CASE WHEN $2 = 'running' THEN 'worker-parent' END,
		        claimed_at = CASE WHEN $2 = 'running' THEN NOW() END,
		        lease_expires_at = CASE WHEN $2 = 'running' THEN NOW() + INTERVAL '1 minute' END,
		        updated_at = NOW()
		    WHERE id = $1
		    RETURNING id
		)
		UPDATE public.traces SET engine_run_status = $2
		FROM run
		WHERE engine_run_id = run.id
		RETURNING trace_id
	`, parentRunID, status).Scan(&traceID))
}

type terminateWakeOp struct {
	done chan struct{}
	err  error
	rec  *httptest.ResponseRecorder
}

// startTerminateWakeOp runs fn in a goroutine bounded by the test: cleanup cancels the
// shared ctx and joins the goroutine, so a failed assertion cannot leak a blocked
// transaction.
func startTerminateWakeOp(t *testing.T, cancel context.CancelFunc, fn func(op *terminateWakeOp) error) *terminateWakeOp {
	t.Helper()
	op := &terminateWakeOp{done: make(chan struct{})}
	go func() {
		defer close(op.done)
		op.err = fn(op)
	}()
	t.Cleanup(func() {
		cancel()
		<-op.done
	})
	return op
}

func (op *terminateWakeOp) finished() bool {
	select {
	case <-op.done:
		return true
	default:
		return false
	}
}

func (op *terminateWakeOp) wait(ctx context.Context) error {
	select {
	case <-op.done:
		return op.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func startTerminateEngineRun(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	server *Server,
	projectID uuid.UUID,
	runID uuid.UUID,
) *terminateWakeOp {
	t.Helper()
	return startTerminateWakeOp(t, cancel, func(op *terminateWakeOp) error {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/engine/runs/"+runID.String()+"/terminate", nil)
		req.Header.Set(enginePreviewHeader, "1")
		req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
		op.rec = httptest.NewRecorder()
		server.TerminateEngineRun(op.rec, req, runID, TerminateEngineRunParams{})
		return nil
	})
}

// startTerminateWakeIngest writes one span under the parent's and the child's traces
// through the platform ingest write path.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func startTerminateWakeIngest(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	platformStore *store.Store,
	projectID uuid.UUID,
) *terminateWakeOp {
	t.Helper()
	req := &ingest.IngestRequest{BatchKey: "terminate-parent-wake-" + uuid.NewString()}
	for _, traceID := range []string{terminateWakeParentTraceID, terminateWakeChildTraceID} {
		req.Spans = append(req.Spans, ingest.SpanInput{TraceID: traceID, SpanID: "ingest-" + traceID, Name: "ingest span", StartTime: time.Now().UTC()})
	}
	return startTerminateWakeOp(t, cancel, func(*terminateWakeOp) error {
		tx, err := platformStore.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := ingest.NewProcessor(platformStore, nil).ProcessBatch(ctx, tx, projectID, req); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

// waitForTerminateLockQueue waits until every unfinished op queues, directly or through
// another op, behind the holder's locks. Following only the holder's queue keeps the
// count clear of other packages sharing the test database. An op that finishes instead
// never queued, which happens on code without the lock order; the assertions after the
// wait then report that.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func waitForTerminateLockQueue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, holderPID int32, ops ...*terminateWakeOp) {
	t.Helper()
	for {
		var queued int
		require.NoError(t, pool.QueryRow(ctx, `
			WITH RECURSIVE queue(pid) AS (
			    SELECT $1::int
			    UNION
			    SELECT a.pid FROM pg_stat_activity AS a JOIN queue AS q ON q.pid = ANY(pg_blocking_pids(a.pid))
			)
			SELECT count(*) - 1 FROM queue
		`, holderPID).Scan(&queued), "read lock queue behind pid %d", holderPID)
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
func assertParentRunQueuedAndProjected(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	engineQueries *enginedb.Queries,
	projectID uuid.UUID,
	parentRunID uuid.UUID,
) {
	t.Helper()
	parent, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{ProjectID: projectID, ID: parentRunID})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, parent.Status, "terminating the child must wake its parent")
	var projected string
	require.NoError(t, pool.QueryRow(ctx, `SELECT engine_run_status FROM public.traces WHERE engine_run_id = $1`, parentRunID).Scan(&projected))
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusQueued), projected, "the wake must project the parent's new status")
}
