package ingest_test

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func newTestService(s *store.Store) *ingest.Service {
	return ingest.NewService(s, nil, ingest.NewProcessor(s, nil), nil)
}

func newAsyncTestService(t *testing.T, s *store.Store) *ingest.Service {
	t.Helper()

	client, err := jobs.NewClient(testutil.TestDB(t), s, ingest.NewProcessor(s, nil), enginecontrol.NewService(s), nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Stop(context.Background())
	})

	return ingest.NewService(s, client, ingest.NewProcessor(s, nil), nil)
}

func TestIngest_RejectsMissingBatchKey(t *testing.T) {
	svc := newTestService(nil)

	resp, err := svc.Ingest(context.Background(), uuid.New(), &ingest.IngestRequest{})
	require.Nil(t, resp)
	require.Error(t, err)

	var vErr *ingest.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Contains(t, vErr.Errors, "batch_key is required")
}

func TestIngest_RejectsInvalidBatchBeforeDBAccess(t *testing.T) {
	svc := newTestService(nil)

	resp, err := svc.Ingest(context.Background(), uuid.New(), &ingest.IngestRequest{
		BatchKey: "batch-1",
		Traces: []ingest.TraceInput{
			{TraceID: ""},
		},
		Spans: []ingest.SpanInput{
			{
				TraceID:   "",
				SpanID:    "",
				Name:      "",
				StartTime: time.Time{},
			},
		},
		Events: []ingest.EventInput{
			{
				TraceID: "",
				SpanID:  "",
			},
		},
	})

	require.Nil(t, resp)
	require.Error(t, err)

	var vErr *ingest.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Contains(t, vErr.Errors, "trace missing required field: trace_id")
	assert.Contains(t, vErr.Errors, "span[0] missing required field: trace_id")
	assert.Contains(t, vErr.Errors, "span[0] missing required field: span_id")
	assert.Contains(t, vErr.Errors, "span[0] missing required field: name")
	assert.Contains(t, vErr.Errors, "span[0] missing required field: start_time")
	assert.Contains(t, vErr.Errors, "event[0] missing required field: trace_id")
	assert.Contains(t, vErr.Errors, "event[0] missing required field: span_id")
}

func TestIngest_RejectsTotalTokensOnlySpan(t *testing.T) {
	svc := newTestService(nil)

	total := int64(123)
	resp, err := svc.Ingest(context.Background(), uuid.New(), &ingest.IngestRequest{
		BatchKey: "batch-1",
		Spans: []ingest.SpanInput{
			{
				TraceID:     "trace-1",
				SpanID:      "span-1",
				Name:        "span",
				StartTime:   time.Now(),
				TotalTokens: &total,
			},
		},
	})

	require.Nil(t, resp)
	require.Error(t, err)

	var vErr *ingest.ValidationError
	require.ErrorAs(t, err, &vErr)
	require.NotEmpty(t, vErr.Errors)
	assert.Contains(t, vErr.Errors[0], "unsupported token format")
}

func TestIngest_RejectsSyntheticTimelineEventTypes(t *testing.T) {
	svc := newTestService(nil)

	eventType := "span_started"
	resp, err := svc.Ingest(context.Background(), uuid.New(), &ingest.IngestRequest{
		BatchKey: "batch-1",
		Events: []ingest.EventInput{
			{
				TraceID:   "trace-1",
				SpanID:    "span-1",
				EventType: &eventType,
			},
		},
	})

	require.Nil(t, resp)
	require.Error(t, err)

	var vErr *ingest.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Contains(t, vErr.Errors, "event[0] invalid event_type: span_started")
}

func TestIngest_AcceptsSemanticEventTypes(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := newTestService(s)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.NewString()[:8]
	start := time.Now().UTC()
	stateChangeType := "state_change"
	decisionType := "decision"

	resp, err := svc.Ingest(ctx, projectID, &ingest.IngestRequest{
		BatchKey: "batch-" + uuid.NewString()[:8],
		Traces: []ingest.TraceInput{
			{
				TraceID:   traceID,
				Name:      testutil.StrPtr("semantic trace"),
				StartTime: &start,
			},
		},
		Spans: []ingest.SpanInput{
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				Name:      "semantic span",
				StartTime: start,
			},
		},
		Events: []ingest.EventInput{
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				EventType: &stateChangeType,
				Payload: map[string]any{
					"key":       "status",
					"old_value": "pending",
					"new_value": "running",
				},
			},
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				EventType: &decisionType,
				Payload: map[string]any{
					"question": "route request?",
					"chosen":   "fast-path",
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(2), resp.EventCount)

	trace, err := q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)

	events, err := q.ListSpanEventsByTrace(ctx, platform.ListSpanEventsByTraceParams{
		TraceID:         trace.ID,
		ProjectFilterID: testutil.PgtypeUUID(projectID),
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.ElementsMatch(
		t,
		[]string{stateChangeType, decisionType},
		[]string{events[0].EventType, events[1].EventType},
	)
}

func TestIngest_WarnsForMissingSemanticEventFieldsButStillAccepts(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := newTestService(s)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.NewString()[:8]
	start := time.Now().UTC()
	stateChangeType := "state_change"
	decisionType := "decision"

	var logBuffer bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&logBuffer)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})

	resp, err := svc.Ingest(ctx, projectID, &ingest.IngestRequest{
		BatchKey: "batch-" + uuid.NewString()[:8],
		Traces: []ingest.TraceInput{
			{
				TraceID:   traceID,
				Name:      testutil.StrPtr("warning trace"),
				StartTime: &start,
			},
		},
		Spans: []ingest.SpanInput{
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				Name:      "warning span",
				StartTime: start,
			},
		},
		Events: []ingest.EventInput{
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				EventType: &stateChangeType,
				Payload: map[string]any{
					"old_value": "pending",
					"new_value": "running",
				},
			},
			{
				TraceID:   traceID,
				SpanID:    "span-1",
				EventType: &decisionType,
				Payload: map[string]any{
					"question": "route request?",
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(2), resp.EventCount)

	trace, err := q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)

	events, err := q.ListSpanEventsByTrace(ctx, platform.ListSpanEventsByTraceParams{
		TraceID:         trace.ID,
		ProjectFilterID: testutil.PgtypeUUID(projectID),
	})
	require.NoError(t, err)
	require.Len(t, events, 2)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "[WARN] ingest state_change event missing semantic payload field 'key'")
	assert.Contains(t, logOutput, "[WARN] ingest decision event missing semantic payload fields")
}

