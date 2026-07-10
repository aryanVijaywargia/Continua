package config

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLoadResolvesEngineProjectIDEnv(t *testing.T) {
	engineProjectID := uuid.New()
	legacyProjectID := uuid.New()

	testCases := []struct {
		name        string
		engineValue string
		legacyValue string
		want        *uuid.UUID
		wantErr     string
	}{
		{
			name:        "engine project id",
			engineValue: engineProjectID.String(),
			want:        &engineProjectID,
		},
		{
			name:        "legacy project filter",
			legacyValue: legacyProjectID.String(),
			want:        &legacyProjectID,
		},
		{
			name:        "engine project id wins over legacy",
			engineValue: engineProjectID.String(),
			legacyValue: legacyProjectID.String(),
			want:        &engineProjectID,
		},
		{
			name:        "invalid engine project id",
			engineValue: "not-a-uuid",
			wantErr:     "ENGINE_PROJECT_ID",
		},
		{
			name: "unset project id",
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENGINE_DATABASE_URL", "postgres://engine")
			t.Setenv("ENGINE_PROJECT_ID", tc.engineValue)
			t.Setenv("CONTINUA_ENGINE_TEST_PROJECT_FILTER", tc.legacyValue)

			cfg, err := Load()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("Load() error = nil, want project id parse error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Load() error = %q, want mention %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if tc.want == nil {
				if cfg.Runtime.ProjectIDFilter != nil {
					t.Fatalf("ProjectIDFilter = %s, want nil", cfg.Runtime.ProjectIDFilter)
				}
				return
			}
			if cfg.Runtime.ProjectIDFilter == nil {
				t.Fatalf("ProjectIDFilter = nil, want %s", *tc.want)
			}
			if *cfg.Runtime.ProjectIDFilter != *tc.want {
				t.Fatalf("ProjectIDFilter = %s, want %s", *cfg.Runtime.ProjectIDFilter, *tc.want)
			}
		})
	}
}
