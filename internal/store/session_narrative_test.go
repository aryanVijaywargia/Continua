package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestBuildSessionNarrative_SummaryAggregatesAcrossMultipleTraces(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-aggregate")
	base := time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC)

	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-1", "Trace 1", "ok", base, timePtr(base.Add(2*time.Minute)), nil, 100, 50, 0.10, 0)
	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-2", "Trace 2", "error", base.Add(3*time.Minute), timePtr(base.Add(5*time.Minute)), nil, 25, 10, 0.02, 2)
	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-3", "Trace 3", "cancelled", base.Add(6*time.Minute), timePtr(base.Add(7*time.Minute)), nil, 5, 0, 0.03, 1)
	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-4", "Trace 4", "running", base.Add(8*time.Minute), nil, nil, 30, 15, 0.04, 0)
	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-5", "Trace 5", "custom-status", base.Add(10*time.Minute), nil, nil, 40, 20, 0.05, 0)

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)

	assert.Equal(t, int64(5), narrative.Summary.TotalTraceCount)
	assert.Equal(t, int64(5), narrative.Summary.ReturnedTraceCount)
	assert.Equal(t, int64(2), narrative.Summary.RunningTraceCount)
	assert.Equal(t, int64(1), narrative.Summary.CompletedTraceCount)
	assert.Equal(t, int64(2), narrative.Summary.FailedTraceCount)
	assert.Equal(t, int64(200), narrative.Summary.TotalTokensIn)
	assert.Equal(t, int64(95), narrative.Summary.TotalTokensOut)
	assert.InDelta(t, 0.24, numericToFloat64(t, narrative.Summary.TotalCostUsd), 0.0001)
	require.NotNil(t, narrative.Summary.StartedAt)
	assert.True(t, narrative.Summary.StartedAt.Equal(base))
	require.NotNil(t, narrative.Summary.LastActivityAt)
	assert.True(t, narrative.Summary.LastActivityAt.Equal(base.Add(10*time.Minute)))
}

func TestBuildSessionNarrative_UnknownStatusCountsAsRunning(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-unknown-status")
	startedAt := time.Date(2026, 3, 20, 11, 0, 0, 0, time.UTC)

	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-unknown", "Unknown", "paused", startedAt, nil, nil, 0, 0, 0, 0)

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)

	assert.Equal(t, int64(1), narrative.Summary.RunningTraceCount)
	assert.Zero(t, narrative.Summary.CompletedTraceCount)
	assert.Zero(t, narrative.Summary.FailedTraceCount)
}

func TestBuildSessionNarrative_LatestActivityIncludesTraceSpanAndEvents(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-activity")
	startedAt := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	traceEndedAt := startedAt.Add(2 * time.Minute)
	spanEndedAt := startedAt.Add(5 * time.Minute)
	eventAt := startedAt.Add(8 * time.Minute)

	trace := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-activity", "Activity", "completed", startedAt, &traceEndedAt, nil, 0, 0, 0, 0)
	createNarrativeSpan(t, ctx, q, projectID, trace.ID, "span-activity", "Activity Span", startedAt.Add(time.Minute), &spanEndedAt)
	createNarrativeEvent(t, ctx, pool, q, projectID, trace.ID, "span-activity", "decision", eventAt, eventAt, testutil.Int32Ptr(1), "latest event")

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)
	require.Len(t, narrative.Traces, 1)

	assert.True(t, narrative.Traces[0].LatestActivityAt.Equal(eventAt))
	require.NotNil(t, narrative.Summary.LastActivityAt)
	assert.True(t, narrative.Summary.LastActivityAt.Equal(traceEndedAt))
}

