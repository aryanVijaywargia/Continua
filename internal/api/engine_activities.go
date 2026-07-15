package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publichistory "github.com/continua-ai/continua/engine/pkg/history"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
	"github.com/continua-ai/continua/internal/store"
)

const (
	remoteActivityWorkerIDMaxLength     = 128
	remoteActivityTypeMaxLength         = 256
	remoteActivityErrorCodeMaxLength    = 128
	remoteActivityErrorMessageMaxLength = 4096

	remoteActivityDefaultMaxTasks  = 10
	remoteActivityMaxTasks         = 50
	remoteActivityMaxActivityTypes = 50

	remoteActivityMinLease     = 10 * time.Second
	remoteActivityDefaultLease = 60 * time.Second
	remoteActivityMaxLease     = 5 * time.Minute
)

type remoteActivityClaimRequest struct {
	WorkerID      string
	ActivityTypes []string
	MaxTasks      int32
	LeaseDuration time.Duration
}

type remoteActivityClaimResult struct {
	Tasks []enginedb.EngineActivityTask
}

type remoteActivityFailRequest struct {
	WorkerID     string
	ErrorCode    string
	ErrorMessage string
	NonRetryable bool
}

func (s *Server) ClaimRemoteActivityTasks(
	w http.ResponseWriter,
	r *http.Request,
	_ ClaimRemoteActivityTasksParams,
) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	var req EngineRemoteActivityClaimRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	claimReq, err := remoteActivityClaimRequestFromAPI(&req)
	if err != nil {
		writeEngineError(w, err, "Failed to validate remote activity claim request")
		return
	}

	result, err := s.engineControl.ClaimRemoteActivityTasks(r.Context(), projectID, claimReq)
	if err != nil {
		writeEngineError(w, err, "Failed to claim remote activity tasks")
		return
	}

	resp := EngineRemoteActivityClaimResponse{Tasks: make([]EngineRemoteActivityTask, 0, len(result.Tasks))}
	for i := range result.Tasks {
		task, mapErr := remoteActivityTaskToAPI(&result.Tasks[i])
		if mapErr != nil {
			writeEngineError(w, mapErr, "Failed to map remote activity tasks")
			return
		}
		resp.Tasks = append(resp.Tasks, task)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) HeartbeatRemoteActivityTask(
	w http.ResponseWriter,
	r *http.Request,
	id openapi_types.UUID,
	_ HeartbeatRemoteActivityTaskParams,
) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	var req EngineRemoteActivityHeartbeatRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	workerID, err := validateRemoteActivityWorkerID(req.WorkerId)
	if err != nil {
		writeEngineError(w, err, "Failed to validate remote activity heartbeat request")
		return
	}

	task, err := s.engineControl.HeartbeatRemoteActivityTask(r.Context(), projectID, id, workerID)
	if err != nil {
		writeEngineError(w, err, "Failed to heartbeat remote activity task")
		return
	}
	writeJSON(w, http.StatusOK, EngineRemoteActivityHeartbeatResponse{
		LeaseExpiresAt:           remoteActivityLeaseExpiresAt(&task),
		EffectiveLeaseDurationMs: remoteActivityLeaseDurationMS(&task),
	})
}

