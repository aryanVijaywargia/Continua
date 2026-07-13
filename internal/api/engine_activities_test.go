package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

type remoteActivityTestTask struct {
	Instance enginedb.EngineInstance
	Run      enginedb.EngineRun
	Task     enginedb.EngineActivityTask
}

type createRemoteActivityTestTaskParams struct {
	ProjectID       uuid.UUID
	ActivityKey     string
	ActivityType    string
	ExecutionTarget string
	AvailableAt     time.Time
	MaxAttempts     int32
	Waiting         bool
}

func TestRemoteActivityClaimHeartbeatAndScoping(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	now := time.Now().UTC()

	local := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "local-task",
		ActivityType:    "email.send",
		ExecutionTarget: "local",
		AvailableAt:     now.Add(-time.Minute),
		MaxAttempts:     1,
	})
	remote := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "remote-task",
		ActivityType:    "email.send",
		ExecutionTarget: "remote",
		AvailableAt:     now.Add(-time.Minute),
		MaxAttempts:     1,
	})
	future := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "future-task",
		ActivityType:    "email.send",
		ExecutionTarget: "remote",
		AvailableAt:     now.Add(time.Hour),
		MaxAttempts:     1,
	})
	crossProject := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       otherProjectID,
		ActivityKey:     "cross-project-task",
		ActivityType:    "email.send",
		ExecutionTarget: "remote",
		AvailableAt:     now.Add(-time.Minute),
		MaxAttempts:     1,
	})

	claimRec := invokeClaimRemoteActivityTasks(t, server, projectID, EngineRemoteActivityClaimRequest{
		WorkerId:      " worker-a ",
		ActivityTypes: []string{" email.send "},
		LeaseDuration: testutil.Ptr("3s"),
		MaxTasks:      testutil.Ptr(50),
	})
	require.Equal(t, http.StatusOK, claimRec.Code)
	claimResp := decodeJSONBody[EngineRemoteActivityClaimResponse](t, claimRec)
	require.Len(t, claimResp.Tasks, 1)
	assert.Equal(t, remote.Task.ID, claimResp.Tasks[0].TaskId)
	assert.Equal(t, int64(10000), claimResp.Tasks[0].EffectiveLeaseDurationMs)

	claimedRemote, err := engineQueries.GetActivityTask(ctx, remote.Task.ID)
	require.NoError(t, err)
	require.NotNil(t, claimedRemote.ClaimedBy)
	assert.Equal(t, "worker-a", *claimedRemote.ClaimedBy)
	assert.Equal(t, int32(1), claimedRemote.AttemptCount)
	require.NotNil(t, claimedRemote.LeaseDurationMs)
	assert.Equal(t, int64(10000), *claimedRemote.LeaseDurationMs)

	localAfter, err := engineQueries.GetActivityTask(ctx, local.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusQueued, localAfter.Status)
	futureAfter, err := engineQueries.GetActivityTask(ctx, future.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusQueued, futureAfter.Status)
	crossAfter, err := engineQueries.GetActivityTask(ctx, crossProject.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusQueued, crossAfter.Status)

	emptyRec := invokeClaimRemoteActivityTasks(t, server, projectID, EngineRemoteActivityClaimRequest{
		WorkerId:      "worker-a",
		ActivityTypes: []string{"image.process"},
	})
	require.Equal(t, http.StatusOK, emptyRec.Code)
	assert.Empty(t, decodeJSONBody[EngineRemoteActivityClaimResponse](t, emptyRec).Tasks)

	heartbeatRec := invokeHeartbeatRemoteActivityTask(t, server, projectID, remote.Task.ID, "worker-a")
	require.Equal(t, http.StatusOK, heartbeatRec.Code)
	heartbeatResp := decodeJSONBody[EngineRemoteActivityHeartbeatResponse](t, heartbeatRec)
	assert.Equal(t, int64(10000), heartbeatResp.EffectiveLeaseDurationMs)
	assert.True(t, heartbeatResp.LeaseExpiresAt.After(time.Now()))

	require.Equal(t, http.StatusConflict, invokeHeartbeatRemoteActivityTask(t, server, projectID, remote.Task.ID, "worker-b").Code)
	require.Equal(t, http.StatusConflict, invokeHeartbeatRemoteActivityTask(t, server, projectID, local.Task.ID, "worker-a").Code)
	require.Equal(t, http.StatusNotFound, invokeHeartbeatRemoteActivityTask(t, server, projectID, crossProject.Task.ID, "worker-a").Code)
	require.Equal(t, http.StatusNotFound, invokeHeartbeatRemoteActivityTask(t, server, projectID, uuid.New(), "worker-a").Code)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.activity_tasks
		SET lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, remote.Task.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, invokeHeartbeatRemoteActivityTask(t, server, projectID, remote.Task.ID, "worker-a").Code)

	reclaimRec := invokeClaimRemoteActivityTasks(t, server, projectID, EngineRemoteActivityClaimRequest{
		WorkerId:      "worker-b",
		ActivityTypes: []string{"email.send"},
	})
	require.Equal(t, http.StatusOK, reclaimRec.Code)
	reclaimResp := decodeJSONBody[EngineRemoteActivityClaimResponse](t, reclaimRec)
	require.Len(t, reclaimResp.Tasks, 1)
	assert.Equal(t, remote.Task.ID, reclaimResp.Tasks[0].TaskId)
	reclaimed, err := engineQueries.GetActivityTask(ctx, remote.Task.ID)
	require.NoError(t, err)
	require.NotNil(t, reclaimed.ClaimedBy)
	assert.Equal(t, "worker-b", *reclaimed.ClaimedBy)
	assert.Equal(t, int32(2), reclaimed.AttemptCount)

	staleCompleteRec := invokeCompleteRemoteActivityTask(t, server, projectID, remote.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"stale": true},
	})
	require.Equal(t, http.StatusConflict, staleCompleteRec.Code)

	maxLeaseTask := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "max-lease-task",
		ActivityType:    "video.render",
		ExecutionTarget: "remote",
		AvailableAt:     now.Add(-time.Minute),
		MaxAttempts:     1,
	})
	maxLeaseRec := invokeClaimRemoteActivityTasks(t, server, projectID, EngineRemoteActivityClaimRequest{
		WorkerId:      "worker-max",
		ActivityTypes: []string{maxLeaseTask.Task.ActivityType},
		LeaseDuration: testutil.Ptr("10m"),
	})
	require.Equal(t, http.StatusOK, maxLeaseRec.Code)
	maxLeaseResp := decodeJSONBody[EngineRemoteActivityClaimResponse](t, maxLeaseRec)
	require.Len(t, maxLeaseResp.Tasks, 1)
	assert.Equal(t, int64(300000), maxLeaseResp.Tasks[0].EffectiveLeaseDurationMs)

	localFailRec := invokeFailRemoteActivityTask(t, server, projectID, local.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-a",
		ErrorCode:    "local_target",
		ErrorMessage: "local target",
	})
	require.Equal(t, http.StatusConflict, localFailRec.Code)
}

