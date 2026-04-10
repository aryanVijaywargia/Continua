package workflow

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeActivityOptions(t *testing.T) {
	t.Run("defaults to single attempt", func(t *testing.T) {
		normalized, err := NormalizeActivityOptions(ActivityOptions{})
		if err != nil {
			t.Fatalf("NormalizeActivityOptions() error = %v", err)
		}
		if normalized.MaxAttempts != 1 {
			t.Fatalf("expected max_attempts=1, got %d", normalized.MaxAttempts)
		}
		if normalized.InitialBackoffMS != nil || normalized.MaxBackoffMS != nil || normalized.BackoffMultiplier != nil {
			t.Fatalf("expected empty retry columns for default options, got %+v", normalized)
		}
	})

	t.Run("converts persisted representation", func(t *testing.T) {
		normalized, err := NormalizeActivityOptions(ActivityOptions{
			RetryPolicy: &RetryPolicy{
				MaxAttempts:       3,
				InitialBackoff:    1500 * time.Millisecond,
				MaxBackoff:        3 * time.Second,
				BackoffMultiplier: 2.5,
			},
		})
		if err != nil {
			t.Fatalf("NormalizeActivityOptions() error = %v", err)
		}
		if normalized.MaxAttempts != 3 {
			t.Fatalf("expected max_attempts=3, got %d", normalized.MaxAttempts)
		}
		if normalized.InitialBackoffMS == nil || *normalized.InitialBackoffMS != 1500 {
			t.Fatalf("expected initial_backoff_ms=1500, got %+v", normalized.InitialBackoffMS)
		}
		if normalized.BackoffMultiplier == nil || *normalized.BackoffMultiplier != 2.5 {
			t.Fatalf("expected backoff_multiplier=2.5, got %+v", normalized.BackoffMultiplier)
		}
	})

	t.Run("rejects invalid policies", func(t *testing.T) {
		testCases := []struct {
			name string
			opts ActivityOptions
		}{
			{
				name: "max attempts zero",
				opts: ActivityOptions{RetryPolicy: &RetryPolicy{MaxAttempts: 0}},
			},
			{
				name: "sub millisecond initial backoff",
				opts: ActivityOptions{RetryPolicy: &RetryPolicy{
					MaxAttempts:       2,
					InitialBackoff:    500 * time.Microsecond,
					MaxBackoff:        time.Millisecond,
					BackoffMultiplier: 2,
				}},
			},
			{
				name: "max backoff smaller than initial",
				opts: ActivityOptions{RetryPolicy: &RetryPolicy{
					MaxAttempts:       2,
					InitialBackoff:    2 * time.Second,
					MaxBackoff:        time.Second,
					BackoffMultiplier: 2,
				}},
			},
			{
				name: "multiplier smaller than one",
				opts: ActivityOptions{RetryPolicy: &RetryPolicy{
					MaxAttempts:       2,
					InitialBackoff:    time.Second,
					MaxBackoff:        2 * time.Second,
					BackoffMultiplier: 0.5,
				}},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := NormalizeActivityOptions(tc.opts); err == nil {
					t.Fatalf("expected NormalizeActivityOptions() to reject %+v", tc.opts)
				}
			})
		}
	})

	t.Run("ignores backoff fields for single attempt policies", func(t *testing.T) {
		normalized, err := NormalizeActivityOptions(ActivityOptions{
			RetryPolicy: &RetryPolicy{
				MaxAttempts:       1,
				InitialBackoff:    -time.Second,
				MaxBackoff:        0,
				BackoffMultiplier: 0,
			},
		})
		if err != nil {
			t.Fatalf("NormalizeActivityOptions() error = %v", err)
		}
		if normalized.MaxAttempts != 1 {
			t.Fatalf("expected max_attempts=1, got %d", normalized.MaxAttempts)
		}
		if normalized.InitialBackoffMS != nil || normalized.MaxBackoffMS != nil || normalized.BackoffMultiplier != nil {
			t.Fatalf("expected retry columns to stay nil for single-attempt policy, got %+v", normalized)
		}
	})
}

func TestNonRetryableError(t *testing.T) {
	cause := errors.New("boom")
	wrapped := NonRetryableError(cause)

	if !IsNonRetryable(wrapped) {
		t.Fatal("expected wrapped error to be marked non-retryable")
	}
	if errors.Is(wrapped, cause) == false {
		t.Fatal("expected wrapped error to unwrap to the original cause")
	}
	if IsNonRetryable(cause) {
		t.Fatal("expected plain error to not be marked non-retryable")
	}
}
