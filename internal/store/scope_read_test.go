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

func TestListReadsEnforceProjectScope(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	token := testutil.UniqueID("scopelist")
	sessionA, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectAID,
		ExternalID: token + "-sess-a",
	})
	require.NoError(t, err)
	sessionB, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectBID,
		ExternalID: token + "-sess-b",
	})
	require.NoError(t, err)

	// Far-future start times keep these traces on the first unbounded
	// DESC-ordered page even in a shared test database.
	traceA := upsertScopedListTrace(ctx, t, q, projectAID, sessionA.ID, token+"-trace-a",
		time.Date(2999, 1, 1, 0, 1, 0, 0, time.UTC))
	traceB := upsertScopedListTrace(ctx, t, q, projectBID, sessionB.ID, token+"-trace-b",
		time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC))

	// ListTraces: a bound scope sees exactly its own project's rows.
	boundTraces, err := s.ListTraces(ctx, store.BoundScope(projectAID), 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{traceA.ID}, traceReadIDs(boundTraces))

	// ListTraces: an unbounded scope lists across projects.
	unboundedTraces, err := s.ListTraces(ctx, store.UnboundedScope(), 20, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	assert.Subset(t, traceReadIDs(unboundedTraces), []uuid.UUID{traceA.ID, traceB.ID})

	// Counts follow the same scope semantics.
	boundCount, err := s.CountTraces(ctx, store.BoundScope(projectAID))
	require.NoError(t, err)
	assert.Equal(t, int64(1), boundCount)

	unboundedCount, err := s.CountTraces(ctx, store.UnboundedScope())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, unboundedCount, int64(2))

	// ListTracesBySession: a bound scope cannot see another project's session traces.
	crossSessionTraces, err := s.ListTracesBySession(ctx, store.BoundScope(projectAID), sessionB.ID, 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	assert.Empty(t, crossSessionTraces)

	crossSessionCount, err := s.CountTracesBySession(ctx, store.BoundScope(projectAID), sessionB.ID)
	require.NoError(t, err)
	assert.Zero(t, crossSessionCount)

	unboundedSessionTraces, err := s.ListTracesBySession(ctx, store.UnboundedScope(), sessionB.ID, 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{traceB.ID}, traceReadIDs(unboundedSessionTraces))

	// ListSessionsWithTraceCount: a bound scope sees exactly its own sessions.
	boundSessions, err := s.ListSessionsWithTraceCount(ctx, store.BoundScope(projectAID), 10, 0)
	require.NoError(t, err)
	require.Len(t, boundSessions, 1)
	assert.Equal(t, sessionA.ID, boundSessions[0].ID)
	assert.Equal(t, int64(1), boundSessions[0].TraceCount)

	boundSessionCount, err := s.CountSessions(ctx, store.BoundScope(projectAID))
	require.NoError(t, err)
	assert.Equal(t, int64(1), boundSessionCount)

	unboundedSessionCount, err := s.CountSessions(ctx, store.UnboundedScope())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, unboundedSessionCount, int64(2))
}

func TestFilteredListReadsEnforceProjectScope(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	// A unique token keeps the filtered result sets deterministic even in a
	// shared test database.
	token := testutil.UniqueID("scopesearch")
	sessionA, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectAID,
		ExternalID: token + "-sess-a",
	})
	require.NoError(t, err)
	sessionB, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectBID,
		ExternalID: token + "-sess-b",
	})
	require.NoError(t, err)

	traceA := upsertScopedListTrace(ctx, t, q, projectAID, sessionA.ID, token+"-trace-a",
		time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	traceB := upsertScopedListTrace(ctx, t, q, projectBID, sessionB.ID, token+"-trace-b",
		time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC))

	// Trace search: bound scope only matches its own project's rows.
	boundResult, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		Scope: store.BoundScope(projectAID),
		Query: token,
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), boundResult.Total)
	require.Len(t, boundResult.Traces, 1)
	assert.Equal(t, traceA.ID, boundResult.Traces[0].ID)

	// Trace search: unbounded scope matches across projects.
	unboundedResult, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		Scope: store.UnboundedScope(),
		Query: token,
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), unboundedResult.Total)
	assert.ElementsMatch(t, []uuid.UUID{traceA.ID, traceB.ID}, traceReadIDs(unboundedResult.Traces))

	// Session search: bound scope only matches its own project's rows.
	boundSessions, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		Scope: store.BoundScope(projectAID),
		Query: token,
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), boundSessions.Total)
	require.Len(t, boundSessions.Sessions, 1)
	assert.Equal(t, sessionA.ID, boundSessions.Sessions[0].ID)

	// Session search: unbounded scope matches across projects, and the
	// per-session trace counts stay project-correct.
	unboundedSessions, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		Scope: store.UnboundedScope(),
		Query: token,
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), unboundedSessions.Total)
	require.Len(t, unboundedSessions.Sessions, 2)
	for _, sess := range unboundedSessions.Sessions {
		assert.Contains(t, []uuid.UUID{sessionA.ID, sessionB.ID}, sess.ID)
		assert.Equal(t, int64(1), sess.TraceCount)
	}
}

func traceReadIDs(traces []store.TraceRead) []uuid.UUID {
	ids := make([]uuid.UUID, len(traces))
	for i := range traces {
		ids[i] = traces[i].ID
	}
	return ids
}

func upsertScopedListTrace(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID, sessionID uuid.UUID,
	name string,
	startTime time.Time,
) platform.Trace {
	t.Helper()

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(sessionID),
		TraceID:   name,
		Name:      testutil.StrPtr(name),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(startTime),
	})
	require.NoError(t, err)
	return trace
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
