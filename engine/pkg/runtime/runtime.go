// Package runtime embeds the Continua durable-execution engine as a library:
// user programs register workflow definitions and activity handlers, then run
// the engine workers against Postgres.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"golang.org/x/sync/errgroup"

	"github.com/continua-ai/continua/engine/internal/activity"
	"github.com/continua-ai/continua/engine/internal/catalog"
	"github.com/continua-ai/continua/engine/internal/config"
	"github.com/continua-ai/continua/engine/internal/health"
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
	// Logger receives structured workflow and activity worker events.
	Logger *slog.Logger
	// ProjectID optionally scopes all polling to a single project.
	ProjectID *uuid.UUID
	// DBMaxConns configures the Postgres pool maximum; non-positive values use the engine default.
	DBMaxConns int32
	// DBMinConns configures the Postgres pool minimum. Positive values override the engine default;
	// set DBMinConnsSet to apply an explicit zero.
	DBMinConns int32
	// DBMinConnsSet distinguishes an explicit zero DBMinConns from an omitted value.
	DBMinConnsSet bool
	// DBMaxConnLifetime, DBMaxConnIdleTime, and DBHealthCheckPeriod configure the Postgres pool;
	// non-positive values use the engine defaults.
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration
	// ProjectorBatchSize bounds rows projected per poll; non-positive values use the engine default.
	ProjectorBatchSize int32
	// MaxChildDepth and MaxContinuationFollowDepth bound child workflow traversal;
	// non-positive values use the engine defaults.
	MaxChildDepth              int32
	MaxContinuationFollowDepth int32
	// Poll intervals and lease TTLs; zero values use the engine defaults.
	WorkflowPollInterval    time.Duration
	ActivityPollInterval    time.Duration
	MaintenancePollInterval time.Duration
	MetricsSampleInterval   time.Duration
	RunLeaseTTL             time.Duration
	ActivityLeaseTTL        time.Duration
	// ShutdownGrace is the maximum time allowed for in-flight work to finish during shutdown.
	ShutdownGrace time.Duration
	// RetentionTerminalRuns and RetentionDedupeGrace use engine defaults when
	// zero; negative values disable the corresponding retention class.
	RetentionTerminalRuns time.Duration
	RetentionDedupeGrace  time.Duration
	// RetentionBatchSize is the maximum rows or runs reaped per pass;
	// non-positive values use the engine default.
	RetentionBatchSize int32
	// MetricsRegistry receives engine Prometheus collectors when configured.
	MetricsRegistry prometheus.Registerer
	// MetricsAddr configures the Prometheus HTTP listen address when non-empty.
	MetricsAddr string
	// MetricsListener supplies a caller-owned listener for the Prometheus endpoint.
	MetricsListener net.Listener
	// HTTPAddr configures the operational HTTP listen address when non-empty.
	HTTPAddr string
	// HTTPListener supplies a caller-owned listener for the operational HTTP endpoints.
	HTTPListener net.Listener
	// LeaseCompletionGrace allows the current remote activity owner to complete,
	// fail, or retry briefly after lease expiry. It does not extend claims or heartbeats.
	LeaseCompletionGrace time.Duration
}

