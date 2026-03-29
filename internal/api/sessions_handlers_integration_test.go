package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestListSessions_FiltersAndSortsByTraceCount(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC)

	lowCount := createSessionRecord(t, ctx, s, projectID, "conv-low", "Low", "user-42", base)
	highCount := createSessionRecord(t, ctx, s, projectID, "conv-high", "High", "user-42", base.Add(time.Hour))
	_ = createSessionRecord(t, ctx, s, projectID, "conv-other", "Other", "user-99", base.Add(2*time.Hour))

	createSessionTraceRecords(t, ctx, q, projectID, lowCount.ID, 1)
	createSessionTraceRecords(t, ctx, q, projectID, highCount.ID, 3)

	rec := invokeListSessions(t, server, projectID, ListSessionsParams{
		UserId:  testutil.StrPtr("user-42"),
		SortBy:  testutil.Ptr(TraceCount),
		SortDir: testutil.Ptr(ListSessionsParamsSortDirDesc),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[SessionList](t, rec)
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Sessions, 2)
	assert.Equal(t, []uuid.UUID{highCount.ID, lowCount.ID}, apiSessionIDs(resp.Sessions))
	require.NotNil(t, resp.Sessions[0].TraceCount)
	assert.Equal(t, 3, *resp.Sessions[0].TraceCount)
	require.NotNil(t, resp.Sessions[1].TraceCount)
	assert.Equal(t, 1, *resp.Sessions[1].TraceCount)
}

func TestListSessions_SearchOverridesSortAndKeepsFilteredTotal(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	exact := createSessionRecord(t, ctx, s, projectID, "conv-123", "Exact", "user-42", base)
	prefix := createSessionRecord(t, ctx, s, projectID, "conv-1234", "Prefix", "user-42", base.Add(time.Hour))
	nameOnly := createSessionRecord(t, ctx, s, projectID, "sess-001", "conv-123 discussion", "user-42", base.Add(2*time.Hour))
	newestNameOnly := createSessionRecord(t, ctx, s, projectID, "sess-002", "conv-123 notes", "user-42", base.Add(3*time.Hour))

	rec := invokeListSessions(t, server, projectID, ListSessionsParams{
		Q:       testutil.StrPtr("conv-123"),
		SortBy:  testutil.Ptr(CreatedAt),
		SortDir: testutil.Ptr(ListSessionsParamsSortDirAsc),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[SessionList](t, rec)
	assert.Equal(t, 4, resp.Total)
	assert.Equal(
		t,
		[]uuid.UUID{exact.ID, prefix.ID, newestNameOnly.ID, nameOnly.ID},
		apiSessionIDs(resp.Sessions),
	)
}

func TestGetSession_ProjectScopingReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	session := createSessionRecord(
		t,
		ctx,
		s,
		projectBID,
		"scoped-session",
		"Scoped",
		"user-42",
		time.Date(2026, 3, 9, 14, 0, 0, 0, time.UTC),
	)

	rec := invokeGetSession(t, server, projectAID, session.ID)
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}

func TestGetSessionNarrative_MissingSessionReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	s := store.New(pool)
	server := NewServer(s, nil)

	rec := invokeGetSessionNarrative(t, server, uuid.New(), uuid.New())
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
	assert.Equal(t, "Session not found", resp.Message)
}

func TestGetSessionNarrative_ProjectScopingReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	session := createSessionRecord(
		t,
		ctx,
		s,
		projectBID,
		"scoped-narrative",
		"Scoped Narrative",
		"user-42",
		time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC),
	)

	rec := invokeGetSessionNarrative(t, server, projectAID, session.ID)
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}

func TestGetSessionNarrative_ZeroTraceSessionReturnsEmptyNarrative(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(
		t,
		ctx,
		s,
		projectID,
		"narrative-zero",
		"Zero Narrative",
		"user-42",
		time.Date(2026, 3, 9, 16, 0, 0, 0, time.UTC),
	)

	rec := invokeGetSessionNarrative(t, server, projectID, session.ID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[SessionNarrativeResponse](t, rec)
	assert.Equal(t, 0, resp.Summary.TotalTraceCount)
	assert.Equal(t, 0, resp.Summary.ReturnedTraceCount)
	assert.False(t, resp.Summary.Truncated)
	assert.Nil(t, resp.Summary.StartedAt)
	assert.Nil(t, resp.Summary.LastActivityAt)
	assert.Empty(t, resp.Traces)
}

func invokeListSessions(t *testing.T, server *Server, projectID uuid.UUID, params ListSessionsParams) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.ListSessions(rec, req.WithContext(ctx), params)

	return rec
}

func invokeGetSession(t *testing.T, server *Server, projectID, sessionID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.GetSession(rec, req.WithContext(ctx), sessionID)

	return rec
}

func invokeGetSessionNarrative(t *testing.T, server *Server, projectID, sessionID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID.String()+"/narrative", nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.GetSessionNarrative(rec, req.WithContext(ctx), sessionID)

	return rec
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createSessionRecord(
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
		Name:       testutil.StrPtr(name),
		UserID:     testutil.StrPtr(userID),
	})
	require.NoError(t, err)

	_, err = s.Pool().Exec(ctx, "UPDATE sessions SET created_at = $2, updated_at = $2 WHERE id = $1", session.ID, createdAt)
	require.NoError(t, err)

	session.CreatedAt = createdAt
	session.UpdatedAt = createdAt
	return session
}

//nolint:revive // Keep testing.T first in shared test helper signatures.
func createSessionTraceRecords(
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
			TraceID:   uuid.NewString(),
			Name:      testutil.StrPtr("Trace"),
		})
		require.NoError(t, err)
	}
}

func apiSessionIDs(sessions []Session) []uuid.UUID {
	ids := make([]uuid.UUID, len(sessions))
	for i := range sessions {
		ids[i] = sessions[i].Id
	}
	return ids
}
