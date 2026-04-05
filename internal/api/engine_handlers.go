package api

import (
	"encoding/json"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) GetEngineInstance(w http.ResponseWriter, r *http.Request, instanceKey string) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetInstance(r.Context(), projectID, instanceKey)
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
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResponseToAPI(&result))
}

func (s *Server) CancelEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ CancelEngineRunParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.CancelRun(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to cancel engine run")
		return
	}

	writeJSON(w, http.StatusOK, engineControlResponseToAPI(&result))
}

func (s *Server) GetEngineRunHistory(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, params GetEngineRunHistoryParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
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

	page, err := s.engineControl.GetRunHistory(r.Context(), projectID, runID, after, limit)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine run history")
		return
	}

	writeJSON(w, http.StatusOK, engineHistoryPageToAPI(&page))
}

func (s *Server) GetEngineRunResult(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
		return
	}

	result, err := s.engineControl.GetRunResult(r.Context(), projectID, runID)
	if err != nil {
		writeEngineError(w, err, "Failed to get engine run result")
		return
	}

	writeJSON(w, http.StatusOK, engineRunResultResponseToAPI(&result))
}

func (s *Server) SignalEngineRun(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID, _ SignalEngineRunParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}
	if s.engineControl == nil {
		http.NotFound(w, r)
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
