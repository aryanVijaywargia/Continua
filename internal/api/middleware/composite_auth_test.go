package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestClassifyRouteProtection_MatchesOperatorAuthPlan(t *testing.T) {
	assert.Equal(t, routeProtectionPublic, classifyRouteProtection("/api/auth/config"))
	assert.Equal(t, routeProtectionAPIKeyOnly, classifyRouteProtection("/v1/ingest"))
	assert.Equal(t, routeProtectionComposite, classifyRouteProtection("/api/traces"))
	assert.Equal(t, routeProtectionComposite, classifyRouteProtection("/api/projects"))
	assert.Equal(t, routeProtectionComposite, classifyRouteProtection("/v1/engine/runs"))
	assert.Equal(t, routeProtectionComposite, classifyRouteProtection("/v1/engine/instances/customer-123"))
	assert.Equal(t, routeProtectionComposite, classifyRouteProtection("/v1/engine/projections/backfill"))
	assert.Equal(t, routeProtectionComposite, classifyRouteProtection("/v1/engine/runs/11111111-1111-1111-1111-111111111111"))
	assert.Equal(
		t,
		routeProtectionComposite,
		classifyRouteProtection("/v1/engine/runs/11111111-1111-1111-1111-111111111111/pending-work"),
	)
	assert.Equal(t, routeProtectionAPIKeyOnly, classifyRouteProtection("/v1/engine/activities/claim"))
	assert.Equal(
		t,
		routeProtectionAPIKeyOnly,
		classifyRouteProtection("/v1/engine/activities/11111111-1111-1111-1111-111111111111/heartbeat"),
	)
}

func TestPublicDemoReadRequest_OnlyMatchesDebuggerReadRoutes(t *testing.T) {
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/traces"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/traces/trace-123"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/traces/trace-123/spans"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/traces/trace-123/events"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/sessions"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/sessions/session-123"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/sessions/session-123/narrative"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/api/sessions/session-123/compare"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/v1/engine/instances/customer-123"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/v1/engine/runs/run-123"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/v1/engine/runs/run-123/pending-work"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/v1/engine/runs/run-123/history"))
	assert.True(t, isPublicDemoReadRequest(http.MethodGet, "/v1/engine/runs/run-123/result"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/api/traces/trace-123/export"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/api/sessions/session-123/admin"))
	assert.False(t, isPublicDemoReadRequest(http.MethodPost, "/api/traces"))
	assert.False(t, isPublicDemoReadRequest(http.MethodPost, "/v1/engine/runs"))
	assert.False(t, isPublicDemoReadRequest(http.MethodPost, "/v1/engine/runs/run-123/signal"))
	assert.False(t, isPublicDemoReadRequest(http.MethodPost, "/v1/engine/projections/backfill"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/api/projects"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/v1/ingest"))
}

func TestPublicDemoAllowedRequest_IncludesProjectionBackfillDryRunRoute(t *testing.T) {
	assert.True(t, isPublicDemoAllowedRequest(http.MethodGet, "/api/traces"))
	assert.True(t, isPublicDemoAllowedRequest(http.MethodPost, "/v1/engine/projections/backfill"))
	assert.False(t, isPublicDemoAllowedRequest(http.MethodPost, "/v1/engine/runs"))
	assert.False(t, isPublicDemoAllowedRequest(http.MethodPost, "/v1/engine/runs/run-123/signal"))
	assert.False(t, isPublicDemoAllowedRequest(http.MethodPost, "/api/traces"))
}

func TestCompositeAuthRejectsMissingCredentialsOnDebuggerRoutes(t *testing.T) {
	authenticator := &Authenticator{}
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("unexpected handler invocation")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "missing_credentials", resp["code"])
}

func TestCompositeAuthAcceptsLegacyAPIKeyBearerFallbackOnDebuggerRoutes(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)

	apiKey := "legacy-debugger-key-" + uuid.NewString()
	project := createCompositeAuthProject(ctx, t, platformStore, apiKey)
	authenticator := &Authenticator{store: platformStore}

	var receivedMode AuthMode
	var receivedProjectID uuid.UUID
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedMode, ok = GetAuthMode(r.Context())
		require.True(t, ok)
		receivedProjectID, ok = GetProjectID(r.Context())
		require.True(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, AuthModeAPIKey, receivedMode)
	assert.Equal(t, project.ID, receivedProjectID)
}

