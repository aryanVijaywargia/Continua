package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestGetEngineHealth_DisabledEngineAPIReturns404(t *testing.T) {
	_, _, handler, apiKey, _ := setupEngineHealthTest(t, false)

	rec := invokeEngineHealth(t, handler, apiKey)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetEngineHealth_EmptyStateReturnsZeros(t *testing.T) {
	_, _, handler, apiKey, _ := setupEngineHealthTest(t, true)

	rec := invokeEngineHealth(t, handler, apiKey)

	require.Equal(t, http.StatusOK, rec.Code)
	response := decodeJSONBody[engineHealthTestResponse](t, rec)
	assert.Zero(t, response.Projector.LagRows)
	assert.Zero(t, response.Projector.RunsCatchingUp)
	assert.Zero(t, response.Queues.RunsReady)
	assert.Zero(t, response.Queues.ActivityTasksPending)
	assert.Zero(t, response.Queues.InboxPending)
	assert.NotNil(t, response.Workers)
	assert.Empty(t, response.Workers)
	assert.Zero(t, response.Retention.SummaryOnlyRuns)
	assert.Zero(t, response.Retention.JournalExpiredRuns)
	_, err := time.Parse(time.RFC3339, response.GeneratedAt)
	require.NoError(t, err)
}

func TestGetEngineHealth_ReportsQueueDepthsAndProjectorLag(t *testing.T) {
	ctx, pool, handler, apiKey, projectID := setupEngineHealthTest(t, true)
	instanceID := seedHealthInstance(t, ctx, pool, projectID, "queue-depth")
	past := time.Now().UTC().Add(-time.Minute)
	queuedRunID := seedHealthRun(t, ctx, pool, projectID, instanceID, 1, "queued", past, nil, nil, nil)
	expiredWorker := "expired-run-worker"
	claimedAt := past.Add(-time.Minute)
	seedHealthRun(t, ctx, pool, projectID, instanceID, 2, "running", past, &expiredWorker, &claimedAt, &past)
	seedHealthActivityTask(t, ctx, pool, projectID, instanceID, queuedRunID, "queued-local", "queued", past, nil, nil, nil)
	seedHealthInbox(t, ctx, pool, projectID, instanceID, queuedRunID, past)
	seedHealthTrace(t, ctx, pool, projectID, queuedRunID, "catching_up", 5, 2)

	rec := invokeEngineHealth(t, handler, apiKey)

	require.Equal(t, http.StatusOK, rec.Code)
	response := decodeJSONBody[engineHealthTestResponse](t, rec)
	assert.EqualValues(t, 2, response.Queues.RunsReady)
	assert.EqualValues(t, 1, response.Queues.ActivityTasksPending)
	assert.EqualValues(t, 1, response.Queues.InboxPending)
	assert.EqualValues(t, 3, response.Projector.LagRows)
	assert.EqualValues(t, 1, response.Projector.RunsCatchingUp)
}

func TestGetEngineHealth_WorkerLivenessFromLeases(t *testing.T) {
	ctx, pool, handler, apiKey, projectID := setupEngineHealthTest(t, true)
	instanceID := seedHealthInstance(t, ctx, pool, projectID, "worker-liveness")
	now := time.Now().UTC().Truncate(time.Microsecond)
	activeClaimedAt := now.Add(-2 * time.Minute)
	activeLeaseExpiry := now.Add(5 * time.Minute)
	activeWorker := "worker-a"
	activityRunID := seedHealthRun(t, ctx, pool, projectID, instanceID, 1, "queued", now.Add(time.Hour), nil, nil, nil)
	seedHealthActivityTask(
		t,
		ctx,
		pool,
		projectID,
		instanceID,
		activityRunID,
		"active-lease",
		"claimed",
		now,
		&activeWorker,
		&activeClaimedAt,
		&activeLeaseExpiry,
	)

	staleWorker := "worker-b"
	staleClaimedAt := now.Add(-10 * time.Minute)
	staleLeaseExpiry := now.Add(-5 * time.Minute)
	seedHealthRun(
		t,
		ctx,
		pool,
		projectID,
		instanceID,
		2,
		"running",
		now.Add(-time.Hour),
		&staleWorker,
		&staleClaimedAt,
		&staleLeaseExpiry,
	)

	rec := invokeEngineHealth(t, handler, apiKey)

	require.Equal(t, http.StatusOK, rec.Code)
	response := decodeJSONBody[engineHealthTestResponse](t, rec)
	workers := workersByID(response.Workers)
	require.Contains(t, workers, activeWorker)
	assert.Equal(t, "active", workers[activeWorker].Status)
	assert.GreaterOrEqual(t, workers[activeWorker].ActiveLeases, 1)
	assertWorkerLastClaimAt(t, workers[activeWorker], activeClaimedAt)
	require.Contains(t, workers, staleWorker)
	assert.Equal(t, "stale", workers[staleWorker].Status)
	assert.GreaterOrEqual(t, workers[staleWorker].ExpiredLeases, 1)
	assertWorkerLastClaimAt(t, workers[staleWorker], staleClaimedAt)
}

func TestGetEngineHealth_ScopedToProject(t *testing.T) {
	ctx, pool, handler, apiKey, projectAID := setupEngineHealthTest(t, true)
	platformStore := store.New(pool)
	projectBID := testutil.CreateTestProject(t, ctx, platformStore.Queries())
	now := time.Now().UTC().Truncate(time.Microsecond)

	projectAInstance := seedHealthInstance(t, ctx, pool, projectAID, "scoped-a")
	projectARun := seedHealthRun(t, ctx, pool, projectAID, projectAInstance, 1, "queued", now.Add(-time.Minute), nil, nil, nil)
	projectAWorker := "project-a-worker"
	projectAClaimedAt := now.Add(-2 * time.Minute)
	projectALeaseExpiry := now.Add(5 * time.Minute)
	seedHealthActivityTask(t, ctx, pool, projectAID, projectAInstance, projectARun, "scoped-a", "claimed", now, &projectAWorker, &projectAClaimedAt, &projectALeaseExpiry)

	projectBInstance := seedHealthInstance(t, ctx, pool, projectBID, "scoped-b")
	projectBRun := seedHealthRun(t, ctx, pool, projectBID, projectBInstance, 1, "queued", now.Add(-time.Minute), nil, nil, nil)
	projectBWorker := "project-b-worker"
	projectBClaimedAt := now.Add(-2 * time.Minute)
	projectBLeaseExpiry := now.Add(5 * time.Minute)
	seedHealthActivityTask(t, ctx, pool, projectBID, projectBInstance, projectBRun, "scoped-b", "claimed", now, &projectBWorker, &projectBClaimedAt, &projectBLeaseExpiry)

	rec := invokeEngineHealth(t, handler, apiKey)

	require.Equal(t, http.StatusOK, rec.Code)
	response := decodeJSONBody[engineHealthTestResponse](t, rec)
	assert.EqualValues(t, 1, response.Queues.RunsReady)
	workerIDs := make([]string, 0, len(response.Workers))
	for _, worker := range response.Workers {
		workerIDs = append(workerIDs, worker.ID)
	}
	assert.ElementsMatch(t, []string{projectAWorker}, workerIDs)
	assert.NotContains(t, workerIDs, projectBWorker)
}

func TestGetEngineHealth_RetentionCounts(t *testing.T) {
	ctx, pool, handler, apiKey, projectID := setupEngineHealthTest(t, true)
	seedHealthTrace(t, ctx, pool, projectID, uuid.New(), "summary_only", 11, 2)
	seedHealthTrace(t, ctx, pool, projectID, uuid.New(), "journal_expired", 21, 3)
	seedHealthTrace(t, ctx, pool, projectID, uuid.New(), "journal_expired", 31, 4)

	rec := invokeEngineHealth(t, handler, apiKey)

	require.Equal(t, http.StatusOK, rec.Code)
	response := decodeJSONBody[engineHealthTestResponse](t, rec)
	assert.EqualValues(t, 1, response.Retention.SummaryOnlyRuns)
	assert.EqualValues(t, 2, response.Retention.JournalExpiredRuns)
	assert.Zero(t, response.Projector.LagRows)
}

type engineHealthTestResponse struct {
	GeneratedAt string `json:"generated_at"`
	Projector   struct {
		LagRows        int64 `json:"lag_rows"`
		RunsCatchingUp int64 `json:"runs_catching_up"`
	} `json:"projector"`
	Queues struct {
		RunsReady            int64 `json:"runs_ready"`
		ActivityTasksPending int64 `json:"activity_tasks_pending"`
		InboxPending         int64 `json:"inbox_pending"`
	} `json:"queues"`
	Workers   []engineWorkerHealthTestResponse `json:"workers"`
	Retention struct {
		SummaryOnlyRuns    int64 `json:"summary_only_runs"`
		JournalExpiredRuns int64 `json:"journal_expired_runs"`
	} `json:"retention"`
}

type engineWorkerHealthTestResponse struct {
	ID            string `json:"id"`
	LastClaimAt   string `json:"last_claim_at"`
	ActiveLeases  int    `json:"active_leases"`
	ExpiredLeases int    `json:"expired_leases"`
	Status        string `json:"status"`
}

func setupEngineHealthTest(t *testing.T, enabled bool) (context.Context, *pgxpool.Pool, http.Handler, string, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	pool := testutil.TestDB(t)
	platformStore := store.New(pool)
	server := NewServer(platformStore, nil)
	server.enginePublicAPIEnabled = enabled
	apiKey := "engine-health-" + uuid.NewString()
	project, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "engine-health-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)

	return ctx, pool, newAuthenticatedRouter(t, server, platformStore), apiKey, project.ID
}

func invokeEngineHealth(t *testing.T, handler http.Handler, apiKey string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/health", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func seedHealthInstance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, key string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO engine.instances (id, project_id, instance_key, definition_name)
		VALUES ($1, $2, $3, 'health-test')
	`, id, projectID, key+"-"+uuid.NewString())
	require.NoError(t, err)
	return id
}

func seedHealthRun(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID uuid.UUID,
	instanceID uuid.UUID,
	runNumber int,
	status string,
	readyAt time.Time,
	claimedBy *string,
	claimedAt *time.Time,
	leaseExpiresAt *time.Time,
) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO engine.runs (
			id, project_id, instance_id, run_number, definition_version, status,
			ready_at, claimed_by, claimed_at, lease_expires_at, root_run_id, child_depth
		)
		VALUES ($1, $2, $3, $4, 'v1', $5, $6, $7, $8, $9, $1, 0)
	`, id, projectID, instanceID, runNumber, status, readyAt, claimedBy, claimedAt, leaseExpiresAt)
	require.NoError(t, err)
	return id
}

