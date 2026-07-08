package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/store"
)

const (
	defaultEngineProjectionBackfillLimit = 50
	maxEngineProjectionBackfillLimit     = 100
)

func (s *Server) engineRunProjectIDForControl(
	w http.ResponseWriter,
	r *http.Request,
	scope store.Scope,
	runID openapi_types.UUID,
) (openapi_types.UUID, bool) {
	if projectID, bound := scope.ProjectID(); bound {
		return projectID, true
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return uuid.Nil, false
	}

	run, err := s.engineControl.getRunForScope(r.Context(), scope, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to resolve engine run")
		return uuid.Nil, false
	}
	return run.ProjectID, true
}

func (s *Server) GetEngineInstance(w http.ResponseWriter, r *http.Request, instanceKey string) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetInstance(r.Context(), scope, instanceKey)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine instance")
		return
	}

	writeJSON(w, http.StatusOK, engineInstanceResponseToAPI(&result))
}

func (s *Server) StartEngineRun(w http.ResponseWriter, r *http.Request, _ StartEngineRunParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	var req EngineStartRunRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	startReq, err := engineStartRunRequestFromAPI(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	result, err := s.engineControl.StartRun(r.Context(), projectID, &startReq)
	if err != nil {
		writeEngineError(w, err, "Failed to start engine run")
		return
	}

	writeJSON(w, http.StatusOK, EngineStartRunResponse{
		InstanceKey: result.InstanceKey,
		RunId:       result.RunID,
		TraceId:     result.TraceID,
	})
}

func (s *Server) GetEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetRun(r.Context(), scope, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResponseToAPI(&result))
}

func (s *Server) CancelEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ CancelEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	result, err := s.engineControl.CancelRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to cancel engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineControlResponseToAPI(&result))
}

func (s *Server) TerminateEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ TerminateEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	result, err := s.engineControl.TerminateRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to terminate engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResultResponseToAPI(&result))
}

func (s *Server) SuspendEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ SuspendEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	result, err := s.engineControl.SuspendRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to suspend engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResponseToAPI(&result))
}

func (s *Server) ResumeEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ ResumeEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	result, err := s.engineControl.ResumeRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to resume engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResponseToAPI(&result))
}

func (s *Server) GetEngineRunHistory(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, params GetEngineRunHistoryParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	after := 0
	if params.After != nil {
		after = *params.After
	}
	limit := 100
	if params.Limit != nil {
		limit = *params.Limit
	}

	page, err := s.engineControl.GetRunHistory(r.Context(), scope, runID, after, limit)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine run history")
		return
	}

	writeJSON(w, http.StatusOK, engineHistoryPageToAPI(&page))
}

func (s *Server) GetEngineRunResult(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetRunResult(r.Context(), scope, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine run result")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResultResponseToAPI(&result))
}

func (s *Server) PurgeEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ PurgeEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineSharedControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	var req EnginePurgeRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	result, err := s.engineSharedControl.PurgeRun(r.Context(), projectID, runID, enginecontrol.PurgeMode(req.Mode))
	if err != nil {
		writeEngineError(w, err, "Failed to purge engine run")
		return
	}

	writeJSON(w, http.StatusOK, enginePurgeResponseToAPI(&result))
}

func (s *Server) RepairEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ RepairEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineSharedControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	result, err := s.engineSharedControl.RepairRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to repair engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineRepairResponseToAPI(&result))
}

func (s *Server) BackfillEngineProjections(w http.ResponseWriter, r *http.Request, _ BackfillEngineProjectionsParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineSharedControl == nil {
		http.NotFound(w, r)
		return
	}

	var req EngineProjectionBackfillRequest
	if !decodeOptionalJSONRequest(w, r, &req) {
		return
	}

	backfillReq, err := engineProjectionBackfillRequestFromAPI(&req)
	if err != nil {
		writeEngineError(w, err, "Invalid engine projection backfill request")
		return
	}
	if mode, ok := middleware.GetAuthMode(r.Context()); ok && mode == middleware.AuthModePublicDemo && !backfillReq.DryRun {
		writeError(w, http.StatusForbidden, "public_demo_read_only", "Public demo projection repair only allows dry runs")
		return
	}

	result, err := s.engineSharedControl.BackfillProjections(r.Context(), projectID, &backfillReq)
	if err != nil {
		writeEngineError(w, err, "Failed to backfill engine projections")
		return
	}

	writeJSON(w, http.StatusOK, engineProjectionBackfillResponseToAPI(&result))
}

func (s *Server) GetEngineRunPendingWork(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetRunPendingWork(r.Context(), scope, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine pending work")
		return
	}

	writeJSON(w, http.StatusOK, enginePendingWorkResponseToAPI(&result))
}