// Runtime is an embedded engine instance built from Options.
type Runtime struct {
	options     Options
	definitions *engineworkflow.Registry
	activities  *activity.Registry
	runMu       sync.Mutex
	started     bool
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

// Run executes the engine workers until ctx is cancelled. A Runtime is
// single-use; subsequent calls return an error. Run returns nil on graceful
// shutdown.
func (r *Runtime) Run(ctx context.Context) error {
	if r == nil {
		return errors.New("runtime: nil runtime")
	}
	if ctx == nil {
		return errors.New("runtime: context is required")
	}
	if err := r.beginRun(); err != nil {
		return err
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
	store := enginestore.New(pool).WithLeaseCompletionGrace(cfg.Runtime.LeaseCompletionGrace)
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

	workflowWorker := engineworkflow.NewWorker(store, r.definitions, cfg.Runtime.RunLeaseTTL, r.options.Logger).WithDepthLimits(engineworkflow.DepthLimits{
		MaxChildDepth:              cfg.Runtime.MaxChildDepth,
		MaxContinuationFollowDepth: cfg.Runtime.MaxContinuationFollowDepth,
	})
	activityWorker := activity.NewWorker(store, r.activities, cfg.Runtime.ActivityLeaseTTL, r.options.Logger)
	maintenanceWorker := engineworker.NewMaintenanceWorker(store)
	retentionWorker := engineworker.NewRetentionWorker(store, engineworker.RetentionConfig{
		TerminalRuns: cfg.Runtime.RetentionTerminalRuns,
		DedupeGrace:  cfg.Runtime.RetentionDedupeGrace,
		BatchSize:    cfg.Runtime.RetentionBatchSize,
	})
	projectorWorker := engineprojector.New(store).WithBatchSize(cfg.Runtime.ProjectorBatchSize)
	tracker := health.NewTracker()
	tracker.Register("workflow", workerStaleAfter(cfg.Runtime.WorkflowPollInterval))
	tracker.Register("activity", workerStaleAfter(cfg.Runtime.ActivityPollInterval))
	tracker.Register("maintenance", workerStaleAfter(cfg.Runtime.MaintenancePollInterval))
	tracker.Register("catalog-heartbeat", workerStaleAfter(cfg.Runtime.MaintenancePollInterval))
	tracker.Register("projector", workerStaleAfter(cfg.Runtime.WorkflowPollInterval))
	if recorder != nil {
		tracker.Register("metrics", workerStaleAfter(cfg.Runtime.MetricsSampleInterval))
	}
	metricsListener := r.options.MetricsListener
	metricsListenerOwned := false
	if metricsListener == nil && r.options.MetricsAddr != "" {
		metricsListener, err = net.Listen("tcp", r.options.MetricsAddr)
		if err != nil {
			return fmt.Errorf("runtime: listen for metrics: %w", err)
		}
		metricsListenerOwned = true
	}
	httpListener := r.options.HTTPListener
	if httpListener == nil && r.options.HTTPAddr != "" {
		httpListener, err = net.Listen("tcp", r.options.HTTPAddr)
		if err != nil {
			if metricsListenerOwned {
				_ = metricsListener.Close()
			}
			return fmt.Errorf("runtime: listen for operational HTTP: %w", err)
		}
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.WorkflowPollInterval,
			"workflow",
			trackIterations(tracker, "workflow", observeIterations(recorder, "workflow", workflowWorker.PollOnce)),
		)
	})
	if recorder != nil {
		group.Go(func() error {
			return engineworker.RunLoop(
				groupCtx,
				cfg.Runtime.MetricsSampleInterval,
				"metrics",
				trackIterations(tracker, "metrics", observeIterations(recorder, "metrics", maintenanceWorker.PollMetricsOnce)),
			)
		})
	}
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.ActivityPollInterval,
			"activity",
			trackIterations(tracker, "activity", observeIterations(recorder, "activity", activityWorker.PollOnce)),
		)
	})
	if cfg.Runtime.RetentionTerminalRuns > 0 || cfg.Runtime.RetentionDedupeGrace > 0 {
		group.Go(func() error {
			return engineworker.RunLoop(
				groupCtx,
				cfg.Runtime.MaintenancePollInterval,
				"retention",
				observeIterations(recorder, "retention", retentionWorker.PollOnce),
			)
		})
	}
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.MaintenancePollInterval,
			"maintenance",
			trackIterations(tracker, "maintenance", observeIterations(recorder, "maintenance", maintenanceWorker.PollOnce)),
		)
	})
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.MaintenancePollInterval,
			"catalog-heartbeat",
			trackIterations(tracker, "catalog-heartbeat", observeIterations(recorder, "catalog-heartbeat", func(ctx context.Context, _ string) error {
				return catalog.HeartbeatStoreDefinitions(ctx, store, r.definitions.List())
			})),
		)
	})
	group.Go(func() error {
		return engineworker.RunLoop(
			groupCtx,
			cfg.Runtime.WorkflowPollInterval,
			"projector",
			trackIterations(tracker, "projector", observeIterations(recorder, "projector", projectorWorker.PollOnce)),
		)
	})
	if metricsListener != nil {
		metricsServer := &http.Server{
			Handler:           metricsMux(metricsGatherer),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
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
	if httpListener != nil {
		httpServer := &http.Server{
			Handler:           operationalMux(pool, tracker, metricsGatherer),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		group.Go(func() error {
			err := httpServer.Serve(httpListener)
			if errors.Is(err, http.ErrServerClosed) || groupCtx.Err() != nil {
				return nil
			}
			return fmt.Errorf("runtime: serve operational HTTP: %w", err)
		})
		group.Go(func() error {
			<-groupCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				_ = httpServer.Close()
			}
			return nil
		})
	}

	return group.Wait()
}

func (r *Runtime) beginRun() error {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if r.started {
		return errors.New("runtime: Run may only be called once")
	}
	r.started = true
	return nil
}

func buildMetrics(opts *Options) (*enginemetrics.Metrics, prometheus.Gatherer, error) {
	serving := opts.MetricsListener != nil || opts.MetricsAddr != "" || opts.HTTPListener != nil || opts.HTTPAddr != ""
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

func trackIterations(tracker *health.Tracker, worker string, fn engineworker.IterationFunc) engineworker.IterationFunc {
	return func(ctx context.Context, workerID string) error {
		err := fn(ctx, workerID)
		tracker.MarkIteration(worker)
		return err
	}
}

func workerStaleAfter(pollInterval time.Duration) time.Duration {
	staleAfter := 10 * pollInterval
	if staleAfter < 10*time.Second {
		return 10 * time.Second
	}
	return staleAfter
}

func metricsMux(gatherer prometheus.Gatherer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", enginemetrics.Handler(gatherer))
	return mux
}

func operationalMux(pool health.Pinger, tracker *health.Tracker, gatherer prometheus.Gatherer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", health.LivenessHandler())
	mux.Handle("/readyz", health.ReadinessHandler(pool, tracker, 2*time.Second))
	if gatherer != nil {
		mux.Handle("/metrics", enginemetrics.Handler(gatherer))
	}
	return mux
}

func applyRuntimeOverrides(cfg *config.Config, opts *Options) {
	if opts.DBMaxConns > 0 {
		cfg.Database.MaxConns = opts.DBMaxConns
	}
	if opts.DBMinConns > 0 || (opts.DBMinConnsSet && opts.DBMinConns == 0) {
		cfg.Database.MinConns = opts.DBMinConns
	}
	if opts.DBMaxConnLifetime > 0 {
		cfg.Database.MaxConnLifetime = opts.DBMaxConnLifetime
	}
	if opts.DBMaxConnIdleTime > 0 {
		cfg.Database.MaxConnIdleTime = opts.DBMaxConnIdleTime
	}
	if opts.DBHealthCheckPeriod > 0 {
		cfg.Database.HealthCheckPeriod = opts.DBHealthCheckPeriod
	}
	if opts.ProjectorBatchSize > 0 {
		cfg.Runtime.ProjectorBatchSize = opts.ProjectorBatchSize
	}
	if opts.MaxChildDepth > 0 {
		cfg.Runtime.MaxChildDepth = opts.MaxChildDepth
	}
	if opts.MaxContinuationFollowDepth > 0 {
		cfg.Runtime.MaxContinuationFollowDepth = opts.MaxContinuationFollowDepth
	}
	if opts.WorkflowPollInterval != 0 {
		cfg.Runtime.WorkflowPollInterval = opts.WorkflowPollInterval
	}
	if opts.ActivityPollInterval != 0 {
		cfg.Runtime.ActivityPollInterval = opts.ActivityPollInterval
	}
	if opts.MaintenancePollInterval != 0 {
		cfg.Runtime.MaintenancePollInterval = opts.MaintenancePollInterval
	}
	if opts.MetricsSampleInterval != 0 {
		cfg.Runtime.MetricsSampleInterval = opts.MetricsSampleInterval
	}
	if opts.RunLeaseTTL != 0 {
		cfg.Runtime.RunLeaseTTL = opts.RunLeaseTTL
	}
	if opts.ActivityLeaseTTL != 0 {
		cfg.Runtime.ActivityLeaseTTL = opts.ActivityLeaseTTL
	}
	if opts.RetentionTerminalRuns < 0 {
		cfg.Runtime.RetentionTerminalRuns = 0
	} else if opts.RetentionTerminalRuns != 0 {
		cfg.Runtime.RetentionTerminalRuns = opts.RetentionTerminalRuns
	}
	if opts.RetentionDedupeGrace < 0 {
		cfg.Runtime.RetentionDedupeGrace = 0
	} else if opts.RetentionDedupeGrace != 0 {
		cfg.Runtime.RetentionDedupeGrace = opts.RetentionDedupeGrace
	}
	if opts.RetentionBatchSize > 0 {
		cfg.Runtime.RetentionBatchSize = opts.RetentionBatchSize
	}
	if opts.LeaseCompletionGrace != 0 {
		cfg.Runtime.LeaseCompletionGrace = opts.LeaseCompletionGrace
	}
	cfg.Runtime.ProjectIDFilter = opts.ProjectID
	cfg.Runtime.MetricsAddr = opts.MetricsAddr
	cfg.Runtime.HTTPAddr = opts.HTTPAddr
}
