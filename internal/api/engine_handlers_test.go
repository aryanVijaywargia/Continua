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
	"strings"
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
	"github.com/continua-ai/continua/internal/enginecontrol"
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

	t.Run("mutating routes pass through without preview header", func(t *testing.T) {
		handler := enginePreviewHeaderMiddleware()(next)
		req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs", bytes.NewReader([]byte(`{}`)))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
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

	handler := newAuthenticatedRouter(t, server, platformStore)
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

	handler := newAuthenticatedRouter(t, server, platformStore)
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

	spans, err := platformStore.ListSpansByTrace(ctx, store.BoundScope(projectID), trace.ID)
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

func TestStartEngineRun_RejectsCatalogOnlyDefinitionAfterRegistryTruthSync(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)

	_, err := engineQueries.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    "agent.research",
		DefinitionVersion: "v1",
	})
	require.NoError(t, err)

	catalogOnly := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "agent.research",
		DefinitionVersion: "v1",
		InstanceKey:       "catalog-only-before-sync",
		RequestKey:        "req-catalog-only-before-sync",
	})
	require.Equal(t, http.StatusOK, catalogOnly.Code)
	_ = decodeJSONBody[EngineStartRunResponse](t, catalogOnly)

	beforeSync := countEngineRowsForProject(ctx, t, platformStore, projectID)
	assert.Equal(t, 1, beforeSync.instances)
	assert.Equal(t, 1, beforeSync.runs)

	require.NoError(t, syncDefinitionCatalogToRegistry(ctx, engineQueries, []definitionCatalogTestEntry{
		{name: "checkout", version: "v1"},
	}))

	_, err = engineQueries.GetDefinitionCatalogEntry(ctx, enginedb.GetDefinitionCatalogEntryParams{
		DefinitionName:    "agent.research",
		DefinitionVersion: "v1",
	})
	require.True(t, errors.Is(err, pgx.ErrNoRows))

	rejected := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "agent.research",
		DefinitionVersion: "v1",
		InstanceKey:       "catalog-only-after-sync",
		RequestKey:        "req-catalog-only-after-sync",
	})
	require.Equal(t, http.StatusBadRequest, rejected.Code)
	resp := decodeJSONBody[Error](t, rejected)
	assert.Equal(t, "definition_not_registered", resp.Code)

	afterReject := countEngineRowsForProject(ctx, t, platformStore, projectID)
	assert.Equal(t, beforeSync, afterReject)

	_, err = engineQueries.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectID,
		InstanceKey: "catalog-only-after-sync",
	})
	require.True(t, errors.Is(err, pgx.ErrNoRows))
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

func TestGetEngineInstance_OperatorProjectIDSelectsAmbiguousKey(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectAID := setupEngineHandlerTest(t)
	projectBID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	req := EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "shared-operator-instance",
		RequestKey:        "shared-operator-request",
	}

	_ = decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectAID, req))
	startB := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectBID, req))

	ambiguousRec := invokeGetEngineInstanceAsOperator(t, server, req.InstanceKey, nil)
	require.Equal(t, http.StatusBadRequest, ambiguousRec.Code)
	assert.Equal(t, "ambiguous_instance_key", decodeJSONBody[Error](t, ambiguousRec).Code)

	selectedRec := invokeGetEngineInstanceAsOperator(t, server, req.InstanceKey, &projectBID)
	require.Equal(t, http.StatusOK, selectedRec.Code)
	selectedResp := decodeJSONBody[EngineInstanceResponse](t, selectedRec)
	assert.Equal(t, startB.RunId, selectedResp.CurrentRun.RunId)

	_, err := engineQueries.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectBID,
		InstanceKey: req.InstanceKey,
	})
	require.NoError(t, err)
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

func TestGetEngineRun_OperatorDoesNotRequireProjectID(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-operator-run",
		RequestKey:        "req-operator-run",
	}))

	rec := invokeGetEngineRunAsOperator(t, server, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[EngineRunResponse](t, rec)
	assert.Equal(t, start.RunId, resp.RunId)
	assert.Equal(t, "instance-operator-run", resp.InstanceKey)
}

func TestEngineHandlers_ExposeContinuationChainAcrossReadSurfaces(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-continue-read",
		RequestKey:        "req-continue-read",
	}))

	oldRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        start.RunId,
	})
	require.NoError(t, err)

	now := time.Now().UTC().Round(time.Microsecond)
	nextRun, err := engineQueries.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:          projectID,
		InstanceID:         oldRun.InstanceID,
		RunNumber:          oldRun.RunNumber + 1,
		DefinitionVersion:  oldRun.DefinitionVersion,
		ReadyAt:            now,
		ContinuedFromRunID: pgtype.UUID{Bytes: oldRun.ID, Valid: true},
	})
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'continued_as_new',
		    completed_at = $2,
		    updated_at = $2,
		    continued_to_run_id = $3,
		    waiting_for = NULL,
		    claimed_by = NULL,
		    claimed_at = NULL,
		    lease_expires_at = NULL
		WHERE id = $1
	`, oldRun.ID, now, nextRun.ID)
	require.NoError(t, err)

	oldTrace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET status = 'completed',
		    end_time = $2,
		    output = NULL,
		    engine_run_status = 'continued_as_new',
		    engine_projection_state = 'up_to_date',
		    engine_projection_updated_at = $2,
		    updated_at = $2
		WHERE id = $1
	`, oldTrace.ID, now)
	require.NoError(t, err)

	nextTrace := upsertTraceRecord(ctx, t, platformStore.Queries(), platformdb.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "engine:" + nextRun.ID.String(),
		Name:      testutil.StrPtr("Checkout Workflow"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(now),
	})

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET engine_run_id = $2,
		    engine_instance_key = $3,
		    engine_definition_name = 'checkout',
		    engine_definition_version = 'v1',
		    engine_run_status = 'queued',
		    engine_projection_state = 'up_to_date',
		    engine_projection_updated_at = $4,
		    updated_at = $4
		WHERE id = $1
	`, nextTrace.ID, nextRun.ID, "instance-continue-read", now)
	require.NoError(t, err)

	instanceRec := invokeGetEngineInstance(t, server, projectID, "instance-continue-read")
	require.Equal(t, http.StatusOK, instanceRec.Code)
	instanceResp := decodeJSONBody[EngineInstanceResponse](t, instanceRec)
	assert.Equal(t, nextRun.ID, instanceResp.CurrentRun.RunId)
	assert.Equal(t, EngineRunStatusQUEUED, instanceResp.CurrentRun.Status)
	require.NotNil(t, instanceResp.CurrentRun.ContinuedFromRunId)
	assert.Equal(t, oldRun.ID, *instanceResp.CurrentRun.ContinuedFromRunId)
	require.NotNil(t, instanceResp.CurrentRun.ContinuedFromTraceId)
	assert.Equal(t, "engine:"+oldRun.ID.String(), *instanceResp.CurrentRun.ContinuedFromTraceId)

	runRec := invokeGetEngineRun(t, server, projectID, oldRun.ID)
	require.Equal(t, http.StatusOK, runRec.Code)
	runResp := decodeJSONBody[EngineRunResponse](t, runRec)
	assert.Equal(t, EngineRunStatusCONTINUEDASNEW, runResp.Status)
	require.NotNil(t, runResp.ContinuedToRunId)
	assert.Equal(t, nextRun.ID, *runResp.ContinuedToRunId)
	require.NotNil(t, runResp.ContinuedToTraceId)
	assert.Equal(t, "engine:"+nextRun.ID.String(), *runResp.ContinuedToTraceId)

	resultRec := invokeGetEngineRunResult(t, server, projectID, oldRun.ID)
	require.Equal(t, http.StatusOK, resultRec.Code)
	assertJSONFieldNull(t, resultRec.Body.Bytes(), "result")
	resultResp := decodeJSONBody[EngineRunResultResponse](t, resultRec)
	assert.Equal(t, EngineRunStatusCONTINUEDASNEW, resultResp.Status)
	assert.Nil(t, resultResp.Result)
	require.NotNil(t, resultResp.ContinuedToRunId)
	assert.Equal(t, nextRun.ID, *resultResp.ContinuedToRunId)
	require.NotNil(t, resultResp.ContinuedToTraceId)
	assert.Equal(t, "engine:"+nextRun.ID.String(), *resultResp.ContinuedToTraceId)

	terminateRec := invokeTerminateEngineRun(t, server, projectID, oldRun.ID)
	require.Equal(t, http.StatusOK, terminateRec.Code)
	assertJSONFieldNull(t, terminateRec.Body.Bytes(), "result")
	terminateResp := decodeJSONBody[EngineRunResultResponse](t, terminateRec)
	assert.Equal(t, EngineRunStatusCONTINUEDASNEW, terminateResp.Status)
	assert.Nil(t, terminateResp.Result)
	require.NotNil(t, terminateResp.ContinuedToRunId)
	assert.Equal(t, nextRun.ID, *terminateResp.ContinuedToRunId)
	require.NotNil(t, terminateResp.ContinuedToTraceId)
	assert.Equal(t, "engine:"+nextRun.ID.String(), *terminateResp.ContinuedToTraceId)

	traceRec := invokeGetTrace(t, server, projectID, oldTrace.ID)
	require.Equal(t, http.StatusOK, traceRec.Code)
	traceResp := decodeJSONBody[TraceDetail](t, traceRec)
	require.NotNil(t, traceResp.Engine)
	assert.Equal(t, EngineRunStatusCONTINUEDASNEW, traceResp.Engine.Status)
	require.NotNil(t, traceResp.Engine.ContinuedToRunId)
	assert.Equal(t, nextRun.ID, *traceResp.Engine.ContinuedToRunId)
	require.NotNil(t, traceResp.Engine.ContinuedToTraceId)
	assert.Equal(t, "engine:"+nextRun.ID.String(), *traceResp.Engine.ContinuedToTraceId)
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

func TestGetEngineRun_IncludesDefinitionMismatchFailureContext(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-definition-mismatch",
		RequestKey:        "req-definition-mismatch",
	}))

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'failed',
		    waiting_for = NULL,
		    completed_at = NOW(),
		    last_error_code = 'definition_version_mismatch',
		    last_error_message = 'requested definition version is not registered',
		    updated_at = NOW()
		WHERE id = $1
	`, start.RunId)
	require.NoError(t, err)

	rec := invokeGetEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[EngineRunResponse](t, rec)
	assert.Equal(t, "checkout", resp.DefinitionName)
	assert.Equal(t, "v1", resp.DefinitionVersion)
	require.NotNil(t, resp.Failure)
	assert.Equal(t, "definition_version_mismatch", resp.Failure.ErrorCode)
	assert.Equal(t, "requested definition version is not registered", resp.Failure.ErrorMessage)
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

