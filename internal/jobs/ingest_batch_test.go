package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func newIngestBatchTestDeps(t *testing.T) (*store.Store, *platform.Queries, *ingest.Processor, *IngestBatchWorker) {
	return newIngestBatchTestDepsWithConfig(t, nil)
}

func newIngestBatchTestDepsWithConfig(
	t *testing.T,
	cfg *config.Config,
) (*store.Store, *platform.Queries, *ingest.Processor, *IngestBatchWorker) {
	t.Helper()

	pool := testutil.TestDB(t)
	s := store.New(pool)
	processor := ingest.NewProcessor(s, cfg)
	client, err := NewClient(pool, s, processor, enginecontrol.NewService(s), cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Stop(context.Background())
	})

	return s, s.Queries(), processor, &IngestBatchWorker{
		store:     s,
		processor: processor,
		client:    client,
	}
}

func acceptAsyncBatchForWorker(
	t *testing.T,
	s *store.Store,
	worker *IngestBatchWorker,
	projectID uuid.UUID,
	req *ingest.IngestRequest,
) uuid.UUID {
	t.Helper()

	rawPayload, err := json.Marshal(req)
	require.NoError(t, err)

	svc := ingest.NewService(s, worker.client, worker.processor, nil)
	resp, err := svc.AcceptAsync(
		context.Background(),
		projectID,
		req,
		rawPayload,
	)
	require.NoError(t, err)
	return resp.BatchID
}

func TestIngestBatchWorker_WorkCompletesQueuedBatch(t *testing.T) {
	s, q, _, worker := newIngestBatchTestDeps(t)
	projectID := testutil.CreateTestProject(t, context.Background(), q)
	batchID := acceptAsyncBatchForWorker(t, s, worker, projectID, &ingest.IngestRequest{
		BatchKey: "worker-complete-" + uuid.NewString()[:8],
	})

	err := worker.Work(context.Background(), &river.Job[jobargs.IngestBatchArgs]{
		Args: jobargs.IngestBatchArgs{BatchID: batchID},
	})
	require.NoError(t, err)

	batch, err := s.GetBatch(context.Background(), batchID)
	require.NoError(t, err)
	assert.Equal(t, "completed", batch.Status)
	assert.Equal(t, int32(1), batch.AttemptCount)
	require.True(t, batch.ProcessingStartedAt.Valid)
	require.True(t, batch.ProcessingCompletedAt.Valid)

	_, err = s.GetBatchPayload(context.Background(), batchID)
	assert.True(t, store.IsNotFound(err))
}

func TestIngestBatchWorker_ReentryAfterCompletionIsNoOp(t *testing.T) {
	s, q, _, worker := newIngestBatchTestDeps(t)
	projectID := testutil.CreateTestProject(t, context.Background(), q)
	batchID := acceptAsyncBatchForWorker(t, s, worker, projectID, &ingest.IngestRequest{
		BatchKey: "worker-idempotent-" + uuid.NewString()[:8],
	})

	job := &river.Job[jobargs.IngestBatchArgs]{Args: jobargs.IngestBatchArgs{BatchID: batchID}}
	require.NoError(t, worker.Work(context.Background(), job))

	completed, err := s.GetBatch(context.Background(), batchID)
	require.NoError(t, err)

	require.NoError(t, worker.Work(context.Background(), job))

	afterRetry, err := s.GetBatch(context.Background(), batchID)
	require.NoError(t, err)
	assert.Equal(t, completed.Status, afterRetry.Status)
	assert.Equal(t, completed.AttemptCount, afterRetry.AttemptCount)
	assert.Equal(t, completed.ProcessingCompletedAt.Time, afterRetry.ProcessingCompletedAt.Time)
}

