package notify

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicnotify "github.com/continua-ai/continua/engine/pkg/notify"
)

const (
	initialReconnectBackoff = 500 * time.Millisecond
	maxReconnectBackoff     = 15 * time.Second
)

var channels = [...]string{
	publicnotify.ChannelRuns,
	publicnotify.ChannelActivity,
	publicnotify.ChannelInbox,
}

// Emit publishes a wake-only notification on channel.
func Emit(ctx context.Context, db enginedb.DBTX, channel string) error {
	_, err := db.Exec(ctx, "SELECT pg_notify($1, '')", channel)
	return err
}

// Listener fans Postgres notifications out to local subscribers.
type Listener struct {
	pool        *pgxpool.Pool
	logger      *slog.Logger
	mu          sync.Mutex
	subscribers map[string][]chan struct{}
	healthy     atomic.Bool
}

// NewListener constructs a listener backed by a dedicated Postgres connection.
func NewListener(pool *pgxpool.Pool, logger *slog.Logger) *Listener {
	return &Listener{
		pool:        pool,
		logger:      logger,
		subscribers: make(map[string][]chan struct{}),
	}
}

// Run listens until ctx is cancelled, reconnecting after connection failures.
func (l *Listener) Run(ctx context.Context) error {
	backoff := initialReconnectBackoff
	for {
		conn, err := l.connect(ctx)
		if err == nil {
			backoff = initialReconnectBackoff
			l.healthy.Store(true)
			l.logger.Info("notify listener connected", "event", "notify_listener_connected")
			l.wakeAll()
			err = l.wait(ctx, conn)
		}

		l.healthy.Store(false)
		if conn != nil {
			_ = conn.Close(context.Background())
		}
		if ctx.Err() != nil {
			return nil
		}
		l.logger.Error("notify listener error", "event", "notify_listener_error", "err", err)

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		backoff = min(backoff*2, maxReconnectBackoff)
	}
}

// Subscribe registers a buffered wake channel for a Postgres channel.
func (l *Listener) Subscribe(channel string) <-chan struct{} {
	subscriber := make(chan struct{}, 64)
	l.mu.Lock()
	l.subscribers[channel] = append(l.subscribers[channel], subscriber)
	l.mu.Unlock()
	return subscriber
}

// Healthy reports whether the listener currently has a live Postgres connection.
func (l *Listener) Healthy() bool {
	return l.healthy.Load()
}

func (l *Listener) connect(ctx context.Context) (*pgx.Conn, error) {
	config := l.pool.Config().ConnConfig.Copy()
	config.RuntimeParams["application_name"] = "continua-engine-notify"
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			_ = conn.Close(context.Background())
			return nil, err
		}
	}
	return conn, nil
}

func (l *Listener) wait(ctx context.Context, conn *pgx.Conn) error {
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		l.wake(notification.Channel)
	}
}

func (l *Listener) wake(channel string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, subscriber := range l.subscribers[channel] {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func (l *Listener) wakeAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, subscribers := range l.subscribers {
		for _, subscriber := range subscribers {
			select {
			case subscriber <- struct{}{}:
			default:
			}
		}
	}
}
