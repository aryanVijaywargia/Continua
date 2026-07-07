package main

import (
	"testing"

	engineworkflow "github.com/continua-ai/continua/engine/internal/workflow"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestResolveStartDefinitionVersionOmittedUsesLatestRegisteredVersion(t *testing.T) {
	run := func(publicworkflow.Context) error { return nil }
	registry, err := engineworkflow.NewRegistry(
		publicworkflow.Definition{Name: "demo", Version: "v1", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v10", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v2", Run: run},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	version, ok := resolveStartDefinitionVersion(registry, "demo", "")
	if !ok {
		t.Fatalf("resolveStartDefinitionVersion() returned ok=false")
	}
	if version != "v10" {
		t.Fatalf("resolved version = %q, want v10", version)
	}
}

func TestResolveStartDefinitionVersionExplicitUsesExactMatch(t *testing.T) {
	run := func(publicworkflow.Context) error { return nil }
	registry, err := engineworkflow.NewRegistry(
		publicworkflow.Definition{Name: "demo", Version: "v1", Run: run},
		publicworkflow.Definition{Name: "demo", Version: "v2", Run: run},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	version, ok := resolveStartDefinitionVersion(registry, "demo", "v1")
	if !ok {
		t.Fatalf("resolveStartDefinitionVersion() returned ok=false")
	}
	if version != "v1" {
		t.Fatalf("resolved version = %q, want v1", version)
	}

	if _, ok := resolveStartDefinitionVersion(registry, "demo", "v10"); ok {
		t.Fatalf("resolveStartDefinitionVersion() returned ok=true for unregistered explicit version")
	}
}
