package worker

import (
	"context"
	"time"
)

// IterationFunc runs one poll. claimed reports that the poll took queued work,
// so the loop polls again at once instead of waiting for the next interval.
type IterationFunc func(ctx context.Context, workerID string) (claimed bool, err error)

// Idle adapts a poll that never reports claimed work, so the loop always waits between polls.
func Idle(fn func(context.Context, string) error) IterationFunc {
	return func(ctx context.Context, workerID string) (bool, error) {
		return false, fn(ctx, workerID)
	}
}

// drain polls until fn claims nothing, fails, or ctx is done.
func drain(ctx context.Context, workerID string, fn IterationFunc) error {
	for {
		claimed, err := fn(ctx, workerID)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !claimed || ctx.Err() != nil {
			return nil
		}
	}
}

func RunLoop(ctx context.Context, pollInterval time.Duration, workerID string, fn IterationFunc) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if err := drain(ctx, workerID, fn); err != nil || ctx.Err() != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
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
		if err := drain(ctx, workerID, fn); err != nil || ctx.Err() != nil {
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
		case _, ok := <-wake:
			timer.Stop()
			if !ok {
				wake = nil
			}
		}
	}
}
