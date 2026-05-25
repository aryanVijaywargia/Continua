package store

import "testing"

func TestHistoryOrdering(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-history")
	runOne := ts.createRun(t, instance, 1)
	runTwo := ts.createRun(t, instance, 2)

	first := ts.createHistory(t, projectID, instance.ID, runOne.ID, 1, "run-one.started")
	second := ts.createHistory(t, projectID, instance.ID, runTwo.ID, 1, "run-two.started")
	third := ts.createHistory(t, projectID, instance.ID, runOne.ID, 2, "run-one.completed")

	if first.ID >= second.ID || second.ID >= third.ID {
		t.Fatalf("expected sequential inserts to have monotonic ids, got %d, %d, %d", first.ID, second.ID, third.ID)
	}

	runOneHistory, err := ts.store.GetHistoryByRun(ts.ctx, runOne.ID)
	if err != nil {
		t.Fatalf("GetHistoryByRun() error = %v", err)
	}
	if len(runOneHistory) != 2 || runOneHistory[0].SequenceNo != 1 || runOneHistory[1].SequenceNo != 2 {
		t.Fatalf("expected stable per-run ordering by sequence_no, got %+v", runOneHistory)
	}
}
