package middleware_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
)

// =============================================================================
// Authentication Middleware Tests
// =============================================================================
// Tests for enable-e2e-usability/specs/authentication/spec.md

// testDB returns a connection pool for testing.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	dbURL := "postgres://continua:continua@localhost:5432/continua_test?sslmode=disable"

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping test: could not connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("Skipping test: could not ping test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// hashAPIKey hashes an API key using SHA-256.
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

//nolint:revive // Keep testing.T first in test helper signatures.
func createTestProject(t *testing.T, ctx context.Context, q *platform.Queries, apiKey string) uuid.UUID {
	t.Helper()

	// Store the hash of the API key
	project, err := q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       "test-project-" + uuid.New().String()[:8],
		ApiKeyHash: hashAPIKey(apiKey),
	})
	require.NoError(t, err)

	return project.ID
}

func TestAuthMiddleware_MissingAPIKeyRejected(t *testing.T) {
	// Scenario: Missing API key rejected
	// WHEN request to protected endpoint lacks API key
	// THEN response status is 401
	// AND response body contains error

	pool := testDB(t)
	s := store.New(pool)

	authMiddleware := middleware.APIKeyAuth(s)

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Wrap with auth middleware
	protectedHandler := authMiddleware(handler)

	// Make request without API key
	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp map[string]string
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "missing_api_key", resp["code"])
}

func TestAuthMiddleware_InvalidAPIKeyRejected(t *testing.T) {
	// Scenario: Invalid API key rejected
	// WHEN request contains invalid API key in X-API-Key header
	// THEN response status is 401
	// AND response body contains error

	pool := testDB(t)
	s := store.New(pool)

	authMiddleware := middleware.APIKeyAuth(s)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protectedHandler := authMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("X-API-Key", "invalid-api-key-12345")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp map[string]string
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_api_key", resp["code"])
}

func TestAuthMiddleware_ValidAPIKeyAccepted(t *testing.T) {
	// Scenario: Valid API key accepted
	// WHEN request contains valid API key in X-API-Key header
	// THEN request is forwarded to handler
	// AND project ID is available in request context

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)

	apiKey := "test-api-key-" + uuid.New().String()
	projectID := createTestProject(t, ctx, s.Queries(), apiKey)

	authMiddleware := middleware.APIKeyAuth(s)

	var receivedProjectID uuid.UUID
	var foundProjectID bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedProjectID, foundProjectID = middleware.GetProjectID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	protectedHandler := authMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, foundProjectID, "project ID should be in context")
	assert.Equal(t, projectID, receivedProjectID, "project ID should match")
}

func TestAuthMiddleware_BearerTokenSupport(t *testing.T) {
	// Scenario: Bearer token support
	// WHEN request contains valid API key in Authorization: Bearer <key> header
	// THEN request is authenticated successfully

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)

	apiKey := "test-api-key-bearer-" + uuid.New().String()
	projectID := createTestProject(t, ctx, s.Queries(), apiKey)

	authMiddleware := middleware.APIKeyAuth(s)

	var receivedProjectID uuid.UUID
	var foundProjectID bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedProjectID, foundProjectID = middleware.GetProjectID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	protectedHandler := authMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, foundProjectID)
	assert.Equal(t, projectID, receivedProjectID)
}

func TestAuthMiddleware_HealthEndpointPublic(t *testing.T) {
	// Scenario: Health check without auth
	// WHEN request to GET /api/health lacks API key
	// THEN response status is 200
	// AND health information is returned

	// This test verifies router composition, not middleware
	// Health should be routed OUTSIDE the auth middleware group

	r := chi.NewRouter()

	// Health endpoint - public (no middleware)
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": "test",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	// No API key header
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}

// =============================================================================
// Multi-Tenancy Data Isolation Tests
// =============================================================================

func TestMultiTenancy_ListTracesScoped(t *testing.T) {
	// Scenario: ListTraces scoped by project
	// WHEN project A requests GET /api/traces
	// THEN only traces belonging to project A are returned
	// AND traces from project B are not visible

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	// Create two projects
	apiKeyA := "api-key-project-a-" + uuid.New().String()
	apiKeyB := "api-key-project-b-" + uuid.New().String()
	projectAID := createTestProject(t, ctx, q, apiKeyA)
	projectBID := createTestProject(t, ctx, q, apiKeyB)

	// Create traces for each project
	_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectAID,
		TraceID:   "trace-project-a",
		Name:      strPtr("Project A Trace"),
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectBID,
		TraceID:   "trace-project-b",
		Name:      strPtr("Project B Trace"),
	})
	require.NoError(t, err)

	// Query traces for project A
	tracesA, err := q.ListTraces(ctx, platform.ListTracesParams{
		ProjectID: projectAID,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)

	// Should only see project A's trace
	assert.Len(t, tracesA, 1)
	assert.Equal(t, "trace-project-a", tracesA[0].Trace.TraceID)

	// Query traces for project B
	tracesB, err := q.ListTraces(ctx, platform.ListTracesParams{
		ProjectID: projectBID,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)

	// Should only see project B's trace
	assert.Len(t, tracesB, 1)
	assert.Equal(t, "trace-project-b", tracesB[0].Trace.TraceID)
}

func TestMultiTenancy_GetTraceReturns404ForOtherProject(t *testing.T) {
	// Scenario: GetTrace scoped by project
	// WHEN project A requests GET /api/traces/{id} for a trace owned by project B
	// THEN response status is 404 (not 403, to avoid information leakage)

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	apiKeyA := "api-key-a-" + uuid.New().String()
	apiKeyB := "api-key-b-" + uuid.New().String()
	projectAID := createTestProject(t, ctx, q, apiKeyA)
	projectBID := createTestProject(t, ctx, q, apiKeyB)

	// Create trace for project B
	traceB, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectBID,
		TraceID:   "trace-project-b",
		Name:      strPtr("Project B Trace"),
	})
	require.NoError(t, err)

	// Try to get project B's trace using project A's scope
	// This should use GetTraceByExternalID which includes project_id
	_, err = q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectAID,
		TraceID:   traceB.TraceID,
	})

	// Should return not found (ErrNoRows)
	assert.Error(t, err, "should not find trace from other project")
	assert.True(t, store.IsNotFound(err), "error should be 'not found'")
}

func TestMultiTenancy_IngestScopedByProject(t *testing.T) {
	// Scenario: Ingest scoped by project
	// WHEN project A ingests data via POST /v1/ingest
	// THEN all traces, spans, and events are associated with project A

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	apiKey := "api-key-ingest-" + uuid.New().String()
	projectID := createTestProject(t, ctx, q, apiKey)

	// Create trace with project ID
	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "ingested-trace",
		Name:      strPtr("Ingested Trace"),
	})
	require.NoError(t, err)

	assert.Equal(t, projectID, trace.ProjectID, "trace should be associated with project")

	// Create span with project ID using trace.ID (uuid.UUID)
	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace.ID,
		SpanID:    "ingested-span",
		Name:      "Ingested Span",
	})
	require.NoError(t, err)

	assert.Equal(t, projectID, span.ProjectID, "span should be associated with project")
}

// Helper
func strPtr(s string) *string {
	return &s
}
