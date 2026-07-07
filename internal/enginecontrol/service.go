// Package enginecontrol is the platform-side adapter over the engine's
// projection Writer (engine/pkg/projection). It owns control-plane policy —
// terminal-state checks, purge modes, repair acceptance, backfill filtering —
// while every actual write into the projected trace/span tables and the
// engine history journal goes through the Writer, which it shares with the
// engine-side projector.
package enginecontrol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	"github.com/continua-ai/continua/internal/store"
)

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

type ProjectionBackfillAction string

const (
	ProjectionBackfillActionWouldRepair     ProjectionBackfillAction = "would_repair"
	ProjectionBackfillActionRepairRequested ProjectionBackfillAction = "repair_requested"
	ProjectionBackfillActionSkipped         ProjectionBackfillAction = "skipped"
)

type ProjectionBackfillRequest struct {
	DryRun                bool
	Limit                 int
	OlderThan             *time.Time
	EngineInstanceKey     string
	EngineDefinitionName  string
	EngineRunStatus       string
	EngineProjectionState string
}

type ProjectionBackfillRunResult struct {
	RunID           uuid.UUID
	TraceID         string
	ProjectionState string
	Action          ProjectionBackfillAction
	Reason          *RepairReason
}

type ProjectionBackfillResult struct {
	DryRun               bool
	Limit                int
	EligibleCount        int
	RepairRequestedCount int
	SkippedCount         int
	Results              []ProjectionBackfillRunResult
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

	writer := publicprojection.NewWriter(tx.Tx())
	locked, projectionState, err := loadLockedRunTrace(ctx, writer, projectID, runID)
	if err != nil {
		return PurgeResult{}, err
	}
	if !isTerminalRun(locked.Run.Status) {
		return PurgeResult{}, &APIError{
			Code:       "run_not_terminal",
			Message:    "run has not reached a terminal state",
			HTTPStatus: 409,
		}
	}
	if err := writer.EnsureTerminalShell(ctx, locked.TraceID, &locked.Run); err != nil {
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
		if err := writer.PurgeProjectionDetail(ctx, locked.TraceID); err != nil {
			return PurgeResult{}, err
		}
		if mutated, err := writer.MarkProjectionSummaryOnly(ctx, runID); err != nil {
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
		if err := writer.PurgeProjectionDetail(ctx, locked.TraceID); err != nil {
			return PurgeResult{}, err
		}
		if err := writer.DeleteRunJournal(ctx, runID); err != nil {
			return PurgeResult{}, err
		}
		if mutated, err := writer.MarkProjectionJournalExpired(ctx, runID); err != nil {
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

	writer := publicprojection.NewWriter(tx.Tx())
	locked, projectionState, err := loadLockedRunTrace(ctx, writer, projectID, runID)
	if err != nil {
		return RepairResult{}, err
	}
	if err := writer.BackfillTraceLineage(ctx, locked.TraceID, &locked.Run); err != nil {
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
		lastProjected := derefInt64(locked.EngineLastProjectedHistoryID)
		latest, err := writer.LatestJournalHistoryID(ctx, runID)
		if err != nil {
			return RepairResult{}, err
		}
		if lastProjected >= latest {
			result.Reason = RepairReasonNoEventsToProject
			break
		}
		if mutated, err := writer.MarkProjectionCatchingUp(ctx, runID); err != nil {
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

func (s *Service) BackfillProjections(
	ctx context.Context,
	projectID uuid.UUID,
	req *ProjectionBackfillRequest,
) (ProjectionBackfillResult, error) {
	if s == nil || s.platform == nil {
		return ProjectionBackfillResult{}, errors.New("engine control service is not configured")
	}
	if req == nil {
		req = &ProjectionBackfillRequest{}
	}

	result := ProjectionBackfillResult{
		DryRun: req.DryRun,
		Limit:  req.Limit,
	}

	projectionState := normalizeBackfillProjectionState(req.EngineProjectionState)
	if !isBackfillTargetProjectionState(projectionState) {
		return result, nil
	}

	candidates, err := s.platform.ListProjectionBackfillCandidates(ctx, &store.ProjectionBackfillFilter{
		ProjectID:             projectID,
		OlderThan:             req.OlderThan,
		EngineInstanceKey:     strings.TrimSpace(req.EngineInstanceKey),
		EngineDefinitionName:  strings.TrimSpace(req.EngineDefinitionName),
		EngineRunStatus:       strings.ToLower(strings.TrimSpace(req.EngineRunStatus)),
		EngineProjectionState: projectionState,
		Limit:                 req.Limit,
	})
	if err != nil {
		return ProjectionBackfillResult{}, err
	}

	result.EligibleCount = len(candidates)
	result.Results = make([]ProjectionBackfillRunResult, 0, len(candidates))
	for _, candidate := range candidates {
		runResult := ProjectionBackfillRunResult{
			RunID:           candidate.RunID,
			TraceID:         candidate.TraceID,
			ProjectionState: candidate.ProjectionState,
		}

		if req.DryRun {
			runResult.Action = ProjectionBackfillActionWouldRepair
			result.Results = append(result.Results, runResult)
			continue
		}

		repairResult, err := s.RepairRun(ctx, projectID, candidate.RunID)
		if err != nil {
			return ProjectionBackfillResult{}, err
		}

		runResult.ProjectionState = repairResult.ProjectionState
		runResult.Reason = &repairResult.Reason
		if repairResult.Reason == RepairReasonRequested {
			runResult.Action = ProjectionBackfillActionRepairRequested
			result.RepairRequestedCount++
		} else {
			runResult.Action = ProjectionBackfillActionSkipped
			result.SkippedCount++
		}

		result.Results = append(result.Results, runResult)
	}

	return result, nil
}

// loadLockedRunTrace row-locks the run+trace pair through the projection
// writer and normalizes the projection state for control-plane decisions.
func loadLockedRunTrace(
	ctx context.Context,
	writer *publicprojection.Writer,
	projectID uuid.UUID,
	runID uuid.UUID,
) (*publicprojection.LockedRunTrace, string, error) {
	locked, err := writer.LockRunTrace(ctx, projectID, runID)
	if err != nil {
		if errors.Is(err, publicprojection.ErrRunNotFound) {
			return nil, "", notFoundError("engine run")
		}
		return nil, "", err
	}
	return locked, normalizedProjectionState(locked.EngineProjectionState), nil
}

func normalizedProjectionState(value *string) string {
	state := strings.TrimSpace(derefString(value))
	if state == "" {
		return publicprojection.StateUpToDate.String()
	}
	return state
}

func normalizeBackfillProjectionState(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return publicprojection.StateSummaryOnly.String()
}

func isBackfillTargetProjectionState(value string) bool {
	return strings.TrimSpace(value) == publicprojection.StateSummaryOnly.String()
}

func isTerminalRun(status enginedb.EngineRunLifecycleStatus) bool {
	switch status {
	case enginedb.EngineRunLifecycleStatusCompleted,
		enginedb.EngineRunLifecycleStatusFailed,
		enginedb.EngineRunLifecycleStatusCancelled,
		enginedb.EngineRunLifecycleStatusTerminated,
		enginedb.EngineRunLifecycleStatusContinuedAsNew:
		return true
	default:
		return false
	}
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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
