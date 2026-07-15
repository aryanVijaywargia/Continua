package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	"github.com/continua-ai/continua/internal/store"
)

const (
	engineRequestScopeStart = "engine.start"
	engineTracePrefix       = "engine:"
	engineRootSpanPrefix    = "engine:root:"
	engineCancelDedupeKey   = "cancel:"
	engineTerminateCode     = "terminated"
	engineTerminateMessage  = "run terminated by operator"

	engineDefinitionLivenessWindow = 60 * time.Second
)

type engineControlService struct {
	platform        *store.Store
	engine          *enginedb.Queries
	completionGrace time.Duration
}

type engineStartRunRequest struct {
	InstanceKey       string
	DefinitionName    string
	DefinitionVersion string
	RequestKey        string
	Input             json.RawMessage
	Session           *engineStartSession
	Trace             *engineStartTrace
}

type engineStartSession struct {
	Key      string
	Name     string
	Metadata map[string]any
}

type engineStartTrace struct {
	Name        string
	UserID      string
	Tags        []string
	Environment string
	Release     string
	Metadata    map[string]any
}

type engineStartRunResult struct {
	RunID       uuid.UUID
	InstanceKey string
	TraceID     string
}

type engineInstanceResult struct {
	Instance   enginedb.EngineInstance
	CurrentRun engineRunSummary
}

type engineRunSummary struct {
	RunID                uuid.UUID
	InstanceID           uuid.UUID
	InstanceKey          string
	ParentRunID          *uuid.UUID
	RootRunID            *uuid.UUID
	ChildKey             *string
	ChildDepth           *int32
	ContinuedFromRunID   *uuid.UUID
	ContinuedToRunID     *uuid.UUID
	ContinuedFromTraceID *string
	ContinuedToTraceID   *string
	DefinitionName       string
	DefinitionVersion    string
	ProjectionState      string
	Status               enginedb.EngineRunLifecycleStatus
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CompletedAt          *time.Time
	CustomStatus         json.RawMessage
	WaitState            json.RawMessage
	PendingActivityTasks int64
	PendingInboxItems    int64
	Result               json.RawMessage
	LastErrorCode        *string
	LastErrorMessage     *string
}

type engineHistoryPage struct {
	Events    []enginedb.EngineHistory
	Expired   bool
	HasMore   bool
	NextAfter *int
}

type engineSignalRequest struct {
	SignalName string
	Payload    json.RawMessage
}

type engineControlResult struct {
	RunID       uuid.UUID
	InstanceKey string
	Accepted    bool
	WakeApplied bool
}

type enginePendingWorkResult struct {
	RunID       uuid.UUID
	CurrentWait json.RawMessage
	Activities  []enginePendingActivityItem
	Timers      []enginePendingTimerItem
	Signals     []enginePendingSignalItem
}

type enginePendingActivityItem struct {
	TaskID       uuid.UUID
	ActivityKey  string
	ActivityType string
	Status       string
	AvailableAt  time.Time
	AttemptCount int32
}

type enginePendingTimerItem struct {
	InboxID     uuid.UUID
	TimerKey    string
	Status      string
	AvailableAt time.Time
}

type enginePendingSignalItem struct {
	InboxID     uuid.UUID
	SignalName  string
	Status      string
	AvailableAt time.Time
}

type engineDefinitionCatalogResult struct {
	Entry enginedb.EngineDefinitionCatalog
	Live  bool
}

type engineAPIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *engineAPIError) Error() string {
	return e.Code + ": " + e.Message
}

func newEngineControlService(platformStore *store.Store) *engineControlService {
	if platformStore == nil {
		return nil
	}

	return &engineControlService{
		platform: platformStore,
		engine:   enginedb.New(platformStore.Pool()),
	}
}

func engineNotFoundError(message string) *engineAPIError {
	return &engineAPIError{
		Code:       "not_found",
		Message:    message,
		HTTPStatus: 404,
	}
}

func definitionCatalogEntryLive(entry *enginedb.EngineDefinitionCatalog, now time.Time) bool {
	if entry == nil || !entry.Enabled {
		return false
	}
	return !entry.RuntimePublishedAt.Before(now.Add(-engineDefinitionLivenessWindow))
}

