package projection_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginehistory "github.com/continua-ai/continua/engine/internal/history"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
	"github.com/continua-ai/continua/engine/pkg/projection"
)

// A history row appended after a purge (a run that finished before #359 can still take a
// retry report) must not move a purged trace back to catching_up.
func TestUpdateLatestHistoryKeepsPurgedProjection(t *testing.T) {
	for _, state := range []projection.State{projection.StateSummaryOnly, projection.StateJournalExpired} {
		t.Run(state.String(), func(t *testing.T) {
			ctx := context.Background()
			db := enginetest.NewTestDatabase(t)
			projectID := enginetest.DefaultPlatformProjectID
			enginetest.EnsurePlatformProject(t, db.Pool, projectID)
			store := enginestore.New(db.Pool)
			instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
				ProjectID:      projectID,
				InstanceKey:    "purged-" + state.String() + "-" + uuid.NewString(),
				DefinitionName: "purged.projection",
			})
			if err != nil {
				t.Fatalf("CreateInstance() error = %v", err)
			}
			run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
				ProjectID:         projectID,
				InstanceID:        instance.ID,
				RunNumber:         1,
				DefinitionVersion: "v1",
				ReadyAt:           time.Now(),
			})
			if err != nil {
				t.Fatalf("CreateRun() error = %v", err)
			}
			started, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
				ProjectID:  projectID,
				InstanceID: instance.ID,
				RunID:      run.ID,
				SequenceNo: 1,
				EventType:  enginehistory.EventWorkflowStarted,
				Payload:    []byte(`{}`),
			})
			if err != nil {
				t.Fatalf("AppendHistory() error = %v", err)
			}
			enginetest.SeedProjectionShell(t, db.Pool, &instance, &run, "purged.projection", "v1", nil, started.ID)
			if _, err := db.Pool.Exec(ctx, `UPDATE public.traces SET engine_projection_state = $2 WHERE engine_run_id = $1`, run.ID, state.String()); err != nil {
				t.Fatalf("mark trace %s: %v", state, err)
			}

			tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				t.Fatalf("BeginTx() error = %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if err := projection.NewWriter(tx).UpdateLatestHistory(ctx, projectID, run.ID, started.ID+1); err != nil {
				t.Fatalf("UpdateLatestHistory() error = %v", err)
			}
			var got string
			if err := tx.QueryRow(ctx, `SELECT engine_projection_state FROM public.traces WHERE engine_run_id = $1`, run.ID).Scan(&got); err != nil {
				t.Fatalf("read projection state: %v", err)
			}
			if got != state.String() {
				t.Fatalf("projection state = %q, want %q", got, state)
			}
		})
	}
}
