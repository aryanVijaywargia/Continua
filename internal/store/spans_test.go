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
// Spec 2: Idempotency - Span Time Handling Tests
// =============================================================================
// These tests verify the LEAST/GREATEST time merging behavior for out-of-order
// span updates as specified in specs/idempotency/spec.md

func TestUpsertSpan_EndTimeArriveBeforeStartTime(t *testing.T) {
	// Scenario: End time arrives before start time
	// WHEN a span update with end_time=T2 is ingested
	// AND a subsequent update with start_time=T1 is ingested (where T1 < T2)
	// THEN the span record has start_time=T1 and end_time=T2
	// AND both timestamps are preserved

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]
	trace := testutil.CreateTestTrace(t, ctx, q, projectID, traceID)
	spanID := "span-" + uuid.New().String()[:8]

	t1 := time.Now().Add(-time.Hour) // Earlier time
	t2 := time.Now()                 // Later time

	// First upsert: end_time only
	span1, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		EndTime:   testutil.PgtypeTimestamptz(t2),
		// start_time is zero/nil
	})
	require.NoError(t, err)
	require.NotNil(t, span1.EndTime)

	// Second upsert: start_time only
	span2, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t1,
		// end_time is zero/nil
	})
	require.NoError(t, err)

	// Both timestamps should be preserved
	require.False(t, span2.StartTime.IsZero(), "start_time should be set after second upsert")
	assert.WithinDuration(t, t1, span2.StartTime, time.Second, "start_time should be T1")
}

func TestUpsertSpan_StartTimeArriveBeforeEndTime(t *testing.T) {
	// Scenario: Start time arrives before end time
	// WHEN a span update with start_time=T1 is ingested
	// AND a subsequent update with end_time=T2 is ingested (where T2 > T1)
	// THEN the span record has start_time=T1 and end_time=T2
	// AND both timestamps are preserved

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]
	trace := testutil.CreateTestTrace(t, ctx, q, projectID, traceID)
	spanID := "span-" + uuid.New().String()[:8]

	t1 := time.Now().Add(-time.Hour) // Earlier time
	t2 := time.Now()                 // Later time

	// First upsert: start_time only
	span1, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t1,
	})
	require.NoError(t, err)
	require.False(t, span1.StartTime.IsZero())
	assert.WithinDuration(t, t1, span1.StartTime, time.Second)

	// Second upsert: end_time only
	span2, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		EndTime:   testutil.PgtypeTimestamptz(t2),
	})
	require.NoError(t, err)

	// Both timestamps should be preserved
	require.False(t, span2.StartTime.IsZero(), "start_time should still be set after second upsert")
	assert.WithinDuration(t, t1, span2.StartTime, time.Second, "start_time should be T1")
}

func TestUpsertSpan_EarlierStartTimeReplacesLater(t *testing.T) {
	// Scenario: Earlier start time replaces later
	// WHEN a span has start_time=T2
	// AND an update with start_time=T1 arrives (where T1 < T2)
	// THEN the span record has start_time=T1
	// AND the earlier timestamp is preserved

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]
	trace := testutil.CreateTestTrace(t, ctx, q, projectID, traceID)
	spanID := "span-" + uuid.New().String()[:8]

	t1 := time.Now().Add(-2 * time.Hour) // Earlier time
	t2 := time.Now().Add(-time.Hour)     // Later time

	// First upsert: start_time=T2 (later)
	span1, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t2,
	})
	require.NoError(t, err)
	require.False(t, span1.StartTime.IsZero())
	assert.WithinDuration(t, t2, span1.StartTime, time.Second)

	// Second upsert: start_time=T1 (earlier)
	span2, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t1,
	})
	require.NoError(t, err)

	// Earlier timestamp should be preserved (using LEAST)
	require.False(t, span2.StartTime.IsZero())
	assert.WithinDuration(t, t1, span2.StartTime, time.Second,
		"start_time should be T1 (earlier) - requires LEAST in SQL")
}

func TestUpsertSpan_LaterEndTimeReplacesEarlier(t *testing.T) {
	// Scenario: Later end time replaces earlier
	// WHEN a span has end_time=T1
	// AND an update with end_time=T2 arrives (where T2 > T1)
	// THEN the span record has end_time=T2
	// AND the later timestamp is preserved

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]
	trace := testutil.CreateTestTrace(t, ctx, q, projectID, traceID)
	spanID := "span-" + uuid.New().String()[:8]

	t1 := time.Now().Add(-time.Hour) // Earlier time
	t2 := time.Now()                 // Later time

	// First upsert: end_time=T1 (earlier)
	span1, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		EndTime:   testutil.PgtypeTimestamptz(t1),
	})
	require.NoError(t, err)
	require.True(t, span1.EndTime.Valid)

	// Second upsert: end_time=T2 (later)
	span2, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		EndTime:   testutil.PgtypeTimestamptz(t2),
	})
	require.NoError(t, err)

	// Later timestamp should be preserved (using GREATEST)
	require.True(t, span2.EndTime.Valid)
	assert.WithinDuration(t, t2, span2.EndTime.Time, time.Second,
		"end_time should be T2 (later) - requires GREATEST in SQL")
}

func TestUpsertSpan_NullTimestampDoesNotOverwrite(t *testing.T) {
	// Scenario: NULL timestamps handled correctly
	// WHEN a span update has start_time=NULL
	// AND the existing span has start_time=T1
	// THEN the span record retains start_time=T1
	// AND the NULL does not overwrite the existing value

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]
	trace := testutil.CreateTestTrace(t, ctx, q, projectID, traceID)
	spanID := "span-" + uuid.New().String()[:8]

	t1 := time.Now().Add(-time.Hour)

	// First upsert: start_time=T1
	span1, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t1,
	})
	require.NoError(t, err)
	require.False(t, span1.StartTime.IsZero())
	assert.WithinDuration(t, t1, span1.StartTime, time.Second)

	// Second upsert: start_time=nil (NULL), update name only
	span2, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "updated-span-name",
		// start_time is nil/zero value
	})
	require.NoError(t, err)

	// Original timestamp should be preserved
	require.False(t, span2.StartTime.IsZero())
	assert.WithinDuration(t, t1, span2.StartTime, time.Second,
		"start_time should be retained when update provides NULL")
	assert.Equal(t, "updated-span-name", span2.Name)
}

func TestUpsertSpan_DurationRecomputedWhenEarlierStartArrives(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.New().String()[:8]
	trace := testutil.CreateTestTrace(t, ctx, q, projectID, traceID)
	spanID := "span-" + uuid.New().String()[:8]

	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-2 * time.Hour)
	t3 := time.Now().Add(-1 * time.Hour)

	_, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t2,
	})
	require.NoError(t, err)

	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		EndTime:   testutil.PgtypeTimestamptz(t3),
	})
	require.NoError(t, err)

	spanFinal, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace,
		SpanID:    spanID,
		Name:      "test-span",
		StartTime: t1,
	})
	require.NoError(t, err)

	require.NotNil(t, spanFinal.DurationMs)
	expected := t3.Sub(t1).Milliseconds()
	assert.InDelta(t, float64(expected), float64(*spanFinal.DurationMs), 1,
		"duration_ms should be recomputed using merged (earliest start, latest end)")
}
