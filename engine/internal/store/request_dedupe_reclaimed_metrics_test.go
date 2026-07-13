package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	enginemetrics "github.com/continua-ai/continua/engine/internal/metrics"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestClaimStartRequestDedupeRecordsReclaimedOutcome(t *testing.T) {
	ts := newTestStore(t)
	registry := prometheus.NewRegistry()
	ts.store = ts.store.WithMetrics(enginemetrics.New(registry))

	for i := range 2 {
		params := ClaimStartRequestDedupeParams{
			ProjectID:    enginetest.DefaultPlatformProjectID,
			RequestScope: "engine.start",
			RequestKey:   fmt.Sprintf("request-dedupe-reclaimed-metrics-%d", i),
			ExpiresAt:    time.Now().Add(time.Hour),
		}

		tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatalf("BeginTx(new) error = %v", err)
		}
		claim, err := tx.ClaimStartRequestDedupe(ts.ctx, params)
		if err != nil {
			t.Fatalf("ClaimStartRequestDedupe(new) error = %v", err)
		}
		if err := tx.Commit(ts.ctx); err != nil {
			t.Fatalf("Commit(new) error = %v", err)
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
			t.Fatalf("BeginTx(reclaimed) error = %v", err)
		}
		reclaimed, err := tx.ClaimStartRequestDedupe(ts.ctx, params)
		if err != nil {
			t.Fatalf("ClaimStartRequestDedupe(reclaimed) error = %v", err)
		}
		if reclaimed.State != StartRequestDedupeClaimStateClaimedReclaimed {
			t.Fatalf("reclaimed claim state = %q, want %q", reclaimed.State, StartRequestDedupeClaimStateClaimedReclaimed)
		}
		if err := tx.Commit(ts.ctx); err != nil {
			t.Fatalf("Commit(reclaimed) error = %v", err)
		}
	}

	assertDedupeClaimMetric(t, registry, "new", 2)
	assertDedupeClaimMetric(t, registry, "reclaimed", 2)
}
