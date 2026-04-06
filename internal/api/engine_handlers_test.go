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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
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
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

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
		ID:        first.RunId,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, run.Status)

	history, err := engineQueries.GetHistoryByRun(ctx, first.RunId)
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
	assert.Equal(t, first.RunId, uuid.UUID(trace.EngineRunID.Bytes))
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

	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

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
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

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
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

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
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-read",
		RequestKey:        "req-read",
	}))

	runID := start.RunId
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
	assert.EqualValues(t, 0, projectedAfterSignal.Engine.PendingWork.PendingActivityTasks)
	assert.EqualValues(t, 1, projectedAfterSignal.Engine.PendingWork.PendingInboxItems)

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
	assert.EqualValues(t, openInboxAfterReplay, projectedAfterCancel.Engine.PendingWork.PendingInboxItems)

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
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-cancelled-result",
		RequestKey:        "req-cancelled-result",
	}))

	runID := start.RunId
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
	body := append([]byte(nil), rec.Body.Bytes()...)

	resp := decodeJSONBody[EngineRunResultResponse](t, rec)
	assert.Equal(t, EngineRunStatusCANCELLED, resp.Status)
	require.NotNil(t, resp.Failure)
	assert.Equal(t, "cancelled", resp.Failure.ErrorCode)
	assert.Equal(t, "workflow cancelled", resp.Failure.ErrorMessage)
	assert.Nil(t, resp.Result)
	assertJSONFieldNull(t, body, "result")
}

func TestTerminateEngineRun_ReturnsTerminatedSummaryAndIsIdempotent(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminated-result",
		RequestKey:        "req-terminated-result",
	}))

	runID := start.RunId
	rec := invokeTerminateEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, rec.Code)
	body := append([]byte(nil), rec.Body.Bytes()...)

	resp := decodeJSONBody[EngineRunResultResponse](t, rec)
	assert.Equal(t, EngineRunStatusTERMINATED, resp.Status)
	require.NotNil(t, resp.Failure)
	assert.Equal(t, "terminated", resp.Failure.ErrorCode)
	assert.Equal(t, "run terminated by operator", resp.Failure.ErrorMessage)
	assert.Nil(t, resp.Result)
	assertJSONFieldNull(t, body, "result")

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusTerminated, run.Status)

	instance, err := engineQueries.GetInstance(ctx, run.InstanceID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineInstanceLifecycleStatusTerminated, instance.Status)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.Len(t, historyRows, 2)
	assert.Equal(t, publichistory.EventWorkflowTerminated, historyRows[1].EventType)
	decoded, err := publichistory.DecodePayload(historyRows[1].EventType, historyRows[1].Payload)
	require.NoError(t, err)
	terminatedPayload, ok := decoded.(*publichistory.WorkflowTerminatedPayload)
	require.True(t, ok)
	assert.Equal(t, "terminated", terminatedPayload.ErrorCode)
	assert.Equal(t, "run terminated by operator", terminatedPayload.ErrorMessage)

	resultRec := invokeGetEngineRunResult(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, resultRec.Code)
	resultResp := decodeJSONBody[EngineRunResultResponse](t, resultRec)
	assert.Equal(t, EngineRunStatusTERMINATED, resultResp.Status)
	require.NotNil(t, resultResp.Failure)
	assert.Equal(t, "terminated", resultResp.Failure.ErrorCode)
	assert.Equal(t, "run terminated by operator", resultResp.Failure.ErrorMessage)
	assert.Nil(t, resultResp.Result)

	signalRec := invokeSignalEngineRun(t, server, projectID, runID, EngineSignalRunRequest{SignalName: "ignored"})
	require.Equal(t, http.StatusConflict, signalRec.Code)
	assert.Equal(t, "run_terminal", decodeJSONBody[Error](t, signalRec).Code)

	cancelRec := invokeCancelEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusConflict, cancelRec.Code)
	assert.Equal(t, "run_terminal", decodeJSONBody[Error](t, cancelRec).Code)

	replayedTerminate := invokeTerminateEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, replayedTerminate.Code)
	replayedResp := decodeJSONBody[EngineRunResultResponse](t, replayedTerminate)
	assert.Equal(t, resp, replayedResp)

	historyAfterReplay, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	assert.Len(t, historyAfterReplay, len(historyRows))

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	notFoundRec := invokeTerminateEngineRun(t, server, otherProjectID, runID)
	require.Equal(t, http.StatusNotFound, notFoundRec.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, notFoundRec).Code)

	missingRunRec := invokeTerminateEngineRun(t, server, projectID, uuid.New())
	require.Equal(t, http.StatusNotFound, missingRunRec.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, missingRunRec).Code)
}

