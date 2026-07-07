package workflow

import (
	"testing"

	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestReplayGetVersionInvalidArgumentsFailControlled(t *testing.T) {
	tests := []struct {
		name         string
		changeID     string
		minSupported int
		maxSupported int
	}{
		{
			name:         "empty change id",
			changeID:     "",
			minSupported: 1,
			maxSupported: 2,
		},
		{
			name:         "min below one",
			changeID:     versionDemoChangeID,
			minSupported: 0,
			maxSupported: 2,
		},
		{
			name:         "min greater than max",
			changeID:     versionDemoChangeID,
			minSupported: 3,
			maxSupported: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := publicworkflow.Definition{
				Name:    "version-demo",
				Version: "v1",
				Run: func(ctx publicworkflow.Context) error {
					version := ctx.GetVersion(tt.changeID, tt.minSupported, tt.maxSupported)
					return ctx.SetResult(map[string]int{"version": version})
				},
			}

			decision, err := replayDefinition(definition, versionStartedHistory(t), nil, nil)
			if err != nil {
				t.Fatalf("replayDefinition() error = %v", err)
			}
			if decision.Kind != decisionFailed {
				t.Fatalf("expected failed decision, got %+v", decision)
			}
			if decision.FailureCode != "version_invalid" {
				t.Fatalf("FailureCode = %q, want version_invalid", decision.FailureCode)
			}
		})
	}
}
