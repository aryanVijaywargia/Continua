package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration.
// For Phase 2, configuration is loaded from environment variables only.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Ingest   IngestConfig
	Engine   EngineConfig
	Jobs     JobsConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host string
	Port string
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	URL string
}

// IngestConfig holds async ingest configuration.
type IngestConfig struct {
	TrueAsyncDefault       bool
	DependencyRetryWindow  time.Duration
	FailedPayloadRetention time.Duration
}

// JobsConfig holds River queue worker configuration.
type JobsConfig struct {
	IngestWorkers      int
	RollupWorkers      int
	MaintenanceWorkers int
	DefaultWorkers     int
}

// EngineConfig holds public engine API rollout settings.
type EngineConfig struct {
	PublicAPIEnabled         bool
	ProjectionRetentionAfter time.Duration
	HistoryRetentionAfter    time.Duration
}

// Address returns the server address in host:port format.
func (s ServerConfig) Address() string {
	return s.Host + ":" + s.Port
}

// Load loads configuration from environment variables.
// Required: DATABASE_URL
// Optional: HOST (default: 0.0.0.0), PORT (default: 8080)
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL environment variable is required")
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	trueAsyncDefault, err := loadBool("INGEST_TRUE_ASYNC_DEFAULT", false)
	if err != nil {
		return nil, err
	}
	enginePublicAPIEnabled, err := loadBool("ENGINE_PUBLIC_API_ENABLED", false)
	if err != nil {
		return nil, err
	}
	engineProjectionRetentionAfter, err := loadOptionalDuration("ENGINE_PROJECTION_RETENTION_AFTER")
	if err != nil {
		return nil, err
	}
	engineHistoryRetentionAfter, err := loadOptionalDuration("ENGINE_HISTORY_RETENTION_AFTER")
	if err != nil {
		return nil, err
	}
	if engineHistoryRetentionAfter > 0 && engineProjectionRetentionAfter <= 0 {
		return nil, errors.New("ENGINE_HISTORY_RETENTION_AFTER requires ENGINE_PROJECTION_RETENTION_AFTER to be set and greater than zero")
	}
	if engineHistoryRetentionAfter > 0 && engineHistoryRetentionAfter <= engineProjectionRetentionAfter {
		return nil, errors.New("ENGINE_HISTORY_RETENTION_AFTER must be greater than ENGINE_PROJECTION_RETENTION_AFTER")
	}
	dependencyRetryWindow, err := loadDuration("INGEST_DEPENDENCY_RETRY_WINDOW", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	failedPayloadRetention, err := loadDuration("INGEST_FAILED_PAYLOAD_RETENTION", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	ingestWorkers, err := loadInt("RIVER_QUEUE_INGEST_WORKERS", 4)
	if err != nil {
		return nil, err
	}
	rollupWorkers, err := loadInt("RIVER_QUEUE_ROLLUP_WORKERS", 10)
	if err != nil {
		return nil, err
	}
	maintenanceWorkers, err := loadInt("RIVER_QUEUE_MAINTENANCE_WORKERS", 1)
	if err != nil {
		return nil, err
	}
	defaultWorkers, err := loadInt("RIVER_QUEUE_DEFAULT_WORKERS", 1)
	if err != nil {
		return nil, err
	}

	return &Config{
		Server: ServerConfig{
			Host: host,
			Port: port,
		},
		Database: DatabaseConfig{
			URL: dbURL,
		},
		Ingest: IngestConfig{
			TrueAsyncDefault:       trueAsyncDefault,
			DependencyRetryWindow:  dependencyRetryWindow,
			FailedPayloadRetention: failedPayloadRetention,
		},
		Engine: EngineConfig{
			PublicAPIEnabled:         enginePublicAPIEnabled,
			ProjectionRetentionAfter: engineProjectionRetentionAfter,
			HistoryRetentionAfter:    engineHistoryRetentionAfter,
		},
		Jobs: JobsConfig{
			IngestWorkers:      ingestWorkers,
			RollupWorkers:      rollupWorkers,
			MaintenanceWorkers: maintenanceWorkers,
			DefaultWorkers:     defaultWorkers,
		},
	}, nil
}

func loadBool(key string, defaultValue bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a valid boolean", key)
	}
	return value, nil
}

func loadInt(key string, defaultValue int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer", key)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be non-negative", key)
	}
	return value, nil
}

func loadDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New(key + " must be a valid duration")
	}
	if value < 0 {
		return 0, errors.New(key + " must be non-negative")
	}
	return value, nil
}

func loadOptionalDuration(key string) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New(key + " must be a valid duration")
	}
	if value < 0 {
		return 0, errors.New(key + " must be non-negative")
	}
	if value == 0 {
		return 0, nil
	}
	return value, nil
}