func (s *engineControlService) getRunForScope(
	ctx context.Context,
	scope store.Scope,
	runID uuid.UUID,
) (enginedb.EngineRun, error) {
	if projectID, bound := scope.ProjectID(); bound {
		run, err := s.engine.GetRunByProjectAndID(ctx, enginedb.GetRunByProjectAndIDParams{
			ProjectID: projectID,
			ID:        runID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return enginedb.EngineRun{}, engineNotFoundError("engine run not found")
		}
		return run, err
	}

	run, err := s.engine.GetRun(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return enginedb.EngineRun{}, engineNotFoundError("engine run not found")
	}
	return run, err
}

func (s *engineControlService) getInstanceForScopeAndKey(
	ctx context.Context,
	scope store.Scope,
	instanceKey string,
) (enginedb.EngineInstance, error) {
	if projectID, bound := scope.ProjectID(); bound {
		instance, err := s.engine.GetInstanceByProjectAndKey(ctx, enginedb.GetInstanceByProjectAndKeyParams{
			ProjectID:   projectID,
			InstanceKey: instanceKey,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return enginedb.EngineInstance{}, engineNotFoundError("engine instance not found")
		}
		return instance, err
	}

	instances, err := s.engine.ListInstancesByKey(ctx, instanceKey)
	if err != nil {
		return enginedb.EngineInstance{}, err
	}
	switch len(instances) {
	case 0:
		return enginedb.EngineInstance{}, engineNotFoundError("engine instance not found")
	case 1:
		return instances[0], nil
	default:
		return enginedb.EngineInstance{}, &engineAPIError{
			Code:       "ambiguous_instance_key",
			Message:    "project_id is required when instance_key matches multiple projects",
			HTTPStatus: 400,
		}
	}
}

func (s *engineControlService) StartRun(
	ctx context.Context,
	projectID uuid.UUID,
	req *engineStartRunRequest,
) (engineStartRunResult, error) {
	if req == nil {
		return engineStartRunResult{}, &engineAPIError{
			Code:       "invalid_request",
			Message:    "request body is required",
			HTTPStatus: 400,
		}
	}
	if stringsTrimSpaceEmpty(req.InstanceKey) || stringsTrimSpaceEmpty(req.DefinitionName) ||
		stringsTrimSpaceEmpty(req.DefinitionVersion) || stringsTrimSpaceEmpty(req.RequestKey) {
		return engineStartRunResult{}, &engineAPIError{
			Code:       "invalid_request",
			Message:    "instance_key, definition_name, definition_version, and request_key are required",
			HTTPStatus: 400,
		}
	}

	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return engineStartRunResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	engineTx := enginedb.New(tx.Tx())
	claim, err := claimStartRequestDedupe(ctx, engineTx, claimStartRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: engineRequestScopeStart,
		RequestKey:   req.RequestKey,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		return engineStartRunResult{}, err
	}

	switch claim.State {
	case startRequestDedupeClaimStateExistingFinalized:
		return decodeStartRunReplay(&claim.Row)
	case startRequestDedupeClaimStateExistingInProgress:
		return engineStartRunResult{}, &engineAPIError{
			Code:       "request_in_progress",
			Message:    "a start request with this request key is still in progress",
			HTTPStatus: 409,
		}
	}

	now := time.Now().UTC()
	entry, err := engineTx.GetDefinitionCatalogEntry(ctx, enginedb.GetDefinitionCatalogEntryParams{
		DefinitionName:    req.DefinitionName,
		DefinitionVersion: req.DefinitionVersion,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineStartRunResult{}, finalizeStartFailure(ctx, engineTx, tx.Tx(), claim.Row.ID, &engineAPIError{
				Code:       "definition_not_registered",
				Message:    fmt.Sprintf("definition %s@%s is not registered", req.DefinitionName, req.DefinitionVersion),
				HTTPStatus: 400,
			})
		}
		return engineStartRunResult{}, err
	}
	if !entry.Enabled {
		return engineStartRunResult{}, finalizeStartFailure(ctx, engineTx, tx.Tx(), claim.Row.ID, &engineAPIError{
			Code:       "definition_not_registered",
			Message:    fmt.Sprintf("definition %s@%s is disabled", req.DefinitionName, req.DefinitionVersion),
			HTTPStatus: 400,
		})
	}
	if entry.RuntimePublishedAt.Before(now.Add(-engineDefinitionLivenessWindow)) {
		return engineStartRunResult{}, finalizeStartFailure(ctx, engineTx, tx.Tx(), claim.Row.ID, &engineAPIError{
			Code:       "definition_not_registered",
			Message:    fmt.Sprintf("definition %s@%s has no live runtime", req.DefinitionName, req.DefinitionVersion),
			HTTPStatus: 400,
		})
	}

	if _, err := tx.Tx().Exec(ctx, "SAVEPOINT start_create_instance"); err != nil {
		return engineStartRunResult{}, err
	}
	instance, err := engineTx.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    req.InstanceKey,
		DefinitionName: req.DefinitionName,
	})
	if err != nil {
		if _, rollbackErr := tx.Tx().Exec(ctx, "ROLLBACK TO SAVEPOINT start_create_instance"); rollbackErr != nil {
			return engineStartRunResult{}, rollbackErr
		}
		if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT start_create_instance"); releaseErr != nil {
			return engineStartRunResult{}, releaseErr
		}
		if isUniqueViolation(err) {
			return engineStartRunResult{}, finalizeStartFailure(ctx, engineTx, tx.Tx(), claim.Row.ID, &engineAPIError{
				Code:       "instance_conflict",
				Message:    "an instance with this key already exists",
				HTTPStatus: 409,
			})
		}
		return engineStartRunResult{}, err
	}
	if _, err := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT start_create_instance"); err != nil {
		return engineStartRunResult{}, err
	}

	run, err := engineTx.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: req.DefinitionVersion,
		ReadyAt:           now,
	})
	if err != nil {
		return engineStartRunResult{}, err
	}
	if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelRuns); err != nil {
		return engineStartRunResult{}, err
	}

	startedPayload, err := publichistory.MarshalPayload(publichistory.WorkflowStartedPayload{
		DefinitionName:    req.DefinitionName,
		DefinitionVersion: req.DefinitionVersion,
		InstanceKey:       req.InstanceKey,
		Input:             cloneRaw(req.Input),
	})
	if err != nil {
		return engineStartRunResult{}, err
	}
	startedEvent, err := engineTx.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  publichistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		return engineStartRunResult{}, err
	}

	sessionExternalID := req.InstanceKey
	if req.Session != nil && !stringsTrimSpaceEmpty(req.Session.Key) {
		sessionExternalID = req.Session.Key
	}

	session, err := tx.GetOrCreateSessionByExternalID(ctx, projectID, sessionExternalID)
	if err != nil {
		return engineStartRunResult{}, err
	}
	if req.Session != nil {
		mergedSession, mergeErr := mergeSessionUpdate(&session, req.Session)
		if mergeErr != nil {
			return engineStartRunResult{}, mergeErr
		}
		if mergedSession != nil {
			session, err = tx.UpdateSession(ctx, *mergedSession)
			if err != nil {
				return engineStartRunResult{}, err
			}
		}
	}

	traceName := req.DefinitionName
	if req.Trace != nil && !stringsTrimSpaceEmpty(req.Trace.Name) {
		traceName = req.Trace.Name
	}

	traceMetadata, err := marshalJSONObject(req.TraceMetadata())
	if err != nil {
		return engineStartRunResult{}, err
	}

	traceID := engineTracePrefix + run.ID.String()
	projectionState := publicprojection.StateUpToDate.String()
	runStatus := string(enginedb.EngineRunLifecycleStatusQueued)
	traceRow, err := tx.CreateEngineTraceShell(ctx, &platformdb.CreateEngineTraceShellParams{
		ProjectID:                    projectID,
		SessionID:                    pgtype.UUID{Bytes: session.ID, Valid: true},
		TraceID:                      traceID,
		Name:                         stringPtr(traceName),
		UserID:                       stringPtr(req.TraceUserID()),
		Tags:                         req.TraceTags(),
		Environment:                  stringPtr(req.TraceEnvironment()),
		Release:                      stringPtr(req.TraceRelease()),
		Metadata:                     traceMetadata,
		Input:                        cloneRaw(req.Input),
		Output:                       nil,
		Status:                       "running",
		StartTime:                    pgtype.Timestamptz{Time: now, Valid: true},
		EndTime:                      pgtype.Timestamptz{},
		EngineRunID:                  pgtype.UUID{Bytes: run.ID, Valid: true},
		EngineInstanceKey:            stringPtr(req.InstanceKey),
		EngineRunStatus:              &runStatus,
		EngineCustomStatus:           nil,
		EngineWaitState:              nil,
		EnginePendingActivityTasks:   int64Ptr(0),
		EnginePendingInboxItems:      int64Ptr(0),
		EngineDefinitionName:         stringPtr(req.DefinitionName),
		EngineDefinitionVersion:      stringPtr(req.DefinitionVersion),
		EngineParentRunID:            pgtype.UUID{},
		EngineRootRunID:              pgtype.UUID{Bytes: run.ID, Valid: true},
		EngineChildKey:               nil,
		EngineChildDepth:             int32Pointer(0),
		EngineProjectionState:        &projectionState,
		EngineLatestHistoryID:        int64Ptr(startedEvent.ID),
		EngineLastProjectedHistoryID: int64Ptr(startedEvent.ID),
		EngineProjectionUpdatedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return engineStartRunResult{}, err
	}

	rootSpanID := engineRootSpanPrefix + run.ID.String()
	if _, err := tx.CreateSpan(ctx, &platformdb.CreateSpanParams{
		ProjectID: projectID,
		TraceID:   traceRow.ID,
		SpanID:    rootSpanID,
		Name:      traceName,
		Type:      "chain",
		Status:    "running",
		Level:     "default",
		StartTime: now,
		Input:     cloneRaw(req.Input),
		Metadata:  nil,
		Depth:     int32Pointer(0),
	}); err != nil {
		return engineStartRunResult{}, err
	}

	result := engineStartRunResult{
		RunID:       run.ID,
		InstanceKey: req.InstanceKey,
		TraceID:     traceID,
	}
	if err := finalizeStartSuccess(ctx, engineTx, tx.Tx(), claim.Row.ID, result); err != nil {
		return engineStartRunResult{}, err
	}

	return result, nil
}

