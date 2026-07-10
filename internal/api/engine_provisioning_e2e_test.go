package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestEngineServeUnderProvisionedProjectE2E(t *testing.T) {
	missingProjectID := uuid.New()
	missingProjectServe := startExternalEngineServeProcessWithEnv(
		t,
		missingProjectID,
		"ENGINE_PROJECT_ID="+missingProjectID.String(),
		"CONTINUA_ENGINE_TEST_PROJECT_FILTER=",
	)

	select {
	case err := <-missingProjectServe.done:
		if err == nil {
			t.Fatal("engine serve exited successfully for missing project, want non-nil error")
		}
		output := missingProjectServe.stdout.String() + missingProjectServe.stderr.String()
		if !strings.Contains(output, missingProjectID.String()) {
			t.Fatalf("engine serve output = %q, want missing project id %s", output, missingProjectID)
		}
	case <-time.After(60 * time.Second):
		missingProjectServe.stop(t)
		t.Fatalf("engine serve did not fail within 60s for missing project %s", missingProjectID)
	}

	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	apiKey := "engine-provisioned-" + uuid.NewString()
	_, err := platformStore.Pool().Exec(ctx, `
		UPDATE public.projects
		SET api_key_hash = $2
		WHERE id = $1
	`, projectID, hashTestAPIKey(apiKey))
	require.NoError(t, err)
	require.NoError(t, publishEngineDefinition(ctx, engineQueries, "darklaunch.demo"))

	engineServe := startExternalEngineServeProcessWithEnv(
		t,
		projectID,
		"ENGINE_PROJECT_ID="+projectID.String(),
		"CONTINUA_ENGINE_TEST_PROJECT_FILTER=",
		"ENGINE_ACTIVITY_POLL_INTERVAL=50ms",
		"ENGINE_MAINTENANCE_POLL_INTERVAL=50ms",
	)
	defer engineServe.stop(t)

	router := newAuthenticatedRouter(t, server, platformStore)
	start := startEngineRunThroughRouter(t, router, apiKey, EngineStartRunRequest{
		DefinitionName:    "darklaunch.demo",
		DefinitionVersion: "v1",
		InstanceKey:       "provisioned-e2e-instance",
		RequestKey:        "provisioned-e2e-request",
		Input: map[string]any{
			"name":     "Provisioned",
			"timer_at": "1970-01-01T00:00:00Z",
		},
		Trace: &EngineStartTrace{
			Name: ptrString("Provisioned Demo"),
		},
	})

	require.Equal(t, http.StatusOK, invokeSignalEngineRun(t, server, projectID, start.RunId, EngineSignalRunRequest{
		SignalName: "approval",
		Payload:    map[string]any{"approval": "yes"},
	}).Code)

	completed := waitForEngineRun(t, server, projectID, start.RunId, func(run EngineRunResponse) bool {
		return run.Status == EngineRunStatusCOMPLETED
	})
	require.Equal(t, EngineRunStatusCOMPLETED, completed.Status)

	resultRec := invokeGetEngineRunResult(t, server, projectID, start.RunId)
	require.Equal(t, http.StatusOK, resultRec.Code)
	result := decodeJSONBody[EngineRunResultResponse](t, resultRec)
	require.Equal(t, EngineRunStatusCOMPLETED, result.Status)
	require.NotNil(t, result.Result)

	traceListRec := invokeListTraces(t, server, projectID, ListTracesParams{})
	require.Equal(t, http.StatusOK, traceListRec.Code)
	traceList := decodeJSONBody[TraceList](t, traceListRec)
	var found bool
	for _, trace := range traceList.Traces {
		if trace.Engine == nil || trace.Engine.RunId != start.RunId {
			continue
		}
		found = true
		require.Equal(t, "darklaunch.demo", trace.Engine.DefinitionName)
		break
	}
	if !found {
		t.Fatalf("GET /api/traces did not include engine run linkage for run %s: %+v", start.RunId, traceList.Traces)
	}

	var sentinelCount int
	err = platformStore.Pool().QueryRow(ctx, `
		SELECT COUNT(*)
		FROM public.projects
		WHERE id = '00000000-0000-0000-0000-000000000001'
	`).Scan(&sentinelCount)
	require.NoError(t, err)
	require.Equal(t, 0, sentinelCount)

	_, err = engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        start.RunId,
	})
	require.NoError(t, err)
}

func startEngineRunThroughRouter(t *testing.T, router http.Handler, apiKey string, reqBody EngineStartRunRequest) EngineStartRunResponse {
	t.Helper()

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs", bytes.NewReader(body))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set(enginePreviewHeader, "1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSONBody[EngineStartRunResponse](t, rec)
}

func ptrString(value string) *string {
	return &value
}