func TestTerminateEngineRun_ReturnsExistingTerminalStateWhenActivationWinsRowLock(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminate-race-completed",
		RequestKey:        "req-terminate-race-completed",
	}))

	runID := start.RunId
	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'running',
		    claimed_by = 'worker-a',
		    claimed_at = NOW(),
		    lease_expires_at = NOW() + INTERVAL '1 minute',
		    updated_at = NOW()
		WHERE id = $1
	`, runID)
	require.NoError(t, err)

	tx, err := platformStore.Pool().BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	engineTx := enginedb.New(tx)
	lockedRun, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)

	terminateDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		terminateDone <- invokeTerminateEngineRun(t, server, projectID, runID)
	}()

	waitForEngineRunLockWaiter(t, platformStore.Pool())

	historyRows, err := engineTx.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	payload, err := publichistory.MarshalPayload(publichistory.WorkflowCompletedPayload{
		Result: []byte(`{"ok":true}`),
	})
	require.NoError(t, err)
	_, err = engineTx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: lockedRun.InstanceID,
		RunID:      lockedRun.ID,
		SequenceNo: historyRows[len(historyRows)-1].SequenceNo + 1,
		EventType:  publichistory.EventWorkflowCompleted,
		Payload:    payload,
	})
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    result = $2,
		    waiting_for = NULL,
		    completed_at = NOW(),
		    last_error_code = NULL,
		    last_error_message = NULL,
		    claimed_by = NULL,
		    claimed_at = NULL,
		    lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`, runID, []byte(`{"ok":true}`))
	require.NoError(t, err)
	_, err = engineTx.UpdateInstanceStatus(ctx, enginedb.UpdateInstanceStatusParams{
		ID:     lockedRun.InstanceID,
		Status: enginedb.EngineInstanceLifecycleStatusCompleted,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	var terminateRec *httptest.ResponseRecorder
	select {
	case terminateRec = <-terminateDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminate handler did not finish after activation commit")
	}

	require.Equal(t, http.StatusOK, terminateRec.Code)
	resp := decodeJSONBody[EngineRunResultResponse](t, terminateRec)
	assert.Equal(t, EngineRunStatusCOMPLETED, resp.Status)
	require.NotNil(t, resp.Result)
	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, result["ok"])
	assert.Nil(t, resp.Failure)

	historyAfter, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.Len(t, historyAfter, 2)
	assert.Equal(t, publichistory.EventWorkflowCompleted, historyAfter[1].EventType)
}

