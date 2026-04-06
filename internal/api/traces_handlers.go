package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/internal/store"
)

// ListTraces returns a paginated list of traces with optional filtering.
//
//nolint:gocritic // Signature is generated from the OpenAPI contract.
func (s *Server) ListTraces(w http.ResponseWriter, r *http.Request, params ListTracesParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	limit, offset := normalizePagination(params.Limit, params.Offset)

	var traces []store.TraceRead
	var total int64
	var err error

	filter := traceFilterFromParams(projectID, &params, limit, offset)
	switch {
	case traceNeedsDynamicQuery(&filter):
		result, err := s.store.ListTracesFiltered(r.Context(), filter)
		if err != nil {
			var filterErr *store.TraceFilterValidationError
			if errors.As(err, &filterErr) {
				writeError(w, http.StatusBadRequest, "invalid_request", filterErr.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to search traces")
			return
		}
		traces = result.Traces
		total = result.Total
	case filter.SessionID != nil:
		traces, err = s.store.ListTracesBySession(r.Context(), projectID, *filter.SessionID, limit, offset, filter.SortDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list traces")
			return
		}
		total, err = s.store.CountTracesBySession(r.Context(), projectID, *filter.SessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count traces")
			return
		}
	default:
		traces, err = s.store.ListTraces(r.Context(), projectID, limit, offset, filter.SortDir)
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

	if err := s.normalizeTraceProjectionStates(r.Context(), traces); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list traces")
		return
	}

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
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	trace, ok := s.getScopedTrace(r.Context(), w, projectID, id)
	if !ok {
		return
	}

	var engineSummary *EngineRunSummary
	readLiveEngineSummary := shouldReadLiveEngineSummary(&trace)
	if !readLiveEngineSummary && s.engineControl != nil {
		needsLiveSummary, err := s.engineControl.shouldReadLiveTraceSummary(r.Context(), &trace)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read engine summary")
			return
		}
		readLiveEngineSummary = needsLiveSummary
	}

	if readLiveEngineSummary {
		if s.engineControl == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read engine summary")
			return
		}

		summary, err := s.engineControl.ReadRunSummary(r.Context(), projectID, uuid.UUID(trace.EngineRunID.Bytes))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read engine summary")
			return
		}

		mapped := engineRunSummaryToAPI(&summary)
		engineSummary = &mapped
	}

	resp := traceDetailToAPIWithEngine(&trace, engineSummary)
	writeJSON(w, http.StatusOK, resp)
}

// ListSpansByTrace returns spans for a trace.
func (s *Server) ListSpansByTrace(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
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
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	trace, ok := s.getScopedTrace(r.Context(), w, projectID, id)
	if !ok {
		return
	}

	if err := s.normalizeTraceProjectionState(r.Context(), &trace); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list timeline events")
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
		Engine:      engineTimelineMetadataFromTrace(&trace),
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

func (s *Server) normalizeTraceProjectionStates(ctx context.Context, traces []store.TraceRead) error {
	if s.engineControl == nil {
		return nil
	}
	for i := range traces {
		if err := s.normalizeTraceProjectionState(ctx, &traces[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) normalizeTraceProjectionState(ctx context.Context, trace *store.TraceRead) error {
	if s.engineControl == nil || trace == nil || !trace.EngineRunID.Valid {
		return nil
	}

	projectionState, err := s.engineControl.projectionStateForTrace(ctx, trace)
	if err != nil {
		return err
	}
	trace.EngineProjectionState = &projectionState
	return nil
}
