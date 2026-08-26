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

func TestGetAuthConfig_PublicDemoReturnsDemoFields(t *testing.T) {
	server := NewServer(nil, nil)
	server.publicDemoConfig = config.PublicDemoConfig{
		Enabled: true,
		Label:   "Portfolio demo",
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

func TestAuthConfigAdvertisesLocalModeOnLoopback(t *testing.T) {
	server := NewServer(nil, nil)
	server.localSingleUserMode = true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
	req.RemoteAddr = "127.0.0.1:54321"

	server.GetAuthConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[AuthConfig](t, rec)
	require.NotNil(t, resp.LocalModeEnabled)
	assert.True(t, *resp.LocalModeEnabled)
}

func TestAuthConfigHidesLocalModeOffLoopback(t *testing.T) {
	// Never tell a remote client that an unauthenticated bypass exists.
	server := NewServer(nil, nil)
	server.localSingleUserMode = true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
	req.RemoteAddr = "203.0.113.5:44444"

	server.GetAuthConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.NotContains(t, body, `"local_mode_enabled":true`)

	resp := decodeJSONBody[AuthConfig](t, rec)
	if resp.LocalModeEnabled != nil {
		assert.False(t, *resp.LocalModeEnabled)
	}
}

func TestListProjects_BootstrapReturnsAllVisibleProjects(t *testing.T) {
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
	// The bootstrap mode is the reachable credential-free caller for this
	// surface since hosted operator login was removed.
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeBootstrap)
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

func TestListProjects_APIKeyContextSeesAllProjects(t *testing.T) {
	// Local-first design: an authenticated caller — including one bound to a single
	// project via API key — can enumerate every project so the management UI works
	// with just a locally created project key.
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	projectAID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	projectBID := testutil.CreateTestProject(t, ctx, platformStore.Queries())

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	reqCtx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectAID)
	rec := httptest.NewRecorder()

	server.ListProjects(rec, req.WithContext(reqCtx))
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[ProjectList](t, rec)
	require.NotNil(t, resp.AuthenticatedProjectId)
	assert.Equal(t, projectAID, *resp.AuthenticatedProjectId)
	ids := make(map[string]struct{}, len(resp.Projects))
	for _, project := range resp.Projects {
		ids[project.Id.String()] = struct{}{}
	}
	assert.Contains(t, ids, projectAID.String())
	assert.Contains(t, ids, projectBID.String())
}
