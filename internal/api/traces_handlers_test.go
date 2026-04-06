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

func TestGetTraceEvents_PaginationHandlesMixedKnownAndUnknownEventTypes(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 7, 11, 30, 0, 0, time.UTC)
	trace := createTimelineTrace(ctx, t, q, projectID, "running", base, nil)

	createTimelineSpan(ctx, t, q, projectID, trace.ID, "alpha", "Alpha Span", "running", base.Add(time.Second), nil)
	createTimelineSpan(ctx, t, q, projectID, trace.ID, "beta", "Beta Span", "completed", base.Add(2*time.Second), testutil.Ptr(base.Add(5*time.Second)))

	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "alpha", "effect", "info",
		base.Add(1500*time.Millisecond), testutil.Int32Ptr(1), "effect emitted", map[string]any{"target": "cache"},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "alpha", "wait", "warning",
		base.Add(2500*time.Millisecond), testutil.Int32Ptr(2), "waiting", map[string]any{"reason": "network"},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "beta", "workflow_step", "info",
		base.Add(3*time.Second), testutil.Int32Ptr(3), "workflow step", map[string]any{"phase": "plan"},
	)

	full := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(
		t, server, projectID, trace.ID, GetTraceEventsParams{Limit: testutil.IntPtr(100)},
	))
	require.Len(t, full.Events, 6)

	seen := make(map[string]struct{}, len(full.Events))
	var after *string
	var tailCursor *string
	foundDowngradedUnknown := false

	for {
		page := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(
			t, server, projectID, trace.ID, GetTraceEventsParams{
				After: after,
				Limit: testutil.IntPtr(2),
			},
		))

		for _, event := range page.Events {
			if _, exists := seen[event.Id]; exists {
				t.Fatalf("duplicate event returned across pages: %s", event.Id)
			}
			seen[event.Id] = struct{}{}

			if event.EventType == TimelineEventTypeCustom {
				require.NotNil(t, event.Payload)
				assert.Equal(t, "workflow_step", (*event.Payload)[originalEventTypePayloadKey])
				foundDowngradedUnknown = true
			}
		}

		require.NotNil(t, page.PollCursor)
		tailCursor = page.PollCursor

		if !page.HasMore {
			break
		}

		require.NotNil(t, page.NextCursor)
		after = page.NextCursor
	}

	assert.Len(t, seen, len(full.Events))
	assert.True(t, foundDowngradedUnknown)
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
		ctx, t, q, projectID, trace.ID, "beta", "workflow_step", "info",
		base.Add(4*time.Second), testutil.Int32Ptr(4), "late workflow step", map[string]any{"phase": "ship"},
	)

	incrementalPoll := decodeJSONBody[TimelineResponse](t, invokeGetTraceEvents(
		t, server, projectID, trace.ID, GetTraceEventsParams{
			After: tailCursor,
			Limit: testutil.IntPtr(2),
		},
	))
	require.Len(t, incrementalPoll.Events, 1)
	assert.Equal(t, lateEventID.String(), incrementalPoll.Events[0].Id)
	assert.Equal(t, TimelineEventTypeCustom, incrementalPoll.Events[0].EventType)
	require.NotNil(t, incrementalPoll.Events[0].Payload)
	assert.Equal(
		t,
		"workflow_step",
		(*incrementalPoll.Events[0].Payload)[originalEventTypePayloadKey],
	)
	require.NotNil(t, incrementalPoll.PollCursor)
	assert.NotEqual(t, *tailCursor, *incrementalPoll.PollCursor)
}

