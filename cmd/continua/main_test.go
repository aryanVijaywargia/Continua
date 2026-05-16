package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/testutil"
)

func TestRunMigrationsDownRejectsTerminatedTracesWithIdentifiers(t *testing.T) {
	db := newIsolatedPlatformTestDatabase(t)
	t.Setenv("DATABASE_URL", db.databaseURL)

	_, err := db.pool.Exec(context.Background(), `
		INSERT INTO projects (id, name, api_key_hash)
		VALUES ($1, $2, $3)
	`, defaultPlatformProjectID, "rollback guard test", "test-api-key-hash")
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	traceRowID := uuid.New()
	traceExternalID := "terminated-rollback-trace"
	_, err = db.pool.Exec(context.Background(), `
		INSERT INTO traces (
		    id,
		    project_id,
		    trace_id,
		    name,
		    status,
		    start_time,
		    engine_run_status
		)
		VALUES ($1, $2, $3, $4, 'error', NOW(), 'terminated')
	`, traceRowID, defaultPlatformProjectID, traceExternalID, "terminated rollback guard")
	if err != nil {
		t.Fatalf("insert terminated trace: %v", err)
	}

	err = runMigrationsDown(5)
	if err == nil {
		t.Fatal("expected migrate down 5 to fail when terminated trace rows exist")
	}

	message := err.Error()
	if !strings.Contains(message, traceRowID.String()) || !strings.Contains(message, traceExternalID) {
		t.Fatalf("expected rollback error to identify offending trace row, got %q", message)
	}
	if !strings.Contains(message, "terminated rows") {
		t.Fatalf("expected rollback error to mention terminated rows, got %q", message)
	}
}

func TestNewServerApp_RejectsInvalidRetentionConfiguration(t *testing.T) {
	testCases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "history without projection",
			env: map[string]string{
				"ENGINE_HISTORY_RETENTION_AFTER": "720h",
			},
			wantErr: "ENGINE_HISTORY_RETENTION_AFTER requires ENGINE_PROJECTION_RETENTION_AFTER",
		},
		{
			name: "history not greater than projection",
			env: map[string]string{
				"ENGINE_PROJECTION_RETENTION_AFTER": "168h",
				"ENGINE_HISTORY_RETENTION_AFTER":    "168h",
			},
			wantErr: "ENGINE_HISTORY_RETENTION_AFTER must be greater than ENGINE_PROJECTION_RETENTION_AFTER",
		},
		{
			name: "unparseable projection retention",
			env: map[string]string{
				"ENGINE_PROJECTION_RETENTION_AFTER": "not-a-duration",
			},
			wantErr: "ENGINE_PROJECTION_RETENTION_AFTER must be a valid duration",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://continua:continua@localhost:5432/continua?sslmode=disable")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			app := newServerApp()
			require.Error(t, app.Err())
			assert.Contains(t, app.Err().Error(), tc.wantErr)
		})
	}
}

var defaultPlatformProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type isolatedPlatformTestDatabase struct {
	pool        *pgxpool.Pool
	databaseURL string
}

func newIsolatedPlatformTestDatabase(t *testing.T) *isolatedPlatformTestDatabase {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	baseDatabaseURL := os.Getenv("TEST_DATABASE_URL")
	if baseDatabaseURL == "" {
		baseDatabaseURL = testutil.DefaultTestDBURL
	}

	adminPool, err := pgxpool.New(ctx, baseDatabaseURL)
	if err != nil {
		t.Skipf("Skipping platform migration test: could not connect to admin database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Skipf("Skipping platform migration test: could not ping admin database: %v", err)
	}

	databaseName := "continua_platform_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quoteIdentifier(databaseName))); err != nil {
		adminPool.Close()
		t.Skipf("Skipping platform migration test: could not create isolated database: %v", err)
	}

	databaseURL, err := withDatabaseName(baseDatabaseURL, databaseName)
	if err != nil {
		adminPool.Close()
		t.Fatalf("build isolated database URL: %v", err)
	}

	migrator, err := newMigrator(databaseURL)
	if err != nil {
		_ = dropIsolatedDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("create migrator: %v", err)
	}
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		closeMigrator(migrator)
		_ = dropIsolatedDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	closeMigrator(migrator)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		_ = dropIsolatedDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("connect isolated database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = dropIsolatedDatabase(ctx, adminPool, databaseName)
		adminPool.Close()
		t.Fatalf("ping isolated database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		if err := dropIsolatedDatabase(ctx, adminPool, databaseName); err != nil {
			t.Fatalf("drop isolated database: %v", err)
		}
		adminPool.Close()
	})

	return &isolatedPlatformTestDatabase{
		pool:        pool,
		databaseURL: databaseURL,
	}
}

func dropIsolatedDatabase(ctx context.Context, adminPool *pgxpool.Pool, databaseName string) error {
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

func withDatabaseName(databaseURL, databaseName string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