func TestTerminateEngineRun_ProjectorEventuallyProjectsTerminalCleanup(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminate-projector",
		RequestKey:        "req-terminate-projector",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	setEngineRunWaiting(t, ctx, platformStore, start.RunId, map[string]any{"step": "waiting"})
	setProjectedEngineSummary(t, ctx, platformStore, trace.ID, projectedEngineSummaryUpdate{
		RunStatus:            string(enginedb.EngineRunLifecycleStatusWaiting),
		CustomStatus:         []byte(`{"step":"projected"}`),
		WaitState:            []byte(`{"kind":"signal","signal_name":"approval"}`),
		PendingActivityTasks: 0,
		PendingInboxItems:    0,
	})

	rec := invokeTerminateEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)

	var beforeRunStatus string
	var beforeWaitState []byte
	err = platformStore.Pool().QueryRow(ctx, `
		SELECT engine_run_status, engine_wait_state
		FROM public.traces
		WHERE id = $1
	`, trace.ID).Scan(&beforeRunStatus, &beforeWaitState)
	require.NoError(t, err)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusWaiting), beforeRunStatus)
	require.NotEmpty(t, beforeWaitState)

	engineServe := startExternalEngineServeProcess(t)
	defer engineServe.stop(t)

	traceStatus, runStatus, waitState := waitForProjectedTraceTerminalSummary(t, platformStore.Pool(), trace.ID)
	assert.Equal(t, "failed", traceStatus)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusTerminated), runStatus)
	assert.Len(t, waitState, 0)
}

func TestTerminateEngineRun_RouterEnforcesPreviewHeaderAndAvailability(t *testing.T) {
	ctx := context.Background()
	platformStore := store.New(testutil.TestDB(t))
	engineQueries := enginedb.New(platformStore.Pool())

	apiKey := "engine-terminate-" + uuid.NewString()
	project, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "terminate-router-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	server := NewServer(platformStore, nil)
	server.engineControl = newEngineControlService(platformStore)
	server.enginePublicAPIEnabled = true

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, project.ID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "terminate-router-instance",
		RequestKey:        "terminate-router-request",
	}))

	router := NewRouter(server, platformStore)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+start.RunId.String()+"/terminate", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "preview_header_required", decodeJSONBody[Error](t, rec).Code)

	server.enginePublicAPIEnabled = false
	router = NewRouter(server, platformStore)

	req = httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+start.RunId.String()+"/terminate", nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set(enginePreviewHeader, "1")
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetEngineRunPendingWork_ReturnsTypedOpenWorkAndExcludesCancelRows(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-pending-work",
		RequestKey:        "req-pending-work",
	}))

	runID := start.RunId
	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)

	baseTime := time.Now().UTC().Round(time.Microsecond)
	activityHistoryA := appendEngineHistoryEvent(t, ctx, engineQueries, projectID, run.InstanceID, runID, 2, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "approval-a",
		ActivityType: "demo.activity",
		Input:        []byte(`{"step":"a"}`),
	})
	activityHistoryB := appendEngineHistoryEvent(t, ctx, engineQueries, projectID, run.InstanceID, runID, 3, publichistory.EventActivityScheduled, publichistory.ActivityScheduledPayload{
		ActivityKey:  "approval-b",
		ActivityType: "demo.activity",
		Input:        []byte(`{"step":"b"}`),
	})
	timerHistory := appendEngineHistoryEvent(t, ctx, engineQueries, projectID, run.InstanceID, runID, 4, publichistory.EventTimerScheduled, publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(3 * time.Minute),
	})

	_, err = engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    projectID,
		InstanceID:   run.InstanceID,
		RunID:        runID,
		HistoryID:    &activityHistoryB.ID,
		ActivityKey:  "approval-b",
		ActivityType: "demo.activity",
		Input:        []byte(`{"step":"b"}`),
		AvailableAt:  baseTime.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    projectID,
		InstanceID:   run.InstanceID,
		RunID:        runID,
		HistoryID:    &activityHistoryA.ID,
		ActivityKey:  "approval-a",
		ActivityType: "demo.activity",
		Input:        []byte(`{"step":"a"}`),
		AvailableAt:  baseTime.Add(1 * time.Minute),
	})
	require.NoError(t, err)

	timerPayload, err := publichistory.MarshalPayload(publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(3 * time.Minute),
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		HistoryID:   &timerHistory.ID,
		Kind:        "timer",
		Payload:     timerPayload,
		AvailableAt: baseTime.Add(3 * time.Minute),
	})
	require.NoError(t, err)

	signalPayload, err := publichistory.MarshalPayload(publichistory.SignalReceivedPayload{
		SignalName: "approval",
		Payload:    []byte(`{"approved":true}`),
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		Kind:        "signal",
		Payload:     signalPayload,
		AvailableAt: baseTime.Add(4 * time.Minute),
	})
	require.NoError(t, err)

	cancelPayload, err := publichistory.MarshalPayload(publichistory.CancelRequestedPayload{})
	require.NoError(t, err)
	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		Kind:        "cancel",
		Payload:     cancelPayload,
		AvailableAt: baseTime.Add(5 * time.Minute),
		DedupeKey:   testutil.StrPtr("cancel:" + runID.String()),
	})
	require.NoError(t, err)

	rec := invokeGetEngineRunPendingWork(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, rec.Code)
	body := append([]byte(nil), rec.Body.Bytes()...)

	resp := decodeJSONBody[EnginePendingWorkResponse](t, rec)
	assert.Equal(t, runID, resp.RunId)
	assert.Nil(t, resp.CurrentWait)
	assert.Len(t, resp.Activities, 2)
	assert.Equal(t, "approval-a", resp.Activities[0].ActivityKey)
	assert.Equal(t, "approval-b", resp.Activities[1].ActivityKey)
	assert.Equal(t, "deadline", resp.Timers[0].TimerKey)
	assert.Equal(t, "approval", resp.Signals[0].SignalName)
	assert.EqualValues(t, 2, resp.PendingActivityTasks)
	assert.EqualValues(t, 2, resp.PendingInboxItems)
	assertJSONFieldNull(t, body, "current_wait")

	openInbox, err := engineQueries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
	require.NoError(t, err)
	assert.EqualValues(t, 2, openInbox)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	notFound := invokeGetEngineRunPendingWork(t, server, otherProjectID, runID)
	require.Equal(t, http.StatusNotFound, notFound.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, notFound).Code)
}

