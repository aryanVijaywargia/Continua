// Package jobs provides async job processing via River queue.
package jobs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"go.uber.org/fx"

	"github.com/continua-ai/continua/internal/store"
)

// Module provides the jobs module for Fx dependency injection.
var Module = fx.Module("jobs",
	fx.Provide(NewClient),
	fx.Invoke(startWorker),
)

// startWorker starts the River worker when the application starts.
func startWorker(lc fx.Lifecycle, client *river.Client[pgx.Tx]) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Use a non-deadline context for the long-lived worker runtime.
			// Fx startup context is short-lived and can be canceled immediately
			// after startup, which causes River internals to reconnect-loop.
			return client.Start(context.Background())
		},
		OnStop: func(ctx context.Context) error {
			return client.Stop(ctx)
		},
	})
}

// NewClient creates a new River client with the trace rollup worker registered.
// The store is injected to allow the worker to compute rollups.
func NewClient(pool *pgxpool.Pool, s *store.Store) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()

	// Create the worker with the store for rollup computation
	worker := &TraceRollupWorker{store: s}
	river.AddWorker(workers, worker)

	// Job retention: keep completed/cancelled/discarded jobs for 7 days per spec
	retentionPeriod := 7 * 24 * time.Hour

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers:                     workers,
		JobTimeout:                  30 * time.Second,
		RescueStuckJobsAfter:        time.Hour,
		CancelledJobRetentionPeriod: retentionPeriod,
		CompletedJobRetentionPeriod: retentionPeriod,
		DiscardedJobRetentionPeriod: retentionPeriod,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}
