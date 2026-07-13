package config

import (
	"errors"
	"fmt"
	"net/url"
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
	Auth0      Auth0Config
	PublicDemo PublicDemoConfig
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
	LeaseCompletionGrace     time.Duration
}

// Auth0Config holds hosted operator authentication settings.
type Auth0Config struct {
	Enabled       bool
	Domain        string
	ClientID      string
	Audience      string
	AllowedEmails []string
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
	engineLeaseCompletionGrace, err := loadDuration("ENGINE_LEASE_COMPLETION_GRACE", 0)
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
	publicDemoConfig, err := loadPublicDemoConfig()
	if err != nil {
		return nil, err
	}
	auth0Config := Auth0Config{}
	if !publicDemoConfig.Enabled {
		auth0Config, err = loadAuth0Config()
		if err != nil {
			return nil, err
		}
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
			LeaseCompletionGrace:     engineLeaseCompletionGrace,
		},
		Jobs: JobsConfig{
			IngestWorkers:      ingestWorkers,
			RollupWorkers:      rollupWorkers,
			MaintenanceWorkers: maintenanceWorkers,
			DefaultWorkers:     defaultWorkers,
		},
		Auth0:      auth0Config,
		PublicDemo: publicDemoConfig,
	}, nil
}

func loadPublicDemoConfig() (PublicDemoConfig, error) {
	enabled, err := loadBool("PUBLIC_DEMO_ENABLED", false)
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

func loadAuth0Config() (Auth0Config, error) {
	rawDomain := strings.TrimSpace(os.Getenv("AUTH0_DOMAIN"))
	rawClientID := strings.TrimSpace(os.Getenv("AUTH0_CLIENT_ID"))
	rawAudience := strings.TrimSpace(os.Getenv("AUTH0_AUDIENCE"))
	rawAllowedEmails := strings.TrimSpace(os.Getenv("AUTH0_ALLOWED_EMAILS"))

	anySet := rawDomain != "" || rawClientID != "" || rawAudience != "" || rawAllowedEmails != ""
	if !anySet {
		return Auth0Config{}, nil
	}

	missing := make([]string, 0, 4)
	if rawDomain == "" {
		missing = append(missing, "AUTH0_DOMAIN")
	}
	if rawClientID == "" {
		missing = append(missing, "AUTH0_CLIENT_ID")
	}
	if rawAudience == "" {
		missing = append(missing, "AUTH0_AUDIENCE")
	}
	if rawAllowedEmails == "" {
		missing = append(missing, "AUTH0_ALLOWED_EMAILS")
	}
	if len(missing) > 0 {
		return Auth0Config{}, fmt.Errorf("partial Auth0 configuration: missing %s", strings.Join(missing, ", "))
	}

	domain, err := normalizeAuth0Domain(rawDomain)
	if err != nil {
		return Auth0Config{}, err
	}
	allowedEmails, err := parseAllowedEmails(rawAllowedEmails)
	if err != nil {
		return Auth0Config{}, err
	}

	return Auth0Config{
		Enabled:       true,
		Domain:        domain,
		ClientID:      rawClientID,
		Audience:      rawAudience,
		AllowedEmails: allowedEmails,
	}, nil
}

func normalizeAuth0Domain(raw string) (string, error) {
	candidate := raw
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("AUTH0_DOMAIN must be a valid domain: %w", err)
	}
	if parsed.Host == "" {
		return "", errors.New("AUTH0_DOMAIN must include a host")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("AUTH0_DOMAIN must not include a path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("AUTH0_DOMAIN must not include query or fragment components")
	}

	return parsed.Host, nil
}

func parseAllowedEmails(raw string) ([]string, error) {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	if len(parts) == 0 {
		return nil, errors.New("AUTH0_ALLOWED_EMAILS must include at least one email address")
	}

	seen := make(map[string]struct{}, len(parts))
	emails := make([]string, 0, len(parts))
	for _, part := range parts {
		email := strings.ToLower(strings.TrimSpace(part))
		if email == "" {
			continue
		}
		if !strings.Contains(email, "@") {
			return nil, fmt.Errorf("AUTH0_ALLOWED_EMAILS contains an invalid email address: %s", part)
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}

	if len(emails) == 0 {
		return nil, errors.New("AUTH0_ALLOWED_EMAILS must include at least one email address")
	}

	return emails, nil
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
