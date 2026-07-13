package workflow

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/continua-ai/continua/engine/internal/store"
)

type Worker struct {
	store       *store.Store
	activator   *Activator
	runLeaseTTL time.Duration
}

func NewWorker(engineStore *store.Store, definitions *Registry, runLeaseTTL time.Duration) *Worker {
	return &Worker{
		store:       engineStore,
		activator:   NewActivator(engineStore, definitions),
		runLeaseTTL: runLeaseTTL,
	}
}

func (w *Worker) PollOnce(ctx context.Context, workerID string) error {
	metrics := w.store.Metrics()
	run, err := w.store.ClaimNextRun(ctx, workerID, w.runLeaseTTL)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			metrics.IncClaim("run", "empty")
			return nil
		}
		return err
	}
	metrics.IncClaim("run", "claimed")
	if err := applyTestClaimHook(ctx); err != nil {
		return err
	}

	if err := w.activator.Activate(ctx, &run); err != nil {
		if errors.Is(err, store.ErrStaleClaim) {
			metrics.IncClaim("run", "stale")
			log.Printf("workflow worker stale claim for run %s", run.ID)
			return nil
		}
		return err
	}

	return nil
}