func (s *engineControlService) GetInstance(
	ctx context.Context,
	scope store.Scope,
	instanceKey string,
) (engineInstanceResult, error) {
	instance, err := s.getInstanceForScopeAndKey(ctx, scope, instanceKey)
	if err != nil {
		return engineInstanceResult{}, err
	}

	run, err := s.engine.GetLatestRunByInstance(ctx, instance.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineInstanceResult{}, engineNotFoundError("engine run not found")
		}
		return engineInstanceResult{}, err
	}

	summary, err := s.buildRunSummary(ctx, &instance, &run)
	if err != nil {
		return engineInstanceResult{}, err
	}

	return engineInstanceResult{
		Instance:   instance,
		CurrentRun: summary,
	}, nil
}

func (s *engineControlService) GetRun(
	ctx context.Context,
	scope store.Scope,
	runID uuid.UUID,
) (engineRunSummary, error) {
	run, err := s.getRunForScope(ctx, scope, runID)
	if err != nil {
		return engineRunSummary{}, err
	}

	instance, err := s.engine.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return engineRunSummary{}, err
	}

	return s.buildRunSummary(ctx, &instance, &run)
}

func (s *engineControlService) ListDefinitions(ctx context.Context) ([]engineDefinitionCatalogResult, error) {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := enginedb.New(tx.Tx()).ListDefinitionCatalog(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	definitions := make([]engineDefinitionCatalogResult, len(rows))
	for i := range rows {
		definitions[i] = engineDefinitionCatalogResult{
			Entry: rows[i],
			Live:  definitionCatalogEntryLive(&rows[i], now),
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return definitions, nil
}

func (s *engineControlService) GetRunResult(
	ctx context.Context,
	scope store.Scope,
	runID uuid.UUID,
) (engineRunSummary, error) {
	summary, err := s.GetRun(ctx, scope, runID)
	if err != nil {
		return engineRunSummary{}, err
	}

	if !isTerminalEngineRun(summary.Status) {
		return engineRunSummary{}, &engineAPIError{
			Code:       "run_not_terminal",
			Message:    "run has not reached a terminal state",
			HTTPStatus: 409,
		}
	}

	if summary.Status == enginedb.EngineRunLifecycleStatusCancelled ||
		summary.Status == enginedb.EngineRunLifecycleStatusTerminated ||
		summary.Status == enginedb.EngineRunLifecycleStatusContinuedAsNew {
		summary.Result = nil
	}

	return summary, nil
}

func (s *engineControlService) GetRunPendingWork(
	ctx context.Context,
	scope store.Scope,
	runID uuid.UUID,
) (enginePendingWorkResult, error) {
	run, err := s.getRunForScope(ctx, scope, runID)
	if err != nil {
		return enginePendingWorkResult{}, err
	}

	activities, err := s.engine.ListOpenActivityTasksByRun(ctx, run.ID)
	if err != nil {
		return enginePendingWorkResult{}, err
	}
	timers, err := s.engine.ListOpenInboxItemsByRunAndKind(ctx, enginedb.ListOpenInboxItemsByRunAndKindParams{
		RunID: pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:  "timer",
	})
	if err != nil {
		return enginePendingWorkResult{}, err
	}
	signals, err := s.engine.ListOpenInboxItemsByRunAndKind(ctx, enginedb.ListOpenInboxItemsByRunAndKindParams{
		RunID: pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:  "signal",
	})
	if err != nil {
		return enginePendingWorkResult{}, err
	}

	result := enginePendingWorkResult{
		RunID:       run.ID,
		CurrentWait: cloneRaw(run.WaitingFor),
		Activities:  make([]enginePendingActivityItem, 0, len(activities)),
		Timers:      make([]enginePendingTimerItem, 0, len(timers)),
		Signals:     make([]enginePendingSignalItem, 0, len(signals)),
	}

	for i := range activities {
		task := activities[i]
		result.Activities = append(result.Activities, enginePendingActivityItem{
			TaskID:       task.ID,
			ActivityKey:  task.ActivityKey,
			ActivityType: task.ActivityType,
			Status:       string(task.Status),
			AvailableAt:  task.AvailableAt,
			AttemptCount: task.AttemptCount,
		})
	}

	for i := range timers {
		inboxRow := timers[i]
		timerKey, err := decodePendingTimerKey(inboxRow.Payload)
		if err != nil {
			return enginePendingWorkResult{}, err
		}
		result.Timers = append(result.Timers, enginePendingTimerItem{
			InboxID:     inboxRow.ID,
			TimerKey:    timerKey,
			Status:      string(inboxRow.Status),
			AvailableAt: inboxRow.AvailableAt,
		})
	}

	for i := range signals {
		inboxRow := signals[i]
		signalName, err := decodePendingSignalName(inboxRow.Payload)
		if err != nil {
			return enginePendingWorkResult{}, err
		}
		result.Signals = append(result.Signals, enginePendingSignalItem{
			InboxID:     inboxRow.ID,
			SignalName:  signalName,
			Status:      string(inboxRow.Status),
			AvailableAt: inboxRow.AvailableAt,
		})
	}

	return result, nil
}

func (s *engineControlService) GetRunHistory(
	ctx context.Context,
	scope store.Scope,
	runID uuid.UUID,
	after int,
	limit int,
) (engineHistoryPage, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if after < 0 {
		after = 0
	}

	run, err := s.getRunForScope(ctx, scope, runID)
	if err != nil {
		return engineHistoryPage{}, err
	}

	rows, err := s.engine.ListHistoryByRunAfterSequence(ctx, enginedb.ListHistoryByRunAfterSequenceParams{
		RunID:      runID,
		SequenceNo: int32(after),
		Limit:      int32(limit + 1),
	})
	if err != nil {
		return engineHistoryPage{}, err
	}

	page := engineHistoryPage{
		Events: rows,
	}
	projectionState, err := s.projectionStateForRun(ctx, run.ProjectID, runID)
	if err != nil {
		return engineHistoryPage{}, err
	}
	page.Expired = projectionState == publicprojection.StateJournalExpired.String()
	if len(rows) > limit {
		page.HasMore = true
		page.Events = rows[:limit]
	}
	if len(page.Events) > 0 {
		nextAfter := int(page.Events[len(page.Events)-1].SequenceNo)
		page.NextAfter = &nextAfter
	}

	return page, nil
}

func (s *engineControlService) SignalRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
	req engineSignalRequest,
) (engineControlResult, error) {
	if stringsTrimSpaceEmpty(req.SignalName) {
		return engineControlResult{}, &engineAPIError{
			Code:       "invalid_request",
			Message:    "signal_name is required",
			HTTPStatus: 400,
		}
	}

	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return engineControlResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	engineTx := enginedb.New(tx.Tx())
	run, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineControlResult{}, &engineAPIError{
				Code:       "not_found",
				Message:    "engine run not found",
				HTTPStatus: 404,
			}
		}
		return engineControlResult{}, err
	}
	if isTerminalEngineRun(run.Status) {
		return engineControlResult{}, &engineAPIError{
			Code:       "run_terminal",
			Message:    "cannot signal a terminal run",
			HTTPStatus: 409,
		}
	}

	instance, err := engineTx.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return engineControlResult{}, err
	}

	payload, err := publichistory.MarshalPayload(publichistory.SignalReceivedPayload{
		SignalName: req.SignalName,
		Payload:    cloneRaw(req.Payload),
	})
	if err != nil {
		return engineControlResult{}, err
	}
	if _, err := engineTx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "signal",
		Payload:     payload,
		AvailableAt: time.Now().UTC(),
	}); err != nil {
		return engineControlResult{}, err
	}
	if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelInbox); err != nil {
		return engineControlResult{}, err
	}

	currentRun, wakeApplied, err := wakeRunIfWaiting(ctx, engineTx, &run)
	if err != nil {
		return engineControlResult{}, err
	}
	if wakeApplied {
		if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelRuns); err != nil {
			return engineControlResult{}, err
		}
	}
	if err := syncProjectedTraceSummary(ctx, tx, engineTx, &currentRun); err != nil {
		return engineControlResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return engineControlResult{}, err
	}

	return engineControlResult{
		RunID:       run.ID,
		InstanceKey: instance.InstanceKey,
		Accepted:    true,
		WakeApplied: wakeApplied,
	}, nil
}

