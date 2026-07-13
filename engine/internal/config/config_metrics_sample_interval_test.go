package config

import (
	"testing"
	"time"
)

func TestLoadParsesEngineMetricsSampleInterval(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
		t.Setenv("ENGINE_METRICS_SAMPLE_INTERVAL", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Runtime.MetricsSampleInterval != 30*time.Second {
			t.Fatalf("Runtime.MetricsSampleInterval = %s, want 30s", cfg.Runtime.MetricsSampleInterval)
		}
	})

	t.Run("configured", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
		t.Setenv("ENGINE_METRICS_SAMPLE_INTERVAL", "45s")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Runtime.MetricsSampleInterval != 45*time.Second {
			t.Fatalf("Runtime.MetricsSampleInterval = %s, want 45s", cfg.Runtime.MetricsSampleInterval)
		}
	})
}
