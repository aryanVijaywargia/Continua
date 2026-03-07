package api

import (
	"context"
	"encoding/json"
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

func TestGetTraceEvents_MergesOrdersAndIncludesOrphans(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	trace := createTimelineTrace(ctx, t, q, projectID, "completed", base, testutil.Ptr(base.Add(6*time.Second)))

	createTimelineSpan(ctx, t, q, projectID, trace.ID, "alpha", "Alpha Span", "completed", base.Add(time.Second), testutil.Ptr(base.Add(2*time.Second)))
	createTimelineSpan(ctx, t, q, projectID, trace.ID, "beta", "Beta Span", "error", base.Add(4*time.Second), testutil.Ptr(base.Add(5*time.Second)))

	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "alpha", "message", "info",
		base.Add(time.Second), testutil.Int32Ptr(1), "alpha message", map[string]any{"step": "alpha-message"},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "alpha", "log", "info",
		base.Add(time.Second), testutil.Int32Ptr(5), "alpha log", map[string]any{"step": "alpha"},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "ghost", "error", "error",
		base.Add(3*time.Second), nil, "ghost error", map[string]any{"kind": "orphan"},
	)

	rec := invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[TimelineResponse](t, rec)
	require.Len(t, resp.Events, 7)
	assert.Equal(t, TimelineResponseTraceStatusCOMPLETED, resp.TraceStatus)
	assert.False(t, resp.HasMore)
	assert.Nil(t, resp.NextCursor)
	require.NotNil(t, resp.PollCursor)

	gotTypes := make([]TimelineEventType, len(resp.Events))
	for i := range resp.Events {
		gotTypes[i] = resp.Events[i].EventType
	}
	assert.Equal(t, []TimelineEventType{
		TimelineEventTypeMessage,
		TimelineEventTypeLog,
		TimelineEventTypeSpanStarted,
		TimelineEventTypeSpanCompleted,
		TimelineEventTypeError,
		TimelineEventTypeSpanStarted,
		TimelineEventTypeSpanFailed,
	}, gotTypes)

	require.NotNil(t, resp.Events[0].Sequence)
	require.NotNil(t, resp.Events[1].Sequence)
	assert.Equal(t, int32(1), *resp.Events[0].Sequence)
	assert.Equal(t, int32(5), *resp.Events[1].Sequence)
	assert.Equal(t, Explicit, resp.Events[0].Source, "explicit events should sort before synthetic events on timestamp ties")
	assert.Equal(t, Explicit, resp.Events[1].Source)
	assert.Equal(t, Synthetic, resp.Events[2].Source)
	require.NotNil(t, resp.Events[0].SpanName)
	assert.Equal(t, "Alpha Span", *resp.Events[0].SpanName)
	assert.Nil(t, resp.Events[4].SpanName, "orphan event should not receive a synthesized span name")
	require.NotNil(t, resp.Events[1].Payload)
	assert.Equal(t, "alpha", (*resp.Events[1].Payload)["step"])

	for i := 1; i < len(resp.Events); i++ {
		assert.False(
			t,
			resp.Events[i].Timestamp.Before(resp.Events[i-1].Timestamp),
			"timeline should be sorted by display timestamp",
		)
	}
}

