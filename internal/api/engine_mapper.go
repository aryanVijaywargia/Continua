package api

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	"github.com/continua-ai/continua/internal/store"
)

func engineInstanceResponseToAPI(result *engineInstanceResult) EngineInstanceResponse {
	return EngineInstanceResponse{
		CurrentRun:     engineRunSummaryToAPI(&result.CurrentRun),
		DefinitionName: result.Instance.DefinitionName,
		InstanceId:     openapi_types.UUID(result.Instance.ID),
		InstanceKey:    result.Instance.InstanceKey,
		Status:         string(result.Instance.Status),
	}
}

func engineRunResponseToAPI(summary *engineRunSummary) EngineRunResponse {
	return EngineRunResponse{
		CompletedAt:       summary.CompletedAt,
		CreatedAt:         summary.CreatedAt,
		CustomStatus:      parseOptionalJSONObjectRaw(summary.CustomStatus),
		DefinitionName:    summary.DefinitionName,
		DefinitionVersion: summary.DefinitionVersion,
		Failure:           engineFailureSummaryToAPI(summary.Status, summary.LastErrorCode, summary.LastErrorMessage),
		InstanceId:        openapi_types.UUID(summary.InstanceID),
		InstanceKey:       summary.InstanceKey,
		PendingWork:       enginePendingWorkToAPI(summary),
		ProjectionState:   engineProjectionStateFromString(summary.ProjectionState),
		Result:            parseOptionalJSONValueRaw(summary.Result),
		RunId:             openapi_types.UUID(summary.RunID),
		Status:            engineRunStatusToAPI(summary.Status),
		UpdatedAt:         summary.UpdatedAt,
		WaitState:         parseOptionalWaitState(summary.WaitState),
	}
}

func engineRunResultResponseToAPI(summary *engineRunSummary) EngineRunResultResponse {
	return EngineRunResultResponse{
		Failure: engineFailureSummaryToAPI(summary.Status, summary.LastErrorCode, summary.LastErrorMessage),
		Result:  parseOptionalJSONValueRaw(summary.Result),
		RunId:   openapi_types.UUID(summary.RunID),
		Status:  engineRunStatusToAPI(summary.Status),
	}
}

func engineRunSummaryToAPI(summary *engineRunSummary) EngineRunSummary {
	return EngineRunSummary{
		CompletedAt:       summary.CompletedAt,
		CreatedAt:         summary.CreatedAt,
		CustomStatus:      parseOptionalJSONObjectRaw(summary.CustomStatus),
		DefinitionName:    summary.DefinitionName,
		DefinitionVersion: summary.DefinitionVersion,
		Failure:           engineFailureSummaryToAPI(summary.Status, summary.LastErrorCode, summary.LastErrorMessage),
		InstanceKey:       summary.InstanceKey,
		PendingWork:       enginePendingWorkToAPI(summary),
		ProjectionState:   engineProjectionStateFromString(summary.ProjectionState),
		Result:            parseOptionalJSONValueRaw(summary.Result),
		RunId:             openapi_types.UUID(summary.RunID),
		Status:            engineRunStatusToAPI(summary.Status),
		UpdatedAt:         summary.UpdatedAt,
		WaitState:         parseOptionalWaitState(summary.WaitState),
	}
}

func engineHistoryPageToAPI(page *engineHistoryPage) EngineRunHistoryResponse {
	events := make([]EngineHistoryEvent, len(page.Events))
	for i := range page.Events {
		events[i] = engineHistoryEventToAPI(&page.Events[i])
	}

	return EngineRunHistoryResponse{
		Events:    events,
		HasMore:   page.HasMore,
		NextAfter: page.NextAfter,
	}
}

func engineHistoryEventToAPI(event *enginedb.EngineHistory) EngineHistoryEvent {
	return EngineHistoryEvent{
		CreatedAt:  event.CreatedAt,
		EventType:  event.EventType,
		Id:         event.ID,
		Payload:    parseOptionalJSONObjectRaw(event.Payload),
		SequenceNo: event.SequenceNo,
	}
}

func engineControlResponseToAPI(result *engineControlResult) EngineControlResponse {
	return EngineControlResponse{
		Accepted:    result.Accepted,
		InstanceKey: result.InstanceKey,
		RunId:       openapi_types.UUID(result.RunID),
		WakeApplied: result.WakeApplied,
	}
}

