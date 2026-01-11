package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// GetProject retrieves a project by its internal UUID.
func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (platform.Project, error) {
	project, err := s.q.GetProject(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Project{}, ErrNotFound
	}
	return project, err
}

// GetProjectByAPIKey retrieves a project by its API key hash.
func (s *Store) GetProjectByAPIKey(ctx context.Context, apiKeyHash string) (platform.Project, error) {
	project, err := s.q.GetProjectByAPIKey(ctx, apiKeyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Project{}, ErrNotFound
	}
	return project, err
}

// GetDefaultProject retrieves the default project.
func (s *Store) GetDefaultProject(ctx context.Context) (platform.Project, error) {
	project, err := s.q.GetDefaultProject(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Project{}, ErrNotFound
	}
	return project, err
}

// ListProjects returns paginated projects.
func (s *Store) ListProjects(ctx context.Context, limit, offset int32) ([]platform.Project, error) {
	return s.q.ListProjects(ctx, platform.ListProjectsParams{
		Limit:  limit,
		Offset: offset,
	})
}

// CreateProject creates a new project.
func (s *Store) CreateProject(ctx context.Context, name, apiKeyHash string) (platform.Project, error) {
	return s.q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       name,
		ApiKeyHash: apiKeyHash,
	})
}

// UpdateProject updates an existing project.
func (s *Store) UpdateProject(ctx context.Context, id uuid.UUID, name string) (platform.Project, error) {
	return s.q.UpdateProject(ctx, platform.UpdateProjectParams{
		ID:   id,
		Name: name,
	})
}
