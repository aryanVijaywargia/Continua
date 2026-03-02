package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/ingest"
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
