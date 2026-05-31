package api_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// =============================================================================
// Spec 5: Sessions UI - API Tests
// =============================================================================
// These tests verify sessions API behavior as specified in specs/sessions-ui/spec.md

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

func TestSessionDetail_ReturnsSession(t *testing.T) {
	// Scenario: Session detail endpoint
	// WHEN GET /api/sessions/{id} is called
	// THEN the session is returned with all fields

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)

	// Create a session
	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "test-session-" + uuid.New().String()[:8],
		Name:       testutil.StrPtr("Test Session"),
		UserID:     testutil.StrPtr("user-123"),
	})
	require.NoError(t, err)

	// Verify session was created
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, projectID, session.ProjectID)
	require.NotNil(t, session.Name)
	assert.Equal(t, "Test Session", *session.Name)
	require.NotNil(t, session.UserID)
	assert.Equal(t, "user-123", *session.UserID)

	loaded, err := s.GetSessionWithTraceCount(ctx, store.BoundScope(projectID), session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ExternalID, loaded.ExternalID)
}

func TestSessionDetail_Returns404ForUnknownSession(t *testing.T) {
	// Scenario: Session detail endpoint returns 404 for unknown session
	// WHEN GET /api/sessions/{id} is called with unknown ID
	// THEN 404 is returned

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)

	unknownID := uuid.New()
	_, err := s.GetSessionWithTraceCount(ctx, store.UnboundedScope(), unknownID)

	// Should return not found
	assert.Error(t, err)
	assert.True(t, store.IsNotFound(err))
}

func TestSessionList_IncludesTraceCounts(t *testing.T) {
	// Scenario: Session list includes trace counts
	// WHEN GET /api/sessions is called
	// THEN each session includes id, name, user_id, trace_count, and created_at

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)

	// Create session
	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "test-session-" + uuid.New().String()[:8],
		Name:       testutil.StrPtr("Test Session"),
		UserID:     testutil.StrPtr("user-123"),
	})
	require.NoError(t, err)

	// Create traces for the session
	for i := 0; i < 3; i++ {
		_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			SessionID: testutil.PgtypeUUID(session.ID),
			TraceID:   "trace-" + uuid.New().String()[:8],
			Name:      testutil.StrPtr("trace " + string(rune('A'+i))),
		})
		require.NoError(t, err)
	}

	// Get sessions with trace count
	sessions, err := s.ListSessionsWithTraceCount(ctx, projectID, 10, 0)
	require.NoError(t, err)

	require.Len(t, sessions, 1)
	assert.Equal(t, session.ID, sessions[0].ID)
	assert.Equal(t, session.ExternalID, sessions[0].ExternalID)
	assert.Equal(t, int64(3), sessions[0].TraceCount, "trace_count should be 3")
}

func TestSessionTraceCount_ComputedCorrectly(t *testing.T) {
	// Scenario: trace_count computed per session
	// WHEN a session is returned from the API
	// THEN trace_count equals the count of traces with the same session_id and project_id

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)

	// Create two sessions
	session1, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "session-1-" + uuid.New().String()[:8],
		Name:       testutil.StrPtr("Session 1"),
	})
	require.NoError(t, err)

	session2, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "session-2-" + uuid.New().String()[:8],
		Name:       testutil.StrPtr("Session 2"),
	})
	require.NoError(t, err)

	// Create 5 traces for session1
	for i := 0; i < 5; i++ {
		_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			SessionID: testutil.PgtypeUUID(session1.ID),
			TraceID:   "trace-s1-" + string(rune('A'+i)),
			Name:      testutil.StrPtr("trace"),
		})
		require.NoError(t, err)
	}

	// Create 2 traces for session2
	for i := 0; i < 2; i++ {
		_, err = q.UpsertTrace(ctx, platform.UpsertTraceParams{
			ProjectID: projectID,
			SessionID: testutil.PgtypeUUID(session2.ID),
			TraceID:   "trace-s2-" + string(rune('A'+i)),
			Name:      testutil.StrPtr("trace"),
		})
		require.NoError(t, err)
	}

	// Get sessions with trace counts
	sessions, err := s.ListSessionsWithTraceCount(ctx, projectID, 10, 0)
	require.NoError(t, err)

	require.Len(t, sessions, 2)

	// Find each session in response
	sessionCounts := make(map[uuid.UUID]int64)
	for _, sess := range sessions {
		sessionCounts[sess.ID] = sess.TraceCount
	}

	assert.Equal(t, int64(5), sessionCounts[session1.ID], "session1 should have 5 traces")
	assert.Equal(t, int64(2), sessionCounts[session2.ID], "session2 should have 2 traces")
}

func TestSessionWithZeroTraces_AppearsInList(t *testing.T) {
	// Scenario: Session with zero traces
	// WHEN a session has no traces
	// THEN it appears in the sessions list with trace_count=0

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)

	// Create session with no traces
	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "empty-session-" + uuid.New().String()[:8],
		Name:       testutil.StrPtr("Empty Session"),
	})
	require.NoError(t, err)

	sessions, err := s.ListSessionsWithTraceCount(ctx, projectID, 10, 0)
	require.NoError(t, err)

	require.Len(t, sessions, 1)
	assert.Equal(t, session.ID, sessions[0].ID)
	assert.Equal(t, int64(0), sessions[0].TraceCount, "trace_count should be 0")
}

func TestSessionList_Pagination(t *testing.T) {
	// Scenario: Sessions pagination
	// WHEN more sessions exist than page size
	// THEN pagination controls are shown

	pool := testDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := createTestProject(t, ctx, q)

	// Create 25 sessions
	for i := 0; i < 25; i++ {
		_, err := q.CreateSession(ctx, platform.CreateSessionParams{
			ProjectID:  projectID,
			ExternalID: "session-" + uuid.New().String()[:8],
			Name:       testutil.StrPtr("Session " + string(rune('A'+i))),
		})
		require.NoError(t, err)
	}

	// First page
	sessions1, err := s.ListSessionsWithTraceCount(ctx, projectID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, sessions1, 10, "first page should have 10 sessions")

	// Second page
	sessions2, err := s.ListSessionsWithTraceCount(ctx, projectID, 10, 10)
	require.NoError(t, err)
	assert.Len(t, sessions2, 10, "second page should have 10 sessions")

	// No duplicates between pages
	page1IDs := make(map[uuid.UUID]bool)
	for _, sess := range sessions1 {
		page1IDs[sess.ID] = true
	}
	for _, sess := range sessions2 {
		assert.False(t, page1IDs[sess.ID], "page2 should not contain duplicates from page1")
	}
}
