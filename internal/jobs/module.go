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

	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobargs"
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

// NewClient creates a new River client with the ingest, rollup, and cleanup workers registered.
func NewClient(
	pool *pgxpool.Pool,
	s *store.Store,
	processor *ingest.Processor,
	sharedControl *enginecontrol.Service,
	cfg *config.Config,
) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()

	ingestWorker := &IngestBatchWorker{store: s, processor: processor}
	rollupWorker := &TraceRollupWorker{store: s}
	cleanupWorker := &CleanupWorker{
		store:            s,
		payloadRetention: failedPayloadRetention(cfg),
	}
	river.AddWorker(workers, ingestWorker)
	river.AddWorker(workers, rollupWorker)
	river.AddWorker(workers, cleanupWorker)

	retentionPeriod := 7 * 24 * time.Hour
	periodicJobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(jobargs.CleanupInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				args := jobargs.CleanupArgs{}
				opts := args.InsertOpts()
				return args, &opts
			},
			&river.PeriodicJobOpts{
				ID:         "ingest-payload-cleanup",
				RunOnStart: true,
			},
		),
	}

	if retentionEnabled(cfg) {
		retentionWorker := NewRetentionWorker(
			s,
			sharedControl,
			projectionRetention(cfg),
			historyRetention(cfg),
		)
		river.AddWorker(workers, retentionWorker)
		periodicJobs = append(periodicJobs, river.NewPeriodicJob(
			river.PeriodicInterval(jobargs.RetentionInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				args := jobargs.RetentionArgs{}
				opts := args.InsertOpts()
				return args, &opts
			},
			&river.PeriodicJobOpts{
				ID:         "engine-retention-maintenance",
				RunOnStart: false,
			},
		))
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			jobargs.QueueIngest:      {MaxWorkers: queueWorkers(cfg, func(c *config.Config) int { return c.Jobs.IngestWorkers }, 4)},
			jobargs.QueueRollup:      {MaxWorkers: queueWorkers(cfg, func(c *config.Config) int { return c.Jobs.RollupWorkers }, 10)},
			jobargs.QueueMaintenance: {MaxWorkers: queueWorkers(cfg, func(c *config.Config) int { return c.Jobs.MaintenanceWorkers }, 1)},
			river.QueueDefault:       {MaxWorkers: queueWorkers(cfg, func(c *config.Config) int { return c.Jobs.DefaultWorkers }, 1)},
		},
		PeriodicJobs:                periodicJobs,
		Workers:                     workers,
		JobTimeout:                  5 * time.Minute,
		RescueStuckJobsAfter:        10 * time.Minute,
		CancelledJobRetentionPeriod: retentionPeriod,
		CompletedJobRetentionPeriod: retentionPeriod,
		DiscardedJobRetentionPeriod: retentionPeriod,
	})
	if err != nil {
		return nil, err
	}

	ingestWorker.client = client
	return client, nil
}

func queueWorkers(cfg *config.Config, getter func(*config.Config) int, fallback int) int {
	if cfg == nil {
		return fallback
	}
	return getter(cfg)
}

func failedPayloadRetention(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Ingest.FailedPayloadRetention <= 0 {
		return 7 * 24 * time.Hour
	}
	return cfg.Ingest.FailedPayloadRetention
}

func retentionEnabled(cfg *config.Config) bool {
	return projectionRetention(cfg) > 0 || historyRetention(cfg) > 0
}

func projectionRetention(cfg *config.Config) time.Duration {
	if cfg == nil {
		return 0
	}
	return cfg.Engine.ProjectionRetentionAfter
}

func historyRetention(cfg *config.Config) time.Duration {
	if cfg == nil {
		return 0
	}
	return cfg.Engine.HistoryRetentionAfter
}
