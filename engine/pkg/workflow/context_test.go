package workflow

import (
	"errors"
	"fmt"
	"testing"
)

func TestContinueAsNewCreatesWrappedSentinel(t *testing.T) {
	err := ContinueAsNew(map[string]any{"cursor": 7, "phase": "next"})
	if err == nil {
		t.Fatal("ContinueAsNew() returned nil")
	}
	if !errors.Is(err, ErrContinueAsNew) {
		t.Fatalf("errors.Is(err, ErrContinueAsNew) = false for %v", err)
	}

	input, ok := ContinueAsNewInput(err)
	if !ok {
		t.Fatal("ContinueAsNewInput() = not found")
	}
	if got := string(input); got != `{"cursor":7,"phase":"next"}` {
		t.Fatalf("ContinueAsNewInput() = %s", got)
	}
}

func TestContinueAsNewInputFindsWrappedError(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", mustContinueAsNewError(t, map[string]any{"cursor": 9}))
	if !errors.Is(err, ErrContinueAsNew) {
		t.Fatalf("wrapped continuation error does not satisfy errors.Is: %v", err)
	}

	input, ok := ContinueAsNewInput(err)
	if !ok {
		t.Fatal("ContinueAsNewInput() did not find wrapped continuation error")
	}
	if got := string(input); got != `{"cursor":9}` {
		t.Fatalf("ContinueAsNewInput() = %s", got)
	}
}

func TestContinueAsNewRejectsUnmarshalableInput(t *testing.T) {
	err := ContinueAsNew(make(chan int))
	if err == nil {
		t.Fatal("ContinueAsNew() unexpectedly succeeded")
	}
	if errors.Is(err, ErrContinueAsNew) {
		t.Fatalf("marshal failure should not satisfy ErrContinueAsNew: %v", err)
	}
}

func mustContinueAsNewError(t *testing.T, input any) error {
	t.Helper()

	err := ContinueAsNew(input)
	if err == nil {
		t.Fatal("ContinueAsNew() returned nil")
	}
	return err
}