func (s *engineControlService) TerminateRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (engineRunSummary, error) {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return engineRunSummary{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	engineTx := enginedb.New(tx.Tx())
	run, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineRunSummary{}, &engineAPIError{
				Code:       "not_found",
				Message:    "engine run not found",
				HTTPStatus: 404,
			}
		}
		return engineRunSummary{}, err
	}

	if isTerminalEngineRun(run.Status) {
		if err := tx.Commit(ctx); err != nil {
			return engineRunSummary{}, err
		}
		return terminalRunSummaryFromRun(&run), nil
	}

	descendants, err := engineTx.ListActiveChildWorkflowDescendants(ctx, enginedb.ListActiveChildWorkflowDescendantsParams{
		ProjectID:   projectID,
		ParentRunID: run.ID,
	})
	if err != nil {
		return engineRunSummary{}, err
	}

	for i := range descendants {
		descendantRun, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
			ProjectID: projectID,
			ID:        descendants[i].CurrentChildRunID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return engineRunSummary{}, fmt.Errorf("terminate descendant invariant failed for run %s", descendants[i].CurrentChildRunID)
			}
			return engineRunSummary{}, err
		}
		if _, err := terminateLockedRun(ctx, tx, engineTx, &descendantRun, false); err != nil {
			return engineRunSummary{}, err
		}
	}

	updatedRun, err := terminateLockedRun(ctx, tx, engineTx, &run, true)
	if err != nil {
		return engineRunSummary{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return engineRunSummary{}, err
	}
	return terminalRunSummaryFromRun(&updatedRun), nil
}

