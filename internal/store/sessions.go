package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// SessionWithCount is a session with its trace count.
type SessionWithCount struct {
	platform.Session
	TraceCount int64
}

// GetSession retrieves a session by its internal UUID.
func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (platform.Session, error) {
	session, err := s.q.GetSession(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Session{}, ErrNotFound
	}
	return session, err
}

// GetSessionWithTraceCount retrieves a session with its trace count.
func (s *Store) GetSessionWithTraceCount(ctx context.Context, id uuid.UUID) (SessionWithCount, error) {
	row, err := s.q.GetSessionWithTraceCount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionWithCount{}, ErrNotFound
	}
	if err != nil {
		return SessionWithCount{}, err
	}
	return SessionWithCount{
		Session: platform.Session{
			ID:         row.ID,
			ProjectID:  row.ProjectID,
			Name:       row.Name,
			UserID:     row.UserID,
			Metadata:   row.Metadata,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
			ExternalID: row.ExternalID,
		},
		TraceCount: row.TraceCount,
	}, nil
}

// ListSessions returns paginated sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]platform.Session, error) {
	return s.q.ListSessions(ctx, platform.ListSessionsParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
}

// ListSessionsWithTraceCount returns paginated sessions with trace counts.
func (s *Store) ListSessionsWithTraceCount(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]SessionWithCount, error) {
	rows, err := s.q.ListSessionsWithTraceCount(ctx, platform.ListSessionsWithTraceCountParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}

	result := make([]SessionWithCount, len(rows))
	for i := range rows {
		row := &rows[i]
		result[i] = SessionWithCount{
			Session: platform.Session{
				ID:         row.ID,
				ProjectID:  row.ProjectID,
				Name:       row.Name,
				UserID:     row.UserID,
				Metadata:   row.Metadata,
				CreatedAt:  row.CreatedAt,
				UpdatedAt:  row.UpdatedAt,
				ExternalID: row.ExternalID,
			},
			TraceCount: row.TraceCount,
		}
	}
	return result, nil
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

// GetOrCreateSessionByExternalID upserts a session by (project_id, external_id).
func (s *Store) GetOrCreateSessionByExternalID(ctx context.Context, projectID uuid.UUID, externalID string) (platform.Session, error) {
	return s.q.GetOrCreateSessionByExternalID(ctx, platform.GetOrCreateSessionByExternalIDParams{
		ProjectID:  projectID,
		ExternalID: externalID,
	})
}

// GetOrCreateSessionByExternalIDTx upserts a session within a transaction.
func (t *Tx) GetOrCreateSessionByExternalID(ctx context.Context, projectID uuid.UUID, externalID string) (platform.Session, error) {
	return t.q.GetOrCreateSessionByExternalID(ctx, platform.GetOrCreateSessionByExternalIDParams{
		ProjectID:  projectID,
		ExternalID: externalID,
	})
}

// UpdateSession updates an existing session.
func (s *Store) UpdateSession(ctx context.Context, params platform.UpdateSessionParams) (platform.Session, error) {
	return s.q.UpdateSession(ctx, params)
}
