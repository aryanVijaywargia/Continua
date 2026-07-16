package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
)

func TestGetEngineRunPendingWorkExposesExecutionTargetAndClaimedBy(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-pending-work-target",
		RequestKey:        "req-pending-work-target",
	}))
	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        start.RunId,
	})
	require.NoError(t, err)

	baseTime := time.Now().UTC().Add(-time.Minute).Round(time.Microsecond)
	localHistory := appendEngineHistoryEvent(
		t,
		ctx,
		engineQueries,
		projectID,
		run.InstanceID,
		start.RunId,
		2,
		publichistory.EventActivityScheduled,
		publichistory.ActivityScheduledPayload{
			ActivityKey:  "local-task",
			ActivityType: "demo.activity",
			Input:        []byte(`{"target":"local"}`),
		},
	)
	remoteHistory := appendEngineHistoryEvent(
		t,
		ctx,
		engineQueries,
		projectID,
		run.InstanceID,
		start.RunId,
		3,
		publichistory.EventActivityScheduled,
		publichistory.ActivityScheduledPayload{
			ActivityKey:  "remote-task",
			ActivityType: "demo.activity",
			Input:        []byte(`{"target":"remote"}`),
		},
	)

	_, err = engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      run.InstanceID,
		RunID:           start.RunId,
		HistoryID:       &localHistory.ID,
		ActivityKey:     "local-task",
		ActivityType:    "demo.activity",
		Input:           []byte(`{"target":"local"}`),
		AvailableAt:     baseTime,
		ExecutionTarget: "local",
		MaxAttempts:     1,
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      run.InstanceID,
		RunID:           start.RunId,
		HistoryID:       &remoteHistory.ID,
		ActivityKey:     "remote-task",
		ActivityType:    "demo.activity",
		Input:           []byte(`{"target":"remote"}`),
		AvailableAt:     baseTime.Add(time.Second),
		ExecutionTarget: "remote",
		MaxAttempts:     1,
	})
	require.NoError(t, err)

	claimedBy := "worker-remote-1"
	leaseDurationMillis := int64(60_000)
	claimed, err := engineQueries.ClaimRemoteActivityTasks(ctx, enginedb.ClaimRemoteActivityTasksParams{
		ClaimedBy:       &claimedBy,
		LeaseDurationMs: &leaseDurationMillis,
		ProjectFilterID: projectID,
		ActivityTypes:   []string{"demo.activity"},
		MaxTasks:        1,
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, "remote-task", claimed[0].ActivityKey)

	rec := invokeEngineBoundaryReadAsOperator(
		t,
		server,
		engineBoundaryPendingWork,
		start.RunId,
		"",
		&projectID,
	)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	resp := decodeJSONBody[EnginePendingWorkResponse](t, rec)
	require.Len(t, resp.Activities, 2)

	activitiesByKey := make(map[string]EnginePendingActivityItem, len(resp.Activities))
	for _, activity := range resp.Activities {
		activitiesByKey[activity.ActivityKey] = activity
	}
	require.Contains(t, activitiesByKey, "local-task")
	require.Contains(t, activitiesByKey, "remote-task")

	local := activitiesByKey["local-task"]
	assert.Equal(t, "queued", local.Status)
	assert.Equal(t, EnginePendingActivityItemExecutionTarget("local"), local.ExecutionTarget)
	assert.Nil(t, local.ClaimedBy)

	remote := activitiesByKey["remote-task"]
	assert.Equal(t, "claimed", remote.Status)
	assert.Equal(t, EnginePendingActivityItemExecutionTarget("remote"), remote.ExecutionTarget)
	require.NotNil(t, remote.ClaimedBy)
	assert.Equal(t, "worker-remote-1", *remote.ClaimedBy)
}