func TestBuildSessionNarrative_CapsAtOldestHundredAndSetsTruncated(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-cap")
	base := time.Date(2026, 3, 20, 13, 0, 0, 0, time.UTC)

	type expectedTrace struct {
		ID        uuid.UUID
		StartedAt time.Time
	}

	expected := make([]expectedTrace, 0, 101)
	for i := 0; i < 101; i++ {
		startedAt := base
		if i > 1 {
			startedAt = base.Add(time.Duration(i-1) * time.Minute)
		}

		trace := createNarrativeTrace(
			t,
			ctx,
			pool,
			q,
			projectID,
			session.ID,
			fmt.Sprintf("trace-cap-%03d", i),
			fmt.Sprintf("Trace %03d", i),
			"completed",
			startedAt,
			timePtr(startedAt.Add(time.Minute)),
			nil,
			0,
			0,
			0,
			0,
		)
		expected = append(expected, expectedTrace{ID: trace.ID, StartedAt: startedAt})
	}

	sort.Slice(expected, func(i, j int) bool {
		if expected[i].StartedAt.Equal(expected[j].StartedAt) {
			return bytes.Compare(expected[i].ID[:], expected[j].ID[:]) < 0
		}
		return expected[i].StartedAt.Before(expected[j].StartedAt)
	})

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)

	assert.Equal(t, int64(101), narrative.Summary.TotalTraceCount)
	assert.Equal(t, int64(100), narrative.Summary.ReturnedTraceCount)
	assert.True(t, narrative.Summary.Truncated)
	require.Len(t, narrative.Traces, 100)

	for i := 0; i < 100; i++ {
		assert.Equal(t, expected[i].ID, narrative.Traces[i].ID)
	}
}

func TestBuildSessionNarrative_SemanticEventsFilteredAndOrdered(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-events")
	startedAt := time.Date(2026, 3, 20, 14, 0, 0, 0, time.UTC)
	trace := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-events", "Events", "completed", startedAt, timePtr(startedAt.Add(10*time.Minute)), nil, 0, 0, 0, 0)
	createNarrativeSpan(t, ctx, q, projectID, trace.ID, "span-events", "Event Span", startedAt, timePtr(startedAt.Add(10*time.Minute)))

	createNarrativeEvent(t, ctx, pool, q, projectID, trace.ID, "span-events", "log", startedAt.Add(time.Minute), startedAt.Add(time.Minute), nil, "ignored log")
	createNarrativeEvent(t, ctx, pool, q, projectID, trace.ID, "span-events", "wait", startedAt.Add(2*time.Minute), startedAt.Add(2*time.Minute), testutil.Int32Ptr(4), "wait first")
	createNarrativeEvent(t, ctx, pool, q, projectID, trace.ID, "span-events", "effect", startedAt.Add(3*time.Minute), startedAt.Add(3*time.Minute), testutil.Int32Ptr(2), "effect second")
	createNarrativeEvent(t, ctx, pool, q, projectID, trace.ID, "span-events", "decision", startedAt.Add(3*time.Minute), startedAt.Add(3*time.Minute), testutil.Int32Ptr(1), "decision second")
	createNarrativeEvent(t, ctx, pool, q, projectID, trace.ID, "span-events", "error", startedAt.Add(4*time.Minute), startedAt.Add(4*time.Minute), nil, "ignored error")

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)
	require.Len(t, narrative.Traces, 1)
	require.Len(t, narrative.Traces[0].SemanticEvents, 3)

	assert.Equal(t, "wait", narrative.Traces[0].SemanticEvents[0].EventType)
	assert.Equal(t, "decision", narrative.Traces[0].SemanticEvents[1].EventType)
	assert.Equal(t, "effect", narrative.Traces[0].SemanticEvents[2].EventType)
}

func TestBuildSessionNarrative_InfersSequentialLineage(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-inferred")
	base := time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)

	parent := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-parent", "Parent", "completed", base, timePtr(base.Add(2*time.Minute)), nil, 0, 0, 0, 0)
	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-child", "Child", "running", base.Add(3*time.Minute), nil, nil, 0, 0, 0, 0)

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)
	require.Len(t, narrative.Traces, 2)

	assert.Equal(t, store.SessionNarrativeLineageTypeUnlinked, narrative.Traces[0].Lineage.Type)
	assert.Equal(t, store.SessionNarrativeLineageTypeInferred, narrative.Traces[1].Lineage.Type)
	require.NotNil(t, narrative.Traces[1].Lineage.ParentTraceID)
	assert.Equal(t, parent.TraceID, *narrative.Traces[1].Lineage.ParentTraceID)
	assert.Equal(t, int64(1), narrative.Summary.InferredLinkCount)
	assert.Equal(t, int64(1), narrative.Summary.UnlinkedTraceCount)
}

