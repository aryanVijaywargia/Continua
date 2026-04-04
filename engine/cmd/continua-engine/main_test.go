package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	enginestore "github.com/continua-ai/continua/engine/internal/store"
	enginetest "github.com/continua-ai/continua/engine/internal/testutil"
)

func TestVersionCommand(t *testing.T) {
	stdout, stderr, err := executeCommand(t, "version")
	if err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "continua-engine") {
		t.Fatalf("expected version output to mention continua-engine, got %q", stdout)
	}
}

func TestMigrateDownRequiresStepCount(t *testing.T) {
	_, _, err := executeCommand(t, "migrate", "down")
	if err == nil {
		t.Fatal("expected migrate down without steps to fail")
	}
	if !strings.Contains(err.Error(), "step count required: use continua-engine migrate down <steps>") {
		t.Fatalf("expected explicit step count guidance, got %q", err.Error())
	}
}

func TestMigrateCommandsRoundTrip(t *testing.T) {
	db := enginetest.NewTestDatabase(t)

	t.Setenv("ENGINE_DATABASE_URL", db.DatabaseURL)
	t.Setenv("DATABASE_URL", "")

	stdout, stderr, err := executeCommand(t, "migrate", "down", "1")
	if err != nil {
		t.Fatalf("execute migrate down 1: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Rolled back 1 engine migration step(s)") {
		t.Fatalf("unexpected migrate down output: %q", stdout)
	}

	if !schemaExists(t, db.DatabaseURL, "engine") {
		t.Fatal("expected engine schema to remain after rolling back only the runtime migration")
	}
	for _, column := range []string{"result", "custom_status", "waiting_for", "completed_at"} {
		if columnExists(t, db.DatabaseURL, "engine", "runs", column) {
			t.Fatalf("expected engine.runs.%s to be removed after migrate down 1", column)
		}
	}
	if enumLabelExists(t, db.DatabaseURL, "engine", "run_lifecycle_status", "waiting") {
		t.Fatal("expected waiting enum label to be removed after migrate down 1")
	}

	stdout, stderr, err = executeCommand(t, "migrate", "up")
	if err != nil {
		t.Fatalf("execute migrate up: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Engine migrations applied successfully") {
		t.Fatalf("unexpected migrate up output: %q", stdout)
	}

	if !schemaExists(t, db.DatabaseURL, "engine") {
		t.Fatal("expected engine schema to exist after migrate up")
	}
	for _, column := range []string{"result", "custom_status", "waiting_for", "completed_at"} {
		if !columnExists(t, db.DatabaseURL, "engine", "runs", column) {
			t.Fatalf("expected engine.runs.%s to exist after migrate up", column)
		}
	}
	if !enumLabelExists(t, db.DatabaseURL, "engine", "run_lifecycle_status", "waiting") {
		t.Fatal("expected waiting enum label to exist after migrate up")
	}

	stdout, stderr, err = executeCommand(t, "migrate", "up")
	if err != nil {
		t.Fatalf("execute second migrate up: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on second migrate up, got %q", stderr)
	}
	if !strings.Contains(stdout, "Engine migrations applied successfully") {
		t.Fatalf("unexpected second migrate up output: %q", stdout)
	}
}

func TestMigrateDownRejectsWaitingRuns(t *testing.T) {
	db := enginetest.NewTestDatabase(t)

	t.Setenv("ENGINE_DATABASE_URL", db.DatabaseURL)
	t.Setenv("DATABASE_URL", "")

	store := enginestore.New(db.Pool)
	ctx := context.Background()
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	instance, err := store.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "migration-waiting-guard",
		DefinitionName: "demo",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	run, err := store.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        instance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	claimed, err := store.ClaimNextRun(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextRun() error = %v", err)
	}
	if _, err := store.TransitionRunToWaiting(ctx, enginedb.TransitionRunToWaitingParams{
		ID:           claimed.ID,
		ClaimedBy:    claimed.ClaimedBy,
		WaitingFor:   []byte(`{"kind":"signal","signal_name":"approval"}`),
		CustomStatus: []byte(`{"phase":"signal"}`),
	}); err != nil {
		t.Fatalf("TransitionRunToWaiting() error = %v", err)
	}

	stdout, stderr, err := executeCommand(t, "migrate", "down", "1")
	if err == nil {
		t.Fatal("expected migrate down 1 to fail when waiting rows exist")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout on failed rollback, got %q", stdout)
	}
	if !strings.Contains(err.Error(), "cannot roll back engine runtime columns while engine.runs still contains waiting rows") &&
		!strings.Contains(stderr, "cannot roll back engine runtime columns while engine.runs still contains waiting rows") {
		t.Fatalf("expected waiting-row rollback guard, err=%q stderr=%q", err, stderr)
	}

	if !columnExists(t, db.DatabaseURL, "engine", "runs", "waiting_for") {
		t.Fatal("expected waiting_for column to remain after failed rollback")
	}
	if !enumLabelExists(t, db.DatabaseURL, "engine", "run_lifecycle_status", "waiting") {
		t.Fatal("expected waiting enum label to remain after failed rollback")
	}

	updatedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updatedRun.Status != enginedb.EngineRunLifecycleStatusWaiting {
		t.Fatalf("expected waiting run to remain unchanged after failed rollback, got %+v", updatedRun)
	}
}

func executeCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func schemaExists(t *testing.T, databaseURL string, schemaName string) bool {
	t.Helper()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open direct pool: %v", err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.schemata
			WHERE schema_name = $1
		)
	`, schemaName).Scan(&exists); err != nil {
		t.Fatalf("check schema existence: %v", err)
	}

	return exists
}

func columnExists(t *testing.T, databaseURL string, schemaName string, tableName string, columnName string) bool {
	t.Helper()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open direct pool: %v", err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = $1
			  AND table_name = $2
			  AND column_name = $3
		)
	`, schemaName, tableName, columnName).Scan(&exists); err != nil {
		t.Fatalf("check column existence: %v", err)
	}

	return exists
}

func enumLabelExists(t *testing.T, databaseURL string, schemaName string, typeName string, label string) bool {
	t.Helper()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open direct pool: %v", err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_type t
			JOIN pg_namespace n ON n.oid = t.typnamespace
			JOIN pg_enum e ON e.enumtypid = t.oid
			WHERE n.nspname = $1
			  AND t.typname = $2
			  AND e.enumlabel = $3
		)
	`, schemaName, typeName, label).Scan(&exists); err != nil {
		t.Fatalf("check enum label existence: %v", err)
	}

	return exists
}
