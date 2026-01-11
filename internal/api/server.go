package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

// MaxBodySize is the maximum request body size (5MB).
const MaxBodySize = 5 * 1024 * 1024

// Server implements the ServerInterface for the Continua API.
type Server struct {
	store         *store.Store
	ingestService *ingest.Service
}

// NewServer creates a new API server with the given dependencies.
func NewServer(s *store.Store) *Server {
	return &Server{
		store:         s,
		ingestService: ingest.NewService(s),
	}
}

// HealthCheck implements the health check endpoint.
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  HealthResponseStatusOk,
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

	// Use default project for now (v1 single-tenant)
	project, err := s.store.GetDefaultProject(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get project")
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
		result, err := s.ingestService.Ingest(r.Context(), project.ID, &svcReq)
		if err != nil {
			if ingest.IsValidationError(err) {
				writeError(w, http.StatusBadRequest, "validation_error", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
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
	result, err := s.ingestService.Ingest(r.Context(), project.ID, &svcReq)
	if err != nil {
		if ingest.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
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

// ListTraces returns a paginated list of traces.
func (s *Server) ListTraces(w http.ResponseWriter, r *http.Request, params ListTracesParams) {
	// Get default project
	project, err := s.store.GetDefaultProject(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get project")
		return
	}

	limit := int32(50)
	offset := int32(0)
	if params.Limit != nil {
		limit = int32(*params.Limit)
	}
	if params.Offset != nil {
		offset = int32(*params.Offset)
	}

	traces, err := s.store.ListTraces(r.Context(), project.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list traces")
		return
	}

	total, err := s.store.CountTraces(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count traces")
		return
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
	trace, err := s.store.GetTrace(r.Context(), id)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Trace not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get trace")
		return
	}

	resp := traceToAPI(&trace)
	writeJSON(w, http.StatusOK, resp)
}

// ListSpansByTrace returns spans for a trace.
func (s *Server) ListSpansByTrace(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
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

// ListSessions returns a paginated list of sessions.
func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request, params ListSessionsParams) {
	project, err := s.store.GetDefaultProject(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get project")
		return
	}

	limit := int32(50)
	offset := int32(0)
	if params.Limit != nil {
		limit = int32(*params.Limit)
	}
	if params.Offset != nil {
		offset = int32(*params.Offset)
	}

	sessions, err := s.store.ListSessions(r.Context(), project.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
		return
	}

	total, err := s.store.CountSessions(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count sessions")
		return
	}

	apiSessions := make([]Session, len(sessions))
	for i := range sessions {
		apiSessions[i] = sessionToAPI(&sessions[i])
	}

	resp := SessionList{
		Sessions: apiSessions,
		Total:    int(total),
	}
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
		s := t.SessionId.String()
		input.SessionID = &s
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
