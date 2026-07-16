package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/internal/api/middleware"
)

func TestRootSideEngineStartPath_DoesNotImportEngineInternals(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	dir := filepath.Dir(currentFile)

	for _, file := range []string{"engine_control.go", "engine_handlers.go", "engine_mapper.go"} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("os.ReadFile(%s) error = %v", file, err)
		}
		source := string(data)
		if strings.Contains(source, "engine/internal/") {
			t.Fatalf("expected %s to avoid engine/internal imports", file)
		}
	}
}

func TestEngineReadPolicy_OperatorUnboundedAcrossProjects(t *testing.T) {
	fixture := seedEngineBoundaryRun(t)

	runRec := invokeEngineBoundaryReadAsOperator(t, fixture.server, engineBoundaryRun, fixture.runID, fixture.instanceKey, nil)
	require.Equal(t, http.StatusOK, runRec.Code, runRec.Body.String())
	run := decodeJSONBody[EngineRunResponse](t, runRec)
	assert.Equal(t, fixture.runID, run.RunId)
	assert.Equal(t, fixture.instanceKey, run.InstanceKey)

	historyRec := invokeEngineBoundaryReadAsOperator(t, fixture.server, engineBoundaryHistory, fixture.runID, fixture.instanceKey, nil)
	require.Equal(t, http.StatusOK, historyRec.Code, historyRec.Body.String())
	history := decodeJSONBody[EngineRunHistoryResponse](t, historyRec)
	require.Len(t, history.Events, 1)
	assert.Equal(t, publichistory.EventWorkflowStarted, history.Events[0].EventType)
	require.NotNil(t, history.Events[0].Payload)
	assert.Equal(t, fixture.instanceKey, (*history.Events[0].Payload)["instance_key"])

	resultRec := invokeEngineBoundaryReadAsOperator(t, fixture.server, engineBoundaryResult, fixture.runID, fixture.instanceKey, nil)
	require.Equal(t, http.StatusConflict, resultRec.Code, resultRec.Body.String())
	assert.Equal(t, "run_not_terminal", decodeJSONBody[Error](t, resultRec).Code)

	pendingRec := invokeEngineBoundaryReadAsOperator(t, fixture.server, engineBoundaryPendingWork, fixture.runID, fixture.instanceKey, nil)
	require.Equal(t, http.StatusOK, pendingRec.Code, pendingRec.Body.String())
	pending := decodeJSONBody[EnginePendingWorkResponse](t, pendingRec)
	assert.Equal(t, fixture.runID, pending.RunId)

	instanceRec := invokeEngineBoundaryReadAsOperator(t, fixture.server, engineBoundaryInstance, fixture.runID, fixture.instanceKey, nil)
	require.Equal(t, http.StatusOK, instanceRec.Code, instanceRec.Body.String())
	instance := decodeJSONBody[EngineInstanceResponse](t, instanceRec)
	assert.Equal(t, fixture.instanceKey, instance.InstanceKey)
	assert.Equal(t, fixture.runID, instance.CurrentRun.RunId)
}

func TestEngineReadPolicy_OperatorProjectIDActsAsFilter(t *testing.T) {
	fixture := seedEngineBoundaryRun(t)

	for _, read := range []engineBoundaryRead{engineBoundaryRun, engineBoundaryInstance} {
		t.Run(string(read), func(t *testing.T) {
			mismatch := invokeEngineBoundaryReadAsOperator(t, fixture.server, read, fixture.runID, fixture.instanceKey, &fixture.projectAID)
			require.Equal(t, http.StatusNotFound, mismatch.Code, mismatch.Body.String())

			selected := invokeEngineBoundaryReadAsOperator(t, fixture.server, read, fixture.runID, fixture.instanceKey, &fixture.projectBID)
			require.Equal(t, http.StatusOK, selected.Code, selected.Body.String())
			if read == engineBoundaryRun {
				response := decodeJSONBody[EngineRunResponse](t, selected)
				assert.Equal(t, fixture.runID, response.RunId)
				assert.Equal(t, fixture.instanceKey, response.InstanceKey)
				return
			}
			response := decodeJSONBody[EngineInstanceResponse](t, selected)
			assert.Equal(t, fixture.instanceKey, response.InstanceKey)
			assert.Equal(t, fixture.runID, response.CurrentRun.RunId)
		})
	}
}

