package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// GetProject retrieves a project by ID.
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

// ListProjects returns projects in descending creation order.
func (s *Store) ListProjects(ctx context.Context, limit, offset int32) ([]platform.Project, error) {
	return s.q.ListProjects(ctx, platform.ListProjectsParams{
		Limit:  limit,
		Offset: offset,
	})
}
