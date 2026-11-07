package workflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	publicworkflow "github.com/continua-ai/continua/engine/pkg/workflow"
	"github.com/continua-ai/continua/internal/api"
	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	platformstore "github.com/continua-ai/continua/internal/store"
)

const terminateTestAPIKey = "terminate-subtree-discovery-key"

// TerminateRun terminates a run and its active subtree. A grandchild's activation holds
// its parent run and its own run, and adds a run under that parent: a new child, or the
// next run of a continue-as-new. Terminating the root queues on the parent's run lock
// while the activation runs. Termination must list a run's children only after it holds
// that run. With a subtree listed before the wait, it left the new child queued under a
// terminated parent, or failed on the run that had continued as new.
func TestTerminateRunTerminatesRunsAddedWhileItWaits(t *testing.T) {
	testCases := []struct {
		name          string
		grandchildRun func(publicworkflow.Context) error
	}{
		{
			name: "grandchild starts a child",
			grandchildRun: func(ctx publicworkflow.Context) error {
				return ctx.ChildWorkflow("print-receipt", "printer", "v1", nil, nil)
			},
		},
		{
			name: "grandchild continues as new",
			grandchildRun: func(publicworkflow.Context) error {
				return publicworkflow.ContinueAsNew(nil)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			billing := func(ctx publicworkflow.Context) error {
				return ctx.ChildWorkflow("issue-receipt", "receipts", "v1", nil, nil)
			}
			fixture := newParentWakeFixture(t, ctx, billing)
			pool := fixture.db.Pool
			rootRunID := fixture.parentRun.ID
			handler := newEngineControlAPI(t, ctx, pool)

			// The grandchild pauses inside its activation, after the activation locked its
			// parent's run and its own, and before it commits the run it adds.
			paused, release := make(chan struct{}), make(chan struct{})
			activator := NewActivator(fixture.store, mustRegistry(t,
				childWaitingParentDefinition("checkout-parent-wake", nil),
				publicworkflow.Definition{Name: "billing", Version: "v1", Run: billing},
				publicworkflow.Definition{Name: "receipts", Version: "v1", Run: func(wctx publicworkflow.Context) error {
					close(paused)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					return testCase.grandchildRun(wctx)
				}},
				publicworkflow.Definition{Name: "printer", Version: "v1", Run: func(publicworkflow.Context) error { return nil }},
			))
			activateRunByID(t, ctx, pool, fixture.store, activator, fixture.childRunID, "worker-child")
			var grandchildRunID uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT current_child_run_id FROM engine.child_workflows WHERE parent_run_id = $1`,
				fixture.childRunID).Scan(&grandchildRunID); err != nil {
				t.Fatalf("read grandchild run: %v", err)
			}

			claimed := forceClaimRun(t, ctx, pool, fixture.store, grandchildRunID, "worker-grandchild")
			activation := startAsync(t, cancel, func() error { return activator.Activate(ctx, &claimed) })
			select {
			case <-paused:
			case <-ctx.Done():
				t.Fatalf("grandchild activation never ran its workflow: %v", ctx.Err())
			}

			terminate := startAsync(t, cancel, func() error { return terminateRunOverAPI(ctx, handler, rootRunID) })
			waitForTerminateRunLockWait(t, ctx, pool, terminate)

			close(release)
			if err := activation.wait(ctx); err != nil {
				t.Fatalf("Activate(grandchild) error = %v", err)
			}
			if err := terminate.wait(ctx); err != nil {
				t.Fatalf("TerminateRun(root) error = %v", err)
			}
			assertSubtreeTerminated(t, ctx, pool, rootRunID, grandchildRunID)
		})
	}
}

// newEngineControlAPI serves the platform API router, whose engine control plane owns
// TerminateRun, over the engine test database, with an API key for the default project.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func newEngineControlAPI(t *testing.T, ctx context.Context, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE public.projects SET api_key_hash = $2 WHERE id = $1`,
		enginetest.DefaultPlatformProjectID, middleware.HashAPIKey(terminateTestAPIKey)); err != nil {
		t.Fatalf("set project API key: %v", err)
	}
	platform := platformstore.New(pool)
	var handler http.Handler
	app := fx.New(
		fx.NopLogger,
		fx.Supply(platform, (*ingest.Service)(nil), enginecontrol.NewService(platform),
			&config.Config{Engine: config.EngineConfig{PublicAPIEnabled: true}}),
		api.Module,
		fx.Populate(&handler),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("build API router: %v", err)
	}
	return handler
}

func terminateRunOverAPI(ctx context.Context, handler http.Handler, runID uuid.UUID) error {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/engine/runs/"+runID.String()+"/terminate", nil)
	req.Header.Set("X-API-Key", terminateTestAPIKey)
	req.Header.Set("X-Continua-Engine-Preview", "1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return fmt.Errorf("terminate run %s: HTTP %d: %s", runID, rec.Code, rec.Body.String())
	}
	return nil
}

// waitForTerminateRunLockWait waits until TerminateRun queues for a run lock that another
// backend holds. If termination finishes instead, the assertions after the wait report it.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func waitForTerminateRunLockWait(t *testing.T, ctx context.Context, pool *pgxpool.Pool, terminate *asyncOp) {
	t.Helper()
	for !terminate.finished() {
		var waiting bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
			    SELECT 1 FROM pg_stat_activity
			    WHERE datname = current_database()
			      AND query LIKE '%GetRunByProjectAndIDForUpdate%'
			      AND cardinality(pg_blocking_pids(pid)) > 0
			)
		`).Scan(&waiting); err != nil {
			t.Fatalf("read run lock waits: %v", err)
		}
		if waiting {
			return
		}
	}
}

// assertSubtreeTerminated checks every run under the root, including the run the
// grandchild added: each run is terminated, except the grandchild when it continued as
// new, and each projected trace points at its run's last history event.
//
//nolint:revive // Keep testing.T first in test helper signatures.
func assertSubtreeTerminated(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rootRunID, grandchildRunID uuid.UUID) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT r.id, r.status, r.continued_to_run_id IS NOT NULL,
		       t.engine_latest_history_id, (SELECT max(h.id) FROM engine.history AS h WHERE h.run_id = r.id)
		FROM engine.runs AS r
		JOIN public.traces AS t ON t.engine_run_id = r.id
		WHERE r.root_run_id = $1
		ORDER BY r.child_depth, r.created_at
	`, rootRunID)
	if err != nil {
		t.Fatalf("list subtree runs: %v", err)
	}
	defer rows.Close()
	runs := 0
	for rows.Next() {
		var (
			runID            uuid.UUID
			status           enginedb.EngineRunLifecycleStatus
			continued        bool
			projectedHistory int64
			latestHistory    int64
		)
		if err := rows.Scan(&runID, &status, &continued, &projectedHistory, &latestHistory); err != nil {
			t.Fatalf("scan subtree run: %v", err)
		}
		runs++
		want := enginedb.EngineRunLifecycleStatusTerminated
		if runID == grandchildRunID && continued {
			want = enginedb.EngineRunLifecycleStatusContinuedAsNew
		}
		if status != want {
			t.Errorf("run %s has status %q, want %q", runID, status, want)
		}
		if projectedHistory != latestHistory {
			t.Errorf("run %s trace points at history %d, want its last event %d", runID, projectedHistory, latestHistory)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read subtree runs: %v", err)
	}
	// The root, its child, the grandchild, and the run the grandchild added.
	if runs != 4 {
		t.Fatalf("expected 4 runs under root %s, got %d", rootRunID, runs)
	}
}
