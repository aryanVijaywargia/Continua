package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// GetSession retrieves a session by its internal UUID.
func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (platform.Session, error) {
	session, err := s.q.GetSession(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Session{}, ErrNotFound
	}
	return session, err
}

// ListSessions returns paginated sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]platform.Session, error) {
	return s.q.ListSessions(ctx, platform.ListSessionsParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
}

// CountSessions returns the total number of sessions for a project.
func (s *Store) CountSessions(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return s.q.CountSessions(ctx, projectID)
}

// CreateSession creates a new session.
func (s *Store) CreateSession(ctx context.Context, params platform.CreateSessionParams) (platform.Session, error) {
	return s.q.CreateSession(ctx, params)
}

// CreateSessionTx creates a session within a transaction.
func (t *Tx) CreateSession(ctx context.Context, params platform.CreateSessionParams) (platform.Session, error) {
	return t.q.CreateSession(ctx, params)
}

// UpdateSession updates an existing session.
func (s *Store) UpdateSession(ctx context.Context, params platform.UpdateSessionParams) (platform.Session, error) {
	return s.q.UpdateSession(ctx, params)
}
