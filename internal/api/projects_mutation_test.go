package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestCreateProject_ReturnsKeyOnceAndPersistsHash(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	body, err := json.Marshal(CreateProjectRequest{Name: "rotor-bot"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(body))
	server.CreateProject(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
	resp := decodeJSONBody[ProjectWithKey](t, rec)
	assert.Equal(t, "rotor-bot", resp.Name)
	require.True(t, strings.HasPrefix(resp.ApiKey, middleware.APIKeyPrefix), "expected pk_ prefix, got %q", resp.ApiKey)

	// Persisted by hash, not plaintext.
	stored, err := platformStore.GetProjectByAPIKey(ctx, middleware.HashAPIKey(resp.ApiKey))
	require.NoError(t, err)
	assert.Equal(t, resp.Id, stored.ID)
	assert.NotEqual(t, resp.ApiKey, stored.ApiKeyHash, "plaintext key must not be stored")
}

func TestCreateProject_RejectsEmptyName(t *testing.T) {
	pool := testutil.TestDB(t)
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	body, err := json.Marshal(CreateProjectRequest{Name: "   "})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(body))
	server.CreateProject(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdateProject_RenamesExisting(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	projectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())

	body, err := json.Marshal(UpdateProjectRequest{Name: "renamed"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/projects/"+projectID.String(), bytes.NewReader(body))
	server.UpdateProject(rec, req, projectID)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	resp := decodeJSONBody[Project](t, rec)
	assert.Equal(t, "renamed", resp.Name)
}

func TestUpdateProject_ReturnsNotFoundForUnknownID(t *testing.T) {
	pool := testutil.TestDB(t)
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	body, err := json.Marshal(UpdateProjectRequest{Name: "x"})
	require.NoError(t, err)

	missing := uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/projects/"+missing.String(), bytes.NewReader(body))
	server.UpdateProject(rec, req, missing)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRotateProjectAPIKey_InvalidatesOldKey(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	// Seed a project with a known starting hash.
	createBody, err := json.Marshal(CreateProjectRequest{Name: "rotate-me"})
	require.NoError(t, err)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(createBody))
	server.CreateProject(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeJSONBody[ProjectWithKey](t, createRec)
	originalKey := created.ApiKey

	// Rotate.
	rotateRec := httptest.NewRecorder()
	rotateReq := httptest.NewRequest(http.MethodPost, "/api/projects/"+created.Id.String()+"/rotate", nil)
	server.RotateProjectAPIKey(rotateRec, rotateReq, created.Id)
	require.Equal(t, http.StatusOK, rotateRec.Code, "body: %s", rotateRec.Body.String())

	rotated := decodeJSONBody[ProjectWithKey](t, rotateRec)
	assert.NotEqual(t, originalKey, rotated.ApiKey, "rotate must produce a new key")

	// Old key no longer resolves to the project.
	_, err = platformStore.GetProjectByAPIKey(ctx, middleware.HashAPIKey(originalKey))
	assert.ErrorIs(t, err, store.ErrNotFound)

	// New key resolves.
	resolved, err := platformStore.GetProjectByAPIKey(ctx, middleware.HashAPIKey(rotated.ApiKey))
	require.NoError(t, err)
	assert.Equal(t, created.Id, resolved.ID)
}

func TestRotateProjectAPIKey_ReturnsNotFoundForUnknownID(t *testing.T) {
	pool := testutil.TestDB(t)
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	missing := uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+missing.String()+"/rotate", nil)
	server.RotateProjectAPIKey(rec, req, missing)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteProject_RemovesRow(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	projectID := testutil.CreateTestProject(t, ctx, platformStore.Queries())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String(), nil)
	server.DeleteProject(rec, req, projectID)

	require.Equal(t, http.StatusNoContent, rec.Code)
	_, err := platformStore.GetProject(ctx, projectID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestDeleteProject_ReturnsNotFoundForUnknownID(t *testing.T) {
	pool := testutil.TestDB(t)
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)

	missing := uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+missing.String(), nil)
	server.DeleteProject(rec, req, missing)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCreateProject_BootstrapContextAllowedWhenProjectsExist(t *testing.T) {
	// In Auth0-disabled local mode the bootstrap window stays open even after the
	// first project exists: the operator owns the box and must always be able to
	// mint a new project key (e.g. after losing the original).
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)
	testutil.CreateTestProject(t, ctx, platformStore.Queries())

	body, err := json.Marshal(CreateProjectRequest{Name: "another-project"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeBootstrap))
	server.CreateProject(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestGenerateAPIKey_ProducesUniquePrefixedKeys(t *testing.T) {
	keys := make(map[string]struct{}, 50)
	for i := 0; i < 50; i++ {
		key, err := middleware.GenerateAPIKey()
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(key, middleware.APIKeyPrefix))
		_, dup := keys[key]
		require.False(t, dup, "duplicate key generated")
		keys[key] = struct{}{}
	}
}
