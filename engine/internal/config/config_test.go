package config

import (
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

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_WORKFLOW_POLL_INTERVAL", "definitely-not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}
