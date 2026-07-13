// Package runtime embeds the Continua durable-execution engine as a library:
// user programs register workflow definitions and activity handlers, then run
// the engine workers against Postgres.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"golang.org/x/sync/errgroup"

	"github.com/continua-ai/continua/engine/internal/activity"
	"github.com/continua-ai/continua/engine/internal/catalog"
	"github.com/continua-ai/continua/engine/internal/config"
	enginemetrics "github.com/continua-ai/continua/engine/internal/metrics"
	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	engineworker "github.com/continua-ai/continua/engine/internal/worker"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

// ActivityHandler executes one activity invocation. It mirrors the engine's
// internal activity handler contract.
type ActivityHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

// Options configures an embedded engine runtime.
type Options struct {
	// DatabaseURL is the Postgres connection string. Required.
	DatabaseURL string
	// Workflows are the workflow definitions this runtime can execute.
	Workflows []workflow.Definition
	// Activities maps activity type to handler.
	Activities map[string]ActivityHandler
	// ProjectID optionally scopes all polling to a single project.
	ProjectID *uuid.UUID
	// Poll intervals and lease TTLs; zero values use the engine defaults.
	WorkflowPollInterval    time.Duration
	ActivityPollInterval    time.Duration
	MaintenancePollInterval time.Duration
	RunLeaseTTL             time.Duration
	ActivityLeaseTTL        time.Duration
	// MetricsRegistry receives engine Prometheus collectors when configured.
	MetricsRegistry prometheus.Registerer
	// MetricsAddr configures the Prometheus HTTP listen address when non-empty.
	MetricsAddr string
	// MetricsListener supplies a caller-owned listener for the Prometheus endpoint.
	MetricsListener net.Listener
}

// Runtime is an embedded engine instance built from Options.
type Runtime struct {
	options     Options
	definitions *engineworkflow.Registry
	activities  *activity.Registry
}

// New validates the options and builds the workflow/activity registries.
func New(opts Options) (*Runtime, error) { //nolint:gocritic // Keep the public constructor ergonomic for literal Options values.
	if opts.DatabaseURL == "" {
		return nil, errors.New("runtime: database URL is required")
	}

	definitions, err := engineworkflow.NewRegistry(opts.Workflows...)
	if err != nil {
		return nil, err
	}

	handlers := make(map[string]activity.Handler, len(opts.Activities))
	for activityType, handler := range opts.Activities {
		handlers[activityType] = activity.Handler(handler)
	}
	activities, err := activity.NewRegistry(handlers)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		options:     opts,
		definitions: definitions,
		activities:  activities,
	}, nil
}

