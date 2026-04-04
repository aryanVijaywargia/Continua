package darklaunch

import (
	"context"
	"fmt"
	"os"
	"time"

	enginetesthooks "github.com/continua-ai/continua/engine/internal/testhooks"
)

const (
	TestActivityAttemptsFileEnv = "CONTINUA_ENGINE_TEST_ACTIVITY_ATTEMPTS_FILE"
	TestActivityReleaseFileEnv  = "CONTINUA_ENGINE_TEST_ACTIVITY_RELEASE_FILE"
)

func applyTestActivityHooks(ctx context.Context, activityName string) error {
	if err := appendTestActivityAttempt(activityName); err != nil {
		return err
	}
	return waitForTestActivityRelease(ctx)
}

func appendTestActivityAttempt(activityName string) error {
	attemptsFile := os.Getenv(TestActivityAttemptsFileEnv)
	if attemptsFile == "" {
		return nil
	}

	line := []byte(fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339Nano), activityName))
	file, err := os.OpenFile(attemptsFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(line)
	return err
}

func waitForTestActivityRelease(ctx context.Context) error {
	releaseFile := os.Getenv(TestActivityReleaseFileEnv)
	if releaseFile == "" {
		return nil
	}
	return enginetesthooks.WaitForReleaseFile(ctx, releaseFile)
}
