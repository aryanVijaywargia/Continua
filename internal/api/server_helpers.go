package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	resp := Error{
		Code:    code,
		Message: message,
	}
	writeJSON(w, status, resp)
}

func writeEngineError(w http.ResponseWriter, err error, fallbackMessage string) {
	var apiErr *engineAPIError
	if errors.As(err, &apiErr) {
		writeError(w, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
		return
	}
	var sharedErr *enginecontrol.APIError
	if errors.As(err, &sharedErr) {
		writeError(w, sharedErr.HTTPStatus, sharedErr.Code, sharedErr.Message)
		return
	}

	writeError(w, http.StatusInternalServerError, "internal_error", fallbackMessage)
}

func notifyEngineChannel(ctx context.Context, db enginedb.DBTX, channel string) error {
	_, err := db.Exec(ctx, "SELECT pg_notify($1, '')", channel)
	return err
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, dest any) bool {
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body: "+err.Error())
		return false
	}
	return true
}

func decodeOptionalJSONRequest(w http.ResponseWriter, r *http.Request, dest any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}

	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body: "+err.Error())
		return false
	}
	return true
}

// write413Error writes a 413 error response in the spec-compliant format.
// Per spec: {"error": "batch exceeds 5MB limit"}
func write413Error(w http.ResponseWriter, message string) {
	resp := struct {
		Error string `json:"error"`
	}{
		Error: message,
	}
	writeJSON(w, http.StatusRequestEntityTooLarge, resp)
}

// isMaxBytesError checks if the error is from MaxBytesReader exceeding the limit.
func isMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}
	return strings.Contains(err.Error(), "request body too large")
}

func normalizePagination(limitParam, offsetParam *int) (limit, offset int32) {
	limit = normalizeLimit(limitParam, defaultPageLimit, maxPageLimit)
	offset = 0

	if offsetParam != nil {
		offset = int32(*offsetParam)
	}
	if offset < 0 {
		offset = 0
	}

	return limit, offset
}

