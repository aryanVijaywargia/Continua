package config

import "testing"

func TestLoadHTTPAddrDefaultsEmpty(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENGINE_HTTP_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.HTTPAddr != "" {
		t.Fatalf("Runtime.HTTPAddr = %q, want empty string", cfg.Runtime.HTTPAddr)
	}
}

func TestLoadHTTPAddrFromEnv(t *testing.T) {
	t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENGINE_HTTP_ADDR", "127.0.0.1:9099")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.HTTPAddr != "127.0.0.1:9099" {
		t.Fatalf("Runtime.HTTPAddr = %q, want %q", cfg.Runtime.HTTPAddr, "127.0.0.1:9099")
	}
}