func TestGetTraceEvents_CursorPaginationDoesNotDuplicate(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)
	trace := createTimelineTrace(ctx, t, q, projectID, "running", base, nil)

	createTimelineSpan(ctx, t, q, projectID, trace.ID, "alpha", "Alpha Span", "running", base.Add(time.Second), nil)
	createTimelineSpan(ctx, t, q, projectID, trace.ID, "beta", "Beta Span", "completed", base.Add(2*time.Second), testutil.Ptr(base.Add(4*time.Second)))

	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "alpha", "log", "info",
		base.Add(1500*time.Millisecond), testutil.Int32Ptr(1), "alpha info", map[string]any{"step": "one"},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "beta", "metric", "info",
		base.Add(3*time.Second), testutil.Int32Ptr(2), "beta metric", map[string]any{"value": 42},
	)

	full := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(
		t, server, projectID, trace.ID, GetTraceEventsParams{Limit: testutil.IntPtr(100)},
	))
	require.Len(t, full.Events, 5)
	assert.Equal(t, TimelineResponseTraceStatusRUNNING, full.TraceStatus)
	require.NotNil(t, full.PollCursor)

	seen := make(map[string]struct{}, len(full.Events))
	var after *string
	var tailCursor *string

	for {
		rec := invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{
			After: after,
			Limit: testutil.IntPtr(2),
		})
		require.Equal(t, http.StatusOK, rec.Code)

		page := decodeJSONBody[TimelineResponse](t, rec)
		assert.Equal(t, TimelineResponseTraceStatusRUNNING, page.TraceStatus)
		require.NotNil(t, page.PollCursor)
		tailCursor = page.PollCursor

		for _, event := range page.Events {
			if _, exists := seen[event.Id]; exists {
				t.Fatalf("duplicate event returned across pages: %s", event.Id)
			}
			seen[event.Id] = struct{}{}
		}

		if !page.HasMore {
			break
		}

		require.NotNil(t, page.NextCursor)
		after = page.NextCursor
	}

	assert.Len(t, seen, len(full.Events), "paged traversal should visit every event exactly once")
	require.NotNil(t, tailCursor)

	emptyPoll := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(
		t, server, projectID, trace.ID, GetTraceEventsParams{
			After: tailCursor,
			Limit: testutil.IntPtr(2),
		},
	))
	assert.Empty(t, emptyPoll.Events)
	assert.False(t, emptyPoll.HasMore)
	require.NotNil(t, emptyPoll.PollCursor)
	assert.Equal(t, *tailCursor, *emptyPoll.PollCursor)

	lateEventID := insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "alpha", "message", "info",
		base.Add(1500*time.Millisecond), testutil.Int32Ptr(0), "late message", map[string]any{"late": true},
	)

	incrementalPoll := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(
		t, server, projectID, trace.ID, GetTraceEventsParams{
			After: tailCursor,
			Limit: testutil.IntPtr(2),
		},
	))
	require.Len(t, incrementalPoll.Events, 1)
	assert.Equal(t, lateEventID.String(), incrementalPoll.Events[0].Id)
	require.NotNil(t, incrementalPoll.PollCursor)
	assert.NotEqual(t, *tailCursor, *incrementalPoll.PollCursor)
}

func TestGetTraceEvents_InvalidCursorReturns400(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	trace := createTimelineTrace(
		ctx,
		t,
		q,
		projectID,
		"running",
		time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
		nil,
	)

	rec := invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{
		After: testutil.StrPtr("not-a-valid-cursor"),
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "invalid_cursor", resp.Code)
}

func TestGetTraceEvents_EmptyTimeline(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	trace := createTimelineTrace(
		ctx,
		t,
		q,
		projectID,
		"running",
		time.Date(2026, 3, 7, 12, 30, 0, 0, time.UTC),
		nil,
	)

	rec := invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[TimelineResponse](t, rec)
	assert.Empty(t, resp.Events)
	assert.Equal(t, TimelineResponseTraceStatusRUNNING, resp.TraceStatus)
	assert.False(t, resp.HasMore)
	assert.Nil(t, resp.NextCursor)
	assert.Nil(t, resp.PollCursor)
}

func TestGetTraceEvents_ProjectScopingReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	trace := createTimelineTrace(
		ctx,
		t,
		q,
		projectBID,
		"running",
		time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC),
		nil,
	)

	rec := invokeGetTraceEvents(t, server, projectAID, trace.ID, GetTraceEventsParams{})
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}

func invokeGetTraceEvents(t *testing.T, server *Server, projectID, traceID uuid.UUID, params GetTraceEventsParams) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID.String()+"/events", nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.GetTraceEvents(rec, req.WithContext(ctx), traceID, params)

	return rec
}

func createTimelineTrace(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	status string,
	start time.Time,
	end *time.Time,
) platform.Trace {
	t.Helper()

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   testutil.UniqueID("timeline-trace"),
		Name:      testutil.StrPtr("Timeline Trace"),
		Status:    status,
		StartTime: testutil.PgtypeTimestamptz(start),
		EndTime:   testutil.PgtypeTimestamptzPtr(end),
	})
	require.NoError(t, err)

	return trace
}

func createTimelineSpan(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	spanID string,
	name string,
	status string,
	start time.Time,
	end *time.Time,
) platform.Span {
	t.Helper()

	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   traceID,
		SpanID:    spanID,
		Name:      name,
		Type:      "tool",
		Status:    status,
		Level:     "default",
		StartTime: start,
		EndTime:   testutil.PgtypeTimestamptzPtr(end),
		TotalCost: testutil.PgtypeNumericFromFloat64(0),
	})
	require.NoError(t, err)

	return span
}

func insertTimelineEvent(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	spanID string,
	eventType string,
	level string,
	eventTime time.Time,
	sequence *int32,
	message string,
	payload map[string]any,
) uuid.UUID {
	t.Helper()

	var payloadBytes []byte
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		require.NoError(t, err)
	}

	eventID, err := q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID: projectID,
		TraceID:   traceID,
		SpanID:    spanID,
		EventType: eventType,
		Level:     level,
		EventTs:   testutil.PgtypeTimestamptz(eventTime),
		Sequence:  sequence,
		Message:   testutil.StrPtr(message),
		Payload:   payloadBytes,
	})
	require.NoError(t, err)

	return eventID
}

func decodeJSONBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&value))
	return value
}
