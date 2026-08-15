package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

type projectScopeFixture struct {
	server   *Server
	router   http.Handler
	apiKeyA  string
	projectA platform.Project
	projectB platform.Project
	traceA   platform.Trace
	traceB   platform.Trace
	sessionA platform.Session
	sessionB platform.Session
}

func TestListTracesWithMismatchedProjectIDIsRejected(t *testing.T) {
	fixture := newProjectScopeFixture(t)

	rec := fixture.getWithAPIKey(
		t,
		"/api/traces?project_id="+fixture.projectB.ID.String(),
		fixture.apiKeyA,
	)

	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "project_scope_mismatch", resp.Code)
	assert.NotContains(t, rec.Body.String(), fixture.traceA.ID.String())
}

func TestListTracesWithMatchingProjectIDReturnsThatProject(t *testing.T) {
	fixture := newProjectScopeFixture(t)

	rec := fixture.getWithAPIKey(
		t,
		"/api/traces?project_id="+fixture.projectA.ID.String(),
		fixture.apiKeyA,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[TraceList](t, rec)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Traces, 1)
	assert.Equal(t, fixture.traceA.ID, resp.Traces[0].Id)
	assert.Equal(t, "Project A trace", resp.Traces[0].Name)
	assert.NotContains(t, rec.Body.String(), fixture.traceB.ID.String())
}

func TestListTracesWithoutProjectIDUsesAPIKeyProject(t *testing.T) {
	fixture := newProjectScopeFixture(t)

	rec := fixture.getWithAPIKey(t, "/api/traces", fixture.apiKeyA)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[TraceList](t, rec)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Traces, 1)
	assert.Equal(t, fixture.traceA.ID, resp.Traces[0].Id)
	assert.Equal(t, "Project A trace", resp.Traces[0].Name)
	assert.NotContains(t, rec.Body.String(), fixture.traceB.ID.String())
}

func TestListTracesWithMalformedProjectIDIsRejected(t *testing.T) {
	fixture := newProjectScopeFixture(t)

	rec := fixture.getWithAPIKey(
		t,
		"/api/traces?project_id=not-a-uuid",
		fixture.apiKeyA,
	)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	// The OpenAPI-generated router owns UUID parameter binding and rejects this
	// request before the handler's shared scope resolver is reached.
	assert.NotContains(t, rec.Body.String(), fixture.traceA.ID.String())
}

func TestListSessionsWithMismatchedProjectIDIsRejected(t *testing.T) {
	fixture := newProjectScopeFixture(t)

	rec := fixture.getWithAPIKey(
		t,
		"/api/sessions?project_id="+fixture.projectB.ID.String(),
		fixture.apiKeyA,
	)

	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "project_scope_mismatch", resp.Code)
	assert.NotContains(t, rec.Body.String(), fixture.sessionA.ID.String())
}

func TestOperatorProjectIDSelectionStillHonored(t *testing.T) {
	fixture := newProjectScopeFixture(t)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/traces?project_id="+fixture.projectB.ID.String(),
		nil,
	)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	reqCtx = context.WithValue(reqCtx, middleware.OperatorEmailKey, "operator@example.com")
	reqCtx = context.WithValue(reqCtx, middleware.OperatorSubjectKey, "google-oauth2|operator")
	rec := httptest.NewRecorder()

	fixture.server.ListTraces(rec, req.WithContext(reqCtx), ListTracesParams{})

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[TraceList](t, rec)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Traces, 1)
	assert.Equal(t, fixture.traceB.ID, resp.Traces[0].Id)
	assert.Equal(t, "Project B trace", resp.Traces[0].Name)
	assert.NotContains(t, rec.Body.String(), fixture.traceA.ID.String())
}

func newProjectScopeFixture(t *testing.T) projectScopeFixture {
	t.Helper()

	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	q := platformStore.Queries()
	apiKeyA := "project-scope-a-" + uuid.NewString()
	apiKeyB := "project-scope-b-" + uuid.NewString()

	projectA, err := q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       "Project A " + uuid.NewString(),
		ApiKeyHash: middleware.HashAPIKey(apiKeyA),
	})
	require.NoError(t, err)
	projectB, err := q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       "Project B " + uuid.NewString(),
		ApiKeyHash: middleware.HashAPIKey(apiKeyB),
	})
	require.NoError(t, err)

	sessionA, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectA.ID,
		ExternalID: "project-a-session-" + uuid.NewString(),
		Name:       testutil.StrPtr("Project A session"),
	})
	require.NoError(t, err)
	sessionB, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectB.ID,
		ExternalID: "project-b-session-" + uuid.NewString(),
		Name:       testutil.StrPtr("Project B session"),
	})
	require.NoError(t, err)

	traceA := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectA.ID,
		SessionID: testutil.PgtypeUUID(sessionA.ID),
		TraceID:   "project-a-trace-" + uuid.NewString(),
		Name:      testutil.StrPtr("Project A trace"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)),
	})
	traceB := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectB.ID,
		SessionID: testutil.PgtypeUUID(sessionB.ID),
		TraceID:   "project-b-trace-" + uuid.NewString(),
		Name:      testutil.StrPtr("Project B trace"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC)),
	})

	server := NewServer(platformStore, nil)
	return projectScopeFixture{
		server:   server,
		router:   newAuthenticatedRouter(t, server, platformStore),
		apiKeyA:  apiKeyA,
		projectA: projectA,
		projectB: projectB,
		traceA:   traceA,
		traceB:   traceB,
		sessionA: sessionA,
		sessionB: sessionB,
	}
}

func (f projectScopeFixture) getWithAPIKey(t *testing.T, path, apiKey string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}
