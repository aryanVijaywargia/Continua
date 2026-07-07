package darklaunch

import (
	"os"
	"time"

	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	DemoDefinitionName               = "darklaunch.demo"
	DemoDefinitionVersion            = "v1"
	CancelCompletesDefinitionName    = "darklaunch.cancel-completes"
	CancelCompletesDefinitionVersion = "v1"
	RetryDemoDefinitionName          = "darklaunch.retry-demo"
	RetryDemoDefinitionVersion       = "v1"
	RetryThreeDefinitionName         = "darklaunch.retry-three-demo"
	RetryThreeDefinitionVersion      = "v1"
	RetryHandledDefinitionName       = "darklaunch.retry-handled-demo"
	RetryHandledDefinitionVersion    = "v1"
	InvalidRetryDefinitionName       = "darklaunch.invalid-retry-policy"
	InvalidRetryDefinitionVersion    = "v1"
	SleepDemoDefinitionName          = "darklaunch.sleep-demo"
	SleepDemoDefinitionVersion       = "v1"
	VersionDemoDefinitionName        = "darklaunch.version-demo"
	VersionDemoDefinitionVersion     = "v1"
	VersionDemoChangeID              = "approval-gate"
	TestVersionDemoCodeEnv           = "CONTINUA_ENGINE_TEST_VERSION_DEMO_CODE"
	VersionDemoCodeOld               = "old"
	VersionDemoCodeNew               = "new"
)

type WorkflowInput struct {
	Name    string `json:"name"`
	TimerAt string `json:"timer_at"`
}

type SleepDemoInput struct {
	Name    string `json:"name"`
	SleepMS int    `json:"sleep_ms"`
}

type SignalPayload struct {
	Approval string `json:"approval"`
}

type WorkflowResult struct {
	Greeting string `json:"greeting"`
	Approval string `json:"approval"`
}

func Definitions() []workflow.Definition {
	return []workflow.Definition{
		{
			Name:    DemoDefinitionName,
			Version: DemoDefinitionVersion,
			Run:     runDemoWorkflow,
		},
		{
			Name:    CancelCompletesDefinitionName,
			Version: CancelCompletesDefinitionVersion,
			Run:     runCancelCompletesWorkflow,
		},
		{
			Name:    RetryDemoDefinitionName,
			Version: RetryDemoDefinitionVersion,
			Run:     runRetryDemoWorkflow,
		},
		{
			Name:    RetryThreeDefinitionName,
			Version: RetryThreeDefinitionVersion,
			Run:     runRetryThreeWorkflow,
		},
		{
			Name:    RetryHandledDefinitionName,
			Version: RetryHandledDefinitionVersion,
			Run:     runRetryHandledWorkflow,
		},
		{
			Name:    InvalidRetryDefinitionName,
			Version: InvalidRetryDefinitionVersion,
			Run:     runInvalidRetryWorkflow,
		},
		{
			Name:    SleepDemoDefinitionName,
			Version: SleepDemoDefinitionVersion,
			Run:     runSleepDemoWorkflow,
		},
		{
			Name:    VersionDemoDefinitionName,
			Version: VersionDemoDefinitionVersion,
			Run:     runVersionDemoWorkflow,
		},
	}
}

func runDemoWorkflow(ctx workflow.Context) error {
	return runDemoLikeWorkflow(ctx, true)
}

func runCancelCompletesWorkflow(ctx workflow.Context) error {
	return runDemoLikeWorkflow(ctx, false)
}

func runRetryDemoWorkflow(ctx workflow.Context) error {
	return runRetryWorkflow(ctx, 2, false)
}

func runRetryThreeWorkflow(ctx workflow.Context) error {
	return runRetryWorkflow(ctx, 3, false)
}

func runRetryHandledWorkflow(ctx workflow.Context) error {
	return runRetryWorkflow(ctx, 2, true)
}

func runInvalidRetryWorkflow(ctx workflow.Context) error {
	var output ActivityOutput
	return ctx.ActivityWithOptions(
		"compose-greeting",
		DemoActivityType,
		ActivityInput{Name: "invalid"},
		&output,
		workflow.ActivityOptions{RetryPolicy: &workflow.RetryPolicy{MaxAttempts: 0}},
	)
}

func runSleepDemoWorkflow(ctx workflow.Context) error {
	var input SleepDemoInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	if err := ctx.Sleep("sleep", time.Duration(input.SleepMS)*time.Millisecond); err != nil {
		return err
	}

	var signal SignalPayload
	if err := ctx.ReceiveSignal("approval", &signal); err != nil {
		return err
	}

	return ctx.SetResult(WorkflowResult{
		Greeting: "hello, " + input.Name,
		Approval: signal.Approval,
	})
}

