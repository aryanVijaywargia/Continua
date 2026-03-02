package store_test

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
// Spec 3: Search & Filtering Tests
// =============================================================================
// These tests verify full-text search and filtering behavior as specified in
// specs/search/spec.md

// TestSearch_ByTraceName tests searching traces by name using full-text search.
func TestSearch_ByTraceName(t *testing.T) {
	// Scenario: Search by trace name
	// WHEN a user searches for "checkout flow"
	// THEN traces with names containing both "checkout" and "flow" are returned
	// AND results are ranked by relevance (name matches weighted higher)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create test traces
	traceCheckoutFlow, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-checkout-flow",
		Name:      testutil.StrPtr("checkout flow process"),
	})
	require.NoError(t, err)

	traceCheckoutOnly, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-checkout-only",
		Name:      testutil.StrPtr("checkout process"),
	})
	require.NoError(t, err)

	traceFlowOnly, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-flow-only",
		Name:      testutil.StrPtr("flow management"),
	})
	require.NoError(t, err)

	traceUnrelated, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-unrelated",
		Name:      testutil.StrPtr("payment processing"),
	})
	require.NoError(t, err)

	// Search for "checkout flow" - should use AND semantics
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "checkout flow",
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)

	// Should return trace with BOTH "checkout" AND "flow"
	require.Len(t, results.Traces, 1, "AND semantics: should match only trace with both words")
	assert.Equal(t, traceCheckoutFlow.ID, results.Traces[0].ID)

	// Traces with only one word should NOT be returned
	traceIDs := make([]uuid.UUID, len(results.Traces))
	for i, tr := range results.Traces {
		traceIDs[i] = tr.ID
	}
	assert.NotContains(t, traceIDs, traceCheckoutOnly.ID, "trace with only 'checkout' should not match")
	assert.NotContains(t, traceIDs, traceFlowOnly.ID, "trace with only 'flow' should not match")
	assert.NotContains(t, traceIDs, traceUnrelated.ID, "unrelated trace should not match")
}

// TestSearch_ByUserID tests searching traces by user_id.
func TestSearch_ByUserID(t *testing.T) {
	// Scenario: Search by user_id
	// WHEN a user searches for "user123"
	// THEN traces with user_id containing "user123" are returned

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create test traces with different user IDs
	trace1, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-1",
		Name:      testutil.StrPtr("trace 1"),
		UserID:    testutil.StrPtr("user123"),
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-2",
		Name:      testutil.StrPtr("trace 2"),
		UserID:    testutil.StrPtr("user456"),
	})
	require.NoError(t, err)

	// Search for "user123"
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "user123",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1)
	assert.Equal(t, trace1.ID, results.Traces[0].ID)
}

// TestSearch_EmptyQuery tests that empty query returns all traces (no FTS filter).
func TestSearch_EmptyQuery(t *testing.T) {
	// Scenario: Empty query
	// WHEN q is empty or whitespace
	// THEN no full-text predicate is applied
	// AND results are filtered only by non-search filters

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create test traces
	for i := 0; i < 3; i++ {
		_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			TraceID:   "trace-" + uuid.New().String()[:8],
			Name:      testutil.StrPtr("trace " + string(rune('A'+i))),
		})
		require.NoError(t, err)
	}

	// Search with empty query - should return all traces
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "", // Empty
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Len(t, results.Traces, 3, "empty query should return all traces")

	// Search with whitespace query - should also return all traces
	results2, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "   ", // Whitespace only
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Len(t, results2.Traces, 3, "whitespace-only query should return all traces")
}

// TestSearch_FilterByStatus tests status filtering.
func TestSearch_FilterByStatus(t *testing.T) {
	// Scenario: Filter by status
	// WHEN status=COMPLETED is passed as query param
	// THEN traces with stored status matching "completed" are returned (case-insensitive)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create traces with different statuses
	traceCompleted, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-completed",
		Name:      testutil.StrPtr("completed trace"),
		Status:    "completed",
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-running",
		Name:      testutil.StrPtr("running trace"),
		Status:    "running",
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-failed",
		Name:      testutil.StrPtr("failed trace"),
		Status:    "failed",
	})
	require.NoError(t, err)

	// Filter by COMPLETED status
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Status:    "COMPLETED",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1)
	assert.Equal(t, traceCompleted.ID, results.Traces[0].ID)
}

