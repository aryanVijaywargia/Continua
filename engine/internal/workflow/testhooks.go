package workflow

import (
	"context"
	"os"

	enginetesthooks "github.com/continua-ai/continua/engine/internal/testhooks"
)

const (
	TestWorkflowClaimMarkerEnv  = "CONTINUA_ENGINE_TEST_WORKFLOW_CLAIM_MARKER_FILE"
	TestWorkflowClaimReleaseEnv = "CONTINUA_ENGINE_TEST_WORKFLOW_CLAIM_RELEASE_FILE"
	TestWorkflowFinalMarkerEnv  = "CONTINUA_ENGINE_TEST_WORKFLOW_FINAL_MARKER_FILE"
	TestWorkflowFinalReleaseEnv = "CONTINUA_ENGINE_TEST_WORKFLOW_FINAL_RELEASE_FILE"
)

func applyTestClaimHook(ctx context.Context) error {
	return applyTestHook(ctx, TestWorkflowClaimMarkerEnv, TestWorkflowClaimReleaseEnv)
}

func applyTestFinalHook(ctx context.Context) error {
	return applyTestHook(ctx, TestWorkflowFinalMarkerEnv, TestWorkflowFinalReleaseEnv)
}

func applyTestHook(ctx context.Context, markerEnv string, releaseEnv string) error {
	return enginetesthooks.ApplyFileGate(ctx, os.Getenv(markerEnv), os.Getenv(releaseEnv))
}