func TestGetEngineRunPendingWork_ReturnsPureSignalWaitWithoutPreviewHeader(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-pending-wait",
		RequestKey:        "req-pending-wait",
	}))

	setEngineRunWaiting(t, ctx, platformStore, start.RunId, map[string]any{"step": "waiting"})

	rec := invokeGetEngineRunPendingWork(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[EnginePendingWorkResponse](t, rec)
	require.NotNil(t, resp.CurrentWait)
	require.NotNil(t, resp.CurrentWait.SignalName)
	assert.Equal(t, "approval", *resp.CurrentWait.SignalName)
	assert.Empty(t, resp.Activities)
	assert.Empty(t, resp.Timers)
	assert.Empty(t, resp.Signals)
	assert.EqualValues(t, 0, resp.PendingActivityTasks)
	assert.EqualValues(t, 0, resp.PendingInboxItems)

	apiKey := "engine-pending-work-" + uuid.NewString()
	project, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "pending-router-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	routerServer := NewServer(platformStore, nil)
	routerServer.engineControl = newEngineControlService(platformStore)
	routerServer.enginePublicAPIEnabled = true
	routerStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, routerServer, project.ID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "pending-router-instance",
		RequestKey:        "pending-router-request",
	}))

	router := NewRouter(routerServer, platformStore)
	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+routerStart.RunId.String()+"/pending-work", nil)
	req.Header.Set("X-API-Key", apiKey)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestEngineRunPendingWorkCountsStayConsistentAcrossRunReadAndProjectedSummary(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-pending-count-consistency",
		RequestKey:        "req-pending-count-consistency",
	}))

	runID := start.RunId
	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)

	baseTime := time.Now().UTC().Round(time.Microsecond)
	timerHistory := appendEngineHistoryEvent(t, ctx, engineQueries, projectID, run.InstanceID, runID, 2, publichistory.EventTimerScheduled, publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(3 * time.Minute),
	})
	timerPayload, err := publichistory.MarshalPayload(publichistory.TimerScheduledPayload{
		TimerKey: "deadline",
		DueAt:    baseTime.Add(3 * time.Minute),
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		HistoryID:   &timerHistory.ID,
		Kind:        "timer",
		Payload:     timerPayload,
		AvailableAt: baseTime.Add(3 * time.Minute),
	})
	require.NoError(t, err)

	signalPayload, err := publichistory.MarshalPayload(publichistory.SignalReceivedPayload{
		SignalName: "approval",
		Payload:    []byte(`{"approved":true}`),
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		Kind:        "signal",
		Payload:     signalPayload,
		AvailableAt: baseTime.Add(4 * time.Minute),
	})
	require.NoError(t, err)

	cancelPayload, err := publichistory.MarshalPayload(publichistory.CancelRequestedPayload{})
	require.NoError(t, err)
	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		Kind:        "cancel",
		Payload:     cancelPayload,
		AvailableAt: baseTime.Add(5 * time.Minute),
		DedupeKey:   testutil.StrPtr("cancel:" + runID.String()),
	})
	require.NoError(t, err)

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	engineServe := startExternalEngineServeProcess(t)
	defer engineServe.stop(t)
	waitForProjectedTracePendingInboxItems(t, ctx, platformStore.Pool(), trace.ID, timerHistory.ID, 2)

	openInbox, err := engineQueries.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: runID, Valid: true})
	require.NoError(t, err)
	assert.EqualValues(t, 2, openInbox)

	runRec := invokeGetEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, runRec.Code)
	runResp := decodeJSONBody[EngineRunResponse](t, runRec)
	assert.EqualValues(t, 2, runResp.PendingWork.PendingInboxItems)

	pendingRec := invokeGetEngineRunPendingWork(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, pendingRec.Code)
	pendingResp := decodeJSONBody[EnginePendingWorkResponse](t, pendingRec)
	assert.EqualValues(t, 2, pendingResp.PendingInboxItems)
	assert.Len(t, pendingResp.Timers, 1)
	assert.Len(t, pendingResp.Signals, 1)

	var projectedPendingInboxItems int64
	err = platformStore.Pool().QueryRow(ctx, `
		SELECT engine_pending_inbox_items
		FROM public.traces
		WHERE id = $1
	`, trace.ID).Scan(&projectedPendingInboxItems)
	require.NoError(t, err)
	assert.EqualValues(t, 2, projectedPendingInboxItems)
}