// TestSearch_FilterByStatusFailed tests FAILED status filter mapping.
func TestSearch_FilterByStatusFailed(t *testing.T) {
	// Scenario: Status filter for FAILED
	// WHEN status=FAILED is passed
	// THEN traces with stored status in ("failed", "error") are returned (case-insensitive)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create traces with error statuses
	traceFailed, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-failed",
		Name:      testutil.StrPtr("failed trace"),
		Status:    "failed",
	})
	require.NoError(t, err)

	traceError, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-error",
		Name:      testutil.StrPtr("error trace"),
		Status:    "error",
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-completed",
		Name:      testutil.StrPtr("completed trace"),
		Status:    "completed",
	})
	require.NoError(t, err)

	// Filter by FAILED status - should match both "failed" and "error"
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Status:    "FAILED",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 2)
	traceIDs := []uuid.UUID{results.Traces[0].ID, results.Traces[1].ID}
	assert.Contains(t, traceIDs, traceFailed.ID)
	assert.Contains(t, traceIDs, traceError.ID)
}

// TestSearch_FilterByTimeRange tests time range filtering.
func TestSearch_FilterByTimeRange(t *testing.T) {
	// Scenario: Filter by time range
	// WHEN start_time_from and start_time_to are passed
	// THEN only traces started within that range are returned

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)
	threeDaysAgo := now.Add(-72 * time.Hour)

	// Create traces at different times
	traceYesterday, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-yesterday",
		Name:      testutil.StrPtr("yesterday trace"),
		StartTime: testutil.PgtypeTimestamptz(yesterday),
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-old",
		Name:      testutil.StrPtr("old trace"),
		StartTime: testutil.PgtypeTimestamptz(threeDaysAgo),
	})
	require.NoError(t, err)

	// Filter for traces from 2 days ago to now
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID:     projectID,
		StartTimeFrom: &twoDaysAgo,
		StartTimeTo:   &now,
		Limit:         10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1)
	assert.Equal(t, traceYesterday.ID, results.Traces[0].ID)
}

// TestSearch_FilterByTimeRangeWithNullStartTime tests fallback to server_received_at.
func TestSearch_FilterByTimeRangeWithNullStartTime(t *testing.T) {
	// Scenario: Filter by time range with missing start_time
	// WHEN a trace has start_time NULL
	// THEN time-range filtering uses COALESCE(start_time, server_received_at)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	// Create trace without start_time (will use server_received_at)
	traceNoStartTime, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-no-start-time",
		Name:      testutil.StrPtr("trace without start_time"),
		// StartTime is nil - will use server_received_at (now)
	})
	require.NoError(t, err)

	// Create trace with old start_time
	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-old",
		Name:      testutil.StrPtr("old trace"),
		StartTime: testutil.PgtypeTimestamptz(twoDaysAgo),
	})
	require.NoError(t, err)

	// Filter for recent traces - should include trace without start_time
	// because server_received_at is recent
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID:     projectID,
		StartTimeFrom: &yesterday,
		StartTimeTo:   &now,
		Limit:         10,
	})
	require.NoError(t, err)

	// Trace without start_time should be included (uses server_received_at)
	require.Len(t, results.Traces, 1)
	assert.Equal(t, traceNoStartTime.ID, results.Traces[0].ID)
}

// TestSearch_FilterByHasErrors tests error filtering.
func TestSearch_FilterByHasErrors(t *testing.T) {
	// Scenario: Filter by has_errors
	// WHEN has_errors=true is passed
	// THEN only traces with error_count > 0 are returned

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create trace with errors
	traceWithErrors, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-with-errors",
		Name:      testutil.StrPtr("trace with errors"),
	})
	require.NoError(t, err)

	// Update error_count directly
	err = q.UpdateTraceRollups(ctx, platform.UpdateTraceRollupsParams{
		ID:         traceWithErrors.ID,
		TotalSpans: testutil.Int32Ptr(5),
		ErrorCount: testutil.Int32Ptr(2),
	})
	require.NoError(t, err)

	// Create trace without errors
	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-no-errors",
		Name:      testutil.StrPtr("trace no errors"),
	})
	require.NoError(t, err)

	// Filter by has_errors=true
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		HasErrors: testutil.BoolPtr(true),
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1)
	assert.Equal(t, traceWithErrors.ID, results.Traces[0].ID)
}

