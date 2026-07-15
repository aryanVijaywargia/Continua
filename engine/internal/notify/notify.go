package notify

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

// Emit is a compile-only scaffold for the notification acceptance tests.
func Emit(context.Context, enginedb.DBTX, string) error {
	return nil
}

// Listener is a compile-only scaffold for the notification acceptance tests.
type Listener struct{}

// NewListener returns a compile-only listener scaffold.
func NewListener(*pgxpool.Pool, *slog.Logger) *Listener {
	return &Listener{}
}

// Run blocks until its context is cancelled.
func (*Listener) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Subscribe returns a channel that is never signalled by this scaffold.
func (*Listener) Subscribe(string) <-chan struct{} {
	return make(chan struct{})
}

// Healthy reports false for the compile-only scaffold.
func (*Listener) Healthy() bool {
	return false
}
