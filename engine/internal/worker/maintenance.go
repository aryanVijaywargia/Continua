package worker

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/engine/internal/store"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
)

type MaintenanceWorker struct {
	store *store.Store
}

func NewMaintenanceWorker(engineStore *store.Store) *MaintenanceWorker {
	return &MaintenanceWorker{store: engineStore}
}

func (w *MaintenanceWorker) PollOnce(ctx context.Context, _ string) error {
	runIDs, err := w.store.ListDueTimerRunIDs(ctx)
	if err != nil {
		return err
	}

	for _, runID := range runIDs {
		tx, err := w.store.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}

		wake, wakeErr := tx.WakeWaitingRun(ctx, runID)
		if wakeErr != nil && !errors.Is(wakeErr, store.ErrNotFound) {
			_ = tx.Rollback(ctx)
			return wakeErr
		}
		if wakeErr == nil {
			if err := publicprojection.NewWriter(tx.Tx()).SyncRunSummary(ctx, &wake.Run); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}

	if _, err = w.store.ExpireRequestDedupe(ctx); err != nil {
		return err
	}

	metrics := w.store.Metrics()
	if metrics == nil {
		return nil
	}
	snapshot, err := w.store.SampleRuntimeMetrics(ctx)
	if err != nil {
		return err
	}
	metrics.SetQueueDepth("runs_ready", snapshot.RunsReady)
	metrics.SetQueueDepth("activity_tasks_pending", snapshot.ActivityTasksPending)
	metrics.SetQueueDepth("inbox_pending", snapshot.InboxPending)
	metrics.SetProjectorLagRows(snapshot.ProjectorLagRows)
	return nil
}
