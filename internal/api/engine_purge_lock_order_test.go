package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/api/middleware"
)

// A full purge of a finished run and a remote report for a task the run still holds
// must not deadlock (#356). The report locks the run, then the task, then the trace, so
// the purge locks the run before the trace and the tasks too. A gate on the run's trace
// stops one side after it holds the run: either the purge holds the run when the report
// arrives, or the report holds the run and the task when the purge arrives.
func TestPurgeAndRemoteActivityReportLockRunFirst(t *testing.T) {
	reports := []remoteActivityReport{
		{
			name:        "complete",
			maxAttempts: 1,
			path:        "complete",
			body:        EngineRemoteActivityCompleteRequest{WorkerId: "worker-a", Output: map[string]any{"ok": true}},
			serve: func(s *Server, w http.ResponseWriter, r *http.Request, taskID uuid.UUID) {
				s.CompleteRemoteActivityTask(w, r, taskID, CompleteRemoteActivityTaskParams{})
			},
			taskStatus: enginedb.EngineActivityTaskStatusCompleted,
		},
		{
			name:        "fail with retry",
			maxAttempts: 3,
			path:        "fail",
			body:        EngineRemoteActivityFailRequest{WorkerId: "worker-a", ErrorCode: "temporary", ErrorMessage: "try again"},
			serve: func(s *Server, w http.ResponseWriter, r *http.Request, taskID uuid.UUID) {
				s.FailRemoteActivityTask(w, r, taskID, FailRemoteActivityTaskParams{})
			},
			taskStatus: enginedb.EngineActivityTaskStatusQueued,
			retried:    true,
		},
		{
			name:        "fail terminally",
			maxAttempts: 1,
			path:        "fail",
			body:        EngineRemoteActivityFailRequest{WorkerId: "worker-a", ErrorCode: "fatal", ErrorMessage: "gave up"},
			serve: func(s *Server, w http.ResponseWriter, r *http.Request, taskID uuid.UUID) {
				s.FailRemoteActivityTask(w, r, taskID, FailRemoteActivityTaskParams{})
			},
			taskStatus: enginedb.EngineActivityTaskStatusFailed,
		},
	}
	for _, report := range reports {
		t.Run(report.name+"/purge first", func(t *testing.T) {
			testPurgeAndRemoteActivityReport(t, &report, true)
		})
		t.Run(report.name+"/report first", func(t *testing.T) {
			testPurgeAndRemoteActivityReport(t, &report, false)
		})
	}
}

// testPurgeAndRemoteActivityReport runs a full purge and one remote report on a completed
// run that still holds a claimed task. A gate on the trace queues both operations, in the
// order purgeFirst picks; then it checks that both succeed and the end state is correct.
func testPurgeAndRemoteActivityReport(t *testing.T, report *remoteActivityReport, purgeFirst bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	pool := platformStore.Pool()
	fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "purge-lock-order-task",
		ActivityType:    "purge.lock.order",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     report.maxAttempts,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-a", fixture.Task.ActivityType)
	// A run can finish while a remote worker still holds one of its tasks: a completed
	// or failed decision does not cancel open tasks.
	var traceID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM public.traces WHERE engine_run_id = $1`, fixture.Run.ID).Scan(&traceID))
	markEngineRunCompleted(t, ctx, platformStore, fixture.Run.ID, traceID)

	gate, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = gate.Rollback(context.Background()) }()
	var gatePID int32
	require.NoError(t, gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID))
	_, err = gate.Exec(ctx, `SELECT id FROM public.traces WHERE id = $1 FOR NO KEY UPDATE`, traceID)
	require.NoError(t, err)

	var purge, reported *terminateWakeOp
	if purgeFirst {
		purge = startPurgeEngineRun(t, ctx, cancel, server, projectID, fixture.Run.ID, Full)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, purge)
		reported = startRemoteActivityReport(t, ctx, cancel, server, projectID, fixture.Task.ID, report)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, purge, reported)
	} else {
		reported = startRemoteActivityReport(t, ctx, cancel, server, projectID, fixture.Task.ID, report)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, reported)
		purge = startPurgeEngineRun(t, ctx, cancel, server, projectID, fixture.Run.ID, Full)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, reported, purge)
	}

	require.NoError(t, gate.Rollback(ctx))
	require.NoError(t, reported.wait(ctx), "report")
	require.NoError(t, purge.wait(ctx), "purge")
	require.Equal(t, http.StatusOK, purge.rec.Code, purge.rec.Body.String())
	require.Equal(t, http.StatusNoContent, reported.rec.Code, reported.rec.Body.String())

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{ProjectID: projectID, ID: fixture.Run.ID})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusCompleted, run.Status)
	task, err := engineQueries.GetActivityTask(ctx, fixture.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, report.taskStatus, task.Status)

	var projectionState, projectedStatus string
	var pendingTasks int64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT engine_projection_state, engine_run_status, engine_pending_activity_tasks
		FROM public.traces
		WHERE id = $1
	`, traceID).Scan(&projectionState, &projectedStatus, &pendingTasks))
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusCompleted), projectedStatus)
	if report.retried && purgeFirst {
		// The retry appends a history row to the finished run. After the purge that row
		// reopens the projection (catching_up). That follows from a finished run keeping
		// open tasks, not from the lock order.
		return
	}
	assert.Equal(t, "journal_expired", projectionState)
	if !report.retried {
		assert.Zero(t, pendingTasks)
	}
	assert.Nil(t, task.HistoryID, "the full purge clears the task's history link")
	historyRows, err := engineQueries.GetHistoryByRun(ctx, fixture.Run.ID)
	require.NoError(t, err)
	assert.Empty(t, historyRows, "the full purge deletes the run's history")
}

