package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
)

func TestEngineStartRunEmitsRunsNotify(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	listener := listenForAPINotifications(t, platformStore.Pool(), publicnotify.ChannelRuns)

	rec := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "notify-start-instance",
		RequestKey:        "notify-start-request",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assertAPINotificationChannels(t, listener, publicnotify.ChannelRuns)
}

func TestEngineSignalRunEmitsInboxAndWakeNotify(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "notify-signal-instance",
		RequestKey:        "notify-signal-request",
	}))

	waitingFor, err := json.Marshal(map[string]any{"kind": "signal", "signal_name": "approval"})
	require.NoError(t, err)
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'waiting',
		    waiting_for = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, start.RunId, waitingFor)
	require.NoError(t, err)

	listener := listenForAPINotifications(t, platformStore.Pool(),
		publicnotify.ChannelInbox,
		publicnotify.ChannelRuns,
	)
	signalRec := invokeSignalEngineRun(t, server, projectID, start.RunId, EngineSignalRunRequest{
		SignalName: "approval",
		Payload:    map[string]any{"approved": true},
	})
	require.Equal(t, http.StatusOK, signalRec.Code)
	require.True(t, decodeJSONBody[EngineControlResponse](t, signalRec).WakeApplied)
	assertAPINotificationChannels(t, listener,
		publicnotify.ChannelInbox,
		publicnotify.ChannelRuns,
	)
}

func TestEngineResumeRunEmitsRunsNotify(t *testing.T) {
	for _, status := range []string{"suspended", "quarantined"} {
		t.Run(status, func(t *testing.T) {
			ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
			require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
			start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
				DefinitionName:    "checkout",
				DefinitionVersion: "v1",
				InstanceKey:       "notify-resume-" + status,
				RequestKey:        "notify-resume-request-" + status,
			}))

			if status == "suspended" {
				require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)
			} else {
				_, err := platformStore.Pool().Exec(ctx, `
					UPDATE engine.runs
					SET status = 'quarantined',
					    last_error_code = 'test_quarantine',
					    last_error_message = 'test quarantine',
					    updated_at = NOW()
					WHERE id = $1
				`, start.RunId)
				require.NoError(t, err)
			}

			listener := listenForAPINotifications(t, platformStore.Pool(), publicnotify.ChannelRuns)
			rec := invokeResumeEngineRun(t, server, projectID, start.RunId)
			require.Equal(t, http.StatusOK, rec.Code)
			assertAPINotificationChannels(t, listener, publicnotify.ChannelRuns)
		})
	}
}

func TestEngineCancelRunEmitsInboxNotifyOnlyForNewIntent(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "notify-cancel-instance",
		RequestKey:        "notify-cancel-request",
	}))
	listener := listenForAPINotifications(t, platformStore.Pool(),
		publicnotify.ChannelInbox,
		publicnotify.ChannelRuns,
	)

	rec := invokeCancelEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)
	assertAPINotificationChannels(t, listener, publicnotify.ChannelInbox)
	assertAPINotificationsQuiet(t, listener)

	rec = invokeCancelEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)
	assertAPINotificationsQuiet(t, listener)
}

func TestEngineRemoteActivityCompletionEmitsRunsNotify(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "notify-complete-task",
		ActivityType:    "notify.complete",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     1,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "notify-worker", fixture.Task.ActivityType)
	listener := listenForAPINotifications(t, platformStore.Pool(), publicnotify.ChannelRuns)

	rec := invokeCompleteRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "notify-worker",
		Output:   map[string]any{"ok": true},
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
	assertAPINotificationChannels(t, listener, publicnotify.ChannelRuns)
}

func listenForAPINotifications(t *testing.T, pool *pgxpool.Pool, channels ...string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.ConnectConfig(context.Background(), pool.Config().ConnConfig.Copy())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	for _, channel := range channels {
		_, err := conn.Exec(context.Background(), "LISTEN "+pgx.Identifier{channel}.Sanitize())
		require.NoError(t, err)
	}
	return conn
}

func assertAPINotificationChannels(t *testing.T, conn *pgx.Conn, channels ...string) {
	t.Helper()

	want := make(map[string]bool, len(channels))
	for _, channel := range channels {
		want[channel] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for len(want) > 0 {
		notification, err := conn.WaitForNotification(ctx)
		require.NoError(t, err, "missing notification channels: %v", want)
		require.Empty(t, notification.Payload, "notification payload on %s must be wake-only", notification.Channel)
		delete(want, notification.Channel)
	}
}

func assertAPINotificationsQuiet(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if notification, err := conn.WaitForNotification(ctx); err == nil {
		t.Fatalf("unexpected notification on channel %q", notification.Channel)
	} else {
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	}
}
