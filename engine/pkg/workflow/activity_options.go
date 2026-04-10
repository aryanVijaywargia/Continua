package workflow

import (
	"errors"
	"fmt"
	"time"
)

type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

type ActivityOptions struct {
	RetryPolicy *RetryPolicy
}

type NormalizedActivityOptions struct {
	MaxAttempts       int32
	InitialBackoffMS  *int64
	MaxBackoffMS      *int64
	BackoffMultiplier *float64
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
	if opts.RetryPolicy == nil {
		return NormalizedActivityOptions{MaxAttempts: 1}, nil
	}

	policy := opts.RetryPolicy
	if policy.MaxAttempts < 1 {
		return NormalizedActivityOptions{}, fmt.Errorf("workflow: retry policy max_attempts must be >= 1")
	}

	normalized := NormalizedActivityOptions{
		MaxAttempts: int32(policy.MaxAttempts),
	}
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
