package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestEngineRouteAvailabilityMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("disabled engine routes return 404", func(t *testing.T) {
		handler := engineRouteAvailabilityMiddleware(&Server{})(next)
		req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+uuid.NewString(), nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestEnginePreviewHeaderMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("preview header is required on mutating routes", func(t *testing.T) {
		handler := enginePreviewHeaderMiddleware()(next)
		req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs", bytes.NewReader([]byte(`{}`)))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		resp := decodeJSONBody[Error](t, rec)
		assert.Equal(t, "preview_header_required", resp.Code)
	})

	t.Run("get routes pass through without preview header", func(t *testing.T) {
		handler := enginePreviewHeaderMiddleware()(next)
		req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+uuid.NewString(), nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestNewRouter_DisabledEngineRoutesReturn404BeforeAuth(t *testing.T) {
	ctx := context.Background()
	platformStore := store.New(testutil.TestDB(t))
	server := NewServer(platformStore, nil)

	apiKey := "engine-router-" + uuid.NewString()
	project, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "router-test-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, project.ID)

	handler := NewRouter(server, platformStore)
	runID := uuid.NewString()

	testCases := []struct {
		name    string
		header  string
		value   string
		want    int
		message string
	}{
		{name: "missing api key", want: http.StatusNotFound},
		{name: "invalid api key", header: "X-API-Key", value: "invalid-key", want: http.StatusNotFound},
		{name: "valid api key", header: "X-API-Key", value: apiKey, want: http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID, nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, tc.want, rec.Code)
		})
	}
}

func TestNewRouter_EnabledEngineRoutesStillAuthenticate(t *testing.T) {
	ctx := context.Background()
	platformStore := store.New(testutil.TestDB(t))
	server := NewServer(platformStore, nil)
	server.enginePublicAPIEnabled = true

	apiKey := "engine-router-enabled-" + uuid.NewString()
	_, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "router-enabled-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)

	handler := NewRouter(server, platformStore)
	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+uuid.NewString(), nil)
	req.Header.Set("X-API-Key", "invalid-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "invalid_api_key", decodeJSONBody[map[string]string](t, rec)["code"])
}

func TestStartEngineRun_CreatesProjectedShellAndReplaysDedupe(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	existingSession, err := platformStore.Queries().GetOrCreateSessionByExternalID(ctx, platformdb.GetOrCreateSessionByExternalIDParams{
		ProjectID:  projectID,
		ExternalID: "checkout-session",
	})
	require.NoError(t, err)

	_, err = platformStore.Queries().UpdateSession(ctx, platformdb.UpdateSessionParams{
		ID:       existingSession.ID,
		Name:     testutil.StrPtr("Before"),
		Metadata: []byte(`{"preserve":"yes","replace":"before"}`),
	})
	require.NoError(t, err)

	req := EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-1",
		RequestKey:        "req-1",
		Input:             map[string]any{"cart_id": "cart-123"},
		Session: &EngineStartSession{
			Key:      testutil.StrPtr("checkout-session"),
			Name:     testutil.StrPtr("Checkout"),
			Metadata: &map[string]any{"replace": "after", "new": "value"},
		},
		Trace: &EngineStartTrace{
			Name:   testutil.StrPtr("Checkout Workflow"),
			UserId: testutil.StrPtr("user-1"),
		},
	}

	first := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, req))
	deleted, err := engineQueries.DeleteDefinitionCatalogEntry(ctx, enginedb.DeleteDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	second := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, req))

	assert.Equal(t, first, second)
	assert.Equal(t, "instance-1", first.InstanceKey)
	assert.Equal(t, "engine:"+first.RunId.String(), first.TraceId)

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        uuid.UUID(first.RunId),
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, run.Status)

	history, err := engineQueries.GetHistoryByRun(ctx, uuid.UUID(first.RunId))
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, publichistory.EventWorkflowStarted, history[0].EventType)
	decodedStarted, err := publichistory.DecodePayload(history[0].EventType, history[0].Payload)
	require.NoError(t, err)
	startedPayload, ok := decodedStarted.(*publichistory.WorkflowStartedPayload)
	require.True(t, ok)
	assert.Equal(t, "checkout", startedPayload.DefinitionName)
	assert.Equal(t, "v1", startedPayload.DefinitionVersion)
	assert.Equal(t, "instance-1", startedPayload.InstanceKey)
	assert.JSONEq(t, `{"cart_id":"cart-123"}`, string(startedPayload.Input))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   first.TraceId,
	})
	require.NoError(t, err)
	assert.True(t, trace.EngineRunID.Valid)
	assert.Equal(t, uuid.UUID(first.RunId), uuid.UUID(trace.EngineRunID.Bytes))
	require.NotNil(t, trace.EngineInstanceKey)
	assert.Equal(t, "instance-1", *trace.EngineInstanceKey)
	require.NotNil(t, trace.EngineRunStatus)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusQueued), *trace.EngineRunStatus)
	require.NotNil(t, trace.EnginePendingActivityTasks)
	require.NotNil(t, trace.EnginePendingInboxItems)
	assert.EqualValues(t, 0, *trace.EnginePendingActivityTasks)
	assert.EqualValues(t, 0, *trace.EnginePendingInboxItems)
	require.NotNil(t, trace.EngineDefinitionName)
	assert.Equal(t, "checkout", *trace.EngineDefinitionName)
	require.NotNil(t, trace.EngineDefinitionVersion)
	assert.Equal(t, "v1", *trace.EngineDefinitionVersion)
	require.NotNil(t, trace.EngineProjectionState)
	assert.Equal(t, publicprojection.StateUpToDate.String(), *trace.EngineProjectionState)
	require.NotNil(t, trace.EngineLatestHistoryID)
	require.NotNil(t, trace.EngineLastProjectedHistoryID)
	assert.Equal(t, history[0].ID, *trace.EngineLatestHistoryID)
	assert.Equal(t, history[0].ID, *trace.EngineLastProjectedHistoryID)

	session, err := platformStore.Queries().GetSession(ctx, trace.SessionID.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "checkout-session", session.ExternalID)
	require.NotNil(t, session.Name)
	assert.Equal(t, "Checkout", *session.Name)

	var metadata map[string]any
	require.NoError(t, json.Unmarshal(session.Metadata, &metadata))
	assert.Equal(t, "yes", metadata["preserve"])
	assert.Equal(t, "after", metadata["replace"])
	assert.Equal(t, "value", metadata["new"])

	spans, err := platformStore.ListSpansByTrace(ctx, trace.ID)
	require.NoError(t, err)
	require.Len(t, spans, 1)
	assert.Equal(t, "engine:root:"+first.RunId.String(), spans[0].SpanID)
}

