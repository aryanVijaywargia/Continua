package workflow

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	ActivityExecutionTargetLocal  = "local"
	ActivityExecutionTargetRemote = "remote"
)

type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

type ActivityOptions struct {
	RetryPolicy     *RetryPolicy
	ExecutionTarget string
}

type NormalizedActivityOptions struct {
	MaxAttempts       int32
	InitialBackoffMS  *int64
	MaxBackoffMS      *int64
	BackoffMultiplier *float64
	ExecutionTarget   string
}

type nonRetryableError struct {
	cause error
}

func (e *nonRetryableError) Error() string {
	if e == nil || e.cause == nil {
		return "workflow: non-retryable activity error"
	}
	return e.cause.Error()
}

func (e *nonRetryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func NonRetryableError(err error) error {
	if err == nil {
		return nil
	}
	return &nonRetryableError{cause: err}
}

func IsNonRetryable(err error) bool {
	var target *nonRetryableError
	return errors.As(err, &target)
}

func NormalizeActivityOptions(opts ActivityOptions) (NormalizedActivityOptions, error) {
	executionTarget, err := normalizeExecutionTarget(opts.ExecutionTarget)
	if err != nil {
		return NormalizedActivityOptions{}, err
	}

	normalized := NormalizedActivityOptions{
		MaxAttempts:     1,
		ExecutionTarget: executionTarget,
	}
	if opts.RetryPolicy == nil {
		return normalized, nil
	}

	policy := opts.RetryPolicy
	if policy.MaxAttempts < 1 {
		return NormalizedActivityOptions{}, fmt.Errorf("workflow: retry policy max_attempts must be >= 1")
	}

	normalized.MaxAttempts = int32(policy.MaxAttempts)
	if policy.MaxAttempts == 1 {
		return normalized, nil
	}

	initialBackoffMS := policy.InitialBackoff.Milliseconds()
	if initialBackoffMS < 1 {
		return NormalizedActivityOptions{}, fmt.Errorf("workflow: retry policy initial_backoff must be at least 1ms")
	}

	maxBackoffMS := policy.MaxBackoff.Milliseconds()
	if maxBackoffMS < initialBackoffMS {
		return NormalizedActivityOptions{}, fmt.Errorf("workflow: retry policy max_backoff must be >= initial_backoff")
	}

	if policy.BackoffMultiplier < 1.0 {
		return NormalizedActivityOptions{}, fmt.Errorf("workflow: retry policy backoff_multiplier must be >= 1.0")
	}

	normalized.InitialBackoffMS = &initialBackoffMS
	normalized.MaxBackoffMS = &maxBackoffMS
	multiplier := policy.BackoffMultiplier
	normalized.BackoffMultiplier = &multiplier
	return normalized, nil
}

func normalizeExecutionTarget(target string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(target))
	if normalized == "" {
		return ActivityExecutionTargetLocal, nil
	}
	switch normalized {
	case ActivityExecutionTargetLocal, ActivityExecutionTargetRemote:
		return normalized, nil
	default:
		return "", fmt.Errorf("workflow: activity execution_target must be %q or %q", ActivityExecutionTargetLocal, ActivityExecutionTargetRemote)
	}
}

func ComputeActivityRetryDelayMS(
	attemptCount int32,
	initialBackoffMS *int64,
	maxBackoffMS *int64,
	backoffMultiplier *float64,
) (int64, error) {
	if attemptCount < 1 {
		return 0, fmt.Errorf("workflow: activity retry attempt_count must be >= 1")
	}
	if initialBackoffMS == nil || maxBackoffMS == nil || backoffMultiplier == nil {
		return 0, fmt.Errorf("workflow: activity retry policy fields are required")
	}

	exponent := float64(attemptCount - 1)
	rawDelayMS := float64(*initialBackoffMS) * math.Pow(*backoffMultiplier, exponent)
	if maxBackoff := float64(*maxBackoffMS); rawDelayMS > maxBackoff {
		rawDelayMS = maxBackoff
	}
	return int64(math.Ceil(rawDelayMS)), nil
}
