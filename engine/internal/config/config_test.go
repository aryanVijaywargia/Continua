package config

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadPrefersEngineDatabaseURL(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("DATABASE_URL", "postgres://platform")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://engine" {
		t.Fatalf("expected ENGINE_DATABASE_URL to win, got %q", cfg.Database.URL)
	}
	if cfg.Database.MaxConns != 10 || cfg.Database.MinConns != 2 {
		t.Fatalf("unexpected pool defaults: %+v", cfg.Database)
	}
	if cfg.Runtime.WorkflowPollInterval != time.Second ||
		cfg.Runtime.ActivityPollInterval != time.Second ||
		cfg.Runtime.MaintenancePollInterval != 10*time.Second ||
		cfg.Runtime.RunLeaseTTL != 30*time.Second ||
		cfg.Runtime.ActivityLeaseTTL != 5*time.Minute ||
		cfg.Runtime.RequestDedupeTTL != time.Hour {
		t.Fatalf("unexpected runtime defaults: %+v", cfg.Runtime)
	}
}

func TestLoadFallsBackToDatabaseURL(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "postgres://platform")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://platform" {
		t.Fatalf("expected DATABASE_URL fallback, got %q", cfg.Database.URL)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing database URL error")
	}
}

func TestLoadRuntimeOverrides(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_WORKFLOW_POLL_INTERVAL", "250ms")
	t.Setenv("ENGINE_ACTIVITY_POLL_INTERVAL", "500ms")
	t.Setenv("ENGINE_MAINTENANCE_POLL_INTERVAL", "2s")
	t.Setenv("ENGINE_RUN_LEASE_TTL", "45s")
	t.Setenv("ENGINE_ACTIVITY_LEASE_TTL", "3m")
	t.Setenv("ENGINE_REQUEST_DEDUPE_TTL", "10m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Runtime.WorkflowPollInterval != 250*time.Millisecond ||
		cfg.Runtime.ActivityPollInterval != 500*time.Millisecond ||
		cfg.Runtime.MaintenancePollInterval != 2*time.Second ||
		cfg.Runtime.RunLeaseTTL != 45*time.Second ||
		cfg.Runtime.ActivityLeaseTTL != 3*time.Minute ||
		cfg.Runtime.RequestDedupeTTL != 10*time.Minute {
		t.Fatalf("unexpected runtime overrides: %+v", cfg.Runtime)
	}
}

func TestLoadPoolAndLimitDefaults(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	setPoolAndLimitEnv(t, map[string]string{})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.MaxConns != 10 {
		t.Errorf("Database.MaxConns = %d, want 10", cfg.Database.MaxConns)
	}
	if cfg.Database.MinConns != 2 {
		t.Errorf("Database.MinConns = %d, want 2", cfg.Database.MinConns)
	}
	if cfg.Database.MaxConnLifetime != time.Hour {
		t.Errorf("Database.MaxConnLifetime = %s, want 1h", cfg.Database.MaxConnLifetime)
	}
	if cfg.Database.MaxConnIdleTime != 30*time.Minute {
		t.Errorf("Database.MaxConnIdleTime = %s, want 30m", cfg.Database.MaxConnIdleTime)
	}
	if cfg.Database.HealthCheckPeriod != time.Minute {
		t.Errorf("Database.HealthCheckPeriod = %s, want 1m", cfg.Database.HealthCheckPeriod)
	}
	if cfg.Runtime.ProjectorBatchSize != 1000 {
		t.Errorf("Runtime.ProjectorBatchSize = %d, want 1000", cfg.Runtime.ProjectorBatchSize)
	}
	if cfg.Runtime.MaxChildDepth != 32 {
		t.Errorf("Runtime.MaxChildDepth = %d, want 32", cfg.Runtime.MaxChildDepth)
	}
	if cfg.Runtime.MaxContinuationFollowDepth != 32 {
		t.Errorf("Runtime.MaxContinuationFollowDepth = %d, want 32", cfg.Runtime.MaxContinuationFollowDepth)
	}
}

func TestLoadPoolAndLimitOverrides(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	setPoolAndLimitEnv(t, map[string]string{
		"ENGINE_DB_MAX_CONNS":                  "25",
		"ENGINE_DB_MIN_CONNS":                  "5",
		"ENGINE_DB_MAX_CONN_LIFETIME":          "2h",
		"ENGINE_DB_MAX_CONN_IDLE_TIME":         "45m",
		"ENGINE_DB_HEALTHCHECK_PERIOD":         "90s",
		"ENGINE_PROJECTOR_BATCH_SIZE":          "250",
		"ENGINE_MAX_CHILD_DEPTH":               "8",
		"ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH": "4",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.MaxConns != 25 {
		t.Errorf("Database.MaxConns = %d, want 25", cfg.Database.MaxConns)
	}
	if cfg.Database.MinConns != 5 {
		t.Errorf("Database.MinConns = %d, want 5", cfg.Database.MinConns)
	}
	if cfg.Database.MaxConnLifetime != 2*time.Hour {
		t.Errorf("Database.MaxConnLifetime = %s, want 2h", cfg.Database.MaxConnLifetime)
	}
	if cfg.Database.MaxConnIdleTime != 45*time.Minute {
		t.Errorf("Database.MaxConnIdleTime = %s, want 45m", cfg.Database.MaxConnIdleTime)
	}
	if cfg.Database.HealthCheckPeriod != 90*time.Second {
		t.Errorf("Database.HealthCheckPeriod = %s, want 90s", cfg.Database.HealthCheckPeriod)
	}
	if cfg.Runtime.ProjectorBatchSize != 250 {
		t.Errorf("Runtime.ProjectorBatchSize = %d, want 250", cfg.Runtime.ProjectorBatchSize)
	}
	if cfg.Runtime.MaxChildDepth != 8 {
		t.Errorf("Runtime.MaxChildDepth = %d, want 8", cfg.Runtime.MaxChildDepth)
	}
	if cfg.Runtime.MaxContinuationFollowDepth != 4 {
		t.Errorf("Runtime.MaxContinuationFollowDepth = %d, want 4", cfg.Runtime.MaxContinuationFollowDepth)
	}
}

func TestLoadRejectsInvalidPoolAndLimitValues(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		wantEnv string
	}{
		{name: "max connections is not an integer", values: map[string]string{"ENGINE_DB_MAX_CONNS": "abc"}, wantEnv: "ENGINE_DB_MAX_CONNS"},
		{name: "max connections is zero", values: map[string]string{"ENGINE_DB_MAX_CONNS": "0"}, wantEnv: "ENGINE_DB_MAX_CONNS"},
		{name: "min connections is negative", values: map[string]string{"ENGINE_DB_MIN_CONNS": "-1"}, wantEnv: "ENGINE_DB_MIN_CONNS"},
		{name: "min connections exceeds max", values: map[string]string{"ENGINE_DB_MIN_CONNS": "20", "ENGINE_DB_MAX_CONNS": "10"}, wantEnv: "ENGINE_DB_MIN_CONNS"},
		{name: "max lifetime is invalid", values: map[string]string{"ENGINE_DB_MAX_CONN_LIFETIME": "nope"}, wantEnv: "ENGINE_DB_MAX_CONN_LIFETIME"},
		{name: "max lifetime is zero", values: map[string]string{"ENGINE_DB_MAX_CONN_LIFETIME": "0s"}, wantEnv: "ENGINE_DB_MAX_CONN_LIFETIME"},
		{name: "max idle time is negative", values: map[string]string{"ENGINE_DB_MAX_CONN_IDLE_TIME": "-5m"}, wantEnv: "ENGINE_DB_MAX_CONN_IDLE_TIME"},
		{name: "healthcheck period is zero", values: map[string]string{"ENGINE_DB_HEALTHCHECK_PERIOD": "0s"}, wantEnv: "ENGINE_DB_HEALTHCHECK_PERIOD"},
		{name: "projector batch size is zero", values: map[string]string{"ENGINE_PROJECTOR_BATCH_SIZE": "0"}, wantEnv: "ENGINE_PROJECTOR_BATCH_SIZE"},
		{name: "projector batch size is not an integer", values: map[string]string{"ENGINE_PROJECTOR_BATCH_SIZE": "x"}, wantEnv: "ENGINE_PROJECTOR_BATCH_SIZE"},
		{name: "max child depth is zero", values: map[string]string{"ENGINE_MAX_CHILD_DEPTH": "0"}, wantEnv: "ENGINE_MAX_CHILD_DEPTH"},
		{name: "max continuation follow depth is negative", values: map[string]string{"ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH": "-3"}, wantEnv: "ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
			setPoolAndLimitEnv(t, tt.values)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want error containing %q", tt.wantEnv)
			}
			if !strings.Contains(err.Error(), tt.wantEnv) {
				t.Fatalf("Load() error = %q, want env var %q", err.Error(), tt.wantEnv)
			}
		})
	}
}

func setPoolAndLimitEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for _, key := range []string{
		"ENGINE_DB_MAX_CONNS",
		"ENGINE_DB_MIN_CONNS",
		"ENGINE_DB_MAX_CONN_LIFETIME",
		"ENGINE_DB_MAX_CONN_IDLE_TIME",
		"ENGINE_DB_HEALTHCHECK_PERIOD",
		"ENGINE_PROJECTOR_BATCH_SIZE",
		"ENGINE_MAX_CHILD_DEPTH",
		"ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH",
	} {
		t.Setenv(key, values[key])
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_WORKFLOW_POLL_INTERVAL", "definitely-not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestLoadLeaseCompletionGrace(t *testing.T) {
	t.Run("configured duration", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "")
		t.Setenv("DATABASE_URL", "postgres://placeholder")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "15s")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Runtime.LeaseCompletionGrace != 15*time.Second {
			t.Fatalf("LeaseCompletionGrace = %s, want 15s", cfg.Runtime.LeaseCompletionGrace)
		}
	})

	t.Run("default is zero", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "")
		t.Setenv("DATABASE_URL", "postgres://placeholder")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Runtime.LeaseCompletionGrace != 0 {
			t.Fatalf("LeaseCompletionGrace = %s, want 0", cfg.Runtime.LeaseCompletionGrace)
		}
	})

	t.Run("negative duration rejected", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "")
		t.Setenv("DATABASE_URL", "postgres://placeholder")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "-5s")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want negative completion grace error")
		}
	})

	t.Run("invalid duration rejected", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "")
		t.Setenv("DATABASE_URL", "postgres://placeholder")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "bogus")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want invalid completion grace error")
		}
	})
}

