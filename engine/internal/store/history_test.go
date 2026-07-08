package store

import (
	"testing"

	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

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

func TestGetMaxHistorySequenceByRunEmptyRunReturnsZero(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-history-max-empty")
	run := ts.createRun(t, instance, 1)

	maxSequence, err := ts.store.GetMaxHistorySequenceByRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("GetMaxHistorySequenceByRun() error = %v", err)
	}
	if maxSequence != 0 {
		t.Fatalf("expected empty run max sequence 0, got %d", maxSequence)
	}
}

func TestGetMaxHistorySequenceByRunReturnsPerRunMax(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-history-max-per-run")
	runA := ts.createRun(t, instance, 1)
	runB := ts.createRun(t, instance, 2)

	ts.createHistory(t, projectID, instance.ID, runA.ID, 1, "run-a.started")
	ts.createHistory(t, projectID, instance.ID, runA.ID, 2, "run-a.progressed")
	ts.createHistory(t, projectID, instance.ID, runA.ID, 7, "run-a.completed")
	ts.createHistory(t, projectID, instance.ID, runB.ID, 9, "run-b.started")

	maxRunA, err := ts.store.GetMaxHistorySequenceByRun(ts.ctx, runA.ID)
	if err != nil {
		t.Fatalf("GetMaxHistorySequenceByRun(runA) error = %v", err)
	}
	if maxRunA != 7 {
		t.Fatalf("expected run A max sequence 7, got %d", maxRunA)
	}

	maxRunB, err := ts.store.GetMaxHistorySequenceByRun(ts.ctx, runB.ID)
	if err != nil {
		t.Fatalf("GetMaxHistorySequenceByRun(runB) error = %v", err)
	}
	if maxRunB != 9 {
		t.Fatalf("expected run B max sequence 9, got %d", maxRunB)
	}
}

func TestGetMaxHistorySequenceByRunInsideTx(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-history-max-tx")
	run := ts.createRun(t, instance, 1)

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ts.ctx) }()

	if _, err := tx.AppendHistory(ts.ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 3,
		EventType:  "run.tx_event",
		Payload:    []byte(`{"event":"run.tx_event"}`),
	}); err != nil {
		t.Fatalf("AppendHistory(tx) error = %v", err)
	}

	maxSequence, err := tx.GetMaxHistorySequenceByRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("GetMaxHistorySequenceByRun(tx) error = %v", err)
	}
	if maxSequence != 3 {
		t.Fatalf("expected tx-visible max sequence 3, got %d", maxSequence)
	}
}
