package worker

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	"github.com/continua-ai/continua/engine/internal/store"
)

type MaintenanceWorker struct {
	store *store.Store
}

func NewMaintenanceWorker(store *store.Store) *MaintenanceWorker {
	return &MaintenanceWorker{store: store}
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
			if err := engineprojector.SyncProjectedRunSummary(ctx, tx.Tx(), wake.Run); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}

	_, err = w.store.ExpireRequestDedupe(ctx)
	return err
}