func TestBuildSessionNarrative_DoesNotInferLineageForOverlappingActivity(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "session-overlap")
	base := time.Date(2026, 3, 20, 16, 0, 0, 0, time.UTC)

	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-a", "Trace A", "completed", base, timePtr(base.Add(10*time.Minute)), nil, 0, 0, 0, 0)
	createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-b", "Trace B", "running", base.Add(5*time.Minute), nil, nil, 0, 0, 0, 0)

	narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
	require.NoError(t, err)
	require.Len(t, narrative.Traces, 2)

	assert.Equal(t, store.SessionNarrativeLineageTypeUnlinked, narrative.Traces[1].Lineage.Type)
	assert.Zero(t, narrative.Summary.InferredLinkCount)
	assert.Equal(t, int64(2), narrative.Summary.UnlinkedTraceCount)
}

func TestBuildSessionNarrative_ExplicitLineageMetadata(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	otherSession := createNarrativeSession(ctx, t, q, projectID, "session-out-of-set")
	otherTrace := createNarrativeTrace(t, ctx, pool, q, projectID, otherSession.ID, "trace-out-of-set", "Other", "completed", time.Date(2026, 3, 20, 17, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 20, 17, 1, 0, 0, time.UTC)), nil, 0, 0, 0, 0)

	testCases := []struct {
		name                string
		metadata            map[string]any
		expectedType        store.SessionNarrativeLineageType
		expectedParentTrace *string
		expectedTriggerSpan *string
		expectedLinkKind    *string
	}{
		{
			name: "valid explicit lineage",
			metadata: map[string]any{
				"__continua_lineage": map[string]any{
					"parent_trace_id": "trace-parent-explicit",
					"trigger_span_id": "trigger-span-1",
					"link_kind":       "handoff",
				},
			},
			expectedType:        store.SessionNarrativeLineageTypeExplicit,
			expectedParentTrace: testutil.StrPtr("trace-parent-explicit"),
			expectedTriggerSpan: testutil.StrPtr("trigger-span-1"),
			expectedLinkKind:    testutil.StrPtr("handoff"),
		},
		{
			name: "malformed metadata ignored",
			metadata: map[string]any{
				"__continua_lineage": map[string]any{
					"parent_trace_id": 42,
				},
			},
			expectedType: store.SessionNarrativeLineageTypeUnlinked,
		},
		{
			name: "out of session metadata ignored",
			metadata: map[string]any{
				"__continua_lineage": map[string]any{
					"parent_trace_id": otherTrace.TraceID,
				},
			},
			expectedType: store.SessionNarrativeLineageTypeUnlinked,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session := createNarrativeSession(ctx, t, q, projectID, "session-explicit-"+uuid.NewString()[:8])
			base := time.Date(2026, 3, 20, 18, 0, 0, 0, time.UTC)

			createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-parent-explicit", "Parent", "completed", base, timePtr(base.Add(10*time.Minute)), nil, 0, 0, 0, 0)
			createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-child-explicit", "Child", "running", base.Add(5*time.Minute), nil, tc.metadata, 0, 0, 0, 0)

			narrative, err := s.BuildSessionNarrative(ctx, projectID, session.ID, 100)
			require.NoError(t, err)
			require.Len(t, narrative.Traces, 2)

			lineage := narrative.Traces[1].Lineage
			assert.Equal(t, tc.expectedType, lineage.Type)
			if tc.expectedParentTrace != nil {
				require.NotNil(t, lineage.ParentTraceID)
				assert.Equal(t, *tc.expectedParentTrace, *lineage.ParentTraceID)
			} else {
				assert.Nil(t, lineage.ParentTraceID)
			}
			if tc.expectedTriggerSpan != nil {
				require.NotNil(t, lineage.TriggerSpanID)
				assert.Equal(t, *tc.expectedTriggerSpan, *lineage.TriggerSpanID)
			} else {
				assert.Nil(t, lineage.TriggerSpanID)
			}
			if tc.expectedLinkKind != nil {
				require.NotNil(t, lineage.LinkKind)
				assert.Equal(t, *tc.expectedLinkKind, *lineage.LinkKind)
			} else {
				assert.Nil(t, lineage.LinkKind)
			}
		})
	}
}