func normalizeLimit(limitParam *int, defaultLimit, maxLimit int32) int32 {
	limit := defaultLimit
	if limitParam != nil {
		limit = int32(*limitParam)
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

type requestScopePolicy int

const (
	scopePolicyRequireProject requestScopePolicy = iota
	scopePolicyAllowUnbounded
)

func projectIDOrUnauthorized(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return boundProjectIDFromRequest(w, r)
}

func boundProjectIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	scope, ok := scopeFromRequest(w, r, scopePolicyRequireProject)
	if !ok {
		return uuid.Nil, false
	}
	projectID, bound := scope.ProjectID()
	if !bound {
		writeError(w, http.StatusInternalServerError, "internal_error", "Expected a bound project scope")
		return uuid.Nil, false
	}
	return projectID, true
}

func scopeFromRequest(w http.ResponseWriter, r *http.Request, policy requestScopePolicy) (store.Scope, bool) {
	if projectID, ok := middleware.GetProjectID(r.Context()); ok {
		return store.BoundScope(projectID), true
	}

	authMode, ok := middleware.GetAuthMode(r.Context())
	if !ok || authMode != middleware.AuthModeOperator {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return store.Scope{}, false
	}

	projectID, selected, ok := projectIDSelectionFromRequest(w, r, policy == scopePolicyRequireProject)
	if !ok {
		return store.Scope{}, false
	}
	if selected {
		return store.BoundScope(projectID), true
	}
	if policy == scopePolicyAllowUnbounded {
		return store.UnboundedScope(), true
	}

	return store.Scope{}, false
}

func projectIDSelectionFromRequest(
	w http.ResponseWriter,
	r *http.Request,
	required bool,
) (uuid.UUID, bool, bool) {
	values, present := r.URL.Query()["project_id"]
	if !present {
		if required {
			writeError(
				w,
				http.StatusBadRequest,
				"missing_project_id",
				"project_id is required for operator requests",
			)
			return uuid.Nil, false, false
		}
		return uuid.Nil, false, true
	}

	rawProjectID := ""
	if len(values) > 0 {
		rawProjectID = strings.TrimSpace(values[0])
	}
	if rawProjectID == "" {
		writeError(
			w,
			http.StatusBadRequest,
			"missing_project_id",
			"project_id is required for operator requests",
		)
		return uuid.Nil, false, false
	}

	projectID, err := uuid.Parse(rawProjectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_project_id", "project_id must be a valid UUID")
		return uuid.Nil, false, false
	}

	return projectID, true, true
}

func traceSortDirectionFromParams(params *ListTracesParams) store.SortDirection {
	if params.SortBy != nil && *params.SortBy != StartedAt {
		return store.SortDirectionDesc
	}
	if params.SortDir != nil && *params.SortDir == ListTracesParamsSortDirAsc {
		return store.SortDirectionAsc
	}
	return store.SortDirectionDesc
}

func traceFilterFromParams(scope store.Scope, params *ListTracesParams, limit, offset int32) store.TraceFilter {
	filter := store.TraceFilter{
		Scope:   scope,
		SortDir: traceSortDirectionFromParams(params),
		Limit:   limit,
		Offset:  offset,
	}

	if params.Q != nil {
		filter.Query = *params.Q
	}
	if params.Status != nil {
		filter.Status = string(*params.Status)
	}
	if params.StartTimeFrom != nil {
		filter.StartTimeFrom = params.StartTimeFrom
	}
	if params.StartTimeTo != nil {
		filter.StartTimeTo = params.StartTimeTo
	}
	if params.UserId != nil {
		filter.UserID = *params.UserId
	}
	if params.EngineInstanceKey != nil {
		filter.EngineInstanceKey = *params.EngineInstanceKey
	}
	if params.EngineRunId != nil {
		id := *params.EngineRunId
		filter.EngineRunID = &id
	}
	if params.EngineDefinitionName != nil {
		filter.EngineDefinitionName = *params.EngineDefinitionName
	}
	if params.EngineDefinitionVersion != nil {
		filter.EngineDefinitionVersion = *params.EngineDefinitionVersion
	}
	if params.EngineRunStatus != nil {
		filter.EngineRunStatus = string(*params.EngineRunStatus)
	}
	if params.EngineParentRunId != nil {
		id := *params.EngineParentRunId
		filter.EngineParentRunID = &id
	}
	if params.EngineRootRunId != nil {
		id := *params.EngineRootRunId
		filter.EngineRootRunID = &id
	}
	if params.EngineChildKey != nil {
		filter.EngineChildKey = *params.EngineChildKey
	}
	if params.EngineChildDepth != nil {
		depth := int32(*params.EngineChildDepth)
		filter.EngineChildDepth = &depth
	}
	if params.EngineProjectionState != nil {
		filter.EngineProjectionState = string(*params.EngineProjectionState)
	}
	if params.EngineOnly != nil {
		filter.EngineOnly = *params.EngineOnly
	}
	if params.SessionId != nil {
		id := *params.SessionId
		filter.SessionID = &id
	}
	if params.HasErrors != nil {
		filter.HasErrors = params.HasErrors
	}
	if params.MinDurationMs != nil {
		filter.MinDurationMs = params.MinDurationMs
	}

	return filter
}

func traceHasSearchQuery(filter *store.TraceFilter) bool {
	return strings.TrimSpace(filter.Query) != ""
}

func traceNeedsDynamicQuery(filter *store.TraceFilter) bool {
	return traceHasSearchQuery(filter) ||
		filter.Status != "" ||
		filter.StartTimeFrom != nil ||
		filter.StartTimeTo != nil ||
		filter.UserID != "" ||
		filter.EngineInstanceKey != "" ||
		filter.EngineRunID != nil ||
		filter.EngineDefinitionName != "" ||
		filter.EngineDefinitionVersion != "" ||
		filter.EngineRunStatus != "" ||
		filter.EngineParentRunID != nil ||
		filter.EngineRootRunID != nil ||
		filter.EngineChildKey != "" ||
		filter.EngineChildDepth != nil ||
		filter.EngineProjectionState != "" ||
		filter.EngineOnly ||
		filter.HasErrors != nil ||
		filter.MinDurationMs != nil
}

func sessionSortFromParams(params *ListSessionsParams) (store.SessionSortBy, store.SortDirection) {
	if params.SortBy != nil {
		switch *params.SortBy {
		case CreatedAt:
			if params.SortDir != nil && *params.SortDir == ListSessionsParamsSortDirAsc {
				return store.SessionSortByCreatedAt, store.SortDirectionAsc
			}
			return store.SessionSortByCreatedAt, store.SortDirectionDesc
		case TraceCount:
			if params.SortDir != nil && *params.SortDir == ListSessionsParamsSortDirAsc {
				return store.SessionSortByTraceCount, store.SortDirectionAsc
			}
			return store.SessionSortByTraceCount, store.SortDirectionDesc
		default:
			return store.SessionSortByCreatedAt, store.SortDirectionDesc
		}
	}

	if params.SortDir != nil && *params.SortDir == ListSessionsParamsSortDirAsc {
		return store.SessionSortByCreatedAt, store.SortDirectionAsc
	}
	return store.SessionSortByCreatedAt, store.SortDirectionDesc
}

func sessionFilterFromParams(scope store.Scope, params *ListSessionsParams, limit, offset int32) store.SessionFilter {
	sortBy, sortDir := sessionSortFromParams(params)
	filter := store.SessionFilter{
		Scope:   scope,
		SortBy:  sortBy,
		SortDir: sortDir,
		Limit:   limit,
		Offset:  offset,
	}

	if params.Q != nil {
		filter.Query = *params.Q
	}
	if params.UserId != nil {
		filter.UserID = *params.UserId
	}

	return filter
}

func sessionNeedsDynamicQuery(filter *store.SessionFilter) bool {
	return strings.TrimSpace(filter.Query) != "" ||
		filter.UserID != "" ||
		filter.SortBy != store.SessionSortByCreatedAt ||
		filter.SortDir != store.SortDirectionDesc
}

func apiIngestResponse(result *ingest.IngestResponse, includeCounts bool) IngestResponse {
	resp := IngestResponse{
		Status:   IngestResponseStatus(result.Status),
		BatchKey: result.BatchKey,
	}

	if result.BatchID != uuid.Nil {
		batchID := result.BatchID
		resp.BatchId = &batchID
	}

	if includeCounts {
		resp.TraceCount = &result.TraceCount
		resp.SpanCount = &result.SpanCount
		resp.EventCount = &result.EventCount
		resp.AcceptedCount = &result.AcceptedCount
		resp.RejectedCount = &result.RejectedCount
		if len(result.Errors) > 0 {
			resp.Errors = &result.Errors
		}
	}

	return resp
}

func apiBatchStatus(status *ingest.BatchStatus) BatchStatusResponse {
	resp := BatchStatusResponse{
		BatchId:          status.BatchID,
		BatchKey:         status.BatchKey,
		Status:           BatchStatusResponseStatus(status.Status),
		AttemptCount:     status.AttemptCount,
		ServerReceivedAt: status.ServerReceivedAt,
		TraceCount:       status.TraceCount,
		SpanCount:        status.SpanCount,
		EventCount:       status.EventCount,
		AcceptedCount:    status.AcceptedCount,
		RejectedCount:    status.RejectedCount,
		LastErrorCode:    status.LastErrorCode,
		LastErrorMessage: status.LastErrorMessage,
	}
	if status.ProcessingStartedAt != nil {
		resp.ProcessingStartedAt = status.ProcessingStartedAt
	}
	if status.ProcessingCompletedAt != nil {
		resp.ProcessingCompletedAt = status.ProcessingCompletedAt
	}
	return resp
}

func (s *Server) getScopedTrace(
	ctx context.Context,
	w http.ResponseWriter,
	scope store.Scope,
	traceID uuid.UUID,
) (store.TraceRead, bool) {
	trace, err := s.store.GetTrace(ctx, scope, traceID)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Trace not found")
		return store.TraceRead{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get trace")
		return store.TraceRead{}, false
	}

	return trace, true
}
