package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// Local single-user mode is read-only plus the project bootstrap. Any process on
// the machine can reach the loopback surface, so project mutations must still
// demand an API key even though reads do not.

type localModeMutationFixture struct {
	router  http.Handler
	store   *store.Store
	project platform.Project
	apiKey  string
}

func newLocalModeMutationFixture(t *testing.T) localModeMutationFixture {
	t.Helper()

	pool := testutil.TestDB(t)
	platformStore := store.New(pool)

	apiKey := "local-mode-mutation-" + uuid.NewString()
	project, err := platformStore.Queries().CreateProject(context.Background(), platform.CreateProjectParams{
		Name:       "Local mode mutation " + uuid.NewString(),
		ApiKeyHash: middleware.HashAPIKey(apiKey),
	})
	require.NoError(t, err)

	authenticator, err := middleware.NewAuthenticator(platformStore, &config.Config{
		LocalSingleUserMode: true,
	})
	require.NoError(t, err)

	return localModeMutationFixture{
		router:  NewRouter(NewServer(platformStore, nil), authenticator),
		store:   platformStore,
		project: project,
		apiKey:  apiKey,
	}
}

// serveFromLoopback issues a request from a genuine loopback peer, optionally
// carrying an API key.
func (f localModeMutationFixture) serveFromLoopback(
	t *testing.T,
	method, target, body, apiKey string,
) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestLocalModeRejectsProjectMutationWithoutAPIKey(t *testing.T) {
	fixture := newLocalModeMutationFixture(t)
	projectPath := "/api/projects/" + fixture.project.ID.String()

	mutations := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"rename", http.MethodPatch, projectPath, `{"name":"Renamed by an unauthenticated caller"}`},
		{"rotate key", http.MethodPost, projectPath + "/rotate", ""},
		{"delete", http.MethodDelete, projectPath, ""},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			rec := fixture.serveFromLoopback(t, mutation.method, mutation.target, mutation.body, "")

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			resp := decodeJSONBody[Error](t, rec)
			assert.Equal(t, "missing_credentials", resp.Code)
		})
	}

	// Status codes alone would not prove the writes were refused, so confirm the
	// project survived intact.
	surviving, err := fixture.store.GetProject(context.Background(), fixture.project.ID)
	require.NoError(t, err, "the project must not have been deleted")
	assert.Equal(t, fixture.project.Name, surviving.Name, "the project must not have been renamed")
	assert.Equal(t, fixture.project.ApiKeyHash, surviving.ApiKeyHash, "the API key must not have been rotated")
}

func TestLocalModeStillAllowsProjectBootstrap(t *testing.T) {
	fixture := newLocalModeMutationFixture(t)

	listed := fixture.serveFromLoopback(t, http.MethodGet, "/api/projects", "", "")
	require.Equal(t, http.StatusOK, listed.Code)
	listResp := decodeJSONBody[ProjectList](t, listed)
	assert.NotEmpty(t, listResp.Projects, "first-run setup must still be able to list projects")

	created := fixture.serveFromLoopback(
		t,
		http.MethodPost,
		"/api/projects",
		`{"name":"Bootstrapped in local mode"}`,
		"",
	)
	require.Equal(t, http.StatusCreated, created.Code)
	createResp := decodeJSONBody[ProjectWithKey](t, created)
	assert.Equal(t, "Bootstrapped in local mode", createResp.Name)
	assert.NotEmpty(t, createResp.ApiKey)
}

func TestProjectMutationStillWorksWithAPIKey(t *testing.T) {
	fixture := newLocalModeMutationFixture(t)
	projectPath := "/api/projects/" + fixture.project.ID.String()

	rec := fixture.serveFromLoopback(
		t,
		http.MethodPatch,
		projectPath,
		`{"name":"Renamed with a valid key"}`,
		fixture.apiKey,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[Project](t, rec)
	assert.Equal(t, "Renamed with a valid key", resp.Name)

	stored, err := fixture.store.GetProject(context.Background(), fixture.project.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed with a valid key", stored.Name)
}

func TestLocalModeRejectsProjectDeleteFromSpoofedRemotePeer(t *testing.T) {
	fixture := newLocalModeMutationFixture(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+fixture.project.ID.String(), nil)
	req.RemoteAddr = remoteNonLoopbackAddr
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	rec := httptest.NewRecorder()
	fixture.router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	_, err := fixture.store.GetProject(context.Background(), fixture.project.ID)
	require.NoError(t, err, "a remote caller must not be able to delete a project")
}
