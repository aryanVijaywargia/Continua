package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config holds the application configuration.
// For Phase 2, configuration is loaded from environment variables only.
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Ingest     IngestConfig
	Engine     EngineConfig
	Jobs       JobsConfig
	PublicDemo PublicDemoConfig

	// LocalSingleUserMode opts the deployment into API-key-free, loopback-only
	// debugger access. Sourced from LOCAL_SINGLE_USER_MODE; defaults to false.
	LocalSingleUserMode bool
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

	// OTLPEnabled gates the preview OTLP/HTTP trace ingestion endpoint. Sourced from
	// INGEST_OTLP_ENABLED; defaults to false, which makes POST /v1/traces 404.
	OTLPEnabled bool
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
	LeaseCompletionGrace     time.Duration
}

// PublicDemoConfig controls the hosted public portfolio demo mode.
type PublicDemoConfig struct {
	Enabled   bool
	ProjectID uuid.UUID
	Label     string
}

// Address returns the server address in host:port format.
func (s ServerConfig) Address() string {
	return s.Host + ":" + s.Port
}

// Load loads configuration from environment variables.
// Required: DATABASE_URL
// Optional: HOST (default: 127.0.0.1), PORT (default: 8080)
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL environment variable is required")
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	trueAsyncDefault, err := loadBool("INGEST_TRUE_ASYNC_DEFAULT")
	if err != nil {
		return nil, err
	}
	otlpEnabled, err := loadBool("INGEST_OTLP_ENABLED")
	if err != nil {
		return nil, err
	}
	engineConfig, err := loadEngineConfig()
	if err != nil {
		return nil, err
	}
	dependencyRetryWindow, err := loadDuration("INGEST_DEPENDENCY_RETRY_WINDOW", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	failedPayloadRetention, err := loadDuration("INGEST_FAILED_PAYLOAD_RETENTION", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}
	jobsConfig, err := loadJobsConfig()
	if err != nil {
		return nil, err
	}
	publicDemoConfig, err := loadPublicDemoConfig()
	if err != nil {
		return nil, err
	}
	localSingleUserMode, err := loadBool("LOCAL_SINGLE_USER_MODE")
	if err != nil {
		return nil, err
	}
	// Fail closed: an unauthenticated loopback bypass must never coexist with a
	// multi-identity auth posture. Refuse to boot rather than silently picking one.
	if localSingleUserMode && publicDemoConfig.Enabled {
		return nil, errors.New("LOCAL_SINGLE_USER_MODE cannot be enabled together with PUBLIC_DEMO_ENABLED")
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
			OTLPEnabled:            otlpEnabled,
		},
		Engine:              engineConfig,
		Jobs:                jobsConfig,
		PublicDemo:          publicDemoConfig,
		LocalSingleUserMode: localSingleUserMode,
	}, nil
}

// loadEngineConfig reads the engine rollout settings. Retention windows are
// validated as a pair because a history window without a projection window
// would purge history the projector still needs.
func loadEngineConfig() (EngineConfig, error) {
	publicAPIEnabled, err := loadBool("ENGINE_PUBLIC_API_ENABLED")
	if err != nil {
		return EngineConfig{}, err
	}
	projectionRetentionAfter, err := loadOptionalDuration("ENGINE_PROJECTION_RETENTION_AFTER")
	if err != nil {
		return EngineConfig{}, err
	}
	historyRetentionAfter, err := loadOptionalDuration("ENGINE_HISTORY_RETENTION_AFTER")
	if err != nil {
		return EngineConfig{}, err
	}
	leaseCompletionGrace, err := loadDuration("ENGINE_LEASE_COMPLETION_GRACE", 0)
	if err != nil {
		return EngineConfig{}, err
	}
	if historyRetentionAfter > 0 && projectionRetentionAfter <= 0 {
		return EngineConfig{}, errors.New("ENGINE_HISTORY_RETENTION_AFTER requires ENGINE_PROJECTION_RETENTION_AFTER to be set and greater than zero")
	}
	if historyRetentionAfter > 0 && historyRetentionAfter <= projectionRetentionAfter {
		return EngineConfig{}, errors.New("ENGINE_HISTORY_RETENTION_AFTER must be greater than ENGINE_PROJECTION_RETENTION_AFTER")
	}

	return EngineConfig{
		PublicAPIEnabled:         publicAPIEnabled,
		ProjectionRetentionAfter: projectionRetentionAfter,
		HistoryRetentionAfter:    historyRetentionAfter,
		LeaseCompletionGrace:     leaseCompletionGrace,
	}, nil
}

func loadJobsConfig() (JobsConfig, error) {
	ingestWorkers, err := loadInt("RIVER_QUEUE_INGEST_WORKERS", 4)
	if err != nil {
		return JobsConfig{}, err
	}
	rollupWorkers, err := loadInt("RIVER_QUEUE_ROLLUP_WORKERS", 10)
	if err != nil {
		return JobsConfig{}, err
	}
	maintenanceWorkers, err := loadInt("RIVER_QUEUE_MAINTENANCE_WORKERS", 1)
	if err != nil {
		return JobsConfig{}, err
	}
	defaultWorkers, err := loadInt("RIVER_QUEUE_DEFAULT_WORKERS", 1)
	if err != nil {
		return JobsConfig{}, err
	}

	return JobsConfig{
		IngestWorkers:      ingestWorkers,
		RollupWorkers:      rollupWorkers,
		MaintenanceWorkers: maintenanceWorkers,
		DefaultWorkers:     defaultWorkers,
	}, nil
}

func loadPublicDemoConfig() (PublicDemoConfig, error) {
	enabled, err := loadBool("PUBLIC_DEMO_ENABLED")
	if err != nil {
		return PublicDemoConfig{}, err
	}

	if !enabled {
		return PublicDemoConfig{}, nil
	}

	rawProjectID := strings.TrimSpace(os.Getenv("PUBLIC_DEMO_PROJECT_ID"))
	if rawProjectID == "" {
		return PublicDemoConfig{}, errors.New("PUBLIC_DEMO_PROJECT_ID is required when PUBLIC_DEMO_ENABLED is true")
	}

	projectID, err := uuid.Parse(rawProjectID)
	if err != nil {
		return PublicDemoConfig{}, fmt.Errorf("PUBLIC_DEMO_PROJECT_ID must be a valid UUID: %w", err)
	}

	label := strings.TrimSpace(os.Getenv("PUBLIC_DEMO_LABEL"))
	if label == "" {
		label = "Sample data"
	}

	return PublicDemoConfig{
		Enabled:   true,
		ProjectID: projectID,
		Label:     label,
	}, nil
}

// loadBool reads an opt-in boolean environment variable. Every boolean setting
// defaults to off, so an unset variable is false.
func loadBool(key string) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return false, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a valid boolean: %w", key, err)
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
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
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
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
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
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	if value < 0 {
		return 0, errors.New(key + " must be non-negative")
	}
	if value == 0 {
		return 0, nil
	}
	return value, nil
}