func TestStartEngineRun_RejectsUnknownDefinitionAndInstanceConflict(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)

	t.Run("unknown definition is rejected before rows are created", func(t *testing.T) {
		rec := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
			DefinitionName:    "missing",
			DefinitionVersion: "v1",
			InstanceKey:       "instance-missing",
			RequestKey:        "req-missing",
		})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		resp := decodeJSONBody[Error](t, rec)
		assert.Equal(t, "definition_not_registered", resp.Code)

		_, err := engineQueries.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
			ProjectID:   projectID,
			InstanceKey: "instance-missing",
		})
		require.True(t, errors.Is(err, pgx.ErrNoRows))
	})

	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	first := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-locked",
		RequestKey:        "req-1",
	})
	require.Equal(t, http.StatusOK, first.Code)

	conflict := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-locked",
		RequestKey:        "req-2",
	})
	require.Equal(t, http.StatusConflict, conflict.Code)
	resp := decodeJSONBody[Error](t, conflict)
	assert.Equal(t, "instance_conflict", resp.Code)

	deleted, err := engineQueries.DeleteDefinitionCatalogEntry(ctx, enginedb.DeleteDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	replayedConflict := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-locked",
		RequestKey:        "req-2",
	})
	require.Equal(t, http.StatusConflict, replayedConflict.Code)
	replayedResp := decodeJSONBody[Error](t, replayedConflict)
	assert.Equal(t, "instance_conflict", replayedResp.Code)
}