func createNarrativeSession(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	externalID string,
) platform.Session {
	t.Helper()

	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: externalID,
	})
	require.NoError(t, err)

	return session
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createNarrativeTrace(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	q *platform.Queries,
	projectID uuid.UUID,
	sessionID uuid.UUID,
	traceID string,
	name string,
	status string,
	startedAt time.Time,
	endedAt *time.Time,
	metadata map[string]any,
	totalTokensIn int64,
	totalTokensOut int64,
	totalCost float64,
	errorCount int32,
) platform.Trace {
	t.Helper()

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(sessionID),
		TraceID:   traceID,
		Name:      testutil.StrPtr(name),
		Status:    status,
		StartTime: testutil.PgtypeTimestamptz(startedAt),
		EndTime:   testutil.PgtypeTimestamptzPtr(endedAt),
		Metadata:  narrativeJSON(t, metadata),
	})
	require.NoError(t, err)

	require.NoError(t, q.UpdateTraceRollups(ctx, platform.UpdateTraceRollupsParams{
		ID:             trace.ID,
		TotalSpans:     testutil.Int32Ptr(0),
		TotalTokensIn:  totalTokensIn,
		TotalTokensOut: totalTokensOut,
		TotalCost:      testutil.PgtypeNumericFromFloat64(totalCost),
		ErrorCount:     testutil.Int32Ptr(errorCount),
	}))

	_, err = pool.Exec(ctx, "UPDATE traces SET server_received_at = $2 WHERE id = $1", trace.ID, startedAt)
	require.NoError(t, err)

	return trace
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createNarrativeSpan(
	t *testing.T,
	ctx context.Context,
	q *platform.Queries,
	projectID uuid.UUID,
	traceUUID uuid.UUID,
	spanID string,
	name string,
	startedAt time.Time,
	endedAt *time.Time,
) platform.Span {
	t.Helper()

	status := "running"
	if endedAt != nil {
		status = "completed"
	}

	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   traceUUID,
		SpanID:    spanID,
		Name:      name,
		Type:      "tool",
		Status:    status,
		Level:     "default",
		StartTime: startedAt,
		EndTime:   testutil.PgtypeTimestamptzPtr(endedAt),
		TotalCost: testutil.PgtypeNumericFromFloat64(0),
	})
	require.NoError(t, err)

	return span
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createNarrativeEvent(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	q *platform.Queries,
	projectID uuid.UUID,
	traceUUID uuid.UUID,
	spanID string,
	eventType string,
	eventAt time.Time,
	serverIngestedAt time.Time,
	sequence *int32,
	message string,
) uuid.UUID {
	t.Helper()

	eventID, err := q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID: projectID,
		TraceID:   traceUUID,
		SpanID:    spanID,
		EventType: eventType,
		Level:     "info",
		EventTs:   testutil.PgtypeTimestamptz(eventAt),
		Sequence:  sequence,
		Message:   testutil.StrPtr(message),
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, "UPDATE span_events SET server_ingested_at = $2 WHERE id = $1", eventID, serverIngestedAt)
	require.NoError(t, err)

	return eventID
}

func narrativeJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	if value == nil {
		return nil
	}

	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func numericToFloat64(t *testing.T, value interface{ Float64Value() (pgtype.Float8, error) }) float64 {
	t.Helper()
	floatValue, err := value.Float64Value()
	require.NoError(t, err)
	require.True(t, floatValue.Valid)
	return floatValue.Float64
}

func timePtr(ts time.Time) *time.Time {
	return &ts
}