func TestTerminateEngineRun_RecursivelyTerminatesActiveDescendants(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminate-descendants",
		RequestKey:        "req-terminate-descendants",
	}))

	rootRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        start.RunId,
	})
	require.NoError(t, err)
	rootInstance, err := engineQueries.GetInstance(ctx, rootRun.InstanceID)
	require.NoError(t, err)

	childInstance, childRun := seedEngineChildRun(
		t,
		ctx,
		platformStore,
		engineQueries,
		rootInstance,
		rootRun,
		"charge-card",
		"billing",
		"v1",
		"instance-terminate-descendants-child",
	)
	grandchildInstance, grandchildRun := seedEngineChildRun(
		t,
		ctx,
		platformStore,
		engineQueries,
		childInstance,
		childRun,
		"issue-receipt",
		"receipts",
		"v1",
		"instance-terminate-descendants-grandchild",
	)

	rec := invokeTerminateEngineRun(t, server, projectID, rootRun.ID)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[EngineRunResultResponse](t, rec)
	assert.Equal(t, EngineRunStatusTERMINATED, resp.Status)

	assertTerminatedRunState(t, ctx, engineQueries, projectID, rootRun.ID, publichistory.EventWorkflowTerminated)
	assertTerminatedRunState(t, ctx, engineQueries, projectID, childRun.ID, publichistory.EventWorkflowTerminated)
	assertTerminatedRunState(t, ctx, engineQueries, projectID, grandchildRun.ID, publichistory.EventWorkflowTerminated)

	childRelationship, err := engineQueries.GetChildWorkflowByParentRunAndKey(ctx, enginedb.GetChildWorkflowByParentRunAndKeyParams{
		ProjectID:   projectID,
		ParentRunID: rootRun.ID,
		ChildKey:    "charge-card",
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineChildWorkflowStatusTerminated, childRelationship.Status)
	require.True(t, childRelationship.TerminalChildRunID.Valid)
	assert.Equal(t, childRun.ID, uuid.UUID(childRelationship.TerminalChildRunID.Bytes))

	grandchildRelationship, err := engineQueries.GetChildWorkflowByParentRunAndKey(ctx, enginedb.GetChildWorkflowByParentRunAndKeyParams{
		ProjectID:   projectID,
		ParentRunID: childRun.ID,
		ChildKey:    "issue-receipt",
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineChildWorkflowStatusTerminated, grandchildRelationship.Status)
	require.True(t, grandchildRelationship.TerminalChildRunID.Valid)
	assert.Equal(t, grandchildRun.ID, uuid.UUID(grandchildRelationship.TerminalChildRunID.Bytes))

	childInstanceState, err := engineQueries.GetInstance(ctx, childRun.InstanceID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineInstanceLifecycleStatusTerminated, childInstanceState.Status)
	grandchildInstanceState, err := engineQueries.GetInstance(ctx, grandchildRun.InstanceID)
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineInstanceLifecycleStatusTerminated, grandchildInstanceState.Status)

	assert.Equal(t, childInstance.ID, childInstanceState.ID)
	assert.Equal(t, grandchildInstance.ID, grandchildInstanceState.ID)
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

	engineServe := startExternalEngineServeProcess(t, projectID)
	defer engineServe.stop(t)

	traceStatus, runStatus, waitState := waitForProjectedTraceTerminalSummary(t, platformStore.Pool(), trace.ID)
	assert.Equal(t, "failed", traceStatus)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusTerminated), runStatus)
	assert.Len(t, waitState, 0)
}

func TestTerminateEngineRun_RouterAcceptsHeaderlessAndEnforcesAvailability(t *testing.T) {
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

	router := newAuthenticatedRouter(t, server, platformStore)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+start.RunId.String()+"/terminate", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	server.enginePublicAPIEnabled = false
	router = newAuthenticatedRouter(t, server, platformStore)

	req = httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+start.RunId.String()+"/terminate", nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set(enginePreviewHeader, "1")
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSuspendResumeEngineRun_QueuedLifecycleAndImmediateSummary(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-queued",
		RequestKey:        "req-suspend-queued",
	}))

	runID := start.RunId
	suspendRec := invokeSuspendEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, suspendRec.Code)
	suspended := decodeJSONBody[EngineRunResponse](t, suspendRec)
	assert.Equal(t, EngineRunStatusSUSPENDED, suspended.Status)
	assert.Nil(t, suspended.WaitState)

	var projectedStatus string
	var projectedWaitState []byte
	err := platformStore.Pool().QueryRow(ctx, `
		SELECT engine_run_status, engine_wait_state
		FROM public.traces
		WHERE engine_run_id = $1
	`, runID).Scan(&projectedStatus, &projectedWaitState)
	require.NoError(t, err)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusSuspended), projectedStatus)
	assert.Len(t, projectedWaitState, 0)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.Len(t, historyRows, 2)
	assert.Equal(t, publichistory.EventWorkflowSuspended, historyRows[1].EventType)

	suspendAgainRec := invokeSuspendEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, suspendAgainRec.Code)
	assert.Equal(t, EngineRunStatusSUSPENDED, decodeJSONBody[EngineRunResponse](t, suspendAgainRec).Status)
	historyRows, err = engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.Len(t, historyRows, 2)

	resumeRec := invokeResumeEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, resumeRec.Code)
	resumed := decodeJSONBody[EngineRunResponse](t, resumeRec)
	assert.Equal(t, EngineRunStatusQUEUED, resumed.Status)

	err = platformStore.Pool().QueryRow(ctx, `
		SELECT engine_run_status, engine_wait_state
		FROM public.traces
		WHERE engine_run_id = $1
	`, runID).Scan(&projectedStatus, &projectedWaitState)
	require.NoError(t, err)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusQueued), projectedStatus)
	assert.Len(t, projectedWaitState, 0)

	historyRows, err = engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.Len(t, historyRows, 3)
	assert.Equal(t, publichistory.EventWorkflowResumed, historyRows[2].EventType)

	resumedRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, resumedRun.Status)
	assert.WithinDuration(t, time.Now(), resumedRun.ReadyAt, 5*time.Second)

	resumeAgainRec := invokeResumeEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, resumeAgainRec.Code)
	assert.Equal(t, EngineRunStatusQUEUED, decodeJSONBody[EngineRunResponse](t, resumeAgainRec).Status)
}