func TestRemoteActivityClaimRequestValidation(t *testing.T) {
	_, _, _, server, projectID := setupEngineHandlerTest(t)
	longWorkerID := strings.Repeat("w", 129)
	longActivityType := strings.Repeat("a", 257)
	tooManyActivityTypes := make([]string, remoteActivityMaxActivityTypes+1)
	for i := range tooManyActivityTypes {
		tooManyActivityTypes[i] = "activity." + strconv.Itoa(i)
	}

	testCases := []struct {
		name string
		req  EngineRemoteActivityClaimRequest
	}{
		{name: "blank worker", req: EngineRemoteActivityClaimRequest{WorkerId: " ", ActivityTypes: []string{"email.send"}}},
		{name: "long worker", req: EngineRemoteActivityClaimRequest{WorkerId: longWorkerID, ActivityTypes: []string{"email.send"}}},
		{name: "empty activity types", req: EngineRemoteActivityClaimRequest{WorkerId: "worker-a"}},
		{name: "too many activity types", req: EngineRemoteActivityClaimRequest{WorkerId: "worker-a", ActivityTypes: tooManyActivityTypes}},
		{name: "blank activity type", req: EngineRemoteActivityClaimRequest{WorkerId: "worker-a", ActivityTypes: []string{" "}}},
		{name: "long activity type", req: EngineRemoteActivityClaimRequest{WorkerId: "worker-a", ActivityTypes: []string{longActivityType}}},
		{name: "invalid lease", req: EngineRemoteActivityClaimRequest{WorkerId: "worker-a", ActivityTypes: []string{"email.send"}, LeaseDuration: testutil.Ptr("soon")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeClaimRemoteActivityTasks(t, server, projectID, tc.req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "invalid_request", decodeJSONBody[Error](t, rec).Code)
		})
	}

	taskID := uuid.New()
	require.Equal(t, http.StatusBadRequest, invokeHeartbeatRemoteActivityTask(t, server, projectID, taskID, " ").Code)
	require.Equal(t, http.StatusBadRequest, invokeCompleteRemoteActivityTask(t, server, projectID, taskID, EngineRemoteActivityCompleteRequest{
		WorkerId: longWorkerID,
		Output:   map[string]any{"ok": true},
	}).Code)
	require.Equal(t, http.StatusBadRequest, invokeFailRemoteActivityTask(t, server, projectID, taskID, EngineRemoteActivityFailRequest{
		WorkerId:     " ",
		ErrorCode:    "failed",
		ErrorMessage: "failed",
	}).Code)
}

