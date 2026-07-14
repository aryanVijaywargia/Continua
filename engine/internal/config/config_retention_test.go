package config

import (
	"testing"
	"time"
)

func TestLoadRetentionDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("ENGINE_RETENTION_TERMINAL_RUNS", "")
	t.Setenv("ENGINE_RETENTION_DEDUPE_GRACE", "")
	t.Setenv("ENGINE_RETENTION_BATCH_SIZE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.RetentionTerminalRuns != 168*time.Hour {
		t.Errorf("RetentionTerminalRuns = %s, want %s", cfg.Runtime.RetentionTerminalRuns, 168*time.Hour)
	}
	if cfg.Runtime.RetentionDedupeGrace != 24*time.Hour {
		t.Errorf("RetentionDedupeGrace = %s, want %s", cfg.Runtime.RetentionDedupeGrace, 24*time.Hour)
	}
	if cfg.Runtime.RetentionBatchSize != 500 {
		t.Errorf("RetentionBatchSize = %d, want 500", cfg.Runtime.RetentionBatchSize)
	}
}

func TestLoadRetentionFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("ENGINE_RETENTION_TERMINAL_RUNS", "36h")
	t.Setenv("ENGINE_RETENTION_DEDUPE_GRACE", "1h")
	t.Setenv("ENGINE_RETENTION_BATCH_SIZE", "50")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.RetentionTerminalRuns != 36*time.Hour {
		t.Errorf("RetentionTerminalRuns = %s, want %s", cfg.Runtime.RetentionTerminalRuns, 36*time.Hour)
	}
	if cfg.Runtime.RetentionDedupeGrace != time.Hour {
		t.Errorf("RetentionDedupeGrace = %s, want %s", cfg.Runtime.RetentionDedupeGrace, time.Hour)
	}
	if cfg.Runtime.RetentionBatchSize != 50 {
		t.Errorf("RetentionBatchSize = %d, want 50", cfg.Runtime.RetentionBatchSize)
	}
}

func TestLoadRetentionZeroDisables(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("ENGINE_RETENTION_TERMINAL_RUNS", "0")
	t.Setenv("ENGINE_RETENTION_DEDUPE_GRACE", "0")
	t.Setenv("ENGINE_RETENTION_BATCH_SIZE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.RetentionTerminalRuns != 0 {
		t.Errorf("RetentionTerminalRuns = %s, want disabled (0)", cfg.Runtime.RetentionTerminalRuns)
	}
	if cfg.Runtime.RetentionDedupeGrace != 0 {
		t.Errorf("RetentionDedupeGrace = %s, want disabled (0)", cfg.Runtime.RetentionDedupeGrace)
	}
}

func TestLoadRetentionInvalid(t *testing.T) {
	testCases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "terminal runs malformed", key: "ENGINE_RETENTION_TERMINAL_RUNS", value: "bogus"},
		{name: "dedupe grace negative", key: "ENGINE_RETENTION_DEDUPE_GRACE", value: "-1h"},
		{name: "terminal runs negative", key: "ENGINE_RETENTION_TERMINAL_RUNS", value: "-1h"},
		{name: "batch size malformed", key: "ENGINE_RETENTION_BATCH_SIZE", value: "abc"},
		{name: "batch size zero", key: "ENGINE_RETENTION_BATCH_SIZE", value: "0"},
		{name: "batch size negative", key: "ENGINE_RETENTION_BATCH_SIZE", value: "-5"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://engine")
			t.Setenv("ENGINE_DATABASE_URL", "")
			t.Setenv("ENGINE_RETENTION_TERMINAL_RUNS", "")
			t.Setenv("ENGINE_RETENTION_DEDUPE_GRACE", "")
			t.Setenv("ENGINE_RETENTION_BATCH_SIZE", "")
			t.Setenv(tc.key, tc.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q error = nil, want validation error", tc.key, tc.value)
			}
		})
	}
}
