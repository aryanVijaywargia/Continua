package ingest_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// ProcessBatch used to build its affected-trace slice straight out of a map, whose
// iteration Go deliberately randomizes. Callers enqueue one rollup per id inside the
// same open transaction, so two concurrent batches touching an overlapping set of
// traces could take those insert locks in opposite orders and deadlock.
func TestProcessBatch_ReturnsTraceIDsInADeterministicOrder(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)

	projectID := testutil.CreateTestProject(t, ctx, s.Queries())
	processor := ingest.NewProcessor(s, nil)

	req := &ingest.IngestRequest{BatchKey: "lock-order-" + uuid.NewString()[:8]}
	for range 24 {
		traceID := "trace-" + uuid.NewString()
		req.Traces = append(req.Traces, ingest.TraceInput{TraceID: traceID})
		req.Spans = append(req.Spans, ingest.SpanInput{
			TraceID:   traceID,
			SpanID:    "span-" + uuid.NewString()[:8],
			Name:      "span",
			StartTime: time.Now().UTC(),
		})
	}

	tx, err := s.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := processor.ProcessBatch(ctx, tx, projectID, req)
	require.NoError(t, err)
	require.Len(t, result.TraceIDs, len(req.Traces))

	assert.True(t,
		slices.IsSortedFunc(result.TraceIDs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }),
		"affected trace ids must be returned in a stable order so concurrent batches lock rows the same way")
}
