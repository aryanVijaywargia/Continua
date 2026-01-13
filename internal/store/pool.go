package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/continua-ai/continua/internal/config"
)

// PoolConfig contains configuration for the connection pool.
type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultPoolConfig returns sensible defaults for a connection pool.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:          25,
		MinConns:          5,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
	}
}

// NewPool creates a new connection pool from configuration.
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return nil, err
	}

	// Apply defaults
	defaults := DefaultPoolConfig()
	poolCfg.MaxConns = defaults.MaxConns
	poolCfg.MinConns = defaults.MinConns
	poolCfg.MaxConnLifetime = defaults.MaxConnLifetime
	poolCfg.MaxConnIdleTime = defaults.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = defaults.HealthCheckPeriod

	return pgxpool.NewWithConfig(ctx, poolCfg)
}
