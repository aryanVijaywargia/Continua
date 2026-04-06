package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestGetSessionCompare_MissingRequiredQueryParamReturns400(t *testing.T) {
	server := NewServer(nil, nil)
	handler := Handler(server)
	sessionID := uuid.New()

	testCases := []struct {
		name         string
		query        string
		missingParam string
	}{
		{
			name:         "missing baseline trace id",
			query:        "?candidate_trace_id=" + uuid.NewString(),
			missingParam: "baseline_trace_id",
		},
		{
			name:         "missing candidate trace id",
			query:        "?baseline_trace_id=" + uuid.NewString(),
			missingParam: "candidate_trace_id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID.String()+"/compare"+tc.query, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.missingParam)
			assert.True(t, strings.Contains(rec.Body.String(), "is required"), "expected required-param error, got %q", rec.Body.String())
		})
	}
}

func TestGetSessionCompare_MissingSessionReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	s := store.New(pool)
	server := NewServer(s, nil)

	rec := invokeGetSessionCompare(t, server, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
	assert.Equal(t, "Session or trace not found", resp.Message)
}

func TestGetSessionCompare_ProjectScopingReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectBID, "compare-scoped", "Scoped", "user-42", time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC))
	baseline := createCompareTraceRecord(ctx, t, pool, q, projectBID, session.ID, "trace-b", "Baseline", "completed", time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 10, 1, 0, 0, time.UTC)))
	candidate := createCompareTraceRecord(ctx, t, pool, q, projectBID, session.ID, "trace-c", "Candidate", "completed", time.Date(2026, 3, 25, 10, 2, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 10, 3, 0, 0, time.UTC)))

	rec := invokeGetSessionCompare(t, server, projectAID, session.ID, baseline.ID, candidate.ID)
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}

func TestGetSessionCompare_TraceNotInSessionReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectID, "compare-main", "Main", "user-42", time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC))
	otherSession := createSessionRecord(t, ctx, s, projectID, "compare-other", "Other", "user-42", time.Date(2026, 3, 25, 11, 5, 0, 0, time.UTC))
	baseline := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-b", "Baseline", "completed", time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 11, 1, 0, 0, time.UTC)))
	candidate := createCompareTraceRecord(ctx, t, pool, q, projectID, otherSession.ID, "trace-c", "Candidate", "completed", time.Date(2026, 3, 25, 11, 2, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 11, 3, 0, 0, time.UTC)))

	rec := invokeGetSessionCompare(t, server, projectID, session.ID, baseline.ID, candidate.ID)
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}

func TestGetSessionCompare_IdenticalTraceIDsReturn400(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectID, "compare-identical", "Identical", "user-42", time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC))
	trace := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-same", "Trace", "completed", time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC)))

	rec := invokeGetSessionCompare(t, server, projectID, session.ID, trace.ID, trace.ID)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "invalid_compare_request", resp.Code)
	assert.Equal(t, "Baseline and candidate traces must be different", resp.Message)
}

func TestGetSessionCompare_RunningTraceReturns400(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectID, "compare-running", "Running", "user-42", time.Date(2026, 3, 25, 13, 0, 0, 0, time.UTC))
	baseline := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-b", "Baseline", "completed", time.Date(2026, 3, 25, 13, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 13, 1, 0, 0, time.UTC)))
	candidate := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-c", "Candidate", "running", time.Date(2026, 3, 25, 13, 2, 0, 0, time.UTC), nil)

	rec := invokeGetSessionCompare(t, server, projectID, session.ID, baseline.ID, candidate.ID)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "invalid_compare_request", resp.Code)
	assert.Equal(t, "Both traces must be terminal to compare", resp.Message)
}

func TestGetSessionCompare_TooLargeReturns422WithDetail(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectID, "compare-too-large", "Too Large", "user-42", time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC))
	baseline := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-b", "Baseline", "completed", time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 14, 1, 0, 0, time.UTC)))
	candidate := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-c", "Candidate", "completed", time.Date(2026, 3, 25, 14, 2, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 14, 3, 0, 0, time.UTC)))

	for i := 0; i < store.SessionCompareMaxSpans+1; i++ {
		createCompareSpanRecord(
			ctx,
			t,
			q,
			projectID,
			baseline.ID,
			"baseline-span-"+uuid.NewString(),
			nil,
			"Baseline Span",
			"tool",
			"completed",
			time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC).Add(time.Duration(i)*time.Second),
			timePtr(time.Date(2026, 3, 25, 14, 0, 1, 0, time.UTC).Add(time.Duration(i)*time.Second)),
			nil,
			nil,
			nil,
			0,
			0,
		)
	}

	rec := invokeGetSessionCompare(t, server, projectID, session.ID, baseline.ID, candidate.ID)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	resp := decodeJSONBody[ComparisonTooLargeError](t, rec)
	assert.Equal(t, "comparison_too_large", resp.Code)
	assert.Equal(t, store.SessionCompareMaxSpans+1, resp.Detail.BaselineSpanCount)
	assert.Equal(t, 0, resp.Detail.CandidateSpanCount)
	assert.Equal(t, store.SessionCompareMaxSpans, resp.Detail.MaxSpans)
	assert.Equal(t, store.SessionCompareMaxSemanticEvents, resp.Detail.MaxSemanticEvents)
}