func TestSuspendEngineRun_WaitingAndErrorCases(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	waitingStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-waiting",
		RequestKey:        "req-suspend-waiting",
	}))
	setEngineRunWaiting(t, ctx, platformStore, waitingStart.RunId, map[string]any{"phase": "waiting"})

	waitingRec := invokeSuspendEngineRun(t, server, projectID, waitingStart.RunId)
	require.Equal(t, http.StatusOK, waitingRec.Code)
	waitingResp := decodeJSONBody[EngineRunResponse](t, waitingRec)
	assert.Equal(t, EngineRunStatusSUSPENDED, waitingResp.Status)
	require.NotNil(t, waitingResp.WaitState)
	assert.Equal(t, "signal", derefString(waitingResp.WaitState.Kind))

	runningStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-running",
		RequestKey:        "req-suspend-running",
	}))
	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'running',
		    claimed_by = 'worker-a',
		    claimed_at = NOW(),
		    lease_expires_at = NOW() + INTERVAL '1 minute',
		    updated_at = NOW()
		WHERE id = $1
	`, runningStart.RunId)
	require.NoError(t, err)

	runningRec := invokeSuspendEngineRun(t, server, projectID, runningStart.RunId)
	require.Equal(t, http.StatusConflict, runningRec.Code)
	assert.Equal(t, "run_not_suspendable", decodeJSONBody[Error](t, runningRec).Code)

	terminalStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-terminal",
		RequestKey:        "req-suspend-terminal",
	}))
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, terminalStart.RunId)
	require.NoError(t, err)

	terminalRec := invokeSuspendEngineRun(t, server, projectID, terminalStart.RunId)
	require.Equal(t, http.StatusConflict, terminalRec.Code)
	assert.Equal(t, "run_terminal", decodeJSONBody[Error](t, terminalRec).Code)

	missingRec := invokeSuspendEngineRun(t, server, projectID, uuid.New())
	require.Equal(t, http.StatusNotFound, missingRec.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, missingRec).Code)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	crossProjectRec := invokeSuspendEngineRun(t, server, otherProjectID, waitingStart.RunId)
	require.Equal(t, http.StatusNotFound, crossProjectRec.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, crossProjectRec).Code)
}

func TestResumeEngineRun_TerminalAndCrossProjectErrors(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-resume-errors",
		RequestKey:        "req-resume-errors",
	}))
	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)

	resumeRec := invokeResumeEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, resumeRec.Code)
	assert.Equal(t, EngineRunStatusQUEUED, decodeJSONBody[EngineRunResponse](t, resumeRec).Status)

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'failed',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, start.RunId)
	require.NoError(t, err)

	terminalRec := invokeResumeEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusConflict, terminalRec.Code)
	assert.Equal(t, "run_terminal", decodeJSONBody[Error](t, terminalRec).Code)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	crossProjectRec := invokeResumeEngineRun(t, server, otherProjectID, start.RunId)
	require.Equal(t, http.StatusNotFound, crossProjectRec.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, crossProjectRec).Code)
}

func TestTerminateEngineRun_SuspendedRunSucceeds(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-terminate-suspended",
		RequestKey:        "req-terminate-suspended",
	}))

	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)

	rec := invokeTerminateEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[EngineRunResultResponse](t, rec)
	assert.Equal(t, EngineRunStatusTERMINATED, resp.Status)
	require.NotNil(t, resp.Failure)
	assert.Equal(t, "terminated", resp.Failure.ErrorCode)

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        start.RunId,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusTerminated, run.Status)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	require.Len(t, historyRows, 3)
	assert.Equal(t, []string{
		publichistory.EventWorkflowStarted,
		publichistory.EventWorkflowSuspended,
		publichistory.EventWorkflowTerminated,
	}, historyEventTypes(historyRows))
}

func TestEngineRunQuarantineSurfacedInRunDetail(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	runID, reason := seedQuarantinedEngineRun(ctx, t, engineQueries, server, projectID, "detail")

	rec := invokeGetEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[EngineRunResponse](t, rec)
	assert.Equal(t, EngineRunStatusQUARANTINED, resp.Status)
	require.NotNil(t, resp.WaitState)
	assert.Equal(t, publichistory.WaitKindReplayMismatch, derefString(resp.WaitState.Kind))
	assert.Equal(t, reason["detail"], resp.WaitState.AdditionalProperties["detail"])
	require.NotNil(t, resp.Failure)
	assert.Equal(t, "replay_mismatch", resp.Failure.ErrorCode)

	pendingRec := invokeGetEngineRunPendingWork(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, pendingRec.Code)
}

func TestEngineRunResumeFromQuarantineRequeues(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	runID, _ := seedQuarantinedEngineRun(ctx, t, engineQueries, server, projectID, "resume")

	resumeRec := invokeResumeEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, resumeRec.Code)
	resumed := decodeJSONBody[EngineRunResponse](t, resumeRec)
	assert.Equal(t, EngineRunStatusQUEUED, resumed.Status)

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQueued, run.Status)
	assert.Len(t, run.WaitingFor, 0)
	assert.Nil(t, run.LastErrorCode)
	assert.Nil(t, run.LastErrorMessage)
	assert.WithinDuration(t, time.Now(), run.ReadyAt, 5*time.Second)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.Len(t, historyRows, 2)
	assert.Equal(t, publichistory.EventWorkflowResumed, historyRows[1].EventType)
}

func TestEngineRunSuspendQuarantinedConflicts(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	runID, reason := seedQuarantinedEngineRun(ctx, t, engineQueries, server, projectID, "suspend")

	suspendRec := invokeSuspendEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusConflict, suspendRec.Code)
	assert.Equal(t, "run_not_suspendable", decodeJSONBody[Error](t, suspendRec).Code)

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusQuarantined, run.Status)
	var wait map[string]any
	require.NoError(t, json.Unmarshal(run.WaitingFor, &wait))
	assert.Equal(t, reason["kind"], wait["kind"])
	assert.Equal(t, reason["detail"], wait["detail"])
}

func TestEngineRunTerminateQuarantinedRun(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	runID, _ := seedQuarantinedEngineRun(ctx, t, engineQueries, server, projectID, "terminate")

	rec := invokeTerminateEngineRun(t, server, projectID, runID)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[EngineRunResultResponse](t, rec)
	assert.Equal(t, EngineRunStatusTERMINATED, resp.Status)

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusTerminated, run.Status)
	assert.Len(t, run.WaitingFor, 0)
}

func TestSuspendResumeEngineRun_RouterAcceptsHeaderlessAndEnforcesAvailability(t *testing.T) {
	ctx := context.Background()
	platformStore := store.New(testutil.TestDB(t))
	engineQueries := enginedb.New(platformStore.Pool())

	apiKey := "engine-suspend-" + uuid.NewString()
	project, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "suspend-router-" + uuid.NewString()[:8],
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
		InstanceKey:       "suspend-router-instance",
		RequestKey:        "suspend-router-request",
	}))

	router := newAuthenticatedRouter(t, server, platformStore)
	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+start.RunId.String()+"/suspend", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	server.enginePublicAPIEnabled = false
	router = newAuthenticatedRouter(t, server, platformStore)
	req = httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+start.RunId.String()+"/resume", nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set(enginePreviewHeader, "1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSuspendResumeEngineRun_SignalAccumulatedDuringSuspension(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishEngineDefinition(ctx, engineQueries, "darklaunch.demo"))

	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
	)
	defer engineServe.stop(t)

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "darklaunch.demo",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-signal",
		RequestKey:        "req-suspend-signal",
		Input: map[string]any{
			"name":     "Signal",
			"timer_at": "1970-01-01T00:00:00Z",
		},
	}))

	run := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "signal"
	})
	assert.Equal(t, EngineRunStatusWAITING, run.Status)

	suspendRec := invokeSuspendEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, suspendRec.Code)
	assert.Equal(t, EngineRunStatusSUSPENDED, decodeJSONBody[EngineRunResponse](t, suspendRec).Status)

	signalRec := invokeSignalEngineRun(t, server, projectID, start.RunId, EngineSignalRunRequest{
		SignalName: "approval",
		Payload:    map[string]any{"approval": "yes"},
	})
	require.Equal(t, http.StatusOK, signalRec.Code)
	assert.False(t, decodeJSONBody[EngineControlResponse](t, signalRec).WakeApplied)

	resumeRec := invokeResumeEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, resumeRec.Code)

	completed := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusCOMPLETED
	})
	result, ok := completed.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "yes", result["approval"])
}

func TestSuspendResumeEngineRun_TimerFiresDuringSuspensionAndProcessesOnResume(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishEngineDefinition(ctx, engineQueries, "darklaunch.demo"))

	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
	)
	defer engineServe.stop(t)

	timerAt := time.Now().Add(5 * time.Second).UTC()
	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "darklaunch.demo",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-timer",
		RequestKey:        "req-suspend-timer",
		Input: map[string]any{
			"name":     "Timer",
			"timer_at": timerAt.Format(time.RFC3339Nano),
		},
	}))

	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "timer"
	})

	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)
	waitForDueTimerRun(ctx, t, engineQueries, projectID, start.RunId)
	require.Equal(t, http.StatusOK, invokeResumeEngineRun(t, server, projectID, start.RunId).Code)

	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "signal"
	})

	require.Equal(t, http.StatusOK, invokeSignalEngineRun(t, server, projectID, start.RunId, EngineSignalRunRequest{
		SignalName: "approval",
		Payload:    map[string]any{"approval": "timer-fired"},
	}).Code)

	completed := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusCOMPLETED
	})
	result, ok := completed.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "timer-fired", result["approval"])
}

func TestSuspendResumeEngineRun_CancelDuringSuspensionCancelsOnResume(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishEngineDefinition(ctx, engineQueries, "darklaunch.demo"))

	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
	)
	defer engineServe.stop(t)

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "darklaunch.demo",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-cancel",
		RequestKey:        "req-suspend-cancel",
		Input: map[string]any{
			"name":     "Cancel",
			"timer_at": "1970-01-01T00:00:00Z",
		},
	}))

	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "signal"
	})

	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)
	cancelRec := invokeCancelEngineRun(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, cancelRec.Code)
	assert.False(t, decodeJSONBody[EngineControlResponse](t, cancelRec).WakeApplied)
	require.Equal(t, http.StatusOK, invokeResumeEngineRun(t, server, projectID, start.RunId).Code)

	cancelled := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusCANCELLED
	})
	require.NotNil(t, cancelled.Failure)
	assert.Equal(t, "cancelled", cancelled.Failure.ErrorCode)
}

func TestSuspendResumeEngineRun_ActivityCompletesDuringSuspensionAndIsObservedAfterResume(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishEngineDefinition(ctx, engineQueries, "darklaunch.demo"))

	releaseFile := filepath.Join(t.TempDir(), "activity.release")
	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
		"CONTINUA_ENGINE_TEST_ACTIVITY_RELEASE_FILE="+releaseFile,
	)
	defer engineServe.stop(t)

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "darklaunch.demo",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-activity",
		RequestKey:        "req-suspend-activity",
		Input: map[string]any{
			"name":     "Activity",
			"timer_at": "1970-01-01T00:00:00Z",
		},
	}))

	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "activity"
	})

	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)
	require.NoError(t, os.WriteFile(releaseFile, []byte("release"), 0o644))

	waitForActivityTask(ctx, t, engineQueries, start.RunId, "compose-greeting", func(task enginedb.EngineActivityTask) bool {
		return task.Status == enginedb.EngineActivityTaskStatusCompleted
	})

	require.Equal(t, http.StatusOK, invokeResumeEngineRun(t, server, projectID, start.RunId).Code)
	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "signal"
	})

	require.Equal(t, http.StatusOK, invokeSignalEngineRun(t, server, projectID, start.RunId, EngineSignalRunRequest{
		SignalName: "approval",
		Payload:    map[string]any{"approval": "after-activity"},
	}).Code)

	completed := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusCOMPLETED
	})
	result, ok := completed.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "after-activity", result["approval"])
	assert.Equal(t, "hello, Activity", result["greeting"])
}

func TestSuspendResumeEngineRun_RetryExhaustedDuringSuspensionFailsAfterResume(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishRetryDemoDefinition(ctx, engineQueries))

	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
		"CONTINUA_ENGINE_TEST_ACTIVITY_FAIL_COUNT=2",
	)
	defer engineServe.stop(t)

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "darklaunch.retry-demo",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-retry-exhausted",
		RequestKey:        "req-suspend-retry-exhausted",
		Input: map[string]any{
			"name": "RetryFailure",
		},
	}))

	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "activity"
	})

	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)

	failedTask := waitForActivityTask(ctx, t, engineQueries, start.RunId, "compose-greeting", func(task enginedb.EngineActivityTask) bool {
		return task.Status == enginedb.EngineActivityTaskStatusFailed
	})
	assert.Equal(t, int32(2), failedTask.AttemptCount)

	suspendedRun := decodeJSONBody[EngineRunResponse](t, invokeGetEngineRun(t, server, projectID, start.RunId))
	assert.Equal(t, EngineRunStatusSUSPENDED, suspendedRun.Status)

	historyBeforeResume, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	assert.Equal(t, 1, countHistoryEventType(historyBeforeResume, publichistory.EventActivityRetryScheduled))
	assert.NotContains(t, historyEventTypes(historyBeforeResume), publichistory.EventActivityFailed)

	require.Equal(t, http.StatusOK, invokeResumeEngineRun(t, server, projectID, start.RunId).Code)

	failedRun := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusFAILED
	})
	require.NotNil(t, failedRun.Failure)
	assert.Equal(t, "activity_failed", failedRun.Failure.ErrorCode)
	assert.Contains(t, failedRun.Failure.ErrorMessage, "forced test activity failure")

	historyAfterResume, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	retryIndex := historyEventIndex(historyAfterResume, publichistory.EventActivityRetryScheduled)
	activityFailedIndex := historyEventIndex(historyAfterResume, publichistory.EventActivityFailed)
	workflowFailedIndex := historyEventIndex(historyAfterResume, publichistory.EventWorkflowFailed)
	assert.Greater(t, activityFailedIndex, retryIndex)
	assert.Greater(t, workflowFailedIndex, activityFailedIndex)
}

func TestSuspendResumeEngineRun_RetryScheduledDuringSuspensionCompletesAfterResume(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishRetryDemoDefinition(ctx, engineQueries))

	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
		"CONTINUA_ENGINE_TEST_ACTIVITY_FAIL_COUNT=1",
	)
	defer engineServe.stop(t)

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "darklaunch.retry-demo",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-suspend-retry-resume",
		RequestKey:        "req-suspend-retry-resume",
		Input: map[string]any{
			"name": "RetryResume",
		},
	}))

	waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusWAITING && run.WaitState != nil && derefString(run.WaitState.Kind) == "activity"
	})

	require.Equal(t, http.StatusOK, invokeSuspendEngineRun(t, server, projectID, start.RunId).Code)

	retriedTask := waitForActivityTask(ctx, t, engineQueries, start.RunId, "compose-greeting", func(task enginedb.EngineActivityTask) bool {
		return task.Status == enginedb.EngineActivityTaskStatusQueued && task.AttemptCount == 1
	})
	assert.Equal(t, int32(1), retriedTask.AttemptCount)

	suspendedRun := decodeJSONBody[EngineRunResponse](t, invokeGetEngineRun(t, server, projectID, start.RunId))
	assert.Equal(t, EngineRunStatusSUSPENDED, suspendedRun.Status)

	historyBeforeResume, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	assert.Equal(t, 1, countHistoryEventType(historyBeforeResume, publichistory.EventActivityRetryScheduled))
	eventTypesBeforeResume := historyEventTypes(historyBeforeResume)
	assert.NotContains(t, eventTypesBeforeResume, publichistory.EventActivityCompleted)
	assert.NotContains(t, eventTypesBeforeResume, publichistory.EventActivityFailed)

	require.Equal(t, http.StatusOK, invokeResumeEngineRun(t, server, projectID, start.RunId).Code)

	completed := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusCOMPLETED
	})
	result, ok := completed.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello, RetryResume", result["greeting"])

	historyAfterResume, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	retryIndex := historyEventIndex(historyAfterResume, publichistory.EventActivityRetryScheduled)
	activityCompletedIndex := historyEventIndex(historyAfterResume, publichistory.EventActivityCompleted)
	workflowCompletedIndex := historyEventIndex(historyAfterResume, publichistory.EventWorkflowCompleted)
	assert.Greater(t, activityCompletedIndex, retryIndex)
	assert.Greater(t, workflowCompletedIndex, activityCompletedIndex)
}

func TestPurgeEngineRun_ProjectionOnlyThenFullPreservesShell(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-purge",
		RequestKey:        "req-purge",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)
	markEngineRunCompleted(t, ctx, platformStore, start.RunId, trace.ID)

	now := time.Now().UTC()
	_, err = platformStore.Queries().CreateSpan(ctx, platformdb.CreateSpanParams{
		ProjectID:    projectID,
		TraceID:      trace.ID,
		SpanID:       "engine:activity:" + start.RunId.String() + ":ship-order",
		ParentSpanID: testutil.StrPtr("engine:root:" + start.RunId.String()),
		Name:         "ship-order",
		Type:         "tool",
		Status:       "completed",
		Level:        "default",
		StartTime:    now.Add(-time.Minute),
		EndTime:      testutil.PgtypeTimestamptz(now),
		Depth:        testutil.Int32Ptr(1),
	})
	require.NoError(t, err)
	_, err = platformStore.Queries().InsertSpanEvent(ctx, platformdb.InsertSpanEventParams{
		ProjectID: projectID,
		TraceID:   trace.ID,
		SpanID:    "engine:root:" + start.RunId.String(),
		EventType: "custom",
		Level:     "info",
		EventTs:   testutil.PgtypeTimestamptz(now),
		Message:   testutil.StrPtr("projected detail"),
		Payload:   []byte(`{"detail":true}`),
	})
	require.NoError(t, err)

	projectionOnly := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(
		t,
		server,
		projectID,
		start.RunId,
		ProjectionOnly,
	))
	assert.True(t, projectionOnly.Deleted)
	assert.Equal(t, SummaryOnly, projectionOnly.ProjectionState)

	idempotentProjectionOnly := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(
		t,
		server,
		projectID,
		start.RunId,
		ProjectionOnly,
	))
	assert.False(t, idempotentProjectionOnly.Deleted)
	assert.Equal(t, SummaryOnly, idempotentProjectionOnly.ProjectionState)

	remainingSpans, err := platformStore.ListSpansByTrace(ctx, store.BoundScope(projectID), trace.ID)
	require.NoError(t, err)
	require.Len(t, remainingSpans, 1)
	assert.Equal(t, "engine:root:"+start.RunId.String(), remainingSpans[0].SpanID)

	events, err := platformStore.ListSpanEventsByTrace(ctx, store.BoundScope(projectID), trace.ID)
	require.NoError(t, err)
	assert.Empty(t, events)

	resultRec := invokeGetEngineRunResult(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, resultRec.Code)
	resultResp := decodeJSONBody[EngineRunResultResponse](t, resultRec)
	assert.Equal(t, EngineRunStatusCOMPLETED, resultResp.Status)
	require.NotNil(t, resultResp.Result)

	full := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(
		t,
		server,
		projectID,
		start.RunId,
		Full,
	))
	assert.True(t, full.Deleted)
	assert.Equal(t, JournalExpired, full.ProjectionState)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	assert.Empty(t, historyRows)

	historyRec := invokeGetEngineRunHistory(t, server, projectID, start.RunId, GetEngineRunHistoryParams{})
	require.Equal(t, http.StatusOK, historyRec.Code)
	historyResp := decodeJSONBody[EngineRunHistoryResponse](t, historyRec)
	require.NotNil(t, historyResp.Expired)
	assert.True(t, *historyResp.Expired)
	assert.Empty(t, historyResp.Events)

	journalExpiredResultRec := invokeGetEngineRunResult(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, journalExpiredResultRec.Code)
	journalExpiredResult := decodeJSONBody[EngineRunResultResponse](t, journalExpiredResultRec)
	assert.Equal(t, EngineRunStatusCOMPLETED, journalExpiredResult.Status)
	require.NotNil(t, journalExpiredResult.Result)

	idempotentFull := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(
		t,
		server,
		projectID,
		start.RunId,
		Full,
	))
	assert.False(t, idempotentFull.Deleted)
	assert.Equal(t, JournalExpired, idempotentFull.ProjectionState)
}

func TestPurgeEngineRun_FullClearsHistoryLinksBeforeDeletingHistory(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-purge-history-links",
		RequestKey:        "req-purge-history-links",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)
	markEngineRunCompleted(t, ctx, platformStore, start.RunId, trace.ID)
	createPendingWorkForRun(t, ctx, engineQueries, projectID, start.RunId)

	activityTasks, err := engineQueries.ListActivityTasksByRun(ctx, start.RunId)
	require.NoError(t, err)
	require.Len(t, activityTasks, 1)
	require.NotNil(t, activityTasks[0].HistoryID)

	inboxItems, err := engineQueries.ListPendingInboxByRun(ctx, pgtype.UUID{Bytes: start.RunId, Valid: true})
	require.NoError(t, err)
	require.Len(t, inboxItems, 1)
	require.NotNil(t, inboxItems[0].HistoryID)

	full := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(
		t,
		server,
		projectID,
		start.RunId,
		Full,
	))
	assert.True(t, full.Deleted)
	assert.Equal(t, JournalExpired, full.ProjectionState)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	assert.Empty(t, historyRows)

	activityTasks, err = engineQueries.ListActivityTasksByRun(ctx, start.RunId)
	require.NoError(t, err)
	require.Len(t, activityTasks, 1)
	assert.Nil(t, activityTasks[0].HistoryID)

	inboxItems, err = engineQueries.ListPendingInboxByRun(ctx, pgtype.UUID{Bytes: start.RunId, Valid: true})
	require.NoError(t, err)
	require.Len(t, inboxItems, 1)
	assert.Nil(t, inboxItems[0].HistoryID)
}

func TestPurgeEngineRun_SynthesizesTerminalShellBeforeBarrier(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-purge-terminal-shell-race",
		RequestKey:        "req-purge-terminal-shell-race",
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

	startHistory, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	require.Len(t, startHistory, 1)

	terminalHistory := appendEngineHistoryEvent(
		t,
		ctx,
		engineQueries,
		projectID,
		run.InstanceID,
		start.RunId,
		2,
		publichistory.EventWorkflowCompleted,
		publichistory.WorkflowCompletedPayload{Result: []byte(`{"ok":true}`)},
	)
	markEngineRunCompletedEngineOnly(t, ctx, platformStore, start.RunId)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "catching_up", terminalHistory.ID, startHistory[0].ID)

	purge := decodeJSONBody[EnginePurgeResponse](t, invokePurgeEngineRun(
		t,
		server,
		projectID,
		start.RunId,
		ProjectionOnly,
	))
	assert.True(t, purge.Deleted)
	assert.Equal(t, SummaryOnly, purge.ProjectionState)

	var traceStatus string
	var traceOutput []byte
	var runStatus string
	if err := platformStore.Pool().QueryRow(ctx, `
		SELECT status, output, engine_run_status
		FROM traces
		WHERE id = $1
	`, trace.ID).Scan(&traceStatus, &traceOutput, &runStatus); err != nil {
		t.Fatalf("query synthesized terminal trace shell: %v", err)
	}
	assert.Equal(t, "completed", traceStatus)
	assert.Equal(t, string(enginedb.EngineRunLifecycleStatusCompleted), runStatus)
	assert.JSONEq(t, `{"ok":true}`, string(traceOutput))

	var rootSpanStatus string
	var rootSpanOutput []byte
	if err := platformStore.Pool().QueryRow(ctx, `
		SELECT status, output
		FROM spans
		WHERE trace_id = $1
		  AND span_id = $2
	`, trace.ID, "engine:root:"+start.RunId.String()).Scan(&rootSpanStatus, &rootSpanOutput); err != nil {
		t.Fatalf("query synthesized terminal root span shell: %v", err)
	}
	assert.Equal(t, "completed", rootSpanStatus)
	assert.JSONEq(t, `{"ok":true}`, string(rootSpanOutput))
}

func TestPurgeEngineRun_RejectsNonTerminalRun(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-purge-nonterminal",
		RequestKey:        "req-purge-nonterminal",
	}))

	rec := invokePurgeEngineRun(t, server, projectID, start.RunId, ProjectionOnly)
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "run_not_terminal", decodeJSONBody[Error](t, rec).Code)
}

func TestPurgeEngineRun_AcceptsContinuedAsNewRun(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-purge-continued",
		RequestKey:        "req-purge-continued",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	completedAt := time.Now().UTC().Round(time.Microsecond)
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'continued_as_new',
		    waiting_for = NULL,
		    result = NULL,
		    completed_at = $2,
		    updated_at = $2
		WHERE id = $1
	`, start.RunId, completedAt)
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET status = 'completed',
		    end_time = $2,
		    output = NULL,
		    engine_run_status = 'continued_as_new',
		    engine_projection_state = 'up_to_date',
		    engine_projection_updated_at = $2,
		    updated_at = $2
		WHERE id = $1
	`, trace.ID, completedAt)
	require.NoError(t, err)

	rec := invokePurgeEngineRun(t, server, projectID, start.RunId, ProjectionOnly)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[EnginePurgeResponse](t, rec)
	assert.True(t, resp.Deleted)
	assert.Equal(t, SummaryOnly, resp.ProjectionState)
}

func TestPurgeEngineRun_ReturnsNotFoundForMissingRunAndCrossProject(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-purge-scope",
		RequestKey:        "req-purge-scope",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)
	markEngineRunCompleted(t, ctx, platformStore, start.RunId, trace.ID)

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	crossProject := invokePurgeEngineRun(t, server, otherProjectID, start.RunId, ProjectionOnly)
	require.Equal(t, http.StatusNotFound, crossProject.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, crossProject).Code)

	missing := invokePurgeEngineRun(t, server, projectID, uuid.New(), ProjectionOnly)
	require.Equal(t, http.StatusNotFound, missing.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, missing).Code)
}

func TestRepairEngineRun_ReturnsExpectedReasons(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-repair",
		RequestKey:        "req-repair",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	t.Run("up_to_date", func(t *testing.T) {
		setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "up_to_date", 5, 5)
		resp := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
		assert.False(t, resp.Accepted)
		assert.Equal(t, EngineRepairReasonAlreadyUpToDate, resp.Reason)
		assert.Equal(t, UpToDate, resp.ProjectionState)
	})

	t.Run("summary_only with retained history", func(t *testing.T) {
		latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
		require.NoError(t, err)
		require.Greater(t, latestHistoryID, int64(0))
		setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID-1)
		resp := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
		assert.True(t, resp.Accepted)
		assert.Equal(t, EngineRepairReasonRepairRequested, resp.Reason)
		assert.Equal(t, CatchingUp, resp.ProjectionState)
	})

	t.Run("summary_only no events", func(t *testing.T) {
		latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
		require.NoError(t, err)
		setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID)
		resp := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
		assert.False(t, resp.Accepted)
		assert.Equal(t, EngineRepairReasonNoEventsToProject, resp.Reason)
		assert.Equal(t, SummaryOnly, resp.ProjectionState)
	})

	t.Run("summary_only uses retained history instead of stale trace latest id", func(t *testing.T) {
		run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
			ProjectID: projectID,
			ID:        start.RunId,
		})
		require.NoError(t, err)
		retainedHistory := appendEngineHistoryEvent(
			t,
			ctx,
			engineQueries,
			projectID,
			run.InstanceID,
			start.RunId,
			2,
			publichistory.EventActivityScheduled,
			publichistory.ActivityScheduledPayload{
				ActivityKey:  "ship-order",
				ActivityType: "demo.ship",
				Input:        []byte(`{"order_id":"ord-123"}`),
			},
		)
		setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", 1, 1)

		resp := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
		assert.True(t, resp.Accepted)
		assert.Equal(t, EngineRepairReasonRepairRequested, resp.Reason)
		assert.Equal(t, CatchingUp, resp.ProjectionState)

		var latestHistoryID int64
		if err := platformStore.Pool().QueryRow(ctx, `
			SELECT engine_latest_history_id
			FROM traces
			WHERE id = $1
		`, trace.ID).Scan(&latestHistoryID); err != nil {
			t.Fatalf("query trace latest history id: %v", err)
		}
		assert.NotEqual(t, retainedHistory.ID, latestHistoryID)
	})

	t.Run("journal_expired", func(t *testing.T) {
		setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "journal_expired", 9, 3)
		resp := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
		assert.False(t, resp.Accepted)
		assert.Equal(t, EngineRepairReasonHistoryExpired, resp.Reason)
		assert.Equal(t, JournalExpired, resp.ProjectionState)
	})

	t.Run("catching_up", func(t *testing.T) {
		setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "catching_up", 9, 3)
		resp := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
		assert.True(t, resp.Accepted)
		assert.Equal(t, EngineRepairReasonAlreadyCatchingUp, resp.Reason)
		assert.Equal(t, CatchingUp, resp.ProjectionState)
	})
}

func TestRepairEngineRun_ReturnsNotFoundForMissingRunAndCrossProject(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-repair-scope",
		RequestKey:        "req-repair-scope",
	}))

	otherProjectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	crossProject := invokeRepairEngineRun(t, server, otherProjectID, start.RunId)
	require.Equal(t, http.StatusNotFound, crossProject.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, crossProject).Code)

	missing := invokeRepairEngineRun(t, server, projectID, uuid.New())
	require.Equal(t, http.StatusNotFound, missing.Code)
	assert.Equal(t, "not_found", decodeJSONBody[Error](t, missing).Code)
}

func TestBackfillEngineProjections_DryRunReturnsWouldRepairWithoutMutation(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-dry-run",
		RequestKey:        "req-backfill-dry-run",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
	require.NoError(t, err)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID-1)

	dryRun := true
	resp := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjections(
		t,
		server,
		projectID,
		&EngineProjectionBackfillRequest{DryRun: &dryRun},
	))

	require.True(t, resp.DryRun)
	assert.Equal(t, defaultEngineProjectionBackfillLimit, resp.Limit)
	assert.Equal(t, 1, resp.EligibleCount)
	assert.Equal(t, 0, resp.RepairRequestedCount)
	assert.Equal(t, 0, resp.SkippedCount)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, start.RunId, resp.Results[0].RunId)
	assert.Equal(t, start.TraceId, resp.Results[0].TraceId)
	assert.Equal(t, SummaryOnly, resp.Results[0].ProjectionState)
	assert.Equal(t, EngineProjectionBackfillActionWouldRepair, resp.Results[0].Action)
	assert.Nil(t, resp.Results[0].Reason)

	var projectionState string
	err = platformStore.Pool().QueryRow(ctx, `
		SELECT engine_projection_state
		FROM traces
		WHERE id = $1
	`, trace.ID).Scan(&projectionState)
	require.NoError(t, err)
	assert.Equal(t, "summary_only", projectionState)
}

func TestBackfillEngineProjections_PublicDemoAllowsOnlyDryRun(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-public-demo",
		RequestKey:        "req-backfill-public-demo",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
	require.NoError(t, err)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID-1)

	dryRun := true
	dryRunResp := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjectionsWithAuthMode(
		t,
		server,
		projectID,
		middleware.AuthModePublicDemo,
		&EngineProjectionBackfillRequest{DryRun: &dryRun},
	))
	assert.True(t, dryRunResp.DryRun)
	assert.Equal(t, 1, dryRunResp.EligibleCount)
	require.Len(t, dryRunResp.Results, 1)
	assert.Equal(t, EngineProjectionBackfillActionWouldRepair, dryRunResp.Results[0].Action)

	apply := false
	applyRec := invokeBackfillEngineProjectionsWithAuthMode(
		t,
		server,
		projectID,
		middleware.AuthModePublicDemo,
		&EngineProjectionBackfillRequest{DryRun: &apply},
	)
	require.Equal(t, http.StatusForbidden, applyRec.Code)
	applyResp := decodeJSONBody[Error](t, applyRec)
	assert.Equal(t, "public_demo_read_only", applyResp.Code)

	var projectionState string
	err = platformStore.Pool().QueryRow(ctx, `
		SELECT engine_projection_state
		FROM traces
		WHERE id = $1
	`, trace.ID).Scan(&projectionState)
	require.NoError(t, err)
	assert.Equal(t, "summary_only", projectionState)
}

func TestBackfillEngineProjections_ApplyRequestsRepairAndFlipsState(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-apply",
		RequestKey:        "req-backfill-apply",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
	require.NoError(t, err)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID-1)

	limit := 10
	resp := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjections(
		t,
		server,
		projectID,
		&EngineProjectionBackfillRequest{Limit: &limit},
	))

	require.False(t, resp.DryRun)
	assert.Equal(t, limit, resp.Limit)
	assert.Equal(t, 1, resp.EligibleCount)
	assert.Equal(t, 1, resp.RepairRequestedCount)
	assert.Equal(t, 0, resp.SkippedCount)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, EngineProjectionBackfillActionRepairRequested, resp.Results[0].Action)
	require.NotNil(t, resp.Results[0].Reason)
	assert.Equal(t, EngineRepairReasonRepairRequested, *resp.Results[0].Reason)
	assert.Equal(t, CatchingUp, resp.Results[0].ProjectionState)

	var projectionState string
	err = platformStore.Pool().QueryRow(ctx, `
		SELECT engine_projection_state
		FROM traces
		WHERE id = $1
	`, trace.ID).Scan(&projectionState)
	require.NoError(t, err)
	assert.Equal(t, "catching_up", projectionState)
}

func TestBackfillEngineProjections_LimitAboveMaxReturnsBadRequest(t *testing.T) {
	_, _, _, server, projectID := setupEngineHandlerTest(t)

	limit := 101
	rec := invokeBackfillEngineProjections(
		t,
		server,
		projectID,
		&EngineProjectionBackfillRequest{Limit: &limit},
	)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "invalid_request", resp.Code)
	assert.Equal(t, "limit must be 100 or less", resp.Message)
}

func TestBackfillEngineProjections_OlderThanFiltersByProjectionUpdatedAt(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	oldStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-old",
		RequestKey:        "req-backfill-old",
	}))
	newStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-new",
		RequestKey:        "req-backfill-new",
	}))

	oldTrace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   oldStart.TraceId,
	})
	require.NoError(t, err)
	newTrace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   newStart.TraceId,
	})
	require.NoError(t, err)

	oldLatestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, oldStart.RunId)
	require.NoError(t, err)
	newLatestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, newStart.RunId)
	require.NoError(t, err)

	oldUpdatedAt := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	newUpdatedAt := time.Date(2026, time.January, 3, 10, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, time.January, 2, 10, 0, 0, 0, time.UTC)

	setEngineProjectionCheckpointAt(
		t,
		ctx,
		platformStore,
		oldTrace.ID,
		"summary_only",
		oldLatestHistoryID,
		oldLatestHistoryID-1,
		oldUpdatedAt,
	)
	setEngineProjectionCheckpointAt(
		t,
		ctx,
		platformStore,
		newTrace.ID,
		"summary_only",
		newLatestHistoryID,
		newLatestHistoryID-1,
		newUpdatedAt,
	)

	dryRun := true
	resp := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjections(
		t,
		server,
		projectID,
		&EngineProjectionBackfillRequest{
			DryRun:    &dryRun,
			OlderThan: &cutoff,
		},
	))

	assert.Equal(t, 1, resp.EligibleCount)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, oldStart.RunId, resp.Results[0].RunId)
	assert.Equal(t, oldStart.TraceId, resp.Results[0].TraceId)
}

func TestBackfillEngineProjections_RepeatedCallsConvergeToZeroEligible(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-converge",
		RequestKey:        "req-backfill-converge",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
	require.NoError(t, err)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID-1)

	first := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjections(t, server, projectID, nil))
	assert.Equal(t, 1, first.EligibleCount)
	require.Len(t, first.Results, 1)
	assert.Equal(t, EngineProjectionBackfillActionRepairRequested, first.Results[0].Action)

	second := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjections(t, server, projectID, nil))
	assert.Equal(t, 0, second.EligibleCount)
	assert.Empty(t, second.Results)
}

func TestBackfillEngineProjections_IncompatibleProjectionStatesReturnEmptyResults(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-backfill-incompatible",
		RequestKey:        "req-backfill-incompatible",
	}))

	trace, err := platformStore.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   start.TraceId,
	})
	require.NoError(t, err)

	latestHistoryID, err := engineQueries.GetLatestHistoryIDByRun(ctx, start.RunId)
	require.NoError(t, err)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", latestHistoryID, latestHistoryID-1)

	for _, projectionState := range []EngineProjectionState{UpToDate, CatchingUp, JournalExpired} {
		t.Run(string(projectionState), func(t *testing.T) {
			dryRun := true
			resp := decodeJSONBody[EngineProjectionBackfillResponse](t, invokeBackfillEngineProjections(
				t,
				server,
				projectID,
				&EngineProjectionBackfillRequest{
					DryRun:                &dryRun,
					EngineProjectionState: &projectionState,
				},
			))

			assert.True(t, resp.DryRun)
			assert.Equal(t, 0, resp.EligibleCount)
			assert.Equal(t, 0, resp.RepairRequestedCount)
			assert.Equal(t, 0, resp.SkippedCount)
			assert.Empty(t, resp.Results)
		})
	}
}

func TestRepairEngineRun_ProjectorEventuallyRebuildsPurgedDetail(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-repair-projector",
		RequestKey:        "req-repair-projector",
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

	historyBefore, err := engineQueries.GetHistoryByRun(ctx, start.RunId)
	require.NoError(t, err)
	require.Len(t, historyBefore, 1)

	activityHistory := appendEngineHistoryEvent(
		t,
		ctx,
		engineQueries,
		projectID,
		run.InstanceID,
		start.RunId,
		2,
		publichistory.EventActivityScheduled,
		publichistory.ActivityScheduledPayload{
			ActivityKey:  "ship-order",
			ActivityType: "demo.ship",
			Input:        []byte(`{"order_id":"ord-123"}`),
		},
	)

	markEngineRunCompleted(t, ctx, platformStore, start.RunId, trace.ID)
	setEngineProjectionCheckpoint(t, ctx, platformStore, trace.ID, "summary_only", activityHistory.ID, historyBefore[0].ID)

	spansBefore, err := platformStore.ListSpansByTrace(ctx, store.BoundScope(projectID), trace.ID)
	require.NoError(t, err)
	require.Len(t, spansBefore, 1)

	eventsBefore, err := platformStore.ListSpanEventsByTrace(ctx, store.BoundScope(projectID), trace.ID)
	require.NoError(t, err)
	assert.Empty(t, eventsBefore)

	repair := decodeJSONBody[EngineRepairResponse](t, invokeRepairEngineRun(t, server, projectID, start.RunId))
	assert.True(t, repair.Accepted)
	assert.Equal(t, EngineRepairReasonRepairRequested, repair.Reason)
	assert.Equal(t, CatchingUp, repair.ProjectionState)

	engineServe := startExternalEngineServeProcess(t, projectID)
	defer engineServe.stop(t)

	waitForProjectedTraceCaughtUp(t, ctx, platformStore.Pool(), trace.ID, activityHistory.ID)

	spansAfter, err := platformStore.ListSpansByTrace(ctx, store.BoundScope(projectID), trace.ID)
	require.NoError(t, err)
	require.Len(t, spansAfter, 2)

	var activitySpanCount int
	for _, span := range spansAfter {
		if span.SpanID == "engine:activity:"+start.RunId.String()+":ship-order" {
			activitySpanCount++
		}
	}
	assert.Equal(t, 1, activitySpanCount)

	eventsAfter, err := platformStore.ListSpanEventsByTrace(ctx, store.BoundScope(projectID), trace.ID)
	require.NoError(t, err)
	require.Len(t, eventsAfter, 2)
	assert.Equal(t, "effect", eventsAfter[0].EventType)
	assert.Equal(t, "wait", eventsAfter[1].EventType)
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
		ProjectID:       projectID,
		InstanceID:      run.InstanceID,
		RunID:           runID,
		HistoryID:       &activityHistoryB.ID,
		ActivityKey:     "approval-b",
		ActivityType:    "demo.activity",
		Input:           []byte(`{"step":"b"}`),
		AvailableAt:     baseTime.Add(2 * time.Minute),
		ExecutionTarget: "local",
		MaxAttempts:     1,
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      run.InstanceID,
		RunID:           runID,
		HistoryID:       &activityHistoryA.ID,
		ActivityKey:     "approval-a",
		ActivityType:    "demo.activity",
		Input:           []byte(`{"step":"a"}`),
		AvailableAt:     baseTime.Add(1 * time.Minute),
		ExecutionTarget: "local",
		MaxAttempts:     1,
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

	router := newAuthenticatedRouter(t, routerServer, platformStore)
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

	engineServe := startExternalEngineServeProcess(t, projectID)
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
			setTraceProjectionState(ctx, t, platformStore, trace.ID, tc.projectionState)
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

	setTraceProjectionState(ctx, t, platformStore, trace.ID, publicprojection.StateCatchingUp.String())
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
	server.engineSharedControl = enginecontrol.NewService(platformStore)
	server.enginePublicAPIEnabled = true

	projectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	return ctx, platformStore, enginedb.New(pool), server, projectID
}

func publishCheckoutDefinition(ctx context.Context, queries *enginedb.Queries) error {
	return publishEngineDefinition(ctx, queries, "checkout")
}

func publishRetryDemoDefinition(ctx context.Context, queries *enginedb.Queries) error {
	return publishEngineDefinition(ctx, queries, "darklaunch.retry-demo")
}

func publishEngineDefinition(ctx context.Context, queries *enginedb.Queries, name string) error {
	_, err := queries.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
		DefinitionName:    name,
		DefinitionVersion: "v1",
	})
	return err
}

type definitionCatalogTestEntry struct {
	name    string
	version string
}

func syncDefinitionCatalogToRegistry(ctx context.Context, queries *enginedb.Queries, entries []definitionCatalogTestEntry) error {
	// Mirrors engine/internal/catalog/publisher.go without importing across the engine module's internal boundary.
	liveDefinitions := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		liveDefinitions[entry.name+"@"+entry.version] = struct{}{}
		if _, err := queries.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
			DefinitionName:    entry.name,
			DefinitionVersion: entry.version,
		}); err != nil {
			return err
		}
	}

	rows, err := queries.ListDefinitionCatalog(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, ok := liveDefinitions[row.DefinitionName+"@"+row.DefinitionVersion]; ok {
			continue
		}
		if _, err := queries.DeleteDefinitionCatalogEntry(ctx, enginedb.DeleteDefinitionCatalogEntryParams{
			DefinitionName:    row.DefinitionName,
			DefinitionVersion: row.DefinitionVersion,
		}); err != nil {
			return err
		}
	}

	return nil
}

func seedQuarantinedEngineRun(
	ctx context.Context,
	t *testing.T,
	engineQueries *enginedb.Queries,
	server *Server,
	projectID uuid.UUID,
	suffix string,
) (uuid.UUID, map[string]any) {
	t.Helper()

	start := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "instance-quarantine-" + suffix,
		RequestKey:        "req-quarantine-" + suffix,
	}))
	worker := "quarantine-seed-" + suffix
	claimed, err := engineQueries.ClaimNextRunByProject(ctx, enginedb.ClaimNextRunByProjectParams{
		ClaimedBy:           &worker,
		LeaseDurationMicros: int64(time.Minute / time.Microsecond),
		ProjectFilterID:     projectID,
	})
	require.NoError(t, err)
	require.Equal(t, start.RunId, claimed.ID)

	reason := map[string]any{
		"kind":          publichistory.WaitKindReplayMismatch,
		"expected_type": publichistory.EventActivityScheduled,
		"expected_key":  "step",
		"actual_type":   publichistory.EventActivityScheduled,
		"actual_key":    "step-renamed",
		"detail":        "activity scheduling did not match recorded history",
	}
	reasonPayload, err := json.Marshal(reason)
	require.NoError(t, err)
	errorCode := "replay_mismatch"
	errorMessage := "replay mismatch quarantined"

	_, err = engineQueries.TransitionRunToQuarantined(ctx, enginedb.TransitionRunToQuarantinedParams{
		ID:               start.RunId,
		ClaimedBy:        &worker,
		WaitingFor:       reasonPayload,
		LastErrorCode:    &errorCode,
		LastErrorMessage: &errorMessage,
	})
	require.NoError(t, err)
	return start.RunId, reason
}

type engineProjectRowCounts struct {
	instances int
	runs      int
}

func countEngineRowsForProject(ctx context.Context, t *testing.T, platformStore *store.Store, projectID uuid.UUID) engineProjectRowCounts {
	t.Helper()

	var counts engineProjectRowCounts
	err := platformStore.Pool().QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM engine.instances WHERE project_id = $1),
			(SELECT COUNT(*) FROM engine.runs WHERE project_id = $1)
	`, projectID).Scan(&counts.instances, &counts.runs)
	require.NoError(t, err)
	return counts
}

