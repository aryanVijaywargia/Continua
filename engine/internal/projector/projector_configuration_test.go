package projector

import "testing"

func TestProjectorBatchSizeConfiguration(t *testing.T) {
	if got := New(nil).BatchSize(); got != 1000 {
		t.Errorf("New(nil).BatchSize() = %d, want 1000", got)
	}
	if got := New(nil).WithBatchSize(250).BatchSize(); got != 250 {
		t.Errorf("New(nil).WithBatchSize(250).BatchSize() = %d, want 250", got)
	}
	if got := New(nil).WithBatchSize(0).BatchSize(); got != 1000 {
		t.Errorf("New(nil).WithBatchSize(0).BatchSize() = %d, want 1000", got)
	}
}
