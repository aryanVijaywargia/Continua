package ingest_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// Two batches that reach the same rows in opposite request orders must not deadlock.
// The schedule is forced, not raced: a blocker transaction holds row "m", the first
// batch (request order b, m, a) parks on it, the second batch (request order a, b)
// parks behind the first, and only then is "m" released. Request-order locking makes
// the first batch then wait on "a", which the second batch holds: a deadlock.
func TestProcessBatch_OppositeOrderBatchesDoNotDeadlock(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	processor := ingest.NewProcessor(s, nil)

	span := func(traceID string) ingest.SpanInput {
		return ingest.SpanInput{TraceID: traceID, SpanID: "span-" + uuid.NewString()[:8], Name: "op", StartTime: time.Now().UTC()}
	}
	traceInSession := func(sessionID string) ingest.TraceInput {
		return ingest.TraceInput{TraceID: "trace-" + uuid.NewString(), SessionID: &sessionID}
	}
	event := func(traceID string) ingest.EventInput {
		return ingest.EventInput{TraceID: traceID, SpanID: "span-" + uuid.NewString()[:8]}
	}
	// Session ids sort opposite to their traces' ids, so locking each session while
	// upserting its trace interleaves session and trace locks in request order.
	crossedSessions := func(p string, traceSuffixes ...string) *ingest.IngestRequest {
		sessionOf := map[string]string{"a": p + "z", "z": p + "a"}
		req := &ingest.IngestRequest{}
		for _, suffix := range traceSuffixes {
			trace := ingest.TraceInput{TraceID: p + suffix}
			if session, ok := sessionOf[suffix]; ok {
				trace.SessionID = &session
			}
			req.Traces = append(req.Traces, trace)
		}
		return req
	}

	tests := []struct {
		name    string
		lockSQL string
		seed    func(p string) *ingest.IngestRequest
		first   func(p string) *ingest.IngestRequest
		second  func(p string) *ingest.IngestRequest
	}{
		{
			name:    "same traces",
			lockSQL: `SELECT 1 FROM traces WHERE project_id = $1 AND trace_id = $2 FOR UPDATE`,
			seed: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{{TraceID: p + "a"}, {TraceID: p + "b"}, {TraceID: p + "m"}}}
			},
			first: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{{TraceID: p + "b"}, {TraceID: p + "m"}, {TraceID: p + "a"}}}
			},
			second: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{{TraceID: p + "a"}, {TraceID: p + "b"}}}
			},
		},
		{
			name:    "spans on existing traces",
			lockSQL: `SELECT 1 FROM traces WHERE project_id = $1 AND trace_id = $2 FOR UPDATE`,
			seed: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{{TraceID: p + "a"}, {TraceID: p + "b"}, {TraceID: p + "m"}}}
			},
			first: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Spans: []ingest.SpanInput{span(p + "b"), span(p + "m"), span(p + "a")}}
			},
			second: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Spans: []ingest.SpanInput{span(p + "a"), span(p + "b")}}
			},
		},
		{
			name:    "fresh traces in the same sessions",
			lockSQL: `SELECT 1 FROM sessions WHERE project_id = $1 AND external_id = $2 FOR UPDATE`,
			seed: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{traceInSession(p + "a"), traceInSession(p + "b"), traceInSession(p + "m")}}
			},
			first: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{traceInSession(p + "b"), traceInSession(p + "m"), traceInSession(p + "a")}}
			},
			second: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{traceInSession(p + "a"), traceInSession(p + "b")}}
			},
		},
		{
			name:    "event-only references against trace upserts",
			lockSQL: `SELECT 1 FROM traces WHERE project_id = $1 AND trace_id = $2 FOR UPDATE`,
			seed: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{{TraceID: p + "a"}, {TraceID: p + "b"}, {TraceID: p + "m"}}}
			},
			first: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Events: []ingest.EventInput{event(p + "b"), event(p + "m"), event(p + "a")}}
			},
			second: func(p string) *ingest.IngestRequest {
				return &ingest.IngestRequest{Traces: []ingest.TraceInput{{TraceID: p + "a"}, {TraceID: p + "b"}}}
			},
		},
		{
			name:    "session order crosses trace order",
			lockSQL: `SELECT 1 FROM traces WHERE project_id = $1 AND trace_id = $2 FOR UPDATE`,
			seed:    func(p string) *ingest.IngestRequest { return crossedSessions(p, "a", "m", "z") },
			first:   func(p string) *ingest.IngestRequest { return crossedSessions(p, "z", "m", "a") },
			second:  func(p string) *ingest.IngestRequest { return crossedSessions(p, "a", "z") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectID := testutil.CreateTestProject(t, ctx, s.Queries())
			prefix := "lock-" + uuid.NewString()[:8] + "-"

			_, err := runBatch(ctx, s, processor, projectID, tt.seed(prefix), nil)
			require.NoError(t, err)

			blocker, err := pool.Begin(ctx)
			require.NoError(t, err)
			defer func() { _ = blocker.Rollback(ctx) }()
			_, err = blocker.Exec(ctx, tt.lockSQL, projectID, prefix+"m")
			require.NoError(t, err)

			firstPID, firstErr := startBatch(ctx, s, processor, projectID, tt.first(prefix))
			waitForLockWait(t, pool, firstPID)
			secondPID, secondErr := startBatch(ctx, s, processor, projectID, tt.second(prefix))
			waitForLockWait(t, pool, secondPID)

			require.NoError(t, blocker.Rollback(ctx))
			assert.NoError(t, <-firstErr, "first batch")
			assert.NoError(t, <-secondErr, "second batch")
		})
	}
}

