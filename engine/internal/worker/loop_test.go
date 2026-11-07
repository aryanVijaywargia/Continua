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

	err := RunLoop(ctx, time.Hour, "test:fixed", func(context.Context, string) (bool, error) {
		cancel()
		return false, iterationErr
	})
	if err != nil {
		t.Fatalf("RunLoop() error = %v, want nil", err)
	}
}

func TestRunLoopReturnsIterationErrorBeforeCancellation(t *testing.T) {
	iterationErr := errors.New("poll failed")

	err := RunLoop(context.Background(), time.Hour, "test:fixed", func(context.Context, string) (bool, error) {
		return false, iterationErr
	})
	if !errors.Is(err, iterationErr) {
		t.Fatalf("RunLoop() error = %v, want %v", err, iterationErr)
	}
}

func TestRunLoopUsesStableWorkerID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	const workerID = "test:fixed"
	iterations := 0

	err := RunLoop(ctx, time.Millisecond, workerID, func(_ context.Context, got string) (bool, error) {
		iterations++
		if got != workerID {
			t.Fatalf("iteration worker ID = %q, want %q", got, workerID)
		}
		if iterations == 2 {
			cancel()
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("RunLoop() error = %v, want nil", err)
	}
}

var drainLoops = map[string]func(context.Context, IterationFunc) error{
	"RunLoop": func(ctx context.Context, fn IterationFunc) error {
		return RunLoop(ctx, time.Hour, "drain-test", fn)
	},
	"RunLoopWithWake": func(ctx context.Context, fn IterationFunc) error {
		return RunLoopWithWake(ctx, time.Hour, time.Hour, func() bool { return true }, nil, "drain-test", fn)
	},
}

func TestLoopsDrainClaimedWorkBeforeWaiting(t *testing.T) {
	t.Parallel()
	for name, runLoop := range drainLoops {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backlog := 100
			idle := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- runLoop(ctx, func(context.Context, string) (bool, error) {
					if backlog == 0 {
						close(idle)
						return false, nil
					}
					backlog--
					return true, nil
				})
			}()

			select {
			case <-idle:
			case <-time.After(2 * time.Second):
				t.Fatal("loop did not drain a 100-item backlog within 2s")
			}
			cancel()
			assertLoopStopsNil(t, done)
		})
	}
}

func TestLoopsStopDrainingAfterCancellation(t *testing.T) {
	t.Parallel()
	for name, runLoop := range drainLoops {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			claims := 0
			done := make(chan error, 1)
			go func() {
				done <- runLoop(ctx, func(context.Context, string) (bool, error) {
					claims++
					if claims == 3 {
						cancel()
					}
					return true, nil
				})
			}()
			assertLoopStopsNil(t, done)
			if claims != 3 {
				t.Fatalf("claims = %d, want 3: the loop must not claim after cancellation", claims)
			}
		})
	}
}
