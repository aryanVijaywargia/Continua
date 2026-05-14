package darklaunch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	enginetesthooks "github.com/continua-ai/continua/engine/internal/testhooks"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
)

const (
	TestActivityAttemptsFileEnv     = "CONTINUA_ENGINE_TEST_ACTIVITY_ATTEMPTS_FILE"
	TestActivityReleaseFileEnv      = "CONTINUA_ENGINE_TEST_ACTIVITY_RELEASE_FILE"
	TestActivityFailCountEnv        = "CONTINUA_ENGINE_TEST_ACTIVITY_FAIL_COUNT"
	TestActivityNonRetryableFailEnv = "CONTINUA_ENGINE_TEST_ACTIVITY_NON_RETRYABLE_FAIL"
)

var testActivityAttemptCounters sync.Map

func applyTestActivityHooks(ctx context.Context, activityName string) error {
	attempt := nextTestActivityAttempt(activityName)
	if err := appendTestActivityAttempt(activityName, attempt); err != nil {
		return err
	}
	if shouldFailTestActivityAttempt(attempt) {
		err := fmt.Errorf("forced test activity failure on attempt %d: %w", attempt, errForcedTestActivityFailure)
		if os.Getenv(TestActivityNonRetryableFailEnv) == "1" {
			return publicworkflow.NonRetryableError(err)
		}
		return err
	}
	return waitForTestActivityRelease(ctx)
}

var errForcedTestActivityFailure = errors.New("forced test activity failure")

func nextTestActivityAttempt(activityName string) int32 {
	counterAny, _ := testActivityAttemptCounters.LoadOrStore(activityName, &atomic.Int32{})
	counter := counterAny.(*atomic.Int32)
	return counter.Add(1)
}

func appendTestActivityAttempt(activityName string, attempt int32) error {
	attemptsFile := os.Getenv(TestActivityAttemptsFileEnv)
	if attemptsFile == "" {
		return nil
	}

	line := []byte(fmt.Sprintf("%s %s attempt=%d\n", time.Now().UTC().Format(time.RFC3339Nano), activityName, attempt))
	file, err := os.OpenFile(attemptsFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(line)
	return err
}

func shouldFailTestActivityAttempt(attempt int32) bool {
	rawFailCount := os.Getenv(TestActivityFailCountEnv)
	if rawFailCount == "" {
		return false
	}

	failCount, err := strconv.Atoi(rawFailCount)
	if err != nil || failCount <= 0 {
		return false
	}
	return int(attempt) <= failCount
}

func waitForTestActivityRelease(ctx context.Context) error {
	releaseFile := os.Getenv(TestActivityReleaseFileEnv)
	if releaseFile == "" {
		return nil
	}
	return enginetesthooks.WaitForReleaseFile(ctx, releaseFile)
}
