package api

import (
	"net/http"

	"github.com/continua-ai/continua/internal/api/middleware"
)

const visibleProjectsPageSize int32 = 500

// ListProjects returns the project set visible to the current request.
// API-key callers remain limited to their bound project; operator tokens can see all projects.
func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request) {
	if projectID, ok := middleware.GetProjectID(r.Context()); ok {
		project, err := s.store.GetProject(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list projects")
			return
		}
		writeJSON(w, http.StatusOK, ProjectList{Projects: []Project{projectToAPI(&project)}})
		return
	}

	apiProjects := make([]Project, 0, visibleProjectsPageSize)
	for offset := int32(0); ; offset += visibleProjectsPageSize {
		projects, err := s.store.ListProjects(r.Context(), visibleProjectsPageSize, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list projects")
			return
		}
		for i := range projects {
			apiProjects = append(apiProjects, projectToAPI(&projects[i]))
		}
		if int32(len(projects)) < visibleProjectsPageSize {
			break
		}
	}

	writeJSON(w, http.StatusOK, ProjectList{Projects: apiProjects})
}
