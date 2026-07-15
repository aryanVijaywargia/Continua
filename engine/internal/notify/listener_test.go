package notify_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	enginenotify "github.com/continua-ai/continua/engine/internal/notify"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
)

func TestListenerFansOutPerChannel(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	listener := enginenotify.NewListener(db.Pool, discardLogger())
	runsWake := listener.Subscribe(publicnotify.ChannelRuns)
	activityWake := listener.Subscribe(publicnotify.ChannelActivity)
	done := runListener(t, listener)

	waitForListenerHealthy(t, listener, done, 5*time.Second)
	drainWakes(runsWake, 300*time.Millisecond)
	drainWakes(activityWake, 300*time.Millisecond)

	if _, err := db.Pool.Exec(context.Background(), "SELECT pg_notify($1, '')", publicnotify.ChannelRuns); err != nil {
		t.Fatalf("pg_notify(%s) error = %v", publicnotify.ChannelRuns, err)
	}
	assertWake(t, runsWake, 3*time.Second)
	select {
	case <-activityWake:
		t.Fatalf("%s subscriber woke for %s notification", publicnotify.ChannelActivity, publicnotify.ChannelRuns)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestListenerReconnectsAfterBackendKill(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	listener := enginenotify.NewListener(db.Pool, discardLogger())
	runsWake := listener.Subscribe(publicnotify.ChannelRuns)
	done := runListener(t, listener)

	waitForListenerHealthy(t, listener, done, 5*time.Second)
	drainWakes(runsWake, 300*time.Millisecond)

	var killed bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COALESCE(bool_or(pg_terminate_backend(pid)), false)
		FROM pg_stat_activity
		WHERE application_name = 'continua-engine-notify'
		  AND pid <> pg_backend_pid()
	`).Scan(&killed); err != nil {
		t.Fatalf("terminate listener backend error = %v", err)
	}
	if !killed {
		t.Fatal("listener backend was not found for termination")
	}

	assertWake(t, runsWake, 10*time.Second)
	waitForListenerHealthy(t, listener, done, 10*time.Second)

	if _, err := db.Pool.Exec(context.Background(), "SELECT pg_notify($1, '')", publicnotify.ChannelRuns); err != nil {
		t.Fatalf("pg_notify(%s) after reconnect error = %v", publicnotify.ChannelRuns, err)
	}
	assertWake(t, runsWake, 3*time.Second)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func runListener(t *testing.T, listener *enginenotify.Listener) <-chan error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Listener.Run() cleanup error = %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Listener.Run() did not stop after cancellation")
		}
	})
	return done
}

func waitForListenerHealthy(t *testing.T, listener *enginenotify.Listener, done <-chan error, timeout time.Duration) {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if listener.Healthy() {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Listener.Run() exited before healthy: %v", err)
		case <-deadline.C:
			t.Fatalf("Listener.Healthy() remained false for %s", timeout)
		case <-ticker.C:
		}
	}
}

func drainWakes(wake <-chan struct{}, quiet time.Duration) {
	for {
		select {
		case <-wake:
		case <-time.After(quiet):
			return
		}
	}
}

func assertWake(t *testing.T, wake <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-wake:
	case <-time.After(timeout):
		t.Fatalf("subscriber did not wake within %s", timeout)
	}
}
