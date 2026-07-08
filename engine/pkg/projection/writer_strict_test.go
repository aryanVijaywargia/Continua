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

func TestGuardedWriteFailsWithoutShellForFormerSentinelProject(t *testing.T) {
	ctx := context.Background()
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	enginetest.EnsurePlatformProject(t, db.Pool, projectID)

	store := enginestore.New(db.Pool)
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "former-sentinel-no-shell",
		DefinitionName: "strict.no-shell",
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

	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	err = projection.NewWriter(tx).SyncRunSummary(ctx, &run)
	if err == nil {
		t.Fatal("SyncRunSummary() error = nil, want missing trace shell error")
	}
}

func TestEnsureStartShellWritesShellUnderRunProject(t *testing.T) {
	ctx := context.Background()
	db := enginetest.NewTestDatabase(t)
	projectID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `DELETE FROM public.projects`); err != nil {
		t.Fatalf("delete seeded projects: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO public.projects (id, name, api_key_hash)
		VALUES ($1, $2, $3)
	`, projectID, "Real Engine Project", "real-engine-project-key"); err != nil {
		t.Fatalf("insert real project: %v", err)
	}

	store := enginestore.New(db.Pool)
	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "real-project-instance",
		DefinitionName: "strict.start",
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
	input := []byte(`{"name":"Ada"}`)
	startedPayload, err := enginehistory.MarshalPayload(enginehistory.WorkflowStartedPayload{
		DefinitionName:    "strict.start",
		DefinitionVersion: "v1",
		InstanceKey:       "real-project-instance",
		Input:             input,
	})
	if err != nil {
		t.Fatalf("MarshalPayload(workflow started) error = %v", err)
	}
	startedEvent, err := store.AppendHistory(ctx, enginedb.AppendHistoryParams{
		ProjectID:  projectID,
		InstanceID: instance.ID,
		RunID:      run.ID,
		SequenceNo: 1,
		EventType:  enginehistory.EventWorkflowStarted,
		Payload:    startedPayload,
	})
	if err != nil {
		t.Fatalf("AppendHistory(workflow started) error = %v", err)
	}

	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := projection.NewWriter(tx).EnsureStartShell(ctx, &instance, &run, "strict.start", "v1", input, startedEvent.ID); err != nil {
		t.Fatalf("EnsureStartShell() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	var traceCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM public.traces
		WHERE project_id = $1
		  AND engine_run_id = $2
		  AND trace_id = $3
	`, projectID, run.ID, "engine:"+run.ID.String()).Scan(&traceCount); err != nil {
		t.Fatalf("query projected trace: %v", err)
	}
	if traceCount != 1 {
		t.Fatalf("projected trace count = %d, want 1", traceCount)
	}

	var rootSpanCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM public.spans AS s
		JOIN public.traces AS t ON t.id = s.trace_id
		WHERE s.project_id = $1
		  AND t.engine_run_id = $2
		  AND s.span_id = $3
	`, projectID, run.ID, "engine:root:"+run.ID.String()).Scan(&rootSpanCount); err != nil {
		t.Fatalf("query projected root span: %v", err)
	}
	if rootSpanCount != 1 {
		t.Fatalf("projected root span count = %d, want 1", rootSpanCount)
	}

	var projectCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM public.projects`).Scan(&projectCount); err != nil {
		t.Fatalf("query project count: %v", err)
	}
	if projectCount != 1 {
		t.Fatalf("project count = %d, want only the real project", projectCount)
	}

	var sentinelCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM public.projects
		WHERE id = '00000000-0000-0000-0000-000000000001'
	`).Scan(&sentinelCount); err != nil {
		t.Fatalf("query sentinel project count: %v", err)
	}
	if sentinelCount != 0 {
		t.Fatalf("sentinel project count = %d, want 0", sentinelCount)
	}
}
