package api

import (
	"net/http"

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

	resp := traceDetailToAPI(&trace)
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