func TestIngestBatchWorker_RetriesDependencyUntilTraceBatchCommits(t *testing.T) {
	s, q, _, worker := newIngestBatchTestDeps(t)
	ctx := context.Background()
	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "dep-trace-" + uuid.NewString()[:8]

	dependentBatchID := acceptAsyncBatchForWorker(t, s, worker, projectID, &ingest.IngestRequest{
		BatchKey: "worker-dependent-" + uuid.NewString()[:8],
		Spans: []ingest.SpanInput{
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				Name:      "child span",
				StartTime: time.Now().UTC(),
			},
		},
	})

	dependentJob := &river.Job[jobargs.IngestBatchArgs]{
		Args: jobargs.IngestBatchArgs{BatchID: dependentBatchID},
	}
	err := worker.Work(ctx, dependentJob)
	require.Error(t, err)

	retryingBatch, err := s.GetBatch(ctx, dependentBatchID)
	require.NoError(t, err)
	assert.Equal(t, "queued", retryingBatch.Status)
	assert.Equal(t, int32(1), retryingBatch.AttemptCount)
	require.NotNil(t, retryingBatch.LastErrorCode)
	assert.Equal(t, "dependency_not_ready", *retryingBatch.LastErrorCode)
	require.NotNil(t, retryingBatch.LastErrorMessage)
	assert.Contains(t, *retryingBatch.LastErrorMessage, traceID)
	assert.True(t, retryingBatch.LastErrorAt.Valid)

	traceBatchID := acceptAsyncBatchForWorker(t, s, worker, projectID, &ingest.IngestRequest{
		BatchKey: "worker-trace-" + uuid.NewString()[:8],
		Traces: []ingest.TraceInput{
			{
				TraceID: traceID,
				Name:    testutil.StrPtr("upstream trace"),
			},
		},
	})
	require.NoError(t, worker.Work(ctx, &river.Job[jobargs.IngestBatchArgs]{
		Args: jobargs.IngestBatchArgs{BatchID: traceBatchID},
	}))

	require.NoError(t, worker.Work(ctx, dependentJob))

	completedBatch, err := s.GetBatch(ctx, dependentBatchID)
	require.NoError(t, err)
	assert.Equal(t, "completed", completedBatch.Status)
	assert.Equal(t, int32(2), completedBatch.AttemptCount)
	assert.Nil(t, completedBatch.LastErrorCode)
	assert.Nil(t, completedBatch.LastErrorMessage)
	assert.False(t, completedBatch.LastErrorAt.Valid)

	_, err = s.GetBatchPayload(ctx, dependentBatchID)
	assert.True(t, store.IsNotFound(err))
}

