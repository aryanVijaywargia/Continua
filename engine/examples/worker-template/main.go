// Command worker-template is a minimal, copyable worker host for your own
// Continua workflows. It registers one workflow and one local activity, reads
// every ENGINE_* setting with runtime.OptionsFromEnv, and drains in-flight
// work on SIGINT or SIGTERM.
//
// See docs-site/guides/run-your-own-workflows.mdx for the full setup.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

// Rename these for your application. A run starts a (name, version) pair, so
// ship a new version when you change workflow code in a way that changes its
// history. See docs-site/guides/workflow-code-versioning.mdx.
const (
	helloWorkflowName    = "app.hello"
	helloWorkflowVersion = "v1"
	sayHelloActivityType = "app.say-hello"
)

type helloInput struct {
	Name string `json:"name"`
}

type helloOutput struct {
	Message string `json:"message"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Cancel the context on SIGINT or SIGTERM. Run then stops claiming work and
	// waits up to ENGINE_SHUTDOWN_GRACE for in-flight work to finish.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	options, err := runtime.OptionsFromEnv(os.Stderr)
	if err != nil {
		return err
	}
	options.Workflows = []workflow.Definition{helloDefinition()}
	options.Activities = map[string]runtime.ActivityHandler{
		sayHelloActivityType: sayHello,
	}

	rt, err := runtime.New(options)
	if err != nil {
		return err
	}
	options.Logger.Info("worker starting", "workflow", helloWorkflowName, "version", helloWorkflowVersion)
	return rt.Run(ctx)
}

func helloDefinition() workflow.Definition {
	return workflow.Definition{
		Name:    helloWorkflowName,
		Version: helloWorkflowVersion,
		Run:     runHello,
	}
}

// runHello is the workflow. Keep it deterministic: do I/O in activities, and
// use ctx.Now, ctx.Sleep, and ctx.SideEffect instead of the time and rand
// packages.
func runHello(ctx workflow.Context) error {
	var input helloInput
	if err := ctx.Input(&input); err != nil {
		return err
	}

	var output helloOutput
	if err := ctx.Activity("say-hello", sayHelloActivityType, input, &output); err != nil {
		return err
	}
	return ctx.SetResult(output)
}

// sayHello is a local activity. It runs in this process and may do I/O.
// ctx.Activity runs it once; use ctx.ActivityWithOptions with a RetryPolicy to
// retry failures.
func sayHello(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input helloInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("decode say-hello input: %w", err)
	}
	if input.Name == "" {
		input.Name = "world"
	}
	return json.Marshal(helloOutput{Message: "hello, " + input.Name})
}
