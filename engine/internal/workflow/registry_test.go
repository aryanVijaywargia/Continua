package workflow

import (
	"testing"

	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestRegistryLatestUsesNumericAwareVersionOrdering(t *testing.T) {
	run := func(publicworkflow.Context) error { return nil }
	registry, err := NewRegistry(
		publicworkflow.Definition{Name: "demo", Version: "v2", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v10", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v3", Run: run},
		publicworkflow.Definition{Name: "other", Version: "v99", Run: run},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	definition, ok := registry.Latest("demo")
	if !ok {
		t.Fatalf("Latest(demo) returned ok=false")
	}
	if definition.Version != "v10" {
		t.Fatalf("Latest(demo) version = %q, want v10", definition.Version)
	}
}

func TestRegistryLatestFallsBackToLexicographicForNonNumericVersions(t *testing.T) {
	run := func(publicworkflow.Context) error { return nil }
	registry, err := NewRegistry(
		publicworkflow.Definition{Name: "demo", Version: "beta", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "alpha", Run: run},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	definition, ok := registry.Latest("demo")
	if !ok {
		t.Fatalf("Latest(demo) returned ok=false")
	}
	if definition.Version != "beta" {
		t.Fatalf("Latest(demo) version = %q, want beta", definition.Version)
	}
}

func TestRegistryLatestMissingDefinition(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if _, ok := registry.Latest("missing"); ok {
		t.Fatalf("Latest(missing) returned ok=true")
	}
}