func TestGetTrace_UsesLiveFallbackForNonCurrentProjectionStatesAndStaleCheckpoints(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	testCases := []struct {
		name                 string
		projectionState      string
		wantStatus           EngineRunStatus
		wantProjectionState  EngineProjectionState
		wantWaitState        bool
		projectedOnly        bool
		disableEngineControl bool
		seedStaleHistory     bool
	}{
		{
			name:                 "up_to_date serves projected summary only",
			projectionState:      publicprojection.StateUpToDate.String(),
			wantStatus:           EngineRunStatusWAITING,
			wantProjectionState:  UpToDate,
			wantWaitState:        true,
			projectedOnly:        true,
			disableEngineControl: true,
		},
		{
			name:                "catching_up supplements from live engine summary",
			projectionState:     publicprojection.StateCatchingUp.String(),
			wantStatus:          EngineRunStatusWAITING,
			wantProjectionState: CatchingUp,
			wantWaitState:       true,
		},
		{
			name:                "summary_only supplements from live engine summary",
			projectionState:     publicprojection.StateSummaryOnly.String(),
			wantStatus:          EngineRunStatusWAITING,
			wantProjectionState: SummaryOnly,
			wantWaitState:       true,
		},
		{
			name:                "journal_expired stays on projected summary shell",
			projectionState:     publicprojection.StateJournalExpired.String(),
			wantStatus:          EngineRunStatusWAITING,
			wantProjectionState: JournalExpired,
			wantWaitState:       true,
			projectedOnly:       true,
		},
		{
			name:                "journal_expired ignores stale checkpoint and stays on projected summary shell",
			projectionState:     publicprojection.StateJournalExpired.String(),
			wantStatus:          EngineRunStatusWAITING,
			wantProjectionState: JournalExpired,
			wantWaitState:       true,
			projectedOnly:       true,
			seedStaleHistory:    true,
		},
		{
			name:                "up_to_date falls back live when history checkpoint is stale",
			projectionState:     publicprojection.StateUpToDate.String(),
			wantStatus:          EngineRunStatusWAITING,
			wantProjectionState: CatchingUp,
			wantWaitState:       true,
			seedStaleHistory:    true,
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

			runID := start.RunId
			trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
				ProjectID: projectID,
				TraceID:   start.TraceId,
			})
			require.NoError(t, err)
			run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
				ProjectID: projectID,
				ID:        runID,
			})
			require.NoError(t, err)

			setEngineRunWaiting(t, ctx, platformStore, runID, map[string]any{"step": "live"})
			createPendingWorkForRun(t, ctx, engineQueries, projectID, runID)
			if tc.seedStaleHistory {
				appendEngineHistoryEvent(t, ctx, engineQueries, projectID, run.InstanceID, runID, 2, publichistory.EventCustomStatusUpdated, publichistory.CustomStatusUpdatedPayload{
					Status: []byte(`{"step":"live"}`),
				})
			}
			setTraceProjectionState(t, ctx, platformStore, trace.ID, tc.projectionState)
			setProjectedEngineSummary(t, ctx, platformStore, trace.ID, projectedEngineSummaryUpdate{
				RunStatus:            string(enginedb.EngineRunLifecycleStatusWaiting),
				CustomStatus:         []byte(`{"step":"projected"}`),
				WaitState:            []byte(`{"kind":"signal","signal_name":"approval"}`),
				PendingActivityTasks: 2,
				PendingInboxItems:    3,
			})

			originalEngineControl := server.engineControl
			if tc.disableEngineControl {
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
			assert.Equal(t, tc.wantProjectionState, detail.Engine.ProjectionState)
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
				assert.EqualValues(t, 2, detail.Engine.PendingWork.PendingActivityTasks)
				assert.EqualValues(t, 3, detail.Engine.PendingWork.PendingInboxItems)
			} else {
				assert.Equal(t, "live", (*detail.Engine.CustomStatus)["step"])
				assert.EqualValues(t, 1, detail.Engine.PendingWork.PendingActivityTasks)
				assert.EqualValues(t, 1, detail.Engine.PendingWork.PendingInboxItems)
			}
		})
	}
}

