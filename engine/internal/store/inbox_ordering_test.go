package store

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestListPendingInboxByRunFIFOOnAvailableAtTie(t *testing.T) {
	ts := newTestStore(t)
	run := createInboxOrderingRun(t, ts, "list-pending")
	insertInboxOrderingItems(t, ts, run, "signal")

	items, err := ts.store.ListPendingInboxByRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("ListPendingInboxByRun() error = %v", err)
	}

	assertInboxPayloadIdxSequence(t, "ListPendingInboxByRun", items)
}

func TestListOpenInboxItemsByRunAndKindFIFOOnAvailableAtTie(t *testing.T) {
	ts := newTestStore(t)
	run := createInboxOrderingRun(t, ts, "list-open-kind")
	insertInboxOrderingItems(t, ts, run, "signal")

	items, err := ts.store.ListOpenInboxItemsByRunAndKind(ts.ctx, run.ID, "signal")
	if err != nil {
		t.Fatalf("ListOpenInboxItemsByRunAndKind() error = %v", err)
	}

	assertInboxPayloadIdxSequence(t, "ListOpenInboxItemsByRunAndKind", items)
}

func TestListDiscardedTimerInboxItemsByRunFIFOOnAvailableAtTie(t *testing.T) {
	ts := newTestStore(t)
	run := createInboxOrderingRun(t, ts, "discarded-timers")
	insertInboxOrderingItems(t, ts, run, "timer")

	if _, err := ts.store.DiscardOpenInboxItemsByRun(ts.ctx, run.ID); err != nil {
		t.Fatalf("DiscardOpenInboxItemsByRun() error = %v", err)
	}
	items, err := ts.store.ListDiscardedTimerInboxItemsByRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("ListDiscardedTimerInboxItemsByRun() error = %v", err)
	}

	assertInboxPayloadIdxSequence(t, "ListDiscardedTimerInboxItemsByRun", items)
}

const inboxOrderingItemCount = 12

func createInboxOrderingRun(
	t *testing.T,
	ts *testStore,
	instanceKey string,
) enginedb.EngineRun {
	t.Helper()

	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-inbox-ordering-"+instanceKey)
	run := ts.createRun(t, instance, 1)
	return run
}

func insertInboxOrderingItems(
	t *testing.T,
	ts *testStore,
	run enginedb.EngineRun,
	kind string,
) {
	t.Helper()

	availableAt := time.Now().Add(-time.Minute)
	for idx := range inboxOrderingItemCount {
		_, err := ts.store.CreateInboxItem(ts.ctx, enginedb.CreateInboxItemParams{
			ProjectID:   run.ProjectID,
			InstanceID:  run.InstanceID,
			RunID:       enginetest.NullableUUID(run.ID),
			Kind:        kind,
			Payload:     []byte(`{"idx":` + strconv.Itoa(idx) + `}`),
			AvailableAt: availableAt,
			DedupeKey:   enginetest.Ptr(kind + "-fifo-" + string(rune('a'+idx))),
		})
		if err != nil {
			t.Fatalf("CreateInboxItem(%d) error = %v", idx, err)
		}
	}
}

func assertInboxPayloadIdxSequence(t *testing.T, operation string, items []enginedb.EngineInbox) {
	t.Helper()

	observed := inboxPayloadIdxSequence(t, items)
	if len(observed) != inboxOrderingItemCount {
		t.Fatalf("%s returned %d items, want %d; observed idx sequence: %v",
			operation,
			len(observed),
			inboxOrderingItemCount,
			observed,
		)
	}
	for idx, observedIdx := range observed {
		if observedIdx != idx {
			t.Fatalf("%s returned idx sequence %v, want [0 1 2 3 4 5 6 7 8 9 10 11]",
				operation,
				observed,
			)
		}
	}
}

func inboxPayloadIdxSequence(t *testing.T, items []enginedb.EngineInbox) []int {
	t.Helper()

	observed := make([]int, 0, len(items))
	for itemIdx, item := range items {
		var payload struct {
			Idx int `json:"idx"`
		}
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			t.Fatalf("decode inbox payload at result index %d: %v; payload=%s", itemIdx, err, item.Payload)
		}
		observed = append(observed, payload.Idx)
	}
	return observed
}
