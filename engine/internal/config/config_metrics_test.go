package config

import "testing"

func TestLoadParsesEngineMetricsAddr(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
		t.Setenv("ENGINE_METRICS_ADDR", "127.0.0.1:9464")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Runtime.MetricsAddr != "127.0.0.1:9464" {
			t.Fatalf("Runtime.MetricsAddr = %q, want %q", cfg.Runtime.MetricsAddr, "127.0.0.1:9464")
		}
	})

	t.Run("disabled when unset", func(t *testing.T) {
		t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
		t.Setenv("ENGINE_METRICS_ADDR", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Runtime.MetricsAddr != "" {
			t.Fatalf("Runtime.MetricsAddr = %q, want empty string", cfg.Runtime.MetricsAddr)
		}
	})
}
