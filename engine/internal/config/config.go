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
	defaultWorkflowPoll    = time.Second
	defaultActivityPoll    = time.Second
	defaultMaintenancePoll = 10 * time.Second
	defaultRunLeaseTTL     = 30 * time.Second
	defaultActivityLease   = 5 * time.Minute
	defaultRequestDedupe   = time.Hour
)

// Config holds engine runtime configuration.
type Config struct {
	Database DatabaseConfig
	Runtime  RuntimeConfig
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

// RuntimeConfig holds polling, lease, and dedupe settings for the engine runtime.
type RuntimeConfig struct {
	WorkflowPollInterval    time.Duration
	ActivityPollInterval    time.Duration
	MaintenancePollInterval time.Duration
	RunLeaseTTL             time.Duration
	ActivityLeaseTTL        time.Duration
	RequestDedupeTTL        time.Duration
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

	workflowPollInterval, err := durationFromEnv("ENGINE_WORKFLOW_POLL_INTERVAL", defaultWorkflowPoll)
	if err != nil {
		return nil, err
	}
	activityPollInterval, err := durationFromEnv("ENGINE_ACTIVITY_POLL_INTERVAL", defaultActivityPoll)
	if err != nil {
		return nil, err
	}
	maintenancePollInterval, err := durationFromEnv("ENGINE_MAINTENANCE_POLL_INTERVAL", defaultMaintenancePoll)
	if err != nil {
		return nil, err
	}
	runLeaseTTL, err := durationFromEnv("ENGINE_RUN_LEASE_TTL", defaultRunLeaseTTL)
	if err != nil {
		return nil, err
	}
	activityLeaseTTL, err := durationFromEnv("ENGINE_ACTIVITY_LEASE_TTL", defaultActivityLease)
	if err != nil {
		return nil, err
	}
	requestDedupeTTL, err := durationFromEnv("ENGINE_REQUEST_DEDUPE_TTL", defaultRequestDedupe)
	if err != nil {
		return nil, err
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
		Runtime: RuntimeConfig{
			WorkflowPollInterval:    workflowPollInterval,
			ActivityPollInterval:    activityPollInterval,
			MaintenancePollInterval: maintenancePollInterval,
			RunLeaseTTL:             runLeaseTTL,
			ActivityLeaseTTL:        activityLeaseTTL,
			RequestDedupeTTL:        requestDedupeTTL,
		},
	}, nil
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.New(key + " must be a valid duration: " + err.Error())
	}
	return parsed, nil
}
