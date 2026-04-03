package store

import (
	"errors"
	"testing"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestAdvanceProjectionCheckpointRejectsBackwardsMoves(t *testing.T) {
	ts := newTestStore(t)

	checkpoint, err := ts.store.AdvanceProjectionCheckpoint(ts.ctx, enginedb.AdvanceProjectionCheckpointParams{
		ProjectionName: "timeline-projector",
		ScopeKey:       "project:all",
		LastHistoryID:  10,
	})
	if err != nil {
		t.Fatalf("AdvanceProjectionCheckpoint() initial error = %v", err)
	}
	if checkpoint.LastHistoryID != 10 {
		t.Fatalf("expected initial checkpoint 10, got %d", checkpoint.LastHistoryID)
	}

	checkpoint, err = ts.store.AdvanceProjectionCheckpoint(ts.ctx, enginedb.AdvanceProjectionCheckpointParams{
		ProjectionName: "timeline-projector",
		ScopeKey:       "project:all",
		LastHistoryID:  20,
	})
	if err != nil {
		t.Fatalf("AdvanceProjectionCheckpoint() forward error = %v", err)
	}
	if checkpoint.LastHistoryID != 20 {
		t.Fatalf("expected advanced checkpoint 20, got %d", checkpoint.LastHistoryID)
	}

	_, err = ts.store.AdvanceProjectionCheckpoint(ts.ctx, enginedb.AdvanceProjectionCheckpointParams{
		ProjectionName: "timeline-projector",
		ScopeKey:       "project:all",
		LastHistoryID:  15,
	})
	if !errors.Is(err, ErrStaleCheckpoint) {
		t.Fatalf("expected ErrStaleCheckpoint, got %v", err)
	}
}
