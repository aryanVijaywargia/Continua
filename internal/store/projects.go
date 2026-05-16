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

// CountProjects returns the total number of configured projects.
func (s *Store) CountProjects(ctx context.Context) (int64, error) {
	return s.q.CountProjects(ctx)
}

// CreateProject inserts a new project row. apiKeyHash must be the SHA-256 hash
// of the plaintext key that will be returned to the caller exactly once.
func (s *Store) CreateProject(ctx context.Context, name, apiKeyHash string) (platform.Project, error) {
	return s.q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       name,
		ApiKeyHash: apiKeyHash,
	})
}

// UpdateProject renames a project. Returns ErrNotFound if no row matches id.
func (s *Store) UpdateProject(ctx context.Context, id uuid.UUID, name string) (platform.Project, error) {
	project, err := s.q.UpdateProject(ctx, platform.UpdateProjectParams{
		ID:   id,
		Name: name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Project{}, ErrNotFound
	}
	return project, err
}

// RotateProjectAPIKey replaces the stored api_key_hash for the project.
// Returns ErrNotFound if no row matches id.
func (s *Store) RotateProjectAPIKey(ctx context.Context, id uuid.UUID, apiKeyHash string) (platform.Project, error) {
	project, err := s.q.RotateProjectAPIKey(ctx, platform.RotateProjectAPIKeyParams{
		ID:         id,
		ApiKeyHash: apiKeyHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Project{}, ErrNotFound
	}
	return project, err
}

// DeleteProject removes a project. Returns ErrNotFound if no row was deleted.
// Associated data (traces, sessions, spans, payloads) cascades via foreign keys.
func (s *Store) DeleteProject(ctx context.Context, id uuid.UUID) error {
	rows, err := s.q.DeleteProject(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
