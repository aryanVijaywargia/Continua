package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
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
	defaultNotifyFallback  = 5 * time.Second
	defaultWorkflowPoll    = time.Second
	defaultActivityPoll    = time.Second
	defaultMaintenancePoll = 10 * time.Second
	defaultMetricsSample   = 30 * time.Second
	defaultRunLeaseTTL     = 30 * time.Second
	defaultActivityLease   = 5 * time.Minute
	defaultShutdownGrace   = 30 * time.Second
	defaultRequestDedupe   = time.Hour
	defaultRetentionRuns   = 168 * time.Hour
	defaultRetentionDedupe = 24 * time.Hour
	defaultRetentionBatch  = int32(500)
	defaultLogLevel        = slog.LevelInfo
	defaultLogFormat       = "json"
)

const (
	defaultProjectorBatchSize         = int32(1000)
	defaultMaxChildDepth              = int32(32)
	defaultMaxContinuationFollowDepth = int32(32)
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
	NotifyEnabled              bool
	NotifyFallbackInterval     time.Duration
	WorkflowPollInterval       time.Duration
	ActivityPollInterval       time.Duration
	MaintenancePollInterval    time.Duration
	MetricsSampleInterval      time.Duration
	RunLeaseTTL                time.Duration
	ActivityLeaseTTL           time.Duration
	ShutdownGrace              time.Duration
	LeaseCompletionGrace       time.Duration
	RequestDedupeTTL           time.Duration
	RetentionTerminalRuns      time.Duration
	RetentionDedupeGrace       time.Duration
	RetentionBatchSize         int32
	ProjectorBatchSize         int32
	MaxChildDepth              int32
	MaxContinuationFollowDepth int32
	ProjectIDFilter            *uuid.UUID
	MetricsAddr                string
	HTTPAddr                   string
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
			NotifyEnabled:              true,
			NotifyFallbackInterval:     defaultNotifyFallback,
			WorkflowPollInterval:       defaultWorkflowPoll,
			ActivityPollInterval:       defaultActivityPoll,
			MaintenancePollInterval:    defaultMaintenancePoll,
			MetricsSampleInterval:      defaultMetricsSample,
			RunLeaseTTL:                defaultRunLeaseTTL,
			ActivityLeaseTTL:           defaultActivityLease,
			ShutdownGrace:              defaultShutdownGrace,
			RequestDedupeTTL:           defaultRequestDedupe,
			RetentionTerminalRuns:      defaultRetentionRuns,
			RetentionDedupeGrace:       defaultRetentionDedupe,
			RetentionBatchSize:         defaultRetentionBatch,
			ProjectorBatchSize:         defaultProjectorBatchSize,
			MaxChildDepth:              defaultMaxChildDepth,
			MaxContinuationFollowDepth: defaultMaxContinuationFollowDepth,
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

	if err := loadDatabaseConfig(&cfg.Database); err != nil {
		return nil, err
	}
	if err := loadPollingConfig(&cfg.Runtime); err != nil {
		return nil, err
	}
	if err := loadLeaseConfig(&cfg.Runtime); err != nil {
		return nil, err
	}
	if err := loadLimitConfig(&cfg.Runtime); err != nil {
		return nil, err
	}
	projectIDFilter, err := runtimeProjectIDFromEnv()
	if err != nil {
		return nil, err
	}
	cfg.Runtime.ProjectIDFilter = projectIDFilter
	cfg.Runtime.MetricsAddr = os.Getenv("ENGINE_METRICS_ADDR")
	cfg.Runtime.HTTPAddr = os.Getenv("ENGINE_HTTP_ADDR")
	if err := loadLoggingConfig(&cfg.Logging); err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadDatabaseConfig(db *DatabaseConfig) error {
	maxConns, err := int32FromEnv("ENGINE_DB_MAX_CONNS", db.MaxConns)
	if err != nil {
		return err
	}
	if maxConns < 1 {
		return errors.New("ENGINE_DB_MAX_CONNS must be at least 1")
	}
	minConns, err := int32FromEnv("ENGINE_DB_MIN_CONNS", db.MinConns)
	if err != nil {
		return err
	}
	if minConns < 0 {
		return errors.New("ENGINE_DB_MIN_CONNS must be non-negative")
	}
	if minConns > maxConns {
		return errors.New("ENGINE_DB_MIN_CONNS must not exceed ENGINE_DB_MAX_CONNS")
	}
	maxConnLifetime, err := durationFromEnv("ENGINE_DB_MAX_CONN_LIFETIME", db.MaxConnLifetime)
	if err != nil {
		return err
	}
	if maxConnLifetime <= 0 {
		return errors.New("ENGINE_DB_MAX_CONN_LIFETIME must be positive")
	}
	maxConnIdleTime, err := durationFromEnv("ENGINE_DB_MAX_CONN_IDLE_TIME", db.MaxConnIdleTime)
	if err != nil {
		return err
	}
	if maxConnIdleTime <= 0 {
		return errors.New("ENGINE_DB_MAX_CONN_IDLE_TIME must be positive")
	}
	healthCheckPeriod, err := durationFromEnv("ENGINE_DB_HEALTHCHECK_PERIOD", db.HealthCheckPeriod)
	if err != nil {
		return err
	}
	if healthCheckPeriod <= 0 {
		return errors.New("ENGINE_DB_HEALTHCHECK_PERIOD must be positive")
	}

	db.MaxConns = maxConns
	db.MinConns = minConns
	db.MaxConnLifetime = maxConnLifetime
	db.MaxConnIdleTime = maxConnIdleTime
	db.HealthCheckPeriod = healthCheckPeriod
	return nil
}

func loadPollingConfig(rt *RuntimeConfig) error {
	notifyEnabled, err := boolFromEnv("ENGINE_NOTIFY_ENABLED", rt.NotifyEnabled)
	if err != nil {
		return err
	}
	notifyFallbackInterval, err := durationFromEnv("ENGINE_NOTIFY_FALLBACK_INTERVAL", rt.NotifyFallbackInterval)
	if err != nil {
		return err
	}
	if notifyFallbackInterval <= 0 {
		return errors.New("ENGINE_NOTIFY_FALLBACK_INTERVAL must be positive")
	}
	workflowPollInterval, err := durationFromEnv("ENGINE_WORKFLOW_POLL_INTERVAL", rt.WorkflowPollInterval)
	if err != nil {
		return err
	}
	activityPollInterval, err := durationFromEnv("ENGINE_ACTIVITY_POLL_INTERVAL", rt.ActivityPollInterval)
	if err != nil {
		return err
	}
	maintenancePollInterval, err := durationFromEnv("ENGINE_MAINTENANCE_POLL_INTERVAL", rt.MaintenancePollInterval)
	if err != nil {
		return err
	}
	metricsSampleInterval, err := durationFromEnv("ENGINE_METRICS_SAMPLE_INTERVAL", rt.MetricsSampleInterval)
	if err != nil {
		return err
	}

	rt.NotifyEnabled = notifyEnabled
	rt.NotifyFallbackInterval = notifyFallbackInterval
	rt.WorkflowPollInterval = workflowPollInterval
	rt.ActivityPollInterval = activityPollInterval
	rt.MaintenancePollInterval = maintenancePollInterval
	rt.MetricsSampleInterval = metricsSampleInterval
	return nil
}

func loadLeaseConfig(rt *RuntimeConfig) error {
	runLeaseTTL, err := durationFromEnv("ENGINE_RUN_LEASE_TTL", rt.RunLeaseTTL)
	if err != nil {
		return err
	}
	activityLeaseTTL, err := durationFromEnv("ENGINE_ACTIVITY_LEASE_TTL", rt.ActivityLeaseTTL)
	if err != nil {
		return err
	}
	shutdownGrace, err := durationFromEnv("ENGINE_SHUTDOWN_GRACE", rt.ShutdownGrace)
	if err != nil {
		return err
	}
	if shutdownGrace < 0 {
		return errors.New("ENGINE_SHUTDOWN_GRACE must be non-negative")
	}
	leaseCompletionGrace, err := durationFromEnv("ENGINE_LEASE_COMPLETION_GRACE", rt.LeaseCompletionGrace)
	if err != nil {
		return err
	}
	if leaseCompletionGrace < 0 {
		return errors.New("ENGINE_LEASE_COMPLETION_GRACE must be non-negative")
	}
	requestDedupeTTL, err := durationFromEnv("ENGINE_REQUEST_DEDUPE_TTL", rt.RequestDedupeTTL)
	if err != nil {
		return err
	}

	rt.RunLeaseTTL = runLeaseTTL
	rt.ActivityLeaseTTL = activityLeaseTTL
	rt.ShutdownGrace = shutdownGrace
	rt.LeaseCompletionGrace = leaseCompletionGrace
	rt.RequestDedupeTTL = requestDedupeTTL
	return nil
}

func loadLimitConfig(rt *RuntimeConfig) error {
	retentionTerminalRuns, err := durationFromEnv("ENGINE_RETENTION_TERMINAL_RUNS", rt.RetentionTerminalRuns)
	if err != nil {
		return err
	}
	if retentionTerminalRuns < 0 {
		return errors.New("ENGINE_RETENTION_TERMINAL_RUNS must be non-negative")
	}
	retentionDedupeGrace, err := durationFromEnv("ENGINE_RETENTION_DEDUPE_GRACE", rt.RetentionDedupeGrace)
	if err != nil {
		return err
	}
	if retentionDedupeGrace < 0 {
		return errors.New("ENGINE_RETENTION_DEDUPE_GRACE must be non-negative")
	}
	retentionBatchSize, err := int32FromEnv("ENGINE_RETENTION_BATCH_SIZE", rt.RetentionBatchSize)
	if err != nil {
		return err
	}
	if retentionBatchSize < 1 {
		return errors.New("ENGINE_RETENTION_BATCH_SIZE must be at least 1")
	}
	projectorBatchSize, err := int32FromEnv("ENGINE_PROJECTOR_BATCH_SIZE", rt.ProjectorBatchSize)
	if err != nil {
		return err
	}
	if projectorBatchSize < 1 {
		return errors.New("ENGINE_PROJECTOR_BATCH_SIZE must be at least 1")
	}
	maxChildDepth, err := int32FromEnv("ENGINE_MAX_CHILD_DEPTH", rt.MaxChildDepth)
	if err != nil {
		return err
	}
	if maxChildDepth < 1 {
		return errors.New("ENGINE_MAX_CHILD_DEPTH must be at least 1")
	}
	maxContinuationFollowDepth, err := int32FromEnv("ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH", rt.MaxContinuationFollowDepth)
	if err != nil {
		return err
	}
	if maxContinuationFollowDepth < 1 {
		return errors.New("ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH must be at least 1")
	}

	rt.RetentionTerminalRuns = retentionTerminalRuns
	rt.RetentionDedupeGrace = retentionDedupeGrace
	rt.RetentionBatchSize = retentionBatchSize
	rt.ProjectorBatchSize = projectorBatchSize
	rt.MaxChildDepth = maxChildDepth
	rt.MaxContinuationFollowDepth = maxContinuationFollowDepth
	return nil
}

func loadLoggingConfig(lg *LoggingConfig) error {
	logLevel, err := logLevelFromEnv("ENGINE_LOG_LEVEL", lg.Level)
	if err != nil {
		return err
	}
	logFormat, err := logFormatFromEnv("ENGINE_LOG_FORMAT", lg.Format)
	if err != nil {
		return err
	}

	lg.Level = logLevel
	lg.Format = logFormat
	return nil
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
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	return parsed, nil
}

func int32FromEnv(key string, fallback int32) (int32, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}
	return int32(parsed), nil
}

func boolFromEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a valid boolean: %w", key, err)
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
		return nil, fmt.Errorf("%s must be a valid UUID: %w", key, err)
	}
	return &parsed, nil
}