// Run executes the engine workers until ctx is cancelled. It returns nil on
// graceful shutdown.
func (r *Runtime) Run(ctx context.Context) error {
	if r == nil {
		return errors.New("runtime: nil runtime")
	}
	if ctx == nil {
		return errors.New("runtime: context is required")
	}

	cfg := config.Defaults(r.options.DatabaseURL)
	applyRuntimeOverrides(cfg, &r.options)
	recorder, metricsGatherer, err := buildMetrics(&r.options)
	if err != nil {
		return err
	}

	pool, err := enginestore.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	store := enginestore.New(pool)
	if recorder != nil {
		store = store.WithMetrics(recorder)
	}
	defer func() {
		store.Close()
	}()
	if r.options.ProjectID != nil {
		exists, err := store.PlatformProjectExists(ctx, *r.options.ProjectID)
		if err != nil {
			return fmt.Errorf("runtime: validate project %s: %w", r.options.ProjectID.String(), err)
		}
		if !exists {
			return fmt.Errorf("runtime: project %s not found; create the project in the platform and set ENGINE_PROJECT_ID before starting the engine runtime", r.options.ProjectID.String())
		}
		store = store.WithProjectFilter(*r.options.ProjectID)
	}

	if err := catalog.PublishStoreDefinitions(ctx, store, r.definitions.List()); err != nil {
		return err
	}

	workflowWorker := engineworkflow.NewWorker(store, r.definitions, cfg.Runtime.RunLeaseTTL)
	activityWorker := activity.NewWorker(store, r.activities, cfg.Runtime.ActivityLeaseTTL)
	maintenanceWorker := engineworker.NewMaintenanceWorker(store)
	projectorWorker := engineprojector.New(store)
	metricsListener := r.options.MetricsListener
	if metricsListener == nil && r.options.MetricsAddr != "" {
		metricsListener, err = net.Listen("tcp", r.options.MetricsAddr)
		if err != nil {
			return fmt.Errorf("runtime: listen for metrics: %w", err)
		}
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.WorkflowPollInterval,
			"workflow",
			observeIterations(recorder, "workflow", workflowWorker.PollOnce),
		)
	})
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.ActivityPollInterval,
			"activity",
			observeIterations(recorder, "activity", activityWorker.PollOnce),
		)
	})
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.MaintenancePollInterval,
			"maintenance",
			observeIterations(recorder, "maintenance", maintenanceWorker.PollOnce),
		)
	})
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.MaintenancePollInterval,
			"catalog-heartbeat",
			observeIterations(recorder, "catalog-heartbeat", func(ctx context.Context, _ string) error {
				return catalog.HeartbeatStoreDefinitions(ctx, store, r.definitions.List())
			}),
		)
	})
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.WorkflowPollInterval,
			"projector",
			observeIterations(recorder, "projector", projectorWorker.PollOnce),
		)
	})
	if metricsListener != nil {
		metricsServer := &http.Server{
			Handler:           metricsMux(metricsGatherer),
			ReadHeaderTimeout: 5 * time.Second,
		}
		group.Go(func() error {
			err := metricsServer.Serve(metricsListener)
			if errors.Is(err, http.ErrServerClosed) || groupCtx.Err() != nil {
				return nil
			}
			return fmt.Errorf("runtime: serve metrics: %w", err)
		})
		group.Go(func() error {
			<-groupCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				_ = metricsServer.Close()
			}
			return nil
		})
	}

	return group.Wait()
}

func buildMetrics(opts *Options) (*enginemetrics.Metrics, prometheus.Gatherer, error) {
	serving := opts.MetricsListener != nil || opts.MetricsAddr != ""
	registerer := opts.MetricsRegistry
	var gatherer prometheus.Gatherer
	if registerer == nil && serving {
		registry := prometheus.NewRegistry()
		registry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
		registerer = registry
		gatherer = registry
	}
	if registerer == nil {
		return nil, nil, nil
	}
	if serving && gatherer == nil {
		var ok bool
		gatherer, ok = registerer.(prometheus.Gatherer)
		if !ok {
			return nil, nil, errors.New("runtime: metrics registry must implement prometheus.Gatherer when serving metrics")
		}
	}
	return enginemetrics.New(registerer), gatherer, nil
}

func observeIterations(
	recorder *enginemetrics.Metrics,
	worker string,
	fn engineworker.IterationFunc,
) engineworker.IterationFunc {
	return func(ctx context.Context, workerID string) (err error) {
		startedAt := time.Now()
		defer func() {
			recorder.ObserveWorkerIteration(worker, time.Since(startedAt))
			if err != nil {
				recorder.IncWorkerIterationError(worker)
			}
		}()
		return fn(ctx, workerID)
	}
}

func metricsMux(gatherer prometheus.Gatherer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", enginemetrics.Handler(gatherer))
	return mux
}

func applyRuntimeOverrides(cfg *config.Config, opts *Options) {
	if opts.WorkflowPollInterval != 0 {
		cfg.Runtime.WorkflowPollInterval = opts.WorkflowPollInterval
	}
	if opts.ActivityPollInterval != 0 {
		cfg.Runtime.ActivityPollInterval = opts.ActivityPollInterval
	}
	if opts.MaintenancePollInterval != 0 {
		cfg.Runtime.MaintenancePollInterval = opts.MaintenancePollInterval
	}
	if opts.RunLeaseTTL != 0 {
		cfg.Runtime.RunLeaseTTL = opts.RunLeaseTTL
	}
	if opts.ActivityLeaseTTL != 0 {
		cfg.Runtime.ActivityLeaseTTL = opts.ActivityLeaseTTL
	}
	cfg.Runtime.ProjectIDFilter = opts.ProjectID
	cfg.Runtime.MetricsAddr = opts.MetricsAddr
}
