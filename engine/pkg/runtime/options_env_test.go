package runtime

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// engineEnvKeys lists every ENGINE_* setting that config.Load reads.
// ENGINE_REQUEST_DEDUPE_TTL is left out: only the CLI start command reads it.
var engineEnvKeys = []string{
	"ENGINE_DATABASE_URL",
	"DATABASE_URL",
	"ENGINE_DB_MAX_CONNS",
	"ENGINE_DB_MIN_CONNS",
	"ENGINE_DB_MAX_CONN_LIFETIME",
	"ENGINE_DB_MAX_CONN_IDLE_TIME",
	"ENGINE_DB_HEALTHCHECK_PERIOD",
	"ENGINE_NOTIFY_ENABLED",
	"ENGINE_NOTIFY_FALLBACK_INTERVAL",
	"ENGINE_WORKFLOW_POLL_INTERVAL",
	"ENGINE_ACTIVITY_POLL_INTERVAL",
	"ENGINE_MAINTENANCE_POLL_INTERVAL",
	"ENGINE_METRICS_SAMPLE_INTERVAL",
	"ENGINE_RUN_LEASE_TTL",
	"ENGINE_ACTIVITY_LEASE_TTL",
	"ENGINE_SHUTDOWN_GRACE",
	"ENGINE_LEASE_COMPLETION_GRACE",
	"ENGINE_RETENTION_TERMINAL_RUNS",
	"ENGINE_RETENTION_DEDUPE_GRACE",
	"ENGINE_RETENTION_BATCH_SIZE",
	"ENGINE_PROJECTOR_BATCH_SIZE",
	"ENGINE_MAX_CHILD_DEPTH",
	"ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH",
	"ENGINE_PROJECT_ID",
	"CONTINUA_ENGINE_TEST_PROJECT_FILTER",
	"ENGINE_METRICS_ADDR",
	"ENGINE_HTTP_ADDR",
	"ENGINE_LOG_LEVEL",
	"ENGINE_LOG_FORMAT",
}