func projectedEngineRunSummaryFromTrace(trace *store.TraceRead) *EngineRunSummary {
	info := engineTraceInfoFromTrace(trace)
	if info == nil {
		return nil
	}

	summary := &EngineRunSummary{
		CreatedAt:         traceStartedAt(trace),
		CustomStatus:      parseOptionalJSONObjectRaw(cloneTraceJSON(trace.EngineCustomStatus)),
		DefinitionName:    info.DefinitionName,
		DefinitionVersion: info.DefinitionVersion,
		InstanceKey:       projectedEngineInstanceKey(trace),
		PendingWork: EnginePendingWork{
			PendingActivityTasks: derefInt64(trace.EnginePendingActivityTasks),
			PendingInboxItems:    derefInt64(trace.EnginePendingInboxItems),
		},
		ProjectionState: info.ProjectionState,
		RunId:           info.RunId,
		Status:          projectedEngineRunStatusFromTrace(trace),
		UpdatedAt:       trace.UpdatedAt,
		WaitState:       parseOptionalWaitState(cloneTraceJSON(trace.EngineWaitState)),
	}

	if trace.EndTime.Valid {
		summary.CompletedAt = &trace.EndTime.Time
	}

	switch summary.Status {
	case EngineRunStatusCOMPLETED:
		summary.Result = parseOptionalJSONValueRaw(trace.Output)
	case EngineRunStatusFAILED, EngineRunStatusCANCELLED:
		summary.Failure = projectedEngineFailureSummary(trace.Status, trace.Output)
	}

	return summary
}

func projectedEngineInstanceKey(trace *store.TraceRead) string {
	return firstNonEmpty(derefString(trace.EngineInstanceKey), deref(trace.SessionExternalID))
}

func cloneTraceJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func shouldReadLiveEngineSummary(trace *store.TraceRead) bool {
	if trace == nil || !trace.EngineRunID.Valid {
		return false
	}

	switch normalizedProjectionState(trace.EngineProjectionState) {
	case publicprojection.StateCatchingUp.String(), publicprojection.StateSummaryOnly.String():
		return true
	default:
		return false
	}
}

func normalizedProjectionState(value *string) string {
	state := strings.TrimSpace(derefString(value))
	if state == "" {
		return publicprojection.StateUpToDate.String()
	}
	return state
}

func projectedEngineRunStatusFromTrace(trace *store.TraceRead) EngineRunStatus {
	switch strings.ToLower(strings.TrimSpace(derefString(trace.EngineRunStatus))) {
	case string(enginedb.EngineRunLifecycleStatusQueued):
		return EngineRunStatusQUEUED
	case string(enginedb.EngineRunLifecycleStatusWaiting):
		return EngineRunStatusWAITING
	case string(enginedb.EngineRunLifecycleStatusCompleted):
		return EngineRunStatusCOMPLETED
	case string(enginedb.EngineRunLifecycleStatusFailed):
		return EngineRunStatusFAILED
	case string(enginedb.EngineRunLifecycleStatusCancelled):
		return EngineRunStatusCANCELLED
	case string(enginedb.EngineRunLifecycleStatusRunning):
		return EngineRunStatusRUNNING
	default:
		return projectedEngineRunStatus(trace.Status)
	}
}

func engineTraceInfoFromTrace(trace *store.TraceRead) *EngineTraceInfo {
	return engineTraceInfoFromParts(
		pgUUIDPtr(trace.EngineRunID),
		trace.EngineDefinitionName,
		trace.EngineDefinitionVersion,
		trace.EngineProjectionState,
	)
}

func engineTraceInfoFromCompareHeader(header *store.SessionCompareTraceHeader) *EngineTraceInfo {
	return engineTraceInfoFromParts(
		header.EngineRunID,
		header.EngineDefinitionName,
		header.EngineDefinitionVersion,
		header.EngineProjectionState,
	)
}

func engineTraceInfoFromParts(
	runID *uuid.UUID,
	definitionName *string,
	definitionVersion *string,
	projectionState *string,
) *EngineTraceInfo {
	if runID == nil || definitionName == nil || definitionVersion == nil || projectionState == nil {
		return nil
	}
	if strings.TrimSpace(*definitionName) == "" || strings.TrimSpace(*definitionVersion) == "" || strings.TrimSpace(*projectionState) == "" {
		return nil
	}

	return &EngineTraceInfo{
		DefinitionName:    *definitionName,
		DefinitionVersion: *definitionVersion,
		ProjectionState:   engineProjectionStateFromString(*projectionState),
		RunId:             openapi_types.UUID(*runID),
	}
}