func (s *Server) CompleteRemoteActivityTask(
	w http.ResponseWriter,
	r *http.Request,
	id openapi_types.UUID,
	_ CompleteRemoteActivityTaskParams,
) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	var req EngineRemoteActivityCompleteRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	workerID, err := validateRemoteActivityWorkerID(req.WorkerId)
	if err != nil {
		writeEngineError(w, err, "Failed to validate remote activity complete request")
		return
	}
	output, err := marshalRemoteActivityJSON(req.Output)
	if err != nil {
		writeEngineError(w, err, "Failed to validate remote activity complete request")
		return
	}

	if err := s.engineControl.CompleteRemoteActivityTask(r.Context(), projectID, id, workerID, output); err != nil {
		writeEngineError(w, err, "Failed to complete remote activity task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) FailRemoteActivityTask(
	w http.ResponseWriter,
	r *http.Request,
	id openapi_types.UUID,
	_ FailRemoteActivityTaskParams,
) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	var req EngineRemoteActivityFailRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	failReq, err := remoteActivityFailRequestFromAPI(&req)
	if err != nil {
		writeEngineError(w, err, "Failed to validate remote activity fail request")
		return
	}

	if err := s.engineControl.FailRemoteActivityTask(r.Context(), projectID, id, failReq); err != nil {
		writeEngineError(w, err, "Failed to fail remote activity task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *engineControlService) ClaimRemoteActivityTasks(
	ctx context.Context,
	projectID uuid.UUID,
	req remoteActivityClaimRequest,
) (remoteActivityClaimResult, error) {
	leaseDurationMS := req.LeaseDuration.Milliseconds()
	tasks, err := s.engine.ClaimRemoteActivityTasks(ctx, enginedb.ClaimRemoteActivityTasksParams{
		ProjectFilterID: projectID,
		ClaimedBy:       &req.WorkerID,
		ActivityTypes:   req.ActivityTypes,
		MaxTasks:        req.MaxTasks,
		LeaseDurationMs: &leaseDurationMS,
	})
	if err != nil {
		return remoteActivityClaimResult{}, err
	}
	return remoteActivityClaimResult{Tasks: tasks}, nil
}

func (s *engineControlService) HeartbeatRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	taskID uuid.UUID,
	workerID string,
) (enginedb.EngineActivityTask, error) {
	task, err := s.engine.HeartbeatRemoteActivityTask(ctx, enginedb.HeartbeatRemoteActivityTaskParams{
		ID:        taskID,
		ProjectID: projectID,
		ClaimedBy: &workerID,
	})
	if err == nil {
		return task, nil
	}
	return enginedb.EngineActivityTask{}, classifyRemoteActivityOwnershipMiss(ctx, s.engine, projectID, taskID, err)
}

func (s *engineControlService) CompleteRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	taskID uuid.UUID,
	workerID string,
	output []byte,
) error {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	engineTx := enginedb.New(tx.Tx())
	task, err := engineTx.CompleteRemoteActivityTask(ctx, enginedb.CompleteRemoteActivityTaskParams{
		ID:                taskID,
		ProjectID:         projectID,
		ClaimedBy:         &workerID,
		Output:            output,
		CompletionGraceMs: s.completionGrace.Milliseconds(),
	})
	if err != nil {
		return classifyRemoteActivityOwnershipMiss(ctx, engineTx, projectID, taskID, err)
	}

	if err := wakeAndSyncRemoteActivityRun(ctx, tx, engineTx, task.RunID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *engineControlService) FailRemoteActivityTask(
	ctx context.Context,
	projectID uuid.UUID,
	taskID uuid.UUID,
	req remoteActivityFailRequest,
) error {
	tx, err := s.platform.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	engineTx := enginedb.New(tx.Tx())
	task, err := engineTx.GetActivityTaskByProjectForUpdate(ctx, enginedb.GetActivityTaskByProjectForUpdateParams{
		ID:        taskID,
		ProjectID: projectID,
	})
	if err != nil {
		return classifyRemoteActivityOwnershipMiss(ctx, engineTx, projectID, taskID, err)
	}
	if !remoteActivityTaskOwnedByWorker(&task, req.WorkerID, time.Now(), s.completionGrace) {
		return remoteActivityOwnershipConflict()
	}

	if !req.NonRetryable && task.AttemptCount < task.MaxAttempts {
		retried, retryErr := retryRemoteActivityTask(ctx, engineTx, projectID, &task, req, s.completionGrace)
		if retryErr != nil {
			return retryErr
		}
		payload, retryErr := publichistory.MarshalPayload(publichistory.ActivityRetryScheduledPayload{
			ActivityKey:     retried.ActivityKey,
			ActivityType:    retried.ActivityType,
			FailedAttempt:   retried.AttemptCount,
			NextAvailableAt: retried.AvailableAt,
			ErrorCode:       req.ErrorCode,
			ErrorMessage:    req.ErrorMessage,
		})
		if retryErr != nil {
			return retryErr
		}
		run, retryErr := engineTx.GetRunForUpdate(ctx, retried.RunID)
		if retryErr != nil {
			return retryErr
		}
		appended, retryErr := appendLockedRunHistoryEvent(ctx, engineTx, &run, publichistory.EventActivityRetryScheduled, payload)
		if retryErr != nil {
			return retryErr
		}
		if retryErr := updateProjectedTraceLatestHistory(ctx, tx, retried.RunID, appended.ID); retryErr != nil {
			return retryErr
		}
		return tx.Commit(ctx)
	}

	failed, err := engineTx.FailRemoteActivityTask(ctx, enginedb.FailRemoteActivityTaskParams{
		ID:                task.ID,
		ProjectID:         projectID,
		ClaimedBy:         &req.WorkerID,
		LastErrorCode:     &req.ErrorCode,
		LastErrorMessage:  &req.ErrorMessage,
		CompletionGraceMs: s.completionGrace.Milliseconds(),
	})
	if err != nil {
		return classifyRemoteActivityOwnershipMiss(ctx, engineTx, projectID, taskID, err)
	}
	if err := wakeAndSyncRemoteActivityRun(ctx, tx, engineTx, failed.RunID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func remoteActivityClaimRequestFromAPI(req *EngineRemoteActivityClaimRequest) (remoteActivityClaimRequest, error) {
	if req == nil {
		return remoteActivityClaimRequest{}, invalidRemoteActivityRequest("request body is required")
	}
	workerID, err := validateRemoteActivityWorkerID(req.WorkerId)
	if err != nil {
		return remoteActivityClaimRequest{}, err
	}
	activityTypes, err := validateRemoteActivityTypes(req.ActivityTypes)
	if err != nil {
		return remoteActivityClaimRequest{}, err
	}
	leaseDuration, err := clampRemoteActivityLease(req.LeaseDuration)
	if err != nil {
		return remoteActivityClaimRequest{}, err
	}

	return remoteActivityClaimRequest{
		WorkerID:      workerID,
		ActivityTypes: activityTypes,
		MaxTasks:      clampRemoteActivityMaxTasks(req.MaxTasks),
		LeaseDuration: leaseDuration,
	}, nil
}

func remoteActivityFailRequestFromAPI(req *EngineRemoteActivityFailRequest) (remoteActivityFailRequest, error) {
	if req == nil {
		return remoteActivityFailRequest{}, invalidRemoteActivityRequest("request body is required")
	}
	workerID, err := validateRemoteActivityWorkerID(req.WorkerId)
	if err != nil {
		return remoteActivityFailRequest{}, err
	}
	errorCode, err := validateBoundedTrimmedRemoteActivityField("error_code", req.ErrorCode, remoteActivityErrorCodeMaxLength)
	if err != nil {
		return remoteActivityFailRequest{}, err
	}
	return remoteActivityFailRequest{
		WorkerID:     workerID,
		ErrorCode:    errorCode,
		ErrorMessage: truncateRunes(req.ErrorMessage, remoteActivityErrorMessageMaxLength),
		NonRetryable: req.NonRetryable != nil && *req.NonRetryable,
	}, nil
}

func validateRemoteActivityWorkerID(workerID string) (string, error) {
	return validateBoundedTrimmedRemoteActivityField("worker_id", workerID, remoteActivityWorkerIDMaxLength)
}

func validateRemoteActivityTypes(activityTypes []string) ([]string, error) {
	if len(activityTypes) == 0 {
		return nil, invalidRemoteActivityRequest("activity_types must contain at least one activity type")
	}
	if len(activityTypes) > remoteActivityMaxActivityTypes {
		return nil, invalidRemoteActivityRequest(fmt.Sprintf("activity_types must contain at most %d activity types", remoteActivityMaxActivityTypes))
	}
	normalized := make([]string, 0, len(activityTypes))
	for _, activityType := range activityTypes {
		trimmed, err := validateBoundedTrimmedRemoteActivityField("activity_type", activityType, remoteActivityTypeMaxLength)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
}

func validateBoundedTrimmedRemoteActivityField(name, value string, maxLength int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", invalidRemoteActivityRequest(name + " must be non-empty")
	}
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", invalidRemoteActivityRequest(fmt.Sprintf("%s must be at most %d characters", name, maxLength))
	}
	return trimmed, nil
}

func clampRemoteActivityMaxTasks(maxTasks *int) int32 {
	if maxTasks == nil {
		return remoteActivityDefaultMaxTasks
	}
	if *maxTasks < 1 {
		return 1
	}
	if *maxTasks > remoteActivityMaxTasks {
		return remoteActivityMaxTasks
	}
	return int32(*maxTasks)
}

func clampRemoteActivityLease(raw *string) (time.Duration, error) {
	if raw == nil {
		return remoteActivityDefaultLease, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return 0, invalidRemoteActivityRequest("lease_duration must be a valid duration")
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, invalidRemoteActivityRequest("lease_duration must be a valid duration")
	}
	switch {
	case duration < remoteActivityMinLease:
		return remoteActivityMinLease, nil
	case duration > remoteActivityMaxLease:
		return remoteActivityMaxLease, nil
	default:
		return duration, nil
	}
}

func remoteActivityTaskToAPI(task *enginedb.EngineActivityTask) (EngineRemoteActivityTask, error) {
	if task == nil {
		return EngineRemoteActivityTask{}, errors.New("remote activity task is required")
	}
	input, err := decodeRemoteActivityJSON(task.Input)
	if err != nil {
		return EngineRemoteActivityTask{}, err
	}
	return EngineRemoteActivityTask{
		TaskId:                   task.ID,
		ActivityKey:              task.ActivityKey,
		ActivityType:             task.ActivityType,
		Input:                    input,
		LeaseExpiresAt:           remoteActivityLeaseExpiresAt(task),
		EffectiveLeaseDurationMs: remoteActivityLeaseDurationMS(task),
	}, nil
}

func remoteActivityLeaseExpiresAt(task *enginedb.EngineActivityTask) time.Time {
	if task != nil && task.LeaseExpiresAt.Valid {
		return task.LeaseExpiresAt.Time
	}
	return time.Time{}
}

func remoteActivityLeaseDurationMS(task *enginedb.EngineActivityTask) int64 {
	if task != nil && task.LeaseDurationMs != nil {
		return *task.LeaseDurationMs
	}
	return 0
}

func decodeRemoteActivityJSON(raw []byte) (interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func marshalRemoteActivityJSON(value interface{}) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(value)
}

func truncateRunes(value string, maxLength int) string {
	if utf8.RuneCountInString(value) <= maxLength {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxLength])
}

func remoteActivityTaskOwnedByWorker(
	task *enginedb.EngineActivityTask,
	workerID string,
	now time.Time,
	completionGrace time.Duration,
) bool {
	if task == nil || task.ExecutionTarget != publicworkflow.ActivityExecutionTargetRemote {
		return false
	}
	if task.Status != enginedb.EngineActivityTaskStatusClaimed {
		return false
	}
	if task.ClaimedBy == nil || *task.ClaimedBy != workerID {
		return false
	}
	return task.LeaseExpiresAt.Valid && task.LeaseExpiresAt.Time.Add(completionGrace).After(now)
}

func retryRemoteActivityTask(
	ctx context.Context,
	engineTx *enginedb.Queries,
	projectID uuid.UUID,
	task *enginedb.EngineActivityTask,
	req remoteActivityFailRequest,
	completionGrace time.Duration,
) (enginedb.EngineActivityTask, error) {
	if task == nil {
		return enginedb.EngineActivityTask{}, errors.New("remote activity task is required")
	}
	retryDelayMS, err := publicworkflow.ComputeActivityRetryDelayMS(
		task.AttemptCount,
		task.InitialBackoffMs,
		task.MaxBackoffMs,
		task.BackoffMultiplier,
	)
	if err != nil {
		return enginedb.EngineActivityTask{}, fmt.Errorf("remote activity task %s retry policy: %w", task.ID, err)
	}

	retried, err := engineTx.RetryRemoteActivityTask(ctx, enginedb.RetryRemoteActivityTaskParams{
		ID:                task.ID,
		ProjectID:         projectID,
		ClaimedBy:         &req.WorkerID,
		RetryDelayMs:      retryDelayMS,
		LastErrorCode:     &req.ErrorCode,
		LastErrorMessage:  &req.ErrorMessage,
		CompletionGraceMs: completionGrace.Milliseconds(),
	})
	if err != nil {
		return enginedb.EngineActivityTask{}, classifyRemoteActivityOwnershipMiss(ctx, engineTx, projectID, task.ID, err)
	}
	return retried, nil
}

func wakeAndSyncRemoteActivityRun(
	ctx context.Context,
	tx *store.Tx,
	engineTx *enginedb.Queries,
	runID uuid.UUID,
) error {
	run, err := engineTx.WakeWaitingRun(ctx, runID)
	wakeApplied := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		run, err = engineTx.GetRun(ctx, runID)
	}
	if err != nil {
		return err
	}
	if wakeApplied {
		if err := notifyEngineChannel(ctx, tx.Tx(), publicnotify.ChannelRuns); err != nil {
			return err
		}
	}
	return syncProjectedTraceSummary(ctx, tx, engineTx, &run)
}

func classifyRemoteActivityOwnershipMiss(
	ctx context.Context,
	engineTx *enginedb.Queries,
	projectID uuid.UUID,
	taskID uuid.UUID,
	err error,
) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, lookupErr := engineTx.GetActivityTaskRemoteConflictState(ctx, enginedb.GetActivityTaskRemoteConflictStateParams{
		ID:        taskID,
		ProjectID: projectID,
	}); lookupErr != nil {
		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return &engineAPIError{
				Code:       "not_found",
				Message:    "activity task not found",
				HTTPStatus: http.StatusNotFound,
			}
		}
		return lookupErr
	}
	return remoteActivityOwnershipConflict()
}

func remoteActivityOwnershipConflict() error {
	return &engineAPIError{
		Code:       "activity_task_conflict",
		Message:    "activity task is not currently owned by this remote worker",
		HTTPStatus: http.StatusConflict,
	}
}

func invalidRemoteActivityRequest(message string) error {
	return &engineAPIError{
		Code:       "invalid_request",
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
	}
}