// TestSearch_FilterByMinDuration tests minimum duration filtering.
func TestSearch_FilterByMinDuration(t *testing.T) {
	// Scenario: Filter by minimum duration
	// WHEN min_duration_ms=5000 is passed
	// THEN only traces with duration >= 5000ms are returned
	// AND running traces use COALESCE(end_time, now()) for duration calculation
	// AND start_time uses COALESCE(start_time, server_received_at) when NULL

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	now := time.Now()
	tenSecondsAgo := now.Add(-10 * time.Second) // 10s duration
	oneSecondAgo := now.Add(-1 * time.Second)   // 1s duration

	// Create long-running trace (10s)
	traceLong, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-long",
		Name:      testutil.StrPtr("long trace"),
		StartTime: testutil.PgtypeTimestamptz(tenSecondsAgo),
		EndTime:   testutil.PgtypeTimestamptz(now),
	})
	require.NoError(t, err)

	// Create short trace (1s)
	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-short",
		Name:      testutil.StrPtr("short trace"),
		StartTime: testutil.PgtypeTimestamptz(oneSecondAgo),
		EndTime:   testutil.PgtypeTimestamptz(now),
	})
	require.NoError(t, err)

	// Filter by min_duration_ms=5000 (5 seconds)
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID:     projectID,
		MinDurationMs: testutil.Int64Ptr(5000),
		Limit:         10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1)
	assert.Equal(t, traceLong.ID, results.Traces[0].ID)
}

// TestSearch_FindTraceBySpanName tests searching for traces by span name.
func TestSearch_FindTraceBySpanName(t *testing.T) {
	// Scenario: Search finds trace by span name
	// WHEN a user searches for "openai_chat"
	// THEN traces containing spans with that name are returned
	// AND each trace appears once

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create traces
	traceWithMatchingSpan, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-with-openai",
		Name:      testutil.StrPtr("unrelated name"),
	})
	require.NoError(t, err)

	traceWithoutMatchingSpan, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-no-openai",
		Name:      testutil.StrPtr("another unrelated name"),
	})
	require.NoError(t, err)

	// Create spans using trace.ID (uuid.UUID)
	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   traceWithMatchingSpan.ID,
		SpanID:    "span-1",
		Name:      "openai_chat completion",
	})
	require.NoError(t, err)

	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   traceWithoutMatchingSpan.ID,
		SpanID:    "span-2",
		Name:      "database query",
	})
	require.NoError(t, err)

	// Search for "openai_chat" - should find trace via span name
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "openai_chat",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1, "should find trace by span name")
	assert.Equal(t, traceWithMatchingSpan.ID, results.Traces[0].ID)
	assert.NotEqual(t, traceWithoutMatchingSpan.ID, results.Traces[0].ID)
}

// TestSearch_TraceMatchOutranksSpanOnlyMatch verifies ordering priority.
func TestSearch_TraceMatchOutranksSpanOnlyMatch(t *testing.T) {
	// Scenario: Trace match outranks span-only match
	// WHEN one trace matches by trace fields and another only by span name
	// THEN the trace field match appears first even if span relevance is high

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	traceFieldMatch, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-field-match",
		Name:      testutil.StrPtr("checkout"),
	})
	require.NoError(t, err)

	traceSpanOnlyMatch, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-span-only-match",
		Name:      testutil.StrPtr("unrelated trace"),
	})
	require.NoError(t, err)

	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   traceSpanOnlyMatch.ID,
		SpanID:    "span-rank-heavy",
		Name:      "checkout checkout checkout checkout",
	})
	require.NoError(t, err)

	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "checkout",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 2)
	assert.Equal(t, traceFieldMatch.ID, results.Traces[0].ID, "trace field match should rank above span-only match")
	assert.Equal(t, traceSpanOnlyMatch.ID, results.Traces[1].ID)
}

