package darklaunch

import (
	"time"

	"github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	DemoDefinitionName    = "darklaunch.demo"
	DemoDefinitionVersion = "v1"
)

type WorkflowInput struct {
	Name    string `json:"name"`
	TimerAt string `json:"timer_at"`
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
	}
}

func runDemoWorkflow(ctx workflow.Context) error {
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
	if err := ctx.Activity("compose-greeting", DemoActivityType, ActivityInput{Name: input.Name}, &activityOutput); err != nil {
		return err
	}

	if err := ctx.SetCustomStatus(map[string]string{"phase": "timer"}); err != nil {
		return err
	}
	if err := ctx.SleepUntil("demo-timer", timerDeadline(input)); err != nil {
		return err
	}

	if err := ctx.SetCustomStatus(map[string]string{"phase": "signal"}); err != nil {
		return err
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
