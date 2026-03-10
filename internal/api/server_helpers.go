package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
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

func projectIDOrUnauthorized(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return uuid.Nil, false
	}
	return projectID, true
}

func traceFilterFromParams(projectID uuid.UUID, params *ListTracesParams, limit, offset int32) (store.TraceFilter, bool) {
	filter := store.TraceFilter{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	}
	hasFilters := false

	if params.Q != nil {
		filter.Query = *params.Q
		hasFilters = true
	}
	if params.Status != nil {
		filter.Status = string(*params.Status)
		hasFilters = true
	}
	if params.StartTimeFrom != nil {
		filter.StartTimeFrom = params.StartTimeFrom
		hasFilters = true
	}
	if params.StartTimeTo != nil {
		filter.StartTimeTo = params.StartTimeTo
		hasFilters = true
	}
	if params.UserId != nil {
		filter.UserID = *params.UserId
		hasFilters = true
	}
	if params.SessionId != nil {
		id := *params.SessionId
		filter.SessionID = &id
		hasFilters = true
	}
	if params.HasErrors != nil {
		filter.HasErrors = params.HasErrors
		hasFilters = true
	}
	if params.MinDurationMs != nil {
		filter.MinDurationMs = params.MinDurationMs
		hasFilters = true
	}

	return filter, hasFilters
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

func (s *Server) getScopedTrace(ctx context.Context, w http.ResponseWriter, projectID, traceID openapi_types.UUID) (platform.Trace, bool) {
	trace, err := s.store.GetTrace(ctx, traceID)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Trace not found")
		return platform.Trace{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get trace")
		return platform.Trace{}, false
	}
	if trace.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "not_found", "Trace not found")
		return platform.Trace{}, false
	}

	return trace, true
}