func TestRemoteActivityRoutesRequireAuthAndPreview(t *testing.T) {
	ctx, platformStore, _, server, _ := setupEngineHandlerTest(t)
	apiKey := "remote-activity-router-" + uuid.NewString()
	_, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "remote-activity-router-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)
	handler := newAuthenticatedRouter(t, server, platformStore)
	body := []byte(`{"worker_id":"worker-a","activity_types":["email.send"]}`)

	t.Run("missing api key is rejected before handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/claim", bytes.NewReader(body))
		req.Header.Set(enginePreviewHeader, "1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "missing_api_key", decodeJSONBody[map[string]string](t, rec)["code"])
	})

	t.Run("preview header is required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/claim", bytes.NewReader(body))
		req.Header.Set("X-API-Key", apiKey)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "preview_header_required", decodeJSONBody[Error](t, rec).Code)
	})

	t.Run("authenticated preview request reaches claim handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/claim", bytes.NewReader(body))
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set(enginePreviewHeader, "1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, decodeJSONBody[EngineRemoteActivityClaimResponse](t, rec).Tasks)
	})
}

func TestRemoteActivityCompleteAndDuplicateConflict(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "complete-task",
		ActivityType:    "email.send",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     1,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-a", fixture.Task.ActivityType)

	completeRec := invokeCompleteRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"ok": true},
	})
	require.Equal(t, http.StatusNoContent, completeRec.Code)

	task, err := engineQueries.GetActivityTask(ctx, fixture.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusCompleted, task.Status)
	assert.JSONEq(t, `{"ok":true}`, string(task.Output))

	run, err := engineQueries.GetRun(ctx, fixture.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, run.Status)

	duplicateRec := invokeCompleteRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"ok": false},
	})
	require.Equal(t, http.StatusConflict, duplicateRec.Code)
	require.Equal(t, http.StatusConflict, invokeHeartbeatRemoteActivityTask(t, server, projectID, fixture.Task.ID, "worker-a").Code)
	unchanged, err := engineQueries.GetActivityTask(ctx, fixture.Task.ID)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(unchanged.Output))

	failCompletedRec := invokeFailRemoteActivityTask(t, server, projectID, fixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-a",
		ErrorCode:    "already_completed",
		ErrorMessage: "already completed",
	})
	require.Equal(t, http.StatusConflict, failCompletedRec.Code)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	crossProjectRec := invokeCompleteRemoteActivityTask(t, server, otherProjectID, fixture.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"ok": true},
	})
	require.Equal(t, http.StatusNotFound, crossProjectRec.Code)

	missingCompleteRec := invokeCompleteRemoteActivityTask(t, server, projectID, uuid.New(), EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"ok": true},
	})
	require.Equal(t, http.StatusNotFound, missingCompleteRec.Code)
}

