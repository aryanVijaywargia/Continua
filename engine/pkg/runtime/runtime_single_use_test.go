package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestRuntimeRunReturnsErrorWhenCalledTwice(t *testing.T) {
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
		MetricsRegistry: prometheus.NewRegistry(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = rt.Run(ctx)

	err = rt.Run(context.Background())
	if err == nil {
		t.Fatal("second Run() error = nil, want single-use error")
	}
	if !strings.Contains(err.Error(), "only be called once") {
		t.Fatalf("second Run() error = %q, want single-use error", err.Error())
	}
}
