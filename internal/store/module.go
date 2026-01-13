package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/continua-ai/continua/internal/config"
)

// Module provides database access for the application.
var Module = fx.Module("store",
	fx.Provide(newPool),
	fx.Provide(New),
)

// newPool creates a connection pool with lifecycle management.
func newPool(lc fx.Lifecycle, cfg *config.Config) (*pgxpool.Pool, error) {
	pool, err := NewPool(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			pool.Close()
			return nil
		},
	})

	return pool, nil
}
