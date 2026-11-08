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
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/internal/api/middleware"
)

type remoteActivityReport struct {
	name        string
	maxAttempts int32
	path        string
	body        any
	serve       func(s *Server, w http.ResponseWriter, r *http.Request, taskID uuid.UUID)
	// taskStatus is the task's final status when the report commits before the terminate.
	taskStatus enginedb.EngineActivityTaskStatus
	retried    bool
}

// A remote worker that reports an activity outcome while its run is terminated must not
// deadlock with the terminate (#352). Termination locks the run and then cancels the
// run's open tasks, so a report must lock the run before the task too. A gate on the
// run's trace stops one side after it holds the run: either the terminate holds the run
// when the report arrives, or the report holds the run and the task when the terminate
// arrives.
func TestRemoteActivityReportAndTerminateLockRunBeforeTask(t *testing.T) {
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
			// The retry queues the task again, and the terminate then cancels it.
			taskStatus: enginedb.EngineActivityTaskStatusCancelled,
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
		t.Run(report.name+"/terminate first", func(t *testing.T) {
			testRemoteActivityReportAndTerminate(t, &report, true)
		})
		t.Run(report.name+"/report first", func(t *testing.T) {
			testRemoteActivityReportAndTerminate(t, &report, false)
		})
	}
}

func testRemoteActivityReportAndTerminate(t *testing.T, report *remoteActivityReport, terminateFirst bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	pool := platformStore.Pool()
	fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "lock-order-task",
		ActivityType:    "lock.order",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     report.maxAttempts,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-a", fixture.Task.ActivityType)

	gate, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = gate.Rollback(context.Background()) }()
	var gatePID int32
	require.NoError(t, gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID))
	_, err = gate.Exec(ctx, `SELECT id FROM public.traces WHERE engine_run_id = $1 FOR NO KEY UPDATE`, fixture.Run.ID)
	require.NoError(t, err)

	var terminate, reported *terminateWakeOp
	if terminateFirst {
		terminate = startTerminateEngineRun(t, ctx, cancel, server, projectID, fixture.Run.ID)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, terminate)
		reported = startRemoteActivityReport(t, ctx, cancel, server, projectID, fixture.Task.ID, report)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, terminate, reported)
	} else {
		reported = startRemoteActivityReport(t, ctx, cancel, server, projectID, fixture.Task.ID, report)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, reported)
		terminate = startTerminateEngineRun(t, ctx, cancel, server, projectID, fixture.Run.ID)
		waitForTerminateLockQueue(t, ctx, pool, gatePID, reported, terminate)
	}

	require.NoError(t, gate.Rollback(ctx))
	require.NoError(t, reported.wait(ctx), "report")
	require.NoError(t, terminate.wait(ctx), "terminate")
	require.Equal(t, http.StatusOK, terminate.rec.Code, terminate.rec.Body.String())
	assertTerminatedRunState(t, ctx, engineQueries, projectID, fixture.Run.ID)

	task, err := engineQueries.GetActivityTask(ctx, fixture.Task.ID)
	require.NoError(t, err)
	historyRows, err := engineQueries.GetHistoryByRun(ctx, fixture.Run.ID)
	require.NoError(t, err)
	retryScheduled := false
	for i := range historyRows {
		retryScheduled = retryScheduled || historyRows[i].EventType == publichistory.EventActivityRetryScheduled
	}
	if terminateFirst {
		require.Equal(t, http.StatusConflict, reported.rec.Code, reported.rec.Body.String())
		assert.Equal(t, enginedb.EngineActivityTaskStatusCancelled, task.Status)
		assert.False(t, retryScheduled, "a report after the terminate must not schedule a retry")
		return
	}
	require.Equal(t, http.StatusNoContent, reported.rec.Code, reported.rec.Body.String())
	assert.Equal(t, report.taskStatus, task.Status)
	assert.Equal(t, report.retried, retryScheduled)
}

//nolint:revive // Keep testing.T first in test helper signatures.
func startRemoteActivityReport(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	server *Server,
	projectID uuid.UUID,
	taskID uuid.UUID,
	report *remoteActivityReport,
) *terminateWakeOp {
	t.Helper()
	body, err := json.Marshal(report.body)
	require.NoError(t, err)
	return startTerminateWakeOp(t, cancel, func(op *terminateWakeOp) error {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost,
			"/v1/engine/activities/"+taskID.String()+"/"+report.path, bytes.NewReader(body))
		req.Header.Set(enginePreviewHeader, "1")
		req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
		op.rec = httptest.NewRecorder()
		report.serve(server, op.rec, req, taskID)
		return nil
	})
}
