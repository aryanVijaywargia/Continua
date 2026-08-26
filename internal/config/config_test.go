package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/config"
)

func TestLoad_DefaultsHostToLoopback(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HOST", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
}

func TestLoad_RejectsInvalidTrueAsyncDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("INGEST_TRUE_ASYNC_DEFAULT", "definitely")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "INGEST_TRUE_ASYNC_DEFAULT")
}

func TestLoad_RejectsInvalidWorkerCount(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("RIVER_QUEUE_INGEST_WORKERS", "-1")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "RIVER_QUEUE_INGEST_WORKERS")
}

func TestLoad_RejectsHistoryRetentionWithoutProjectionRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ENGINE_HISTORY_RETENTION_AFTER", "720h")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "ENGINE_HISTORY_RETENTION_AFTER")
	assert.Contains(t, err.Error(), "ENGINE_PROJECTION_RETENTION_AFTER")
}

func TestLoad_RejectsHistoryRetentionNotGreaterThanProjectionRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ENGINE_PROJECTION_RETENTION_AFTER", "168h")
	t.Setenv("ENGINE_HISTORY_RETENTION_AFTER", "168h")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "ENGINE_HISTORY_RETENTION_AFTER")
}

func TestLoad_AllowsProjectionOnlyRetentionAndZeroDisable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ENGINE_PROJECTION_RETENTION_AFTER", "168h")
	t.Setenv("ENGINE_HISTORY_RETENTION_AFTER", "0")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 168*time.Hour, cfg.Engine.ProjectionRetentionAfter)
	assert.Zero(t, cfg.Engine.HistoryRetentionAfter)
}

func TestLoad_RejectsUnparseableRetentionDuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ENGINE_PROJECTION_RETENTION_AFTER", "not-a-duration")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "ENGINE_PROJECTION_RETENTION_AFTER")
}

func TestLoad_LeaseCompletionGrace(t *testing.T) {
	t.Run("configured duration", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://example")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "15s")

		cfg, err := config.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, 15*time.Second, cfg.Engine.LeaseCompletionGrace)
	})

	t.Run("default is zero", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://example")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "")

		cfg, err := config.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Zero(t, cfg.Engine.LeaseCompletionGrace)
	})

	t.Run("negative duration rejected", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://example")
		t.Setenv("ENGINE_LEASE_COMPLETION_GRACE", "-5s")

		cfg, err := config.Load()
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "ENGINE_LEASE_COMPLETION_GRACE")
	})
}

func TestLocalSingleUserModeDefaultsDisabled(t *testing.T) {
	t.Run("unset environment leaves local single-user mode off", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://example")
		t.Setenv("LOCAL_SINGLE_USER_MODE", "")

		cfg, err := config.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.False(t, cfg.LocalSingleUserMode)
	})

	t.Run("explicit opt-in enables local single-user mode", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://example")
		t.Setenv("LOCAL_SINGLE_USER_MODE", "true")

		cfg, err := config.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.True(t, cfg.LocalSingleUserMode)
	})
}

func TestLocalSingleUserModeRejectedWithPublicDemo(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("LOCAL_SINGLE_USER_MODE", "true")
	t.Setenv("PUBLIC_DEMO_ENABLED", "true")
	t.Setenv("PUBLIC_DEMO_PROJECT_ID", "11111111-1111-1111-1111-111111111111")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "LOCAL_SINGLE_USER_MODE")
	assert.Contains(t, err.Error(), "PUBLIC_DEMO_ENABLED")
}

func TestLoad_RejectsPublicDemoWithoutProjectID(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("PUBLIC_DEMO_ENABLED", "true")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "PUBLIC_DEMO_PROJECT_ID")
}

func TestLoad_RejectsInvalidPublicDemoProjectID(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("PUBLIC_DEMO_ENABLED", "true")
	t.Setenv("PUBLIC_DEMO_PROJECT_ID", "not-a-uuid")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "PUBLIC_DEMO_PROJECT_ID")
}

func TestLoad_DefaultsPublicDemoLabel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("PUBLIC_DEMO_ENABLED", "true")
	t.Setenv("PUBLIC_DEMO_PROJECT_ID", "11111111-1111-1111-1111-111111111111")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.PublicDemo.Enabled)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", cfg.PublicDemo.ProjectID.String())
	assert.Equal(t, "Sample data", cfg.PublicDemo.Label)
}
