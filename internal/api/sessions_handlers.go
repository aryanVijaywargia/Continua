package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/internal/store"
)

const sessionNarrativeTraceLimit int32 = 100

// ListSessions returns a paginated list of sessions.
func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request, params ListSessionsParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
		return
	}

	limit, offset := normalizePagination(params.Limit, params.Offset)

	filter := sessionFilterFromParams(projectID, &params, limit, offset)

	var sessions []store.SessionWithCount
	var total int64
	var err error

	if sessionNeedsDynamicQuery(&filter) {
		result, err := s.store.ListSessionsFiltered(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
			return
		}
		sessions = result.Sessions
		total = result.Total
	} else {
		sessions, err = s.store.ListSessionsWithTraceCount(r.Context(), projectID, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
			return
		}

		total, err = s.store.CountSessions(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count sessions")
			return
		}
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
func (s *Server) GetSession(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, _ GetSessionParams) {
	selectedProjectID, ok := selectedProjectIDFromRequest(w, r, true)
	if !ok {
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

	if !projectMatchesSelection(selectedProjectID, sessionWithCount.ProjectID) {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}

	resp := sessionWithCountToAPI(&sessionWithCount.Session, sessionWithCount.TraceCount)
	writeJSON(w, http.StatusOK, resp)
}

// GetSessionNarrative returns a session narrative by ID.
func (s *Server) GetSessionNarrative(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, _ GetSessionNarrativeParams) {
	selectedProjectID, ok := selectedProjectIDFromRequest(w, r, true)
	if !ok {
		return
	}

	sessionWithCount, err := s.store.GetSessionWithTraceCount(r.Context(), id)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get session narrative")
		return
	}
	if !projectMatchesSelection(selectedProjectID, sessionWithCount.ProjectID) {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}

	narrative, err := s.store.BuildSessionNarrative(
		r.Context(),
		sessionWithCount.ProjectID,
		id,
		sessionNarrativeTraceLimit,
	)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get session narrative")
		return
	}

	writeJSON(w, http.StatusOK, sessionNarrativeToAPI(&narrative))
}

// GetSessionCompare returns a deterministic comparison for two traces in a session.
func (s *Server) GetSessionCompare(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, params GetSessionCompareParams) {
	selectedProjectID, ok := selectedProjectIDFromRequest(w, r, true)
	if !ok {
		return
	}

	sessionWithCount, err := s.store.GetSessionWithTraceCount(r.Context(), id)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Session or trace not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get session comparison")
		return
	}
	if !projectMatchesSelection(selectedProjectID, sessionWithCount.ProjectID) {
		writeError(w, http.StatusNotFound, "not_found", "Session or trace not found")
		return
	}

	comparison, err := s.store.BuildSessionComparison(
		r.Context(),
		sessionWithCount.ProjectID,
		id,
		params.BaselineTraceId,
		params.CandidateTraceId,
	)
	if store.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "Session or trace not found")
		return
	}

	var validationErr *store.SessionCompareValidationError
	if errors.As(err, &validationErr) {
		writeError(w, http.StatusBadRequest, validationErr.Code, validationErr.Message)
		return
	}

	var tooLargeErr *store.SessionCompareTooLargeError
	if errors.As(err, &tooLargeErr) {
		writeJSON(w, http.StatusUnprocessableEntity, ComparisonTooLargeError{
			Code:    "comparison_too_large",
			Message: tooLargeErr.Message,
			Detail: ComparisonTooLargeErrorDetail{
				BaselineSpanCount:      tooLargeErr.Detail.BaselineSpanCount,
				CandidateSpanCount:     tooLargeErr.Detail.CandidateSpanCount,
				BaselineSemanticCount:  tooLargeErr.Detail.BaselineSemanticCount,
				CandidateSemanticCount: tooLargeErr.Detail.CandidateSemanticCount,
				MaxSpans:               tooLargeErr.Detail.MaxSpans,
				MaxSemanticEvents:      tooLargeErr.Detail.MaxSemanticEvents,
			},
		})
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get session comparison")
		return
	}

	if err := s.normalizeComparisonProjectionState(
		r.Context(),
		sessionWithCount.ProjectID,
		&comparison,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get session comparison")
		return
	}

	writeJSON(w, http.StatusOK, sessionCompareToAPI(&comparison))
}

func (s *Server) normalizeComparisonProjectionState(
	ctx context.Context,
	projectID uuid.UUID,
	comparison *store.SessionComparison,
) error {
	if s.engineControl == nil || comparison == nil {
		return nil
	}
	if err := s.normalizeCompareTraceProjectionState(ctx, projectID, &comparison.Baseline); err != nil {
		return err
	}
	return s.normalizeCompareTraceProjectionState(ctx, projectID, &comparison.Candidate)
}

func (s *Server) normalizeCompareTraceProjectionState(
	ctx context.Context,
	projectID uuid.UUID,
	header *store.SessionCompareTraceHeader,
) error {
	if s.engineControl == nil || header == nil || header.EngineRunID == nil {
		return nil
	}

	projectionState, err := s.engineControl.projectionStateForRun(ctx, projectID, *header.EngineRunID)
	if err != nil {
		return err
	}
	header.EngineProjectionState = &projectionState
	return nil
}
