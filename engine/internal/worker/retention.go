package worker

import (
	"context"
	"time"

	"github.com/continua-ai/continua/engine/internal/store"
)

// RetentionConfig configures terminal journal and request-dedupe retention.
type RetentionConfig struct {
	TerminalRuns time.Duration
	DedupeGrace  time.Duration
	BatchSize    int32
}

// RetentionWorker removes expired engine journal data.
type RetentionWorker struct {
	store *store.Store
	cfg   RetentionConfig
}

// NewRetentionWorker constructs a retention worker.
func NewRetentionWorker(engineStore *store.Store, cfg RetentionConfig) *RetentionWorker {
	return &RetentionWorker{store: engineStore, cfg: cfg}
}

// PollOnce reaps at most one configured batch from each retention class.
func (w *RetentionWorker) PollOnce(ctx context.Context, _ string) error {
	if w.cfg.BatchSize < 1 || (w.cfg.TerminalRuns <= 0 && w.cfg.DedupeGrace <= 0) {
		return nil
	}

	now := time.Now()
	metrics := w.store.Metrics()
	if w.cfg.DedupeGrace > 0 {
		reaped, err := w.store.ReapRequestDedupe(ctx, now.Add(-w.cfg.DedupeGrace), w.cfg.BatchSize)
		if err != nil {
			return err
		}
		metrics.AddRetentionReaped("request_dedupe", float64(reaped))
	}

	if w.cfg.TerminalRuns > 0 {
		runIDs, err := w.store.ListRetainableTerminalRunIDs(ctx, now.Add(-w.cfg.TerminalRuns), w.cfg.BatchSize)
		if err != nil {
			return err
		}
		for _, runID := range runIDs {
			counts, err := w.store.ReapTerminalRunJournal(ctx, runID)
			if err != nil {
				return err
			}
			metrics.AddRetentionReaped("history", float64(counts.History))
			metrics.AddRetentionReaped("inbox", float64(counts.Inbox))
			metrics.AddRetentionReaped("activity_tasks", float64(counts.ActivityTasks))
		}
	}

	return nil
}