func TestStartEngineRun_RejectsMissingRequiredFields(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	type counts struct {
		instances int
		runs      int
		history   int
		dedupe    int
	}

	countProjectRows := func() counts {
		var result counts
		err := platformStore.Pool().QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM engine.instances WHERE project_id = $1),
				(SELECT COUNT(*) FROM engine.runs WHERE project_id = $1),
				(SELECT COUNT(*) FROM engine.history WHERE project_id = $1),
				(SELECT COUNT(*) FROM engine.request_dedupe WHERE project_id = $1)
		`, projectID).Scan(&result.instances, &result.runs, &result.history, &result.dedupe)
		require.NoError(t, err)
		return result
	}

	baseReq := EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-required",
		RequestKey:        "req-required",
	}

	testCases := []struct {
		name   string
		mutate func(*EngineStartRunRequest)
	}{
		{
			name: "missing instance key",
			mutate: func(req *EngineStartRunRequest) {
				req.InstanceKey = ""
			},
		},
		{
			name: "missing definition name",
			mutate: func(req *EngineStartRunRequest) {
				req.DefinitionName = ""
			},
		},
		{
			name: "missing definition version",
			mutate: func(req *EngineStartRunRequest) {
				req.DefinitionVersion = ""
			},
		},
		{
			name: "missing request key",
			mutate: func(req *EngineStartRunRequest) {
				req.RequestKey = ""
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := baseReq
			tc.mutate(&req)
			before := countProjectRows()

			rec := invokeStartEngineRun(t, server, projectID, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			resp := decodeJSONBody[Error](t, rec)
			assert.Equal(t, "invalid_request", resp.Code)
			assert.Equal(t, before, countProjectRows())
		})
	}
}

func TestStartEngineRun_ProjectScopedDedupeDoesNotLeakAcrossProjects(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectAID := setupEngineHandlerTest(t)
	projectBID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	req := EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "shared-instance",
		RequestKey:        "shared-request",
	}

	startA := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectAID, req))
	startB := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectBID, req))
	replayA := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectAID, req))
	replayB := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectBID, req))

	assert.NotEqual(t, startA.RunId, startB.RunId)
	assert.Equal(t, startA, replayA)
	assert.Equal(t, startB, replayB)

	instanceA, err := engineQueries.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectAID,
		InstanceKey: req.InstanceKey,
	})
	require.NoError(t, err)
	instanceB, err := engineQueries.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectBID,
		InstanceKey: req.InstanceKey,
	})
	require.NoError(t, err)
	assert.NotEqual(t, instanceA.ID, instanceB.ID)
}

func TestEngineHandlers_ReadSignalCancelAndTraceSurfaces(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-read",
		RequestKey:        "req-read",
	}))

	runID := uuid.UUID(start.RunId)
	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	instanceRec := invokeGetEngineInstance(t, server, projectID, "instance-read")
	require.Equal(t, http.StatusOK, instanceRec.Code)
	instanceResp := decodeJSONBody[EngineInstanceResponse](t, instanceRec)
	assert.Equal(t, start.RunId, instanceResp.CurrentRun.RunId)
	assert.Equal(t, UpToDate, instanceResp.CurrentRun.ProjectionState)

	runRec := invokeGetEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, runRec.Code)
	runResp := decodeJSONBody[EngineRunResponse](t, runRec)
	assert.Equal(t, start.RunId, runResp.RunId)
	assert.Equal(t, UpToDate, runResp.ProjectionState)

	historyRec := invokeGetEngineRunHistory(t, server, projectID, runID, GetEngineRunHistoryParams{})
	require.Equal(t, http.StatusOK, historyRec.Code)
	historyResp := decodeJSONBody[EngineRunHistoryResponse](t, historyRec)
	require.Len(t, historyResp.Events, 1)
	assert.Equal(t, publichistory.EventWorkflowStarted, historyResp.Events[0].EventType)

	resultRec := invokeGetEngineRunResult(t, server, projectID, runID)
	require.Equal(t, http.StatusConflict, resultRec.Code)
	assert.Equal(t, "run_not_terminal", decodeJSONBody[Error](t, resultRec).Code)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	notFoundRec := invokeGetEngineRun(t, server, otherProjectID, runID)
	require.Equal(t, http.StatusNotFound, notFoundRec.Code)
	notFoundSignalRec := invokeSignalEngineRun(t, server, otherProjectID, runID, EngineSignalRunRequest{SignalName: "approval"})
	require.Equal(t, http.StatusNotFound, notFoundSignalRec.Code)
	notFoundCancelRec := invokeCancelEngineRun(t, server, otherProjectID, runID)
	require.Equal(t, http.StatusNotFound, notFoundCancelRec.Code)

	traceList := decodeJSONBody[TraceList](t, invokeListTraces(t, server, projectID, ListTracesParams{}))
	require.Len(t, traceList.Traces, 1)
	require.NotNil(t, traceList.Traces[0].Engine)
	assert.Equal(t, start.RunId, traceList.Traces[0].Engine.RunId)

	traceDetail := decodeJSONBody[TraceDetail](t, invokeGetTrace(t, server, projectID, trace.ID))
	require.NotNil(t, traceDetail.Engine)
	assert.Equal(t, start.RunId, traceDetail.Engine.RunId)
	assert.Equal(t, "instance-read", traceDetail.Engine.InstanceKey)

	timeline := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{}))
	require.NotNil(t, timeline.Engine)
	assert.Equal(t, UpToDate, timeline.Engine.ProjectionState)

	waitingFor, err := json.Marshal(map[string]any{"kind": "signal", "signal_name": "approval"})
	require.NoError(t, err)
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'waiting',
		    waiting_for = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, runID, waitingFor)
	require.NoError(t, err)

	signalRec := invokeSignalEngineRun(t, server, projectID, runID, EngineSignalRunRequest{SignalName: "approval"})
	require.Equal(t, http.StatusOK, signalRec.Code)
	signalResp := decodeJSONBody[EngineControlResponse](t, signalRec)
	assert.True(t, signalResp.Accepted)
	assert.True(t, signalResp.WakeApplied)

	runAfterSignal, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, runAfterSignal.Status)

	originalEngineControl := server.engineControl
	server.engineControl = nil
	projectedAfterSignal := decodeJSONBody[TraceDetail](t, invokeGetTrace(t, server, projectID, trace.ID))
	server.engineControl = originalEngineControl
	require.NotNil(t, projectedAfterSignal.Engine)
	assert.Equal(t, EngineRunStatusQUEUED, projectedAfterSignal.Engine.Status)
	assert.Nil(t, projectedAfterSignal.Engine.WaitState)
	assert.Equal(t, 0, projectedAfterSignal.Engine.PendingWork.PendingActivityTasks)
	assert.Equal(t, 1, projectedAfterSignal.Engine.PendingWork.PendingInboxItems)

	cancelRec := invokeCancelEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, cancelRec.Code)
	cancelResp := decodeJSONBody[EngineControlResponse](t, cancelRec)
	assert.True(t, cancelResp.Accepted)
	assert.False(t, cancelResp.WakeApplied)

	openInboxBeforeReplay, err := engineQueries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
	require.NoError(t, err)

	cancelReplayRec := invokeCancelEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, cancelReplayRec.Code)

	openInboxAfterReplay, err := engineQueries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
	require.NoError(t, err)
	assert.Equal(t, openInboxBeforeReplay, openInboxAfterReplay)

	server.engineControl = nil
	projectedAfterCancel := decodeJSONBody[TraceDetail](t, invokeGetTrace(t, server, projectID, trace.ID))
	server.engineControl = originalEngineControl
	require.NotNil(t, projectedAfterCancel.Engine)
	assert.Equal(t, EngineRunStatusQUEUED, projectedAfterCancel.Engine.Status)
	assert.Equal(t, int(openInboxAfterReplay), projectedAfterCancel.Engine.PendingWork.PendingInboxItems)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    waiting_for = NULL,
		    result = '{"ok":true}',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, runID)
	require.NoError(t, err)

	terminalSignalRec := invokeSignalEngineRun(t, server, projectID, runID, EngineSignalRunRequest{SignalName: "ignored"})
	require.Equal(t, http.StatusConflict, terminalSignalRec.Code)
	assert.Equal(t, "run_terminal", decodeJSONBody[Error](t, terminalSignalRec).Code)
}

func TestGetEngineRunResult_ReturnsCancelledFailureSummary(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-cancelled-result",
		RequestKey:        "req-cancelled-result",
	}))

	runID := uuid.UUID(start.RunId)
	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'cancelled',
		    result = NULL,
		    waiting_for = NULL,
		    completed_at = NOW(),
		    last_error_code = 'cancelled',
		    last_error_message = 'workflow cancelled',
		    updated_at = NOW()
		WHERE id = $1
	`, runID)
	require.NoError(t, err)

	rec := invokeGetEngineRunResult(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[EngineRunResultResponse](t, rec)
	assert.Equal(t, EngineRunStatusCANCELLED, resp.Status)
	require.NotNil(t, resp.Failure)
	assert.Equal(t, "cancelled", resp.Failure.ErrorCode)
	assert.Equal(t, "workflow cancelled", resp.Failure.ErrorMessage)
	assert.Nil(t, resp.Result)
}

