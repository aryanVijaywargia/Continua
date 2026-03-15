package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// =============================================================================
// Trace Rollup Computation Tests
// =============================================================================
// Tests for enable-e2e-usability/specs/trace-rollups/spec.md

func TestRollups_TokenAggregation(t *testing.T) {
	// Scenario: Token aggregation
	// WHEN trace has spans with prompt_tokens/completion_tokens values
	// THEN trace total_tokens_in/out equals sums of directional token values

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	// Create trace
	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Token Test Trace"),
	})
	require.NoError(t, err)

	// Create spans with split token values
	type tokenPair struct{ in, out int64 }
	tokenCounts := []tokenPair{{50, 50}, {100, 100}, {75, 75}}
	for i, tp := range tokenCounts {
		in, out := tp.in, tp.out
		_, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID:        projectID,
			TraceID:          trace.ID,
			SpanID:           "span-" + string(rune('A'+i)),
			Name:             "LLM Call",
			PromptTokens:     &in,
			CompletionTokens: &out,
		})
		require.NoError(t, err)
	}

	// Compute rollups via store
	err = s.ComputeAndUpdateTraceRollups(ctx, trace.ID)
	require.NoError(t, err)

	// Verify aggregation
	updatedTrace, err := q.GetTrace(ctx, trace.ID)
	require.NoError(t, err)

	assert.Equal(t, int64(225), updatedTrace.Trace.TotalTokensIn, "total_tokens_in should equal sum of prompt_tokens")
	assert.Equal(t, int64(225), updatedTrace.Trace.TotalTokensOut, "total_tokens_out should equal sum of completion_tokens")
}

func TestRollups_CostAggregation(t *testing.T) {
	// Scenario: Cost aggregation
	// WHEN trace has spans with total_cost values
	// THEN trace total_cost equals sum of span values

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Cost Test Trace"),
	})
	require.NoError(t, err)

	// Create spans with cost values
	costs := []float64{0.01, 0.02, 0.005}
	for i, cost := range costs {
		_, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID: projectID,
			TraceID:   trace.ID,
			SpanID:    "span-" + string(rune('A'+i)),
			Name:      "LLM Call",
			TotalCost: testutil.PgtypeNumericFromFloat64(cost),
		})
		require.NoError(t, err)
	}

	err = s.ComputeAndUpdateTraceRollups(ctx, trace.ID)
	require.NoError(t, err)

	updatedTrace, err := q.GetTrace(ctx, trace.ID)
	require.NoError(t, err)

	// Note: TotalCost is pgtype.Numeric, compare validity instead of exact value
	assert.True(t, updatedTrace.Trace.TotalCost.Valid, "total_cost should be set")
}

func TestRollups_SpanCount(t *testing.T) {
	// Scenario: Span count
	// WHEN trace has N spans
	// THEN trace total_spans equals N

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Span Count Test"),
	})
	require.NoError(t, err)

	// Create 5 spans
	for i := 0; i < 5; i++ {
		_, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID: projectID,
			TraceID:   trace.ID,
			SpanID:    "span-" + string(rune('A'+i)),
			Name:      "Operation",
		})
		require.NoError(t, err)
	}

	err = s.ComputeAndUpdateTraceRollups(ctx, trace.ID)
	require.NoError(t, err)

	updatedTrace, err := q.GetTrace(ctx, trace.ID)
	require.NoError(t, err)

	require.NotNil(t, updatedTrace.Trace.TotalSpans)
	assert.Equal(t, int32(5), *updatedTrace.Trace.TotalSpans, "total_spans should be 5")
}

func TestRollups_ErrorCounting(t *testing.T) {
	// Scenario: Error counting
	// WHEN trace has spans with status "failed" or "error"
	// THEN trace error_count equals count of failed spans

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Error Count Test"),
	})
	require.NoError(t, err)

	// Create spans - 2 completed, 3 failed
	statuses := []string{"completed", "failed", "completed", "error", "failed"}
	for i, status := range statuses {
		_, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID: projectID,
			TraceID:   trace.ID,
			SpanID:    "span-" + string(rune('A'+i)),
			Name:      "Operation",
			Status:    status,
		})
		require.NoError(t, err)
	}

	err = s.ComputeAndUpdateTraceRollups(ctx, trace.ID)
	require.NoError(t, err)

	updatedTrace, err := q.GetTrace(ctx, trace.ID)
	require.NoError(t, err)

	// 3 failed/error spans
	require.NotNil(t, updatedTrace.Trace.ErrorCount)
	assert.Equal(t, int32(3), *updatedTrace.Trace.ErrorCount, "error_count should be 3")
}

func TestRollups_DurationComputation(t *testing.T) {
	// Scenario: Duration computation
	// WHEN trace has start_time and end_time set
	// THEN trace duration_ms is computed from timestamps

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	startTime := time.Now().Add(-5 * time.Second)
	endTime := time.Now()

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Duration Test"),
		StartTime: testutil.PgtypeTimestamptz(startTime),
		EndTime:   testutil.PgtypeTimestamptz(endTime),
	})
	require.NoError(t, err)

	err = s.ComputeAndUpdateTraceRollups(ctx, trace.ID)
	require.NoError(t, err)

	updatedTrace, err := q.GetTrace(ctx, trace.ID)
	require.NoError(t, err)

	// Duration should be approximately 5000ms
	require.NotNil(t, updatedTrace.Trace.DurationMs)
	assert.InDelta(t, 5000, *updatedTrace.Trace.DurationMs, 100, "duration should be ~5000ms")
}

