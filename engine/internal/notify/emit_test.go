package notify_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	enginenotify "github.com/continua-ai/continua/engine/internal/notify"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
)

func TestEmitDeliversOnCommit(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	ctx := context.Background()
	listener := newListeningConn(t, db, publicnotify.ChannelRuns)

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := enginenotify.Emit(ctx, tx, publicnotify.ChannelRuns); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	quietCtx, quietCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer quietCancel()
	if notification, err := listener.WaitForNotification(quietCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pre-commit WaitForNotification() = (%+v, %v), want deadline exceeded with no notification", notification, err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assertNotificationChannel(t, listener, publicnotify.ChannelRuns, 3*time.Second)
}

func TestEmitOutsideTxDeliversImmediately(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	ctx := context.Background()
	listener := newListeningConn(t, db, publicnotify.ChannelRuns)

	if err := enginenotify.Emit(ctx, db.Pool, publicnotify.ChannelRuns); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	assertNotificationChannel(t, listener, publicnotify.ChannelRuns, 3*time.Second)
}

func newListeningConn(t *testing.T, db *enginetest.TestDatabase, channels ...string) *pgx.Conn {
	t.Helper()

	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, db.Pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("connect LISTEN connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	for _, channel := range channels {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			t.Fatalf("LISTEN %s error = %v", channel, err)
		}
	}
	return conn
}

func assertNotificationChannel(t *testing.T, conn *pgx.Conn, want string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	notification, err := conn.WaitForNotification(ctx)
	if err != nil {
		t.Fatalf("WaitForNotification(%s) error = %v", want, err)
	}
	if notification.Channel != want {
		t.Fatalf("notification channel = %q, want %q", notification.Channel, want)
	}
	if notification.Payload != "" {
		t.Fatalf("notification payload = %q, want empty wake-only payload", notification.Payload)
	}
}
