package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

func TestRuntimeRunFailsWhenProjectMissing(t *testing.T) {
	db := enginetest.NewTestDatabase(t)
	missingProjectID := uuid.New()

	rt, err := engineruntime.New(engineruntime.Options{
		DatabaseURL: db.DatabaseURL,
		Workflows: []workflow.Definition{
			{
				Name:    "missingproject.noop",
				Version: "v1",
				Run: func(ctx workflow.Context) error {
					return ctx.SetResult(map[string]bool{"ok": true})
				},
			},
		},
		ProjectID:            &missingProjectID,
		WorkflowPollInterval: 25 * time.Millisecond,
		ActivityPollInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	started := time.Now()
	err = rt.Run(ctx)
	if err == nil {
		t.Fatal("Runtime.Run() error = nil, want missing project error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Runtime.Run() error = %v, want missing project error before context deadline", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("Runtime.Run() returned after context expired in %s with err=%v", time.Since(started), err)
	}
	if !strings.Contains(err.Error(), missingProjectID.String()) {
		t.Fatalf("Runtime.Run() error = %q, want missing project id %s", err.Error(), missingProjectID)
	}
}
