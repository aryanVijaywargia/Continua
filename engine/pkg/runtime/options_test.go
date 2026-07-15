package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/continua-ai/continua/engine/internal/config"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestNewValidatesOptions(t *testing.T) {
	validDefinition := workflow.Definition{
		Name:    "usertest.greeter",
		Version: "v1",
		Run: func(workflow.Context) error {
			return nil
		},
	}
	validActivity := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}

	tests := []struct {
		name    string
		options Options
		wantErr string
	}{
		{
			name: "valid",
			options: Options{
				DatabaseURL: "postgres://example/db",
				Workflows:   []workflow.Definition{validDefinition},
				Activities: map[string]ActivityHandler{
					"usertest.greet": validActivity,
				},
			},
		},
		{
			name: "missing database url",
			options: Options{
				Workflows: []workflow.Definition{validDefinition},
				Activities: map[string]ActivityHandler{
					"usertest.greet": validActivity,
				},
			},
			wantErr: "database",
		},
		{
			name: "duplicate workflow definition",
			options: Options{
				DatabaseURL: "postgres://example/db",
				Workflows: []workflow.Definition{
					validDefinition,
					validDefinition,
				},
				Activities: map[string]ActivityHandler{
					"usertest.greet": validActivity,
				},
			},
			wantErr: "duplicate",
		},
		{
			name: "definition missing run func",
			options: Options{
				DatabaseURL: "postgres://example/db",
				Workflows: []workflow.Definition{{
					Name:    "usertest.greeter",
					Version: "v1",
				}},
				Activities: map[string]ActivityHandler{
					"usertest.greet": validActivity,
				},
			},
			wantErr: "run function",
		},
		{
			name: "invalid activity entry",
			options: Options{
				DatabaseURL: "postgres://example/db",
				Workflows:   []workflow.Definition{validDefinition},
				Activities: map[string]ActivityHandler{
					"": validActivity,
				},
			},
			wantErr: "activity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := New(tt.options)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("New() error = %v, want nil", err)
				}
				if rt == nil {
					t.Fatal("New() runtime = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("New() error = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Fatalf("New() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRuntimeRunRejectsNilContext(t *testing.T) {
	rt, err := New(Options{
		DatabaseURL: "postgres://example/db",
		Workflows: []workflow.Definition{{
			Name:    "usertest.greeter",
			Version: "v1",
			Run: func(workflow.Context) error {
				return nil
			},
		}},
		Activities: map[string]ActivityHandler{
			"usertest.greet": func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return nil, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = rt.Run(nil) //nolint:staticcheck // This test verifies the invalid nil-context path.
	if err == nil {
		t.Fatal("Run(nil) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("Run(nil) error = %q, want context required error", err.Error())
	}
}

func TestApplyRuntimeOverridesRetentionBatchSize(t *testing.T) {
	tests := []struct {
		name     string
		override int32
		want     int32
	}{
		{name: "zero keeps default", override: 0, want: 500},
		{name: "negative keeps default", override: -1, want: 500},
		{name: "positive overrides default", override: 25, want: 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults("postgres://example/db")
			applyRuntimeOverrides(cfg, &Options{RetentionBatchSize: tt.override})
			if cfg.Runtime.RetentionBatchSize != tt.want {
				t.Fatalf("RetentionBatchSize = %d, want %d", cfg.Runtime.RetentionBatchSize, tt.want)
			}
		})
	}
}

func TestApplyRuntimeOverridesPoolAndLimits(t *testing.T) {
	t.Run("positive values override engine defaults", func(t *testing.T) {
		cfg := config.Defaults("postgres://example/db")
		applyRuntimeOverrides(cfg, &Options{
			DBMaxConns:                 25,
			DBMinConns:                 5,
			DBMaxConnLifetime:          2 * time.Hour,
			DBMaxConnIdleTime:          45 * time.Minute,
			DBHealthCheckPeriod:        90 * time.Second,
			ProjectorBatchSize:         250,
			MaxChildDepth:              8,
			MaxContinuationFollowDepth: 4,
		})

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
	})

	t.Run("omitted values preserve engine defaults", func(t *testing.T) {
		cfg := config.Defaults("postgres://example/db")
		applyRuntimeOverrides(cfg, &Options{})

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
	})

	t.Run("explicit zero minimum connections overrides engine default", func(t *testing.T) {
		cfg := config.Defaults("postgres://example/db")
		applyRuntimeOverrides(cfg, &Options{DBMinConns: 0, DBMinConnsSet: true})

		if cfg.Database.MinConns != 0 {
			t.Errorf("Database.MinConns = %d, want 0", cfg.Database.MinConns)
		}
	})
}