func invokeStartEngineRun(t *testing.T, server *Server, projectID uuid.UUID, req EngineStartRunRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/engine/runs", bytes.NewReader(body))
	httpReq.Header.Set(enginePreviewHeader, "1")
	httpReq = httpReq.WithContext(context.WithValue(httpReq.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.StartEngineRun(rec, httpReq, StartEngineRunParams{})
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

func invokeGetEngineInstanceAsOperator(
	t *testing.T,
	server *Server,
	instanceKey string,
	projectID *uuid.UUID,
) *httptest.ResponseRecorder {
	t.Helper()

	target := "/v1/engine/instances/" + instanceKey
	if projectID != nil {
		target += "?project_id=" + projectID.String()
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	reqCtx = context.WithValue(reqCtx, middleware.OperatorEmailKey, "operator@example.com")
	reqCtx = context.WithValue(reqCtx, middleware.OperatorSubjectKey, "google-oauth2|operator")
	rec := httptest.NewRecorder()

	server.GetEngineInstance(rec, req.WithContext(reqCtx), instanceKey)
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

func invokeGetEngineRunAsOperator(t *testing.T, server *Server, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+runID.String(), nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	reqCtx = context.WithValue(reqCtx, middleware.OperatorEmailKey, "operator@example.com")
	reqCtx = context.WithValue(reqCtx, middleware.OperatorSubjectKey, "google-oauth2|operator")
	rec := httptest.NewRecorder()

	server.GetEngineRun(rec, req.WithContext(reqCtx), runID)
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

func invokePurgeEngineRun(
	t *testing.T,
	server *Server,
	projectID, runID uuid.UUID,
	mode EnginePurgeMode,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(EnginePurgeRequest{Mode: mode})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/purge", bytes.NewReader(body))
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.PurgeEngineRun(rec, req, runID, PurgeEngineRunParams{})
	return rec
}

func invokeRepairEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/repair", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.RepairEngineRun(rec, req, runID, RepairEngineRunParams{})
	return rec
}

func invokeBackfillEngineProjections(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	reqBody *EngineProjectionBackfillRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	return invokeBackfillEngineProjectionsWithAuthMode(t, server, projectID, "", reqBody)
}

func invokeBackfillEngineProjectionsWithAuthMode(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	authMode middleware.AuthMode,
	reqBody *EngineProjectionBackfillRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	var body *bytes.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		require.NoError(t, err)
		body = bytes.NewReader(payload)
	} else {
		body = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/projections/backfill", body)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	if authMode != "" {
		req = req.WithContext(context.WithValue(req.Context(), middleware.AuthModeKey, authMode))
	}
	rec := httptest.NewRecorder()

	server.BackfillEngineProjections(rec, req, BackfillEngineProjectionsParams{})
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

	server.SignalEngineRun(rec, req, runID, SignalEngineRunParams{})
	return rec
}

func invokeCancelEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/cancel", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.CancelEngineRun(rec, req, runID, CancelEngineRunParams{})
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

	server.TerminateEngineRun(rec, req, runID, TerminateEngineRunParams{})
	return rec
}

func invokeSuspendEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/suspend", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.SuspendEngineRun(rec, req, runID, SuspendEngineRunParams{})
	return rec
}

func invokeResumeEngineRun(t *testing.T, server *Server, projectID, runID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs/"+runID.String()+"/resume", nil)
	req.Header.Set(enginePreviewHeader, "1")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	server.ResumeEngineRun(rec, req, runID, ResumeEngineRunParams{})
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
func seedEngineChildRun(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	engineQueries *enginedb.Queries,
	parentInstance enginedb.EngineInstance,
	parentRun enginedb.EngineRun,
	childKey string,
	definitionName string,
	definitionVersion string,
	instanceKey string,
) (enginedb.EngineInstance, enginedb.EngineRun) {
	t.Helper()

	childInstance, err := engineQueries.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      parentRun.ProjectID,
		InstanceKey:    instanceKey,
		DefinitionName: definitionName,
	})
	require.NoError(t, err)

	rootRunID := parentRun.RootRunID
	if rootRunID == uuid.Nil {
		rootRunID = parentRun.ID
	}
	childRun, err := engineQueries.CreateChildRun(ctx, enginedb.CreateChildRunParams{
		ProjectID:          parentRun.ProjectID,
		InstanceID:         childInstance.ID,
		RunNumber:          1,
		DefinitionVersion:  definitionVersion,
		ReadyAt:            time.Now().UTC().Add(-time.Minute),
		ContinuedFromRunID: pgtype.UUID{},
		ParentRunID:        pgtype.UUID{Bytes: parentRun.ID, Valid: true},
		RootRunID:          rootRunID,
		ChildKey:           &childKey,
		ChildDepth:         parentRun.ChildDepth + 1,
	})
	require.NoError(t, err)

	startedHistory := appendEngineHistoryEvent(
		t,
		ctx,
		engineQueries,
		parentRun.ProjectID,
		childInstance.ID,
		childRun.ID,
		1,
		publichistory.EventWorkflowStarted,
		publichistory.WorkflowStartedPayload{
			DefinitionName:    definitionName,
			DefinitionVersion: definitionVersion,
			InstanceKey:       instanceKey,
			Input:             []byte(`{"seeded":true}`),
		},
	)

	_, err = engineQueries.CreateChildWorkflow(ctx, enginedb.CreateChildWorkflowParams{
		ProjectID:                  parentRun.ProjectID,
		ParentInstanceID:           parentInstance.ID,
		ParentRunID:                parentRun.ID,
		ChildKey:                   childKey,
		RequestedDefinitionName:    definitionName,
		RequestedDefinitionVersion: definitionVersion,
		ChildInstanceID:            childInstance.ID,
		ChildInstanceKey:           instanceKey,
		CurrentChildRunID:          childRun.ID,
		RootRunID:                  childRun.RootRunID,
		ChildDepth:                 childRun.ChildDepth,
	})
	require.NoError(t, err)

	createEngineProjectedTraceShell(t, ctx, platformStore, childInstance, childRun, startedHistory.ID, startedHistory.CreatedAt)
	return childInstance, childRun
}

//nolint:revive // Keep testing.T first in test helper signatures.
func createEngineProjectedTraceShell(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	instance enginedb.EngineInstance,
	run enginedb.EngineRun,
	latestHistoryID int64,
	startTime time.Time,
) {
	t.Helper()

	runStatus := string(run.Status)
	projectionState := publicprojection.StateUpToDate.String()
	_, err := platformStore.Queries().CreateEngineTraceShell(ctx, platformdb.CreateEngineTraceShellParams{
		ProjectID:                    run.ProjectID,
		SessionID:                    pgtype.UUID{},
		TraceID:                      "engine:" + run.ID.String(),
		Name:                         testutil.StrPtr(instance.DefinitionName),
		UserID:                       nil,
		Tags:                         nil,
		Environment:                  nil,
		Release:                      nil,
		Metadata:                     nil,
		Input:                        []byte(`{"seeded":true}`),
		Output:                       nil,
		Status:                       "running",
		StartTime:                    pgtype.Timestamptz{Time: startTime, Valid: true},
		EndTime:                      pgtype.Timestamptz{},
		EngineRunID:                  pgtype.UUID{Bytes: run.ID, Valid: true},
		EngineInstanceKey:            testutil.StrPtr(instance.InstanceKey),
		EngineRunStatus:              &runStatus,
		EngineCustomStatus:           nil,
		EngineWaitState:              nil,
		EnginePendingActivityTasks:   testutil.Ptr(int64(0)),
		EnginePendingInboxItems:      testutil.Ptr(int64(0)),
		EngineDefinitionName:         testutil.StrPtr(instance.DefinitionName),
		EngineDefinitionVersion:      &run.DefinitionVersion,
		EngineParentRunID:            run.ParentRunID,
		EngineRootRunID:              pgtype.UUID{Bytes: run.RootRunID, Valid: run.RootRunID != uuid.Nil},
		EngineChildKey:               run.ChildKey,
		EngineChildDepth:             testutil.Ptr(run.ChildDepth),
		EngineProjectionState:        &projectionState,
		EngineLatestHistoryID:        &latestHistoryID,
		EngineLastProjectedHistoryID: &latestHistoryID,
		EngineProjectionUpdatedAt:    pgtype.Timestamptz{Time: startTime, Valid: true},
	})
	require.NoError(t, err)
}

//nolint:revive // Keep testing.T first in test helper signatures.
func assertTerminatedRunState(
	t *testing.T,
	ctx context.Context,
	engineQueries *enginedb.Queries,
	projectID uuid.UUID,
	runID uuid.UUID,
	wantTerminalEvent string,
) {
	t.Helper()

	run, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	require.NoError(t, err)
	assert.Equal(t, enginedb.EngineRunLifecycleStatusTerminated, run.Status)

	historyRows, err := engineQueries.GetHistoryByRun(ctx, runID)
	require.NoError(t, err)
	require.NotEmpty(t, historyRows)
	assert.Equal(t, wantTerminalEvent, historyRows[len(historyRows)-1].EventType)
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
func markEngineRunCompleted(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	runID uuid.UUID,
	traceID uuid.UUID,
) {
	t.Helper()

	completedAt := time.Now().UTC().Round(time.Microsecond)
	result := []byte(`{"ok":true}`)

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    waiting_for = NULL,
		    result = $2,
		    completed_at = $3,
		    updated_at = $3
		WHERE id = $1
	`, runID, result, completedAt)
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.instances
		SET status = 'completed',
		    updated_at = $2
		WHERE id = (
		    SELECT instance_id
		    FROM engine.runs
		    WHERE id = $1
		)
	`, runID, completedAt)
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET status = 'completed',
		    end_time = $2,
		    output = $3::jsonb,
		    engine_run_status = 'completed',
		    updated_at = $2
		WHERE id = $1
	`, traceID, completedAt, result)
	require.NoError(t, err)
}

//nolint:revive // Keep testing.T first in test helper signatures.
func markEngineRunCompletedEngineOnly(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	runID uuid.UUID,
) {
	t.Helper()

	completedAt := time.Now().UTC().Round(time.Microsecond)
	result := []byte(`{"ok":true}`)

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.runs
		SET status = 'completed',
		    waiting_for = NULL,
		    result = $2,
		    completed_at = $3,
		    updated_at = $3
		WHERE id = $1
	`, runID, result, completedAt)
	require.NoError(t, err)

	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE engine.instances
		SET status = 'completed',
		    updated_at = $2
		WHERE id = (
		    SELECT instance_id
		    FROM engine.runs
		    WHERE id = $1
		)
	`, runID, completedAt)
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
		ProjectID:       projectID,
		InstanceID:      run.InstanceID,
		RunID:           runID,
		HistoryID:       &history[0].ID,
		ActivityKey:     "approval-task",
		ActivityType:    "demo.activity",
		Input:           []byte(`{"ok":true}`),
		AvailableAt:     history[0].CreatedAt,
		ExecutionTarget: "local",
		MaxAttempts:     1,
	})
	require.NoError(t, err)

	_, err = engineQueries.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: runID, Valid: true},
		HistoryID:   &history[0].ID,
		Kind:        "signal",
		Payload:     []byte(`{"signal_name":"approval"}`),
		AvailableAt: history[0].CreatedAt,
	})
	require.NoError(t, err)
}

//nolint:revive // Keep testing.T first in test helper signatures.
func setEngineProjectionCheckpoint(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	traceID uuid.UUID,
	projectionState string,
	latestHistoryID int64,
	lastProjectedHistoryID int64,
) {
	t.Helper()

	setEngineProjectionCheckpointAt(
		t,
		ctx,
		platformStore,
		traceID,
		projectionState,
		latestHistoryID,
		lastProjectedHistoryID,
		time.Now().UTC(),
	)
}

//nolint:revive // Keep testing.T first in test helper signatures.
func setEngineProjectionCheckpointAt(
	t *testing.T,
	ctx context.Context,
	platformStore *store.Store,
	traceID uuid.UUID,
	projectionState string,
	latestHistoryID int64,
	lastProjectedHistoryID int64,
	updatedAt time.Time,
) {
	t.Helper()

	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET engine_projection_state = $2,
		    engine_latest_history_id = $3,
		    engine_last_projected_history_id = $4,
		    engine_projection_updated_at = $5,
		    updated_at = $5
		WHERE id = $1
	`, traceID, projectionState, latestHistoryID, lastProjectedHistoryID, updatedAt)
	require.NoError(t, err)
}

