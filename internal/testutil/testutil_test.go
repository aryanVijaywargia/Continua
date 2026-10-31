package testutil

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// Parallel test packages each migrate the shared test database on startup. On a
// fresh database those migrators overlap the engine's CREATE INDEX CONCURRENTLY
// migrations, which must not deadlock or leave a dirty version.
func TestApplyRepoMigrationsConcurrentlyOnFreshDatabase(t *testing.T) {
	ctx := context.Background()
	freshURL := freshTestDatabase(t)

	const migrators = 4
	errs := make(chan error, migrators)
	for range migrators {
		go func() { errs <- applyRepoMigrations(ctx, freshURL) }()
	}
	for range migrators {
		require.NoError(t, <-errs)
	}

	conn, err := pgx.Connect(ctx, freshURL)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close(ctx)) }()
	// Every migrator closed its lock session, so only this one remains.
	requireNoOtherSessions(t, conn)
	for _, table := range []string{"schema_migrations", "engine_schema_migrations"} {
		var dirty bool
		require.NoError(t, conn.QueryRow(ctx, "SELECT dirty FROM "+table).Scan(&dirty))
		require.False(t, dirty, "%s is dirty", table)
	}
}

// A stalled lock holder must not block a migrator past its context deadline,
// and the migrator must close its lock session when it gives up.
func TestApplyRepoMigrationsStopsWaitingForAHeldLock(t *testing.T) {
	freshURL := freshTestDatabase(t)

	holder, err := pgx.Connect(context.Background(), freshURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, holder.Close(context.Background())) })
	_, err = holder.Exec(context.Background(), "SELECT pg_advisory_lock($1)", testMigrationLockKey)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- applyRepoMigrations(ctx, freshURL) }()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("applyRepoMigrations kept waiting for the held lock after its context expired")
	}
	requireNoOtherSessions(t, holder)
}

// freshTestDatabase creates an empty database next to the test database and
// returns its URL. The database is dropped when the test ends.
func freshTestDatabase(t *testing.T) string {
	t.Helper()
	admin := TestDB(t)

	databaseName := "continua_testutil_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+quotedName); err != nil {
		t.Skipf("Skipping: could not create fresh database: %v", err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), "DROP DATABASE "+quotedName+" WITH (FORCE)")
		require.NoError(t, err)
	})

	freshURL, err := url.Parse(testDatabaseURL())
	require.NoError(t, err)
	freshURL.Path = "/" + databaseName
	return freshURL.String()
}

// requireNoOtherSessions waits until conn is the only session on its database.
// Postgres ends a backend shortly after its client disconnects, so it polls.
func requireNoOtherSessions(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	require.Eventually(t, func() bool {
		var others int
		err := conn.QueryRow(context.Background(),
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()",
		).Scan(&others)
		return err == nil && others == 0
	}, 5*time.Second, 20*time.Millisecond, "a lock or migration session is still open")
}
