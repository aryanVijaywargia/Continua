package enginecontrol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	"github.com/continua-ai/continua/internal/store"
)

const engineRootSpanPrefix = "engine:root:"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	return e.Code + ": " + e.Message
}

type PurgeMode string

const (
	PurgeModeProjectionOnly PurgeMode = "projection_only"
	PurgeModeFull           PurgeMode = "full"
)

type RepairReason string

const (
	RepairReasonAlreadyUpToDate   RepairReason = "already_up_to_date"
	RepairReasonHistoryExpired    RepairReason = "history_expired"
	RepairReasonNoEventsToProject RepairReason = "no_events_to_project"
	RepairReasonRequested         RepairReason = "repair_requested"
	RepairReasonAlreadyCatchingUp RepairReason = "already_catching_up"
)

type PurgeResult struct {
	RunID           uuid.UUID
	Mode            PurgeMode
	ProjectionState string
	Deleted         bool
}

type RepairResult struct {
	RunID           uuid.UUID
	Accepted        bool
	Reason          RepairReason
	ProjectionState string
}

type Service struct {
	platform *store.Store
}

func NewService(platformStore *store.Store) *Service {
	if platformStore == nil {
		return nil
	}
	return &Service{platform: platformStore}
}

func (s *Service) PurgeRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
	mode PurgeMode,
) (PurgeResult, error) {
	if s == nil || s.platform == nil {
		return PurgeResult{}, errors.New("engine control service is not configured")
	}
	if mode != PurgeModeProjectionOnly && mode != PurgeModeFull {
		return PurgeResult{}, &APIError{
			Code:       "invalid_request",
			Message:    "mode must be projection_only or full",
			HTTPStatus: 400,
		}
	}

	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PurgeResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	trace, run, projectionState, err := loadLockedRunTrace(ctx, tx, projectID, runID)
	if err != nil {
		return PurgeResult{}, err
	}
	if !isTerminalRun(run.Status) {
		return PurgeResult{}, &APIError{
			Code:       "run_not_terminal",
			Message:    "run has not reached a terminal state",
			HTTPStatus: 409,
		}
	}
	if err := ensureTerminalShell(ctx, tx, &trace, &run); err != nil {
		return PurgeResult{}, err
	}

	result := PurgeResult{
		RunID:           runID,
		Mode:            mode,
		ProjectionState: projectionState,
	}

	switch mode {
	case PurgeModeProjectionOnly:
		if projectionState == publicprojection.StateSummaryOnly.String() ||
			projectionState == publicprojection.StateJournalExpired.String() {
			if err := tx.Commit(ctx); err != nil {
				return PurgeResult{}, err
			}
			return result, nil
		}
		if err := tx.DeleteSpanEventsByTrace(ctx, trace.ID); err != nil {
			return PurgeResult{}, err
		}
		if err := tx.DeleteNonRootSpansByTrace(ctx, trace.ID); err != nil {
			return PurgeResult{}, err
		}
		if mutated, err := tx.FlipProjectionStateToSummaryOnly(ctx, runID); err != nil {
			return PurgeResult{}, err
		} else if mutated == 0 {
			return PurgeResult{}, fmt.Errorf("flip projection state to summary_only matched zero rows for run %s", runID)
		}
		result.ProjectionState = publicprojection.StateSummaryOnly.String()
		result.Deleted = true
	case PurgeModeFull:
		if projectionState == publicprojection.StateJournalExpired.String() {
			if err := tx.Commit(ctx); err != nil {
				return PurgeResult{}, err
			}
			return result, nil
		}
		if err := tx.DeleteSpanEventsByTrace(ctx, trace.ID); err != nil {
			return PurgeResult{}, err
		}
		if err := tx.DeleteNonRootSpansByTrace(ctx, trace.ID); err != nil {
			return PurgeResult{}, err
		}
		if err := enginedb.New(tx.Tx()).DeleteHistoryByRun(ctx, runID); err != nil {
			return PurgeResult{}, err
		}
		if mutated, err := tx.FlipProjectionStateToJournalExpired(ctx, runID); err != nil {
			return PurgeResult{}, err
		} else if mutated == 0 {
			return PurgeResult{}, fmt.Errorf("flip projection state to journal_expired matched zero rows for run %s", runID)
		}
		result.ProjectionState = publicprojection.StateJournalExpired.String()
		result.Deleted = true
	}

	if err := tx.Commit(ctx); err != nil {
		return PurgeResult{}, err
	}
	return result, nil
}

