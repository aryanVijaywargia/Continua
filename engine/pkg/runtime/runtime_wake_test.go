package runtime

import (
	"context"
	"testing"
	"time"
)

func TestMergeWakeChannelsIgnoresClosureAndForwardsActiveChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan struct{})
	second := make(chan struct{}, 1)
	close(first)

	merged := mergeWakeChannels(ctx, first, second)
	if got := cap(merged); got != 1 {
		t.Fatalf("cap(mergeWakeChannels()) = %d, want 1 for wake coalescing", got)
	}
	select {
	case <-merged:
		t.Fatal("closed input produced a spurious wake")
	case <-time.After(50 * time.Millisecond):
	}

	second <- struct{}{}
	select {
	case <-merged:
	case <-time.After(2 * time.Second):
		t.Fatal("active input did not produce a wake")
	}
}
