package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunLoopWithWakeTriggersImmediateIteration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	iterations := make(chan time.Time, 4)
	done := make(chan error, 1)
	go func() {
		done <- RunLoopWithWake(ctx, time.Minute, time.Minute, func() bool { return true }, wake, "wake-test", func(context.Context, string) error {
			iterations <- time.Now()
			return nil
		})
	}()

	select {
	case <-iterations:
	case <-time.After(2 * time.Second):
		t.Fatal("startup iteration did not run within 2s")
	}
	wake <- struct{}{}
	select {
	case <-iterations:
	case <-time.After(2 * time.Second):
		t.Fatal("wake did not trigger a second iteration within 2s")
	}

	cancel()
	assertLoopStopsNil(t, done)
}

func TestRunLoopWithWakeUsesBaseIntervalWhenUnhealthy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var healthy atomic.Bool
	var iterations atomic.Int64
	done := make(chan error, 1)
	go func() {
		done <- RunLoopWithWake(ctx, 100*time.Millisecond, time.Minute, healthy.Load, nil, "health-test", func(context.Context, string) error {
			iterations.Add(1)
			return nil
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for iterations.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := iterations.Load(); got < 3 {
		t.Fatalf("iterations while unhealthy = %d, want at least 3 within 2s", got)
	}

	healthy.Store(true)
	time.Sleep(300 * time.Millisecond)
	before := iterations.Load()
	time.Sleep(500 * time.Millisecond)
	if additional := iterations.Load() - before; additional > 1 {
		t.Fatalf("iterations after listener became healthy = %d, want no more than 1 in 500ms", additional)
	}

	cancel()
	assertLoopStopsNil(t, done)
}

func TestRunLoopWithWakeNilWakeStillPolls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var iterations atomic.Int64
	done := make(chan error, 1)
	go func() {
		done <- RunLoopWithWake(ctx, 50*time.Millisecond, 50*time.Millisecond, func() bool { return true }, nil, "nil-wake-test", func(context.Context, string) error {
			iterations.Add(1)
			return nil
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for iterations.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := iterations.Load(); got < 3 {
		cancel()
		t.Fatalf("iterations with nil wake = %d, want at least 3 within 2s", got)
	}

	cancel()
	assertLoopStopsNil(t, done)
}

func assertLoopStopsNil(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunLoopWithWake() after cancellation error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoopWithWake() did not stop within 2s after cancellation")
	}
}
