package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
)

func TestWorkCreatingWritesEmitNotify(t *testing.T) {
	ts := newTestStore(t)
	listener := listenForStoreNotifications(t, ts, publicnotify.ChannelRuns, publicnotify.ChannelActivity, publicnotify.ChannelInbox)

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	instance, run := createNotifiableWork(t, ts.ctx, tx, uuidOrFatal(t), "notify-work")
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assertStoreNotificationChannels(t, listener, 5*time.Second,
		publicnotify.ChannelRuns,
		publicnotify.ChannelActivity,
		publicnotify.ChannelInbox,
	)

	claimed, err := ts.store.ClaimNextRun(ts.ctx, "notify-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if claimed.ID != run.ID {
		t.Fatalf("ClaimNextRun() ID = %s, want %s for instance %s", claimed.ID, run.ID, instance.ID)
	}
	if _, err := ts.store.TransitionRunToWaiting(ts.ctx, enginedb.TransitionRunToWaitingParams{
		ID:         run.ID,
		ClaimedBy:  claimed.ClaimedBy,
		WaitingFor: []byte(`{"kind":"signal","signal_name":"approval"}`),
	}); err != nil {
		t.Fatalf("TransitionRunToWaiting() error = %v", err)
	}

	wakeTx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx(wake) error = %v", err)
	}
	wake, err := wakeTx.WakeWaitingRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("WakeWaitingRun() error = %v", err)
	}
	if !wake.Applied {
		t.Fatal("WakeWaitingRun() applied = false, want true")
	}
	if err := wakeTx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit(wake) error = %v", err)
	}
	assertStoreNotificationChannels(t, listener, 3*time.Second, publicnotify.ChannelRuns)
}

func TestWakeWaitingRunNotAppliedEmitsNothing(t *testing.T) {
	ts := newTestStore(t)
	instance := ts.createInstance(t, uuidOrFatal(t), "notify-noop-wake")
	run := ts.createRun(t, instance, 1)
	listener := listenForStoreNotifications(t, ts, publicnotify.ChannelRuns)

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	wake, err := tx.WakeWaitingRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("WakeWaitingRun() error = %v", err)
	}
	if wake.Applied {
		t.Fatal("WakeWaitingRun() applied = true for queued run, want false")
	}
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assertStoreNotificationsQuiet(t, listener, 500*time.Millisecond)
}

func TestWithNotifyDisabledSuppressesEmission(t *testing.T) {
	ts := newTestStore(t)
	listener := listenForStoreNotifications(t, ts, publicnotify.ChannelRuns, publicnotify.ChannelActivity, publicnotify.ChannelInbox)
	disabled := ts.store.WithNotifyDisabled()

	tx, err := disabled.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	createNotifiableWork(t, ts.ctx, tx, uuidOrFatal(t), "notify-disabled")
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assertStoreNotificationsQuiet(t, listener, 500*time.Millisecond)
}

func createNotifiableWork(
	t *testing.T,
	ctx context.Context,
	tx *Tx,
	projectID uuid.UUID,
	instanceKey string,
) (enginedb.EngineInstance, enginedb.EngineRun) {
	t.Helper()

	instance, err := tx.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    instanceKey,
		DefinitionName: "notify.workflow",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := tx.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := tx.CreateActivityTask(ctx, enginedb.CreateActivityTaskParams{
		ProjectID:       projectID,
		InstanceID:      instance.ID,
		RunID:           run.ID,
		ActivityKey:     "step-1",
		ActivityType:    "notify.activity",
		AvailableAt:     time.Now(),
		ExecutionTarget: "local",
		MaxAttempts:     1,
	}); err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}
	if _, err := tx.CreateInboxItem(ctx, enginedb.CreateInboxItemParams{
		ProjectID:   projectID,
		InstanceID:  instance.ID,
		RunID:       enginetest.NullableUUID(run.ID),
		Kind:        "signal",
		Payload:     []byte(`{"signal_name":"approval"}`),
		AvailableAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateInboxItem() error = %v", err)
	}
	return instance, run
}

func listenForStoreNotifications(t *testing.T, ts *testStore, channels ...string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.ConnectConfig(ts.ctx, ts.db.Pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("connect LISTEN connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	for _, channel := range channels {
		if _, err := conn.Exec(ts.ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			t.Fatalf("LISTEN %s error = %v", channel, err)
		}
	}
	return conn
}

func assertStoreNotificationChannels(t *testing.T, conn *pgx.Conn, timeout time.Duration, channels ...string) {
	t.Helper()

	want := make(map[string]bool, len(channels))
	for _, channel := range channels {
		want[channel] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for len(want) > 0 {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			t.Fatalf("WaitForNotification() error = %v; missing channels = %v", err, want)
		}
		if notification.Payload != "" {
			t.Fatalf("notification payload on %s = %q, want empty", notification.Channel, notification.Payload)
		}
		delete(want, notification.Channel)
	}
}

func assertStoreNotificationsQuiet(t *testing.T, conn *pgx.Conn, duration time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	if notification, err := conn.WaitForNotification(ctx); err == nil {
		t.Fatalf("unexpected notification on channel %q", notification.Channel)
	} else if ctx.Err() == nil {
		t.Fatalf("WaitForNotification() error = %v, want timeout", err)
	}
}