func (s *engineControlService) SuspendRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (engineRunSummary, error) {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return engineRunSummary{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	engineTx := enginedb.New(tx.Tx())
	run, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineRunSummary{}, &engineAPIError{
				Code:       "not_found",
				Message:    "engine run not found",
				HTTPStatus: 404,
			}
		}
		return engineRunSummary{}, err
	}

	instance, err := engineTx.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return engineRunSummary{}, err
	}

	switch run.Status {
	case enginedb.EngineRunLifecycleStatusSuspended:
		if err := tx.Commit(ctx); err != nil {
			return engineRunSummary{}, err
		}
		return s.buildRunSummary(ctx, &instance, &run)
	case enginedb.EngineRunLifecycleStatusQuarantined:
		return engineRunSummary{}, &engineAPIError{
			Code:       "run_not_suspendable",
			Message:    "cannot suspend a quarantined run; resume or terminate it",
			HTTPStatus: 409,
		}
	case enginedb.EngineRunLifecycleStatusRunning:
		return engineRunSummary{}, &engineAPIError{
			Code:       "run_not_suspendable",
			Message:    "cannot suspend a running run; wait for the current activation to finish",
			HTTPStatus: 409,
		}
	}
	if isTerminalEngineRun(run.Status) {
		return engineRunSummary{}, &engineAPIError{
			Code:       "run_terminal",
			Message:    "cannot suspend a terminal run",
			HTTPStatus: 409,
		}
	}

	updatedRun, err := engineTx.TransitionRunToSuspended(ctx, run.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineRunSummary{}, fmt.Errorf("suspend transition invariant failed for run %s", run.ID)
		}
		return engineRunSummary{}, err
	}

	payload, err := publichistory.MarshalPayload(publichistory.WorkflowSuspendedPayload{})
	if err != nil {
		return engineRunSummary{}, err
	}
	appended, err := appendLockedRunHistoryEvent(ctx, engineTx, &updatedRun, publichistory.EventWorkflowSuspended, payload)
	if err != nil {
		return engineRunSummary{}, err
	}
	if err := updateProjectedTraceLatestHistory(ctx, tx, updatedRun.ID, appended.ID); err != nil {
		return engineRunSummary{}, err
	}
	if err := syncProjectedTraceSummary(ctx, tx, engineTx, &updatedRun); err != nil {
		return engineRunSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return engineRunSummary{}, err
	}
	return s.buildRunSummary(ctx, &instance, &updatedRun)
}

func terminateLockedRun(
	ctx context.Context,
	tx *store.Tx,
	engineTx *enginedb.Queries,
	run *enginedb.EngineRun,
	wakeParent bool,
) (enginedb.EngineRun, error) {
	if tx == nil || engineTx == nil || run == nil {
		return enginedb.EngineRun{}, errors.New("transaction, engine queries, and run are required")
	}

	payload, err := publichistory.MarshalPayload(publichistory.WorkflowTerminatedPayload{
		ErrorCode:    engineTerminateCode,
		ErrorMessage: engineTerminateMessage,
	})
	if err != nil {
		return enginedb.EngineRun{}, err
	}
	appended, err := appendLockedRunHistoryEvent(ctx, engineTx, run, publichistory.EventWorkflowTerminated, payload)
	if err != nil {
		return enginedb.EngineRun{}, err
	}
	if err := updateProjectedTraceLatestHistory(ctx, tx, run.ID, appended.ID); err != nil {
		return enginedb.EngineRun{}, err
	}

	updatedRun, err := engineTx.TransitionRunToTerminated(ctx, run.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return enginedb.EngineRun{}, fmt.Errorf("terminate transition invariant failed for run %s", run.ID)
		}
		return enginedb.EngineRun{}, err
	}
	if _, err := engineTx.CancelOpenActivityTasksByRun(ctx, run.ID); err != nil {
		return enginedb.EngineRun{}, err
	}
	if _, err := engineTx.DiscardOpenInboxItemsByRun(ctx, pgtype.UUID{Bytes: run.ID, Valid: true}); err != nil {
		return enginedb.EngineRun{}, err
	}
	if _, err := engineTx.UpdateInstanceStatus(ctx, enginedb.UpdateInstanceStatusParams{
		ID:     run.InstanceID,
		Status: enginedb.EngineInstanceLifecycleStatusTerminated,
	}); err != nil {
		return enginedb.EngineRun{}, err
	}
	if err := terminateChildRelationship(ctx, engineTx, &updatedRun, wakeParent); err != nil {
		return enginedb.EngineRun{}, err
	}
	return updatedRun, nil
}

