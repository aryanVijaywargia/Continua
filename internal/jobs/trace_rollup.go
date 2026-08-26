package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
)

// TraceRollupArgs contains the arguments for a trace rollup job.
type TraceRollupArgs = jobargs.TraceRollupArgs

// TraceRollupWorker processes trace rollup jobs.
type TraceRollupWorker struct {
	river.WorkerDefaults[TraceRollupArgs]
	store *store.Store
}

// NewTraceRollupWorker creates a new trace rollup worker.
// Test helper: production wiring in jobs.NewClient builds the struct directly.
func NewTraceRollupWorker(s *store.Store) *TraceRollupWorker {
	return &TraceRollupWorker{store: s}
}

// Timeout bounds rollup execution time.
func (w *TraceRollupWorker) Timeout(*river.Job[TraceRollupArgs]) time.Duration {
	return 30 * time.Second
}

// Work processes a trace rollup job by computing and updating trace aggregates.
// It re-runs in-place if new spans arrive while processing so updates aren't lost
// when uniqueness coalesces duplicate enqueues.
func (w *TraceRollupWorker) Work(ctx context.Context, job *river.Job[TraceRollupArgs]) error {
	traceID := job.Args.TraceID
	const maxRollupLoops = 3

	for iter := 0; iter < maxRollupLoops; iter++ {
		// Get trace version before rollup.
		versionBefore, err := w.store.GetTraceVersion(ctx, traceID)
		versionTracked := err == nil && versionBefore > 0
		if err != nil {
			slog.Warn("could not get trace version before rollup",
				"trace_id", traceID,
				"err", err,
			)
		}

		// Compute and update rollups.
		if err := w.store.ComputeAndUpdateTraceRollups(ctx, traceID); err != nil {
			// River logs returned job errors below the default logger's threshold.
			slog.Error("trace_rollup_failed",
				"trace_id", traceID,
				"err", err,
			)
			return err
		}

		// If versioning can't be checked, finish this pass and rely on future enqueue.
		if !versionTracked {
			return nil
		}

		versionAfter, err := w.store.GetTraceVersion(ctx, traceID)
		if err != nil {
			slog.Warn("could not get trace version after rollup",
				"trace_id", traceID,
				"err", err,
			)
			return nil
		}
		if versionAfter <= versionBefore {
			return nil
		}

		slog.Info("trace modified during rollup, rerunning in same job",
			"trace_id", traceID,
			"version_before", versionBefore,
			"version_after", versionAfter,
		)
	}

	slog.Info("trace changed repeatedly during rollup, deferring remaining updates to follow-up enqueue",
		"trace_id", traceID,
	)
	return nil
}

// ProcessRollup computes and updates trace rollups.
// Exposed for direct testing without River job wrapper.
func (w *TraceRollupWorker) ProcessRollup(ctx context.Context, traceID uuid.UUID) error {
	return w.store.ComputeAndUpdateTraceRollups(ctx, traceID)
}

// EnqueueRollupInTx enqueues a rollup job within an existing transaction.
// The job becomes visible only after the transaction commits.
// Returns inserted=false when a unique duplicate was coalesced.
func EnqueueRollupInTx(ctx context.Context, client *river.Client[pgx.Tx], tx pgx.Tx, traceID uuid.UUID) (bool, error) {
	if client == nil {
		return false, errors.New("river client is nil")
	}
	res, err := client.InsertTx(ctx, tx, jobargs.TraceRollupArgs{TraceID: traceID}, nil)
	if err != nil {
		return false, err
	}
	return !res.UniqueSkippedAsDuplicate, nil
}
