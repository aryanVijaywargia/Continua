// Package runtime embeds the Continua durable-execution engine as a library:
// user programs register workflow definitions and activity handlers, then run
// the engine workers against Postgres.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/activity"
	"github.com/continua-ai/continua/engine/internal/catalog"
	"github.com/continua-ai/continua/engine/internal/config"
	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	engineworker "github.com/continua-ai/continua/engine/internal/worker"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
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
func New(opts Options) (*Runtime, error) {
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
		ctx = context.Background()
	}

	cfg := config.Defaults(r.options.DatabaseURL)
	applyRuntimeOverrides(cfg, r.options)

	pool, err := enginestore.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	store := enginestore.New(pool)
	if r.options.ProjectID != nil {
		store = store.WithProjectFilter(*r.options.ProjectID)
	}
	defer store.Close()

	if err := catalog.PublishStoreDefinitions(ctx, store, r.definitions.List()); err != nil {
		return err
	}
	if err := ensureInitialProjectionShells(ctx, store, r.options.ProjectID); err != nil {
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

func applyRuntimeOverrides(cfg *config.Config, opts Options) {
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

func ensureInitialProjectionShells(ctx context.Context, store *enginestore.Store, projectID *uuid.UUID) error {
	tx, err := store.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	runIDs, err := missingProjectionShellRunIDs(ctx, tx, projectID)
	if err != nil {
		return err
	}

	for _, runID := range runIDs {
		if err := ensureInitialProjectionShell(ctx, tx, runID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func missingProjectionShellRunIDs(ctx context.Context, tx *enginestore.Tx, projectID *uuid.UUID) ([]uuid.UUID, error) {
	const baseQuery = `
		SELECT r.id
		FROM engine.runs AS r
		WHERE EXISTS (
			SELECT 1
			FROM engine.history AS h
			WHERE h.run_id = r.id
			  AND h.event_type = $1
		)
		  AND NOT EXISTS (
			SELECT 1
			FROM public.traces AS t
			WHERE t.engine_run_id = r.id
		)`

	var (
		rows pgx.Rows
		err  error
	)
	if projectID == nil {
		rows, err = tx.Tx().Query(ctx, baseQuery+"\nORDER BY r.created_at ASC, r.id ASC", publichistory.EventWorkflowStarted)
	} else {
		rows, err = tx.Tx().Query(ctx, baseQuery+"\n  AND r.project_id = $2\nORDER BY r.created_at ASC, r.id ASC", publichistory.EventWorkflowStarted, *projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runIDs []uuid.UUID
	for rows.Next() {
		var runID uuid.UUID
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runIDs, nil
}

func ensureInitialProjectionShell(ctx context.Context, tx *enginestore.Tx, runID uuid.UUID) error {
	run, err := tx.GetRunForUpdate(ctx, runID)
	if err != nil {
		return err
	}
	instance, err := tx.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return err
	}
	startedEvent, ok, err := workflowStartedEvent(ctx, tx, run.ID)
	if err != nil || !ok {
		return err
	}

	var startedPayload publichistory.WorkflowStartedPayload
	if err := publichistory.UnmarshalPayload(startedEvent.Payload, &startedPayload); err != nil {
		return err
	}

	traceName := instance.DefinitionName
	return publicprojection.NewWriter(tx.Tx()).CreateTraceShell(
		ctx,
		&instance,
		&run,
		&publicprojection.TraceShellSeed{},
		&startedEvent,
		startedPayload.Input,
		&traceName,
	)
}

func workflowStartedEvent(
	ctx context.Context,
	tx *enginestore.Tx,
	runID uuid.UUID,
) (enginedb.EngineHistory, bool, error) {
	historyRows, err := tx.GetHistoryByRun(ctx, runID)
	if err != nil {
		return enginedb.EngineHistory{}, false, err
	}
	for i := range historyRows {
		if historyRows[i].EventType == publichistory.EventWorkflowStarted {
			return historyRows[i], true, nil
		}
	}
	return enginedb.EngineHistory{}, false, nil
}
