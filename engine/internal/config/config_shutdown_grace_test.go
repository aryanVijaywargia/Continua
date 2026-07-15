package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadShutdownGraceDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("ENGINE_SHUTDOWN_GRACE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.ShutdownGrace != 30*time.Second {
		t.Fatalf("ShutdownGrace = %s, want %s", cfg.Runtime.ShutdownGrace, 30*time.Second)
	}
}

func TestLoadShutdownGraceFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://engine")
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("ENGINE_SHUTDOWN_GRACE", "5s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.ShutdownGrace != 5*time.Second {
		t.Fatalf("ShutdownGrace = %s, want %s", cfg.Runtime.ShutdownGrace, 5*time.Second)
	}
}

func TestLoadShutdownGraceInvalid(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://engine")
		t.Setenv("ENGINE_DATABASE_URL", "")
		t.Setenv("ENGINE_SHUTDOWN_GRACE", "nope")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want invalid ENGINE_SHUTDOWN_GRACE error")
		}
		if !strings.Contains(err.Error(), "ENGINE_SHUTDOWN_GRACE") {
			t.Fatalf("Load() error = %q, want mention of ENGINE_SHUTDOWN_GRACE", err)
		}
	})

	t.Run("negative", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://engine")
		t.Setenv("ENGINE_DATABASE_URL", "")
		t.Setenv("ENGINE_SHUTDOWN_GRACE", "-1s")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want non-negative ENGINE_SHUTDOWN_GRACE validation error")
		}
	})
}