func TestGetTraceEvents_EngineProjectedSemanticEventsUseExistingTimelineContract(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 7, 11, 45, 0, 0, time.UTC)
	trace := createTimelineTrace(ctx, t, q, projectID, "running", base, nil)
	runID := uuid.New()

	_, err := pool.Exec(ctx, `
		UPDATE traces
		SET engine_run_id = $2,
		    engine_definition_name = 'checkout',
		    engine_definition_version = 'v1',
		    engine_projection_state = 'up_to_date',
		    engine_projection_updated_at = NOW()
		WHERE id = $1
	`, trace.ID, runID)
	require.NoError(t, err)

	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "engine:root:"+runID.String(), "effect", "info",
		base.Add(time.Second), testutil.Int32Ptr(11), "scheduled activity", map[string]any{
			"effect_kind":              "activity",
			"has_external_side_effect": true,
			"effect_id":                "activity:send-email",
		},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "engine:root:"+runID.String(), "wait", "info",
		base.Add(2*time.Second), testutil.Int32Ptr(12), "waiting on activity", map[string]any{
			"wait_kind": "activity",
			"phase":     "entered",
			"wait_id":   "activity:send-email",
		},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "engine:root:"+runID.String(), "wait", "info",
		base.Add(3*time.Second), testutil.Int32Ptr(21), "timer entered", map[string]any{
			"wait_kind": "timer",
			"phase":     "entered",
			"wait_id":   "timer:reminder",
		},
	)
	insertTimelineEvent(
		ctx, t, q, projectID, trace.ID, "engine:root:"+runID.String(), "wait", "info",
		base.Add(4*time.Second), testutil.Int32Ptr(22), "timer fired", map[string]any{
			"wait_kind":  "timer",
			"phase":      "resolved",
			"wait_id":    "timer:reminder",
			"resolution": "fired",
		},
	)

	rec := invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[TimelineResponse](t, rec)
	require.Len(t, resp.Events, 4)
	require.NotNil(t, resp.Engine)
	assert.Equal(t, UpToDate, resp.Engine.ProjectionState)
	assert.Equal(t, []TimelineEventType{
		TimelineEventTypeEffect,
		TimelineEventTypeWait,
		TimelineEventTypeWait,
		TimelineEventTypeWait,
	}, []TimelineEventType{
		resp.Events[0].EventType,
		resp.Events[1].EventType,
		resp.Events[2].EventType,
		resp.Events[3].EventType,
	})
	require.NotNil(t, resp.Events[0].Payload)
	assert.Equal(t, "activity:send-email", (*resp.Events[0].Payload)["effect_id"])
	require.NotNil(t, resp.Events[1].Payload)
	assert.Equal(t, "activity", (*resp.Events[1].Payload)["wait_kind"])
	assert.Equal(t, "entered", (*resp.Events[1].Payload)["phase"])
	require.NotNil(t, resp.Events[3].Payload)
	assert.Equal(t, "fired", (*resp.Events[3].Payload)["resolution"])
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
	assert.Nil(t, resp.Engine)
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

func TestGetTrace_ReturnsTraceDetailFields(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	userID := "user@example.com"
	environment := "production"
	release := "v1.2.3"
	start := time.Date(2026, 3, 7, 14, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)

	trace := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID:   projectID,
		TraceID:     "external-trace-123",
		Name:        testutil.StrPtr("Debugger Detail Trace"),
		UserID:      &userID,
		Tags:        []string{"prod", "v2"},
		Environment: &environment,
		Release:     &release,
		Input:       []byte(`{"prompt":"hello"}`),
		Output:      []byte(`["done",false]`),
		Status:      "completed",
		StartTime:   testutil.PgtypeTimestamptz(start),
		EndTime:     testutil.PgtypeTimestamptz(end),
	})

	rec := invokeGetTrace(t, server, projectID, trace.ID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[TraceDetail](t, rec)
	require.NotNil(t, resp.TraceId)
	assert.Equal(t, "external-trace-123", *resp.TraceId)
	require.NotNil(t, resp.UserId)
	assert.Equal(t, userID, *resp.UserId)
	require.NotNil(t, resp.Tags)
	assert.Equal(t, []string{"prod", "v2"}, *resp.Tags)
	require.NotNil(t, resp.Environment)
	assert.Equal(t, environment, *resp.Environment)
	require.NotNil(t, resp.Release)
	assert.Equal(t, release, *resp.Release)
	assertJSONValue(t, resp.Input, `{"prompt":"hello"}`)
	assertJSONValue(t, resp.Output, `["done",false]`)
}

