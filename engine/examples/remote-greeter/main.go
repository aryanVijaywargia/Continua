package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	engineruntime "github.com/continua-ai/continua/engine/pkg/runtime"
	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	remoteGreeterWorkflowName    = "examples.remote-greeter"
	remoteGreeterWorkflowVersion = "v1"
	composeGreetingActivityType  = "examples.compose-greeting"
)

type remoteGreeterInput struct {
	Name string `json:"name"`
}

type remoteGreeterOutput struct {
	Greeting string `json:"greeting"`
}

func remoteGreeterDefinition() workflow.Definition {
	return workflow.Definition{
		Name:    remoteGreeterWorkflowName,
		Version: remoteGreeterWorkflowVersion,
		Run:     runRemoteGreeterWorkflow,
	}
}

func runRemoteGreeterWorkflow(ctx workflow.Context) error {
	var input remoteGreeterInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	var output remoteGreeterOutput
	if err := ctx.ActivityWithOptions(
		"compose-greeting",
		composeGreetingActivityType,
		input,
		&output,
		workflow.ActivityOptions{
			ExecutionTarget: workflow.ActivityExecutionTargetRemote,
			RetryPolicy: &workflow.RetryPolicy{
				MaxAttempts:       3,
				InitialBackoff:    500 * time.Millisecond,
				MaxBackoff:        5 * time.Second,
				BackoffMultiplier: 2.0,
			},
		},
	); err != nil {
		return err
	}

	return ctx.SetResult(output)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	options, err := engineruntime.OptionsFromEnv(os.Stderr)
	if err != nil {
		return err
	}
	options.Workflows = []workflow.Definition{remoteGreeterDefinition()}

	rt, err := engineruntime.New(options)
	if err != nil {
		return err
	}

	return rt.Run(ctx)
}