func setTraceProjectionState(ctx context.Context, t *testing.T, platformStore *store.Store, traceID uuid.UUID, projectionState string) {
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

func waitForEngineRun(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	runID uuid.UUID,
	predicate func(EngineRunResponse) bool,
) EngineRunResponse {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var last EngineRunResponse
	for time.Now().Before(deadline) {
		rec := invokeGetEngineRun(t, server, projectID, runID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected get run to succeed while polling, got status=%d body=%s", rec.Code, rec.Body.String())
		}
		last = decodeJSONBody[EngineRunResponse](t, rec)
		if predicate(last) {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for engine run predicate, last run = %+v", last)
	return EngineRunResponse{}
}

func waitForActivityTask(
	ctx context.Context,
	t *testing.T,
	engineQueries *enginedb.Queries,
	runID uuid.UUID,
	activityKey string,
	predicate func(enginedb.EngineActivityTask) bool,
) enginedb.EngineActivityTask {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var last enginedb.EngineActivityTask
	for time.Now().Before(deadline) {
		task, err := engineQueries.GetActivityTaskByRunAndKey(ctx, enginedb.GetActivityTaskByRunAndKeyParams{
			RunID:       runID,
			ActivityKey: activityKey,
		})
		require.NoError(t, err)
		last = task
		if predicate(task) {
			return task
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for activity task predicate, last task = %+v", last)
	return enginedb.EngineActivityTask{}
}

func waitForDueTimerRun(
	ctx context.Context,
	t *testing.T,
	engineQueries *enginedb.Queries,
	projectID uuid.UUID,
	runID uuid.UUID,
) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var last []pgtype.UUID
	for time.Now().Before(deadline) {
		dueRunIDs, err := engineQueries.ListDueTimerRunIDsByProject(ctx, projectID)
		require.NoError(t, err)
		last = dueRunIDs
		for _, dueRunID := range dueRunIDs {
			if dueRunID.Valid && dueRunID.Bytes == runID {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for due timer inbox item for run %s, last due run IDs = %+v", runID, last)
}

func historyEventTypes(rows []enginedb.EngineHistory) []string {
	eventTypes := make([]string, 0, len(rows))
	for _, row := range rows {
		eventTypes = append(eventTypes, row.EventType)
	}
	return eventTypes
}

func historyEventIndex(rows []enginedb.EngineHistory, eventType string) int {
	for i, row := range rows {
		if row.EventType == eventType {
			return i
		}
	}
	return -1
}

func countHistoryEventType(rows []enginedb.EngineHistory, eventType string) int {
	count := 0
	for _, row := range rows {
		if row.EventType == eventType {
			count++
		}
	}
	return count
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

func startExternalEngineServeProcess(t *testing.T, projectID uuid.UUID) *externalEngineServeProcess {
	return startExternalEngineServeProcessWithEnv(t, projectID)
}

func startExternalEngineServeProcessWithEnv(t *testing.T, projectID uuid.UUID, extraEnv ...string) *externalEngineServeProcess {
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
		"CONTINUA_ENGINE_TEST_PROJECT_FILTER="+projectID.String(),
	)
	cmd.Env = applyEnvOverrides(cmd.Env, extraEnv...)

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

func applyEnvOverrides(base []string, overrides ...string) []string {
	result := append([]string(nil), base...)
	for _, override := range overrides {
		key, _, ok := strings.Cut(override, "=")
		if !ok {
			result = append(result, override)
			continue
		}

		replaced := false
		for i, entry := range result {
			entryKey, _, entryOK := strings.Cut(entry, "=")
			if entryOK && entryKey == key {
				result[i] = override
				replaced = true
			}
		}
		if !replaced {
			result = append(result, override)
		}
	}
	return result
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
func waitForProjectedTraceCaughtUp(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	traceID uuid.UUID,
	expectedHistoryID int64,
) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var lastProjectionState string
	var lastLatestHistoryID int64
	var lastProjectedHistoryID int64
	for time.Now().Before(deadline) {
		err := pool.QueryRow(ctx, `
			SELECT engine_projection_state,
			       engine_latest_history_id,
			       engine_last_projected_history_id
			FROM public.traces
			WHERE id = $1
		`, traceID).Scan(&lastProjectionState, &lastLatestHistoryID, &lastProjectedHistoryID)
		require.NoError(t, err)
		if lastProjectionState == publicprojection.StateUpToDate.String() &&
			lastLatestHistoryID >= expectedHistoryID &&
			lastProjectedHistoryID >= expectedHistoryID {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf(
		"timed out waiting for projected trace to catch up to history id %d, last state=%q latest=%d projected=%d",
		expectedHistoryID,
		lastProjectionState,
		lastLatestHistoryID,
		lastProjectedHistoryID,
	)
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