func TestGetSessionCompare_TooManySemanticEventsReturns422WithDetail(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectID, "compare-too-many-semantics", "Too Many Semantics", "user-42", time.Date(2026, 3, 25, 14, 30, 0, 0, time.UTC))
	baseline := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-b", "Baseline", "completed", time.Date(2026, 3, 25, 14, 30, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 14, 31, 0, 0, time.UTC)))
	candidate := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-c", "Candidate", "completed", time.Date(2026, 3, 25, 14, 32, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 14, 33, 0, 0, time.UTC)))

	createCompareSpanRecord(
		ctx,
		t,
		q,
		projectID,
		baseline.ID,
		"shared-root",
		nil,
		"Baseline Span",
		"tool",
		"completed",
		time.Date(2026, 3, 25, 14, 30, 0, 0, time.UTC),
		timePtr(time.Date(2026, 3, 25, 14, 30, 1, 0, time.UTC)),
		nil,
		nil,
		nil,
		0,
		0,
	)

	for i := 0; i < store.SessionCompareMaxSemanticEvents+1; i++ {
		createCompareSemanticEventRecord(
			ctx,
			t,
			q,
			projectID,
			baseline.ID,
			"shared-root",
			"decision",
			time.Date(2026, 3, 25, 14, 30, 0, 0, time.UTC).Add(time.Duration(i)*time.Millisecond),
			int32PtrCompare(int32(i+1)),
			"decision",
			map[string]any{"question": "Which path?"},
		)
	}

	rec := invokeGetSessionCompare(t, server, projectID, session.ID, baseline.ID, candidate.ID)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	resp := decodeJSONBody[ComparisonTooLargeError](t, rec)
	assert.Equal(t, "comparison_too_large", resp.Code)
	assert.Equal(t, store.SessionCompareMaxSemanticEvents+1, resp.Detail.BaselineSemanticCount)
	assert.Equal(t, 0, resp.Detail.CandidateSemanticCount)
	assert.Equal(t, store.SessionCompareMaxSpans, resp.Detail.MaxSpans)
	assert.Equal(t, store.SessionCompareMaxSemanticEvents, resp.Detail.MaxSemanticEvents)
}

func TestGetSessionCompare_EmptyComparisonReturns200(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createSessionRecord(t, ctx, s, projectID, "compare-empty", "Empty", "user-42", time.Date(2026, 3, 25, 15, 0, 0, 0, time.UTC))
	baseline := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-b", "Baseline", "completed", time.Date(2026, 3, 25, 15, 0, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 15, 1, 0, 0, time.UTC)))
	candidate := createCompareTraceRecord(ctx, t, pool, q, projectID, session.ID, "trace-c", "Candidate", "completed", time.Date(2026, 3, 25, 15, 2, 0, 0, time.UTC), timePtr(time.Date(2026, 3, 25, 15, 3, 0, 0, time.UTC)))

	rec := invokeGetSessionCompare(t, server, projectID, session.ID, baseline.ID, candidate.ID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[SessionCompareResponse](t, rec)
	assert.Equal(t, session.ExternalID, resp.Session.ExternalId)
	assert.Empty(t, resp.SpanDiffs)
	assert.Equal(t, 0, resp.Summary.TotalSpansBaseline)
	assert.Equal(t, 0, resp.Summary.TotalSpansCandidate)
	assert.Equal(t, 0, resp.Summary.TotalSemanticBaseline)
	assert.Equal(t, 0, resp.Summary.TotalSemanticCandidate)
}

func TestGetSessionCompare_ReportsCatchingUpProjectionStateWhenHistoryCheckpointIsStale(t *testing.T) {
	ctx, platformStore, engineQueries, server, projectID := setupEngineHandlerTest(t)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	sessionKey := "compare-engine-stale-" + uuid.NewString()[:8]
	baselineStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "compare-baseline-" + uuid.NewString()[:8],
		RequestKey:        "req-compare-baseline-" + uuid.NewString()[:8],
		Session: &EngineStartSession{
			Key: testutil.StrPtr(sessionKey),
		},
	}))
	candidateStart := decodeJSONBody[EngineStartRunResponse](t, invokeStartEngineRun(t, server, projectID, EngineStartRunRequest{
		DefinitionName:    "checkout",
		DefinitionVersion: "v1",
		InstanceKey:       "compare-candidate-" + uuid.NewString()[:8],
		RequestKey:        "req-compare-candidate-" + uuid.NewString()[:8],
		Session: &EngineStartSession{
			Key: testutil.StrPtr(sessionKey),
		},
	}))

	baselineTrace, err := platformStore.Queries().GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   baselineStart.TraceId,
	})
	require.NoError(t, err)
	candidateTrace, err := platformStore.Queries().GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   candidateStart.TraceId,
	})
	require.NoError(t, err)
	require.True(t, baselineTrace.SessionID.Valid)
	require.Equal(t, baselineTrace.SessionID.Bytes, candidateTrace.SessionID.Bytes)

	baselineRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        baselineStart.RunId,
	})
	require.NoError(t, err)
	candidateRun, err := engineQueries.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        candidateStart.RunId,
	})
	require.NoError(t, err)

	appendEngineHistoryEvent(t, ctx, engineQueries, projectID, baselineRun.InstanceID, baselineRun.ID, 2, publichistory.EventCustomStatusUpdated, publichistory.CustomStatusUpdatedPayload{
		Status: []byte(`{"step":"baseline-stale"}`),
	})
	appendEngineHistoryEvent(t, ctx, engineQueries, projectID, candidateRun.InstanceID, candidateRun.ID, 2, publichistory.EventCustomStatusUpdated, publichistory.CustomStatusUpdatedPayload{
		Status: []byte(`{"step":"candidate-stale"}`),
	})

	endedAt := time.Now().UTC()
	_, err = platformStore.Pool().Exec(ctx, `
		UPDATE traces
		SET status = 'completed',
		    end_time = $2,
		    updated_at = NOW()
		WHERE id IN ($1, $3)
	`, baselineTrace.ID, endedAt, candidateTrace.ID)
	require.NoError(t, err)

	rec := invokeGetSessionCompare(t, server, projectID, baselineTrace.SessionID.Bytes, baselineTrace.ID, candidateTrace.ID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[SessionCompareResponse](t, rec)
	require.NotNil(t, resp.Baseline.Engine)
	require.NotNil(t, resp.Candidate.Engine)
	assert.Equal(t, CatchingUp, resp.Baseline.Engine.ProjectionState)
	assert.Equal(t, CatchingUp, resp.Candidate.Engine.ProjectionState)
}