func TestGetTrace_JSONIsFlat(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	trace := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "flat-trace-123",
		Name:      testutil.StrPtr("Flat Trace"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)),
	})

	rec := invokeGetTrace(t, server, projectID, trace.ID)
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Contains(t, body, "id")
	assert.Contains(t, body, "trace_id")
	assert.NotContains(t, body, "trace")
}

func TestListSpansByTrace_ReturnsLLMContextAndTruncationFields(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	trace := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "span-trace-123",
		Name:      testutil.StrPtr("Span Trace"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 7, 16, 0, 0, 0, time.UTC)),
	})

	model := "gpt-4o"
	provider := "openai"
	inputTruncated := true
	outputTruncated := true
	inputSize := int64(524288)
	outputSize := int64(1048576)
	reason := "size_limit"

	upsertSpanRecord(ctx, t, q, platform.UpsertSpanParams{
		ProjectID:               projectID,
		TraceID:                 trace.ID,
		SpanID:                  "llm-span-1",
		Name:                    "LLM Span",
		Type:                    "llm",
		Status:                  "completed",
		Level:                   "default",
		StartTime:               time.Date(2026, 3, 7, 16, 0, 1, 0, time.UTC),
		EndTime:                 testutil.PgtypeTimestamptz(time.Date(2026, 3, 7, 16, 0, 2, 0, time.UTC)),
		Model:                   &model,
		Provider:                &provider,
		InputTruncated:          &inputTruncated,
		InputOriginalSizeBytes:  &inputSize,
		InputTruncationReason:   &reason,
		OutputTruncated:         &outputTruncated,
		OutputOriginalSizeBytes: &outputSize,
		OutputTruncationReason:  &reason,
		TotalCost:               testutil.PgtypeNumericFromFloat64(0),
	})

	rec := invokeListSpansByTrace(t, server, projectID, trace.ID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[SpanList](t, rec)
	require.Len(t, resp.Spans, 1)

	span := resp.Spans[0]
	require.NotNil(t, span.Model)
	assert.Equal(t, model, *span.Model)
	require.NotNil(t, span.Provider)
	assert.Equal(t, provider, *span.Provider)
	require.NotNil(t, span.InputTruncated)
	assert.True(t, *span.InputTruncated)
	require.NotNil(t, span.InputOriginalSizeBytes)
	assert.Equal(t, inputSize, *span.InputOriginalSizeBytes)
	require.NotNil(t, span.InputTruncationReason)
	assert.Equal(t, reason, *span.InputTruncationReason)
	require.NotNil(t, span.OutputTruncated)
	assert.True(t, *span.OutputTruncated)
	require.NotNil(t, span.OutputOriginalSizeBytes)
	assert.Equal(t, outputSize, *span.OutputOriginalSizeBytes)
	require.NotNil(t, span.OutputTruncationReason)
	assert.Equal(t, reason, *span.OutputTruncationReason)
}

