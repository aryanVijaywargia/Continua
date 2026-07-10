package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestStartEngineRun_RejectsDisabledDefinition(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	deleteDefinitionCatalogEntry(ctx, t, engineQueries, "checkout", "v1")
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	disableDefinitionCatalogEntry(ctx, t, platformStore, "checkout", "v1")
	instanceKey := "disabled-definition-" + uuid.NewString()

	rec := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
		RequestKey:        "req-" + instanceKey,
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "definition_not_registered", decodeJSONBody[Error](t, rec).Code)
	requireNoEngineInstance(ctx, t, engineQueries, projectID, instanceKey)
}

func TestStartEngineRun_RejectsStaleDefinition(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	deleteDefinitionCatalogEntry(ctx, t, engineQueries, "checkout", "v1")
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	backdateDefinitionCatalogEntry(ctx, t, platformStore, "checkout", "v1")
	instanceKey := "stale-definition-" + uuid.NewString()

	rec := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
		RequestKey:        "req-" + instanceKey,
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "definition_not_registered", decodeJSONBody[Error](t, rec).Code)
	requireNoEngineInstance(ctx, t, engineQueries, projectID, instanceKey)
}

func TestStartEngineRun_AcceptsFreshEnabledDefinition(t *testing.T) {
	ctx, _, engineQueries, server, projectID := setupEngineHandlerTest(t)
	deleteDefinitionCatalogEntry(ctx, t, engineQueries, "checkout", "v1")
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))
	instanceKey := "fresh-definition-" + uuid.NewString()

	rec := invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       instanceKey,
		RequestKey:        "req-" + instanceKey,
	})

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestListEngineDefinitions_ReportsLiveness(t *testing.T) {
	ctx := context.Background()
	platformStore := store.New(testutil.TestDB(t))
	engineQueries := enginedb.New(platformStore.Pool())
	server := NewServer(platformStore, nil)
	server.engineControl = newEngineControlService(platformStore)
	server.enginePublicAPIEnabled = true

	apiKey := "engine-definitions-" + uuid.NewString()
	_, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "engine-definitions-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)

	suffix := uuid.NewString()[:8]
	freshName := "fresh-" + suffix
	staleName := "stale-" + suffix
	disabledName := "disabled-" + suffix
	for _, definition := range []struct {
		name    string
		version string
	}{
		{name: freshName, version: "v1"},
		{name: staleName, version: "v1"},
		{name: disabledName, version: "v2"},
	} {
		_, err := engineQueries.UpsertDefinitionCatalogEntry(ctx, enginedb.UpsertDefinitionCatalogEntryParams{
			DefinitionName:    definition.name,
			DefinitionVersion: definition.version,
		})
		require.NoError(t, err)
	}
	backdateDefinitionCatalogEntry(ctx, t, platformStore, staleName, "v1")
	disableDefinitionCatalogEntry(ctx, t, platformStore, disabledName, "v2")

	handler := newAuthenticatedRouter(t, server, platformStore)
	req := httptest.NewRequest(http.MethodGet, "/v1/engine/definitions", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[engineDefinitionListResponse](t, rec)
	byKey := make(map[string]engineDefinitionListEntry, len(resp.Definitions))
	for _, definition := range resp.Definitions {
		byKey[definition.DefinitionName+"@"+definition.DefinitionVersion] = definition
	}

	fresh := requireEngineDefinitionEntry(t, byKey, freshName+"@v1")
	assert.True(t, fresh.Enabled)
	assert.True(t, fresh.Live)
	require.False(t, fresh.RuntimePublishedAt.IsZero())
	require.False(t, fresh.PublishedAt.IsZero())

	stale := requireEngineDefinitionEntry(t, byKey, staleName+"@v1")
	assert.True(t, stale.Enabled)
	assert.False(t, stale.Live)

	disabled := requireEngineDefinitionEntry(t, byKey, disabledName+"@v2")
	assert.False(t, disabled.Enabled)
	assert.False(t, disabled.Live)

	disabledServer := NewServer(platformStore, nil)
	disabledHandler := newAuthenticatedRouter(t, disabledServer, platformStore)
	disabledReq := httptest.NewRequest(http.MethodGet, "/v1/engine/definitions", nil)
	disabledReq.Header.Set("X-API-Key", apiKey)
	disabledRec := httptest.NewRecorder()

	disabledHandler.ServeHTTP(disabledRec, disabledReq)

	require.Equal(t, http.StatusNotFound, disabledRec.Code)
}

func deleteDefinitionCatalogEntry(
	ctx context.Context,
	t *testing.T,
	engineQueries *enginedb.Queries,
	definitionName string,
	definitionVersion string,
) {
	t.Helper()

	_, err := engineQueries.DeleteDefinitionCatalogEntry(ctx, enginedb.DeleteDefinitionCatalogEntryParams{
		DefinitionName:    definitionName,
		DefinitionVersion: definitionVersion,
	})
	require.NoError(t, err)
}

type engineDefinitionListResponse struct {
	Definitions []engineDefinitionListEntry `json:"definitions"`
}

type engineDefinitionListEntry struct {
	DefinitionName     string    `json:"definition_name"`
	DefinitionVersion  string    `json:"definition_version"`
	Enabled            bool      `json:"enabled"`
	Live               bool      `json:"live"`
	RuntimePublishedAt time.Time `json:"runtime_published_at"`
	PublishedAt        time.Time `json:"published_at"`
}

func requireEngineDefinitionEntry(
	t *testing.T,
	entries map[string]engineDefinitionListEntry,
	key string,
) engineDefinitionListEntry {
	t.Helper()

	entry, ok := entries[key]
	require.Truef(t, ok, "expected definition %s in response, got %+v", key, entries)
	return entry
}

func disableDefinitionCatalogEntry(
	ctx context.Context,
	t *testing.T,
	platformStore *store.Store,
	definitionName string,
	definitionVersion string,
) {
	t.Helper()

	tag, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.definition_catalog
		SET enabled = false
		WHERE definition_name = $1
		  AND definition_version = $2
	`, definitionName, definitionVersion)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected())
}

func backdateDefinitionCatalogEntry(
	ctx context.Context,
	t *testing.T,
	platformStore *store.Store,
	definitionName string,
	definitionVersion string,
) {
	t.Helper()

	tag, err := platformStore.Pool().Exec(ctx, `
		UPDATE engine.definition_catalog
		SET runtime_published_at = NOW() - INTERVAL '10 minutes'
		WHERE definition_name = $1
		  AND definition_version = $2
	`, definitionName, definitionVersion)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected())
}

func requireNoEngineInstance(
	ctx context.Context,
	t *testing.T,
	engineQueries *enginedb.Queries,
	projectID uuid.UUID,
	instanceKey string,
) {
	t.Helper()

	_, err := engineQueries.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectID,
		InstanceKey: instanceKey,
	})
	require.True(t, errors.Is(err, pgx.ErrNoRows))
}
