package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	assert.Equal(t, routeProtectionAPIKeyOnly, classifyRouteProtection("/v1/engine/runs"))
	assert.Equal(t, routeProtectionAPIKeyOnly, classifyRouteProtection("/v1/engine/instances/customer-123"))
	assert.Equal(t, routeProtectionAPIKeyOnly, classifyRouteProtection("/v1/engine/projections/backfill"))
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
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/api/traces/trace-123/export"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/api/sessions/session-123/admin"))
	assert.False(t, isPublicDemoReadRequest(http.MethodPost, "/api/traces"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/api/projects"))
	assert.False(t, isPublicDemoReadRequest(http.MethodGet, "/v1/ingest"))
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

func TestCompositeAuthRejectsInvalidJWTOnDebuggerRoutes(t *testing.T) {
	authenticator := &Authenticator{
		auth0: &auth0Authenticator{
			validateToken: func(context.Context, string) (any, error) {
				return nil, errors.New("invalid token")
			},
			allowedEmails: map[string]struct{}{},
		},
	}
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("unexpected handler invocation")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("Authorization", "Bearer header.payload.signature")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "invalid_token", resp["code"])
}

func TestCompositeAuthRejectsNonAllowlistedOperatorOnDebuggerRoutes(t *testing.T) {
	authenticator := &Authenticator{
		auth0: &auth0Authenticator{
			validateToken: func(context.Context, string) (any, error) {
				return validatedAuth0Claims(
					"google-oauth2|operator",
					"outside@example.com",
					time.Now().Add(time.Hour),
				), nil
			},
			allowedEmails: map[string]struct{}{"operator@example.com": {}},
		},
	}
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("unexpected handler invocation")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("Authorization", "Bearer header.payload.signature")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "forbidden_operator", resp["code"])
}

func TestCompositeAuthAcceptsAllowlistedOperatorBearerOnDebuggerRoutes(t *testing.T) {
	authenticator := &Authenticator{
		auth0: &auth0Authenticator{
			validateToken: func(context.Context, string) (any, error) {
				return validatedAuth0Claims(
					"google-oauth2|operator",
					"Operator@Example.com",
					time.Now().Add(time.Hour),
				), nil
			},
			allowedEmails: map[string]struct{}{"operator@example.com": {}},
		},
	}

	var receivedMode AuthMode
	var receivedEmail string
	var receivedSubject string
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedMode, ok = GetAuthMode(r.Context())
		require.True(t, ok)
		receivedEmail, ok = GetOperatorEmail(r.Context())
		require.True(t, ok)
		receivedSubject, ok = GetOperatorSubject(r.Context())
		require.True(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("Authorization", "Bearer header.payload.signature")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, AuthModeOperator, receivedMode)
	assert.Equal(t, "operator@example.com", receivedEmail)
	assert.Equal(t, "google-oauth2|operator", receivedSubject)
}

func TestCompositeAuthAcceptsLegacyAPIKeyBearerFallbackOnDebuggerRoutes(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)

	apiKey := "legacy-debugger-key-" + uuid.NewString()
	project := createCompositeAuthProject(t, ctx, platformStore, apiKey)
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

func createCompositeAuthProject(
	t *testing.T,
	ctx context.Context,
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