func runVersionDemoWorkflow(ctx workflow.Context) error {
	if os.Getenv(TestVersionDemoCodeEnv) == VersionDemoCodeNew {
		return runVersionDemoNewWorkflow(ctx)
	}
	return runVersionDemoOldWorkflow(ctx)
}

func runVersionDemoOldWorkflow(ctx workflow.Context) error {
	var input WorkflowInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	var activityOutput ActivityOutput
	if err := ctx.Activity("greet", DemoActivityType, ActivityInput{Name: input.Name}, &activityOutput); err != nil {
		return err
	}

	var signal SignalPayload
	if err := ctx.ReceiveSignal("approval", &signal); err != nil {
		return err
	}

	return ctx.SetResult(map[string]string{
		"branch":   "old",
		"greeting": activityOutput.Greeting,
		"approval": signal.Approval,
	})
}

func runVersionDemoNewWorkflow(ctx workflow.Context) error {
	version := ctx.GetVersion(VersionDemoChangeID, 1, 2)

	var input WorkflowInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	var activityOutput ActivityOutput
	if err := ctx.Activity("greet", DemoActivityType, ActivityInput{Name: input.Name}, &activityOutput); err != nil {
		return err
	}

	if version >= 2 {
		return ctx.SetResult(map[string]string{
			"branch":   "new",
			"greeting": activityOutput.Greeting,
		})
	}

	var signal SignalPayload
	if err := ctx.ReceiveSignal("approval", &signal); err != nil {
		return err
	}

	return ctx.SetResult(map[string]string{
		"branch":   "old",
		"greeting": activityOutput.Greeting,
		"approval": signal.Approval,
	})
}

func runRetryWorkflow(ctx workflow.Context, maxAttempts int, handleActivityFailure bool) error {
	var input WorkflowInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	if err := ctx.SetCustomStatus(map[string]string{"phase": "activity"}); err != nil {
		return err
	}

	var activityOutput ActivityOutput
	if err := ctx.ActivityWithOptions(
		"compose-greeting",
		DemoActivityType,
		ActivityInput{Name: input.Name},
		&activityOutput,
		workflow.ActivityOptions{
			RetryPolicy: &workflow.RetryPolicy{
				MaxAttempts:       maxAttempts,
				InitialBackoff:    500 * time.Millisecond,
				MaxBackoff:        500 * time.Millisecond,
				BackoffMultiplier: 1,
			},
		},
	); err != nil {
		if handleActivityFailure {
			return ctx.SetResult(map[string]any{
				"handled": true,
				"error":   err.Error(),
			})
		}
		return err
	}

	return ctx.SetResult(activityOutput)
}

func runDemoLikeWorkflow(ctx workflow.Context, cancelReturnsCancelled bool) error {
	var input WorkflowInput
	if err := ctx.Input(&input); err != nil {
		return err
	}
	if input.Name == "" {
		input.Name = "world"
	}

	if err := ctx.SetCustomStatus(map[string]string{"phase": "activity"}); err != nil {
		return err
	}
	if ctx.CancellationRequested() {
		return cancelOutcome(cancelReturnsCancelled)
	}

	var activityOutput ActivityOutput
	if err := ctx.Activity("compose-greeting", DemoActivityType, ActivityInput{Name: input.Name}, &activityOutput); err != nil {
		return err
	}

	if err := ctx.SetCustomStatus(map[string]string{"phase": "timer"}); err != nil {
		return err
	}
	if ctx.CancellationRequested() {
		return cancelOutcome(cancelReturnsCancelled)
	}
	if err := ctx.SleepUntil("demo-timer", timerDeadline(input)); err != nil {
		return err
	}

	if err := ctx.SetCustomStatus(map[string]string{"phase": "signal"}); err != nil {
		return err
	}
	if ctx.CancellationRequested() {
		return cancelOutcome(cancelReturnsCancelled)
	}

	var signal SignalPayload
	if err := ctx.ReceiveSignal("approval", &signal); err != nil {
		return err
	}

	result := WorkflowResult{
		Greeting: activityOutput.Greeting,
		Approval: signal.Approval,
	}
	if err := ctx.SetCustomStatus(map[string]string{"phase": "completed", "approval": signal.Approval}); err != nil {
		return err
	}
	if err := ctx.SetResult(result); err != nil {
		return err
	}
	return nil
}

func cancelOutcome(cancelReturnsCancelled bool) error {
	if cancelReturnsCancelled {
		return workflow.ErrCancelled
	}
	return nil
}

func timerDeadline(input WorkflowInput) time.Time {
	if input.TimerAt == "" {
		return time.Unix(0, 0).UTC()
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, input.TimerAt)
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Unix(0, 0).UTC()
}
