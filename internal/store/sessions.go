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

type sessionWithCountRow interface {
	platform.GetSessionWithTraceCountRow | platform.ListSessionsWithTraceCountRow
}

func mapSessionWithCountRow[T sessionWithCountRow](row T) SessionWithCount {
	switch row := any(row).(type) {
	case platform.GetSessionWithTraceCountRow:
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
		}
	case platform.ListSessionsWithTraceCountRow:
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
		}
	default:
		return SessionWithCount{}
	}
}

// GetSessionWithTraceCount retrieves a session with its trace count within the supplied scope.
func (s *Store) GetSessionWithTraceCount(ctx context.Context, scope Scope, id uuid.UUID) (SessionWithCount, error) {
	row, err := s.q.GetSessionWithTraceCount(ctx, platform.GetSessionWithTraceCountParams{
		ID:              id,
		ProjectFilterID: scope.nullableProjectFilter(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionWithCount{}, ErrNotFound
	}
	if err != nil {
		return SessionWithCount{}, err
	}
	return mapSessionWithCountRow(row), nil
}

// ListSessionsWithTraceCount returns paginated sessions with trace counts within the supplied scope.
func (s *Store) ListSessionsWithTraceCount(ctx context.Context, scope Scope, limit, offset int32) ([]SessionWithCount, error) {
	rows, err := s.q.ListSessionsWithTraceCount(ctx, platform.ListSessionsWithTraceCountParams{
		ProjectFilterID: scope.nullableProjectFilter(),
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		return nil, err
	}

	result := make([]SessionWithCount, len(rows))
	for i := range rows {
		result[i] = mapSessionWithCountRow(rows[i])
	}
	return result, nil
}

// CountSessions returns the total number of sessions within the supplied scope.
func (s *Store) CountSessions(ctx context.Context, scope Scope) (int64, error) {
	return s.q.CountSessions(ctx, scope.nullableProjectFilter())
}

// GetOrCreateSessionByExternalIDTx upserts a session within a transaction.
func (t *Tx) GetOrCreateSessionByExternalID(ctx context.Context, projectID uuid.UUID, externalID string) (platform.Session, error) {
	return t.q.GetOrCreateSessionByExternalID(ctx, platform.GetOrCreateSessionByExternalIDParams{
		ProjectID:  projectID,
		ExternalID: externalID,
	})
}

// UpdateSession updates session mutable fields within a transaction.
func (t *Tx) UpdateSession(ctx context.Context, params platform.UpdateSessionParams) (platform.Session, error) {
	return t.q.UpdateSession(ctx, params)
}
