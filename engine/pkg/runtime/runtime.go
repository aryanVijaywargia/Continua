// Package runtime embeds the Continua durable-execution engine as a library:
// user programs register workflow definitions and activity handlers, then run
// the engine workers against Postgres.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/continua-ai/continua/engine/pkg/workflow"
)

// ActivityHandler executes one activity invocation. It mirrors the engine's
// internal activity handler contract.
type ActivityHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

// Options configures an embedded engine runtime.
type Options struct {
	// DatabaseURL is the Postgres connection string. Required.
	DatabaseURL string
	// Workflows are the workflow definitions this runtime can execute.
	Workflows []workflow.Definition
	// Activities maps activity type to handler.
	Activities map[string]ActivityHandler
	// ProjectID optionally scopes all polling to a single project.
	ProjectID *uuid.UUID
	// Poll intervals and lease TTLs; zero values use the engine defaults.
	WorkflowPollInterval    time.Duration
	ActivityPollInterval    time.Duration
	MaintenancePollInterval time.Duration
	RunLeaseTTL             time.Duration
	ActivityLeaseTTL        time.Duration
}

// Runtime is an embedded engine instance built from Options.
type Runtime struct{}

// New validates the options and builds the workflow/activity registries.
func New(opts Options) (*Runtime, error) {
	return nil, errors.New("runtime: not implemented")
}

// Run executes the engine workers until ctx is cancelled. It returns nil on
// graceful shutdown.
func (r *Runtime) Run(ctx context.Context) error {
	return errors.New("runtime: not implemented")
}
