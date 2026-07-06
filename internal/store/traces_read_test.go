package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestListTraces_SortDirectionAndSessionIdentity(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "conv-123",
	})
	require.NoError(t, err)

	older := time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)

	traceWithSession, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(session.ID),
		TraceID:   "trace-with-session",
		Name:      testutil.StrPtr("Trace With Session"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(older),
	})
	require.NoError(t, err)

	traceWithoutSession, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-without-session",
		Name:      testutil.StrPtr("Trace Without Session"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(newer),
	})
	require.NoError(t, err)

	ascending, err := s.ListTraces(ctx, store.BoundScope(projectID), 10, 0, store.SortDirectionAsc)
	require.NoError(t, err)
	require.Len(t, ascending, 2)
	assert.Equal(t, traceWithSession.ID, ascending[0].ID)
	require.NotNil(t, ascending[0].SessionExternalID)
	assert.Equal(t, session.ExternalID, *ascending[0].SessionExternalID)
	assert.Nil(t, ascending[1].SessionExternalID)

	descending, err := s.ListTraces(ctx, store.BoundScope(projectID), 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	require.Len(t, descending, 2)
	assert.Equal(t, traceWithoutSession.ID, descending[0].ID)
	assert.Equal(t, traceWithSession.ID, descending[1].ID)

	sessionScoped, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	require.Len(t, sessionScoped, 1)
	assert.Equal(t, traceWithSession.ID, sessionScoped[0].ID)
	require.NotNil(t, sessionScoped[0].SessionExternalID)
	assert.Equal(t, session.ExternalID, *sessionScoped[0].SessionExternalID)

	total, err := s.CountTracesBySession(ctx, store.BoundScope(projectID), session.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

func TestListTracesFiltered_SearchIncludesSessionIdentityAndUsesRanking(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC)

	nameMatchSession, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "conv-name-match",
	})
	require.NoError(t, err)

	spanMatchSession, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "conv-span-match",
	})
	require.NoError(t, err)

	nameMatchTrace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(nameMatchSession.ID),
		TraceID:   "trace-name-match",
		Name:      testutil.StrPtr("checkout"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(base),
	})
	require.NoError(t, err)

	spanMatchTrace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(spanMatchSession.ID),
		TraceID:   "trace-span-match",
		Name:      testutil.StrPtr("background work"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(base.Add(2 * time.Hour)),
	})
	require.NoError(t, err)

	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   spanMatchTrace.ID,
		SpanID:    "span-checkout",
		Name:      "checkout",
		Type:      "tool",
		Status:    "completed",
		Level:     "default",
		StartTime: base.Add(2 * time.Hour),
		TotalCost: testutil.PgtypeNumericFromFloat64(0),
	})
	require.NoError(t, err)

	result, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		Scope:   store.BoundScope(projectID),
		Query:   "checkout",
		SortDir: store.SortDirectionAsc,
		Limit:   10,
		Offset:  0,
	})
	require.NoError(t, err)
	require.Len(t, result.Traces, 2)

	assert.Equal(t, nameMatchTrace.ID, result.Traces[0].ID, "trace-name match should rank ahead of span-only match")
	assert.Equal(t, spanMatchTrace.ID, result.Traces[1].ID)
	require.NotNil(t, result.Traces[0].SessionExternalID)
	require.NotNil(t, result.Traces[1].SessionExternalID)
	assert.Equal(t, nameMatchSession.ExternalID, *result.Traces[0].SessionExternalID)
	assert.Equal(t, spanMatchSession.ExternalID, *result.Traces[1].SessionExternalID)
	assert.Equal(t, int64(2), result.Total)
}