func TestCompleteRemoteActivityTaskHonorsCompletionGrace(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)

	createExpiredClaim := func(activityKey string) remoteActivityTestTask {
		t.Helper()
		fixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
			ProjectID:       projectID,
			ActivityKey:     activityKey,
			ActivityType:    "email." + activityKey,
			ExecutionTarget: "remote",
			AvailableAt:     time.Now().Add(-time.Minute),
			MaxAttempts:     1,
			Waiting:         true,
		})
		claimRemoteActivityForTest(t, server, projectID, "worker-a", fixture.Task.ActivityType)
		_, err := platformStore.Pool().Exec(ctx, `
			UPDATE engine.activity_tasks
			SET lease_expires_at = NOW() - INTERVAL '10 seconds'
			WHERE id = $1
		`, fixture.Task.ID)
		require.NoError(t, err)
		return fixture
	}

	withinGrace := createExpiredClaim("complete-with-grace")
	server.engineControl.completionGrace = 30 * time.Second
	graceRec := invokeCompleteRemoteActivityTask(t, server, projectID, withinGrace.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"grace": true},
	})
	require.Equal(t, http.StatusNoContent, graceRec.Code)
	completed, err := engineQueries.GetActivityTask(ctx, withinGrace.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusCompleted, completed.Status)

	zeroGrace := createExpiredClaim("complete-without-grace")
	server.engineControl.completionGrace = 0
	zeroGraceRec := invokeCompleteRemoteActivityTask(t, server, projectID, zeroGrace.Task.ID, EngineRemoteActivityCompleteRequest{
		WorkerId: "worker-a",
		Output:   map[string]any{"grace": false},
	})
	require.Equal(t, http.StatusConflict, zeroGraceRec.Code)
	unchanged, err := engineQueries.GetActivityTask(ctx, zeroGrace.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusClaimed, unchanged.Status)
	require.NotNil(t, unchanged.ClaimedBy)
	assert.Equal(t, "worker-a", *unchanged.ClaimedBy)
}

