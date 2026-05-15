package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type IterationFunc func(context.Context, string) error

func RunLoop(ctx context.Context, pollInterval time.Duration, prefix string, fn IterationFunc) error {
	if err := fn(ctx, workerID(prefix)); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := fn(ctx, workerID(prefix)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func workerID(prefix string) string {
	return prefix + ":" + uuid.NewString()
}
