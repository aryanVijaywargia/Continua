// Package runtime embeds the Continua durable-execution engine as a library:
// user programs register workflow definitions and activity handlers, then run
// the engine workers against Postgres.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/continua-ai/continua/engine/internal/activity"
	"github.com/continua-ai/continua/engine/internal/catalog"
	"github.com/continua-ai/continua/engine/internal/config"
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

	pool, err := enginestore.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	store := enginestore.New(pool)
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

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return engineworker.RunLoop(groupCtx, cfg.Runtime.WorkflowPollInterval, "workflow", workflowWorker.PollOnce)
	})
	group.Go(func() error {
		return engineworker.RunLoop(groupCtx, cfg.Runtime.ActivityPollInterval, "activity", activityWorker.PollOnce)
	})
	group.Go(func() error {
		return engineworker.RunLoop(groupCtx, cfg.Runtime.MaintenancePollInterval, "maintenance", maintenanceWorker.PollOnce)
	})
	group.Go(func() error {
		return engineworker.RunLoop(groupCtx, cfg.Runtime.WorkflowPollInterval, "projector", projectorWorker.PollOnce)
	})

	return group.Wait()
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
}