func invokeGetSessionCompare(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	sessionID uuid.UUID,
	baselineTraceID uuid.UUID,
	candidateTraceID uuid.UUID,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID.String()+"/compare", nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.GetSessionCompare(rec, req.WithContext(ctx), sessionID, GetSessionCompareParams{
		BaselineTraceId:  baselineTraceID,
		CandidateTraceId: candidateTraceID,
	})

	return rec
}

func createCompareTraceRecord(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	q *platform.Queries,
	projectID uuid.UUID,
	sessionID uuid.UUID,
	traceID string,
	name string,
	status string,
	startedAt time.Time,
	endedAt *time.Time,
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
	})
	require.NoError(t, err)

	require.NoError(t, q.UpdateTraceRollups(ctx, platform.UpdateTraceRollupsParams{
		ID:             trace.ID,
		TotalSpans:     testutil.Int32Ptr(0),
		TotalTokensIn:  0,
		TotalTokensOut: 0,
		TotalCost:      testutil.PgtypeNumericFromFloat64(0),
		ErrorCount:     testutil.Int32Ptr(0),
	}))

	_, err = pool.Exec(ctx, "UPDATE traces SET server_received_at = $2 WHERE id = $1", trace.ID, startedAt)
	require.NoError(t, err)

	return trace
}

func createCompareSpanRecord(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	spanID string,
	parentSpanID *string,
	name string,
	spanType string,
	status string,
	startedAt time.Time,
	endedAt *time.Time,
	statusMessage *string,
	model *string,
	sequence *int32,
	promptTokens int64,
	completionTokens int64,
) platform.Span {
	t.Helper()

	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID:        projectID,
		TraceID:          traceID,
		SpanID:           spanID,
		ParentSpanID:     parentSpanID,
		Name:             name,
		Type:             spanType,
		Status:           status,
		StatusMessage:    statusMessage,
		Level:            "default",
		StartTime:        startedAt,
		EndTime:          testutil.PgtypeTimestamptzPtr(endedAt),
		Model:            model,
		PromptTokens:     testutil.Int64Ptr(promptTokens),
		CompletionTokens: testutil.Int64Ptr(completionTokens),
		TotalCost:        testutil.PgtypeNumericFromFloat64(0),
		Sequence:         sequence,
	})
	require.NoError(t, err)

	return span
}

func createCompareSemanticEventRecord(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	spanID string,
	eventType string,
	eventAt time.Time,
	sequence *int32,
	message string,
	payload map[string]any,
) uuid.UUID {
	t.Helper()

	eventPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	eventID, err := q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID: projectID,
		TraceID:   traceID,
		SpanID:    spanID,
		EventType: eventType,
		Level:     "info",
		EventTs:   testutil.PgtypeTimestamptz(eventAt),
		Sequence:  sequence,
		Message:   testutil.StrPtr(message),
		Payload:   eventPayload,
	})
	require.NoError(t, err)

	return eventID
}

func int32PtrCompare(value int32) *int32 {
	return &value
}

func timePtr(ts time.Time) *time.Time {
	return &ts
}
