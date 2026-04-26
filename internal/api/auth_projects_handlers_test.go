package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestGetAuthConfig_DisabledReturnsOnlyEnabledFalse(t *testing.T) {
	server := NewServer(nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)

	server.GetAuthConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[AuthConfig](t, rec)
	assert.False(t, resp.Enabled)
	assert.Nil(t, resp.Domain)
	assert.Nil(t, resp.ClientId)
	assert.Nil(t, resp.Audience)
	assert.Nil(t, resp.PublicDemoEnabled)
	assert.Nil(t, resp.PublicDemoLabel)
}

func TestGetAuthConfig_EnabledReturnsRuntimeBootstrapFields(t *testing.T) {
	server := NewServer(nil, nil)
	server.auth0Config = config.Auth0Config{
		Enabled:  true,
		Domain:   "continua.us.auth0.com",
		ClientID: "operator-client-id",
		Audience: "https://continua/operator",
		AllowedEmails: []string{
			"operator@example.com",
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)

	server.GetAuthConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[AuthConfig](t, rec)
	require.True(t, resp.Enabled)
	require.NotNil(t, resp.Domain)
	require.NotNil(t, resp.ClientId)
	require.NotNil(t, resp.Audience)
	assert.Equal(t, server.auth0Config.Domain, *resp.Domain)
	assert.Equal(t, server.auth0Config.ClientID, *resp.ClientId)
	assert.Equal(t, server.auth0Config.Audience, *resp.Audience)
}

func TestGetAuthConfig_PublicDemoReturnsDemoFields(t *testing.T) {
	server := NewServer(nil, nil)
	server.publicDemoConfig = config.PublicDemoConfig{
		Enabled: true,
		Label:   "Portfolio demo",
	}
	server.auth0Config = config.Auth0Config{
		Enabled:  true,
		Domain:   "continua.us.auth0.com",
		ClientID: "operator-client-id",
		Audience: "https://continua/operator",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)

	server.GetAuthConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[AuthConfig](t, rec)
	assert.False(t, resp.Enabled)
	assert.Nil(t, resp.Domain)
	assert.Nil(t, resp.ClientId)
	assert.Nil(t, resp.Audience)
	require.NotNil(t, resp.PublicDemoEnabled)
	require.NotNil(t, resp.PublicDemoLabel)
	assert.True(t, *resp.PublicDemoEnabled)
	assert.Equal(t, "Portfolio demo", *resp.PublicDemoLabel)
}

func TestListProjects_OperatorReturnsAllVisibleProjects(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	const extraProjects = 503
	expectedProjectIDs := make(map[string]struct{}, extraProjects+2)

	projectAID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	projectBID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	expectedProjectIDs[projectAID.String()] = struct{}{}
	expectedProjectIDs[projectBID.String()] = struct{}{}
	for i := 0; i < extraProjects; i++ {
		projectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
		expectedProjectIDs[projectID.String()] = struct{}{}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	reqCtx = context.WithValue(reqCtx, middleware.OperatorEmailKey, "operator@example.com")
	reqCtx = context.WithValue(reqCtx, middleware.OperatorSubjectKey, "google-oauth2|operator")
	rec := httptest.NewRecorder()

	server.ListProjects(rec, req.WithContext(reqCtx))
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[ProjectList](t, rec)
	require.GreaterOrEqual(t, len(resp.Projects), len(expectedProjectIDs))
	for _, project := range resp.Projects {
		delete(expectedProjectIDs, project.Id.String())
	}
	assert.Empty(t, expectedProjectIDs)
}

func TestListProjects_APIKeyContextReturnsOnlyScopedProject(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	projectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	_ = testutil.CreateTestProject(t, ctx, platformStore.Queries())

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	reqCtx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.ListProjects(rec, req.WithContext(reqCtx))
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[ProjectList](t, rec)
	require.Len(t, resp.Projects, 1)
	assert.Equal(t, projectID, resp.Projects[0].Id)
}
