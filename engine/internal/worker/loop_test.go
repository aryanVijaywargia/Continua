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

	err := RunLoop(ctx, time.Hour, "test", func(context.Context, string) error {
		cancel()
		return iterationErr
	})
	if err != nil {
		t.Fatalf("RunLoop() error = %v, want nil", err)
	}
}

func TestRunLoopReturnsIterationErrorBeforeCancellation(t *testing.T) {
	iterationErr := errors.New("poll failed")

	err := RunLoop(context.Background(), time.Hour, "test", func(context.Context, string) error {
		return iterationErr
	})
	if !errors.Is(err, iterationErr) {
		t.Fatalf("RunLoop() error = %v, want %v", err, iterationErr)
	}
}
