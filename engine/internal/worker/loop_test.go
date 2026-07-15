package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunLoopSuppressesIterationErrorAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	iterationErr := errors.New("write failed after cancellation")

	err := RunLoop(ctx, time.Hour, "test:fixed", func(context.Context, string) error {
		cancel()
		return iterationErr
	})
	if err != nil {
		t.Fatalf("RunLoop() error = %v, want nil", err)
	}
}

func TestRunLoopReturnsIterationErrorBeforeCancellation(t *testing.T) {
	iterationErr := errors.New("poll failed")

	err := RunLoop(context.Background(), time.Hour, "test:fixed", func(context.Context, string) error {
		return iterationErr
	})
	if !errors.Is(err, iterationErr) {
		t.Fatalf("RunLoop() error = %v, want %v", err, iterationErr)
	}
}

func TestRunLoopUsesStableWorkerID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	const workerID = "test:fixed"
	iterations := 0

	err := RunLoop(ctx, time.Millisecond, workerID, func(_ context.Context, got string) error {
		iterations++
		if got != workerID {
			t.Fatalf("iteration worker ID = %q, want %q", got, workerID)
		}
		if iterations == 2 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunLoop() error = %v, want nil", err)
	}
}