func TestLoadLoggingDefaults(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_LOG_LEVEL", "")
	t.Setenv("ENGINE_LOG_FORMAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Logging.Level != slog.LevelInfo {
		t.Errorf("Logging.Level = %v, want %v", cfg.Logging.Level, slog.LevelInfo)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format = %q, want %q", cfg.Logging.Format, "json")
	}
}

func TestLoadLogLevelOverrides(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{input: "debug", want: slog.LevelDebug},
		{input: "DEBUG", want: slog.LevelDebug},
		{input: "info", want: slog.LevelInfo},
		{input: "Warn", want: slog.LevelWarn},
		{input: "error", want: slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
			t.Setenv("ENGINE_LOG_LEVEL", tt.input)
			t.Setenv("ENGINE_LOG_FORMAT", "")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Logging.Level != tt.want {
				t.Fatalf("Logging.Level = %v, want %v", cfg.Logging.Level, tt.want)
			}
		})
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_LOG_LEVEL", "verbose")
	t.Setenv("ENGINE_LOG_FORMAT", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid log level error")
	}
}

func TestLoadLogFormatOverrides(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "json", want: "json"},
		{input: "text", want: "text"},
		{input: "TEXT", want: "text"},
		{input: "JSON", want: "json"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
			t.Setenv("ENGINE_LOG_LEVEL", "")
			t.Setenv("ENGINE_LOG_FORMAT", tt.input)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Logging.Format != tt.want {
				t.Fatalf("Logging.Format = %q, want %q", cfg.Logging.Format, tt.want)
			}
		})
	}
}

func TestLoadRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_LOG_LEVEL", "")
	t.Setenv("ENGINE_LOG_FORMAT", "xml")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid log format error")
	}
}

func TestNewLoggerJSONFormat(t *testing.T) {
	var buffer bytes.Buffer
	var output io.Writer = &buffer
	logger := NewLogger(LoggingConfig{Level: slog.LevelInfo, Format: "json"}, output)

	logger.Info("hello", "k", "v")

	if !json.Valid(buffer.Bytes()) {
		t.Fatalf("logger output is not valid JSON: %q", buffer.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &entry); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if entry["msg"] != "hello" {
		t.Errorf("msg = %v, want %q", entry["msg"], "hello")
	}
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want %q", entry["level"], "INFO")
	}
	if entry["k"] != "v" {
		t.Errorf("k = %v, want %q", entry["k"], "v")
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	var buffer bytes.Buffer
	var output io.Writer = &buffer
	logger := NewLogger(LoggingConfig{Level: slog.LevelInfo, Format: "text"}, output)

	logger.Info("hello")

	got := buffer.String()
	if !strings.Contains(got, "level=INFO") {
		t.Errorf("logger output %q does not contain %q", got, "level=INFO")
	}
	if !strings.Contains(got, "msg=hello") {
		t.Errorf("logger output %q does not contain %q", got, "msg=hello")
	}
	if json.Valid(buffer.Bytes()) {
		t.Errorf("text logger output is valid JSON: %q", got)
	}
}

func TestNewLoggerRespectsLevelFilter(t *testing.T) {
	var buffer bytes.Buffer
	var output io.Writer = &buffer
	logger := NewLogger(LoggingConfig{Level: slog.LevelWarn, Format: "json"}, output)

	logger.Info("skip")
	if buffer.Len() != 0 {
		t.Fatalf("Info output at warn level = %q, want empty", buffer.String())
	}

	logger.Warn("kept")
	if buffer.Len() == 0 {
		t.Fatal("Warn output at warn level is empty")
	}
	if !strings.Contains(buffer.String(), "kept") {
		t.Errorf("Warn output %q does not contain %q", buffer.String(), "kept")
	}
}