// Every purge mode and repair lock the run before the run's trace. A gate holds the run
// lock; each operation must queue behind it while the trace is still free.
func TestPurgeAndRepairLockRunBeforeTrace(t *testing.T) {
	ops := []struct {
		name  string
		start func(t *testing.T, ctx context.Context, cancel context.CancelFunc, server *Server, projectID, runID uuid.UUID) *terminateWakeOp
	}{
		{name: "purge projection_only", start: func(t *testing.T, ctx context.Context, cancel context.CancelFunc, server *Server, projectID, runID uuid.UUID) *terminateWakeOp {
			return startPurgeEngineRun(t, ctx, cancel, server, projectID, runID, ProjectionOnly)
		}},
		{name: "purge full", start: func(t *testing.T, ctx context.Context, cancel context.CancelFunc, server *Server, projectID, runID uuid.UUID) *terminateWakeOp {
			return startPurgeEngineRun(t, ctx, cancel, server, projectID, runID, Full)
		}},
		{name: "repair", start: startRepairEngineRun},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
			pool := platformStore.Pool()
			fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
				ProjectID:       projectID,
				ActivityKey:     "purge-run-first",
				ActivityType:    "purge.run.first",
				ExecutionTarget: "remote",
				AvailableAt:     time.Now().Add(-time.Minute),
			})
			var traceID uuid.UUID
			require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM public.traces WHERE engine_run_id = $1`, fixture.Run.ID).Scan(&traceID))
			markEngineRunCompleted(t, ctx, platformStore, fixture.Run.ID, traceID)

			gate, err := pool.Begin(ctx)
			require.NoError(t, err)
			defer func() { _ = gate.Rollback(context.Background()) }()
			var gatePID int32
			require.NoError(t, gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID))
			_, err = gate.Exec(ctx, `SELECT id FROM engine.runs WHERE id = $1 FOR NO KEY UPDATE`, fixture.Run.ID)
			require.NoError(t, err)

			running := op.start(t, ctx, cancel, server, projectID, fixture.Run.ID)
			waitForTerminateLockQueue(t, ctx, pool, gatePID, running)
			require.False(t, running.finished(), "%s must wait for the run lock", op.name)

			probe, err := pool.Begin(ctx)
			require.NoError(t, err)
			_, err = probe.Exec(ctx, `SELECT id FROM public.traces WHERE id = $1 FOR UPDATE NOWAIT`, traceID)
			require.NoError(t, err, "%s must not hold the trace while it waits for the run", op.name)
			require.NoError(t, probe.Rollback(ctx))

			require.NoError(t, gate.Rollback(ctx))
			require.NoError(t, running.wait(ctx))
			require.Equal(t, http.StatusOK, running.rec.Code, running.rec.Body.String())
		})
	}
}

//nolint:revive // Keep testing.T first in test helper signatures.
func startPurgeEngineRun(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	server *Server,
	projectID uuid.UUID,
	runID uuid.UUID,
	mode EnginePurgeMode,
) *terminateWakeOp {
	t.Helper()
	body, err := json.Marshal(EnginePurgeRequest{Mode: mode})
	require.NoError(t, err)
	return startTerminateWakeOp(t, cancel, func(op *terminateWakeOp) error {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/engine/runs/"+runID.String()+"/purge", bytes.NewReader(body))
		req.Header.Set(enginePreviewHeader, "1")
		req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
		op.rec = httptest.NewRecorder()
		server.PurgeEngineRun(op.rec, req, runID, PurgeEngineRunParams{})
		return nil
	})
}

//nolint:revive // Keep testing.T first in test helper signatures.
func startRepairEngineRun(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	server *Server,
	projectID uuid.UUID,
	runID uuid.UUID,
) *terminateWakeOp {
	t.Helper()
	return startTerminateWakeOp(t, cancel, func(op *terminateWakeOp) error {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/engine/runs/"+runID.String()+"/repair", nil)
		req.Header.Set(enginePreviewHeader, "1")
		req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
		op.rec = httptest.NewRecorder()
		server.RepairEngineRun(op.rec, req, runID, RepairEngineRunParams{})
		return nil
	})
}
