package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/activity"
	"github.com/continua-ai/continua/engine/internal/catalog"
	"github.com/continua-ai/continua/engine/internal/config"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	engineprojector "github.com/continua-ai/continua/engine/internal/projector"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	engineworker "github.com/continua-ai/continua/engine/internal/worker"
	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"

	"github.com/continua-ai/continua/engine/cmd/continua-engine/internal/darklaunch"
)

// darkLaunchProjectID aliases the projection writer's single definition of
// the fixed dark-launch demo project.
var darkLaunchProjectID = publicprojection.DarkLaunchProjectID

type commandExitError struct {
	code int
}

func (e commandExitError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.code)
}

type jsonCommandError struct {
	Error jsonErrorPayload `json:"error"`
}

type jsonErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type startResponse struct {
	InstanceID  string `json:"instance_id"`
	InstanceKey string `json:"instance_key"`
	RunID       string `json:"run_id"`
	RunNumber   int32  `json:"run_number"`
	Status      string `json:"status"`
}

type controlResponse struct {
	InstanceID  string `json:"instance_id"`
	RunID       string `json:"run_id"`
	Accepted    bool   `json:"accepted"`
	WakeApplied bool   `json:"wake_applied"`
}

type inspectHistoryEvent struct {
	SequenceNo int32           `json:"sequence_no"`
	EventType  string          `json:"event_type"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

type inspectResponse struct {
	InstanceID        string                `json:"instance_id"`
	InstanceKey       string                `json:"instance_key"`
	DefinitionName    string                `json:"definition_name"`
	DefinitionVersion string                `json:"definition_version"`
	RunID             string                `json:"run_id"`
	RunNumber         int32                 `json:"run_number"`
	Status            string                `json:"status"`
	Result            json.RawMessage       `json:"result,omitempty"`
	CustomStatus      json.RawMessage       `json:"custom_status,omitempty"`
	WaitingFor        json.RawMessage       `json:"waiting_for,omitempty"`
	History           []inspectHistoryEvent `json:"history"`
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the dark-launch engine runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			return withRuntime(ctx, func(ctx context.Context, cfg *config.Config, store *enginestore.Store, definitions *engineworkflow.Registry, activities *activity.Registry) error {
				if err := catalog.PublishStoreDefinitions(ctx, store, definitions.List()); err != nil {
					return err
				}

				workflowWorker := engineworkflow.NewWorker(store, definitions, cfg.Runtime.RunLeaseTTL)
				activityWorker := activity.NewWorker(store, activities, cfg.Runtime.ActivityLeaseTTL)
				maintenanceWorker := engineworker.NewMaintenanceWorker(store)
				projectorWorker := engineprojector.New(store)

				log.Printf("starting continua-engine serve")

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

				err := group.Wait()
				if err == nil {
					log.Printf("continua-engine serve stopped")
				}
				return err
			})
		},
	}
}

func startCmd() *cobra.Command {
	var instanceKey string
	var definitionName string
	var definitionVersion string
	var requestKey string
	var input string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a dark-launch workflow instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceKey == "" || definitionName == "" || definitionVersion == "" || requestKey == "" {
				return writeJSONError(cmd.OutOrStdout(), "internal_error", "start requires --instance-key, --definition, --version, and --request-key")
			}

			inputPayload, err := parseOptionalJSON(input)
			if err != nil {
				return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
			}

			err = withRuntime(cmd.Context(), func(ctx context.Context, cfg *config.Config, store *enginestore.Store, definitions *engineworkflow.Registry, _ *activity.Registry) error {
				if _, ok := definitions.Get(definitionName, definitionVersion); !ok {
					return writeJSONError(cmd.OutOrStdout(), "definition_not_registered", fmt.Sprintf("definition %s@%s is not registered", definitionName, definitionVersion))
				}

				tx, err := store.BeginTx(ctx, pgx.TxOptions{})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				defer func() {
					_ = tx.Rollback(ctx)
				}()

				claim, err := tx.ClaimStartRequestDedupe(ctx, enginestore.ClaimStartRequestDedupeParams{
					ProjectID:    darkLaunchProjectID,
					RequestScope: "engine.start",
					RequestKey:   requestKey,
					ExpiresAt:    time.Now().Add(cfg.Runtime.RequestDedupeTTL),
				})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				switch claim.State {
				case enginestore.StartRequestDedupeClaimStateExistingFinalized:
					payload := claim.Row.ResponsePayload
					exitCode := 0
					if claim.Row.Status == enginedb.EngineRequestDedupeStatusFailed {
						payload, err = json.Marshal(jsonCommandError{
							Error: jsonErrorPayload{
								Code:    derefString(claim.Row.ErrorCode),
								Message: derefString(claim.Row.ErrorMessage),
							},
						})
						if err != nil {
							return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
						}
						exitCode = 1
					}
					if err := tx.Commit(ctx); err != nil {
						return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
					}
					return writeRawJSON(cmd.OutOrStdout(), payload, exitCode)
				case enginestore.StartRequestDedupeClaimStateExistingInProgress:
					return writeJSONError(cmd.OutOrStdout(), "request_in_progress", "a start request with this request key is still in progress")
				}

				if _, err := tx.Tx().Exec(ctx, "SAVEPOINT start_create_instance"); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				instance, err := tx.CreateInstance(ctx, enginedb.CreateInstanceParams{
					ProjectID:      darkLaunchProjectID,
					InstanceKey:    instanceKey,
					DefinitionName: definitionName,
				})
				if err != nil {
					if _, rollbackErr := tx.Tx().Exec(ctx, "ROLLBACK TO SAVEPOINT start_create_instance"); rollbackErr != nil {
						return writeJSONError(cmd.OutOrStdout(), "internal_error", rollbackErr.Error())
					}
					if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT start_create_instance"); releaseErr != nil {
						return writeJSONError(cmd.OutOrStdout(), "internal_error", releaseErr.Error())
					}
					if errors.Is(err, enginestore.ErrAlreadyExists) {
						return finalizeStartFailure(ctx, tx, claim.Row.ID, cmd.OutOrStdout(), "instance_conflict", "an instance with this key already exists")
					}
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				if _, err := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT start_create_instance"); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				run, err := tx.CreateRun(ctx, enginedb.CreateRunParams{
					ProjectID:         darkLaunchProjectID,
					InstanceID:        instance.ID,
					RunNumber:         1,
					DefinitionVersion: definitionVersion,
					ReadyAt:           time.Now(),
				})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				startedPayload := enginehistory.WorkflowStartedPayload{
					DefinitionName:    definitionName,
					DefinitionVersion: definitionVersion,
					InstanceKey:       instanceKey,
					Input:             inputPayload,
				}
				startedRaw, err := enginehistory.MarshalPayload(startedPayload)
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				startedEvent, err := tx.AppendHistory(ctx, enginedb.AppendHistoryParams{
					ProjectID:  darkLaunchProjectID,
					InstanceID: instance.ID,
					RunID:      run.ID,
					SequenceNo: 1,
					EventType:  enginehistory.EventWorkflowStarted,
					Payload:    startedRaw,
				})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				if err := publicprojection.NewWriter(tx.Tx()).EnsureDarkLaunchShell(ctx, &instance, &run, definitionName, definitionVersion, inputPayload, startedEvent.ID); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				response := startResponse{
					InstanceID:  instance.ID.String(),
					InstanceKey: instance.InstanceKey,
					RunID:       run.ID.String(),
					RunNumber:   run.RunNumber,
					Status:      string(run.Status),
				}
				responsePayload, err := json.Marshal(response)
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				if _, err := tx.FinalizeRequestDedupeWithResponse(ctx, enginedb.FinalizeRequestDedupeWithResponseParams{
					ID:              claim.Row.ID,
					ResponsePayload: responsePayload,
				}); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				if err := tx.Commit(ctx); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				return writeRawJSON(cmd.OutOrStdout(), responsePayload)
			})
			if err != nil {
				var exitErr commandExitError
				if errors.As(err, &exitErr) {
					return err
				}
				return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceKey, "instance-key", "", "Durable instance key")
	cmd.Flags().StringVar(&definitionName, "definition", "", "Compiled workflow definition name")
	cmd.Flags().StringVar(&definitionVersion, "version", "", "Compiled workflow definition version")
	cmd.Flags().StringVar(&requestKey, "request-key", "", "Durable request dedupe key")
	cmd.Flags().StringVar(&input, "input", "", "Optional workflow input as JSON")
	return cmd
}

func signalCmd() *cobra.Command {
	var instanceKey string
	var signalName string
	var payload string
	var dedupeKey string

	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Deliver a durable signal to the active run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceKey == "" || signalName == "" {
				return writeJSONError(cmd.OutOrStdout(), "internal_error", "signal requires --instance-key and --signal-name")
			}

			payloadRaw, err := parseOptionalJSON(payload)
			if err != nil {
				return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
			}

			err = withRuntime(cmd.Context(), func(ctx context.Context, _ *config.Config, store *enginestore.Store, _ *engineworkflow.Registry, _ *activity.Registry) error {
				tx, err := store.BeginTx(ctx, pgx.TxOptions{})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				defer func() { _ = tx.Rollback(ctx) }()

				instance, run, err := loadLatestRunForUpdate(ctx, tx, instanceKey)
				if err != nil {
					return commandErrorFromStore(cmd.OutOrStdout(), err)
				}
				if isTerminalRun(run.Status) {
					return writeJSONError(cmd.OutOrStdout(), "run_terminal", "cannot signal a terminal run")
				}

				signalPayload := enginehistory.SignalReceivedPayload{
					SignalName: signalName,
					Payload:    payloadRaw,
				}
				signalRaw, err := enginehistory.MarshalPayload(signalPayload)
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				createErr := func() error {
					_, createErr := tx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
						ProjectID:   run.ProjectID,
						InstanceID:  run.InstanceID,
						RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
						Kind:        "signal",
						Payload:     signalRaw,
						AvailableAt: time.Now(),
						DedupeKey:   stringPtrOrNil(dedupeKey),
					})
					return createErr
				}()
				if createErr != nil && (!errors.Is(createErr, enginestore.ErrAlreadyExists) || dedupeKey == "") {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", createErr.Error())
				}

				wake, err := tx.WakeWaitingRun(ctx, run.ID)
				if err != nil && !errors.Is(err, enginestore.ErrNotFound) {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				if err == nil {
					if syncErr := publicprojection.NewWriter(tx.Tx()).SyncRunSummary(ctx, &wake.Run); syncErr != nil {
						return writeJSONError(cmd.OutOrStdout(), "internal_error", syncErr.Error())
					}
				}

				response := controlResponse{
					InstanceID:  instance.ID.String(),
					RunID:       run.ID.String(),
					Accepted:    true,
					WakeApplied: err == nil && wake.Applied,
				}

				if err := tx.Commit(ctx); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				return writeJSON(cmd.OutOrStdout(), response)
			})
			if err != nil {
				var exitErr commandExitError
				if errors.As(err, &exitErr) {
					return err
				}
				return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceKey, "instance-key", "", "Durable instance key")
	cmd.Flags().StringVar(&signalName, "signal-name", "", "Signal name")
	cmd.Flags().StringVar(&payload, "payload", "", "Optional JSON signal payload")
	cmd.Flags().StringVar(&dedupeKey, "dedupe-key", "", "Optional inbox dedupe key")
	return cmd
}

func cancelCmd() *cobra.Command {
	var instanceKey string

	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Deliver a durable cancellation request to the active run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceKey == "" {
				return writeJSONError(cmd.OutOrStdout(), "internal_error", "cancel requires --instance-key")
			}

			err := withRuntime(cmd.Context(), func(ctx context.Context, _ *config.Config, store *enginestore.Store, _ *engineworkflow.Registry, _ *activity.Registry) error {
				tx, err := store.BeginTx(ctx, pgx.TxOptions{})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				defer func() { _ = tx.Rollback(ctx) }()

				instance, run, err := loadLatestRunForUpdate(ctx, tx, instanceKey)
				if err != nil {
					return commandErrorFromStore(cmd.OutOrStdout(), err)
				}
				if isTerminalRun(run.Status) {
					return writeJSONError(cmd.OutOrStdout(), "run_terminal", "cannot cancel a terminal run")
				}

				cancelRaw, err := enginehistory.MarshalPayload(enginehistory.CancelRequestedPayload{})
				if err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}

				cancelDedupe := "cancel:" + run.ID.String()
				_, createErr := tx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
					ProjectID:   run.ProjectID,
					InstanceID:  run.InstanceID,
					RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
					Kind:        "cancel",
					Payload:     cancelRaw,
					AvailableAt: time.Now(),
					DedupeKey:   &cancelDedupe,
				})
				if createErr != nil && !errors.Is(createErr, enginestore.ErrAlreadyExists) {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", createErr.Error())
				}

				wake, err := tx.WakeWaitingRun(ctx, run.ID)
				if err != nil && !errors.Is(err, enginestore.ErrNotFound) {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				if err == nil {
					if syncErr := publicprojection.NewWriter(tx.Tx()).SyncRunSummary(ctx, &wake.Run); syncErr != nil {
						return writeJSONError(cmd.OutOrStdout(), "internal_error", syncErr.Error())
					}
				}

				response := controlResponse{
					InstanceID:  instance.ID.String(),
					RunID:       run.ID.String(),
					Accepted:    true,
					WakeApplied: err == nil && wake.Applied,
				}

				if err := tx.Commit(ctx); err != nil {
					return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
				}
				return writeJSON(cmd.OutOrStdout(), response)
			})
			if err != nil {
				var exitErr commandExitError
				if errors.As(err, &exitErr) {
					return err
				}
				return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceKey, "instance-key", "", "Durable instance key")
	return cmd
}

func inspectCmd() *cobra.Command {
	var instanceKey string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect the current state of a dark-launch workflow instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceKey == "" {
				return writeJSONError(cmd.OutOrStdout(), "internal_error", "inspect requires --instance-key")
			}

			err := withRuntime(cmd.Context(), func(ctx context.Context, _ *config.Config, store *enginestore.Store, _ *engineworkflow.Registry, _ *activity.Registry) error {
				instance, run, historyRows, err := loadInstanceState(ctx, store, instanceKey)
				if err != nil {
					return commandErrorFromStore(cmd.OutOrStdout(), err)
				}

				response := inspectResponse{
					InstanceID:        instance.ID.String(),
					InstanceKey:       instance.InstanceKey,
					DefinitionName:    instance.DefinitionName,
					DefinitionVersion: run.DefinitionVersion,
					RunID:             run.ID.String(),
					RunNumber:         run.RunNumber,
					Status:            string(run.Status),
					Result:            cloneRaw(run.Result),
					CustomStatus:      cloneRaw(run.CustomStatus),
					WaitingFor:        cloneRaw(run.WaitingFor),
					History:           make([]inspectHistoryEvent, 0, len(historyRows)),
				}

				for i := range historyRows {
					historyRow := historyRows[i]
					response.History = append(response.History, inspectHistoryEvent{
						SequenceNo: historyRow.SequenceNo,
						EventType:  historyRow.EventType,
						Payload:    cloneRaw(historyRow.Payload),
						CreatedAt:  historyRow.CreatedAt,
					})
				}

				return writeJSON(cmd.OutOrStdout(), response)
			})
			if err != nil {
				var exitErr commandExitError
				if errors.As(err, &exitErr) {
					return err
				}
				return writeJSONError(cmd.OutOrStdout(), "internal_error", err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceKey, "instance-key", "", "Durable instance key")
	return cmd
}

func withRuntime(
	ctx context.Context,
	fn func(context.Context, *config.Config, *enginestore.Store, *engineworkflow.Registry, *activity.Registry) error,
) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := enginestore.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	store := enginestore.New(pool)
	if cfg.Runtime.ProjectIDFilter != nil {
		store = store.WithProjectFilter(*cfg.Runtime.ProjectIDFilter)
	}
	defer store.Close()

	definitions, activities, err := buildRegistries()
	if err != nil {
		return err
	}

	return fn(ctx, cfg, store, definitions, activities)
}

func buildRegistries() (definitions *engineworkflow.Registry, activities *activity.Registry, err error) {
	definitions, err = engineworkflow.NewRegistry(darklaunch.Definitions()...)
	if err != nil {
		return nil, nil, err
	}
	activities, err = activity.NewRegistry(darklaunch.Handlers())
	if err != nil {
		return nil, nil, err
	}
	return definitions, activities, nil
}

func loadInstanceState(
	ctx context.Context,
	store *enginestore.Store,
	instanceKey string,
) (enginedb.EngineInstance, enginedb.EngineRun, []enginedb.EngineHistory, error) {
	instance, err := store.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   darkLaunchProjectID,
		InstanceKey: instanceKey,
	})
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, err
	}

	runs, err := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
		InstanceID: instance.ID,
		Limit:      1,
	})
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, err
	}
	if len(runs) == 0 {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, enginestore.ErrNotFound
	}

	historyRows, err := store.GetHistoryByRun(ctx, runs[0].ID)
	if err != nil {
		return enginedb.EngineInstance{}, enginedb.EngineRun{}, nil, err
	}

	return instance, runs[0], historyRows, nil
}

func loadLatestRun(
	ctx context.Context,
	store interface {
		GetInstanceByProjectAndKey(context.Context, enginedb.GetInstanceByProjectAndKeyParams) (enginedb.EngineInstance, error)
		ListRunsByInstance(context.Context, enginedb.ListRunsByInstanceParams) ([]enginedb.EngineRun, error)
	},
	instanceKey string,
) (instance enginedb.EngineInstance, run enginedb.EngineRun, err error) {
	instance, err = store.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   darkLaunchProjectID,
		InstanceKey: instanceKey,
	})
	if err != nil {
		return instance, run, err
	}

	runs, err := store.ListRunsByInstance(ctx, enginedb.ListRunsByInstanceParams{
		InstanceID: instance.ID,
		Limit:      1,
	})
	if err != nil {
		return instance, run, err
	}
	if len(runs) == 0 {
		return instance, run, enginestore.ErrNotFound
	}

	run = runs[0]
	return instance, run, nil
}

func loadLatestRunForUpdate(
	ctx context.Context,
	store interface {
		GetInstanceByProjectAndKey(context.Context, enginedb.GetInstanceByProjectAndKeyParams) (enginedb.EngineInstance, error)
		ListRunsByInstance(context.Context, enginedb.ListRunsByInstanceParams) ([]enginedb.EngineRun, error)
		GetRunForUpdate(context.Context, uuid.UUID) (enginedb.EngineRun, error)
	},
	instanceKey string,
) (instance enginedb.EngineInstance, run enginedb.EngineRun, err error) {
	instance, run, err = loadLatestRun(ctx, store, instanceKey)
	if err != nil {
		return instance, run, err
	}

	lockedRun, err := store.GetRunForUpdate(ctx, run.ID)
	if err != nil {
		return instance, run, err
	}

	run = lockedRun
	return instance, run, nil
}

func finalizeStartFailure(
	ctx context.Context,
	tx *enginestore.Tx,
	dedupeID uuid.UUID,
	writer io.Writer,
	code string,
	message string,
) error {
	payload, err := json.Marshal(jsonCommandError{
		Error: jsonErrorPayload{
			Code:    code,
			Message: message,
		},
	})
	if err != nil {
		return writeJSONError(writer, "internal_error", err.Error())
	}
	if _, err := tx.FinalizeRequestDedupeWithError(ctx, enginedb.FinalizeRequestDedupeWithErrorParams{
		ID:           dedupeID,
		ErrorCode:    stringPtrOrNil(code),
		ErrorMessage: stringPtrOrNil(message),
	}); err != nil {
		return writeJSONError(writer, "internal_error", err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return writeJSONError(writer, "internal_error", err.Error())
	}
	return writeRawJSON(writer, payload, 1)
}

func writeJSON(writer io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, string(payload))
	return err
}

func writeRawJSON(writer io.Writer, payload []byte, exitCode ...int) error {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	if _, err := fmt.Fprintln(writer, string(payload)); err != nil {
		return err
	}
	if len(exitCode) > 0 && exitCode[0] != 0 {
		return commandExitError{code: exitCode[0]}
	}
	return nil
}

func writeJSONError(writer io.Writer, code, message string) error {
	if err := writeJSON(writer, jsonCommandError{
		Error: jsonErrorPayload{
			Code:    code,
			Message: message,
		},
	}); err != nil {
		return err
	}
	return commandExitError{code: 1}
}

func commandErrorFromStore(writer io.Writer, err error) error {
	if errors.Is(err, enginestore.ErrNotFound) {
		return writeJSONError(writer, "not_found", "workflow instance not found")
	}
	return writeJSONError(writer, "internal_error", err.Error())
}

func parseOptionalJSON(value string) (json.RawMessage, error) {
	if value == "" {
		return nil, nil
	}
	if !json.Valid([]byte(value)) {
		return nil, fmt.Errorf("invalid JSON payload")
	}
	return json.RawMessage(value), nil
}

func isTerminalRun(status enginedb.EngineRunLifecycleStatus) bool {
	return status == enginedb.EngineRunLifecycleStatusCompleted ||
		status == enginedb.EngineRunLifecycleStatusFailed ||
		status == enginedb.EngineRunLifecycleStatusCancelled ||
		status == enginedb.EngineRunLifecycleStatusTerminated
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
