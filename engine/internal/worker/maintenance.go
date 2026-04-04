package worker

import (
	"context"
	"errors"

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
		_, wakeErr := w.store.WakeWaitingRun(ctx, runID)
		if wakeErr != nil && !errors.Is(wakeErr, store.ErrNotFound) {
			return wakeErr
		}
	}

	_, err = w.store.ExpireRequestDedupe(ctx)
	return err
}