func TestEngineReadPolicy_APIKeyBoundCrossProject(t *testing.T) {
	fixture := seedEngineBoundaryRun(t)

	paths := []struct {
		name string
		path string
	}{
		{name: "run", path: "/v1/engine/runs/" + fixture.runID.String()},
		{name: "history", path: "/v1/engine/runs/" + fixture.runID.String() + "/history"},
		{name: "result", path: "/v1/engine/runs/" + fixture.runID.String() + "/result"},
		{name: "pending-work", path: "/v1/engine/runs/" + fixture.runID.String() + "/pending-work"},
		{name: "instance", path: "/v1/engine/instances/" + fixture.instanceKey},
	}

	for _, target := range paths {
		t.Run(target.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target.path, nil)
			req.Header.Set("X-API-Key", fixture.apiKeyA)
			rec := httptest.NewRecorder()

			fixture.router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
}

func TestEngineReadPolicy_ActivityRoutesRejectOperator(t *testing.T) {
	_, platformStore, _, server, _ := setupEngineHandlerTest(t)
	router := newAuthenticatedRouter(t, server, platformStore)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/engine/activities/claim",
		bytes.NewBufferString(`{"worker_id":"operator","activity_types":["email.send"]}`),
	)
	req.Header.Set("Authorization", "Bearer operator.header.signature")
	req.Header.Set(enginePreviewHeader, "1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.Equal(t, "invalid_api_key", decodeJSONBody[map[string]string](t, rec)["code"])
}

type engineBoundaryFixture struct {
	server      *Server
	router      http.Handler
	projectAID  uuid.UUID
	projectBID  uuid.UUID
	apiKeyA     string
	runID       uuid.UUID
	instanceKey string
}

func seedEngineBoundaryRun(t *testing.T) engineBoundaryFixture {
	t.Helper()

	ctx, platformStore, engineQueries, server, projectAID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	apiKeyA := "engine-boundary-a-" + uuid.NewString()
	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE public.projects
		SET api_key_hash = $2
		WHERE id = $1
	`, projectAID, hashTestAPIKey(apiKeyA))
	require.NoError(t, err)

	apiKeyB := "engine-boundary-b-" + uuid.NewString()
	projectB, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "engine-boundary-b-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKeyB),
	})
	require.NoError(t, err)

	router := newAuthenticatedRouter(t, server, platformStore)
	suffix := uuid.NewString()
	instanceKey := "operator-boundary-instance-" + suffix
	start := startEngineRunThroughRouter(t, router, apiKeyB, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
		RequestKey:        "operator-boundary-request-" + suffix,
	})

	return engineBoundaryFixture{
		server:      server,
		router:      router,
		projectAID:  projectAID,
		projectBID:  projectB.ID,
		apiKeyA:     apiKeyA,
		runID:       start.RunId,
		instanceKey: instanceKey,
	}
}

type engineBoundaryRead string

const (
	engineBoundaryRun         engineBoundaryRead = "run"
	engineBoundaryHistory     engineBoundaryRead = "history"
	engineBoundaryResult      engineBoundaryRead = "result"
	engineBoundaryPendingWork engineBoundaryRead = "pending-work"
	engineBoundaryInstance    engineBoundaryRead = "instance"
)

func invokeEngineBoundaryReadAsOperator(
	t *testing.T,
	server *Server,
	read engineBoundaryRead,
	runID uuid.UUID,
	instanceKey string,
	projectID *uuid.UUID,
) *httptest.ResponseRecorder {
	t.Helper()

	path := "/v1/engine/runs/" + runID.String()
	switch read {
	case engineBoundaryHistory:
		path += "/history"
	case engineBoundaryResult:
		path += "/result"
	case engineBoundaryPendingWork:
		path += "/pending-work"
	case engineBoundaryInstance:
		path = "/v1/engine/instances/" + instanceKey
	}
	if projectID != nil {
		path += "?project_id=" + projectID.String()
	}

	req := httptest.NewRequest(http.MethodGet, path, nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	reqCtx = context.WithValue(reqCtx, middleware.OperatorEmailKey, "operator@example.com")
	reqCtx = context.WithValue(reqCtx, middleware.OperatorSubjectKey, "google-oauth2|operator")
	rec := httptest.NewRecorder()

	switch read {
	case engineBoundaryRun:
		server.GetEngineRun(rec, req.WithContext(reqCtx), runID)
	case engineBoundaryHistory:
		server.GetEngineRunHistory(rec, req.WithContext(reqCtx), runID, GetEngineRunHistoryParams{})
	case engineBoundaryResult:
		server.GetEngineRunResult(rec, req.WithContext(reqCtx), runID)
	case engineBoundaryPendingWork:
		server.GetEngineRunPendingWork(rec, req.WithContext(reqCtx), runID)
	case engineBoundaryInstance:
		server.GetEngineInstance(rec, req.WithContext(reqCtx), instanceKey)
	default:
		t.Fatalf("unknown engine boundary read %q", read)
	}
	return rec
}