func TestRemoteActivityFailRetryAndNonRetryable(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	retryFixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "retry-task",
		ActivityType:    "email.send",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     3,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-a", retryFixture.Task.ActivityType)

	longMessage := strings.Repeat("m", remoteActivityErrorMessageMaxLength+10)
	retryRec := invokeFailRemoteActivityTask(t, server, projectID, retryFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-a",
		ErrorCode:    "temporary_error",
		ErrorMessage: longMessage,
	})
	require.Equal(t, http.StatusNoContent, retryRec.Code)

	retried, err := engineQueries.GetActivityTask(ctx, retryFixture.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusQueued, retried.Status)
	assert.Equal(t, "temporary_error", *retried.LastErrorCode)
	require.NotNil(t, retried.LastErrorMessage)
	assert.Len(t, *retried.LastErrorMessage, remoteActivityErrorMessageMaxLength)
	assert.True(t, retried.AvailableAt.After(time.Now()))

	historyRows, err := engineQueries.GetHistoryByRun(ctx, retryFixture.Run.ID)
	require.NoError(t, err)
	require.NotEmpty(t, historyRows)
	assert.Equal(t, publichistory.EventActivityRetryScheduled, historyRows[len(historyRows)-1].EventType)

	duplicateRetryRec := invokeFailRemoteActivityTask(t, server, projectID, retryFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-a",
		ErrorCode:    "temporary_error",
		ErrorMessage: "again",
	})
	require.Equal(t, http.StatusConflict, duplicateRetryRec.Code)

	staleFixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "stale-fail-task",
		ActivityType:    "report.build",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     1,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "stale-worker", staleFixture.Task.ActivityType)
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.activity_tasks
		SET lease_expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, staleFixture.Task.ID)
	require.NoError(t, err)
	claimRemoteActivityForTest(t, server, projectID, "fresh-worker", staleFixture.Task.ActivityType)
	staleFailRec := invokeFailRemoteActivityTask(t, server, projectID, staleFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "stale-worker",
		ErrorCode:    "stale",
		ErrorMessage: "stale",
	})
	require.Equal(t, http.StatusConflict, staleFailRec.Code)

	earlyReclaim := invokeClaimRemoteActivityTasks(t, server, projectID, EngineRemoteActivityClaimRequest{
		WorkerId:      "worker-b",
		ActivityTypes: []string{retryFixture.Task.ActivityType},
	})
	require.Equal(t, http.StatusOK, earlyReclaim.Code)
	assert.Empty(t, decodeJSONBody[EngineRemoteActivityClaimResponse](t, earlyReclaim).Tasks)

	nonRetryableFixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "non-retryable-task",
		ActivityType:    "image.process",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     3,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-c", nonRetryableFixture.Task.ActivityType)

	terminalRec := invokeFailRemoteActivityTask(t, server, projectID, nonRetryableFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-c",
		ErrorCode:    "bad_input",
		ErrorMessage: "invalid",
		NonRetryable: testutil.Ptr(true),
	})
	require.Equal(t, http.StatusNoContent, terminalRec.Code)
	failed, err := engineQueries.GetActivityTask(ctx, nonRetryableFixture.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusFailed, failed.Status)

	exhaustedFixture := createRemoteActivityTestTask(t, ctx, platformStore, engineQueries, createRemoteActivityTestTaskParams{
		ProjectID:       projectID,
		ActivityKey:     "retry-exhausted-task",
		ActivityType:    "once.only",
		ExecutionTarget: "remote",
		AvailableAt:     time.Now().Add(-time.Minute),
		MaxAttempts:     1,
		Waiting:         true,
	})
	claimRemoteActivityForTest(t, server, projectID, "worker-d", exhaustedFixture.Task.ActivityType)
	exhaustedRec := invokeFailRemoteActivityTask(t, server, projectID, exhaustedFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-d",
		ErrorCode:    "still_retryable",
		ErrorMessage: "attempts exhausted",
	})
	require.Equal(t, http.StatusNoContent, exhaustedRec.Code)
	exhausted, err := engineQueries.GetActivityTask(ctx, exhaustedFixture.Task.ID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineActivityTaskStatusFailed, exhausted.Status)
	assert.Equal(t, "still_retryable", *exhausted.LastErrorCode)

	invalidCodeRec := invokeFailRemoteActivityTask(t, server, projectID, nonRetryableFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-c",
		ErrorCode:    strings.Repeat("e", remoteActivityErrorCodeMaxLength+1),
		ErrorMessage: "invalid",
	})
	require.Equal(t, http.StatusBadRequest, invalidCodeRec.Code)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	crossProjectFailRec := invokeFailRemoteActivityTask(t, server, otherProjectID, nonRetryableFixture.Task.ID, EngineRemoteActivityFailRequest{
		WorkerId:     "worker-c",
		ErrorCode:    "cross_project",
		ErrorMessage: "cross project",
	})
	require.Equal(t, http.StatusNotFound, crossProjectFailRec.Code)

	missingFailRec := invokeFailRemoteActivityTask(t, server, projectID, uuid.New(), EngineRemoteActivityFailRequest{
		WorkerId:     "worker-c",
		ErrorCode:    "missing",
		ErrorMessage: "missing",
	})
	require.Equal(t, http.StatusNotFound, missingFailRec.Code)
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createRemoteActivityTestTask(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	engineQueries *enginedb.Queries,
	params createRemoteActivityTestTaskParams,
) remoteActivityTestTask {
	t.Helper()

	instance, err := engineQueries.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      params.ProjectID,
		InstanceKey:    "remote-activity-" + params.ActivityKey + "-" + uuid.NewString(),
		DefinitionName: "remote.activity.test",
		Metadata:       []byte(`{"source":"test"}`),
	})
	require.NoError(t, err)
	run, err := engineQueries.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:          params.ProjectID,
		InstanceID:         instance.ID,
		RunNumber:          1,
		DefinitionVersion:  "v1",
		ReadyAt:            time.Now().Add(-time.Minute),
		ContinuedFromRunID: pgtype.UUID{},
	})
	require.NoError(t, err)
	startedHistory := appendEngineHistoryEvent(t, ctx, engineQueries, params.ProjectID, instance.ID, run.ID, 1, publichistory.EventWorkflowStarted, publichistory.WorkflowStartedPayload{
		DefinitionName:    instance.DefinitionName,
		DefinitionVersion: run.DefinitionVersion,
		InstanceKey:       instance.InstanceKey,
		Input:             []byte(`{"source":"test"}`),
	})
	createEngineProjectedTraceShell(t, ctx, platformStore, instance, run, startedHistory.ID, startedHistory.CreatedAt)
	if params.Waiting {
		setEngineRunWaiting(t, ctx, platformStore, run.ID, map[string]any{"remote": true})
	}
	activityHistory := appendEngineHistoryEvent(t, ctx, engineQueries, params.ProjectID, instance.ID, run.ID, 2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  params.ActivityKey,
		ActivityType: params.ActivityType,
		Input:        []byte(`{"source":"test"}`),
	})

	maxAttempts := params.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 1
	}
	var initialBackoffMS *int64
	var maxBackoffMS *int64
	var backoffMultiplier *float64
	if maxAttempts > 1 {
		initialBackoffMS = testutil.Ptr(int64(1000))
		maxBackoffMS = testutil.Ptr(int64(5000))
		backoffMultiplier = testutil.Ptr(2.0)
	}
	task, err := engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:         params.ProjectID,
		InstanceID:        instance.ID,
		RunID:             run.ID,
		HistoryID:         &activityHistory.ID,
		ActivityKey:       params.ActivityKey,
		ActivityType:      params.ActivityType,
		Input:             []byte(`{"source":"test"}`),
		AvailableAt:       params.AvailableAt,
		ExecutionTarget:   params.ExecutionTarget,
		MaxAttempts:       maxAttempts,
		InitialBackoffMs:  initialBackoffMS,
		MaxBackoffMs:      maxBackoffMS,
		BackoffMultiplier: backoffMultiplier,
	})
	require.NoError(t, err)

	return remoteActivityTestTask{Instance: instance, Run: run, Task: task}
}

