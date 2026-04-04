package testutil

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/continua-ai/continua/engine/internal/migrations"
)

// DefaultTestDatabaseURL is the fallback test database URL for engine tests.
//
// PG 13+ is required because the platform migrations rely on gen_random_uuid()
// as a built-in function.
const DefaultTestDatabaseURL = "postgres://continua:continua@localhost:5432/continua_test?sslmode=disable"

// TestDatabase is an isolated Postgres database prepared with both platform and
// engine migrations.
type TestDatabase struct {
	Pool        *pgxpool.Pool
	DatabaseURL string
	RepoRoot    string
}

// NewTestDatabase provisions a fresh database, applies platform then engine
// migrations, and returns a pool connected to the isolated test database.
func NewTestDatabase(t *testing.T) *TestDatabase {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping engine integration test in short mode")
	}

	ctx := context.Background()
	baseDatabaseURL := testDatabaseURL()

	adminPool, err := pgxpool.New(ctx, baseDatabaseURL)
	if err != nil {
		t.Skipf("Skipping engine integration test: could not connect to admin database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Skipf("Skipping engine integration test: could not ping admin database: %v", err)
	}

	databaseName := "continua_engine_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quoteIdentifier(databaseName))); err != nil {
		adminPool.Close()
		t.Skipf("Skipping engine integration test: could not create fresh test database: %v", err)
	}

	databaseURL, err := withDatabaseName(baseDatabaseURL, databaseName)
	if err != nil {
		adminPool.Close()
		t.Fatalf("build isolated database URL: %v", err)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		adminPool.Close()
		t.Fatalf("find repo root: %v", err)
	}

	platformMigrationsDir := filepath.Join(repoRoot, "db", "platform", "migrations", "postgres")
	engineMigrationsDir := filepath.Join(repoRoot, "engine", "db", "migrations", "postgres")

	if err := applyMigrations(databaseURL, platformMigrationsDir, ""); err != nil {
		_ = dropDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("apply platform migrations: %v", err)
	}

	if err := applyMigrations(databaseURL, engineMigrationsDir, migrations.EngineMigrationsTable); err != nil {
		_ = dropDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("apply engine migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		_ = dropDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("connect to isolated test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = dropDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("ping isolated test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		if err := dropDatabase(ctx, adminPool, databaseName); err != nil {
			t.Fatalf("drop isolated test database: %v", err)
		}
		adminPool.Close()
	})

	return &TestDatabase{
		Pool:        pool,
		DatabaseURL: databaseURL,
		RepoRoot:    repoRoot,
	}
}

// NullableUUID converts a UUID into a pgtype.UUID suitable for nullable sqlc parameters.
func NullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// Ptr returns a pointer to the provided value.
func Ptr[T any](value T) *T {
	return &value
}

func testDatabaseURL() string {
	if value := os.Getenv("ENGINE_TEST_DATABASE_URL"); value != "" {
		return value
	}
	if value := os.Getenv("TEST_DATABASE_URL"); value != "" {
		return value
	}
	return DefaultTestDatabaseURL
}

func withDatabaseName(databaseURL, databaseName string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func applyMigrations(databaseURL, migrationsDir, migrationsTable string) error {
	migrator, err := migrations.NewWithTable(databaseURL, os.DirFS(migrationsDir), migrationsTable)
	if err != nil {
		return err
	}
	defer func() {
		_ = migrations.Close(migrator)
	}()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func dropDatabase(ctx context.Context, adminPool *pgxpool.Pool, databaseName string) error {
	if _, err := adminPool.Exec(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1
		  AND pid <> pg_backend_pid()
	`, databaseName); err != nil {
		return fmt.Errorf("terminate active connections: %w", err)
	}

	if _, err := adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(databaseName))); err != nil {
		return fmt.Errorf("drop database: %w", err)
	}
	return nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func findRepoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}

	dir := filepath.Dir(file)
	for {
		if exists(filepath.Join(dir, "go.work")) || exists(filepath.Join(dir, "Makefile")) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repo root not found")
		}
		dir = parent
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
