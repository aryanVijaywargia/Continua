package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

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

	if schemaExists(t, db.DatabaseURL, "engine") {
		t.Fatal("expected engine schema to be removed after migrate down")
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
