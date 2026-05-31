package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
	"github.com/continua-ai/continua/pkg/truncation"
)

type semanticHarness struct {
	ctx       context.Context
	q         *platform.Queries
	service   *Service
	projectID uuid.UUID
	traceID   string
	spanID    string
	startTime time.Time
}

func newSemanticHarness(t *testing.T) *semanticHarness {
	t.Helper()

	pool := testutil.TestDB(t)
	s := store.New(pool)
	ctx := context.Background()

	return &semanticHarness{
		ctx:       ctx,
		q:         s.Queries(),
		service:   NewService(s, nil, NewProcessor(s, nil), nil),
		projectID: testutil.CreateTestProject(t, ctx, s.Queries()),
		traceID:   testutil.UniqueID("semantic-trace"),
		spanID:    testutil.UniqueID("semantic-span"),
		startTime: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func (h *semanticHarness) ingestWithTraceAndSpan(t *testing.T, events ...EventInput) *IngestResponse {
	t.Helper()

	resp, err := h.service.Ingest(h.ctx, h.projectID, &IngestRequest{
		BatchKey: testutil.UniqueID("batch"),
		Traces: []TraceInput{
			{
				TraceID:   h.traceID,
				Name:      testutil.StrPtr("semantic trace"),
				StartTime: &h.startTime,
			},
		},
		Spans: []SpanInput{
			{
				TraceID:   h.traceID,
				SpanID:    h.spanID,
				Name:      "semantic span",
				StartTime: h.startTime,
			},
		},
		Events: events,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	return resp
}

func (h *semanticHarness) createTraceOnly(t *testing.T) uuid.UUID {
	t.Helper()

	trace, err := h.q.UpsertTrace(h.ctx, platform.UpsertTraceParams{
		ProjectID: h.projectID,
		TraceID:   h.traceID,
		Name:      testutil.StrPtr("semantic trace"),
		StartTime: testutil.PgtypeTimestamptz(h.startTime),
	})
	require.NoError(t, err)

	return trace.ID
}

func (h *semanticHarness) ingestEventsOnly(t *testing.T, events ...EventInput) *IngestResponse {
	t.Helper()

	resp, err := h.service.Ingest(h.ctx, h.projectID, &IngestRequest{
		BatchKey: testutil.UniqueID("batch"),
		Events:   events,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	return resp
}

func (h *semanticHarness) listEvents(t *testing.T) []platform.SpanEvent {
	t.Helper()

	trace, err := h.q.GetTraceByExternalID(h.ctx, platform.GetTraceByExternalIDParams{
		ProjectID: h.projectID,
		TraceID:   h.traceID,
	})
	require.NoError(t, err)

	events, err := h.q.ListSpanEventsByTrace(h.ctx, platform.ListSpanEventsByTraceParams{
		TraceID:         trace.ID,
		ProjectFilterID: testutil.PgtypeUUID(h.projectID),
	})
	require.NoError(t, err)

	return events
}

func decodeEventPayload(t *testing.T, event platform.SpanEvent) map[string]any {
	t.Helper()

	if len(event.Payload) == 0 {
		return nil
	}

	var payload map[string]any
	require.NoError(t, json.Unmarshal(event.Payload, &payload))
	return payload
}

func payloadsByEventType(t *testing.T, events []platform.SpanEvent) map[string][]map[string]any {
	t.Helper()

	grouped := make(map[string][]map[string]any, len(events))
	for i := range events {
		grouped[events[i].EventType] = append(grouped[events[i].EventType], decodeEventPayload(t, events[i]))
	}

	return grouped
}

func TestDeriveSemanticID_DeterministicForEffect(t *testing.T) {
	payload := map[string]any{
		"target": "tool-output",
		"nested": map[string]any{
			"step": "summarize",
		},
	}

	idOne := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "effect emitted", payload)
	idTwo := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "effect emitted", payload)

	require.Regexp(t, `^effect_[0-9a-f]{32}$`, idOne)
	assert.Equal(t, idOne, idTwo)
}

func TestDeriveSemanticID_DeterministicForWait(t *testing.T) {
	eventTS := time.Date(2026, 3, 7, 12, 0, 0, 123456000, time.UTC)

	idOne := deriveSemanticID("wait", "trace-1", "span-1", testutil.Int32Ptr(7), &eventTS, "warning", "waiting", map[string]any{
		"reason": "dependency",
	})
	idTwo := deriveSemanticID("wait", "trace-1", "span-1", testutil.Int32Ptr(7), &eventTS, "warning", "waiting", map[string]any{
		"reason": "dependency",
	})

	require.Regexp(t, `^wait_[0-9a-f]{32}$`, idOne)
	assert.Equal(t, idOne, idTwo)
}

func TestDeriveSemanticID_TreatsNilAndEmptyPayloadEqually(t *testing.T) {
	nilPayloadID := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "", nil)
	emptyPayloadID := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "", map[string]any{})

	assert.Equal(t, nilPayloadID, emptyPayloadID)
}

func TestDeriveSemanticID_DistinguishesOmittedAndExplicitInfoLevels(t *testing.T) {
	omittedLevelID := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "", "", map[string]any{
		"target": "cache",
	})
	explicitInfoLevelID := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "", map[string]any{
		"target": "cache",
	})

	assert.NotEqual(t, omittedLevelID, explicitInfoLevelID)
}

