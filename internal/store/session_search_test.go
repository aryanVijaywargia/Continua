package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestListSessionsFiltered_CreatedAtSortAndTotal(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	older := createSessionAt(t, ctx, s, projectID, "sess-older", "Older", "user-1", base)
	middle := createSessionAt(t, ctx, s, projectID, "sess-middle", "Middle", "user-1", base.Add(time.Hour))
	newer := createSessionAt(t, ctx, s, projectID, "sess-newer", "Newer", "user-1", base.Add(2*time.Hour))

	defaultResult, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectID,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), defaultResult.Total)
	assert.Equal(t, []uuid.UUID{newer.ID, middle.ID, older.ID}, sessionIDs(defaultResult.Sessions))

	ascendingResult, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectID,
		SortBy:    store.SessionSortByCreatedAt,
		SortDir:   store.SortDirectionAsc,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{older.ID, middle.ID, newer.ID}, sessionIDs(ascendingResult.Sessions))
}

func TestListSessionsFiltered_TraceCountSort(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)

	zero := createSessionAt(t, ctx, s, projectID, "sess-zero", "Zero", "user-1", base)
	one := createSessionAt(t, ctx, s, projectID, "sess-one", "One", "user-1", base.Add(time.Hour))
	two := createSessionAt(t, ctx, s, projectID, "sess-two", "Two", "user-1", base.Add(2*time.Hour))

	createSessionTraces(t, ctx, q, projectID, one.ID, 1)
	createSessionTraces(t, ctx, q, projectID, two.ID, 2)

	descending, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectID,
		SortBy:    store.SessionSortByTraceCount,
		SortDir:   store.SortDirectionDesc,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{two.ID, one.ID, zero.ID}, sessionIDs(descending.Sessions))

	ascending, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectID,
		SortBy:    store.SessionSortByTraceCount,
		SortDir:   store.SortDirectionAsc,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{zero.ID, one.ID, two.ID}, sessionIDs(ascending.Sessions))
}

func TestListSessionsFiltered_SearchRankingAndUserFilter(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC)

	exact := createSessionAt(t, ctx, s, projectID, "conv-123", "Exact", "user-42", base)
	prefix := createSessionAt(t, ctx, s, projectID, "conv-1234", "Prefix", "user-42", base.Add(time.Hour))
	nameOnly := createSessionAt(t, ctx, s, projectID, "sess-001", "conv-123 discussion", "user-42", base.Add(2*time.Hour))
	_ = createSessionAt(t, ctx, s, projectID, "conv-1239", "Filtered Out", "user-99", base.Add(3*time.Hour))

	result, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectID,
		Query:     "conv-123",
		UserID:    "user-42",
		SortBy:    store.SessionSortByCreatedAt,
		SortDir:   store.SortDirectionAsc,
		Limit:     10,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Total)
	assert.Equal(t, []uuid.UUID{exact.ID, prefix.ID, nameOnly.ID}, sessionIDs(result.Sessions))
}

func TestListSessionsFiltered_ProjectScopingAndStablePagination(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)
	stableTimestamp := time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)

	projectASessionIDs := make([]uuid.UUID, 0, 5)
	for i := 0; i < 5; i++ {
		session := createSessionAt(
			t,
			ctx,
			s,
			projectAID,
			fmt.Sprintf("stable-%d", i),
			fmt.Sprintf("Stable %d", i),
			"user-a",
			stableTimestamp,
		)
		projectASessionIDs = append(projectASessionIDs, session.ID)
		createSessionTraces(t, ctx, q, projectAID, session.ID, 1)
	}

	otherProjectSession := createSessionAt(t, ctx, s, projectBID, "other-project", "Other", "user-b", stableTimestamp)
	createSessionTraces(t, ctx, q, projectBID, otherProjectSession.ID, 3)

	pageOne, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectAID,
		SortBy:    store.SessionSortByTraceCount,
		SortDir:   store.SortDirectionDesc,
		Limit:     2,
		Offset:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), pageOne.Total)
	require.Len(t, pageOne.Sessions, 2)

	pageTwo, err := s.ListSessionsFiltered(ctx, store.SessionFilter{
		ProjectID: projectAID,
		SortBy:    store.SessionSortByTraceCount,
		SortDir:   store.SortDirectionDesc,
		Limit:     2,
		Offset:    2,
	})
	require.NoError(t, err)
	require.Len(t, pageTwo.Sessions, 2)

	pageOneIDs := sessionIDs(pageOne.Sessions)
	pageTwoIDs := sessionIDs(pageTwo.Sessions)
	for _, pageTwoID := range pageTwoIDs {
		assert.NotContains(t, pageOneIDs, pageTwoID)
	}
	for _, id := range append(pageOneIDs, pageTwoIDs...) {
		assert.Contains(t, projectASessionIDs, id)
		assert.NotEqual(t, otherProjectSession.ID, id)
	}
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createSessionAt(
	t *testing.T,
	ctx context.Context,
	s *store.Store,
	projectID uuid.UUID,
	externalID string,
	name string,
	userID string,
	createdAt time.Time,
) platform.Session {
	t.Helper()

	session, err := s.Queries().CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: externalID,
		Name:       optionalString(name),
		UserID:     optionalString(userID),
	})
	require.NoError(t, err)

	_, err = s.Pool().Exec(ctx, "UPDATE sessions SET created_at = $2, updated_at = $2 WHERE id = $1", session.ID, createdAt)
	require.NoError(t, err)

	session.CreatedAt = createdAt
	session.UpdatedAt = createdAt
	return session
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createSessionTraces(
	t *testing.T,
	ctx context.Context,
	q *platform.Queries,
	projectID uuid.UUID,
	sessionID uuid.UUID,
	count int,
) {
	t.Helper()

	for i := 0; i < count; i++ {
		_, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			SessionID: testutil.PgtypeUUID(sessionID),
			TraceID:   fmt.Sprintf("trace-%s-%d", sessionID.String()[:8], i),
			Name:      testutil.StrPtr(fmt.Sprintf("Trace %d", i)),
		})
		require.NoError(t, err)
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func sessionIDs(sessions []store.SessionWithCount) []uuid.UUID {
	ids := make([]uuid.UUID, len(sessions))
	for i := range sessions {
		ids[i] = sessions[i].ID
	}
	return ids
}