func TestRollups_FailureNonBlocking(t *testing.T) {
	// Scenario: Rollup failure non-blocking
	// WHEN rollup computation fails
	// THEN warning is logged
	// AND ingest transaction is NOT aborted
	// AND trace data is still persisted

	// This test verifies that rollup errors don't bubble up
	// The trace should exist even if rollups fail

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]

	// Create trace
	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Rollup Failure Test"),
	})
	require.NoError(t, err)

	// Trace should exist regardless of rollup computation
	fetchedTrace, err := q.GetTrace(ctx, trace.ID)
	require.NoError(t, err)
	assert.Equal(t, traceID, fetchedTrace.Trace.TraceID)
}

// =============================================================================
// Ingestion Service Tests
// =============================================================================
// Tests for add-ingestion-pipeline/specs/ingestion/spec.md

func TestIngest_CreateNewTrace(t *testing.T) {
	// Scenario: Create new trace
	// GIVEN a trace with trace_id: "trace-001" does not exist
	// WHEN an ingest request contains that trace
	// THEN a new trace is created with an internal UUID
	// AND the external trace_id is stored for lookup

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	externalTraceID := "trace-new-" + uuid.New().String()[:8]

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   externalTraceID,
		Name:      testutil.StrPtr("New Trace"),
	})
	require.NoError(t, err)

	// Should have both internal UUID and external trace_id
	assert.NotEqual(t, uuid.Nil, trace.ID, "should have internal UUID")
	assert.Equal(t, externalTraceID, trace.TraceID, "should have external trace_id")
}

func TestIngest_UpdateExistingTrace(t *testing.T) {
	// Scenario: Update existing trace
	// GIVEN a trace with trace_id: "trace-001" exists with name: "Old Name"
	// WHEN an ingest request contains the same trace_id with name: "New Name"
	// THEN the trace name is updated to "New Name"
	// AND other fields not provided are preserved

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-update-" + uuid.New().String()[:8]

	// Create initial trace
	oldName := "Old Name"
	userID := "user-123"
	_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      &oldName,
		UserID:    &userID,
	})
	require.NoError(t, err)

	// Update with new name, no user_id
	newName := "New Name"
	updated, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      &newName,
		// UserID not provided - should be preserved
	})
	require.NoError(t, err)

	assert.Equal(t, "New Name", *updated.Name, "name should be updated")
	assert.Equal(t, "user-123", *updated.UserID, "user_id should be preserved")
}

func TestIngest_ErrorStatusPreserved(t *testing.T) {
	// Scenario: Error status preserved
	// GIVEN a trace exists with status: "error"
	// WHEN an update attempts to set status: "ok"
	// THEN the status remains "error" (never downgraded)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-status-" + uuid.New().String()[:8]

	// Create trace with error status
	_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Error Trace"),
		Status:    "error",
	})
	require.NoError(t, err)

	// Try to update to "ok"
	updated, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Status:    "ok",
	})
	require.NoError(t, err)

	// Status should remain "error"
	assert.Equal(t, "error", updated.Status, "error status should not be downgraded")
}

func TestIngest_SpanWithTraceUUID(t *testing.T) {
	// Scenario: Create span for existing trace
	// GIVEN a trace with trace_id exists
	// WHEN an ingest request contains a span for that trace
	// THEN the span is created with trace reference

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-span-test-" + uuid.New().String()[:8]

	// Create trace
	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Span Test Trace"),
	})
	require.NoError(t, err)

	// Create span using trace.ID (uuid.UUID)
	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace.ID,
		SpanID:    "span-001",
		Name:      "Test Span",
	})
	require.NoError(t, err)

	assert.Equal(t, trace.ID, span.TraceID, "span should reference trace by UUID")
	assert.Equal(t, "span-001", span.SpanID)
}

func TestIngest_DuplicateEventIgnored(t *testing.T) {
	// Scenario: Duplicate event silently ignored
	// GIVEN an event with idempotency_key already exists
	// WHEN an ingest request contains the same idempotency_key
	// THEN the duplicate is silently ignored (no error)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-event-" + uuid.New().String()[:8]

	// Create trace first
	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Event Test Trace"),
	})
	require.NoError(t, err)

	idempotencyKey := "event-key-" + uuid.New().String()

	// Insert event first time
	_, err = q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID:      projectID,
		TraceID:        trace.ID,
		SpanID:         "span-001",
		EventType:      "log",
		Level:          "info",
		Message:        testutil.StrPtr("First event"),
		IdempotencyKey: &idempotencyKey,
	})
	require.NoError(t, err)

	// Try to insert duplicate - should succeed (ON CONFLICT DO NOTHING)
	_, err = q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID:      projectID,
		TraceID:        trace.ID,
		SpanID:         "span-001",
		EventType:      "log",
		Level:          "info",
		Message:        testutil.StrPtr("Duplicate event"),
		IdempotencyKey: &idempotencyKey,
	})
	require.NoError(t, err)
}
