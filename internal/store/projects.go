package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// GetProjectByAPIKey retrieves a project by its API key hash.
func (s *Store) GetProjectByAPIKey(ctx context.Context, apiKeyHash string) (platform.Project, error) {
	project, err := s.q.GetProjectByAPIKey(ctx, apiKeyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Project{}, ErrNotFound
	}
	return project, err
}