func TestIngestBatchWorker_DependencyRetryWindowExpires(t *testing.T) {
	cfg := &config.Config{
		Ingest: config.IngestConfig{
			DependencyRetryWindow: time.Minute,
		},
		Jobs: config.JobsConfig{
			IngestWorkers:      1,
			RollupWorkers:      1,
			MaintenanceWorkers: 1,
			DefaultWorkers:     1,
		},
	}
	s, q, _, worker := newIngestBatchTestDepsWithConfig(t, cfg)
	ctx := context.Background()
	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "missing-trace-" + uuid.NewString()[:8]

	batchID := acceptAsyncBatchForWorker(t, s, worker, projectID, &ingest.IngestRequest{
		BatchKey: "worker-timeout-" + uuid.NewString()[:8],
		Spans: []ingest.SpanInput{
			{
				TraceID:   traceID,
				SpanID:    "span-timeout",
				Name:      "timed out span",
				StartTime: time.Now().UTC(),
			},
		},
	})

	_, err := s.Pool().Exec(
		ctx,
		`UPDATE ingest_batches SET server_received_at = $2 WHERE id = $1`,
		batchID,
		time.Now().Add(-2*time.Minute),
	)
	require.NoError(t, err)

	err = worker.Work(ctx, &river.Job[jobargs.IngestBatchArgs]{
		Args: jobargs.IngestBatchArgs{BatchID: batchID},
	})
	require.NoError(t, err)

	failedBatch, err := s.GetBatch(ctx, batchID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failedBatch.Status)
	require.NotNil(t, failedBatch.LastErrorCode)
	assert.Equal(t, "reference_timeout", *failedBatch.LastErrorCode)
	require.NotNil(t, failedBatch.LastErrorMessage)
	assert.Contains(t, *failedBatch.LastErrorMessage, traceID)
	assert.True(t, failedBatch.ProcessingCompletedAt.Valid)

	_, err = s.GetBatchPayload(ctx, batchID)
	require.NoError(t, err)

	_, err = q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestCleanupWorker_DeletesExpiredFailedPayloadsOnly(t *testing.T) {
	s, q, _, _ := newIngestBatchTestDeps(t)
	ctx := context.Background()
	projectID := testutil.CreateTestProject(t, ctx, q)

	expiredBatchID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  "cleanup-expired-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)
	err = s.InsertBatchPayload(ctx, &platform.InsertBatchPayloadParams{
		BatchID:      expiredBatchID,
		PayloadBytes: []byte("expired"),
		Compression:  "gzip",
		ContentType:  "application/json",
		ByteSize:     int32(len("expired")),
	})
	require.NoError(t, err)
	err = s.MarkBatchFailed(ctx, platform.MarkBatchFailedParams{
		ID:               expiredBatchID,
		LastErrorCode:    testutil.StrPtr("failed"),
		LastErrorMessage: testutil.StrPtr("failed"),
	})
	require.NoError(t, err)

	activeBatchID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  "cleanup-active-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)
	err = s.InsertBatchPayload(ctx, &platform.InsertBatchPayloadParams{
		BatchID:      activeBatchID,
		PayloadBytes: []byte("active"),
		Compression:  "gzip",
		ContentType:  "application/json",
		ByteSize:     int32(len("active")),
	})
	require.NoError(t, err)

	_, err = s.Pool().Exec(ctx,
		`UPDATE ingest_batches SET processing_completed_at = $2 WHERE id = $1`,
		expiredBatchID,
		time.Now().Add(-8*24*time.Hour),
	)
	require.NoError(t, err)

	worker := &CleanupWorker{
		store:            s,
		payloadRetention: 7 * 24 * time.Hour,
	}
	require.NoError(t, worker.Work(ctx, &river.Job[jobargs.CleanupArgs]{}))

	_, err = s.GetBatchPayload(ctx, expiredBatchID)
	assert.True(t, store.IsNotFound(err))

	_, err = s.GetBatchPayload(ctx, activeBatchID)
	require.NoError(t, err)

	_, err = s.GetBatch(ctx, expiredBatchID)
	require.NoError(t, err)
}

func TestLegacyDefaultQueueRollupJobExecutesDuringTransition(t *testing.T) {
	cfg := &config.Config{
		Jobs: config.JobsConfig{
			IngestWorkers:      1,
			RollupWorkers:      1,
			MaintenanceWorkers: 1,
			DefaultWorkers:     1,
		},
	}
	_, q, _, worker := newIngestBatchTestDepsWithConfig(t, cfg)
	ctx := context.Background()
	projectID := testutil.CreateTestProject(t, ctx, q)

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "legacy-default-" + uuid.NewString()[:8],
		Name:      testutil.StrPtr("legacy queue trace"),
	})
	require.NoError(t, err)

	promptTokens := int64(11)
	completionTokens := int64(7)
	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID:        projectID,
		TraceID:          trace.ID,
		SpanID:           "legacy-span",
		Name:             "legacy span",
		PromptTokens:     &promptTokens,
		CompletionTokens: &completionTokens,
	})
	require.NoError(t, err)

	require.NoError(t, worker.client.Start(context.Background()))

	insertRes, err := worker.client.Insert(ctx, jobargs.TraceRollupArgs{TraceID: trace.ID}, &river.InsertOpts{
		Queue: river.QueueDefault,
	})
	require.NoError(t, err)
	assert.Equal(t, river.QueueDefault, insertRes.Job.Queue)

	require.Eventually(t, func() bool {
		updatedTrace, getErr := q.GetTrace(ctx, platform.GetTraceParams{
			ID:              trace.ID,
			ProjectFilterID: testutil.PgtypeUUID(projectID),
		})
		if getErr != nil || updatedTrace.Trace.TotalSpans == nil {
			return false
		}
		return *updatedTrace.Trace.TotalSpans == 1 && updatedTrace.Trace.TotalTokensIn == 11 && updatedTrace.Trace.TotalTokensOut == 7
	}, 5*time.Second, 100*time.Millisecond)
}
