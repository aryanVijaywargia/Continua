package jobs_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/jobs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// =============================================================================
// Spec 1: Async Rollups via River - Job Queue Tests
// =============================================================================
// These tests verify async rollup processing behavior as specified in
// specs/async-rollups/spec.md

// testDB returns a connection pool for testing.
func testDB(t *testing.T) *pgxpool.Pool {
	return testutil.TestDB(t)
}

// createTestProject creates a test project and returns its ID.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func createTestProject(t *testing.T, ctx context.Context, q *platform.Queries) uuid.UUID {
	return testutil.CreateTestProject(t, ctx, q)
}

func TestRollupJob_Execution(t *testing.T) {
	// Scenario: Rollup job execution
	// WHEN the River worker processes a rollup job
	// THEN the worker computes trace aggregates (total_tokens_in/out, total_cost, span_count, error_count)
	// AND updates the trace record with computed values

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("test trace"),
	})
	require.NoError(t, err)

	// Create spans with token and cost data
	for i := 0; i < 3; i++ {
		promptTok := int64(100)
		completionTok := int64(50)
		_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID:        projectID,
			TraceID:          trace.ID,
			SpanID:           "span-" + string(rune('A'+i)),
			Name:             "llm call",
			PromptTokens:     &promptTok,
			CompletionTokens: &completionTok,
			TotalCost:        testutil.PgtypeNumericFromFloat64(0.01),
			Status:           "completed",
		})
		require.NoError(t, err)
	}

	// Create one error span
	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace.ID,
		SpanID:    "span-error",
		Name:      "failed operation",
		Status:    "error",
	})
	require.NoError(t, err)

	// Execute the rollup worker directly
	worker := jobs.NewTraceRollupWorker(s)
	err = worker.ProcessRollup(ctx, trace.ID)
	require.NoError(t, err)

	// Verify trace was updated with aggregates
	updatedTrace, err := q.GetTrace(ctx, platform.GetTraceParams{ID: trace.ID, ProjectFilterID: testutil.PgtypeUUID(projectID)})
	require.NoError(t, err)

	require.NotNil(t, updatedTrace.Trace.TotalSpans)
	assert.Equal(t, int32(4), *updatedTrace.Trace.TotalSpans, "total_spans should be 4")
	assert.Equal(t, int64(300), updatedTrace.Trace.TotalTokensIn, "total_tokens_in should be 300 (100 * 3)")
	assert.Equal(t, int64(150), updatedTrace.Trace.TotalTokensOut, "total_tokens_out should be 150 (50 * 3)")
	require.NotNil(t, updatedTrace.Trace.ErrorCount)
	assert.Equal(t, int32(1), *updatedTrace.Trace.ErrorCount, "error_count should be 1")
}

func TestRollupJob_RetryDoesNotDoubleCount(t *testing.T) {
	// Scenario: Retry does not double-count
	// WHEN a rollup job is retried
	// THEN the resulting totals match a single execution on the same spans

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("test trace"),
	})
	require.NoError(t, err)

	// Create spans
	for i := 0; i < 2; i++ {
		promptTok := int64(30)
		completionTok := int64(20)
		_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID:        projectID,
			TraceID:          trace.ID,
			SpanID:           "span-" + string(rune('A'+i)),
			Name:             "operation",
			PromptTokens:     &promptTok,
			CompletionTokens: &completionTok,
		})
		require.NoError(t, err)
	}

	worker := jobs.NewTraceRollupWorker(s)

	// Execute rollup first time
	err = worker.ProcessRollup(ctx, trace.ID)
	require.NoError(t, err)

	// Execute rollup second time (simulate retry)
	err = worker.ProcessRollup(ctx, trace.ID)
	require.NoError(t, err)

	// Verify totals are same (not doubled)
	updatedTrace, err := q.GetTrace(ctx, platform.GetTraceParams{ID: trace.ID, ProjectFilterID: testutil.PgtypeUUID(projectID)})
	require.NoError(t, err)

	require.NotNil(t, updatedTrace.Trace.TotalSpans)
	assert.Equal(t, int32(2), *updatedTrace.Trace.TotalSpans, "total_spans should be 2, not 4")
	assert.Equal(t, int64(60), updatedTrace.Trace.TotalTokensIn, "total_tokens_in should be 60, not 120")
	assert.Equal(t, int64(40), updatedTrace.Trace.TotalTokensOut, "total_tokens_out should be 40, not 80")
}

func TestRollupJob_EmptyTrace(t *testing.T) {
	// Scenario: Rollup on trace with no spans
	// WHEN rollup is computed for a trace with no spans
	// THEN rollup succeeds with zero values

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("empty trace"),
	})
	require.NoError(t, err)

	worker := jobs.NewTraceRollupWorker(s)
	err = worker.ProcessRollup(ctx, trace.ID)
	require.NoError(t, err)

	// Verify trace was updated with zero values
	updatedTrace, err := q.GetTrace(ctx, platform.GetTraceParams{ID: trace.ID, ProjectFilterID: testutil.PgtypeUUID(projectID)})
	require.NoError(t, err)

	require.NotNil(t, updatedTrace.Trace.TotalSpans)
	assert.Equal(t, int32(0), *updatedTrace.Trace.TotalSpans, "total_spans should be 0")
}

func TestRollupJob_MultipleLLMCalls(t *testing.T) {
	// Scenario: Multiple LLM calls aggregated
	// WHEN trace has multiple LLM spans with different token counts
	// THEN total_tokens is the sum of all span tokens

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("multi-llm trace"),
	})
	require.NoError(t, err)

	// Different token counts for each span (prompt_tokens, completion_tokens)
	type tokenPair struct{ in, out int64 }
	tokenCounts := []tokenPair{{60, 40}, {150, 100}, {300, 200}, {40, 35}}
	for i, tp := range tokenCounts {
		in, out := tp.in, tp.out
		_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID:        projectID,
			TraceID:          trace.ID,
			SpanID:           "span-" + string(rune('A'+i)),
			Name:             "llm-call",
			PromptTokens:     &in,
			CompletionTokens: &out,
		})
		require.NoError(t, err)
	}

	worker := jobs.NewTraceRollupWorker(s)
	err = worker.ProcessRollup(ctx, trace.ID)
	require.NoError(t, err)

	updatedTrace, err := q.GetTrace(ctx, platform.GetTraceParams{ID: trace.ID, ProjectFilterID: testutil.PgtypeUUID(projectID)})
	require.NoError(t, err)

	assert.Equal(t, int64(550), updatedTrace.Trace.TotalTokensIn, "total_tokens_in should be sum of prompt_tokens")
	assert.Equal(t, int64(375), updatedTrace.Trace.TotalTokensOut, "total_tokens_out should be sum of completion_tokens")
}