func terminateChildRelationship(
	ctx context.Context,
	engineTx *enginedb.Queries,
	run *enginedb.EngineRun,
	wakeParent bool,
) error {
	if engineTx == nil || run == nil || !run.ParentRunID.Valid {
		return nil
	}
	childWorkflow, err := engineTx.UpdateChildWorkflowTerminal(ctx, enginedb.UpdateChildWorkflowTerminalParams{
		ProjectID:          run.ProjectID,
		CurrentChildRunID:  run.ID,
		TerminalChildRunID: pgtype.UUID{Bytes: run.ID, Valid: true},
		Status:             enginedb.EngineChildWorkflowStatusTerminated,
	})
	if err != nil {
		return err
	}
	if !wakeParent || childWorkflow.ParentWaitFailedAt.Valid {
		return nil
	}
	if _, err := engineTx.WakeWaitingChildWorkflowRun(ctx, enginedb.WakeWaitingChildWorkflowRunParams{
		ID:       childWorkflow.ParentRunID,
		ChildKey: childWorkflow.ChildKey,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

func (s *engineControlService) ResumeRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (engineRunSummary, error) {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return engineRunSummary{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	engineTx := enginedb.New(tx.Tx())
	run, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineRunSummary{}, &engineAPIError{
				Code:       "not_found",
				Message:    "engine run not found",
				HTTPStatus: 404,
			}
		}
		return engineRunSummary{}, err
	}

	instance, err := engineTx.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return engineRunSummary{}, err
	}

	if isTerminalEngineRun(run.Status) {
		return engineRunSummary{}, &engineAPIError{
			Code:       "run_terminal",
			Message:    "cannot resume a terminal run",
			HTTPStatus: 409,
		}
	}
	if run.Status != enginedb.EngineRunLifecycleStatusSuspended &&
		run.Status != enginedb.EngineRunLifecycleStatusQuarantined {
		if err := tx.Commit(ctx); err != nil {
			return engineRunSummary{}, err
		}
		return s.buildRunSummary(ctx, &instance, &run)
	}

	var updatedRun enginedb.EngineRun
	if run.Status == enginedb.EngineRunLifecycleStatusQuarantined {
		updatedRun, err = engineTx.TransitionRunToQueuedFromQuarantined(ctx, run.ID)
	} else {
		updatedRun, err = engineTx.TransitionRunToQueuedFromSuspended(ctx, run.ID)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineRunSummary{}, fmt.Errorf("resume transition invariant failed for run %s", run.ID)
		}
		return engineRunSummary{}, err
	}
	if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelRuns); err != nil {
		return engineRunSummary{}, err
	}

	payload, err := publichistory.MarshalPayload(publichistory.WorkflowResumedPayload{})
	if err != nil {
		return engineRunSummary{}, err
	}
	appended, err := appendLockedRunHistoryEvent(ctx, engineTx, &updatedRun, publichistory.EventWorkflowResumed, payload)
	if err != nil {
		return engineRunSummary{}, err
	}
	if err := updateProjectedTraceLatestHistory(ctx, tx, updatedRun.ID, appended.ID); err != nil {
		return engineRunSummary{}, err
	}
	if err := syncProjectedTraceSummary(ctx, tx, engineTx, &updatedRun); err != nil {
		return engineRunSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return engineRunSummary{}, err
	}
	return s.buildRunSummary(ctx, &instance, &updatedRun)
}

// CancelRun is cooperative only: success enqueues cancel intent and may wake the
// run, but the workflow decides between COMPLETED and CANCELLED by returning
// workflow.ErrCancelled on a later activation.
func (s *engineControlService) CancelRun(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
) (engineControlResult, error) {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return engineControlResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	engineTx := enginedb.New(tx.Tx())
	run, err := engineTx.GetRunByProjectAndIDForUpdate(ctx, enginedb.GetRunByProjectAndIDForUpdateParams{
		ProjectID: projectID,
		ID:        runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engineControlResult{}, &engineAPIError{
				Code:       "not_found",
				Message:    "engine run not found",
				HTTPStatus: 404,
			}
		}
		return engineControlResult{}, err
	}
	if isTerminalEngineRun(run.Status) {
		return engineControlResult{}, &engineAPIError{
			Code:       "run_terminal",
			Message:    "cannot cancel a terminal run",
			HTTPStatus: 409,
		}
	}

	instance, err := engineTx.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return engineControlResult{}, err
	}

	cancelPayload, err := publichistory.MarshalPayload(publichistory.CancelRequestedPayload{})
	if err != nil {
		return engineControlResult{}, err
	}
	cancelDedupeKey := engineCancelDedupeKey + run.ID.String()
	if _, err := tx.Tx().Exec(ctx, "SAVEPOINT cancel_enqueue"); err != nil {
		return engineControlResult{}, err
	}
	if _, err := engineTx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  run.InstanceID,
		RunID:       pgtype.UUID{Bytes: run.ID, Valid: true},
		Kind:        "cancel",
		Payload:     cancelPayload,
		AvailableAt: time.Now().UTC(),
		DedupeKey:   &cancelDedupeKey,
	}); err != nil {
		if _, rollbackErr := tx.Tx().Exec(ctx, "ROLLBACK TO SAVEPOINT cancel_enqueue"); rollbackErr != nil {
			return engineControlResult{}, rollbackErr
		}
		if _, releaseErr := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT cancel_enqueue"); releaseErr != nil {
			return engineControlResult{}, releaseErr
		}
		if !isUniqueViolation(err) {
			return engineControlResult{}, err
		}
	} else {
		if _, err := tx.Tx().Exec(ctx, "RELEASE SAVEPOINT cancel_enqueue"); err != nil {
			return engineControlResult{}, err
		}
		if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelInbox); err != nil {
			return engineControlResult{}, err
		}
	}

	currentRun, wakeApplied, err := wakeRunIfWaiting(ctx, engineTx, &run)
	if err != nil {
		return engineControlResult{}, err
	}
	if wakeApplied {
		if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelRuns); err != nil {
			return engineControlResult{}, err
		}
	}
	if err := syncProjectedTraceSummary(ctx, tx, engineTx, &currentRun); err != nil {
		return engineControlResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return engineControlResult{}, err
	}

	return engineControlResult{
		RunID:       run.ID,
		InstanceKey: instance.InstanceKey,
		Accepted:    true,
		WakeApplied: wakeApplied,
	}, nil
}