func claimRemoteActivityForTest(t *testing.T, server *Server, projectID uuid.UUID, workerID, activityType string) {
	t.Helper()
	rec := invokeClaimRemoteActivityTasks(t, server, projectID, EngineRemoteActivityClaimRequest{
		WorkerId:      workerID,
		ActivityTypes: []string{activityType},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[EngineRemoteActivityClaimResponse](t, rec)
	require.Len(t, resp.Tasks, 1)
}

func invokeClaimRemoteActivityTasks(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	reqBody EngineRemoteActivityClaimRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/claim", bytes.NewReader(body))
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.ClaimRemoteActivityTasks(rec, req, ClaimRemoteActivityTasksParams{XContinuaEnginePreview: "1"})
	return rec
}

func invokeHeartbeatRemoteActivityTask(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	taskID uuid.UUID,
	workerID string,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(EngineRemoteActivityHeartbeatRequest{WorkerId: workerID})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/"+taskID.String()+"/heartbeat", bytes.NewReader(body))
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.HeartbeatRemoteActivityTask(rec, req, taskID, HeartbeatRemoteActivityTaskParams{XContinuaEnginePreview: "1"})
	return rec
}

func invokeCompleteRemoteActivityTask(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	taskID uuid.UUID,
	reqBody EngineRemoteActivityCompleteRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/"+taskID.String()+"/complete", bytes.NewReader(body))
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.CompleteRemoteActivityTask(rec, req, taskID, CompleteRemoteActivityTaskParams{XContinuaEnginePreview: "1"})
	return rec
}

func invokeFailRemoteActivityTask(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	taskID uuid.UUID,
	reqBody EngineRemoteActivityFailRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/activities/"+taskID.String()+"/fail", bytes.NewReader(body))
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.FailRemoteActivityTask(rec, req, taskID, FailRemoteActivityTaskParams{XContinuaEnginePreview: "1"})
	return rec
}