// Twelve batches each mix sessions, trace upserts, spans, and event references, every
// one in a different request order, and all of them queue behind one held trace before
// it is released. Request-order locking turns that release into a chain of deadlocks;
// with one global order every batch must commit.
func TestProcessBatch_ManyConcurrentBatchesDoNotDeadlock(t *testing.T) {
	const batches = 12
	pool := testutil.TestDB(t)
	ctx := context.Background()
	// Each waiting batch holds a connection, more than the default pool size allows.
	s := store.New(newTestPool(t, pool, batches+1))
	processor := ingest.NewProcessor(s, nil)
	projectID := testutil.CreateTestProject(t, ctx, s.Queries())
	p := "many-" + uuid.NewString()[:8] + "-"

	keys := []string{"a", "b", "c", "d", "e", "f"}
	// Trace keys[i] belongs to session keys[5-i], so session order runs against trace order.
	trace := func(i int) ingest.TraceInput {
		session := p + "session-" + keys[len(keys)-1-i]
		return ingest.TraceInput{TraceID: p + keys[i], SessionID: &session}
	}
	seed := &ingest.IngestRequest{}
	for i := range keys {
		seed.Traces = append(seed.Traces, trace(i))
	}
	_, err := runBatch(ctx, s, processor, projectID, seed, nil)
	require.NoError(t, err)

	blocker, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback(ctx) }()
	_, err = blocker.Exec(ctx, `SELECT 1 FROM traces WHERE project_id = $1 AND trace_id = $2 FOR UPDATE`, projectID, p+"c")
	require.NoError(t, err)

	errs := make([]<-chan error, batches)
	for b := range batches {
		// Batch b upserts three traces from rotation b, writes spans on the other three in
		// reverse, and logs an event on one of the spanned traces.
		req := &ingest.IngestRequest{}
		for j := range 3 {
			req.Traces = append(req.Traces, trace((b+j)%len(keys)))
			req.Spans = append(req.Spans, ingest.SpanInput{
				TraceID:   p + keys[(b+5-j)%len(keys)],
				SpanID:    "span-" + uuid.NewString()[:8],
				Name:      "op",
				StartTime: time.Now().UTC(),
			})
		}
		req.Events = append(req.Events, ingest.EventInput{TraceID: p + keys[(b+4)%len(keys)], SpanID: "span-event"})

		pid, batchErr := startBatch(ctx, s, processor, projectID, req)
		waitForLockWait(t, pool, pid)
		errs[b] = batchErr
	}

	require.NoError(t, blocker.Rollback(ctx))
	for b, batchErr := range errs {
		assert.NoError(t, <-batchErr, "batch %d", b)
	}
}

// newTestPool opens another pool on the test database, capped at maxConns connections.
func newTestPool(t *testing.T, pool *pgxpool.Pool, maxConns int32) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config()
	cfg.MaxConns = maxConns
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// startBatch runs one batch in its own goroutine and transaction. It returns once the
// transaction's backend pid is known, so the caller can watch that backend for a lock wait.
func startBatch(
	ctx context.Context,
	s *store.Store,
	processor *ingest.Processor,
	projectID uuid.UUID,
	req *ingest.IngestRequest,
) (int32, <-chan error) {
	pids := make(chan int32, 1)
	errs := make(chan error, 1)
	go func() {
		pid, err := runBatch(ctx, s, processor, projectID, req, pids)
		if err != nil && pid == 0 {
			pids <- 0
		}
		errs <- err
	}()
	return <-pids, errs
}

// runBatch processes req in one committed transaction. When pids is non-nil it receives
// the backend pid before any batch write runs.
func runBatch(
	ctx context.Context,
	s *store.Store,
	processor *ingest.Processor,
	projectID uuid.UUID,
	req *ingest.IngestRequest,
	pids chan<- int32,
) (int32, error) {
	tx, err := s.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pid int32
	if err := tx.Tx().QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		return 0, err
	}
	if pids != nil {
		pids <- pid
	}

	if _, err := processor.ProcessBatch(ctx, tx, projectID, req); err != nil {
		return pid, err
	}
	return pid, tx.Commit(ctx)
}

func waitForLockWait(t *testing.T, pool *pgxpool.Pool, pid int32) {
	t.Helper()
	require.NotZero(t, pid, "batch transaction failed before it reported its backend pid")
	require.Eventually(t, func() bool {
		var waiting bool
		err := pool.QueryRow(context.Background(),
			`SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1`, pid,
		).Scan(&waiting)
		return err == nil && waiting
	}, 10*time.Second, 5*time.Millisecond, "backend %d never waited on a row lock", pid)
}
