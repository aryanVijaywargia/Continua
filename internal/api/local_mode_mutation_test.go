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

// Local single-user mode is fully credential-free on loopback: reads, the
// project bootstrap and project management all work without an API key. That is
// a deliberate decision for single-user localhost use, and it widens the local
// surface — so the property that keeps it confined, loopback-only admission that
// ignores proxy headers, is the one these tests guard hardest.

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

// TestLocalModeAllowsProjectMutationWithoutAPIKey pins the maintainer's ruling:
// a credential-free loopback caller can rename, rotate and delete. Status codes
// alone would not prove the writes landed, so each step is confirmed against the
// store.
func TestLocalModeAllowsProjectMutationWithoutAPIKey(t *testing.T) {
	fixture := newLocalModeMutationFixture(t)
	projectPath := "/api/projects/" + fixture.project.ID.String()
	ctx := context.Background()

	renamed := fixture.serveFromLoopback(
		t,
		http.MethodPatch,
		projectPath,
		`{"name":"Renamed without a key"}`,
		"",
	)
	require.Equal(t, http.StatusOK, renamed.Code)
	assert.Equal(t, "Renamed without a key", decodeJSONBody[Project](t, renamed).Name)

	stored, err := fixture.store.GetProject(ctx, fixture.project.ID)
	require.NoError(t, err)
	require.Equal(t, "Renamed without a key", stored.Name, "the rename must have reached the database")

	rotated := fixture.serveFromLoopback(t, http.MethodPost, projectPath+"/rotate", "", "")
	require.Equal(t, http.StatusOK, rotated.Code)
	rotateResp := decodeJSONBody[ProjectWithKey](t, rotated)
	assert.NotEmpty(t, rotateResp.ApiKey, "rotation must return the new plaintext key")

	stored, err = fixture.store.GetProject(ctx, fixture.project.ID)
	require.NoError(t, err)
	require.NotEqual(
		t,
		fixture.project.ApiKeyHash,
		stored.ApiKeyHash,
		"the stored key hash must have changed",
	)
	assert.Equal(
		t,
		middleware.HashAPIKey(rotateResp.ApiKey),
		stored.ApiKeyHash,
		"the stored hash must match the returned key",
	)

	deleted := fixture.serveFromLoopback(t, http.MethodDelete, projectPath, "", "")
	require.Equal(t, http.StatusNoContent, deleted.Code)

	_, err = fixture.store.GetProject(ctx, fixture.project.ID)
	require.Error(t, err, "the project row must be gone")
	assert.True(t, store.IsNotFound(err), "expected a not-found error, got %v", err)
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

// TestLocalModeRejectsProjectMutationFromRemotePeer is the load-bearing negative
// case now that local mode can rename, rotate and delete without a key: the only
// thing standing between a project and a remote caller is the loopback check.
// These run through the production router so chi's RealIP really sits in the
// stack, which is where the spoofing hazard lives — every header RealIP consults
// is replayed against every mutation, and the project is checked afterwards to
// prove nothing changed.
func TestLocalModeRejectsProjectMutationFromRemotePeer(t *testing.T) {
	fixture := newLocalModeMutationFixture(t)
	projectPath := "/api/projects/" + fixture.project.ID.String()

	mutations := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"rename", http.MethodPatch, projectPath, `{"name":"Renamed by a remote caller"}`},
		{"rotate key", http.MethodPost, projectPath + "/rotate", ""},
		{"delete", http.MethodDelete, projectPath, ""},
	}

	// nil headers is the plain remote caller; the rest are the spoofing attempts.
	peers := map[string]map[string]string{"no headers": nil}
	for header, value := range spoofedLoopbackHeaders() {
		peers["spoofed "+header] = map[string]string{header: value}
	}

	for _, mutation := range mutations {
		for peer, headers := range peers {
			t.Run(mutation.name+"/"+peer, func(t *testing.T) {
				req := httptest.NewRequest(mutation.method, mutation.target, strings.NewReader(mutation.body))
				req.RemoteAddr = remoteNonLoopbackAddr
				req.Header.Set("Content-Type", "application/json")
				for name, value := range headers {
					req.Header.Set(name, value)
				}

				rec := httptest.NewRecorder()
				fixture.router.ServeHTTP(rec, req)

				require.Equal(
					t,
					http.StatusUnauthorized,
					rec.Code,
					"a remote peer must not reach %s %s", mutation.method, mutation.target,
				)
				resp := decodeJSONBody[Error](t, rec)
				assert.Equal(t, "missing_credentials", resp.Code)
			})
		}
	}

	// Status codes alone would not prove the writes were refused.
	surviving, err := fixture.store.GetProject(context.Background(), fixture.project.ID)
	require.NoError(t, err, "a remote caller must not be able to delete a project")
	assert.Equal(t, fixture.project.Name, surviving.Name, "the project must not have been renamed")
	assert.Equal(t, fixture.project.ApiKeyHash, surviving.ApiKeyHash, "the API key must not have been rotated")
}
