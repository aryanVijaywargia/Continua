package config

import (
	"strings"
	"testing"
	"time"
)

func TestNotifyDefaults(t *testing.T) {
	setNotifyTestDatabase(t)
	t.Setenv("ENGINE_NOTIFY_ENABLED", "")
	t.Setenv("ENGINE_NOTIFY_FALLBACK_INTERVAL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Runtime.NotifyEnabled {
		t.Fatal("Runtime.NotifyEnabled = false, want true by default")
	}
	if cfg.Runtime.NotifyFallbackInterval != 5*time.Second {
		t.Fatalf("Runtime.NotifyFallbackInterval = %s, want 5s", cfg.Runtime.NotifyFallbackInterval)
	}
}

func TestNotifyEnabledFromEnv(t *testing.T) {
	tests := []struct {
		value   string
		want    bool
		wantErr bool
	}{
		{value: "false", want: false},
		{value: "0", want: false},
		{value: "true", want: true},
		{value: "banana", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			setNotifyTestDatabase(t)
			t.Setenv("ENGINE_NOTIFY_ENABLED", tt.value)
			t.Setenv("ENGINE_NOTIFY_FALLBACK_INTERVAL", "")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() error = nil, want invalid ENGINE_NOTIFY_ENABLED error")
				}
				if !strings.Contains(err.Error(), "ENGINE_NOTIFY_ENABLED") {
					t.Fatalf("Load() error = %q, want ENGINE_NOTIFY_ENABLED", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Runtime.NotifyEnabled != tt.want {
				t.Fatalf("Runtime.NotifyEnabled = %t, want %t", cfg.Runtime.NotifyEnabled, tt.want)
			}
		})
	}
}

func TestNotifyFallbackIntervalFromEnv(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{value: "2s", want: 2 * time.Second},
		{value: "0", wantErr: true},
		{value: "-1s", wantErr: true},
		{value: "garbage", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			setNotifyTestDatabase(t)
			t.Setenv("ENGINE_NOTIFY_ENABLED", "")
			t.Setenv("ENGINE_NOTIFY_FALLBACK_INTERVAL", tt.value)

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil for ENGINE_NOTIFY_FALLBACK_INTERVAL=%q, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Runtime.NotifyFallbackInterval != tt.want {
				t.Fatalf("Runtime.NotifyFallbackInterval = %s, want %s", cfg.Runtime.NotifyFallbackInterval, tt.want)
			}
		})
	}
}

func setNotifyTestDatabase(t *testing.T) {
	t.Helper()
	t.Setenv("ENGINE_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "postgres://engine")
}