func TestDeriveSemanticID_RecursivelySortsNestedPayloadObjects(t *testing.T) {
	first := map[string]any{
		"root": map[string]any{
			"nested": map[string]any{
				"b": "two",
				"a": "one",
			},
			"list": []any{
				map[string]any{"z": 1, "y": 2},
			},
		},
	}
	second := map[string]any{
		"root": map[string]any{
			"list": []any{
				map[string]any{"y": 2, "z": 1},
			},
			"nested": map[string]any{
				"a": "one",
				"b": "two",
			},
		},
	}

	firstID := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "", first)
	secondID := deriveSemanticID("effect", "trace-1", "span-1", nil, nil, "info", "", second)

	assert.Equal(t, firstID, secondID)
}

func TestIngest_AcceptsEffectAndWaitEventTypes(t *testing.T) {
	h := newSemanticHarness(t)
	effectType := "effect"
	waitType := "wait"

	resp := h.ingestWithTraceAndSpan(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &effectType,
			Message:   testutil.StrPtr("effect event"),
			Payload:   map[string]any{"target": "cache"},
		},
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &waitType,
			Message:   testutil.StrPtr("wait event"),
			Payload:   map[string]any{"reason": "io"},
		},
	)

	assert.Equal(t, int32(2), resp.EventCount)

	events := h.listEvents(t)
	require.Len(t, events, 2)
	assert.ElementsMatch(t, []string{"effect", "wait"}, []string{events[0].EventType, events[1].EventType})
}

func TestIngest_PreservesProvidedSemanticIDs(t *testing.T) {
	h := newSemanticHarness(t)
	effectType := "effect"
	waitType := "wait"

	h.ingestWithTraceAndSpan(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &effectType,
			Payload: map[string]any{
				"effect_id": "effect_manual",
				"target":    "db",
			},
		},
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &waitType,
			Payload: map[string]any{
				"wait_id": "wait_manual",
				"reason":  "network",
			},
		},
	)

	events := h.listEvents(t)
	require.Len(t, events, 2)
	payloads := payloadsByEventType(t, events)

	require.Len(t, payloads["effect"], 1)
	require.Len(t, payloads["wait"], 1)
	assert.Equal(t, "effect_manual", payloads["effect"][0]["effect_id"])
	assert.Equal(t, "wait_manual", payloads["wait"][0]["wait_id"])
}

func TestIngest_DerivesSemanticIDsForEmptyAndNonStringReservedFields(t *testing.T) {
	h := newSemanticHarness(t)
	effectType := "effect"
	waitType := "wait"

	h.ingestWithTraceAndSpan(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &effectType,
			Payload: map[string]any{
				"effect_id": "",
				"target":    "db",
			},
		},
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &waitType,
			Payload: map[string]any{
				"wait_id": 123,
				"reason":  "dependency",
			},
		},
	)

	events := h.listEvents(t)
	require.Len(t, events, 2)
	payloads := payloadsByEventType(t, events)

	require.Len(t, payloads["effect"], 1)
	require.Len(t, payloads["wait"], 1)
	require.Regexp(t, `^effect_[0-9a-f]{32}$`, payloads["effect"][0]["effect_id"])
	require.Regexp(t, `^wait_[0-9a-f]{32}$`, payloads["wait"][0]["wait_id"])
}