func TestTraceMetadataReadSurfacesReportCatchingUpWhenHistoryCheckpointIsStale(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-trace-projection-stale",
		RequestKey:        "req-trace-projection-stale",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)
	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        start.RunId,
	})
	require.NoError(t, err)

	appendEngineHistoryEvent(t, ctx, engineQueries, projectID, run.InstanceID, run.ID, 2, publichistory.EventCustomStatusUpdated, publichistory.CustomStatusUpdatedPayload{
		Status: []byte(`{"step":"stale"}`),
	})

	traceList := decodeJSONBody[TraceList](t, invokeListTraces(t, server, projectID, ListTracesParams{}))
	require.Len(t, traceList.Traces, 1)
	require.NotNil(t, traceList.Traces[0].Engine)
	assert.Equal(t, CatchingUp, traceList.Traces[0].Engine.ProjectionState)

	timeline := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{}))
	require.NotNil(t, timeline.Engine)
	assert.Equal(t, CatchingUp, timeline.Engine.ProjectionState)
}

func TestGetTrace_LiveFallbackFailureReturns500(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

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
	t.Cleanup(func() {
		_, restoreErr := platformStore.Pool().Exec(ctx, `
			UPDATE traces
			SET engine_run_id = $2,
			    updated_at = NOW()
			WHERE id = $1
		`, trace.ID, start.RunId)
		require.NoError(t, restoreErr)
	})

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

func publishCheckoutDefinition(ctx context.Context, queries *enginedb.Queries) error {
	_, err := queries.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
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

	server.StartEngineRun(rec, httpReq, StartEngineRunParams{XContinuaEnginePreview: "1"})
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

	server.GetEngineRun(rec, req, runID)
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

	server.GetEngineRunHistory(rec, req, runID, params)
	return rec
}

func invokeGetEngineRunResult(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID.String()+"/result", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.GetEngineRunResult(rec, req, runID)
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

	server.SignalEngineRun(rec, req, runID, SignalEngineRunParams{XContinuaEnginePreview: "1"})
	return rec
}

func invokeCancelEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/cancel", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.CancelEngineRun(rec, req, runID, CancelEngineRunParams{XContinuaEnginePreview: "1"})
	return rec
}

