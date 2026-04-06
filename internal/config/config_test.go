package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/config"
)

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
