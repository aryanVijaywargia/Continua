package config

import (
	"errors"
	"os"
	"time"
)

const (
	defaultMaxConns        = int32(10)
	defaultMinConns        = int32(2)
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
	defaultHealthCheck     = time.Minute
)

// Config holds engine runtime configuration.
type Config struct {
	Database DatabaseConfig
}

// DatabaseConfig holds the engine database settings.
type DatabaseConfig struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// Load resolves the engine database configuration from environment variables.
func Load() (*Config, error) {
	databaseURL := os.Getenv("ENGINE_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return nil, errors.New("ENGINE_DATABASE_URL or DATABASE_URL environment variable is required")
	}

	return &Config{
		Database: DatabaseConfig{
			URL:               databaseURL,
			MaxConns:          defaultMaxConns,
			MinConns:          defaultMinConns,
			MaxConnLifetime:   defaultMaxConnLifetime,
			MaxConnIdleTime:   defaultMaxConnIdleTime,
			HealthCheckPeriod: defaultHealthCheck,
		},
	}, nil
}
