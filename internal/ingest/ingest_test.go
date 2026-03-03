package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestIngest_RejectsMissingBatchKey(t *testing.T) {
	svc := ingest.NewService(nil, nil)

	resp, err := svc.Ingest(context.Background(), uuid.New(), &ingest.IngestRequest{})
	require.Nil(t, resp)
	require.Error(t, err)

	var vErr *ingest.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Contains(t, vErr.Errors, "batch_key is required")
}

func TestIngest_RejectsInvalidBatchBeforeDBAccess(t *testing.T) {
	svc := ingest.NewService(nil, nil)

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
	svc := ingest.NewService(nil, nil)

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

func TestIngest_AcceptsNonUUIDSessionKey(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	svc := ingest.NewService(s, nil)
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
	svc := ingest.NewService(s, nil)
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
