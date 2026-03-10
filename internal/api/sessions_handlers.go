package api

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/internal/store"
)

// ListSessions returns a paginated list of sessions.
func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request, params ListSessionsParams) {
	projectID, ok := projectIDOrUnauthorized(w, r)
	if !ok {
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
	projectID, ok := projectIDOrUnauthorized(w, r)
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

	if sessionWithCount.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}

	resp := sessionWithCountToAPI(&sessionWithCount.Session, sessionWithCount.TraceCount)
	writeJSON(w, http.StatusOK, resp)
}
