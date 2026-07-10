package store

import (
	"context"

	"github.com/google/uuid"
)

// PlatformProjectExists reports whether the platform public.projects row exists.
func (s *Store) PlatformProjectExists(ctx context.Context, projectID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM public.projects
			WHERE id = $1
		)
	`, projectID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