func invokeGetEngineRunPendingWork(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID.String()+"/pending-work", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.GetEngineRunPendingWork(rec, req, runID)
	return rec
}

func invokeTerminateEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/terminate", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.TerminateEngineRun(rec, req, runID, TerminateEngineRunParams{XContinuaEnginePreview: "1"})
	return rec
}

func assertJSONFieldNull(t *testing.T, body []byte, field string) {
	t.Helper()

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &payload))

	value, ok := payload[field]
	require.True(t, ok, "expected field %q to be present in %s", field, string(body))
	assert.Equal(t, "null", string(bytes.TrimSpace(value)))
}

//nolint:revive // Keep testing.T first in test helper signatures.
func appendEngineHistoryEvent(
	t *testing.T,
	ctx context.Context,
	engineQueries *enginedb.Queries,
	projectID, instanceID, runID uuid.UUID,
	sequence int32,
	eventType string,
	payload any,
) enginedb.EngineHistory {
	t.Helper()

	raw, err := publichistory.MarshalPayload(payload)
	require.NoError(t, err)

	row, err := engineQueries.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instanceID,
		RunID:      runID,
		SequenceNo: sequence,
		EventType:  eventType,
		Payload:    raw,
	})
	require.NoError(t, err)
	return row
}

//nolint:revive // Keep testing.T first in test helper signatures.
func setEngineRunWaiting(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	runID uuid.UUID,
	customStatus map[string]any,
) {
	t.Helper()

	waitingFor, err := json.Marshal(map[string]any{"kind": "signal", "signal_name": "approval"})
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

//nolint:revive // Keep testing.T first in test helper signatures.
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

//nolint:revive // Keep testing.T first in test helper signatures.
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

//nolint:revive // Keep testing.T first in test helper signatures.
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

func waitForEngineRunLockWaiter(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		err := pool.QueryRow(context.Background(), `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND wait_event_type = 'Lock'
				  AND query LIKE '%FROM engine.runs%'
				  AND query LIKE '%FOR UPDATE%'
			)
		`).Scan(&waiting)
		require.NoError(t, err)
		if waiting {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for terminate transaction to block on engine.runs row lock")
}

type externalEngineServeProcess struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
	done   chan error
	stdout lockedBuffer
	stderr lockedBuffer
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func startExternalEngineServeProcess(t *testing.T) *externalEngineServeProcess {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/continua-engine", "serve")
	cmd.Dir = filepath.Join(apiTestRepoRoot(t), "engine")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		"ENGINE_DATABASE_URL="+apiTestDatabaseURL(),
		"DATABASE_URL=",
		"ENGINE_WORKFLOW_POLL_INTERVAL=50ms",
		"ENGINE_ACTIVITY_POLL_INTERVAL=5s",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=5s",
		"ENGINE_RUN_LEASE_TTL=500ms",
		"ENGINE_ACTIVITY_LEASE_TTL=500ms",
		"ENGINE_REQUEST_DEDUPE_TTL=2s",
	)

	process := &externalEngineServeProcess{
		cancel: cancel,
		cmd:    cmd,
		done:   make(chan error, 1),
	}
	cmd.Stdout = &process.stdout
	cmd.Stderr = &process.stderr

	require.NoError(t, cmd.Start())
	go func() {
		process.done <- cmd.Wait()
	}()
	return process
}

func (p *externalEngineServeProcess) stop(t *testing.T) {
	t.Helper()

	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}

	select {
	case err := <-p.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("engine serve process exited unexpectedly: %v\nstdout=%s\nstderr=%s", err, p.stdout.String(), p.stderr.String())
		}
		return
	default:
	}

	p.cancel()

	stopped, err := waitExternalEngineServeProcess(p.done, 2*time.Second)
	if !stopped {
		_ = signalProcessGroup(p.cmd.Process.Pid, syscall.SIGTERM)
		stopped, err = waitExternalEngineServeProcess(p.done, 2*time.Second)
	}

	if !stopped {
		_ = signalProcessGroup(p.cmd.Process.Pid, syscall.SIGKILL)
		stopped, err = waitExternalEngineServeProcess(p.done, 5*time.Second)
	}

	if !stopped {
		t.Fatalf("engine serve process did not stop\nstdout=%s\nstderr=%s", p.stdout.String(), p.stderr.String())
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("engine serve process exited with error: %v\nstdout=%s\nstderr=%s", err, p.stdout.String(), p.stderr.String())
		}
	}
}

