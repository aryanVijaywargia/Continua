package worker

import (
	"context"
	"time"

	"github.com/continua-ai/continua/engine/internal/store"
)

// RetentionConfig configures terminal journal and request-dedupe retention.
type RetentionConfig struct {
	TerminalRuns time.Duration
	DedupeGrace  time.Duration
	BatchSize    int32
}

// RetentionWorker removes expired engine journal data.
type RetentionWorker struct {
	store *store.Store
	cfg   RetentionConfig
}

// NewRetentionWorker constructs the retention worker API scaffold.
func NewRetentionWorker(engineStore *store.Store, cfg RetentionConfig) *RetentionWorker {
	return &RetentionWorker{store: engineStore, cfg: cfg}
}

// PollOnce is the retention worker API scaffold.
func (w *RetentionWorker) PollOnce(context.Context, string) error {
	return nil
}
