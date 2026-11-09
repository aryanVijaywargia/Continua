package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/store"
)

// A run that completes or fails cancels its open activity tasks in the same transaction
// (#359; the activator side is TestFinishedRunCancelsOpenWork in the engine module). A
// remote worker that still holds such a task gets 409 for every report, so a fully
// purged run stays purged: the trace keeps journal_expired and the run gets no history.
func TestRemoteActivityReportsOnFinishedRunConflict(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	fixture, traceID := finishedRunWithClaimedRemoteTask(t, ctx, platformStore, engineQueries, server, projectID)
	_, err := engineQueries.CancelOpenActivityTasksByRun(ctx, fixture.Run.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, invokePurgeEngineRun(t, server, projectID, fixture.Run.ID, Full).Code)

	nonRetryable := true
	reports := map[string]func() int{
		"heartbeat": func() int {
			return invokeHeartbeatRemoteActivityTask(t, server, projectID, fixture.Task.ID, "worker-a").Code
		},
		"complete": func() int {
			return invokeCompleteRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityCompleteRequest{
				WorkerId: "worker-a",
				Output:   map[string]any{"ok": true},
			}).Code
		},
		"fail with retry": func() int {
			return invokeFailRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityFailRequest{
				WorkerId: "worker-a", ErrorCode: "temporary", ErrorMessage: "try again",
			}).Code
		},
		"fail terminally": func() int {
			return invokeFailRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityFailRequest{
				WorkerId: "worker-a", ErrorCode: "fatal", ErrorMessage: "gave up", NonRetryable: &nonRetryable,
			}).Code
		},
	}
	for name, report := range reports {
		assert.Equal(t, http.StatusConflict, report(), name)
	}

	task, err := engineQueries.GetActivityTask(ctx, fixture.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusCancelled, task.Status)
	assert.Equal(t, string(JournalExpired), tracePurgeState(t, ctx, platformStore, traceID))
	history, err := engineQueries.GetHistoryByRun(ctx, fixture.Run.ID)
	require.NoError(t, err)
	assert.Empty(t, history, "a late report must not append history to the purged run")
}

// A run that finished before the #359 fix can still hold a claimed task, and a retry
// report on it still appends a history row. The projection writer must not undo a purge
// for that row: the projector skips a purged trace, so catching_up would never clear.
func TestRemoteRetryReportKeepsPurgedProjection(t *testing.T) {
	for _, mode := range []EnginePurgeMode{Full, ProjectionOnly} {
		t.Run(string(mode), func(t *testing.T) {
			ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
			fixture, traceID := finishedRunWithClaimedRemoteTask(t, ctx, platformStore, engineQueries, server, projectID)
			purge := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(t, server, projectID, fixture.Run.ID, mode))
			require.True(t, purge.Deleted)

			rec := invokeFailRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityFailRequest{
				WorkerId: "worker-a", ErrorCode: "temporary", ErrorMessage: "try again",
			})
			require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

			assert.Equal(t, string(purge.ProjectionState), tracePurgeState(t, ctx, platformStore, traceID))
		})
	}
}

// finishedRunWithClaimedRemoteTask returns a completed run whose retryable remote task
// worker-a still holds, as a completed decision left it before the #359 fix.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func finishedRunWithClaimedRemoteTask(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	engineQueries *enginedb.Queries,
	server *Server,
	projectID uuid.UUID,
) (remoteActivityTestTask, uuid.UUID) {
	t.Helper()
	fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "finished-run-task",
		ActivityType:    "finished.run.task",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     3,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-a", fixture.Task.ActivityType)
	var traceID uuid.UUID
	require.NoError(t, platformStore.Pool().QueryRow(ctx, `SELECT id FROM public.traces WHERE engine_run_id = $1`, fixture.Run.ID).Scan(&traceID))
	markEngineRunCompleted(t, ctx, platformStore, fixture.Run.ID, traceID)
	return fixture, traceID
}

//nolint:revive // Keep testing.T first in test helper signatures.
func tracePurgeState(t *testing.T, ctx context.Context, platformStore *store.Store, traceID uuid.UUID) string {
	t.Helper()
	var state string
	require.NoError(t, platformStore.Pool().QueryRow(ctx, `SELECT engine_projection_state FROM public.traces WHERE id = $1`, traceID).Scan(&state))
	return state
}