func TestListTraces_PaginationRemainsStableWhenTimestampsTie(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "conv-stable-order",
	})
	require.NoError(t, err)

	tiedStart := time.Date(2026, 3, 12, 15, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			SessionID: testutil.PgtypeUUID(session.ID),
			TraceID:   fmt.Sprintf("trace-tied-%d", i),
			Name:      testutil.StrPtr(fmt.Sprintf("Trace %d", i)),
			Status:    "completed",
			StartTime: testutil.PgtypeTimestamptz(tiedStart),
		})
		require.NoError(t, err)
	}

	fullDescending, err := s.ListTraces(ctx, store.BoundScope(projectID), 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	firstDescendingPage, err := s.ListTraces(ctx, store.BoundScope(projectID), 2, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	secondDescendingPage, err := s.ListTraces(ctx, store.BoundScope(projectID), 2, 2, store.SortDirectionDesc)
	require.NoError(t, err)
	assertStableTracePagination(t, fullDescending, firstDescendingPage, secondDescendingPage)

	fullAscending, err := s.ListTraces(ctx, store.BoundScope(projectID), 10, 0, store.SortDirectionAsc)
	require.NoError(t, err)
	firstAscendingPage, err := s.ListTraces(ctx, store.BoundScope(projectID), 2, 0, store.SortDirectionAsc)
	require.NoError(t, err)
	secondAscendingPage, err := s.ListTraces(ctx, store.BoundScope(projectID), 2, 2, store.SortDirectionAsc)
	require.NoError(t, err)
	assertStableTracePagination(t, fullAscending, firstAscendingPage, secondAscendingPage)

	fullSessionDescending, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 10, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	firstSessionDescendingPage, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 2, 0, store.SortDirectionDesc)
	require.NoError(t, err)
	secondSessionDescendingPage, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 2, 2, store.SortDirectionDesc)
	require.NoError(t, err)
	assertStableTracePagination(t, fullSessionDescending, firstSessionDescendingPage, secondSessionDescendingPage)

	fullSessionAscending, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 10, 0, store.SortDirectionAsc)
	require.NoError(t, err)
	firstSessionAscendingPage, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 2, 0, store.SortDirectionAsc)
	require.NoError(t, err)
	secondSessionAscendingPage, err := s.ListTracesBySession(ctx, store.BoundScope(projectID), session.ID, 2, 2, store.SortDirectionAsc)
	require.NoError(t, err)
	assertStableTracePagination(t, fullSessionAscending, firstSessionAscendingPage, secondSessionAscendingPage)
}

func TestListTracesFiltered_SearchPaginationRemainsStableWhenSortKeysTie(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	tiedStart := time.Date(2026, 3, 12, 18, 0, 0, 0, time.UTC)

	for i := 0; i < 4; i++ {
		_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			TraceID:   fmt.Sprintf("trace-search-tied-%d", i),
			Name:      testutil.StrPtr("checkout"),
			Status:    "completed",
			StartTime: testutil.PgtypeTimestamptz(tiedStart),
		})
		require.NoError(t, err)
	}

	fullResult, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		Scope:  store.BoundScope(projectID),
		Query:  "checkout",
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	firstPageResult, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		Scope:  store.BoundScope(projectID),
		Query:  "checkout",
		Limit:  2,
		Offset: 0,
	})
	require.NoError(t, err)
	secondPageResult, err := s.ListTracesFiltered(ctx, store.TraceFilter{
		Scope:  store.BoundScope(projectID),
		Query:  "checkout",
		Limit:  2,
		Offset: 2,
	})
	require.NoError(t, err)

	assertStableTracePagination(t, fullResult.Traces, firstPageResult.Traces, secondPageResult.Traces)
	assert.Equal(t, int64(4), fullResult.Total)
}

func assertStableTracePagination(t *testing.T, full []store.TraceRead, pages ...[]store.TraceRead) {
	t.Helper()

	var combined []string
	seen := make(map[string]struct{}, len(full))
	for _, page := range pages {
		for _, trace := range page {
			traceID := trace.ID.String()
			if _, exists := seen[traceID]; exists {
				t.Fatalf("duplicate trace returned across pages: %s", traceID)
			}
			seen[traceID] = struct{}{}
			combined = append(combined, traceID)
		}
	}

	assert.Equal(t, traceIDStrings(full), combined)
	assert.Len(t, seen, len(full))
}

func traceIDStrings(traces []store.TraceRead) []string {
	ids := make([]string, len(traces))
	for i := range traces {
		ids[i] = traces[i].ID.String()
	}
	return ids
}
