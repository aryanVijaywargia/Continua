package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