func TestGetTrace_UsesLiveFallbackOnlyForNonCurrentProjectionStates(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	testCases := []struct {
		name            string
		projectionState string
		wantStatus      EngineRunStatus
		wantWaitState   bool
		projectedOnly   bool
	}{
		{
			name:            "up_to_date serves projected summary only",
			projectionState: publicprojection.StateUpToDate.String(),
			wantStatus:      EngineRunStatusWAITING,
			wantWaitState:   true,
			projectedOnly:   true,
		},
		{
			name:            "catching_up supplements from live engine summary",
			projectionState: publicprojection.StateCatchingUp.String(),
			wantStatus:      EngineRunStatusWAITING,
			wantWaitState:   true,
		},
		{
			name:            "summary_only supplements from live engine summary",
			projectionState: publicprojection.StateSummaryOnly.String(),
			wantStatus:      EngineRunStatusWAITING,
			wantWaitState:   true,
		},
		{
			name:            "journal_expired stays on projected summary shell",
			projectionState: publicprojection.StateJournalExpired.String(),
			wantStatus:      EngineRunStatusWAITING,
			wantWaitState:   true,
			projectedOnly:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestKey := "req-" + uuid.NewString()
			instanceKey := "instance-" + uuid.NewString()[:8]
			sessionKey := "session-" + uuid.NewString()[:8]

			start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
				DefinitionName:    "checkout",
				DefinitionVersion: "v1",
				InstanceKey:       instanceKey,
				RequestKey:        requestKey,
				Session: &EngineStartSession{
					Key: testutil.StrPtr(sessionKey),
				},
			}))

			runID := uuid.UUID(start.RunId)
			trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
				ProjectID: projectID,
				TraceID:   start.TraceId,
			})
			require.NoError(t, err)

			setEngineRunWaiting(t, ctx, platformStore, runID, "approval", map[string]any{"step": "live"})
			createPendingWorkForRun(t, ctx, engineQueries, projectID, runID)
			setTraceProjectionState(t, ctx, platformStore, trace.ID, tc.projectionState)
			setProjectedEngineSummary(t, ctx, platformStore, trace.ID, projectedEngineSummaryUpdate{
				RunStatus:            string(enginedb.EngineRunLifecycleStatusWaiting),
				CustomStatus:         []byte(`{"step":"projected"}`),
				WaitState:            []byte(`{"kind":"signal","signal_name":"approval"}`),
				PendingActivityTasks: 2,
				PendingInboxItems:    3,
			})

			originalEngineControl := server.engineControl
			if tc.projectedOnly {
				server.engineControl = nil
			} else {
				server.engineControl = originalEngineControl
			}
			t.Cleanup(func() {
				server.engineControl = originalEngineControl
			})

			detail := decodeJSONBody[TraceDetail](t, invokeGetTrace(t, server, projectID, trace.ID))
			require.NotNil(t, detail.Engine)
			assert.Equal(t, instanceKey, detail.Engine.InstanceKey)
			assert.Equal(t, engineProjectionStateFromString(tc.projectionState), detail.Engine.ProjectionState)
			assert.Equal(t, tc.wantStatus, detail.Engine.Status)
			require.NotNil(t, detail.Engine.CustomStatus)

			if tc.wantWaitState {
				require.NotNil(t, detail.Engine.WaitState)
				require.NotNil(t, detail.Engine.WaitState.SignalName)
				assert.Equal(t, "approval", *detail.Engine.WaitState.SignalName)
			} else {
				assert.Nil(t, detail.Engine.WaitState)
			}

			if tc.projectedOnly {
				assert.Equal(t, "projected", (*detail.Engine.CustomStatus)["step"])
				assert.Equal(t, 2, detail.Engine.PendingWork.PendingActivityTasks)
				assert.Equal(t, 3, detail.Engine.PendingWork.PendingInboxItems)
			} else {
				assert.Equal(t, "live", (*detail.Engine.CustomStatus)["step"])
				assert.Equal(t, 1, detail.Engine.PendingWork.PendingActivityTasks)
				assert.Equal(t, 1, detail.Engine.PendingWork.PendingInboxItems)
			}
		})
	}
}

