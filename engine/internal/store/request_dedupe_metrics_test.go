package store

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginemetrics "github.com/continua-ai/continua/engine/internal/metrics"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestClaimStartRequestDedupeRecordsOutcomes(t *testing.T) {
	ts := newTestStore(t)
	registry := prometheus.NewRegistry()
	ts.store = ts.store.WithMetrics(enginemetrics.New(registry))
	params := ClaimStartRequestDedupeParams{
		ProjectID:    enginetest.DefaultPlatformProjectID,
		RequestScope: "engine.start",
		RequestKey:   "request-dedupe-metrics",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx(new) error = %v", err)
	}
	newClaim, err := tx.ClaimStartRequestDedupe(ts.ctx, params)
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe(new) error = %v", err)
	}
	if newClaim.State != StartRequestDedupeClaimStateClaimedNew {
		t.Fatalf("new claim state = %q, want %q", newClaim.State, StartRequestDedupeClaimStateClaimedNew)
	}
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit(new) error = %v", err)
	}
	assertDedupeClaimMetric(t, registry, "new", 1)

	tx, err = ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx(existing in progress) error = %v", err)
	}
	inProgressClaim, err := tx.ClaimStartRequestDedupe(ts.ctx, params)
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe(existing in progress) error = %v", err)
	}
	if inProgressClaim.State != StartRequestDedupeClaimStateExistingInProgress {
		t.Fatalf("live claim state = %q, want %q", inProgressClaim.State, StartRequestDedupeClaimStateExistingInProgress)
	}
	if err := tx.Rollback(ts.ctx); err != nil {
		t.Fatalf("Rollback(existing in progress) error = %v", err)
	}
	assertDedupeClaimMetric(t, registry, "existing_in_progress", 1)

	if _, err := ts.store.FinalizeRequestDedupeWithResponse(ts.ctx, enginedb.FinalizeRequestDedupeWithResponseParams{
		ID:              newClaim.Row.ID,
		ResponsePayload: []byte(`{"instance_id":"cached"}`),
	}); err != nil {
		t.Fatalf("FinalizeRequestDedupeWithResponse() error = %v", err)
	}

	tx, err = ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx(existing finalized) error = %v", err)
	}
	finalizedClaim, err := tx.ClaimStartRequestDedupe(ts.ctx, params)
	if err != nil {
		t.Fatalf("ClaimStartRequestDedupe(existing finalized) error = %v", err)
	}
	if finalizedClaim.State != StartRequestDedupeClaimStateExistingFinalized {
		t.Fatalf("finalized claim state = %q, want %q", finalizedClaim.State, StartRequestDedupeClaimStateExistingFinalized)
	}
	if err := tx.Rollback(ts.ctx); err != nil {
		t.Fatalf("Rollback(existing finalized) error = %v", err)
	}
	assertDedupeClaimMetric(t, registry, "existing_finalized", 1)
}

func assertDedupeClaimMetric(t *testing.T, gatherer prometheus.Gatherer, outcome string, want float64) {
	t.Helper()

	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() != "continua_engine_dedupe_claims_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := metric.GetLabel()
			if len(labels) != 1 || labels[0].GetName() != "outcome" || labels[0].GetValue() != outcome {
				continue
			}
			if got := metric.GetCounter().GetValue(); got != want {
				t.Fatalf("continua_engine_dedupe_claims_total{outcome=%q} = %v, want %v", outcome, got, want)
			}
			return
		}
	}
	t.Fatalf("continua_engine_dedupe_claims_total{outcome=%q} series is missing", outcome)
}