func (s *Server) SignalEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ SignalEngineRunParams) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}
	projectID, ok := s.engineRunProjectIDForControl(w, r, scope, runID)
	if !ok {
		return
	}

	var req EngineSignalRunRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	payload, err := marshalOptionalJSONValue(req.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "payload must be valid JSON")
		return
	}

	result, err := s.engineControl.SignalRun(r.Context(), projectID, runID, engineSignalRequest{
		SignalName: req.SignalName,
		Payload:    payload,
	})
	if err != nil {
		writeEngineError(w, err, "Failed to signal engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineControlResponseToAPI(&result))
}

func engineProjectionBackfillRequestFromAPI(
	req *EngineProjectionBackfillRequest,
) (enginecontrol.ProjectionBackfillRequest, error) {
	result := enginecontrol.ProjectionBackfillRequest{
		Limit: defaultEngineProjectionBackfillLimit,
	}
	if req == nil {
		return result, nil
	}

	if req.DryRun != nil {
		result.DryRun = *req.DryRun
	}
	if req.Limit != nil {
		if *req.Limit > maxEngineProjectionBackfillLimit {
			return enginecontrol.ProjectionBackfillRequest{}, errInvalidEngineProjectionBackfillLimit(*req.Limit)
		}
		if *req.Limit > 0 {
			result.Limit = *req.Limit
		}
	}
	if req.OlderThan != nil {
		result.OlderThan = req.OlderThan
	}
	if req.EngineInstanceKey != nil {
		result.EngineInstanceKey = strings.TrimSpace(*req.EngineInstanceKey)
	}
	if req.EngineDefinitionName != nil {
		result.EngineDefinitionName = strings.TrimSpace(*req.EngineDefinitionName)
	}
	if req.EngineRunStatus != nil {
		normalizedStatus, err := normalizeEngineRunStatusValue(string(*req.EngineRunStatus))
		if err != nil {
			return enginecontrol.ProjectionBackfillRequest{}, err
		}
		result.EngineRunStatus = normalizedStatus
	}
	if req.EngineProjectionState != nil {
		normalizedState, err := normalizeEngineProjectionStateValue(string(*req.EngineProjectionState))
		if err != nil {
			return enginecontrol.ProjectionBackfillRequest{}, err
		}
		result.EngineProjectionState = normalizedState
	}

	return result, nil
}

func errInvalidEngineProjectionBackfillLimit(_ int) error {
	return &enginecontrol.APIError{
		Code:       "invalid_request",
		Message:    "limit must be 100 or less",
		HTTPStatus: http.StatusBadRequest,
	}
}

func normalizeEngineRunStatusValue(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch EngineRunStatus(normalized) {
	case "":
		return "", nil
	case EngineRunStatusQUEUED,
		EngineRunStatusRUNNING,
		EngineRunStatusWAITING,
		EngineRunStatusSUSPENDED,
		EngineRunStatusQUARANTINED,
		EngineRunStatusCOMPLETED,
		EngineRunStatusFAILED,
		EngineRunStatusCANCELLED,
		EngineRunStatusTERMINATED,
		EngineRunStatusCONTINUEDASNEW:
		return normalized, nil
	default:
		return "", &enginecontrol.APIError{
			Code:       "invalid_request",
			Message:    "engine_run_status must be a valid EngineRunStatus value",
			HTTPStatus: http.StatusBadRequest,
		}
	}
}

func normalizeEngineProjectionStateValue(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch EngineProjectionState(normalized) {
	case "":
		return "", nil
	case UpToDate, CatchingUp, SummaryOnly, JournalExpired:
		return normalized, nil
	default:
		return "", &enginecontrol.APIError{
			Code:       "invalid_request",
			Message:    "engine_projection_state must be a valid EngineProjectionState value",
			HTTPStatus: http.StatusBadRequest,
		}
	}
}

func engineStartRunRequestFromAPI(req *EngineStartRunRequest) (engineStartRunRequest, error) {
	input, err := marshalOptionalJSONValue(req.Input)
	if err != nil {
		return engineStartRunRequest{}, err
	}

	result := engineStartRunRequest{
		InstanceKey:       req.InstanceKey,
		DefinitionName:    req.DefinitionName,
		DefinitionVersion: req.DefinitionVersion,
		RequestKey:        req.RequestKey,
		Input:             input,
	}

	if req.Session != nil {
		var sessionMetadata map[string]interface{}
		if req.Session.Metadata != nil {
			sessionMetadata = cloneJSONObject(*req.Session.Metadata)
		}
		result.Session = &engineStartSession{
			Key:      deref(req.Session.Key),
			Name:     deref(req.Session.Name),
			Metadata: sessionMetadata,
		}
	}

	if req.Trace != nil {
		var traceMetadata map[string]interface{}
		if req.Trace.Metadata != nil {
			traceMetadata = cloneJSONObject(*req.Trace.Metadata)
		}
		result.Trace = &engineStartTrace{
			Name:        deref(req.Trace.Name),
			UserID:      deref(req.Trace.UserId),
			Tags:        derefSlice(req.Trace.Tags),
			Environment: deref(req.Trace.Environment),
			Release:     deref(req.Trace.Release),
			Metadata:    traceMetadata,
		}
	}

	return result, nil
}

func marshalOptionalJSONValue(value interface{}) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func cloneJSONObject(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(value))
	for key, value := range value {
		cloned[key] = value
	}
	return cloned
}
