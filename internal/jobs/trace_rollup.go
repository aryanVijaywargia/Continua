package jobs

import (
	"context"
	"errors"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/continua-ai/continua/internal/store"
)

// TraceRollupArgs contains the arguments for a trace rollup job.
type TraceRollupArgs struct {
	TraceID uuid.UUID `json:"trace_id"`
}

// Kind returns the job kind for River.
func (TraceRollupArgs) Kind() string {
	return "trace_rollup"
}

// InsertOpts returns River insert options with uniqueness configuration.
// Coalescing is enforced for active jobs of the same trace.
func (TraceRollupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			// Keep uniqueness scoped to "active" states so completed jobs
			// don't block re-enqueue of fresh rollups.
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
				rivertype.JobStateRetryable,
			},
		},
	}
}

// TraceRollupWorker processes trace rollup jobs.
type TraceRollupWorker struct {
	river.WorkerDefaults[TraceRollupArgs]
	store *store.Store
}

// NewTraceRollupWorker creates a new trace rollup worker.
// Used for testing purposes - in production the worker is created by NewClient.
func NewTraceRollupWorker(s *store.Store) *TraceRollupWorker {
	return &TraceRollupWorker{store: s}
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
			log.Printf("Warning: could not get trace version before rollup: %v", err)
		}

		// Compute and update rollups.
		if err := w.store.ComputeAndUpdateTraceRollups(ctx, traceID); err != nil {
			log.Printf("Error computing rollups for trace %s: %v", traceID, err)
			return err
		}

		// If versioning can't be checked, finish this pass and rely on future enqueue.
		if !versionTracked {
			return nil
		}

		versionAfter, err := w.store.GetTraceVersion(ctx, traceID)
		if err != nil {
			log.Printf("Warning: could not get trace version after rollup: %v", err)
			return nil
		}
		if versionAfter <= versionBefore {
			return nil
		}

		log.Printf("Trace %s modified during rollup (v%d -> v%d), rerunning in same job", traceID, versionBefore, versionAfter)
	}

	log.Printf("Trace %s changed repeatedly during rollup; deferring remaining updates to follow-up enqueue", traceID)
	return nil
}

// ProcessRollup computes and updates trace rollups.
// Exposed for direct testing without River job wrapper.
func (w *TraceRollupWorker) ProcessRollup(ctx context.Context, traceID uuid.UUID) error {
	if err := w.store.ComputeAndUpdateTraceRollups(ctx, traceID); err != nil {
		log.Printf("Error computing rollups for trace %s: %v", traceID, err)
		return err
	}
	return nil
}

// EnqueueRollup enqueues a rollup job for the given trace.
// Returns inserted=false when a unique duplicate was coalesced.
func EnqueueRollup(ctx context.Context, client *river.Client[pgx.Tx], traceID uuid.UUID) (bool, error) {
	if client == nil {
		return false, errors.New("river client is nil")
	}
	res, err := client.Insert(ctx, TraceRollupArgs{TraceID: traceID}, nil)
	if err != nil {
		return false, err
	}
	return !res.UniqueSkippedAsDuplicate, nil
}

// EnqueueRollupInTx enqueues a rollup job within an existing transaction.
// The job becomes visible only after the transaction commits.
// Returns inserted=false when a unique duplicate was coalesced.
func EnqueueRollupInTx(ctx context.Context, client *river.Client[pgx.Tx], tx pgx.Tx, traceID uuid.UUID) (bool, error) {
	if client == nil {
		return false, errors.New("river client is nil")
	}
	res, err := client.InsertTx(ctx, tx, TraceRollupArgs{TraceID: traceID}, nil)
	if err != nil {
		return false, err
	}
	return !res.UniqueSkippedAsDuplicate, nil
}