func TestGetTrace_LiveFallbackFailureReturns500(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishDefinition(ctx, engineQueries, "checkout", "v1"))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-fallback",
		RequestKey:        "req-fallback",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	setTraceProjectionState(t, ctx, platformStore, trace.ID, publicprojection.StateCatchingUp.String())
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET engine_run_id = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, trace.ID, uuid.New())
	require.NoError(t, err)

	rec := invokeGetTrace(t, server, projectID, trace.ID)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "internal_error", resp.Code)
	assert.Equal(t, "Failed to read engine summary", resp.Message)
}

func setupEngineHandlerTest(t *testing.T) (context.Context, *store.Store, *enginedb.Queries, *Server, uuid.UUID) {
	t.Helper()

	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)
	server.engineControl = newEngineControlService(platformStore)
	server.enginePublicAPIEnabled = true

	projectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	return ctx, platformStore, enginedb.New(pool), server, projectID
}

func publishDefinition(ctx context.Context, queries *enginedb.Queries, name, version string) error {
	_, err := queries.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    name,
		DefinitionVersion: version,
	})
	return err
}

func invokeStartEngineRun(t *testing.T, server *Server, projectID uuid.UUID, req EngineStartRunRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/engine/runs", bytes.NewReader(body))
	httpReq.Header.Set(enginePreviewHeader, "1")
	httpReq = httpReq.WithContext(context.WithValue(httpReq.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.StartEngineRun(rec, httpReq, StartEngineRunParams{XContinuaEnginePreview: testutil.StrPtr("1")})
	return rec
}

func invokeGetEngineInstance(t *testing.T, server *Server, projectID uuid.UUID, instanceKey string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/instances/"+instanceKey, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.GetEngineInstance(rec, req, instanceKey)
	return rec
}

func invokeGetEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.GetEngineRun(rec, req, openapi_types.UUID(runID))
	return rec
}

func invokeGetEngineRunHistory(
	t *testing.T,
	server *Server,
	projectID, runID uuid.UUID,
	params GetEngineRunHistoryParams,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID.String()+"/history", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.GetEngineRunHistory(rec, req, openapi_types.UUID(runID), params)
	return rec
}

func invokeGetEngineRunResult(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID.String()+"/result", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.GetEngineRunResult(rec, req, openapi_types.UUID(runID))
	return rec
}

func invokeSignalEngineRun(
	t *testing.T,
	server *Server,
	projectID, runID uuid.UUID,
	reqBody EngineSignalRunRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/signal", bytes.NewReader(body))
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.SignalEngineRun(rec, req, openapi_types.UUID(runID), SignalEngineRunParams{XContinuaEnginePreview: testutil.StrPtr("1")})
	return rec
}

func invokeCancelEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/cancel", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.CancelEngineRun(rec, req, openapi_types.UUID(runID), CancelEngineRunParams{XContinuaEnginePreview: testutil.StrPtr("1")})
	return rec
}

