package config

import "testing"

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