func engineTimelineMetadataFromTrace(trace *store.TraceRead) *struct {
	ProjectionState EngineProjectionState `json:"projection_state"`
} {
	info := engineTraceInfoFromTrace(trace)
	if info == nil {
		return nil
	}

	return &struct {
		ProjectionState EngineProjectionState `json:"projection_state"`
	}{
		ProjectionState: info.ProjectionState,
	}
}

func enginePendingWorkToAPI(summary *engineRunSummary) EnginePendingWork {
	return EnginePendingWork{
		PendingActivityTasks: summary.PendingActivityTasks,
		PendingInboxItems:    summary.PendingInboxItems,
	}
}

func engineFailureSummaryToAPI(
	status enginedb.EngineRunLifecycleStatus,
	errorCode *string,
	errorMessage *string,
) *EngineFailureSummary {
	if strings.TrimSpace(derefString(errorCode)) == "" && strings.TrimSpace(derefString(errorMessage)) == "" &&
		status != enginedb.EngineRunLifecycleStatusFailed && status != enginedb.EngineRunLifecycleStatusCancelled {
		return nil
	}

	return &EngineFailureSummary{
		ErrorCode:    derefString(errorCode),
		ErrorMessage: derefString(errorMessage),
		Status:       strings.ToUpper(string(status)),
	}
}

func projectedEngineFailureSummary(status string, output []byte) *EngineFailureSummary {
	var payload struct {
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return &EngineFailureSummary{Status: strings.ToUpper(normalizeProjectedEngineStatus(status))}
	}

	return &EngineFailureSummary{
		ErrorCode:    payload.ErrorCode,
		ErrorMessage: payload.ErrorMessage,
		Status:       firstNonEmpty(strings.ToUpper(payload.Status), strings.ToUpper(normalizeProjectedEngineStatus(status))),
	}
}

func parseOptionalWaitState(raw json.RawMessage) *EngineWaitState {
	if len(raw) == 0 {
		return nil
	}

	var state EngineWaitState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil
	}
	return &state
}

func parseOptionalJSONObjectRaw(raw json.RawMessage) *map[string]interface{} {
	if payload := parseJSONObject(raw); payload != nil {
		return &payload
	}
	return nil
}

func parseOptionalJSONValueRaw(raw json.RawMessage) interface{} {
	value, ok := parseJSONValue(raw)
	if !ok {
		return nil
	}
	return value
}

func engineProjectionStateFromString(value string) EngineProjectionState {
	switch strings.TrimSpace(value) {
	case publicprojection.StateCatchingUp.String():
		return CatchingUp
	case publicprojection.StateSummaryOnly.String():
		return SummaryOnly
	case publicprojection.StateJournalExpired.String():
		return JournalExpired
	default:
		return UpToDate
	}
}

func engineRunStatusToAPI(status enginedb.EngineRunLifecycleStatus) EngineRunStatus {
	switch status {
	case enginedb.EngineRunLifecycleStatusQueued:
		return EngineRunStatusQUEUED
	case enginedb.EngineRunLifecycleStatusCompleted:
		return EngineRunStatusCOMPLETED
	case enginedb.EngineRunLifecycleStatusFailed:
		return EngineRunStatusFAILED
	case enginedb.EngineRunLifecycleStatusCancelled:
		return EngineRunStatusCANCELLED
	case enginedb.EngineRunLifecycleStatusWaiting:
		return EngineRunStatusWAITING
	default:
		return EngineRunStatusRUNNING
	}
}

func projectedEngineRunStatus(status string) EngineRunStatus {
	switch normalizeProjectedEngineStatus(status) {
	case "completed":
		return EngineRunStatusCOMPLETED
	case "failed":
		return EngineRunStatusFAILED
	case "cancelled":
		return EngineRunStatusCANCELLED
	default:
		return EngineRunStatusRUNNING
	}
}

func normalizeProjectedEngineStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "ok":
		return "completed"
	case "failed", "error":
		return "failed"
	case "cancelled":
		return "cancelled"
	default:
		return "running"
	}
}

func traceStartedAt(trace *store.TraceRead) time.Time {
	if trace.StartTime.Valid {
		return trace.StartTime.Time
	}
	return trace.ServerReceivedAt
}

func pgUUIDPtr(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}

	id := uuid.UUID(value.Bytes)
	return &id
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
