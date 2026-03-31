package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobs"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func newAsyncIngestServer(t *testing.T) (*Server, *store.Store, *platform.Queries, uuid.UUID) {
	t.Helper()

	pool := testutil.TestDB(t)
	s := store.New(pool)
	client, err := jobs.NewClient(pool, s, ingest.NewProcessor(s, nil), nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Stop(context.Background())
	})

	server := NewServer(s, ingest.NewService(s, client, ingest.NewProcessor(s, nil), nil))
	projectID := testutil.CreateTestProject(t, context.Background(), s.Queries())
	return server, s, s.Queries(), projectID
}

func invokeIngest(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	body string,
	params IngestParams,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.Ingest(rec, req.WithContext(ctx), params)
	return rec
}

func invokeGetBatchStatus(
	t *testing.T,
	server *Server,
	projectID uuid.UUID,
	batchID uuid.UUID,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/ingest/batches/"+batchID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	rec := httptest.NewRecorder()

	server.GetBatchStatus(rec, req.WithContext(ctx), batchID)
	return rec
}

func TestIngest_AsyncHeaderReturnsAcceptedWithoutInlineWrites(t *testing.T) {
	server, _, q, projectID := newAsyncIngestServer(t)
	traceID := "trace-" + uuid.NewString()[:8]

	rec := invokeIngest(
		t,
		server,
		projectID,
		`{"batch_key":"batch-header","traces":[{"trace_id":"`+traceID+`","name":"async"}]}`,
		IngestParams{},
		map[string]string{"X-Continua-Async-Version": "2"},
	)

	require.Equal(t, http.StatusAccepted, rec.Code)
	resp := decodeJSONBody[IngestResponse](t, rec)
	assert.Equal(t, IngestResponseStatusAccepted, resp.Status)
	require.NotNil(t, resp.BatchId)
	assert.Nil(t, resp.TraceCount)
	assert.Nil(t, resp.AcceptedCount)

	_, err := q.GetTraceByExternalID(context.Background(), platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	assert.Error(t, err)
}

func TestIngest_LegacyAsyncStillProcessesInlineWithoutHeader(t *testing.T) {
	pool := testutil.TestDB(t)
	s := store.New(pool)
	server := NewServer(s, ingest.NewService(s, nil, ingest.NewProcessor(s, nil), nil))
	projectID := testutil.CreateTestProject(t, context.Background(), s.Queries())
	traceID := "trace-" + uuid.NewString()[:8]

	rec := invokeIngest(
		t,
		server,
		projectID,
		`{"batch_key":"batch-legacy","traces":[{"trace_id":"`+traceID+`","name":"legacy"}]}`,
		IngestParams{},
		nil,
	)

	require.Equal(t, http.StatusAccepted, rec.Code)
	resp := decodeJSONBody[IngestResponse](t, rec)
	assert.Equal(t, IngestResponseStatusAccepted, resp.Status)
	require.NotNil(t, resp.BatchId)
	assert.Nil(t, resp.TraceCount)

	trace, err := s.Queries().GetTraceByExternalID(context.Background(), platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)
	assert.Equal(t, traceID, trace.TraceID)
}

func TestIngest_SyncTrueOverridesAsyncHeader(t *testing.T) {
	server, _, q, projectID := newAsyncIngestServer(t)
	traceID := "trace-" + uuid.NewString()[:8]
	sync := true

	rec := invokeIngest(
		t,
		server,
		projectID,
		`{"batch_key":"batch-sync","traces":[{"trace_id":"`+traceID+`","name":"sync"}]}`,
		IngestParams{Sync: &sync},
		map[string]string{"X-Continua-Async-Version": "2"},
	)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[IngestResponse](t, rec)
	assert.Equal(t, IngestResponseStatusOk, resp.Status)
	require.NotNil(t, resp.BatchId)
	require.NotNil(t, resp.TraceCount)
	assert.Equal(t, int32(1), *resp.TraceCount)

	trace, err := q.GetTraceByExternalID(context.Background(), platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)
	assert.Equal(t, traceID, trace.TraceID)
}

func TestIngest_SyncTrueAcceptsUnknownExplicitEventType(t *testing.T) {
	server, _, q, projectID := newAsyncIngestServer(t)
	traceID := "trace-" + uuid.NewString()[:8]
	startTime := time.Now().UTC().Format(time.RFC3339Nano)
	sync := true

	rec := invokeIngest(
		t,
		server,
		projectID,
		`{
			"batch_key":"batch-unknown-explicit",
			"traces":[{"trace_id":"`+traceID+`","name":"unknown explicit trace"}],
			"spans":[{"trace_id":"`+traceID+`","span_id":"span-1","name":"unknown explicit span","start_time":"`+startTime+`"}],
			"events":[{"trace_id":"`+traceID+`","span_id":"span-1","event_type":"workflow_step","message":"planning"}]
		}`,
		IngestParams{Sync: &sync},
		nil,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[IngestResponse](t, rec)
	assert.Equal(t, IngestResponseStatusOk, resp.Status)
	require.NotNil(t, resp.EventCount)
	assert.Equal(t, int32(1), *resp.EventCount)

	trace, err := q.GetTraceByExternalID(context.Background(), platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)

	events, err := q.ListSpanEventsByTrace(context.Background(), trace.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "workflow_step", events[0].EventType)
}

func TestIngest_SyncSnapshotMarkerRoundTripsThroughTimeline(t *testing.T) {
	server, _, q, projectID := newAsyncIngestServer(t)
	traceID := "trace-" + uuid.NewString()[:8]
	startTime := time.Now().UTC().Format(time.RFC3339Nano)
	sync := true

	rec := invokeIngest(
		t,
		server,
		projectID,
		`{
			"batch_key":"batch-snapshot-marker",
			"traces":[{"trace_id":"`+traceID+`","name":"snapshot marker trace"}],
			"spans":[{"trace_id":"`+traceID+`","span_id":"span-1","name":"snapshot marker span","start_time":"`+startTime+`"}],
			"events":[{"trace_id":"`+traceID+`","span_id":"span-1","event_type":"snapshot_marker","payload":{}}]
		}`,
		IngestParams{Sync: &sync},
		nil,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSONBody[IngestResponse](t, rec)
	assert.Equal(t, IngestResponseStatusOk, resp.Status)
	require.NotNil(t, resp.EventCount)
	assert.Equal(t, int32(1), *resp.EventCount)

	trace, err := q.GetTraceByExternalID(context.Background(), platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)

	timelineRec := invokeGetTraceEvents(t, server, projectID, trace.ID, GetTraceEventsParams{
		Limit: testutil.IntPtr(100),
	})
	require.Equal(t, http.StatusOK, timelineRec.Code)

	timelineResp := decodeJSONBody[TimelineResponse](t, timelineRec)
	snapshotEvents := make([]TimelineEvent, 0, len(timelineResp.Events))
	for _, event := range timelineResp.Events {
		if event.EventType == TimelineEventTypeSnapshotMarker {
			snapshotEvents = append(snapshotEvents, event)
		}
	}

	require.Len(t, snapshotEvents, 1)
	assert.Equal(t, Explicit, snapshotEvents[0].Source)
	assert.Equal(t, "span-1", *snapshotEvents[0].SpanId)
	assert.NotNil(t, snapshotEvents[0].Payload)
	assert.Empty(t, *snapshotEvents[0].Payload)
}

func TestIngest_InvalidAsyncVersionReturnsBadRequest(t *testing.T) {
	server, _, _, projectID := newAsyncIngestServer(t)

	rec := invokeIngest(
		t,
		server,
		projectID,
		`{"batch_key":"batch-invalid"}`,
		IngestParams{},
		map[string]string{"X-Continua-Async-Version": "3"},
	)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "unsupported_async_version", resp.Code)
}

func TestGetBatchStatus_ReturnsProcessing(t *testing.T) {
	server, s, q, projectID := newAsyncIngestServer(t)
	batchID, err := q.ClaimBatch(context.Background(), platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  "batch-processing-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)

	_, err = s.MarkBatchProcessingIfQueued(context.Background(), batchID)
	require.NoError(t, err)

	rec := invokeGetBatchStatus(t, server, projectID, batchID)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSONBody[BatchStatusResponse](t, rec)
	assert.Equal(t, BatchStatusResponseStatusProcessing, resp.Status)
	assert.Equal(t, int32(1), resp.AttemptCount)
	require.NotNil(t, resp.ProcessingStartedAt)
}

func TestGetBatchStatus_ReturnsNotFoundForWrongProject(t *testing.T) {
	server, _, q, projectID := newAsyncIngestServer(t)
	otherProjectID := testutil.CreateTestProject(t, context.Background(), q)
	batchID, err := q.ClaimBatch(context.Background(), platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  "batch-scope-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)

	rec := invokeGetBatchStatus(t, server, otherProjectID, batchID)
	require.Equal(t, http.StatusNotFound, rec.Code)

	resp := decodeJSONBody[Error](t, rec)
	assert.Equal(t, "not_found", resp.Code)
}
