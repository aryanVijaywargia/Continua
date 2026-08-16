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
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

type localModeScopeFixture struct {
	router   http.Handler
	projectA platform.Project
	projectB platform.Project
	traceA   platform.Trace
	traceB   platform.Trace
	sessionA platform.Session
	sessionB platform.Session
}

func TestLocalModeTracesScopedToSelectedProject(t *testing.T) {
	fixture := newLocalModeScopeFixture(t)

	recB := fixture.getFromLoopback(t, "/api/traces?project_id="+fixture.projectB.ID.String())
	require.Equal(t, http.StatusOK, recB.Code)
	bodyB := recB.Body.String()
	respB := decodeJSONBody[TraceList](t, recB)
	assert.Equal(t, 1, respB.Total)
	require.Len(t, respB.Traces, 1)
	assert.Equal(t, fixture.traceB.ID, respB.Traces[0].Id)
	assert.Equal(t, "Local mode project B trace", respB.Traces[0].Name)
	assert.NotContains(t, bodyB, fixture.traceA.ID.String())

	recA := fixture.getFromLoopback(t, "/api/traces?project_id="+fixture.projectA.ID.String())
	require.Equal(t, http.StatusOK, recA.Code)
	bodyA := recA.Body.String()
	respA := decodeJSONBody[TraceList](t, recA)
	assert.Equal(t, 1, respA.Total)
	require.Len(t, respA.Traces, 1)
	assert.Equal(t, fixture.traceA.ID, respA.Traces[0].Id)
	assert.Equal(t, "Local mode project A trace", respA.Traces[0].Name)
	assert.NotContains(t, bodyA, fixture.traceB.ID.String())
}

func TestLocalModeTracesWithoutProjectIDIsRejected(t *testing.T) {
	fixture := newLocalModeScopeFixture(t)

	rec := fixture.getFromLoopback(t, "/api/traces")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := rec.Body.String()
	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "missing_project_id", resp.Code)
	assert.NotContains(t, body, fixture.traceA.ID.String())
	assert.NotContains(t, body, fixture.traceB.ID.String())
}

func TestLocalModeSessionsScopedToSelectedProject(t *testing.T) {
	fixture := newLocalModeScopeFixture(t)

	recB := fixture.getFromLoopback(t, "/api/sessions?project_id="+fixture.projectB.ID.String())
	require.Equal(t, http.StatusOK, recB.Code)
	bodyB := recB.Body.String()
	respB := decodeJSONBody[SessionList](t, recB)
	assert.Equal(t, 1, respB.Total)
	require.Len(t, respB.Sessions, 1)
	assert.Equal(t, fixture.sessionB.ID, respB.Sessions[0].Id)
	require.NotNil(t, respB.Sessions[0].Name)
	assert.Equal(t, "Local mode project B session", *respB.Sessions[0].Name)
	assert.NotContains(t, bodyB, fixture.sessionA.ID.String())

	recA := fixture.getFromLoopback(t, "/api/sessions?project_id="+fixture.projectA.ID.String())
	require.Equal(t, http.StatusOK, recA.Code)
	bodyA := recA.Body.String()
	respA := decodeJSONBody[SessionList](t, recA)
	assert.Equal(t, 1, respA.Total)
	require.Len(t, respA.Sessions, 1)
	assert.Equal(t, fixture.sessionA.ID, respA.Sessions[0].Id)
	require.NotNil(t, respA.Sessions[0].Name)
	assert.Equal(t, "Local mode project A session", *respA.Sessions[0].Name)
	assert.NotContains(t, bodyA, fixture.sessionB.ID.String())
}

func newLocalModeScopeFixture(t *testing.T) localModeScopeFixture {
	t.Helper()

	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	q := platformStore.Queries()

	projectA, err := q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       "Local mode project A " + uuid.NewString(),
		ApiKeyHash: middleware.HashAPIKey("local-mode-a-" + uuid.NewString()),
	})
	require.NoError(t, err)
	projectB, err := q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       "Local mode project B " + uuid.NewString(),
		ApiKeyHash: middleware.HashAPIKey("local-mode-b-" + uuid.NewString()),
	})
	require.NoError(t, err)

	sessionA, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectA.ID,
		ExternalID: "local-mode-a-session-" + uuid.NewString(),
		Name:       testutil.StrPtr("Local mode project A session"),
	})
	require.NoError(t, err)
	sessionB, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectB.ID,
		ExternalID: "local-mode-b-session-" + uuid.NewString(),
		Name:       testutil.StrPtr("Local mode project B session"),
	})
	require.NoError(t, err)

	traceA := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectA.ID,
		SessionID: testutil.PgtypeUUID(sessionA.ID),
		TraceID:   "local-mode-a-trace-" + uuid.NewString(),
		Name:      testutil.StrPtr("Local mode project A trace"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)),
	})
	traceB := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectB.ID,
		SessionID: testutil.PgtypeUUID(sessionB.ID),
		TraceID:   "local-mode-b-trace-" + uuid.NewString(),
		Name:      testutil.StrPtr("Local mode project B trace"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC)),
	})

	server := NewServer(platformStore, nil)
	authenticator, err := middleware.NewAuthenticator(platformStore, &config.Config{
		LocalSingleUserMode: true,
	})
	require.NoError(t, err)

	return localModeScopeFixture{
		router:   NewRouter(server, authenticator),
		projectA: projectA,
		projectB: projectB,
		traceA:   traceA,
		traceB:   traceB,
		sessionA: sessionA,
		sessionB: sessionB,
	}
}

// getFromLoopback issues a credential-free request that appears to originate
// from the local machine, which is the only shape local single-user mode serves.
func (f localModeScopeFixture) getFromLoopback(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}