func (s *engineControlService) ReadRunSummary(
	ctx context.Context,
	scope store.Scope,
	runID uuid.UUID,
) (engineRunSummary, error) {
	return s.GetRun(ctx, scope, runID)
}

func (s *engineControlService) buildRunSummary(
	ctx context.Context,
	instance *enginedb.EngineInstance,
	run *enginedb.EngineRun,
) (engineRunSummary, error) {
	if instance == nil || run == nil {
		return engineRunSummary{}, errors.New("instance and run are required")
	}
	pendingActivityTasks, err := s.engine.CountOpenActivityTasksByRun(ctx, run.ID)
	if err != nil {
		return engineRunSummary{}, err
	}

	// CountOpenInboxByRun intentionally excludes cancel inbox rows so read APIs
	// reflect only operator-visible pending work: timers plus signals.
	pendingInboxItems, err := s.engine.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: run.ID, Valid: true})
	if err != nil {
		return engineRunSummary{}, err
	}

	projectionState, err := s.projectionStateForRun(ctx, run.ProjectID, run.ID)
	if err != nil {
		return engineRunSummary{}, err
	}

	return engineRunSummary{
		RunID:                run.ID,
		InstanceID:           instance.ID,
		InstanceKey:          instance.InstanceKey,
		ParentRunID:          pgUUIDPtr(run.ParentRunID),
		RootRunID:            cloneUUID(run.RootRunID),
		ChildKey:             cloneStringPtr(run.ChildKey),
		ChildDepth:           int32Pointer(run.ChildDepth),
		ContinuedFromRunID:   pgUUIDPtr(run.ContinuedFromRunID),
		ContinuedToRunID:     pgUUIDPtr(run.ContinuedToRunID),
		ContinuedFromTraceID: engineTraceIDPtr(pgUUIDPtr(run.ContinuedFromRunID)),
		ContinuedToTraceID:   engineTraceIDPtr(pgUUIDPtr(run.ContinuedToRunID)),
		DefinitionName:       instance.DefinitionName,
		DefinitionVersion:    run.DefinitionVersion,
		ProjectionState:      projectionState,
		Status:               run.Status,
		CreatedAt:            run.CreatedAt,
		UpdatedAt:            run.UpdatedAt,
		CompletedAt:          pgTimePtr(run.CompletedAt),
		CustomStatus:         cloneRaw(run.CustomStatus),
		WaitState:            cloneRaw(run.WaitingFor),
		PendingActivityTasks: pendingActivityTasks,
		PendingInboxItems:    pendingInboxItems,
		Result:               cloneRaw(run.Result),
		LastErrorCode:        run.LastErrorCode,
		LastErrorMessage:     run.LastErrorMessage,
	}, nil
}

func (req *engineStartRunRequest) TraceMetadata() map[string]any {
	if req.Trace == nil {
		return nil
	}
	return req.Trace.Metadata
}

func (s *engineControlService) projectionStateForRun(ctx context.Context, projectID, runID uuid.UUID) (string, error) {
	trace, err := s.platform.Queries().GetTraceByExternalID(ctx, platformdb.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   engineTracePrefix + runID.String(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return publicprojection.StateUpToDate.String(), nil
		}
		return "", err
	}
	return s.effectiveProjectionState(ctx, runID, normalizedProjectionState(trace.EngineProjectionState), trace.EngineLastProjectedHistoryID)
}

func (s *engineControlService) shouldReadLiveTraceSummary(
	ctx context.Context,
	trace *store.TraceRead,
) (bool, error) {
	if shouldReadLiveEngineSummary(trace) {
		return true, nil
	}
	if trace == nil || !trace.EngineRunID.Valid {
		return false, nil
	}
	if normalizedProjectionState(trace.EngineProjectionState) == publicprojection.StateJournalExpired.String() {
		return false, nil
	}
	return s.traceProjectionCheckpointStale(ctx, uuid.UUID(trace.EngineRunID.Bytes), trace.EngineLastProjectedHistoryID)
}

func (s *engineControlService) projectionStateForTrace(ctx context.Context, trace *store.TraceRead) (string, error) {
	if trace == nil || !trace.EngineRunID.Valid {
		return publicprojection.StateUpToDate.String(), nil
	}
	return s.effectiveProjectionState(
		ctx,
		uuid.UUID(trace.EngineRunID.Bytes),
		normalizedProjectionState(trace.EngineProjectionState),
		trace.EngineLastProjectedHistoryID,
	)
}

func (s *engineControlService) effectiveProjectionState(
	ctx context.Context,
	runID uuid.UUID,
	state string,
	lastProjectedHistoryID *int64,
) (string, error) {
	stale, err := s.traceProjectionCheckpointStale(ctx, runID, lastProjectedHistoryID)
	if err != nil {
		return "", err
	}
	if stale && state == publicprojection.StateUpToDate.String() {
		return publicprojection.StateCatchingUp.String(), nil
	}
	return state, nil
}

func (s *engineControlService) traceProjectionCheckpointStale(
	ctx context.Context,
	runID uuid.UUID,
	lastProjectedHistoryID *int64,
) (bool, error) {
	latestHistoryID, err := s.engine.GetLatestHistoryIDByRun(ctx, runID)
	if err != nil {
		return false, err
	}
	return latestHistoryID > derefInt64(lastProjectedHistoryID), nil
}