func seedHealthActivityTask(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID uuid.UUID,
	instanceID uuid.UUID,
	runID uuid.UUID,
	activityKey string,
	status string,
	availableAt time.Time,
	claimedBy *string,
	claimedAt *time.Time,
	leaseExpiresAt *time.Time,
) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO engine.activity_tasks (
			project_id, instance_id, run_id, activity_key, activity_type,
			execution_target, status, available_at, claimed_by, claimed_at, lease_expires_at
		)
		VALUES ($1, $2, $3, $4, 'health-test', 'local', $5, $6, $7, $8, $9)
	`, projectID, instanceID, runID, activityKey, status, availableAt, claimedBy, claimedAt, leaseExpiresAt)
	require.NoError(t, err)
}

func seedHealthInbox(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID uuid.UUID,
	instanceID uuid.UUID,
	runID uuid.UUID,
	availableAt time.Time,
) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO engine.inbox (project_id, instance_id, run_id, kind, status, available_at)
		VALUES ($1, $2, $3, 'signal', 'pending', $4)
	`, projectID, instanceID, runID, availableAt)
	require.NoError(t, err)
}

func seedHealthTrace(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID uuid.UUID,
	runID uuid.UUID,
	projectionState string,
	latestHistoryID int64,
	lastProjectedHistoryID int64,
) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO public.traces (
			project_id, trace_id, name, engine_run_id, engine_projection_state,
			engine_latest_history_id, engine_last_projected_history_id
		)
		VALUES ($1, $2, 'engine health test', $3, $4, $5, $6)
	`, projectID, "health-trace-"+uuid.NewString(), runID, projectionState, latestHistoryID, lastProjectedHistoryID)
	require.NoError(t, err)
}

func workersByID(workers []engineWorkerHealthTestResponse) map[string]engineWorkerHealthTestResponse {
	byID := make(map[string]engineWorkerHealthTestResponse, len(workers))
	for _, worker := range workers {
		byID[worker.ID] = worker
	}
	return byID
}

func assertWorkerLastClaimAt(t *testing.T, worker engineWorkerHealthTestResponse, expected time.Time) {
	t.Helper()

	actual, err := time.Parse(time.RFC3339, worker.LastClaimAt)
	require.NoError(t, err)
	assert.True(t, expected.Equal(actual), "last_claim_at: expected %s, got %s", expected, actual)
}