func TestIngest_DerivesSemanticIDsWhenAbsent(t *testing.T) {
	h := newSemanticHarness(t)
	effectType := "effect"
	waitType := "wait"

	h.ingestWithTraceAndSpan(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &effectType,
			Message:   testutil.StrPtr("effect event"),
			Payload:   map[string]any{"target": "db"},
		},
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &effectType,
			Message:   testutil.StrPtr("effect event"),
			Payload:   map[string]any{"target": "db"},
		},
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &waitType,
			Message:   testutil.StrPtr("wait event"),
			Payload:   map[string]any{"reason": "dependency"},
		},
	)

	events := h.listEvents(t)
	require.Len(t, events, 3)
	payloads := payloadsByEventType(t, events)

	require.Len(t, payloads["effect"], 2)
	require.Len(t, payloads["wait"], 1)
	require.Regexp(t, `^effect_[0-9a-f]{32}$`, payloads["effect"][0]["effect_id"])
	require.Regexp(t, `^wait_[0-9a-f]{32}$`, payloads["wait"][0]["wait_id"])
	assert.Equal(t, payloads["effect"][0]["effect_id"], payloads["effect"][1]["effect_id"])
}

func TestIngest_DerivesSemanticIDBeforeTruncationAndPersistsIt(t *testing.T) {
	h := newSemanticHarness(t)
	effectType := "effect"
	largePayload := map[string]any{
		"blob": strings.Repeat("x", truncation.DefaultMaxBytes),
		"meta": map[string]any{
			"kind": "oversized",
		},
	}

	expectedID := deriveSemanticID("effect", h.traceID, h.spanID, nil, nil, "", "", largePayload)

	h.ingestWithTraceAndSpan(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &effectType,
			Payload:   largePayload,
		},
	)

	events := h.listEvents(t)
	require.Len(t, events, 1)
	require.NotNil(t, events[0].Truncated)
	assert.True(t, *events[0].Truncated)
	assert.LessOrEqual(t, len(events[0].Payload), truncation.DefaultMaxBytes)

	payload := decodeEventPayload(t, events[0])
	require.NotNil(t, payload)
	assert.Equal(t, expectedID, payload["effect_id"])
}

func TestIngest_StoresOrphanWaitEvent(t *testing.T) {
	h := newSemanticHarness(t)
	waitType := "wait"

	h.createTraceOnly(t)

	resp := h.ingestEventsOnly(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    "missing-span",
			EventType: &waitType,
			Payload:   map[string]any{"reason": "dependency"},
		},
	)

	assert.Equal(t, int32(1), resp.EventCount)

	events := h.listEvents(t)
	require.Len(t, events, 1)
	assert.Equal(t, "wait", events[0].EventType)
	assert.Equal(t, "missing-span", events[0].SpanID)
}

func TestIngest_AcceptsUnknownExplicitEventTypes(t *testing.T) {
	h := newSemanticHarness(t)
	workflowStepType := "workflow_step"

	resp := h.ingestWithTraceAndSpan(
		t,
		EventInput{
			TraceID:   h.traceID,
			SpanID:    h.spanID,
			EventType: &workflowStepType,
			Payload:   map[string]any{"step": "plan"},
		},
	)

	assert.Equal(t, int32(1), resp.EventCount)

	events := h.listEvents(t)
	require.Len(t, events, 1)
	assert.Equal(t, workflowStepType, events[0].EventType)
}

func TestIngest_RejectsSyntheticTimelineOnlyEventTypes(t *testing.T) {
	svc := NewService(nil, nil, NewProcessor(nil, nil), nil)

	for _, eventType := range []string{"span_started", "span_completed", "span_failed"} {
		t.Run(eventType, func(t *testing.T) {
			resp, err := svc.Ingest(context.Background(), uuid.New(), &IngestRequest{
				BatchKey: testutil.UniqueID("batch"),
				Events: []EventInput{
					{
						TraceID:   "trace-1",
						SpanID:    "span-1",
						EventType: &eventType,
					},
				},
			})

			require.Nil(t, resp)
			require.Error(t, err)

			var validationErr *ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Contains(t, validationErr.Errors, "event[0] invalid event_type: "+eventType)
		})
	}
}

func TestIngest_RejectsEmptyEventType(t *testing.T) {
	svc := NewService(nil, nil, NewProcessor(nil, nil), nil)
	emptyType := ""

	resp, err := svc.Ingest(context.Background(), uuid.New(), &IngestRequest{
		BatchKey: testutil.UniqueID("batch"),
		Events: []EventInput{
			{
				TraceID:   "trace-1",
				SpanID:    "span-1",
				EventType: &emptyType,
			},
		},
	})

	require.Nil(t, resp)
	require.Error(t, err)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Errors, "event[0] invalid event_type: ")
}