func (req *engineStartRunRequest) TraceUserID() string {
	if req.Trace == nil {
		return ""
	}
	return req.Trace.UserID
}

func (req *engineStartRunRequest) TraceTags() []string {
	if req.Trace == nil {
		return nil
	}
	return append([]string(nil), req.Trace.Tags...)
}

func (req *engineStartRunRequest) TraceEnvironment() string {
	if req.Trace == nil {
		return ""
	}
	return req.Trace.Environment
}

func (req *engineStartRunRequest) TraceRelease() string {
	if req.Trace == nil {
		return ""
	}
	return req.Trace.Release
}

func mergeSessionUpdate(
	session *platformdb.Session,
	input *engineStartSession,
) (*platformdb.UpdateSessionParams, error) {
	if input == nil || session == nil {
		return nil, nil
	}

	existingMetadata, err := parseJSONObjectBytes(session.Metadata)
	if err != nil {
		return nil, err
	}

	updated := false
	name := session.Name
	if !stringsTrimSpaceEmpty(input.Name) {
		name = stringPtr(input.Name)
		updated = true
	}

	mergedMetadata := mergeJSONObject(existingMetadata, input.Metadata)
	if len(input.Metadata) > 0 {
		updated = true
	}
	metadataBytes, err := marshalJSONObject(mergedMetadata)
	if err != nil {
		return nil, err
	}

	if !updated {
		return nil, nil
	}

	return &platformdb.UpdateSessionParams{
		ID:       session.ID,
		Name:     name,
		Metadata: metadataBytes,
	}, nil
}

func mergeJSONObject(base, incoming map[string]any) map[string]any {
	if len(base) == 0 && len(incoming) == 0 {
		return map[string]any{}
	}

	merged := make(map[string]any, len(base)+len(incoming))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range incoming {
		merged[key] = value
	}
	return merged
}

func parseJSONObjectBytes(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

func marshalJSONObject(value map[string]any) ([]byte, error) {
	if len(value) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

func wakeRunIfWaiting(
	ctx context.Context,
	engineTx *enginedb.Queries,
	run *enginedb.EngineRun,
) (enginedb.EngineRun, bool, error) {
	if run == nil {
		return enginedb.EngineRun{}, false, errors.New("run is required")
	}
	updatedRun, err := engineTx.WakeWaitingRun(ctx, run.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return *run, false, nil
		}
		return enginedb.EngineRun{}, false, err
	}
	return updatedRun, true, nil
}

func syncProjectedTraceSummary(
	ctx context.Context,
	tx *store.Tx,
	engineTx *enginedb.Queries,
	run *enginedb.EngineRun,
) error {
	if run == nil {
		return errors.New("run is required")
	}
	pendingActivityTasks, err := engineTx.CountOpenActivityTasksByRun(ctx, run.ID)
	if err != nil {
		return err
	}

	// Projected pending inbox counts intentionally exclude cancel rows to match
	// GET /pending-work and the live engine run summary.
	pendingInboxItems, err := engineTx.CountOpenInboxByRun(ctx, pgtype.UUID{Bytes: run.ID, Valid: true})
	if err != nil {
		return err
	}

	_, err = tx.UpdateEngineTraceSummary(ctx, &platformdb.UpdateEngineTraceSummaryParams{
		EngineRunID:                pgtype.UUID{Bytes: run.ID, Valid: true},
		EngineRunStatus:            stringPtr(string(run.Status)),
		EngineCustomStatus:         cloneRaw(run.CustomStatus),
		EngineWaitState:            cloneRaw(run.WaitingFor),
		EnginePendingActivityTasks: int64Ptr(pendingActivityTasks),
		EnginePendingInboxItems:    int64Ptr(pendingInboxItems),
	})
	return err
}

func isTerminalEngineRun(status enginedb.EngineRunLifecycleStatus) bool {
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

func terminalRunSummaryFromRun(run *enginedb.EngineRun) engineRunSummary {
	if run == nil {
		return engineRunSummary{}
	}

	continuedFromRunID := pgUUIDPtr(run.ContinuedFromRunID)
	continuedToRunID := pgUUIDPtr(run.ContinuedToRunID)

	return engineRunSummary{
		ParentRunID:          pgUUIDPtr(run.ParentRunID),
		RootRunID:            cloneUUID(run.RootRunID),
		ChildKey:             cloneStringPtr(run.ChildKey),
		ChildDepth:           int32Pointer(run.ChildDepth),
		RunID:                run.ID,
		ContinuedFromRunID:   continuedFromRunID,
		ContinuedToRunID:     continuedToRunID,
		ContinuedFromTraceID: engineTraceIDPtr(continuedFromRunID),
		ContinuedToTraceID:   engineTraceIDPtr(continuedToRunID),
		Status:               run.Status,
		Result:               cloneRaw(run.Result),
		LastErrorCode:        run.LastErrorCode,
		LastErrorMessage:     run.LastErrorMessage,
		PendingActivityTasks: 0,
		PendingInboxItems:    0,
	}
}

func decodePendingTimerKey(raw json.RawMessage) (string, error) {
	var payload publichistory.TimerScheduledPayload
	if err := publichistory.UnmarshalPayload(raw, &payload); err != nil {
		return "", fmt.Errorf("decode timer payload: %w", err)
	}
	return payload.TimerKey, nil
}

func decodePendingSignalName(raw json.RawMessage) (string, error) {
	var payload publichistory.SignalReceivedPayload
	if err := publichistory.UnmarshalPayload(raw, &payload); err != nil {
		return "", fmt.Errorf("decode signal payload: %w", err)
	}
	return payload.SignalName, nil
}

func pgTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func cloneUUID(value uuid.UUID) *uuid.UUID {
	cloned := value
	return &cloned
}
