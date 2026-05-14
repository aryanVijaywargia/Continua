package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
)

const (
	visibleProjectsPageSize int32  = 500
	maxProjectNameLength    int    = 100
	defaultProjectIDString  string = "00000000-0000-0000-0000-000000000001"
)

// ListProjects returns every project in the deployment.
// Project management is an operator-equivalent concern: in local-first mode the API-key
// holder IS the operator. When Auth0 is enabled, only operator tokens may enumerate.
func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorWhenAuth0Enabled(w, r) {
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

// CreateProject generates a fresh API key and returns it once.
func (s *Server) CreateProject(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorWhenAuth0Enabled(w, r) {
		return
	}
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
	if !s.requireOperatorWhenAuth0Enabled(w, r) {
		return
	}
	var req UpdateProjectRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if !validateProjectName(w, name) {
		return
	}

	project, err := s.store.UpdateProject(r.Context(), uuid.UUID(id), name)
	if err != nil {
		writeProjectMutationError(w, err, "Failed to update project")
		return
	}

	writeJSON(w, http.StatusOK, projectToAPI(&project))
}

// RotateProjectAPIKey replaces the project's API key and returns the new plaintext key once.
func (s *Server) RotateProjectAPIKey(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if !s.requireOperatorWhenAuth0Enabled(w, r) {
		return
	}
	plaintextKey, err := middleware.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate API key")
		return
	}

	project, err := s.store.RotateProjectAPIKey(r.Context(), uuid.UUID(id), middleware.HashAPIKey(plaintextKey))
	if err != nil {
		writeProjectMutationError(w, err, "Failed to rotate API key")
		return
	}

	writeJSON(w, http.StatusOK, projectWithKeyResponse(&project, plaintextKey))
}

// DeleteProject removes a project and cascades its data via FK. The seeded default cannot be deleted.
func (s *Server) DeleteProject(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if !s.requireOperatorWhenAuth0Enabled(w, r) {
		return
	}
	if uuid.UUID(id).String() == defaultProjectIDString {
		writeError(w, http.StatusConflict, "default_project_protected", "The seeded default project cannot be deleted")
		return
	}

	if err := s.store.DeleteProject(r.Context(), uuid.UUID(id)); err != nil {
		writeProjectMutationError(w, err, "Failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// requireOperatorWhenAuth0Enabled restricts project management to operator (Auth0) auth
// when Auth0 is configured. In local-first mode (Auth0 disabled) the API-key holder is
// the operator and is allowed through.
//
// Deliberate trade-off: in Auth0-disabled local mode, *any* valid project API key —
// not just the seeded `default` — can list, create, rename, rotate, and delete any
// project. This is acceptable because local mode is single-tenant: the key holder
// owns the box. Deployments that need cross-tenant isolation must enable Auth0,
// which causes API-key callers to receive 403 on every endpoint guarded here.
func (s *Server) requireOperatorWhenAuth0Enabled(w http.ResponseWriter, r *http.Request) bool {
	if !s.auth0Config.Enabled {
		return true
	}
	mode, _ := middleware.GetAuthMode(r.Context())
	if mode == middleware.AuthModeOperator {
		return true
	}
	writeError(
		w,
		http.StatusForbidden,
		"operator_required",
		"Project management requires operator authentication on this deployment",
	)
	return false
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
