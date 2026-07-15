package worker

import (
	"context"
	"time"
)

type IterationFunc func(context.Context, string) error

func RunLoop(ctx context.Context, pollInterval time.Duration, workerID string, fn IterationFunc) error {
	if err := fn(ctx, workerID); err != nil {
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
			if err := fn(ctx, workerID); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// RunLoopWithWake is a compile-only scaffold for notification acceptance tests.
func RunLoopWithWake(
	ctx context.Context,
	baseInterval time.Duration,
	_ time.Duration,
	_ func() bool,
	_ <-chan struct{},
	workerID string,
	fn IterationFunc,
) error {
	return RunLoop(ctx, baseInterval, workerID, fn)
}