func TestIngest_RejectsInvalidEventLevel(t *testing.T) {
	svc := newTestService(nil)

	level := "verbose"
	resp, err := svc.Ingest(context.Background(), uuid.New(), &ingest.IngestRequest{
		BatchKey: "batch-1",
		Events: []ingest.EventInput{
			{
				TraceID: "trace-1",
				SpanID:  "span-1",
				Level:   &level,
			},
		},
	})

	require.Nil(t, resp)
	require.Error(t, err)

	var vErr *ingest.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Contains(t, vErr.Errors, "event[0] invalid level: verbose")
}

func TestIngest_AcceptsNonUUIDSessionKey(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := newTestService(s)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.NewString()[:8]
	sessionKey := "checkout-flow-42"

	resp, err := svc.Ingest(ctx, projectID, &ingest.IngestRequest{
		BatchKey: "batch-" + uuid.NewString()[:8],
		Traces: []ingest.TraceInput{
			{
				TraceID:   traceID,
				SessionID: &sessionKey,
				Name:      testutil.StrPtr("session key trace"),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	trace, err := q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)
	require.True(t, trace.SessionID.Valid, "trace should be linked to session")

	session, err := q.GetSession(ctx, trace.SessionID.Bytes)
	require.NoError(t, err)
	assert.Equal(t, sessionKey, session.ExternalID)
}

func TestIngest_UUIDLookingSessionKeyIsTreatedAsExternalID(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := newTestService(s)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-" + uuid.NewString()[:8]
	sessionKey := uuid.NewString()

	resp, err := svc.Ingest(ctx, projectID, &ingest.IngestRequest{
		BatchKey: "batch-" + uuid.NewString()[:8],
		Traces: []ingest.TraceInput{
			{
				TraceID:   traceID,
				SessionID: &sessionKey,
				Name:      testutil.StrPtr("uuid-looking session key trace"),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	trace, err := q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)
	require.True(t, trace.SessionID.Valid, "trace should be linked to session")

	session, err := q.GetSession(ctx, trace.SessionID.Bytes)
	require.NoError(t, err)
	assert.Equal(t, sessionKey, session.ExternalID)
}

func TestAcceptAsync_StoresPayloadAndReturnsAccepted(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := newAsyncTestService(t, s)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	rawPayload := []byte(`{"batch_key":"batch-async","traces":[]}`)

	resp, err := svc.AcceptAsync(ctx, projectID, &ingest.IngestRequest{
		BatchKey: "batch-async",
	}, rawPayload)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEqual(t, uuid.Nil, resp.BatchID)

	batch, err := s.GetBatch(ctx, resp.BatchID)
	require.NoError(t, err)
	assert.Equal(t, "queued", batch.Status)

	payload, err := s.GetBatchPayload(ctx, resp.BatchID)
	require.NoError(t, err)
	assert.Equal(t, int32(len(rawPayload)), payload.ByteSize)

	decoded, err := ingest.DecompressPayload(payload.PayloadBytes)
	require.NoError(t, err)
	assert.Equal(t, rawPayload, decoded)
}

func TestAcceptAsync_DuplicateReturnsExistingBatchID(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := newAsyncTestService(t, s)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	req := &ingest.IngestRequest{BatchKey: "batch-dup-async"}
	rawPayload := []byte(`{"batch_key":"batch-dup-async"}`)

	first, err := svc.AcceptAsync(ctx, projectID, req, rawPayload)
	require.NoError(t, err)

	second, err := svc.AcceptAsync(ctx, projectID, req, rawPayload)
	require.NoError(t, err)

	assert.Equal(t, "duplicate", second.Status)
	assert.Equal(t, first.BatchID, second.BatchID)

	payload, err := s.GetBatchPayload(ctx, first.BatchID)
	require.NoError(t, err)
	decoded, err := ingest.DecompressPayload(payload.PayloadBytes)
	require.NoError(t, err)
	assert.Equal(t, rawPayload, decoded)
}