func TestOptionsFromEnvMapsEverySetting(t *testing.T) {
	projectID := uuid.MustParse("6f1d3b8e-2c4a-4e8f-9b7d-1a2b3c4d5e6f")
	tests := []struct {
		name string
		env  map[string]string
		want Options
	}{
		{
			name: "defaults",
			env:  map[string]string{"DATABASE_URL": "postgres://defaults/db"},
			want: Options{
				DatabaseURL:                "postgres://defaults/db",
				DBMaxConns:                 10,
				DBMinConns:                 2,
				DBMinConnsSet:              true,
				DBMaxConnLifetime:          time.Hour,
				DBMaxConnIdleTime:          30 * time.Minute,
				DBHealthCheckPeriod:        time.Minute,
				ProjectorBatchSize:         1000,
				MaxChildDepth:              32,
				MaxContinuationFollowDepth: 32,
				WorkflowPollInterval:       time.Second,
				ActivityPollInterval:       time.Second,
				MaintenancePollInterval:    10 * time.Second,
				MetricsSampleInterval:      30 * time.Second,
				RunLeaseTTL:                30 * time.Second,
				ActivityLeaseTTL:           5 * time.Minute,
				NotifyFallbackInterval:     5 * time.Second,
				ShutdownGrace:              30 * time.Second,
				RetentionTerminalRuns:      168 * time.Hour,
				RetentionDedupeGrace:       24 * time.Hour,
				RetentionBatchSize:         500,
			},
		},
		{
			name: "every setting overridden",
			env: map[string]string{
				"ENGINE_DATABASE_URL":                  "postgres://engine/db",
				"DATABASE_URL":                         "postgres://platform/db",
				"ENGINE_DB_MAX_CONNS":                  "7",
				"ENGINE_DB_MIN_CONNS":                  "0",
				"ENGINE_DB_MAX_CONN_LIFETIME":          "2h",
				"ENGINE_DB_MAX_CONN_IDLE_TIME":         "3m",
				"ENGINE_DB_HEALTHCHECK_PERIOD":         "4s",
				"ENGINE_NOTIFY_ENABLED":                "false",
				"ENGINE_NOTIFY_FALLBACK_INTERVAL":      "6s",
				"ENGINE_WORKFLOW_POLL_INTERVAL":        "11ms",
				"ENGINE_ACTIVITY_POLL_INTERVAL":        "12ms",
				"ENGINE_MAINTENANCE_POLL_INTERVAL":     "13ms",
				"ENGINE_METRICS_SAMPLE_INTERVAL":       "14ms",
				"ENGINE_RUN_LEASE_TTL":                 "15s",
				"ENGINE_ACTIVITY_LEASE_TTL":            "16s",
				"ENGINE_SHUTDOWN_GRACE":                "17s",
				"ENGINE_LEASE_COMPLETION_GRACE":        "18s",
				"ENGINE_RETENTION_TERMINAL_RUNS":       "19h",
				"ENGINE_RETENTION_DEDUPE_GRACE":        "20h",
				"ENGINE_RETENTION_BATCH_SIZE":          "21",
				"ENGINE_PROJECTOR_BATCH_SIZE":          "22",
				"ENGINE_MAX_CHILD_DEPTH":               "23",
				"ENGINE_MAX_CONTINUATION_FOLLOW_DEPTH": "24",
				"ENGINE_PROJECT_ID":                    projectID.String(),
				"ENGINE_METRICS_ADDR":                  "127.0.0.1:9091",
				"ENGINE_HTTP_ADDR":                     "127.0.0.1:9092",
			},
			want: Options{
				DatabaseURL:                "postgres://engine/db",
				DBMaxConns:                 7,
				DBMinConns:                 0,
				DBMinConnsSet:              true,
				DBMaxConnLifetime:          2 * time.Hour,
				DBMaxConnIdleTime:          3 * time.Minute,
				DBHealthCheckPeriod:        4 * time.Second,
				DisableNotify:              true,
				NotifyFallbackInterval:     6 * time.Second,
				WorkflowPollInterval:       11 * time.Millisecond,
				ActivityPollInterval:       12 * time.Millisecond,
				MaintenancePollInterval:    13 * time.Millisecond,
				MetricsSampleInterval:      14 * time.Millisecond,
				RunLeaseTTL:                15 * time.Second,
				ActivityLeaseTTL:           16 * time.Second,
				ShutdownGrace:              17 * time.Second,
				LeaseCompletionGrace:       18 * time.Second,
				RetentionTerminalRuns:      19 * time.Hour,
				RetentionDedupeGrace:       20 * time.Hour,
				RetentionBatchSize:         21,
				ProjectorBatchSize:         22,
				MaxChildDepth:              23,
				MaxContinuationFollowDepth: 24,
				ProjectID:                  &projectID,
				MetricsAddr:                "127.0.0.1:9091",
				HTTPAddr:                   "127.0.0.1:9092",
			},
		},
		{
			name: "zero windows disable shutdown grace and retention",
			env: map[string]string{
				"DATABASE_URL":                   "postgres://zero/db",
				"ENGINE_SHUTDOWN_GRACE":          "0s",
				"ENGINE_RETENTION_TERMINAL_RUNS": "0s",
				"ENGINE_RETENTION_DEDUPE_GRACE":  "0s",
			},
			want: Options{
				DatabaseURL:                "postgres://zero/db",
				DBMaxConns:                 10,
				DBMinConns:                 2,
				DBMinConnsSet:              true,
				DBMaxConnLifetime:          time.Hour,
				DBMaxConnIdleTime:          30 * time.Minute,
				DBHealthCheckPeriod:        time.Minute,
				ProjectorBatchSize:         1000,
				MaxChildDepth:              32,
				MaxContinuationFollowDepth: 32,
				WorkflowPollInterval:       time.Second,
				ActivityPollInterval:       time.Second,
				MaintenancePollInterval:    10 * time.Second,
				MetricsSampleInterval:      30 * time.Second,
				RunLeaseTTL:                30 * time.Second,
				ActivityLeaseTTL:           5 * time.Minute,
				NotifyFallbackInterval:     5 * time.Second,
				ShutdownGrace:              -1,
				RetentionTerminalRuns:      -1,
				RetentionDedupeGrace:       -1,
				RetentionBatchSize:         500,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range engineEnvKeys {
				t.Setenv(key, tt.env[key])
			}

			got, err := OptionsFromEnv(nil)
			if err != nil {
				t.Fatalf("OptionsFromEnv() error = %v", err)
			}
			if got.Logger == nil {
				t.Fatal("OptionsFromEnv() Logger = nil, want a configured logger")
			}
			got.Logger = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("OptionsFromEnv() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestOptionsFromEnvLoggerUsesLoggingConfig(t *testing.T) {
	for _, key := range engineEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", "postgres://logging/db")
	t.Setenv("ENGINE_LOG_LEVEL", "debug")
	t.Setenv("ENGINE_LOG_FORMAT", "text")

	var out bytes.Buffer
	got, err := OptionsFromEnv(&out)
	if err != nil {
		t.Fatalf("OptionsFromEnv() error = %v", err)
	}
	got.Logger.Debug("probe", "event", "options_env_test")
	if line := out.String(); !strings.Contains(line, "level=DEBUG") || !strings.Contains(line, "event=options_env_test") {
		t.Fatalf("logger output = %q, want a text-format debug line", line)
	}
}

func TestOptionsFromEnvRejectsInvalidSettings(t *testing.T) {
	for _, key := range engineEnvKeys {
		t.Setenv(key, "")
	}
	if _, err := OptionsFromEnv(nil); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("OptionsFromEnv() without a database URL error = %v, want DATABASE_URL error", err)
	}

	t.Setenv("DATABASE_URL", "postgres://invalid/db")
	t.Setenv("ENGINE_PROJECT_ID", "not-a-uuid")
	if _, err := OptionsFromEnv(nil); err == nil || !strings.Contains(err.Error(), "ENGINE_PROJECT_ID") {
		t.Fatalf("OptionsFromEnv() with a bad project id error = %v, want ENGINE_PROJECT_ID error", err)
	}
}
