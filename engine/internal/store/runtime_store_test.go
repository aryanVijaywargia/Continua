package store

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestWakeWaitingRunReportsAppliedAndNoOp(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-wake")
	run := ts.createRun(t, instance, 1)

	claimed, err := ts.store.ClaimNextRun(ts.ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	waitingFor := []byte(`{"kind":"signal","signal_name":"approval"}`)
	if _, err := ts.store.TransitionRunToWaiting(ts.ctx, enginedb.TransitionRunToWaitingParams{
		ID:           claimed.ID,
		ClaimedBy:    claimed.ClaimedBy,
		WaitingFor:   waitingFor,
		CustomStatus: []byte(`{"phase":"signal"}`),
	}); err != nil {
		t.Fatalf("TransitionRunToWaiting() error = %v", err)
	}

	wake, err := ts.store.WakeWaitingRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("WakeWaitingRun() error = %v", err)
	}
	if !wake.Applied || wake.Run.Status != enginedb.EngineRunLifecycleStatusQueued {
		t.Fatalf("expected applied wake to queue the run, got %+v", wake)
	}

	wake, err = ts.store.WakeWaitingRun(ts.ctx, run.ID)
	if err != nil {
		t.Fatalf("WakeWaitingRun() second call error = %v", err)
	}
	if wake.Applied {
		t.Fatalf("expected second wake to be a no-op, got %+v", wake)
	}
}

func TestActivityTaskCASRejectsStaleClaim(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	instance := ts.createInstance(t, projectID, "instance-stale-activity")
	run := ts.createRun(t, instance, 1)
	history := ts.createHistory(t, projectID, instance.ID, run.ID, 1, "activity.scheduled")

	task, err := ts.store.CreateActivityTask(ts.ctx, enginedb.CreateActivityTaskParams{
		ProjectID:    projectID,
		InstanceID:   instance.ID,
		RunID:        run.ID,
		HistoryID:    &history.ID,
		ActivityKey:  "activity-1",
		ActivityType: "demo.echo",
		Input:        []byte(`{"name":"Ada"}`),
		AvailableAt:  time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateActivityTask() error = %v", err)
	}

	if _, err := ts.store.ClaimNextActivityTask(ts.ctx, "worker-a", time.Minute); err != nil {
		t.Fatalf("ClaimNextActivityTask() error = %v", err)
	}

	_, err = ts.store.CompleteActivityTask(ts.ctx, task.ID, "worker-b", []byte(`{"ok":true}`))
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("expected ErrStaleClaim, got %v", err)
	}
}

func TestClaimStartRequestDedupeScenarios(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)
	requestScope := "engine.start"

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	claim, err := tx.ClaimStartRequestDedupe(ts.ctx, ClaimStartRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: requestScope,
		RequestKey:   "req-new",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe() new error = %v", err)
	}
	if claim.State != StartRequestDedupeClaimStateClaimedNew {
		t.Fatalf("expected new claim state, got %s", claim.State)
	}
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	tx, err = ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	liveClaim, err := tx.ClaimStartRequestDedupe(ts.ctx, ClaimStartRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: requestScope,
		RequestKey:   "req-new",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe() live error = %v", err)
	}
	if liveClaim.State != StartRequestDedupeClaimStateExistingInProgress {
		t.Fatalf("expected existing in-progress state, got %s", liveClaim.State)
	}
	if err := tx.Rollback(ts.ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	if _, err := ts.db.Pool.Exec(ts.ctx, `
		UPDATE engine.request_dedupe
		SET expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, claim.Row.ID); err != nil {
		t.Fatalf("expire request dedupe: %v", err)
	}

	tx, err = ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	reclaimed, err := tx.ClaimStartRequestDedupe(ts.ctx, ClaimStartRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: requestScope,
		RequestKey:   "req-new",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe() reclaimed error = %v", err)
	}
	if reclaimed.State != StartRequestDedupeClaimStateClaimedReclaimed {
		t.Fatalf("expected reclaimed state, got %s", reclaimed.State)
	}
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if _, err := ts.store.FinalizeRequestDedupeWithResponse(ts.ctx, enginedb.FinalizeRequestDedupeWithResponseParams{
		ID:              reclaimed.Row.ID,
		ResponsePayload: []byte(`{"instance_id":"cached"}`),
	}); err != nil {
		t.Fatalf("FinalizeRequestDedupeWithResponse() error = %v", err)
	}

	tx, err = ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	finalized, err := tx.ClaimStartRequestDedupe(ts.ctx, ClaimStartRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: requestScope,
		RequestKey:   "req-new",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe() finalized error = %v", err)
	}
	if finalized.State != StartRequestDedupeClaimStateExistingFinalized {
		t.Fatalf("expected finalized state, got %s", finalized.State)
	}
	if err := tx.Rollback(ts.ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
}

func TestExpireRequestDedupeAllowsReclaim(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)

	row, err := ts.store.CreateRequestDedupe(ts.ctx, enginedb.CreateRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: "engine.start",
		RequestKey:   "req-expire",
		ExpiresAt:    time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRequestDedupe() error = %v", err)
	}

	expiredCount, err := ts.store.ExpireRequestDedupe(ts.ctx)
	if err != nil {
		t.Fatalf("ExpireRequestDedupe() error = %v", err)
	}
	if expiredCount != 1 {
		t.Fatalf("expected one expired request dedupe row, got %d", expiredCount)
	}

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	claim, err := tx.ClaimStartRequestDedupe(ts.ctx, ClaimStartRequestDedupeParams{
		ProjectID:    projectID,
		RequestScope: "engine.start",
		RequestKey:   "req-expire",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe() error = %v", err)
	}
	if claim.State != StartRequestDedupeClaimStateClaimedReclaimed {
		t.Fatalf("expected reclaimed claim after expiry, got %+v", claim)
	}
	if claim.Row.ID != row.ID {
		t.Fatalf("expected reclaim to reuse row %s, got %s", row.ID, claim.Row.ID)
	}
	if err := tx.Rollback(ts.ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
}
