package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

// HealthResponse is the response for the health check endpoint.
// Note: This is defined locally because /api/health is routed directly
// in Chi (not via OpenAPI) to avoid auth middleware complexity.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
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
	if r.ContentLength > MaxBodySize {
		write413Error(w, "batch exceeds 5MB limit")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if isMaxBytesError(err) {
			write413Error(w, "batch exceeds 5MB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to read request body: "+err.Error())
		return
	}

	var req IngestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		if isMaxBytesError(err) {
			write413Error(w, "batch exceeds 5MB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body: "+err.Error())
		return
	}

	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	svcReq := convertToServiceRequest(req)

	isSync := params.Sync != nil && *params.Sync
	asyncVersion := strings.TrimSpace(r.Header.Get("X-Continua-Async-Version"))
	if asyncVersion != "" && asyncVersion != "2" {
		writeError(w, http.StatusBadRequest, "unsupported_async_version", "Unsupported X-Continua-Async-Version header")
		return
	}

	if !isSync && (asyncVersion == "2" || s.ingestService.TrueAsyncDefault()) {
		result, err := s.ingestService.AcceptAsync(r.Context(), projectID, &svcReq, body)
		if err != nil {
			if ingest.IsValidationError(err) {
				writeError(w, http.StatusBadRequest, "validation_error", err.Error())
				return
			}
			log.Printf("ingest true async acceptance failed: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred")
			return
		}

		writeJSON(w, http.StatusAccepted, apiIngestResponse(result, false))
		return
	}

	result, err := s.ingestService.Ingest(r.Context(), projectID, &svcReq)
	if err != nil {
		if ingest.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		log.Printf("ingest inline mode failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred")
		return
	}

	if !isSync {
		resp := apiIngestResponse(result, false)
		if resp.Status != IngestResponseStatusDuplicate {
			resp.Status = IngestResponseStatusAccepted
			resp.TraceCount = nil
			resp.SpanCount = nil
			resp.EventCount = nil
			resp.AcceptedCount = nil
			resp.RejectedCount = nil
			resp.Errors = nil
		}
		writeJSON(w, http.StatusAccepted, resp)
		return
	}

	writeJSON(w, http.StatusOK, apiIngestResponse(result, true))
}

// GetBatchStatus returns the processing status for a previously accepted batch.
func (s *Server) GetBatchStatus(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	status, err := s.ingestService.GetBatchStatus(r.Context(), projectID, id)
	if err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Batch not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get batch status")
		return
	}

	writeJSON(w, http.StatusOK, apiBatchStatus(status))
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
