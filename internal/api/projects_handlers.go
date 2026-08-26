package api

import (
	"errors"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
)

const (
	visibleProjectsPageSize int32 = 500
	maxProjectNameLength    int   = 100
)

// ListProjects returns every project in the deployment.
// Project management is an operator-equivalent concern: in local-first mode the
// API-key holder IS the operator.
func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request) {
	var authenticatedProjectID *openapi_types.UUID
	if projectID, ok := middleware.GetProjectID(r.Context()); ok {
		authenticatedProjectID = &projectID
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

	writeJSON(w, http.StatusOK, ProjectList{
		AuthenticatedProjectId: authenticatedProjectID,
		Projects:               apiProjects,
	})
}

// CreateProject generates a fresh API key and returns it once.
func (s *Server) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req CreateProjectRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if !validateProjectName(w, name) {
		return
	}

	plaintextKey, err := middleware.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate API key")
		return
	}

	project, err := s.store.CreateProject(r.Context(), name, middleware.HashAPIKey(plaintextKey))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create project")
		return
	}

	writeJSON(w, http.StatusCreated, projectWithKeyResponse(&project, plaintextKey))
}

// UpdateProject renames an existing project.
func (s *Server) UpdateProject(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var req UpdateProjectRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if !validateProjectName(w, name) {
		return
	}

	project, err := s.store.UpdateProject(r.Context(), id, name)
	if err != nil {
		writeProjectMutationError(w, err, "Failed to update project")
		return
	}

	writeJSON(w, http.StatusOK, projectToAPI(&project))
}

// RotateProjectAPIKey replaces the project's API key and returns the new plaintext key once.
func (s *Server) RotateProjectAPIKey(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	plaintextKey, err := middleware.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate API key")
		return
	}

	project, err := s.store.RotateProjectAPIKey(r.Context(), id, middleware.HashAPIKey(plaintextKey))
	if err != nil {
		writeProjectMutationError(w, err, "Failed to rotate API key")
		return
	}

	writeJSON(w, http.StatusOK, projectWithKeyResponse(&project, plaintextKey))
}

// DeleteProject removes a project and cascades its data via FK.
func (s *Server) DeleteProject(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if err := s.store.DeleteProject(r.Context(), id); err != nil {
		writeProjectMutationError(w, err, "Failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validateProjectName(w http.ResponseWriter, name string) bool {
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_name", "Project name is required")
		return false
	}
	if len(name) > maxProjectNameLength {
		writeError(w, http.StatusBadRequest, "invalid_name", "Project name must be 100 characters or fewer")
		return false
	}
	return true
}

func writeProjectMutationError(w http.ResponseWriter, err error, fallbackMessage string) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", fallbackMessage)
}

func projectWithKeyResponse(project *platform.Project, apiKey string) ProjectWithKey {
	return ProjectWithKey{
		Id:        project.ID,
		Name:      project.Name,
		CreatedAt: project.CreatedAt,
		UpdatedAt: project.UpdatedAt,
		ApiKey:    apiKey,
	}
}