func waitExternalEngineServeProcess(done <-chan error, timeout time.Duration) (bool, error) {
	select {
	case err := <-done:
		return true, err
	case <-time.After(timeout):
		return false, nil
	}
}

func signalProcessGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, sig)
}

func waitForProjectedTraceTerminalSummary(t *testing.T, pool *pgxpool.Pool, traceID uuid.UUID) (string, string, []byte) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var lastTraceStatus string
	var lastRunStatus string
	var lastWaitState []byte
	for time.Now().Before(deadline) {
		err := pool.QueryRow(context.Background(), `
			SELECT status, engine_run_status, engine_wait_state
			FROM public.traces
			WHERE id = $1
		`, traceID).Scan(&lastTraceStatus, &lastRunStatus, &lastWaitState)
		require.NoError(t, err)
		if lastTraceStatus == "failed" && lastRunStatus == string(enginedb.EngineRunLifecycleStatusTerminated) && len(lastWaitState) == 0 {
			return lastTraceStatus, lastRunStatus, lastWaitState
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for projected terminated trace summary, last trace=%q run=%q wait_state=%s", lastTraceStatus, lastRunStatus, string(lastWaitState))
	return "", "", nil
}

//nolint:revive // Keep testing.T first in test helper signatures.
func waitForProjectedTracePendingInboxItems(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	traceID uuid.UUID,
	expectedHistoryID int64,
	expectedPendingInboxItems int64,
) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var lastPendingInboxItems int64
	var lastLatestHistoryID int64
	var lastProjectedHistoryID int64
	for time.Now().Before(deadline) {
		err := pool.QueryRow(ctx, `
			SELECT engine_pending_inbox_items,
			       engine_latest_history_id,
			       engine_last_projected_history_id
			FROM public.traces
			WHERE id = $1
		`, traceID).Scan(&lastPendingInboxItems, &lastLatestHistoryID, &lastProjectedHistoryID)
		require.NoError(t, err)
		if lastPendingInboxItems == expectedPendingInboxItems &&
			lastLatestHistoryID >= expectedHistoryID &&
			lastProjectedHistoryID >= expectedHistoryID {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf(
		"timed out waiting for projected pending inbox count=%d at history id %d, last pending=%d latest=%d projected=%d",
		expectedPendingInboxItems,
		expectedHistoryID,
		lastPendingInboxItems,
		lastLatestHistoryID,
		lastProjectedHistoryID,
	)
}

func apiTestDatabaseURL() string {
	if value := os.Getenv("TEST_DATABASE_URL"); value != "" {
		return value
	}
	return testutil.DefaultTestDBURL
}

func apiTestRepoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller() failed")

	dir := filepath.Dir(currentFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func hashTestAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
