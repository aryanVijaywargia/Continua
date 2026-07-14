package config

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
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
	defaultMetricsSample   = 30 * time.Second
	defaultRunLeaseTTL     = 30 * time.Second
	defaultActivityLease   = 5 * time.Minute
	defaultRequestDedupe   = time.Hour
	defaultRetentionRuns   = 168 * time.Hour
	defaultRetentionDedupe = 24 * time.Hour
	defaultRetentionBatch  = int32(500)
	defaultLogLevel        = slog.LevelInfo
	defaultLogFormat       = "json"
)

// Config holds engine runtime configuration.
type Config struct {
	Database DatabaseConfig
	Runtime  RuntimeConfig
	Logging  LoggingConfig
}

// LoggingConfig holds the engine structured logging settings.
type LoggingConfig struct {
	Level  slog.Level
	Format string
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
	MetricsSampleInterval   time.Duration
	RunLeaseTTL             time.Duration
	ActivityLeaseTTL        time.Duration
	LeaseCompletionGrace    time.Duration
	RequestDedupeTTL        time.Duration
	RetentionTerminalRuns   time.Duration
	RetentionDedupeGrace    time.Duration
	RetentionBatchSize      int32
	ProjectIDFilter         *uuid.UUID
	MetricsAddr             string
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
			MetricsSampleInterval:   defaultMetricsSample,
			RunLeaseTTL:             defaultRunLeaseTTL,
			ActivityLeaseTTL:        defaultActivityLease,
			RequestDedupeTTL:        defaultRequestDedupe,
			RetentionTerminalRuns:   defaultRetentionRuns,
			RetentionDedupeGrace:    defaultRetentionDedupe,
			RetentionBatchSize:      defaultRetentionBatch,
		},
		Logging: LoggingConfig{
			Level:  defaultLogLevel,
			Format: defaultLogFormat,
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
	metricsSampleInterval, err := durationFromEnv("ENGINE_METRICS_SAMPLE_INTERVAL", cfg.Runtime.MetricsSampleInterval)
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
	leaseCompletionGrace, err := durationFromEnv("ENGINE_LEASE_COMPLETION_GRACE", cfg.Runtime.LeaseCompletionGrace)
	if err != nil {
		return nil, err
	}
	if leaseCompletionGrace < 0 {
		return nil, errors.New("ENGINE_LEASE_COMPLETION_GRACE must be non-negative")
	}
	requestDedupeTTL, err := durationFromEnv("ENGINE_REQUEST_DEDUPE_TTL", cfg.Runtime.RequestDedupeTTL)
	if err != nil {
		return nil, err
	}
	projectIDFilter, err := runtimeProjectIDFromEnv()
	if err != nil {
		return nil, err
	}
	logLevel, err := logLevelFromEnv("ENGINE_LOG_LEVEL", cfg.Logging.Level)
	if err != nil {
		return nil, err
	}
	logFormat, err := logFormatFromEnv("ENGINE_LOG_FORMAT", cfg.Logging.Format)
	if err != nil {
		return nil, err
	}

	cfg.Runtime.WorkflowPollInterval = workflowPollInterval
	cfg.Runtime.ActivityPollInterval = activityPollInterval
	cfg.Runtime.MaintenancePollInterval = maintenancePollInterval
	cfg.Runtime.MetricsSampleInterval = metricsSampleInterval
	cfg.Runtime.RunLeaseTTL = runLeaseTTL
	cfg.Runtime.ActivityLeaseTTL = activityLeaseTTL
	cfg.Runtime.LeaseCompletionGrace = leaseCompletionGrace
	cfg.Runtime.RequestDedupeTTL = requestDedupeTTL
	cfg.Runtime.ProjectIDFilter = projectIDFilter
	cfg.Runtime.MetricsAddr = os.Getenv("ENGINE_METRICS_ADDR")
	cfg.Logging.Level = logLevel
	cfg.Logging.Format = logFormat
	return cfg, nil
}

// NewLogger constructs an engine structured logger.
func NewLogger(cfg LoggingConfig, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}

	options := &slog.HandlerOptions{Level: cfg.Level}
	if cfg.Format == "text" {
		return slog.New(slog.NewTextHandler(w, options))
	}
	return slog.New(slog.NewJSONHandler(w, options))
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

func logLevelFromEnv(key string, fallback slog.Level) (slog.Level, error) {
	value := strings.ToLower(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New(key + " must be one of debug, info, warn, error")
	}
}

func logFormatFromEnv(key, fallback string) (string, error) {
	value := strings.ToLower(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	switch value {
	case "json", "text":
		return value, nil
	default:
		return "", errors.New(key + " must be one of json, text")
	}
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
