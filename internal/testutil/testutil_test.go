package testutil

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// Parallel test packages each migrate the shared test database on startup. On a
// fresh database those migrators overlap the engine's CREATE INDEX CONCURRENTLY
// migrations, which must not deadlock or leave a dirty version.
func TestApplyRepoMigrationsConcurrentlyOnFreshDatabase(t *testing.T) {
	admin := TestDB(t)
	ctx := context.Background()

	databaseName := "continua_testutil_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Skipf("Skipping concurrent migration test: could not create fresh database: %v", err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), "DROP DATABASE "+quotedName+" WITH (FORCE)")
		require.NoError(t, err)
	})

	freshURL, err := url.Parse(testDatabaseURL())
	require.NoError(t, err)
	freshURL.Path = "/" + databaseName

	const migrators = 4
	errs := make(chan error, migrators)
	for range migrators {
		go func() { errs <- applyRepoMigrations(freshURL.String()) }()
	}
	for range migrators {
		require.NoError(t, <-errs)
	}

	conn, err := pgx.Connect(ctx, freshURL.String())
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close(ctx)) }()
	for _, table := range []string{"schema_migrations", "engine_schema_migrations"} {
		var dirty bool
		require.NoError(t, conn.QueryRow(ctx, "SELECT dirty FROM "+table).Scan(&dirty))
		require.False(t, dirty, "%s is dirty", table)
	}
}