// TestSearch_MultipleSpansMatchSameTrace tests deduplication.
func TestSearch_MultipleSpansMatchSameTrace(t *testing.T) {
	// Scenario: Multiple matching spans
	// WHEN a trace has multiple spans that match the search query
	// THEN the trace appears once in results
	// AND total counts it once

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create trace
	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-multi-span",
		Name:      testutil.StrPtr("multi span trace"),
	})
	require.NoError(t, err)

	// Create multiple matching spans using trace.ID
	for i := 1; i <= 3; i++ {
		_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID: projectID,
			TraceID:   trace.ID,
			SpanID:    "span-" + string(rune('0'+i)),
			Name:      "llm_call operation",
		})
		require.NoError(t, err)
	}

	// Search should return trace once
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "llm_call",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1, "trace should appear only once despite multiple matching spans")
	assert.Equal(t, trace.ID, results.Traces[0].ID)
	assert.Equal(t, int64(1), results.Total, "total should be 1")
}

// TestSearch_CombinedSearchAndFilter tests combining search with filters.
func TestSearch_CombinedSearchAndFilter(t *testing.T) {
	// Scenario: Search with status filter
	// WHEN q=checkout&status=FAILED is passed
	// THEN only failed traces matching "checkout" are returned

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create traces
	_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-checkout-completed",
		Name:      testutil.StrPtr("checkout completed"),
		Status:    "completed",
	})
	require.NoError(t, err)

	traceCheckoutFailed, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-checkout-failed",
		Name:      testutil.StrPtr("checkout failed"),
		Status:    "failed",
	})
	require.NoError(t, err)

	_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-payment-failed",
		Name:      testutil.StrPtr("payment failed"),
		Status:    "failed",
	})
	require.NoError(t, err)

	// Search with both query and status filter
	results, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "checkout",
		Status:    "FAILED",
		Limit:     10,
	})
	require.NoError(t, err)

	require.Len(t, results.Traces, 1)
	assert.Equal(t, traceCheckoutFailed.ID, results.Traces[0].ID)
}

// TestSearch_Pagination tests pagination semantics.
func TestSearch_Pagination(t *testing.T) {
	// Scenario: Pagination with span join
	// WHEN searching with span name filter
	// THEN results use SELECT DISTINCT on traces
	// AND pagination applies after de-duplication

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)

	// Create multiple traces with matching spans
	for i := 0; i < 5; i++ {
		externalTraceID := "trace-" + string(rune('A'+i))
		trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			TraceID:   externalTraceID,
			Name:      testutil.StrPtr("trace " + string(rune('A'+i))),
		})
		require.NoError(t, err)

		_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
			ProjectID: projectID,
			TraceID:   trace.ID,
			SpanID:    "span-" + string(rune('A'+i)),
			Name:      "api_call",
		})
		require.NoError(t, err)
	}

	// First page
	page1, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "api_call",
		Limit:     2,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Len(t, page1.Traces, 2, "first page should have 2 traces")
	assert.Equal(t, int64(5), page1.Total, "total should be 5")

	// Second page
	page2, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "api_call",
		Limit:     2,
		Offset:    2,
	})
	require.NoError(t, err)
	assert.Len(t, page2.Traces, 2, "second page should have 2 traces")

	// Third page (partial)
	page3, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		ProjectID: projectID,
		Query:     "api_call",
		Limit:     2,
		Offset:    4,
	})
	require.NoError(t, err)
	assert.Len(t, page3.Traces, 1, "third page should have 1 trace")

	// No duplicates between pages
	allIDs := make(map[uuid.UUID]bool)
	for _, tr := range page1.Traces {
		allIDs[tr.ID] = true
	}
	for _, tr := range page2.Traces {
		assert.False(t, allIDs[tr.ID], "page2 should not contain duplicates from page1")
		allIDs[tr.ID] = true
	}
	for _, tr := range page3.Traces {
		assert.False(t, allIDs[tr.ID], "page3 should not contain duplicates from previous pages")
	}
}

// Helper functions
// (moved to internal/testutil/testutil.go to avoid redeclaration)
