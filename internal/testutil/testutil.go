// Package testutil provides shared utilities for testing.
package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// DefaultTestDBURL is the default database URL for integration tests.
const DefaultTestDBURL = "postgres://continua:continua@localhost:5432/continua_test?sslmode=disable"

// TestDB returns a connection pool for testing.
// Requires TEST_DATABASE_URL environment variable or uses DefaultTestDBURL.
// Skips the test if the database is not available.
func TestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = DefaultTestDBURL
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping test: could not connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("Skipping test: could not ping test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// CreateTestProject creates a test project and returns its ID.
// Automatically cleans up the project after the test.
//
//nolint:revive // Keep testing.T first in shared test helper signatures.
func CreateTestProject(t *testing.T, ctx context.Context, q *platform.Queries) uuid.UUID {
	t.Helper()

	project, err := q.CreateProject(ctx, platform.CreateProjectParams{
		Name:       "test-project-" + uuid.New().String()[:8],
		ApiKeyHash: "test-api-key-" + uuid.New().String(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		// Best-effort cleanup - deletes cascade from project
		// Note: DeleteProject doesn't exist in SQLC queries, rely on test DB cleanup
	})

	return project.ID
}

// CreateTestTrace creates a test trace and returns its UUID.
//
//nolint:revive // Keep testing.T first in shared test helper signatures.
func CreateTestTrace(t *testing.T, ctx context.Context, q *platform.Queries, projectID uuid.UUID, traceID string) uuid.UUID {
	t.Helper()

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
	})
	require.NoError(t, err)

	return trace.ID
}

// Ptr returns a pointer to the given value.
// Useful for creating pointers to literals in test code.
func Ptr[T any](v T) *T {
	return &v
}

// StrPtr returns a pointer to the given string.
func StrPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int {
	return &i
}

// Int32Ptr returns a pointer to the given int32.
func Int32Ptr(i int32) *int32 {
	return &i
}

// Int64Ptr returns a pointer to the given int64.
func Int64Ptr(i int64) *int64 {
	return &i
}

// BoolPtr returns a pointer to the given bool.
func BoolPtr(b bool) *bool {
	return &b
}

// UniqueID returns a unique ID string for use in tests.
func UniqueID(prefix string) string {
	return prefix + "-" + uuid.New().String()[:8]
}

// PgtypeUUID converts a uuid.UUID to pgtype.UUID for use in SQLC params.
func PgtypeUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// PgtypeTimestamptz converts a time.Time to pgtype.Timestamptz.
func PgtypeTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// PgtypeTimestamptzPtr converts a *time.Time to pgtype.Timestamptz.
func PgtypeTimestamptzPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// PgtypeNumericFromFloat64 converts a float64 to pgtype.Numeric.
func PgtypeNumericFromFloat64(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(f)
	return n
}
