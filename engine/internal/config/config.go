package config

import (
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
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
	LeaseCompletionGrace    time.Duration
	RequestDedupeTTL        time.Duration
	ProjectIDFilter         *uuid.UUID
}

// Defaults returns the engine runtime defaults for a database URL.
func Defaults(databaseURL string) *Config {
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
			WorkflowPollInterval:    defaultWorkflowPoll,
			ActivityPollInterval:    defaultActivityPoll,
			MaintenancePollInterval: defaultMaintenancePoll,
			RunLeaseTTL:             defaultRunLeaseTTL,
			ActivityLeaseTTL:        defaultActivityLease,
			RequestDedupeTTL:        defaultRequestDedupe,
		},
	}
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

	cfg := Defaults(databaseURL)

	workflowPollInterval, err := durationFromEnv("ENGINE_WORKFLOW_POLL_INTERVAL", cfg.Runtime.WorkflowPollInterval)
	if err != nil {
		return nil, err
	}
	activityPollInterval, err := durationFromEnv("ENGINE_ACTIVITY_POLL_INTERVAL", cfg.Runtime.ActivityPollInterval)
	if err != nil {
		return nil, err
	}
	maintenancePollInterval, err := durationFromEnv("ENGINE_MAINTENANCE_POLL_INTERVAL", cfg.Runtime.MaintenancePollInterval)
	if err != nil {
		return nil, err
	}
	runLeaseTTL, err := durationFromEnv("ENGINE_RUN_LEASE_TTL", cfg.Runtime.RunLeaseTTL)
	if err != nil {
		return nil, err
	}
	activityLeaseTTL, err := durationFromEnv("ENGINE_ACTIVITY_LEASE_TTL", cfg.Runtime.ActivityLeaseTTL)
	if err != nil {
		return nil, err
	}
	requestDedupeTTL, err := durationFromEnv("ENGINE_REQUEST_DEDUPE_TTL", cfg.Runtime.RequestDedupeTTL)
	if err != nil {
		return nil, err
	}
	projectIDFilter, err := runtimeProjectIDFromEnv()
	if err != nil {
		return nil, err
	}

	cfg.Runtime.WorkflowPollInterval = workflowPollInterval
	cfg.Runtime.ActivityPollInterval = activityPollInterval
	cfg.Runtime.MaintenancePollInterval = maintenancePollInterval
	cfg.Runtime.RunLeaseTTL = runLeaseTTL
	cfg.Runtime.ActivityLeaseTTL = activityLeaseTTL
	cfg.Runtime.RequestDedupeTTL = requestDedupeTTL
	cfg.Runtime.ProjectIDFilter = projectIDFilter
	return cfg, nil
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

func runtimeProjectIDFromEnv() (*uuid.UUID, error) {
	if value := os.Getenv("ENGINE_PROJECT_ID"); value != "" {
		return parseUUIDEnv("ENGINE_PROJECT_ID", value)
	}
	if value := os.Getenv("CONTINUA_ENGINE_TEST_PROJECT_FILTER"); value != "" {
		return parseUUIDEnv("CONTINUA_ENGINE_TEST_PROJECT_FILTER", value)
	}
	return nil, nil
}

func parseUUIDEnv(key, value string) (*uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, errors.New(key + " must be a valid UUID: " + err.Error())
	}
	return &parsed, nil
}
