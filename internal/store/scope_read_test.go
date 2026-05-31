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

func TestScopedReadsEnforceProjectScope(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	sessionA, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectAID,
		ExternalID: testutil.UniqueID("scope-session-a"),
	})
	require.NoError(t, err)
	sessionB, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectBID,
		ExternalID: testutil.UniqueID("scope-session-b"),
	})
	require.NoError(t, err)

	traceA := upsertScopedReadTrace(ctx, t, q, projectAID, sessionA.ID, "scope-trace-a")
	traceB := upsertScopedReadTrace(ctx, t, q, projectBID, sessionB.ID, "scope-trace-b")
	spanA := upsertScopedReadSpan(ctx, t, q, projectAID, traceA.ID, "scope-span-a")
	spanB := upsertScopedReadSpan(ctx, t, q, projectBID, traceB.ID, "scope-span-b")
	insertScopedReadEvent(ctx, t, q, projectAID, traceA.ID, spanA.SpanID, "scope-event-a")
	insertScopedReadEvent(ctx, t, q, projectBID, traceB.ID, spanB.SpanID, "scope-event-b")

	trace, err := s.GetTrace(ctx, store.BoundScope(projectAID), traceA.ID)
	require.NoError(t, err)
	assert.Equal(t, traceA.ID, trace.ID)

	_, err = s.GetTrace(ctx, store.BoundScope(projectAID), traceB.ID)
	assert.True(t, store.IsNotFound(err))

	trace, err = s.GetTrace(ctx, store.UnboundedScope(), traceB.ID)
	require.NoError(t, err)
	assert.Equal(t, traceB.ID, trace.ID)

	session, err := s.GetSessionWithTraceCount(ctx, store.BoundScope(projectAID), sessionA.ID)
	require.NoError(t, err)
	assert.Equal(t, sessionA.ID, session.ID)

	_, err = s.GetSessionWithTraceCount(ctx, store.BoundScope(projectAID), sessionB.ID)
	assert.True(t, store.IsNotFound(err))

	session, err = s.GetSessionWithTraceCount(ctx, store.UnboundedScope(), sessionB.ID)
	require.NoError(t, err)
	assert.Equal(t, sessionB.ID, session.ID)

	spans, err := s.ListSpansByTrace(ctx, store.BoundScope(projectAID), traceA.ID)
	require.NoError(t, err)
	require.Len(t, spans, 1)
	assert.Equal(t, spanA.ID, spans[0].ID)

	spans, err = s.ListSpansByTrace(ctx, store.BoundScope(projectAID), traceB.ID)
	require.NoError(t, err)
	assert.Empty(t, spans)

	spans, err = s.ListSpansByTrace(ctx, store.UnboundedScope(), traceB.ID)
	require.NoError(t, err)
	require.Len(t, spans, 1)
	assert.Equal(t, spanB.ID, spans[0].ID)

	events, err := s.ListSpanEventsByTrace(ctx, store.BoundScope(projectAID), traceA.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "scope-event-a", events[0].EventType)

	events, err = s.ListSpanEventsByTrace(ctx, store.BoundScope(projectAID), traceB.ID)
	require.NoError(t, err)
	assert.Empty(t, events)

	events, err = s.ListSpanEventsByTrace(ctx, store.UnboundedScope(), traceB.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "scope-event-b", events[0].EventType)
}

func upsertScopedReadTrace(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID, sessionID uuid.UUID,
	traceIDPrefix string,
) platform.Trace {
	t.Helper()

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(sessionID),
		TraceID:   testutil.UniqueID(traceIDPrefix),
		Name:      testutil.StrPtr(traceIDPrefix),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	return trace
}

func upsertScopedReadSpan(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID, traceID uuid.UUID,
	spanIDPrefix string,
) platform.Span {
	t.Helper()

	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   traceID,
		SpanID:    testutil.UniqueID(spanIDPrefix),
		Name:      spanIDPrefix,
		Type:      "chain",
		Status:    "completed",
		Level:     "default",
		StartTime: time.Date(2026, 5, 31, 9, 1, 0, 0, time.UTC),
		TotalCost: testutil.PgtypeNumericFromFloat64(0),
	})
	require.NoError(t, err)
	return span
}

func insertScopedReadEvent(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID, traceID uuid.UUID,
	spanID string,
	eventType string,
) {
	t.Helper()

	_, err := q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID: projectID,
		TraceID:   traceID,
		SpanID:    spanID,
		EventType: eventType,
		Level:     "info",
		EventTs:   testutil.PgtypeTimestamptz(time.Date(2026, 5, 31, 9, 2, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
}
