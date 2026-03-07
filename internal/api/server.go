package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

// MaxBodySize is the maximum request body size (5MB).
const MaxBodySize = 5 * 1024 * 1024

const (
	defaultPageLimit = int32(50)
	maxPageLimit     = int32(200)
)

// HealthResponse is the response for the health check endpoint.
// Note: This is defined locally because /api/health is routed directly
// in Chi (not via OpenAPI) to avoid auth middleware complexity.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Server implements the ServerInterface for the Continua API.
type Server struct {
	store         *store.Store
	ingestService *ingest.Service
}

// NewServer creates a new API server with the given dependencies.
func NewServer(s *store.Store, ingestService *ingest.Service) *Server {
	return &Server{
		store:         s,
		ingestService: ingestService,
	}
}

// HealthCheck implements the health check endpoint.
// Note: This is NOT part of ServerInterface - it's routed directly in Chi.
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Version: "0.1.0",
	}
	writeJSON(w, http.StatusOK, resp)
}

// Ingest implements the batch ingestion endpoint.
// Per spec:
// - sync=true: process immediately, return 200 with result
// - sync=false or not provided: return 202 "accepted" (async mode)
func (s *Server) Ingest(w http.ResponseWriter, r *http.Request, params IngestParams) {
	// Check content length upfront for 413
	if r.ContentLength > MaxBodySize {
		write413Error(w, "batch exceeds 5MB limit")
		return
	}

	// Limit reader to prevent DoS - this will cause an error if body exceeds limit
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

	// Parse request body
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Check if this is a MaxBytesReader error (body too large)
		if isMaxBytesError(err) {
			write413Error(w, "batch exceeds 5MB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body: "+err.Error())
		return
	}

	// Get project ID from auth context
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	// Convert API types to service types
	svcReq := convertToServiceRequest(req)

	// Determine if this is sync or async mode
	// Per spec: sync=false (or not provided) returns 202 "accepted"
	isSync := params.Sync != nil && *params.Sync

	if !isSync {
		// Async mode: For v1, we still process synchronously but return 202
		// In v1.1 with River queue, this would actually queue the work
		result, err := s.ingestService.Ingest(r.Context(), projectID, &svcReq)
		if err != nil {
			if ingest.IsValidationError(err) {
				writeError(w, http.StatusBadRequest, "validation_error", err.Error())
				return
			}
			log.Printf("ingest async mode failed: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred")
			return
		}

		// Async mode returns 202 with status: "accepted" per spec
		// For duplicates, return 202 with status: "duplicate"
		status := IngestResponseStatus(ingest.IngestStatusAccepted)
		if result.Status == string(ingest.IngestStatusDuplicate) {
			status = IngestResponseStatusDuplicate
		}
		resp := IngestResponse{
			Status:   status,
			BatchKey: result.BatchKey,
		}
		writeJSON(w, http.StatusAccepted, resp)
		return
	}

	// Sync mode: process and return 200
	result, err := s.ingestService.Ingest(r.Context(), projectID, &svcReq)
	if err != nil {
		if ingest.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		log.Printf("ingest sync mode failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred")
		return
	}

	// Sync mode returns 200 with full result
	resp := IngestResponse{
		Status:        IngestResponseStatus(result.Status),
		BatchKey:      result.BatchKey,
		TraceCount:    &result.TraceCount,
		SpanCount:     &result.SpanCount,
		EventCount:    &result.EventCount,
		AcceptedCount: &result.AcceptedCount,
		RejectedCount: &result.RejectedCount,
	}
	if len(result.Errors) > 0 {
		resp.Errors = &result.Errors
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListTraces returns a paginated list of traces with optional filtering.
//
//nolint:gocritic // Signature is generated from the OpenAPI contract.
func (s *Server) ListTraces(w http.ResponseWriter, r *http.Request, params ListTracesParams) {
	// Get project ID from auth context
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	limit, offset := normalizePagination(params.Limit, params.Offset)

	// Check if any filter params are provided (besides limit/offset/session_id)
	hasFilters := params.Q != nil || params.Status != nil || params.StartTimeFrom != nil ||
		params.StartTimeTo != nil || params.UserId != nil || params.HasErrors != nil ||
		params.MinDurationMs != nil

	var traces []platform.Trace
	var total int64
	var err error

	if hasFilters || params.SessionId != nil {
		// Use filtered search
		filter := store.TraceFilter{
			ProjectID: projectID,
			Limit:     limit,
			Offset:    offset,
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

		result, err := s.store.ListTracesFiltered(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to search traces")
			return
		}
		traces = result.Traces
		total = result.Total
	} else {
		// Use simple list without filters
		traces, err = s.store.ListTraces(r.Context(), projectID, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list traces")
			return
		}
		total, err = s.store.CountTraces(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count traces")
			return
		}
	}

	// Convert to API types
	apiTraces := make([]Trace, len(traces))
	for i := range traces {
		apiTraces[i] = traceToAPI(&traces[i])
	}

	resp := TraceList{
		Traces: apiTraces,
		Total:  int(total),
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetTrace returns a trace by ID.
func (s *Server) GetTrace(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	// Get project ID from auth context
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	trace, ok := s.getScopedTrace(r.Context(), w, projectID, id)
	if !ok {
		return
	}

	resp := traceToAPI(&trace)
	writeJSON(w, http.StatusOK, resp)
}

// ListSpansByTrace returns spans for a trace.
func (s *Server) ListSpansByTrace(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	// Get project ID from auth context
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	if _, ok := s.getScopedTrace(r.Context(), w, projectID, id); !ok {
		return
	}

	spans, err := s.store.ListSpansByTrace(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list spans")
		return
	}

	apiSpans := make([]Span, len(spans))
	for i := range spans {
		apiSpans[i] = spanToAPI(&spans[i])
	}

	resp := SpanList{
		Spans: apiSpans,
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetTraceEvents returns merged explicit and synthetic timeline events for a trace.
func (s *Server) GetTraceEvents(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, params GetTraceEventsParams) {
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	trace, ok := s.getScopedTrace(r.Context(), w, projectID, id)
	if !ok {
		return
	}

	explicitEvents, err := s.store.ListSpanEventsByTrace(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list timeline events")
		return
	}

	spans, err := s.store.ListSpansByTrace(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list spans")
		return
	}

	entries := buildTimelineEntries(explicitEvents, spans)
	limit := int(normalizeLimit(params.Limit, defaultTimelineLimit, maxTimelineLimit))

	page, err := paginateTimelineEntries(entries, params.After, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor")
		return
	}

	apiEvents := make([]TimelineEvent, len(page.page))
	for i := range page.page {
		apiEvents[i] = page.page[i].event
	}

	resp := TimelineResponse{
		Events:      apiEvents,
		HasMore:     page.hasMore,
		TraceStatus: mapTimelineTraceStatus(trace.Status),
	}
	if page.nextCursor != nil {
		resp.NextCursor = page.nextCursor
	}
	if page.pollCursor != nil {
		resp.PollCursor = page.pollCursor
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListSessions returns a paginated list of sessions.
func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request, params ListSessionsParams) {
	// Get project ID from auth context
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	limit, offset := normalizePagination(params.Limit, params.Offset)

	sessions, err := s.store.ListSessionsWithTraceCount(r.Context(), projectID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
		return
	}

	total, err := s.store.CountSessions(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count sessions")
		return
	}

	apiSessions := make([]Session, len(sessions))
	for i := range sessions {
		apiSessions[i] = sessionWithCountToAPI(&sessions[i].Session, sessions[i].TraceCount)
	}

	resp := SessionList{
		Sessions: apiSessions,
		Total:    int(total),
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetSession returns a session by ID.
func (s *Server) GetSession(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	// Get project ID from auth context
	projectID, ok := middleware.GetProjectID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing project context")
		return
	}

	sessionWithCount, err := s.store.GetSessionWithTraceCount(r.Context(), id)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get session")
		return
	}

	// Verify session belongs to the project (multi-tenant isolation)
	if sessionWithCount.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}

	resp := sessionWithCountToAPI(&sessionWithCount.Session, sessionWithCount.TraceCount)
	writeJSON(w, http.StatusOK, resp)
}

// Helper functions

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
	// MaxBytesReader returns an error with message "http: request body too large"
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}
	// Fallback for older Go versions or wrapped errors
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

// convertToServiceRequest converts API types to service types.
func convertToServiceRequest(req IngestRequest) ingest.IngestRequest {
	svcReq := ingest.IngestRequest{
		BatchKey: req.BatchKey,
	}

	if req.Traces != nil {
		for i := range *req.Traces {
			svcReq.Traces = append(svcReq.Traces, convertTraceInput(&(*req.Traces)[i]))
		}
	}

	if req.Spans != nil {
		for i := range *req.Spans {
			svcReq.Spans = append(svcReq.Spans, convertSpanInput(&(*req.Spans)[i]))
		}
	}

	if req.Events != nil {
		for i := range *req.Events {
			svcReq.Events = append(svcReq.Events, convertEventInput(&(*req.Events)[i]))
		}
	}

	return svcReq
}

func convertTraceInput(t *IngestTraceInput) ingest.TraceInput {
	input := ingest.TraceInput{
		TraceID: t.TraceId,
		Tags:    derefSlice(t.Tags),
	}

	if t.SessionId != nil {
		input.SessionID = t.SessionId
	}
	if t.Name != nil {
		input.Name = t.Name
	}
	if t.UserId != nil {
		input.UserID = t.UserId
	}
	if t.Environment != nil {
		input.Environment = t.Environment
	}
	if t.Release != nil {
		input.Release = t.Release
	}
	if t.Metadata != nil {
		input.Metadata = *t.Metadata
	}
	if t.Input != nil {
		input.Input = t.Input
	}
	if t.Output != nil {
		input.Output = t.Output
	}
	if t.Status != nil {
		s := string(*t.Status)
		input.Status = &s
	}
	if t.StartTime != nil {
		input.StartTime = t.StartTime
	}
	if t.EndTime != nil {
		input.EndTime = t.EndTime
	}

	return input
}

func convertSpanInput(sp *IngestSpanInput) ingest.SpanInput {
	input := ingest.SpanInput{
		TraceID:   sp.TraceId,
		SpanID:    sp.SpanId,
		Name:      sp.Name,
		StartTime: sp.StartTime,
	}

	if sp.ParentSpanId != nil {
		input.ParentSpanID = sp.ParentSpanId
	}
	if sp.Type != nil {
		s := string(*sp.Type)
		input.Type = &s
	}
	if sp.Status != nil {
		s := string(*sp.Status)
		input.Status = &s
	}
	if sp.StatusMessage != nil {
		input.StatusMessage = sp.StatusMessage
	}
	if sp.Level != nil {
		s := string(*sp.Level)
		input.Level = &s
	}
	if sp.EndTime != nil {
		input.EndTime = sp.EndTime
	}
	if sp.Input != nil {
		input.Input = sp.Input
	}
	if sp.Output != nil {
		input.Output = sp.Output
	}
	if sp.Model != nil {
		input.Model = sp.Model
	}
	if sp.Provider != nil {
		input.Provider = sp.Provider
	}
	if sp.PromptTokens != nil {
		input.PromptTokens = sp.PromptTokens
	}
	if sp.CompletionTokens != nil {
		input.CompletionTokens = sp.CompletionTokens
	}
	if sp.TotalTokens != nil {
		input.TotalTokens = sp.TotalTokens
	}
	if sp.TotalCost != nil {
		input.TotalCost = sp.TotalCost
	}
	if sp.Metadata != nil {
		input.Metadata = *sp.Metadata
	}
	if sp.Sequence != nil {
		input.Sequence = sp.Sequence
	}
	if sp.Depth != nil {
		input.Depth = sp.Depth
	}

	return input
}

func convertEventInput(e *IngestEventInput) ingest.EventInput {
	input := ingest.EventInput{
		TraceID: e.TraceId,
		SpanID:  e.SpanId,
	}

	if e.EventType != nil {
		s := string(*e.EventType)
		input.EventType = &s
	}
	if e.Level != nil {
		s := string(*e.Level)
		input.Level = &s
	}
	if e.EventTs != nil {
		input.EventTs = e.EventTs
	}
	if e.Sequence != nil {
		input.Sequence = e.Sequence
	}
	if e.Message != nil {
		input.Message = e.Message
	}
	if e.Payload != nil {
		input.Payload = *e.Payload
	}
	if e.IdempotencyKey != nil {
		input.IdempotencyKey = e.IdempotencyKey
	}

	return input
}

func derefSlice[T any](ptr *[]T) []T {
	if ptr == nil {
		return nil
	}
	return *ptr
}