func setEngineRunWaiting(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	runID uuid.UUID,
	signalName string,
	customStatus map[string]any,
) {
	t.Helper()

	waitingFor, err := json.Marshal(map[string]any{"kind": "signal", "signal_name": signalName})
	require.NoError(t, err)
	customStatusRaw, err := json.Marshal(customStatus)
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'waiting',
		    waiting_for = $2,
		    custom_status = $3,
		    updated_at = NOW()
		WHERE id = $1
	`, runID, waitingFor, customStatusRaw)
	require.NoError(t, err)
}

type projectedEngineSummaryUpdate struct {
	RunStatus            string
	CustomStatus         []byte
	WaitState            []byte
	PendingActivityTasks int64
	PendingInboxItems    int64
}

func setProjectedEngineSummary(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	traceID uuid.UUID,
	update projectedEngineSummaryUpdate,
) {
	t.Helper()

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET engine_run_status = $2,
		    engine_custom_status = $3,
		    engine_wait_state = $4,
		    engine_pending_activity_tasks = $5,
		    engine_pending_inbox_items = $6,
		    updated_at = NOW()
		WHERE id = $1
	`, traceID, update.RunStatus, update.CustomStatus, update.WaitState, update.PendingActivityTasks, update.PendingInboxItems)
	require.NoError(t, err)
}

func createPendingWorkForRun(
	t *testing.T,
	ctx context.Context,
	engineQueries *enginedb.Queries,
	projectID, runID uuid.UUID,
) {
	t.Helper()

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)

	history, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.NotEmpty(t, history)

	_, err = engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    projectID,
		InstanceID:   run.InstanceID,
		RunID:        runID,
		HistoryID:    &history[0].ID,
		ActivityKey:  "approval-task",
		ActivityType: "demo.activity",
		Input:        []byte(`{"ok":true}`),
		AvailableAt:  history[0].CreatedAt,
	})
	require.NoError(t, err)

	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		Kind:        "signal",
		Payload:     []byte(`{"signal_name":"approval"}`),
		AvailableAt: history[0].CreatedAt,
	})
	require.NoError(t, err)
}

func setTraceProjectionState(t *testing.T, ctx context.Context, platformStore *store.Store, traceID uuid.UUID, projectionState string) {
	t.Helper()

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET engine_projection_state = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, traceID, projectionState)
	require.NoError(t, err)
}

func hashTestAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