func (s *Service) RepairRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (RepairResult, error) {
	if s == nil || s.platform == nil {
		return RepairResult{}, errors.New("engine control service is not configured")
	}

	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RepairResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	trace, _, projectionState, err := loadLockedRunTrace(ctx, tx, projectID, runID)
	if err != nil {
		return RepairResult{}, err
	}

	result := RepairResult{
		RunID:           runID,
		ProjectionState: projectionState,
	}

	switch projectionState {
	case publicprojection.StateJournalExpired.String():
		result.Reason = RepairReasonHistoryExpired
	case publicprojection.StateSummaryOnly.String():
		lastProjected := derefInt64(trace.EngineLastProjectedHistoryID)
		latest, err := enginedb.New(tx.Tx()).GetLatestHistoryIDByRun(ctx, runID)
		if err != nil {
			return RepairResult{}, err
		}
		if lastProjected >= latest {
			result.Reason = RepairReasonNoEventsToProject
			break
		}
		if mutated, err := tx.FlipProjectionStateToCatchingUp(ctx, runID); err != nil {
			return RepairResult{}, err
		} else if mutated == 0 {
			return RepairResult{}, fmt.Errorf("flip projection state to catching_up matched zero rows for run %s", runID)
		}
		result.Accepted = true
		result.Reason = RepairReasonRequested
		result.ProjectionState = publicprojection.StateCatchingUp.String()
	case publicprojection.StateCatchingUp.String():
		result.Accepted = true
		result.Reason = RepairReasonAlreadyCatchingUp
	default:
		result.Reason = RepairReasonAlreadyUpToDate
	}

	if err := tx.Commit(ctx); err != nil {
		return RepairResult{}, err
	}
	return result, nil
}

func loadLockedRunTrace(
	ctx context.Context,
	tx *store.Tx,
	projectID uuid.UUID,
	runID uuid.UUID,
) (platformdb.Trace, enginedb.EngineRun, string, error) {
	trace, err := tx.GetTraceByProjectAndEngineRunIDForUpdate(ctx, projectID, runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return platformdb.Trace{}, enginedb.EngineRun{}, "", notFoundError("engine run")
		}
		return platformdb.Trace{}, enginedb.EngineRun{}, "", err
	}

	run, err := enginedb.New(tx.Tx()).GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return platformdb.Trace{}, enginedb.EngineRun{}, "", notFoundError("engine run")
		}
		return platformdb.Trace{}, enginedb.EngineRun{}, "", err
	}

	return trace, run, normalizedProjectionState(trace.EngineProjectionState), nil
}

func normalizedProjectionState(value *string) string {
	state := strings.TrimSpace(derefString(value))
	if state == "" {
		return publicprojection.StateUpToDate.String()
	}
	return state
}

func isTerminalRun(status enginedb.EngineRunLifecycleStatus) bool {
	switch status {
	case enginedb.EngineRunLifecycleStatusCompleted,
		enginedb.EngineRunLifecycleStatusFailed,
		enginedb.EngineRunLifecycleStatusCancelled,
		enginedb.EngineRunLifecycleStatusTerminated:
		return true
	default:
		return false
	}
}

func ensureTerminalShell(
	ctx context.Context,
	tx *store.Tx,
	trace *platformdb.Trace,
	run *enginedb.EngineRun,
) error {
	if trace == nil || run == nil {
		return errors.New("terminal shell requires trace and run")
	}
	completedAt := terminalCompletedAt(run)
	traceStatus, spanStatus := publicprojection.TerminalStatuses(string(run.Status))
	outputPayload, err := publicprojection.TerminalOutputPayload(
		string(run.Status),
		run.Result,
		run.LastErrorCode,
		run.LastErrorMessage,
	)
	if err != nil {
		return err
	}

	if _, err := tx.UpdateEngineTraceSummary(ctx, &platformdb.UpdateEngineTraceSummaryParams{
		EngineRunID:                pgtype.UUID{Bytes: run.ID, Valid: true},
		EngineRunStatus:            stringPtr(string(run.Status)),
		EngineCustomStatus:         cloneRaw(run.CustomStatus),
		EngineWaitState:            cloneRaw(run.WaitingFor),
		EnginePendingActivityTasks: int64Ptr(0),
		EnginePendingInboxItems:    int64Ptr(0),
	}); err != nil {
		return err
	}
	if err := tx.EnsureTerminalTraceShell(ctx, trace.ID, traceStatus, completedAt, outputPayload); err != nil {
		return err
	}
	if err := tx.EnsureTerminalRootSpanShell(
		ctx,
		trace.ID,
		engineRootSpanPrefix+run.ID.String(),
		spanStatus,
		completedAt,
		outputPayload,
		run.LastErrorMessage,
	); err != nil {
		return err
	}
	return nil
}

func terminalCompletedAt(run *enginedb.EngineRun) time.Time {
	if run == nil {
		return time.Time{}
	}
	if run.CompletedAt.Valid {
		return run.CompletedAt.Time
	}
	return run.UpdatedAt
}

func notFoundError(resource string) error {
	return &APIError{
		Code:       "not_found",
		Message:    resource + " not found",
		HTTPStatus: 404,
	}
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func cloneRaw(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return append([]byte(nil), raw...)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
