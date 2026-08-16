package store

import (
	"context"
	"encoding/json"
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

// SessionContextParams carries the session fields an ingestion adapter resolved for a
// trace. Empty values must be passed as nil: they mean "the producer did not emit this".
type SessionContextParams struct {
	Name     *string
	UserID   *string
	UserName *string
}

// sessionUserNameKey is where a session's display user name lives inside sessions.metadata.
const sessionUserNameKey = "user_name"

// UpsertSessionContext resolves a session by (project_id, external_id) and materializes
// the supplied context with first-non-empty-wins semantics: stored non-empty values are
// never replaced, missing fields are filled, and metadata merges additively.
func (t *Tx) UpsertSessionContext(
	ctx context.Context,
	projectID uuid.UUID,
	externalID string,
	params SessionContextParams,
) (platform.Session, error) {
	metadata := []byte("{}")
	if params.UserName != nil {
		metadata, _ = json.Marshal(map[string]string{sessionUserNameKey: *params.UserName})
	}

	return t.q.UpsertSessionContext(ctx, platform.UpsertSessionContextParams{
		ProjectID:  projectID,
		ExternalID: externalID,
		Name:       params.Name,
		UserID:     params.UserID,
		Metadata:   metadata,
	})
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
