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

func RunLoopWithWake(
	ctx context.Context,
	baseInterval time.Duration,
	fallbackInterval time.Duration,
	healthy func() bool,
	wake <-chan struct{},
	workerID string,
	fn IterationFunc,
) error {
	for {
		if err := fn(ctx, workerID); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		interval := baseInterval
		if healthy() {
			interval = fallbackInterval
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		case <-wake:
			timer.Stop()
		}
	}
}
