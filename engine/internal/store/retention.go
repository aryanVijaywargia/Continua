package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RetentionReapCounts reports journal rows deleted for a terminal run.
type RetentionReapCounts struct {
	History       int64
	Inbox         int64
	ActivityTasks int64
}

// ReapRequestDedupe is the retention store API scaffold.
func (s *Store) ReapRequestDedupe(context.Context, time.Time, int32) (int64, error) {
	return 0, nil
}

// ListRetainableTerminalRunIDs is the retention store API scaffold.
func (s *Store) ListRetainableTerminalRunIDs(context.Context, time.Time, int32) ([]uuid.UUID, error) {
	return nil, nil
}

// ReapTerminalRunJournal is the retention store API scaffold.
func (s *Store) ReapTerminalRunJournal(context.Context, uuid.UUID) (RetentionReapCounts, error) {
	return RetentionReapCounts{}, nil
}