func TestListTraces_OmitsDetailFields(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	userID := "user@example.com"
	environment := "production"
	release := "v1.2.3"

	upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID:   projectID,
		TraceID:     "summary-trace-123",
		Name:        testutil.StrPtr("Summary Trace"),
		UserID:      &userID,
		Tags:        []string{"prod"},
		Environment: &environment,
		Release:     &release,
		Input:       []byte(`{"prompt":"hello"}`),
		Output:      []byte(`{"result":"ok"}`),
		Status:      "completed",
		StartTime:   testutil.PgtypeTimestamptz(time.Date(2026, 3, 7, 17, 0, 0, 0, time.UTC)),
	})

	rec := invokeListTraces(t, server, projectID, ListTracesParams{})
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Traces []map[string]json.RawMessage `json:"traces"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Traces, 1)

	for _, key := range []string{"trace_id", "user_id", "tags", "environment", "release", "input", "output"} {
		assert.NotContains(t, body.Traces[0], key)
	}
}

func TestListTracesAndGetTrace_IncludeSessionExternalID(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session, err := q.CreateSession(ctx, platform.CreateSessionParams{
		ProjectID:  projectID,
		ExternalID: "conv-abc-123",
	})
	require.NoError(t, err)

	withSession := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		SessionID: testutil.PgtypeUUID(session.ID),
		TraceID:   "trace-with-session",
		Name:      testutil.StrPtr("Trace With Session"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC)),
	})

	withoutSession := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-without-session",
		Name:      testutil.StrPtr("Trace Without Session"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)),
	})

	listRec := invokeListTraces(t, server, projectID, ListTracesParams{})
	require.Equal(t, http.StatusOK, listRec.Code)

	listResp := decodeJSONBody[TraceList](t, listRec)
	withSessionAPI := findTraceByID(t, listResp.Traces, withSession.ID)
	withoutSessionAPI := findTraceByID(t, listResp.Traces, withoutSession.ID)

	require.NotNil(t, withSessionAPI.SessionExternalId)
	assert.Equal(t, session.ExternalID, *withSessionAPI.SessionExternalId)
	assert.Nil(t, withoutSessionAPI.SessionExternalId)

	detailRec := invokeGetTrace(t, server, projectID, withSession.ID)
	require.Equal(t, http.StatusOK, detailRec.Code)

	detailResp := decodeJSONBody[TraceDetail](t, detailRec)
	require.NotNil(t, detailResp.SessionExternalId)
	assert.Equal(t, session.ExternalID, *detailResp.SessionExternalId)
}

func TestListTraces_SortDirectionAndDefaultOrdering(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	earliest := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-earliest",
		Name:      testutil.StrPtr("Earliest"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC)),
	})
	middle := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-middle",
		Name:      testutil.StrPtr("Middle"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)),
	})
	latest := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-latest",
		Name:      testutil.StrPtr("Latest"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 8, 11, 0, 0, 0, time.UTC)),
	})

	defaultRec := invokeListTraces(t, server, projectID, ListTracesParams{})
	require.Equal(t, http.StatusOK, defaultRec.Code)
	defaultResp := decodeJSONBody[TraceList](t, defaultRec)
	assert.Equal(t, []uuid.UUID{latest.ID, middle.ID, earliest.ID}, apiTraceIDs(defaultResp.Traces))

	ascRec := invokeListTraces(t, server, projectID, ListTracesParams{
		SortBy:  testutil.Ptr(StartedAt),
		SortDir: testutil.Ptr(ListTracesParamsSortDirAsc),
	})
	require.Equal(t, http.StatusOK, ascRec.Code)
	ascResp := decodeJSONBody[TraceList](t, ascRec)
	assert.Equal(t, []uuid.UUID{earliest.ID, middle.ID, latest.ID}, apiTraceIDs(ascResp.Traces))

	descRec := invokeListTraces(t, server, projectID, ListTracesParams{
		SortBy:  testutil.Ptr(StartedAt),
		SortDir: testutil.Ptr(ListTracesParamsSortDirDesc),
	})
	require.Equal(t, http.StatusOK, descRec.Code)
	descResp := decodeJSONBody[TraceList](t, descRec)
	assert.Equal(t, []uuid.UUID{latest.ID, middle.ID, earliest.ID}, apiTraceIDs(descResp.Traces))
}

func TestListTraces_SearchOverridesSortParams(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	base := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)

	nameMatch := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-name-match",
		Name:      testutil.StrPtr("checkout"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(base),
	})

	spanOnlyMatch := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "trace-span-only",
		Name:      testutil.StrPtr("background work"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(base.Add(2 * time.Hour)),
	})

	upsertSpanRecord(ctx, t, q, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   spanOnlyMatch.ID,
		SpanID:    "span-checkout",
		Name:      "checkout",
		Type:      "tool",
		Status:    "completed",
		Level:     "default",
		StartTime: base.Add(2 * time.Hour),
		TotalCost: testutil.PgtypeNumericFromFloat64(0),
	})

	rec := invokeListTraces(t, server, projectID, ListTracesParams{
		Q:       testutil.StrPtr("checkout"),
		SortBy:  testutil.Ptr(StartedAt),
		SortDir: testutil.Ptr(ListTracesParamsSortDirAsc),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[TraceList](t, rec)
	require.Len(t, resp.Traces, 2)
	assert.Equal(t, []uuid.UUID{nameMatch.ID, spanOnlyMatch.ID}, apiTraceIDs(resp.Traces))
}

func TestListTraces_InvalidEngineFiltersReturn400(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)

	projectID := testutil.CreateTestProject(t, ctx, s.Queries())
	invalidRunStatus := ListTracesParamsEngineRunStatus("not-a-status")
	invalidProjectionState := EngineProjectionState("not-a-state")

	runStatusRec := invokeListTraces(t, server, projectID, ListTracesParams{
		EngineRunStatus: &invalidRunStatus,
	})
	require.Equal(t, http.StatusBadRequest, runStatusRec.Code)
	assert.Equal(t, "invalid_request", decodeJSONBody[Error](t, runStatusRec).Code)

	projectionStateRec := invokeListTraces(t, server, projectID, ListTracesParams{
		EngineProjectionState: &invalidProjectionState,
	})
	require.Equal(t, http.StatusBadRequest, projectionStateRec.Code)
	assert.Equal(t, "invalid_request", decodeJSONBody[Error](t, projectionStateRec).Code)
}

func TestGetTrace_ProjectScopingReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	trace := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectBID,
		TraceID:   "scoped-trace-123",
		Name:      testutil.StrPtr("Scoped Trace"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)),
	})

	rec := invokeGetTrace(t, server, projectAID, trace.ID)
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}

func TestListSpansByTrace_ProjectScopingReturns404(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	server := NewServer(s, nil)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)

	trace := upsertTraceRecord(ctx, t, q, platform.UpsertTraceParams{
		ProjectID: projectBID,
		TraceID:   "scoped-span-trace-123",
		Name:      testutil.StrPtr("Scoped Span Trace"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(time.Date(2026, 3, 7, 19, 0, 0, 0, time.UTC)),
	})

	rec := invokeListSpansByTrace(t, server, projectAID, trace.ID)
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

func invokeGetTrace(t *testing.T, server *Server, projectID, traceID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.GetTrace(rec, req.WithContext(ctx), traceID)

	return rec
}

func invokeListSpansByTrace(t *testing.T, server *Server, projectID, traceID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID.String()+"/spans", nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.ListSpansByTrace(rec, req.WithContext(ctx), traceID)

	return rec
}

func invokeListTraces(t *testing.T, server *Server, projectID uuid.UUID, params ListTracesParams) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.ListTraces(rec, req.WithContext(ctx), params)

	return rec
}

func findTraceByID(t *testing.T, traces []Trace, traceID uuid.UUID) Trace {
	t.Helper()

	for i := range traces {
		if traces[i].Id == traceID {
			return traces[i]
		}
	}

	t.Fatalf("trace %s not found in response", traceID)
	return Trace{}
}

func apiTraceIDs(traces []Trace) []uuid.UUID {
	ids := make([]uuid.UUID, len(traces))
	for i := range traces {
		ids[i] = traces[i].Id
	}
	return ids
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

func upsertTraceRecord(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	params platform.UpsertTraceParams,
) platform.Trace {
	t.Helper()

	trace, err := q.UpsertTrace(ctx, params)
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

func upsertSpanRecord(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	params platform.UpsertSpanParams,
) platform.Span {
	t.Helper()

	span, err := q.UpsertSpan(ctx, params)
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