func TestCompositeAuthAcceptsAPIKeyOnEngineProjectionBackfill(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)

	apiKey := "projection-backfill-key-" + uuid.NewString()
	project := createCompositeAuthProject(ctx, t, platformStore, apiKey)
	authenticator := &Authenticator{store: platformStore}

	var receivedMode AuthMode
	var receivedProjectID uuid.UUID
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedMode, ok = GetAuthMode(r.Context())
		require.True(t, ok)
		receivedProjectID, ok = GetProjectID(r.Context())
		require.True(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/projections/backfill", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, AuthModeAPIKey, receivedMode)
	assert.Equal(t, project.ID, receivedProjectID)
}

func TestCompositeAuthAcceptsAPIKeyOnEngineConsoleRoutes(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)

	apiKey := "engine-console-key-" + uuid.NewString()
	project := createCompositeAuthProject(ctx, t, platformStore, apiKey)
	authenticator := &Authenticator{store: platformStore}

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "start run", method: http.MethodPost, path: "/v1/engine/runs"},
		{name: "instance lookup", method: http.MethodGet, path: "/v1/engine/instances/customer-123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var receivedMode AuthMode
			var receivedProjectID uuid.UUID
			protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var ok bool
				receivedMode, ok = GetAuthMode(r.Context())
				require.True(t, ok)
				receivedProjectID, ok = GetProjectID(r.Context())
				require.True(t, ok)
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-API-Key", apiKey)
			rec := httptest.NewRecorder()

			protectedHandler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNoContent, rec.Code)
			assert.Equal(t, AuthModeAPIKey, receivedMode)
			assert.Equal(t, project.ID, receivedProjectID)
		})
	}
}

func TestCompositeAuthAllowsPublicDemoReadWithoutCredentials(t *testing.T) {
	demoProjectID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	authenticator := &Authenticator{
		publicDemo: &publicDemoAccess{projectID: demoProjectID},
	}

	var receivedMode AuthMode
	var receivedProjectID uuid.UUID
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedMode, ok = GetAuthMode(r.Context())
		require.True(t, ok)
		receivedProjectID, ok = GetProjectID(r.Context())
		require.True(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/traces?project_id=22222222-2222-2222-2222-222222222222",
		nil,
	)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, AuthModePublicDemo, receivedMode)
	assert.Equal(t, demoProjectID, receivedProjectID)
}

func TestCompositeAuthAllowsPublicDemoProjectionBackfillWithoutCredentials(t *testing.T) {
	demoProjectID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	authenticator := &Authenticator{
		publicDemo: &publicDemoAccess{projectID: demoProjectID},
	}

	var receivedMode AuthMode
	var receivedProjectID uuid.UUID
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedMode, ok = GetAuthMode(r.Context())
		require.True(t, ok)
		receivedProjectID, ok = GetProjectID(r.Context())
		require.True(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/projections/backfill", nil)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, AuthModePublicDemo, receivedMode)
	assert.Equal(t, demoProjectID, receivedProjectID)
}

func TestCompositeAuthStillRejectsProjectListWithoutCredentialsInPublicDemo(t *testing.T) {
	authenticator := &Authenticator{
		publicDemo: &publicDemoAccess{projectID: uuid.MustParse("11111111-1111-1111-1111-111111111111")},
	}
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("unexpected handler invocation")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "missing_credentials", resp["code"])
}

func TestProjectBootstrapRouteOnlyMatchesFirstRunProjectSurface(t *testing.T) {
	assert.True(t, isProjectBootstrapRoute(http.MethodGet, "/api/projects"))
	assert.True(t, isProjectBootstrapRoute(http.MethodPost, "/api/projects"))
	assert.False(t, isProjectBootstrapRoute(http.MethodPatch, "/api/projects"))
	assert.False(t, isProjectBootstrapRoute(http.MethodGet, "/api/projects/project-id"))
	assert.False(t, isProjectBootstrapRoute(http.MethodGet, "/api/traces"))
}

func createCompositeAuthProject(
	ctx context.Context,
	t *testing.T,
	platformStore *store.Store,
	apiKey string,
) platform.Project {
	t.Helper()

	project, err := platformStore.Queries().CreateProject(ctx, platform.CreateProjectParams{
		Name:       "auth-project-" + uuid.NewString()[:8],
		ApiKeyHash: hashAPIKey(apiKey),
	})
	require.NoError(t, err)

	return project
}

func decodeAuthErrorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()

	var body map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	return body
}
