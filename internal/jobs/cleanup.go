package jobs

import (
	"context"
	"log"
	"time"

	"github.com/riverqueue/river"

	"github.com/continua-ai/continua/internal/jobargs"
	"github.com/continua-ai/continua/internal/store"
)

// CleanupWorker runs periodic cleanup for expired async ingest payloads.
type CleanupWorker struct {
	river.WorkerDefaults[jobargs.CleanupArgs]
	store            *store.Store
	payloadRetention time.Duration
}

// Timeout bounds cleanup work.
func (w *CleanupWorker) Timeout(*river.Job[jobargs.CleanupArgs]) time.Duration {
	return time.Minute
}

// Work deletes failed payload rows older than the configured retention window.
func (w *CleanupWorker) Work(ctx context.Context, _ *river.Job[jobargs.CleanupArgs]) error {
	startedAt := time.Now()
	cutoff := time.Now().Add(-w.payloadRetention)

	deletedBatchIDs, err := w.store.CleanupExpiredPayloads(ctx, cutoff)
	if err != nil {
		return err
	}

	log.Printf(
		"event=batch_cleanup_completed deleted_count=%d duration_ms=%d",
		len(deletedBatchIDs),
		time.Since(startedAt).Milliseconds(),
	)
	return nil
}
